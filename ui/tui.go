package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qingbolan/gotaskmaster/process"
)

var (
	titleStyle = lipgloss.NewStyle().
		MarginLeft(2).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(0, 1)

	itemStyle = lipgloss.NewStyle().
		PaddingLeft(4).
		Foreground(lipgloss.Color("#FAFAFA"))

	selectedItemStyle = lipgloss.NewStyle().
		PaddingLeft(2).
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#61AFEF"))

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF5555")).
		Bold(true)

	infoStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#50FA7B"))

	promptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF79C6"))

	highlightStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFB86C")).
		Bold(true)

	docStyle = lipgloss.NewStyle().Margin(1, 2)
)

type item struct {
	title string
	desc  string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type viewState int

const (
	stateList viewState = iota
	stateAdd
	stateEdit
	stateConfirmDelete
	stateViewDetails
)

type model struct {
	list           list.Model
	textInput      textinput.Model
	pm             *process.Manager
	state          viewState
	selectedProc   *process.Process
	quitting       bool
	err            error
}

func initialModel(pm *process.Manager) model {
	ti := textinput.New()
	ti.Placeholder = "Add new process (name,command,args,env,maxInstances)"
	ti.Focus()

	items := []list.Item{}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Process Manager"

	m := model{
		list:      l,
		textInput: ti,
		pm:        pm,
		state:     stateList,
	}

	m.updateItems()

	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.state != stateList {
				m.state = stateList
				m.err = nil
				m.updateItems()
				return m, nil
			}
		case "enter":
			return m.handleEnter()
		case "a":
			if m.state == stateList {
				m.state = stateAdd
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, textinput.Blink
			}
		case "e":
			if m.state == stateList {
				return m.handleEdit()
			}
		case "d":
			if m.state == stateList {
				return m.handleDelete()
			}
		case "v":
			if m.state == stateList {
				return m.handleViewDetails()
			}
		case "s":
			if m.state == stateList {
				return m.handleStartProcess()
			}
		case "p":
			if m.state == stateList {
				return m.handleStopProcess()
			}
		case "r":
			if m.state == stateList {
				return m.handleRestartProcess()
			}
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	if m.state == stateAdd || m.state == stateEdit {
		m.textInput, cmd = m.textInput.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}

	return m, cmd
}

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.state {
	case stateAdd:
		m.addNewProcess(m.textInput.Value())
		m.state = stateList
		m.updateItems()
	case stateEdit:
		m.editProcess(m.textInput.Value())
		m.state = stateList
		m.updateItems()
	case stateConfirmDelete:
		if m.selectedProc != nil {
			if err := m.pm.DeleteProcess(m.selectedProc.Name); err != nil {
				m.err = err
			}
			m.state = stateList
			m.updateItems()
		}
	}
	return m, nil
}

func (m model) handleEdit() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected != nil {
		proc, err := m.pm.GetProcess(selected.(item).title)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.state = stateEdit
		m.selectedProc = proc
		m.textInput.SetValue(fmt.Sprintf("%s,%s,%s,%s,%d",
			proc.Name, proc.Command, strings.Join(proc.Args, " "),
			strings.Join(proc.Env, " "), proc.MaxInstances))
		m.textInput.Focus()
		return m, textinput.Blink
	}
	m.err = fmt.Errorf("no process selected for editing")
	return m, nil
}

func (m model) handleDelete() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected != nil {
		proc, err := m.pm.GetProcess(selected.(item).title)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.state = stateConfirmDelete
		m.selectedProc = proc
		return m, nil
	}
	m.err = fmt.Errorf("no process selected for deletion")
	return m, nil
}

func (m model) handleViewDetails() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected != nil {
		proc, err := m.pm.GetProcess(selected.(item).title)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.state = stateViewDetails
		m.selectedProc = proc
		return m, nil
	}
	m.err = fmt.Errorf("no process selected for viewing details")
	return m, nil
}

func (m model) handleStartProcess() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected != nil {
		if err := m.pm.StartProcess(selected.(item).title); err != nil {
			m.err = err
		}
		m.updateItems()
	} else {
		m.err = fmt.Errorf("no process selected to start")
	}
	return m, nil
}

func (m model) handleStopProcess() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected != nil {
		if err := m.pm.StopProcess(selected.(item).title); err != nil {
			m.err = err
		}
		m.updateItems()
	} else {
		m.err = fmt.Errorf("no process selected to stop")
	}
	return m, nil
}

func (m model) handleRestartProcess() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected != nil {
		if err := m.pm.RestartProcess(selected.(item).title); err != nil {
			m.err = err
		}
		m.updateItems()
	} else {
		m.err = fmt.Errorf("no process selected to restart")
	}
	return m, nil
}

