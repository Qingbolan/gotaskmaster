package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"encoding/json"

	"github.com/qingbolan/gotaskmaster/config"
)

// Error definitions
var (
	ErrProcessExists   = errors.New("process already exists")
	ErrProcessNotFound = errors.New("process not found")
	ErrProcessRunning  = errors.New("cannot modify running process")
)

// ProcessStatus represents the current status of a process
type ProcessStatus int

const (
	StatusIdle ProcessStatus = iota
	StatusRunning
	StatusStopping
	StatusError
)

func (s ProcessStatus) String() string {
	switch s {
	case StatusIdle:
		return "Idle"
	case StatusRunning:
		return "Running"
	case StatusStopping:
		return "Stopping"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// Process represents a managed process
type Process struct {
	Name         string
	Command      string
	Args         []string
	Env          []string
	WorkDir      string
	MaxInstances int
	LogFile      string
	Instances    int32
	Status       ProcessStatus
	ErrorCount   int32
	LastError    string
	CPU          float64
	Memory       int64
	Uptime       time.Duration
	AutoStart    bool
	Group        string

	// cmd          *exec.Cmd
	cancel       context.CancelFunc
	mu           sync.RWMutex
	logFile      *os.File
	logMutex     sync.Mutex
	// outputBuffer *bufio.Writer
	exitChan     chan struct{}
}

// Manager manages multiple processes
type Manager struct {
	processes map[string]*Process
	mu        sync.RWMutex
	config    *config.Config
}

// NewManager creates a new process manager
func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		processes: make(map[string]*Process),
		config:    cfg,
	}

	log.Printf("Initializing manager with %d processes from config", len(cfg.Processes))

	for _, pc := range cfg.Processes {
		if err := m.AddProcess(&Process{
			Name:         pc.Name,
			Command:      pc.Command,
			Args:         pc.Args,
			Env:          pc.Env,
			WorkDir:      pc.WorkDir,
			MaxInstances: pc.MaxInstances,
			LogFile:      filepath.Join(cfg.LogDir, pc.Name+".log"),
			Status:       StatusIdle,
			AutoStart:    pc.AutoStart,
		}); err != nil {
			log.Printf("Error adding process %s: %v", pc.Name, err)
		} else {
			log.Printf("Successfully added process: %s", pc.Name)
		}
	}

	m.initProcesses()

	return m
}

// AddProcess adds a new process to the manager
func (m *Manager) AddProcess(p *Process) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.processes[p.Name]; exists {
		return fmt.Errorf("%w: %s", ErrProcessExists, p.Name)
	}
	m.processes[p.Name] = p
	return nil
}

// RemoveProcess removes a process from the manager
func (m *Manager) RemoveProcess(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.processes[name]; !exists {
		return fmt.Errorf("%w: %s", ErrProcessNotFound, name)
	}
	delete(m.processes, name)
	return nil
}

// GetProcesses returns a slice of all managed processes
func (m *Manager) GetProcesses() []*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()
	processes := make([]*Process, 0, len(m.processes))
	for _, p := range m.processes {
		processes = append(processes, p)
	}
	return processes
}

// GetProcess returns a specific process by name
func (m *Manager) GetProcess(name string) (*Process, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	p, exists := m.processes[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrProcessNotFound, name)
	}
	return p, nil
}

// initProcesses initializes all processes marked for auto-start
func (m *Manager) initProcesses() {
	for _, p := range m.processes {
		if p.AutoStart {
			log.Printf("Auto-starting process: %s", p.Name)
			if err := m.StartProcess(p.Name); err != nil {
				log.Printf("Failed to auto-start process %s: %v", p.Name, err)
			}
		}
	}
}

// Start starts all processes
func (m *Manager) Start() {
	log.Println("Starting all processes...")
	m.initProcesses()
	log.Println("All processes started.")
}

// Stop stops all managed processes
func (m *Manager) Stop() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.processes {
		if err := m.StopProcess(p.Name); err != nil {
			log.Printf("Error stopping process %s: %v", p.Name, err)
		}
	}
}

