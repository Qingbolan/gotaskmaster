package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"regexp"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/qingbolan/gotaskmaster/process"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

type TUI struct {
	app            *tview.Application
	pages          *tview.Pages
	processList    *tview.List
	processInfo    *tview.TextView
	logView        *tview.TextView
	commandInput   *tview.InputField
	statusBar      *tview.TextView
	errorCount     *tview.TextView
	eventCount     *tview.TextView
	systemStats    *tview.TextView
	healthIndicator *tview.TextView
	manager        *process.Manager
	currentProcess *process.Process
}

func NewTUI(manager *process.Manager) *TUI {
	tui := &TUI{
		app:     tview.NewApplication(),
		pages:   tview.NewPages(),
		manager: manager,
	}
	tui.setupUI()
	return tui
}

func (t *TUI) setupUI() {
	// Main layout
	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Top bar with error and event counts
	topBar := tview.NewFlex().SetDirection(tview.FlexColumn)
	t.errorCount = tview.NewTextView().SetTextColor(tcell.ColorRed)
	t.eventCount = tview.NewTextView().SetTextColor(tcell.ColorGreen)
	t.systemStats = tview.NewTextView().SetTextColor(tcell.ColorYellow)
	topBar.AddItem(t.errorCount, 0, 1, false).
		AddItem(t.eventCount, 0, 1, false).
		AddItem(t.systemStats, 0, 2, false)

	// Process list, info, and logs
	middleFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	t.processList = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetSelectedFunc(t.onProcessSelected)
	
	rightFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	t.processInfo = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true)
	t.healthIndicator = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	t.logView = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetChangedFunc(func() {
			t.app.Draw()
		})

	rightFlex.AddItem(t.healthIndicator, 1, 0, false).
		AddItem(t.processInfo, 0, 2, false).
		AddItem(t.logView, 0, 3, false)

	middleFlex.AddItem(t.processList, 0, 1, true).
		AddItem(rightFlex, 0, 2, false)

	// Command input
	t.commandInput = tview.NewInputField().
		SetLabel("Command: ").
		SetFieldWidth(0).
		SetPlaceholder("Enter command (e.g., [s]start, [t]stop, [r]restart, [q]quit)").
		SetFieldTextColor(tcell.ColorWhite).
		SetPlaceholderTextColor(tcell.ColorYellow).
		SetDoneFunc(t.onCommandEntered)

	// Status bar
	t.statusBar = tview.NewTextView().
		SetTextColor(tcell.ColorYellow)

	// Add all components to main layout
	mainFlex.AddItem(topBar, 1, 1, false).
		AddItem(middleFlex, 0, 1, true).
		AddItem(t.commandInput, 1, 1, false).
		AddItem(t.statusBar, 1, 1, false)

	// Set up pages
	t.pages.AddPage("main", mainFlex, true, true)

	// Set up key bindings
	t.setupKeyBindings()

	t.app.SetRoot(t.pages, true).EnableMouse(true)
}

func (t *TUI) setupKeyBindings() {
    t.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
        switch event.Key() {
		case 'q':
        case tcell.KeyCtrlC:
            t.app.Stop()
        case tcell.KeyCtrlR:
            t.refreshProcessList()
        case tcell.KeyCtrlL:
            t.showLogView()
        case tcell.KeyCtrlE:
            t.showConfigEditor()
        case tcell.KeyCtrlH:
            t.showHelp()
        }
        return event
    })

    t.processList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
        switch event.Rune() {
        case 's', 't', 'r':
            if process := t.getSelectedProcess(); process != nil {
                switch event.Rune() {
                case 's':
                    t.startSelectedProcess()
                case 't':
                    t.stopSelectedProcess()
                case 'r':
                    t.restartSelectedProcess()
                }
                t.refreshProcessList()
            }
        case 'e':
            t.showConfigEditor()
        case 'l':
            t.showLogView()
        }
        return event
    })
}


func (t *TUI) updateProcessInfo(p *process.Process) {
	if p == nil {
		t.processInfo.SetText("No process selected")
		t.healthIndicator.SetText("")
		return
	}
	
	status := ""
	switch p.Status {
	case process.StatusRunning:
		status = "● Running"
	case process.StatusStopping:
		status = "○ Stopped"
	default:
		status = "◍ Unknown"
	}
	
	info := fmt.Sprintf("Name: %s\nStatus: %s\nCommand: %s\nArgs: %v\nWork Dir: %s\nInstances: %d/%d\nError Count: %d\nLast Error: %s\nAuto Start: %v\n",
		p.Name, status, p.Command, p.Args, p.WorkDir, p.Instances, p.MaxInstances, p.ErrorCount, p.LastError, p.AutoStart)
	t.processInfo.SetText(info)
}


func (t *TUI) Run() error {
	t.refreshProcessList()
	go t.periodicUpdates()
	return t.app.Run()
}

func (t *TUI) periodicUpdates() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		t.app.QueueUpdateDraw(func() {
			t.refreshProcessList()
			t.updateErrorCount()
			t.updateEventCount()
			t.updateSystemStats()
			if t.currentProcess != nil {
				t.updateProcessInfo(t.currentProcess)
				t.updateLogView(t.currentProcess)
			}
		})
	}
}

