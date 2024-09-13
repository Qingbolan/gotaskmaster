package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/qingbolan/gotaskmaster/config"
)

type State int

const (
	StateIdle State = iota
	StateRunning
	StatePaused
	StateError
)

type Process struct {
	Name         string
	Command      string
	Args         []string
	Env          []string
	MaxInstances int
	LogFile      string
	Instances    int
	State        State
	ErrorCount   int
	LastError    string
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	mutex        sync.Mutex
}

type Manager struct {
	processes map[string]*Process
	mutex     sync.RWMutex
	wg        sync.WaitGroup
	config    *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		processes: make(map[string]*Process),
		config:    cfg,
	}

	for _, pc := range cfg.Processes {
		if err := m.AddProcess(&Process{
			Name:         pc.Name,
			Command:      pc.Command,
			Args:         pc.Args,
			Env:          pc.Env,
			MaxInstances: pc.MaxInstances,
			LogFile:      filepath.Join(cfg.LogDir, pc.Name+".log"),
			State:        StateIdle,
		}); err != nil {
			log.Printf("Error adding process %s: %v", pc.Name, err)
		}
	}

	return m
}

func (m *Manager) AddProcess(p *Process) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.processes[p.Name]; exists {
		return fmt.Errorf("process %s already exists", p.Name)
	}
	m.processes[p.Name] = p
	return nil
}

func (m *Manager) RemoveProcess(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.processes[name]; !exists {
		return fmt.Errorf("process %s not found", name)
	}
	delete(m.processes, name)
	return nil
}

func (m *Manager) GetProcesses() []*Process {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	processes := make([]*Process, 0, len(m.processes))
	for _, p := range m.processes {
		processes = append(processes, p)
	}
	return processes
}

func (m *Manager) GetProcess(name string) (*Process, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	p, exists := m.processes[name]
	if !exists {
		return nil, fmt.Errorf("process %s not found", name)
	}
	return p, nil
}

func (m *Manager) Start() {
	for _, p := range m.GetProcesses() {
		m.wg.Add(1)
		go func(proc *Process) {
			defer m.wg.Done()
			m.manageProcess(proc)
		}(p)
	}
}

func (m *Manager) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, p := range m.processes {
		p.mutex.Lock()
		if p.cancel != nil {
			p.cancel()
		}
		p.State = StateIdle
		p.mutex.Unlock()
	}
	m.wg.Wait()
}

func (m *Manager) manageProcess(p *Process) {
	for {
		p.mutex.Lock()
		if p.State == StateIdle {
			p.mutex.Unlock()
			return
		}
		if p.State == StateRunning && p.Instances < p.MaxInstances {
			p.mutex.Unlock()
			m.runProcess(p)
		} else {
			p.mutex.Unlock()
			time.Sleep(5 * time.Second)
		}
	}
}

func (m *Manager) runProcess(p *Process) {
	p.mutex.Lock()
	p.Instances++
	p.mutex.Unlock()

	defer func() {
		p.mutex.Lock()
		p.Instances--
		p.mutex.Unlock()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" && p.Command == "echo" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", "echo", strings.Join(p.Args, " "))
	} else {
		cmd = exec.CommandContext(ctx, p.Command, p.Args...)
	}

	if len(p.Env) > 0 {
		cmd.Env = append(os.Environ(), p.Env...)
	}

	logFile, err := os.OpenFile(p.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		m.logError(p, fmt.Sprintf("Error opening log file: %v", err))
		return
	}
	defer logFile.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.logError(p, fmt.Sprintf("Error creating stdout pipe: %v", err))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.logError(p, fmt.Sprintf("Error creating stderr pipe: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		m.logError(p, fmt.Sprintf("Error starting process: %v", err))
		return
	}

	p.cmd = cmd

	go m.handleOutput(p, stdout, logFile, false)
	go m.handleOutput(p, stderr, logFile, true)

	if err := cmd.Wait(); err != nil {
		if err != context.Canceled {
			m.logError(p, fmt.Sprintf("Process exited with error: %v", err))
		}
	}

	p.mutex.Lock()
	p.cmd = nil
	p.cancel = nil
	p.mutex.Unlock()
}

func (m *Manager) handleOutput(p *Process, r io.Reader, logFile *os.File, isError bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		var logEntry string
		if isError {
			logEntry = fmt.Sprintf("### Error\n[%s] %s: %s\n", timestamp, p.Name, line)
		} else {
			logEntry = fmt.Sprintf("### Info\n[%s] %s: %s\n", timestamp, p.Name, line)
		}
		if _, err := logFile.WriteString(logEntry + "\n"); err != nil {
			log.Printf("Error writing to log file for process %s: %v", p.Name, err)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading output for process %s: %v", p.Name, err)
	}
}

func (m *Manager) logError(p *Process, errMsg string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.ErrorCount++
	p.LastError = errMsg
	log.Printf("Process %s: %s", p.Name, errMsg)
	
	// Update process state to Error
	p.State = StateError
}

func (m *Manager) StartProcess(name string) error {
	p, err := m.GetProcess(name)
	if err != nil {
		return err
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.State == StateRunning {
		return fmt.Errorf("process %s is already running", name)
	}

	p.State = StateRunning
	go m.manageProcess(p)
	return nil
}

func (m *Manager) StopProcess(name string) error {
	p, err := m.GetProcess(name)
	if err != nil {
		return err
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.State != StateRunning {
		return fmt.Errorf("process %s is not running", name)
	}

	p.State = StateIdle
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func (m *Manager) RestartProcess(name string) error {
	if err := m.StopProcess(name); err != nil {
		return err
	}
	time.Sleep(time.Second) // Give it a moment to stop
	return m.StartProcess(name)
}

func (m *Manager) ModifyProcess(name string, newProc *Process) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	oldProc, exists := m.processes[name]
	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	if oldProc.State == StateRunning {
		return fmt.Errorf("cannot modify running process %s", name)
	}

	m.processes[name] = newProc
	return nil
}

func (m *Manager) DeleteProcess(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	p, exists := m.processes[name]
	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	if p.State == StateRunning {
		p.mutex.Lock()
		if p.cancel != nil {
			p.cancel()
		}
		p.State = StateIdle
		p.mutex.Unlock()
		time.Sleep(time.Second) // Give it a moment to stop
	}

	delete(m.processes, name)
	return nil
}

func (m *Manager) ToggleProcessState(name string) error {
	p, err := m.GetProcess(name)
	if err != nil {
		return err
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	switch p.State {
	case StateIdle, StatePaused:
		p.State = StateRunning
		go m.manageProcess(p)
	case StateRunning:
		p.State = StatePaused
		if p.cancel != nil {
			p.cancel()
		}
	}
	return nil
}

func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateRunning:
		return "Running"
	case StatePaused:
		return "Paused"
	case StateError:
		return "Error"
	default:
		return "Unknown"
	}
}