// runProcess executes a single instance of a process
func (m *Manager) runProcess(p *Process) {
	if err := p.ensureLogFileExists(); err != nil {
		log.Printf("Failed to ensure log file for process %s: %v", p.Name, err)
		m.updateProcessStatus(p, StatusError)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.exitChan = make(chan struct{})

	defer func() {
		cancel()
		close(p.exitChan)
	}()

	maxRetries := 3
	retryCount := 0

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled for process %s", p.Name)
			m.updateProcessStatus(p, StatusIdle)
			return
		default:
			err := m.executeProcess(ctx, p)
			if err != nil {
				log.Printf("Process %s exited with error: %v", p.Name, err)
				retryCount++
				
				if retryCount >= maxRetries {
					log.Printf("Process %s failed to start after %d attempts. Marking as error.", p.Name, maxRetries)
					m.updateProcessStatus(p, StatusError)
					return
				}
				
				log.Printf("Retrying to start process %s in 5 seconds (attempt %d of %d)...", p.Name, retryCount, maxRetries)
				time.Sleep(1 * time.Second)
			} else {
				log.Printf("Process %s exited normally", p.Name)
				m.updateProcessStatus(p, StatusIdle)
				return
			}
		}
	}
}


func (m *Manager) updateProcessStatus(p *Process, status ProcessStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = status
	// 可以在这里添加额外的状态更新逻辑，比如通知 TUI 更新显示
}

func (m *Manager) executeProcess(ctx context.Context, p *Process) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 打开日志文件
	logFile, err := os.OpenFile(p.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	bufferedWriter := bufio.NewWriter(logFile)
	defer bufferedWriter.Flush()

	// 创建命令
	cmd := exec.CommandContext(ctx, p.Command, p.Args...)
	
	if len(p.Env) > 0 {
		cmd.Env = append(os.Environ(), p.Env...)
	}

	if p.WorkDir != "" {
		cmd.Dir = p.WorkDir
	}

	// 创建 stdout 和 stderr 管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		m.logError(p, fmt.Sprintf("Failed to start process: %v", err), bufferedWriter)
		return fmt.Errorf("failed to start process: %w", err)
	}

	log.Printf("Process %s (PID: %d) started successfully", p.Name, cmd.Process.Pid)
	m.logInfo(p, fmt.Sprintf("Process started (PID: %d)", cmd.Process.Pid), bufferedWriter)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		m.handleOutput(p, stdout, "STDOUT", bufferedWriter)
	}()

	go func() {
		defer wg.Done()
		m.handleOutput(p, stderr, "STDERR", bufferedWriter)
	}()

	// 等待命令完成
	err = cmd.Wait()

	// 等待输出处理完成
	wg.Wait()

	if err != nil {
		m.logError(p, fmt.Sprintf("Process exited with error: %v", err), bufferedWriter)
	} else {
		m.logInfo(p, "Process exited normally", bufferedWriter)
	}

	return err
}

func (m *Manager) logError(p *Process, message string, writer *bufio.Writer) {
	m.writeLog(p, fmt.Sprintf("ERROR: %s", message), writer)
}

func (m *Manager) logInfo(p *Process, message string, writer *bufio.Writer) {
	m.writeLog(p, fmt.Sprintf("INFO: %s", message), writer)
}

func (m *Manager) writeLog(p *Process, message string, writer *bufio.Writer) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] %s\n", timestamp, message)
	
	p.logMutex.Lock()
	defer p.logMutex.Unlock()

	_, err := writer.WriteString(logLine)
	if err != nil {
		log.Printf("Error writing to log file for process %s: %v", p.Name, err)
	}
	writer.Flush()
}

func (m *Manager) handleOutput(p *Process, r io.Reader, prefix string, writer *bufio.Writer) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		m.writeLog(p, fmt.Sprintf("%s: %s", prefix, line), writer)
	}
	if err := scanner.Err(); err != nil {
		m.logError(p, fmt.Sprintf("Error reading %s: %v", prefix, err), writer)
	}
}