func (t *TUI) updateSystemStats() {
	cpuPercent, _ := cpu.Percent(0, false)
	memInfo, _ := mem.VirtualMemory()
	
	stats := fmt.Sprintf("CPU: %.2f%% | MEM: %.2f%%", cpuPercent[0], memInfo.UsedPercent)
	t.systemStats.SetText(stats)
}



func (t *TUI) startSelectedProcess() {
    if process := t.getSelectedProcess(); process != nil {
        t.showMessage(fmt.Sprintf("Attempting to start process: %s", process.Name))
        if err := t.manager.StartProcess(process.Name); err != nil {
            t.showError(fmt.Sprintf("Failed to start process %s: %v", process.Name, err))
        } else {
            t.showMessage(fmt.Sprintf("Process %s started successfully", process.Name))
        }
    } else {
        t.showError("No process selected or process not found")
    }
}

func (t *TUI) stopSelectedProcess() {
    if process := t.getSelectedProcess(); process != nil {
        t.showMessage(fmt.Sprintf("Attempting to stop process: %s", process.Name))
        if err := t.manager.StopProcess(process.Name); err != nil {
            t.showError(fmt.Sprintf("Failed to stop process %s: %v", process.Name, err))
        } else {
            t.showMessage(fmt.Sprintf("Process %s stopped successfully", process.Name))
        }
    } else {
        t.showError("No process selected or process not found")
    }
}

func (t *TUI) restartSelectedProcess() {
    if process := t.getSelectedProcess(); process != nil {
        t.showMessage(fmt.Sprintf("Attempting to restart process: %s", process.Name))
        if err := t.manager.RestartProcess(process.Name); err != nil {
            t.showError(fmt.Sprintf("Failed to restart process %s: %v", process.Name, err))
        } else {
            t.showMessage(fmt.Sprintf("Process %s restarted successfully", process.Name))
        }
    } else {
        t.showError("No process selected or process not found")
    }
}

func (t *TUI) getSelectedProcess() *process.Process {
    index := t.processList.GetCurrentItem()
    if index == -1 {
        t.showError("No process selected")
        return nil
    }
    text, _ := t.processList.GetItemText(index)
    
    processName := extractProcessName(text)
    if processName == "" {
        t.showError("Invalid process name format")
        return nil
    }
    
    t.showMessage(fmt.Sprintf("Attempting to get process: '%s'", processName))
    
    process, err := t.manager.GetProcess(processName)
    if err != nil {
        t.showError(fmt.Sprintf("Failed to get process '%s': %v", processName, err))
        return nil
    }
    return process
}

func (t *TUI) refreshProcessList() {
    t.processList.Clear()
    for _, p := range t.manager.GetProcesses() {
        status := ""
        switch p.Status {
        case process.StatusRunning:
            status = "● "
        case process.StatusStopping:
            status = "○ "
        default:
            status = "◍ "
        }
        itemText := fmt.Sprintf("%s%s", status, p.Name)
        t.processList.AddItem(itemText, "", 0, nil)
    }
}

func (t *TUI) onCommandEntered(key tcell.Key) {
	if key != tcell.KeyEnter {
		return
	}
	command := t.commandInput.GetText()
	t.commandInput.SetText("")

	if t.currentProcess == nil {
		t.showError("No process selected")
		return
	}

	switch command {
	case "start":
		t.manager.StartProcess(t.currentProcess.Name)
	case "stop":
		t.manager.StopProcess(t.currentProcess.Name)
	case "restart":
		t.manager.RestartProcess(t.currentProcess.Name)
	default:
		t.showError("Unknown command")
	}
}

func (t *TUI) showLogView() {
	if t.currentProcess == nil {
		t.showError("No process selected")
		return
	}

	logModal := tview.NewModal().
		SetText("Process Logs").
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			t.pages.RemovePage("logs")
		})

	logs, err := t.manager.GetProcessLogs(t.currentProcess.Name, 1000)
	if err != nil {
		t.showError(fmt.Sprintf("Failed to get logs: %v", err))
		return
	}

	logText := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetText(strings.Join(logs, "\n"))

	flex := tview.NewFlex().
		AddItem(logText, 0, 1, true)

	t.pages.AddPage("logs", tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(flex, 0, 8, true).
			AddItem(logModal, 3, 1, false).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false), true, true)
}