func (m *model) addNewProcess(input string) {
	parts := strings.Split(input, ",")
	if len(parts) < 4 {
		m.err = fmt.Errorf("invalid input: need at least 4 parts")
		return
	}

	name := strings.TrimSpace(parts[0])
	command := strings.TrimSpace(parts[1])
	args := strings.Fields(parts[2])
	env := strings.Fields(parts[3])
	maxInstances := 1
	if len(parts) > 4 {
		fmt.Sscanf(strings.TrimSpace(parts[4]), "%d", &maxInstances)
	}

	p := &process.Process{
		Name:         name,
		Command:      command,
		Args:         args,
		Env:          env,
		MaxInstances: maxInstances,
		State:        process.StateIdle,
	}

	if err := m.pm.AddProcess(p); err != nil {
		m.err = err
	}
}

func (m *model) editProcess(input string) {
	if m.selectedProc == nil {
		m.err = fmt.Errorf("no process selected for editing")
		return
	}

	parts := strings.Split(input, ",")
	if len(parts) < 4 {
		m.err = fmt.Errorf("invalid input: need at least 4 parts")
		return
	}

	name := strings.TrimSpace(parts[0])
	command := strings.TrimSpace(parts[1])
	args := strings.Fields(parts[2])
	env := strings.Fields(parts[3])
	maxInstances := 1
	if len(parts) > 4 {
		fmt.Sscanf(strings.TrimSpace(parts[4]), "%d", &maxInstances)
	}

	newProc := &process.Process{
		Name:         name,
		Command:      command,
		Args:         args,
		Env:          env,
		MaxInstances: maxInstances,
		State:        m.selectedProc.State,
		Instances:    m.selectedProc.Instances,
		ErrorCount:   m.selectedProc.ErrorCount,
		LastError:    m.selectedProc.LastError,
	}

	if err := m.pm.ModifyProcess(m.selectedProc.Name, newProc); err != nil {
		m.err = err
	}
}

func (m *model) updateItems() {
	processes := m.pm.GetProcesses()
	items := make([]list.Item, len(processes))
	for i, p := range processes {
		stateStr := p.State.String()
		stateStyle := infoStyle
		if p.State == process.StateError {
			stateStyle = errorStyle
		} else if p.State == process.StateRunning {
			stateStyle = highlightStyle
		}

		items[i] = item{
			title: p.Name,
			desc: fmt.Sprintf(
				"State: %s, Instances: %d/%d, Errors: %d, Command: %s\nLast Error: %s",
				stateStyle.Render(stateStr), p.Instances, p.MaxInstances, p.ErrorCount, p.Command,
				errorStyle.Render(p.LastError),
			),
		}
	}
	m.list.SetItems(items)
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	switch m.state {
	case stateList:
		return m.listView()
	case stateAdd:
		return m.addView()
	case stateEdit:
		return m.editView()
	case stateConfirmDelete:
		return m.confirmDeleteView()
	case stateViewDetails:
		return m.viewDetailsView()
	}

	return ""
}

func (m model) listView() string {
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\n%s",
		titleStyle.Render("Process Manager"),
		m.list.View(),
		promptStyle.Render("a: add, e: edit, d: delete, v: view details, s: start, p: stop, r: restart, q: quit"),
		errorStyle.Render(fmt.Sprintf("%v", m.err)),
	)
}

func (m model) addView() string {
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		titleStyle.Render("Add New Process"),
		m.textInput.View(),
		promptStyle.Render("(Press Enter to add, Esc to cancel)"),
	)
}

func (m model) editView() string {
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		titleStyle.Render("Edit Process"),
		m.textInput.View(),
		promptStyle.Render("(Press Enter to save, Esc to cancel)"),
	)
}

func (m model) confirmDeleteView() string {
	if m.selectedProc == nil {
		return errorStyle.Render("No process selected for deletion")
	}
	return fmt.Sprintf(
		"%s\n\nAre you sure you want to delete the process '%s'?\n\n%s",
		titleStyle.Render("Confirm Delete"),
		m.selectedProc.Name,
		promptStyle.Render("(Press Enter to confirm, Esc to cancel)"),
	)
}

func (m model) viewDetailsView() string {
	if m.selectedProc == nil {
		return errorStyle.Render("No process selected for viewing details")
	}
	return fmt.Sprintf(
		"%s\n\nName: %s\nCommand: %s\nArgs: %v\nEnv: %v\nState: %s\nInstances: %d/%d\nError Count: %d\nLast Error: %s\n\n%s",
		titleStyle.Render("Process Details"),
		m.selectedProc.Name,
		m.selectedProc.Command,
		m.selectedProc.Args,
		m.selectedProc.Env,
		highlightStyle.Render(m.selectedProc.State.String()),
		m.selectedProc.Instances,
		m.selectedProc.MaxInstances,
		m.selectedProc.ErrorCount,
		errorStyle.Render(m.selectedProc.LastError),
		promptStyle.Render("(Press Esc to return to list)"),
	)
}

func Run(pm *process.Manager) error {
	p := tea.NewProgram(initialModel(pm), tea.WithAltScreen())
	_, err := p.Run()
	return err
}