// StartProcess starts a specific process
func (m *Manager) StartProcess(name string) error {
	p, err := m.GetProcess(name)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Status != StatusIdle {
		return fmt.Errorf("process %s is not idle", name)
	}

	p.Status = StatusRunning
	atomic.StoreInt32(&p.Instances, 1)

	go m.runProcess(p)

	return nil
}

// StopProcess stops a specific process
func (m *Manager) StopProcess(name string) error {
	p, err := m.GetProcess(name)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Status != StatusRunning {
		return fmt.Errorf("process %s is not running", name)
	}

	p.Status = StatusStopping

	if p.cancel != nil {
		p.cancel()
	}

	select {
	case <-p.exitChan:
		log.Printf("Process %s stopped successfully", name)
	case <-time.After(10 * time.Second):
		log.Printf("Timeout waiting for process %s to stop", name)
	}

	p.Status = StatusIdle
	atomic.StoreInt32(&p.Instances, 0)

	return nil
}

// RestartProcess restarts a specific process
func (m *Manager) RestartProcess(name string) error {
	if err := m.StopProcess(name); err != nil && !errors.Is(err, ErrProcessNotFound) {
		return err
	}
	time.Sleep(time.Second) // Give it a moment to stop
	return m.StartProcess(name)
}

// UpdateProcessConfig updates the configuration of a process
func (m *Manager) UpdateProcessConfig(name string, updatedConfig *Process) error {
	p, err := m.GetProcess(name)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Status == StatusRunning {
		return fmt.Errorf("%w: %s", ErrProcessRunning, name)
	}

	p.Command = updatedConfig.Command
	p.Args = updatedConfig.Args
	p.Env = updatedConfig.Env
	p.WorkDir = updatedConfig.WorkDir
	p.MaxInstances = updatedConfig.MaxInstances
	p.AutoStart = updatedConfig.AutoStart

	return nil
}

// GetProcessLogs retrieves the latest logs for a process
func (m *Manager) GetProcessLogs(name string, lines int) ([]string, error) {
	p, err := m.GetProcess(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	p.logMutex.Lock()
	defer p.logMutex.Unlock()

	logFilePath, err := filepath.Abs(p.LogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for log file: %w", err)
	}

	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		return []string{}, nil
	}

	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("error opening log file (%s): %w", logFilePath, err)
	}
	defer file.Close()

	return readLastLines(file, lines)
}

// readLastLines reads the last n lines from a file
func readLastLines(file *os.File, n int) ([]string, error) {
	scanner := bufio.NewScanner(file)
	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning log file: %w", err)
	}

	return lines, nil
}

// ExportProcessConfig exports the configuration of a process to a file
func (m *Manager) ExportProcessConfig(name, filename string) error {
	p, err := m.GetProcess(name)
	if err != nil {
		return err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating config file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(p); err != nil {
		return fmt.Errorf("error encoding process config: %w", err)
	}

	return nil
}

// ImportProcessConfig imports the configuration of a process from a file
func (m *Manager) ImportProcessConfig(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening config file: %w", err)
	}
	defer file.Close()

	var p Process
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&p); err != nil {
		return fmt.Errorf("error decoding process config: %w", err)
	}

	return m.AddProcess(&p)
}

// ensureLogFileExists ensures that the log file for a process exists
// ensureLogFileExists ensures that the log file for a process exists
func (p *Process) ensureLogFileExists() error {
	// 获取日志文件的目录
	dir := filepath.Dir(p.LogFile)

	// 创建日志目录（如果不存在）
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// 检查日志文件是否已存在
	if _, err := os.Stat(p.LogFile); os.IsNotExist(err) {
		// 如果文件不存在，创建它
		file, err := os.OpenFile(p.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to create log file: %w", err)
		}
		defer file.Close() // 确保文件被关闭

		// 写入一个初始日志条目
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		_, err = file.WriteString(fmt.Sprintf("[%s] Log file created for process: %s\n", timestamp, p.Name))
		if err != nil {
			return fmt.Errorf("failed to write initial log entry: %w", err)
		}
	} else if err != nil {
		// 如果发生了除"文件不存在"之外的错误
		return fmt.Errorf("failed to check log file status: %w", err)
	}

	return nil
}