func (t *TUI) showConfigEditor() {
	if t.currentProcess == nil {
		t.showError("No process selected")
		return
	}

	form := tview.NewForm()
	form.AddInputField("Command", t.currentProcess.Command, 0, nil, nil)
	form.AddInputField("Args", strings.Join(t.currentProcess.Args, " "), 0, nil, nil)
	form.AddInputField("Work Dir", t.currentProcess.WorkDir, 0, nil, nil)
	form.AddInputField("Max Instances", fmt.Sprintf("%d", t.currentProcess.MaxInstances), 0, nil, nil)
	form.AddCheckbox("Auto Start", t.currentProcess.AutoStart, nil)

	saveFunc := func() {
		command := form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
		args := strings.Fields(form.GetFormItemByLabel("Args").(*tview.InputField).GetText())
		workDir := form.GetFormItemByLabel("Work Dir").(*tview.InputField).GetText()
		maxInstances, _ := strconv.Atoi(form.GetFormItemByLabel("Max Instances").(*tview.InputField).GetText())
		autoStart := form.GetFormItemByLabel("Auto Start").(*tview.Checkbox).IsChecked()

		updatedConfig := &process.Process{
			Name:         t.currentProcess.Name,
			Command:      command,
			Args:         args,
			WorkDir:      workDir,
			MaxInstances: maxInstances,
			AutoStart:    autoStart,
		}

		if err := t.manager.UpdateProcessConfig(t.currentProcess.Name, updatedConfig); err != nil {
			t.showError(fmt.Sprintf("Failed to update config: %v", err))
		} else {
			t.showMessage("Configuration updated successfully")
			t.updateProcessInfo(updatedConfig)
		}
		t.pages.RemovePage("config")
	}

	form.AddButton("Save", saveFunc)
	form.AddButton("Cancel", func() {
		t.pages.RemovePage("config")
	})

	// 创建一个 Flex 布局来包含表单和标题
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// 添加标题
	title := tview.NewTextView().
		SetText("Edit Process Configuration").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorYellow)
	
	flex.AddItem(title, 1, 0, false).
		AddItem(form, 0, 1, true)

	// 将 Flex 布局添加到页面中
	t.pages.AddPage("config", flex, true, true)
}

func (t *TUI) showError(message string) {
	t.statusBar.SetText(fmt.Sprintf("[red]Error: %s[-]", message))
	go func() {
		time.Sleep(5 * time.Second)
		t.app.QueueUpdateDraw(func() {
			t.statusBar.SetText("")
		})
	}()
}

func (t *TUI) showMessage(message string) {
	t.statusBar.SetText(fmt.Sprintf("[green]%s[-]", message))
	go func() {
		time.Sleep(5 * time.Second)
		t.app.QueueUpdateDraw(func() {
			t.statusBar.SetText("")
		})
	}()
}

func (t *TUI) updateErrorCount() {
	totalErrors := 0
	for _, p := range t.manager.GetProcesses() {
		totalErrors += int(p.ErrorCount)
	}
	t.errorCount.SetText(fmt.Sprintf("Errors: %d", totalErrors))
}

func (t *TUI) updateEventCount() {
	// Assume we have a way to count events
	eventCount := 0 // This should be replaced with actual event counting logic
	t.eventCount.SetText(fmt.Sprintf("Events: %d", eventCount))
}

// 在 TUI 中添加一个新方法来实时更新日志视图
func (t *TUI) startLogWatcher(p *process.Process) {
	go func() {
		for {
			time.Sleep(time.Second) // 每秒检查一次
			if p != t.currentProcess {
				return // 如果当前进程已经改变，停止观察
			}
			t.app.QueueUpdateDraw(func() {
				t.updateLogView(p)
			})
		}
	}()
}

func (t *TUI) onProcessSelected(index int, _ string, _ string, _ rune) {
	text, _ := t.processList.GetItemText(index)
	processName := extractProcessName(text)
	if processName == "" {
		t.showError("Invalid process name format")
		return
	}
	
	p, err := t.manager.GetProcess(processName)
	if err != nil {
		t.showError(fmt.Sprintf("Failed to get process '%s': %v", processName, err))
		return
	}
	t.currentProcess = p
	t.updateProcessInfo(p)
	t.updateLogView(p)
	t.startLogWatcher(p) // 开始观察日志更新
}

func (t *TUI) updateLogView(p *process.Process) {
	logs, err := t.manager.GetProcessLogs(p.Name, 100)
	if err != nil {
		t.showError(fmt.Sprintf("Failed to get logs: %v", err))
		return
	}
	t.logView.SetText(strings.Join(logs, "\n"))
	t.logView.ScrollToEnd()
}



func (t *TUI) showHelp() {
    helpText := `
GoTaskMaster Help

Global Shortcuts:
  Ctrl+C: Quit the application
  Ctrl+R: Refresh process list
  Ctrl+L: View process logs
  Ctrl+E: Edit process configuration
  Ctrl+H: Show this help

Process List Navigation:
  ↑/↓: Move selection
  Enter: View process details

Process Control:
  s: Start selected process
  t: Stop selected process
  r: Restart selected process

Configuration:
  e: Edit selected process configuration

Logs:
  l: View logs of selected process

Press any key to close this help.
`
    modal := tview.NewModal().
        SetText(helpText).
        AddButtons([]string{"Close"}).
        SetDoneFunc(func(buttonIndex int, buttonLabel string) {
            t.pages.RemovePage("help")
        })
    
    t.pages.AddPage("help", modal, true, true)
}

func extractProcessName(itemText string) string {
    re := regexp.MustCompile(`[●○◍]\s+(\S+)`)
    matches := re.FindStringSubmatch(itemText)
    if len(matches) < 2 {
        return ""
    }
    return matches[1]
}