// GetProcessStatus returns the current status of a process
func (m *Manager) GetProcessStatus(name string) (string, error) {
p, err := m.GetProcess(name)
if err != nil {
	return "", err
}

p.mu.RLock()
defer p.mu.RUnlock()

status := fmt.Sprintf("Name: %s\nStatus: %s\nInstances: %d/%d\nError Count: %d\nLast Error: %s\n",
	p.Name, p.Status, p.Instances, p.MaxInstances, p.ErrorCount, p.LastError)

return status, nil
}

// GetAllProcessesStatus returns the current status of all processes
func (m *Manager) GetAllProcessesStatus() string {
m.mu.RLock()
defer m.mu.RUnlock()

var status strings.Builder
for _, p := range m.processes {
	p.mu.RLock()
	status.WriteString(fmt.Sprintf("Name: %s\nStatus: %s\nInstances: %d/%d\nError Count: %d\nLast Error: %s\n\n",
		p.Name, p.Status, p.Instances, p.MaxInstances, p.ErrorCount, p.LastError))
	p.mu.RUnlock()
}

return status.String()
}

// CleanupProcessLogs removes old log files for a specific process
func (m *Manager) CleanupProcessLogs(name string, olderThan time.Duration) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

logDir := filepath.Dir(p.LogFile)
return filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if !info.IsDir() && strings.HasPrefix(filepath.Base(path), name) && time.Since(info.ModTime()) > olderThan {
		if err := os.Remove(path); err != nil {
			log.Printf("Error removing old log file %s: %v", path, err)
		} else {
			log.Printf("Removed old log file: %s", path)
		}
	}
	return nil
})
}

// RotateProcessLogs rotates the log files for a specific process
func (m *Manager) RotateProcessLogs(name string) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

p.logMutex.Lock()
defer p.logMutex.Unlock()

if p.logFile != nil {
	p.logFile.Close()
	p.logFile = nil
}

timestamp := time.Now().Format("20060102-150405")
newLogFile := fmt.Sprintf("%s.%s", p.LogFile, timestamp)

if err := os.Rename(p.LogFile, newLogFile); err != nil && !os.IsNotExist(err) {
	return fmt.Errorf("error rotating log file: %w", err)
}

return p.ensureLogFileExists()
}

// GetProcessMetrics returns the current metrics for a process
func (m *Manager) GetProcessMetrics(name string) (float64, int64, time.Duration, error) {
p, err := m.GetProcess(name)
if err != nil {
	return 0, 0, 0, err
}

p.mu.RLock()
defer p.mu.RUnlock()

return p.CPU, p.Memory, p.Uptime, nil
}

// UpdateProcessMetrics updates the metrics for a process
func (m *Manager) UpdateProcessMetrics(name string, cpu float64, memory int64, uptime time.Duration) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

p.mu.Lock()
defer p.mu.Unlock()

p.CPU = cpu
p.Memory = memory
p.Uptime = uptime

return nil
}

// SetMaxInstances sets the maximum number of instances for a process
func (m *Manager) SetMaxInstances(name string, maxInstances int) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

p.mu.Lock()
defer p.mu.Unlock()

if p.Status == StatusRunning {
	return fmt.Errorf("%w: %s", ErrProcessRunning, name)
}

p.MaxInstances = maxInstances
return nil
}

// IsProcessRunning checks if a process is currently running
func (m *Manager) IsProcessRunning(name string) (bool, error) {
p, err := m.GetProcess(name)
if err != nil {
	return false, err
}

p.mu.RLock()
defer p.mu.RUnlock()

return p.Status == StatusRunning, nil
}

// WaitForProcess waits for a process to finish or until a timeout occurs
func (m *Manager) WaitForProcess(name string, timeout time.Duration) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

timer := time.NewTimer(timeout)
defer timer.Stop()

select {
case <-p.exitChan:
	return nil
case <-timer.C:
	return fmt.Errorf("timeout waiting for process %s to finish", name)
}
}

// SetEnvironmentVariable sets an environment variable for a process
func (m *Manager) SetEnvironmentVariable(name, key, value string) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

p.mu.Lock()
defer p.mu.Unlock()

if p.Status == StatusRunning {
	return fmt.Errorf("%w: %s", ErrProcessRunning, name)
}

for i, env := range p.Env {
	if strings.HasPrefix(env, key+"=") {
		p.Env[i] = key + "=" + value
		return nil
	}
}

p.Env = append(p.Env, key+"="+value)
return nil
}

// GetEnvironmentVariables returns all environment variables for a process
func (m *Manager) GetEnvironmentVariables(name string) (map[string]string, error) {
p, err := m.GetProcess(name)
if err != nil {
	return nil, err
}

p.mu.RLock()
defer p.mu.RUnlock()

envMap := make(map[string]string)
for _, env := range p.Env {
	parts := strings.SplitN(env, "=", 2)
	if len(parts) == 2 {
		envMap[parts[0]] = parts[1]
	}
}

return envMap, nil
}

// SetGroup sets the group for a process
func (m *Manager) SetGroup(name, group string) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

p.mu.Lock()
defer p.mu.Unlock()

p.Group = group
return nil
}

// GetProcessesByGroup returns all processes in a specific group
func (m *Manager) GetProcessesByGroup(group string) []*Process {
m.mu.RLock()
defer m.mu.RUnlock()

var processes []*Process
for _, p := range m.processes {
	if p.Group == group {
		processes = append(processes, p)
	}
}

return processes
}

// StartGroup starts all processes in a specific group
func (m *Manager) StartGroup(group string) error {
processes := m.GetProcessesByGroup(group)
for _, p := range processes {
	if err := m.StartProcess(p.Name); err != nil {
		return fmt.Errorf("failed to start process %s in group %s: %w", p.Name, group, err)
	}
}
return nil
}

// StopGroup stops all processes in a specific group
func (m *Manager) StopGroup(group string) error {
processes := m.GetProcessesByGroup(group)
for _, p := range processes {
	if err := m.StopProcess(p.Name); err != nil {
		return fmt.Errorf("failed to stop process %s in group %s: %w", p.Name, group, err)
	}
}
return nil
}

// RestartGroup restarts all processes in a specific group
func (m *Manager) RestartGroup(group string) error {
if err := m.StopGroup(group); err != nil {
	return err
}
return m.StartGroup(group)
}

// GetProcessUptime returns the uptime of a process
func (m *Manager) GetProcessUptime(name string) (time.Duration, error) {
p, err := m.GetProcess(name)
if err != nil {
	return 0, err
}

p.mu.RLock()
defer p.mu.RUnlock()

return p.Uptime, nil
}

// ResetErrorCount resets the error count for a process
func (m *Manager) ResetErrorCount(name string) error {
p, err := m.GetProcess(name)
if err != nil {
	return err
}

p.mu.Lock()
defer p.mu.Unlock()

atomic.StoreInt32(&p.ErrorCount, 0)
p.LastError = ""

return nil
}

// GetProcessGroups returns all unique process groups
func (m *Manager) GetProcessGroups() []string {
m.mu.RLock()
defer m.mu.RUnlock()

groupSet := make(map[string]struct{})
for _, p := range m.processes {
	if p.Group != "" {
		groupSet[p.Group] = struct{}{}
	}
}

groups := make([]string, 0, len(groupSet))
for group := range groupSet {
	groups = append(groups, group)
}

return groups
}