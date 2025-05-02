// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	headerHeight = 2
)

var BubbleProgram *tea.Program

var (
	titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	stepStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	successStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	cursorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	statusUpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusDownStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusPartialStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	statusLoadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	serverNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Italic(true)
	identifierColor    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

type state int

const (
	stateLoadingProjects state = iota
	stateProjectList
	stateRunningSequence
	stateSequenceError
	stateProjectDetails
	stateSshConfigList
	stateSshConfigRemoveConfirm
	stateSshConfigAddForm
	stateSshConfigImportSelect
	stateSshConfigImportDetails
	stateSshConfigEditForm
)

const (
	authMethodKey = iota + 1
	authMethodAgent
	authMethodPassword
)

type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PgUp     key.Binding
	PgDown   key.Binding
	Home     key.Binding
	End      key.Binding
	Quit     key.Binding
	Enter    key.Binding
	Esc      key.Binding
	Back     key.Binding
	Select   key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Yes      key.Binding
	No       key.Binding

	Config        key.Binding
	UpAction      key.Binding
	DownAction    key.Binding
	RefreshAction key.Binding

	Remove key.Binding
	Add    key.Binding
	Import key.Binding
	Edit   key.Binding

	ToggleDisabled key.Binding
}

var DefaultKeyMap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	PgUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	PgDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home"),
		key.WithHelp("home", "home"),
	),
	End: key.NewBinding(
		key.WithKeys("end"),
		key.WithHelp("end", "end"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select/confirm"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back/cancel"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "b"),
		key.WithHelp("esc/b", "back"),
	),
	Select: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle select"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Yes: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "yes"),
	),
	No: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "no"),
	),

	Config: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "manage ssh config"),
	),
	UpAction: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "up project(s)"),
	),
	DownAction: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "down project(s)"),
	),
	RefreshAction: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh project(s)"),
	),

	Remove: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "remove host"),
	),
	Add: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "add host"),
	),
	Import: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "import from file"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit host"),
	),

	ToggleDisabled: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle disabled"),
	),
}

type model struct {
	keymap              KeyMap
	projects            []discovery.Project
	cursor              int
	selectedProjectIdxs map[int]struct{}
	configCursor        int
	hostToRemove        *config.SSHHost
	hostToEdit          *config.SSHHost
	configuredHosts     []config.SSHHost
	viewport            viewport.Model
	sshConfigViewport   viewport.Model
	detailsViewport     viewport.Model
	formViewport        viewport.Model
	importSelectViewport viewport.Model
	currentState        state
	isDiscovering       bool
	currentSequence     []runner.CommandStep
	currentStepIndex    int
	outputContent       string
	lastError           error
	discoveryErrors     []error
	ready               bool
	width               int
	height              int
	outputChan          <-chan runner.OutputLine
	errorChan           <-chan error
	projectStatuses     map[string]runner.ProjectRuntimeInfo
	loadingStatus       map[string]bool
	detailedProject     *discovery.Project
	sequenceProject     *discovery.Project
	projectsInSequence  []*discovery.Project

	formInputs     []textinput.Model
	formFocusIndex int
	formAuthMethod int
	formDisabled   bool
	formError      error

	importableHosts    []config.PotentialHost
	selectedImportIdxs map[int]struct{}
	importCursor       int
	importError        error
	importInfoMsg      string
	hostsToConfigure   []config.SSHHost
	configuringHostIdx int
}

type projectDiscoveredMsg struct {
	project discovery.Project
}

type discoveryErrorMsg struct {
	err error
}

type discoveryFinishedMsg struct{}

type sshConfigLoadedMsg struct {
	hosts []config.SSHHost
	Err   error
}

type sshHostAddedMsg struct {
	err error
}

type sshHostEditedMsg struct {
	err error
}

type sshConfigParsedMsg struct {
	potentialHosts []config.PotentialHost
	err            error
}

type sshHostsImportedMsg struct {
	importedCount int
	err           error
}

type outputLineMsg struct {
	line runner.OutputLine
}

type stepFinishedMsg struct {
	err error
}

type projectStatusLoadedMsg struct {
	projectIdentifier string
	statusInfo        runner.ProjectRuntimeInfo
}

func findProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		projectChan, errorChan, doneChan := discovery.FindProjects()

		go func() {
			for p := range projectChan {
				if BubbleProgram != nil {
					BubbleProgram.Send(projectDiscoveredMsg{project: p})
				}
			}
		}()

		go func() {
			for e := range errorChan {
				if BubbleProgram != nil {
					BubbleProgram.Send(discoveryErrorMsg{err: e})
				}
			}
		}()

		go func() {
			<-doneChan
			if BubbleProgram != nil {
				BubbleProgram.Send(discoveryFinishedMsg{})
			}
		}()

		return nil
	}
}

func fetchProjectStatusCmd(project discovery.Project) tea.Cmd {
	return func() tea.Msg {
		statusInfo := runner.GetProjectStatus(project)
		return projectStatusLoadedMsg{
			projectIdentifier: project.Identifier(),
			statusInfo:        statusInfo,
		}
	}
}

func loadSshConfigCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		return sshConfigLoadedMsg{hosts: cfg.SSHHosts, Err: err}
	}
}

func saveEditedSshHostCmd(originalName string, editedHost config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostEditedMsg{fmt.Errorf("failed to load config before saving edit: %w", err)}
		}

		found := false
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == originalName {
				if originalName != editedHost.Name {
					for j, otherHost := range cfg.SSHHosts {
						if i != j && otherHost.Name == editedHost.Name {
							return sshHostEditedMsg{fmt.Errorf("host name '%s' already exists", editedHost.Name)}
						}
					}
				}
				cfg.SSHHosts[i] = editedHost
				found = true
				break
			}
		}

		if !found {
			return sshHostEditedMsg{fmt.Errorf("original host '%s' not found during save", originalName)}
		}

		err = config.SaveConfig(cfg)
		if err != nil {
			return sshHostEditedMsg{fmt.Errorf("failed to save config after edit: %w", err)}
		}
		return sshHostEditedMsg{nil}
	}
}

func saveNewSshHostCmd(newHost config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostAddedMsg{fmt.Errorf("failed to load config before saving: %w", err)}
		}

		for _, h := range cfg.SSHHosts {
			if h.Name == newHost.Name {
				return sshHostAddedMsg{fmt.Errorf("host name '%s' already exists", newHost.Name)}
			}
		}

		cfg.SSHHosts = append(cfg.SSHHosts, newHost)
		err = config.SaveConfig(cfg)
		if err != nil {
			return sshHostAddedMsg{fmt.Errorf("failed to save config: %w", err)}
		}
		return sshHostAddedMsg{nil}
	}
}

func removeSshHostCmd(hostToRemove config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return stepFinishedMsg{fmt.Errorf("failed to load config before remove: %w", err)}
		}
		newHosts := []config.SSHHost{}
		found := false
		for _, h := range cfg.SSHHosts {
			if h.Name != hostToRemove.Name {
				newHosts = append(newHosts, h)
			} else {
				found = true
			}
		}
		if !found {
			return stepFinishedMsg{err: fmt.Errorf("host '%s' not found in config during removal", hostToRemove.Name)}
		}
		cfg.SSHHosts = newHosts
		err = config.SaveConfig(cfg)
		if err != nil {
			return stepFinishedMsg{err: fmt.Errorf("failed to save config after remove: %w", err)}
		}
		return stepFinishedMsg{nil}
	}
}

func parseSshConfigCmd() tea.Cmd {
	return func() tea.Msg {
		potentialHosts, err := config.ParseSSHConfig()
		return sshConfigParsedMsg{potentialHosts: potentialHosts, err: err}
	}
}

func saveImportedSshHostsCmd(hostsToSave []config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		if len(hostsToSave) == 0 {
			return sshHostsImportedMsg{importedCount: 0, err: nil}
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostsImportedMsg{err: fmt.Errorf("failed to load config before saving imports: %w", err)}
		}

		currentNames := make(map[string]bool)
		for _, h := range cfg.SSHHosts {
			currentNames[h.Name] = true
		}

		finalHostsToAdd := []config.SSHHost{}
		skippedCount := 0
		for _, newHost := range hostsToSave {
			if _, exists := currentNames[newHost.Name]; exists {
				skippedCount++
				continue
			}
			finalHostsToAdd = append(finalHostsToAdd, newHost)
			currentNames[newHost.Name] = true
		}

		if len(finalHostsToAdd) == 0 && skippedCount > 0 {
			return sshHostsImportedMsg{err: fmt.Errorf("all selected hosts conflicted with existing names")}
		}

		cfg.SSHHosts = slices.Concat(cfg.SSHHosts, finalHostsToAdd)
		err = config.SaveConfig(cfg)
		if err != nil {
			return sshHostsImportedMsg{err: fmt.Errorf("failed to save config after import: %w", err)}
		}

		errMsg := ""
		if skippedCount > 0 {
			errMsg = fmt.Sprintf(" (skipped %d due to conflicts)", skippedCount)
		}

		return sshHostsImportedMsg{
			importedCount: len(finalHostsToAdd),
			err:           fmt.Errorf("import failed: %s", errMsg),
		}
	}
}

type channelsAvailableMsg struct {
	outChan <-chan runner.OutputLine
	errChan <-chan error
}

func runStepCmd(step runner.CommandStep) tea.Cmd {
	return func() tea.Msg {
		outChan, errChan := runner.StreamCommand(step)
		return channelsAvailableMsg{outChan: outChan, errChan: errChan}
	}
}

func waitForOutputCmd(outChan <-chan runner.OutputLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-outChan
		if !ok {
			return nil
		}
		return outputLineMsg{line}
	}
}

func waitForErrorCmd(errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		err := <-errChan
		return stepFinishedMsg{err}
	}
}

func InitialModel() model {
	vp := viewport.New(0, 0)
	m := model{
		keymap:               DefaultKeyMap,
		currentState:         stateLoadingProjects,
		isDiscovering:        true,
		cursor:               0,
		selectedProjectIdxs:  make(map[int]struct{}),
		configCursor:         0,
		projectStatuses:      make(map[string]runner.ProjectRuntimeInfo),
		loadingStatus:        make(map[string]bool),
		configuredHosts:      []config.SSHHost{},
		discoveryErrors:      []error{},
		detailedProject:      nil,
		sequenceProject:      nil,
		projectsInSequence:   nil,
		viewport:             vp,
		sshConfigViewport:    vp,
		detailsViewport:      vp,
		formViewport:         vp,
		importSelectViewport: vp,
	}
	return m
}

func createAddForm() []textinput.Model {
	inputs := make([]textinput.Model, 7)
	var t textinput.Model

	t = textinput.New()
	t.Placeholder = "Unique Name (e.g., server1)"
	t.Focus()
	t.CharLimit = 50
	t.Width = 40
	inputs[0] = t

	t = textinput.New()
	t.Placeholder = "Hostname or IP Address"
	t.CharLimit = 100
	t.Width = 40
	inputs[1] = t

	t = textinput.New()
	t.Placeholder = "SSH Username"
	t.CharLimit = 50
	t.Width = 40
	inputs[2] = t

	t = textinput.New()
	t.Placeholder = "Port (default 22)"
	t.CharLimit = 5
	t.Width = 20
	t.Validate = func(s string) error {
		if s == "" {
			return nil
		}
		_, err := strconv.Atoi(s)
		return err
	}
	inputs[3] = t

	t = textinput.New()
	t.Placeholder = "Remote Root Path (optional, defaults: ~/bucket or ~/compose-bucket)"
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	t = textinput.New()
	t.Placeholder = "Path to Private Key (e.g., ~/.ssh/id_rsa)"
	t.CharLimit = 200
	t.Width = 60
	inputs[5] = t

	t = textinput.New()
	t.Placeholder = "Password (stored insecurely!)"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	return inputs
}

func createEditForm(host config.SSHHost) ([]textinput.Model, int, bool) {
	inputs := make([]textinput.Model, 7)
	var t textinput.Model
	initialAuthMethod := authMethodAgent
	if host.KeyPath != "" {
		initialAuthMethod = authMethodKey
	} else if host.Password != "" {
		initialAuthMethod = authMethodPassword
	}

	t = textinput.New()
	t.Placeholder = "Unique Name"
	t.SetValue(host.Name)
	t.Focus()
	t.CharLimit = 50
	t.Width = 40
	inputs[0] = t

	t = textinput.New()
	t.Placeholder = "Hostname or IP Address"
	t.SetValue(host.Hostname)
	t.CharLimit = 100
	t.Width = 40
	inputs[1] = t

	t = textinput.New()
	t.Placeholder = "SSH Username"
	t.SetValue(host.User)
	t.CharLimit = 50
	t.Width = 40
	inputs[2] = t

	t = textinput.New()
	t.Placeholder = "Port (default 22)"
	portStr := ""
	if host.Port != 0 {
		portStr = strconv.Itoa(host.Port)
	}
	t.SetValue(portStr)
	t.CharLimit = 5
	t.Width = 20
	t.Validate = func(s string) error {
		if s == "" {
			return nil
		}
		_, err := strconv.Atoi(s)
		return err
	}
	inputs[3] = t

	t = textinput.New()
	t.Placeholder = "Remote Root Path (leave blank for default)"
	t.SetValue(host.RemoteRoot)
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	t = textinput.New()
	t.Placeholder = "Path to Private Key"
	t.SetValue(host.KeyPath)
	t.CharLimit = 200
	t.Width = 60
	inputs[5] = t

	t = textinput.New()
	t.Placeholder = "Password (leave blank to keep current)"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	return inputs, initialAuthMethod, host.Disabled
}

func createImportDetailsForm(pHost config.PotentialHost) ([]textinput.Model, int) {
	inputs := make([]textinput.Model, 7)
	var t textinput.Model
	initialAuthMethod := authMethodAgent

	t = textinput.New()
	t.Placeholder = "Remote Root Path (optional, defaults: ~/bucket or ~/compose-bucket)"
	t.Focus()
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	t = textinput.New()
	t.Placeholder = "Path to Private Key"
	t.CharLimit = 200
	t.Width = 60
	if pHost.KeyPath != "" {
		t.SetValue(pHost.KeyPath)
		initialAuthMethod = authMethodKey
	}
	inputs[5] = t

	t = textinput.New()
	t.Placeholder = "Password (stored insecurely!)"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	for i := 0; i < 4; i++ {
		inputs[i] = textinput.New()
	}

	return inputs, initialAuthMethod
}

func (m *model) Init() tea.Cmd {
	return findProjectsCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	viewportActive := m.currentState == stateRunningSequence || m.currentState == stateSequenceError

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			m.viewport = viewport.New(m.width, 1)
			m.sshConfigViewport = viewport.New(m.width, 1)
			m.detailsViewport = viewport.New(m.width, 1)
			m.formViewport = viewport.New(m.width, 1)
			m.importSelectViewport = viewport.New(m.width, 1)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.sshConfigViewport.Width = m.width
			m.detailsViewport.Width = m.width
			m.formViewport.Width = m.width
			m.importSelectViewport.Width = m.width
		}

	case tea.KeyMsg:
		if viewportActive {
			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back), key.Matches(msg, m.keymap.Enter):
				for _, proj := range m.projectsInSequence {
					if proj != nil {
						projID := proj.Identifier()
						if !m.loadingStatus[projID] {
							m.loadingStatus[projID] = true
							cmds = append(cmds, fetchProjectStatusCmd(*proj))
						}
					}
				}
				m.currentState = stateProjectList
				m.outputContent = ""
				m.lastError = nil
				m.currentSequence = nil
				m.currentStepIndex = 0
				m.sequenceProject = nil
				m.projectsInSequence = nil
				m.viewport.GotoTop()
				return m, tea.Batch(cmds...)
			}

			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
			// Removed early return, batching happens at the end.
		}

		switch m.currentState {
		case stateProjectList:
			switch {
			case key.Matches(msg, m.keymap.Config):
				m.currentState = stateSshConfigList
				m.configCursor = 0
				cmds = append(cmds, loadSshConfigCmd())
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			default:
				cmds = append(cmds, m.handleProjectListKeys(msg)...)
			}

		case stateProjectDetails:
			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back):
				m.currentState = stateProjectList
				m.detailedProject = nil
			}

		case stateSshConfigList:
			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back):
				m.currentState = stateProjectList
				m.lastError = nil
				m.importError = nil
				m.importInfoMsg = ""
			case key.Matches(msg, m.keymap.Up):
				if m.configCursor > 0 {
					m.configCursor--
				}
				m.sshConfigViewport.LineUp(1)
			case key.Matches(msg, m.keymap.Down):
				if m.configCursor < len(m.configuredHosts)-1 {
					m.configCursor++
				}
				m.sshConfigViewport.LineDown(1)
			case key.Matches(msg, m.keymap.PgUp), key.Matches(msg, m.keymap.Home):
				m.sshConfigViewport, vpCmd = m.sshConfigViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			case key.Matches(msg, m.keymap.PgDown), key.Matches(msg, m.keymap.End):
				m.sshConfigViewport, vpCmd = m.sshConfigViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			case key.Matches(msg, m.keymap.Remove):
				if len(m.configuredHosts) > 0 && m.configCursor < len(m.configuredHosts) {
					m.hostToRemove = &m.configuredHosts[m.configCursor]
					m.currentState = stateSshConfigRemoveConfirm
					m.lastError = nil
				}
			case key.Matches(msg, m.keymap.Add):
				m.formInputs = createAddForm()
				m.formFocusIndex = 0
				m.formAuthMethod = authMethodAgent
				m.formError = nil
				m.currentState = stateSshConfigAddForm
				m.formViewport.GotoTop()
				// Apply initial focus style
				if len(m.formInputs) > 0 {
					m.formInputs[0].Prompt = cursorStyle.Render("> ")
					m.formInputs[0].TextStyle = cursorStyle
				}
			case key.Matches(msg, m.keymap.Import):
				m.currentState = stateLoadingProjects
				m.importError = nil
				m.lastError = nil
				cmds = append(cmds, parseSshConfigCmd())
			case key.Matches(msg, m.keymap.Edit):
				if len(m.configuredHosts) > 0 && m.configCursor < len(m.configuredHosts) {
					m.hostToEdit = &m.configuredHosts[m.configCursor]
					m.formInputs, m.formAuthMethod, m.formDisabled = createEditForm(*m.hostToEdit)
					m.formFocusIndex = 0
					m.formError = nil
					m.currentState = stateSshConfigEditForm
					m.formViewport.GotoTop()
					// Apply initial focus style
					if len(m.formInputs) > 0 {
						m.formInputs[0].Prompt = cursorStyle.Render("> ")
						m.formInputs[0].TextStyle = cursorStyle
					}
				}
			}
			if vpCmd == nil {
				m.sshConfigViewport, vpCmd = m.sshConfigViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			}

		case stateSshConfigRemoveConfirm:
			switch {
			case key.Matches(msg, m.keymap.Yes):
				if m.hostToRemove != nil {
					cmds = append(cmds, removeSshHostCmd(*m.hostToRemove))
					m.hostToRemove = nil
					m.lastError = nil
				} else {
					m.currentState = stateSshConfigList
				}
			case key.Matches(msg, m.keymap.No), key.Matches(msg, m.keymap.Back):
				m.currentState = stateSshConfigList
				m.hostToRemove = nil
				m.lastError = nil
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			}

		case stateSshConfigAddForm:
			switch {
			case key.Matches(msg, m.keymap.Esc):
				m.currentState = stateSshConfigList
				m.formError = nil
				m.formInputs = nil
				m.importError = nil
				m.importInfoMsg = ""
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			default:
				cmds = append(cmds, m.handleSshAddFormKeys(msg)...)
			}

		case stateSshConfigEditForm:
			switch {
			case key.Matches(msg, m.keymap.Esc):
				m.currentState = stateSshConfigList
				m.formError = nil
				m.formInputs = nil
				m.hostToEdit = nil
				m.importError = nil
				m.importInfoMsg = ""
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			default:
				cmds = append(cmds, m.handleSshEditFormKeys(msg)...)
			}

		case stateSshConfigImportSelect:
			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back):
				m.currentState = stateSshConfigList
				m.importableHosts = nil
				m.selectedImportIdxs = nil
				m.importError = nil
				m.importInfoMsg = ""
				m.lastError = nil
			case key.Matches(msg, m.keymap.Up):
				if m.importCursor > 0 {
					m.importCursor--
				}
				m.importSelectViewport.LineUp(1)
			case key.Matches(msg, m.keymap.Down):
				if m.importCursor < len(m.importableHosts)-1 {
					m.importCursor++
				}
				m.importSelectViewport.LineDown(1)
			case key.Matches(msg, m.keymap.PgUp), key.Matches(msg, m.keymap.Home):
				m.importSelectViewport, vpCmd = m.importSelectViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			case key.Matches(msg, m.keymap.PgDown), key.Matches(msg, m.keymap.End):
				m.importSelectViewport, vpCmd = m.importSelectViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			case key.Matches(msg, m.keymap.Select):
				if len(m.importableHosts) > 0 && m.importCursor >= 0 && m.importCursor < len(m.importableHosts) {
					if _, ok := m.selectedImportIdxs[m.importCursor]; ok {
						delete(m.selectedImportIdxs, m.importCursor)
					} else {
						m.selectedImportIdxs[m.importCursor] = struct{}{}
					}
				}
			case key.Matches(msg, m.keymap.Enter):
				if len(m.selectedImportIdxs) > 0 {
					m.currentState = stateSshConfigImportDetails
					m.hostsToConfigure = []config.SSHHost{}
					m.configuringHostIdx = 0
					m.formError = nil
					m.formViewport.GotoTop()

					firstSelectedIdx := -1
					for i := 0; i < len(m.importableHosts); i++ {
						if _, ok := m.selectedImportIdxs[i]; ok {
							firstSelectedIdx = i
							break
						}
					}
					if firstSelectedIdx != -1 {
						m.configuringHostIdx = firstSelectedIdx
						pHostToConfigure := m.importableHosts[m.configuringHostIdx]
						m.formInputs, m.formAuthMethod = createImportDetailsForm(pHostToConfigure)
						m.formFocusIndex = 0
						m.formError = nil
						// Apply initial focus style for import details (index 4 is Remote Root)
						if len(m.formInputs) > 4 {
							m.formInputs[4].Prompt = cursorStyle.Render("> ")
							m.formInputs[4].TextStyle = cursorStyle
						}
					} else {
						m.importError = fmt.Errorf("internal error: no selected host index found")
						m.currentState = stateSshConfigList
						m.importableHosts = nil
						m.selectedImportIdxs = nil
					}
				} else {
					m.importError = fmt.Errorf("no hosts selected for import")
					m.currentState = stateSshConfigList
					m.importableHosts = nil
					m.selectedImportIdxs = nil
				}
			}

		case stateSshConfigImportDetails:
			isScrollKey := key.Matches(msg, m.keymap.PgUp) || key.Matches(msg, m.keymap.PgDown) || key.Matches(msg, m.keymap.Home) || key.Matches(msg, m.keymap.End)
			if isScrollKey {
				m.formViewport, vpCmd = m.formViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			}

			switch {
			case key.Matches(msg, m.keymap.Esc):
				m.currentState = stateSshConfigList
				m.importError = fmt.Errorf("import cancelled")
				m.importInfoMsg = ""
				m.importableHosts = nil
				m.selectedImportIdxs = nil
				m.hostsToConfigure = nil
				m.formInputs = nil
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			default:
				if !isScrollKey {
					cmds = append(cmds, m.handleSshImportDetailsFormKeys(msg)...)
				}
			}

		default:
			if key.Matches(msg, m.keymap.Quit) {
				return m, tea.Quit
			}
		}

	case sshConfigParsedMsg:
		if msg.err != nil {
			m.lastError = fmt.Errorf("failed to parse ssh config: %w", msg.err)
			m.currentState = stateSshConfigList
		} else {
			cfg, loadErr := config.LoadConfig()
			if loadErr != nil {
				m.lastError = fmt.Errorf("failed to load current config for import filtering: %w", loadErr)
				m.currentState = stateSshConfigList
			} else {
				currentConfigNames := make(map[string]bool)
				for _, h := range cfg.SSHHosts {
					currentConfigNames[h.Name] = true
				}

				m.importableHosts = []config.PotentialHost{}
				for _, pHost := range msg.potentialHosts {
					if _, exists := currentConfigNames[pHost.Alias]; !exists {
						m.importableHosts = append(m.importableHosts, pHost)
					}
				}

				if len(m.importableHosts) == 0 {
					m.importError = fmt.Errorf("no new importable hosts found in ssh config")
					m.currentState = stateSshConfigList
				} else {
					m.currentState = stateSshConfigImportSelect
					m.importCursor = 0
					m.selectedImportIdxs = make(map[int]struct{})
					m.importError = nil
					m.lastError = nil
				}
			}
		}
	case sshHostsImportedMsg:
		m.currentState = stateSshConfigList
		m.importError = nil
		m.importInfoMsg = ""

		isInfoOnly := false
		if msg.err != nil {
			errMsgStr := msg.err.Error()
			isInfoOnly = strings.HasPrefix(strings.TrimSpace(errMsgStr), "(") && strings.HasSuffix(strings.TrimSpace(errMsgStr), ")") || strings.TrimSpace(errMsgStr) == ""
		}

		if msg.err != nil && !isInfoOnly {
			m.importError = fmt.Errorf("import failed: %w", msg.err)
		} else {
			info := fmt.Sprintf("Import finished: %d hosts added.", msg.importedCount)
			if msg.err != nil && isInfoOnly {
				info += fmt.Sprintf(" %s", msg.err.Error())
			}
			m.importInfoMsg = info
		}

		m.importableHosts = nil
		m.selectedImportIdxs = nil
		m.hostsToConfigure = nil
		m.formInputs = nil
		cmds = append(cmds, loadSshConfigCmd())

	case projectDiscoveredMsg:
		if m.currentState == stateLoadingProjects {
			m.currentState = stateProjectList
		}
		m.projects = append(m.projects, msg.project)
	projID := msg.project.Identifier()
	if !m.loadingStatus[projID] {
			m.loadingStatus[projID] = true
			cmds = append(cmds, fetchProjectStatusCmd(msg.project))
		}

	case discoveryErrorMsg:
		m.discoveryErrors = append(m.discoveryErrors, msg.err)
		m.lastError = msg.err

	case discoveryFinishedMsg:
		stateChanged := false
		if m.currentState == stateLoadingProjects {
			m.currentState = stateProjectList
			stateChanged = true
			if len(m.projects) == 0 {
				if len(m.discoveryErrors) == 0 {
					m.lastError = fmt.Errorf("no projects found")
				} else {
					m.lastError = fmt.Errorf("discovery finished with errors, no projects found")
				}
			} else {
				m.lastError = nil
				if len(m.discoveryErrors) > 0 {
					m.lastError = fmt.Errorf("discovery finished with errors")
				}
			}
		}
		m.isDiscovering = false

		if stateChanged && m.currentState == stateProjectList {
			listContent := strings.Builder{}
			listContent.WriteString("Select a project:\n")
			for i, project := range m.projects {
				cursor := "  "
				if m.cursor == i {
					cursor = cursorStyle.Render("> ")
				}
				projID := project.Identifier()
				statusStr := ""
				if m.loadingStatus[projID] {
					statusStr = statusLoadingStyle.Render(" [loading...]")
				} else if _, ok := m.projectStatuses[projID]; ok {
					statusStr = statusUpStyle.Render(" [loaded]")
				} else {
					statusStr = statusLoadingStyle.Render(" [?]")
				}
				listContent.WriteString(fmt.Sprintf("%s%s (%s)%s\n", cursor, project.Name, serverNameStyle.Render(project.ServerName), statusStr))
			}
			m.viewport.SetContent(listContent.String())
			m.viewport.GotoTop()
		}

	case sshConfigLoadedMsg:
		if msg.Err != nil {
			m.lastError = fmt.Errorf("failed to load ssh config: %w", msg.Err)
		} else {
			m.configuredHosts = msg.hosts
			m.lastError = nil
		}
		if m.configCursor >= len(m.configuredHosts) {
			m.configCursor = max(0, len(m.configuredHosts)-1)
		}

	case projectStatusLoadedMsg:
		m.loadingStatus[msg.projectIdentifier] = false
		m.projectStatuses[msg.projectIdentifier] = msg.statusInfo

	case stepFinishedMsg:
		if m.currentState == stateSshConfigRemoveConfirm {
			if msg.err != nil {
				m.lastError = fmt.Errorf("failed to remove host: %w", msg.err)
				m.currentState = stateSshConfigList
				cmds = append(cmds, loadSshConfigCmd())
			} else {
				m.currentState = stateSshConfigList
				cmds = append(cmds, loadSshConfigCmd())
			}
		} else if m.currentState == stateRunningSequence {
			m.outputChan = nil
			m.errorChan = nil
			if msg.err != nil {
				m.lastError = msg.err
				m.currentState = stateSequenceError
				m.outputContent += errorStyle.Render(fmt.Sprintf("\n--- STEP FAILED: %v ---", msg.err)) + "\n"
				m.viewport.SetContent(m.outputContent)
				m.viewport.GotoBottom()
			} else {
				m.outputContent += successStyle.Render(fmt.Sprintf("\n--- Step '%s' Succeeded ---", m.currentSequence[m.currentStepIndex].Name)) + "\n"
				m.currentStepIndex++
				if m.currentStepIndex >= len(m.currentSequence) {
					m.outputContent += successStyle.Render("\n--- Action Sequence Completed Successfully ---") + "\n"
					m.viewport.SetContent(m.outputContent)
					m.viewport.GotoBottom()
					for _, proj := range m.projectsInSequence {
						if proj != nil {
							projID := proj.Identifier()
							if !m.loadingStatus[projID] {
								m.loadingStatus[projID] = true
								cmds = append(cmds, fetchProjectStatusCmd(*proj))
							}
						}
					}
				} else {
					cmds = append(cmds, m.startNextStepCmd())
				}
			}
		}

	case channelsAvailableMsg:
		if m.currentState == stateRunningSequence {
			m.outputChan = msg.outChan
			m.errorChan = msg.errChan
			cmds = append(cmds, waitForOutputCmd(m.outputChan), waitForErrorCmd(m.errorChan))
		}

	case outputLineMsg:
		if m.currentState == stateRunningSequence && m.outputChan != nil {
			if msg.line.IsError {
				m.outputContent += errorStyle.Render(msg.line.Line) + "\n"
			} else {
				m.outputContent += msg.line.Line + "\n"
			}
			m.viewport.SetContent(m.outputContent)
			m.viewport.GotoBottom()
			cmds = append(cmds, waitForOutputCmd(m.outputChan))
		}

	case sshHostAddedMsg:
		if m.currentState == stateSshConfigAddForm {
			if msg.err != nil {
				m.formError = msg.err
			} else {
				m.currentState = stateSshConfigList
				m.formError = nil
				m.formInputs = nil
				m.configCursor = 0
				cmds = append(cmds, loadSshConfigCmd())
			}
		}

	case sshHostEditedMsg:
		if m.currentState == stateSshConfigEditForm {
			if msg.err != nil {
				m.formError = msg.err
			} else {
				m.currentState = stateSshConfigList
				m.formError = nil
				m.formInputs = nil
				m.hostToEdit = nil
				m.configCursor = 0
				cmds = append(cmds, loadSshConfigCmd())
			}
		}

	}

	isFormState := m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm || m.currentState == stateSshConfigImportDetails
	// Removed call to updateInputs, logic should be handled within specific key handlers.

	if isFormState && vpCmd == nil {
		shouldUpdateViewport := true
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			k := keyMsg.String()
			if len(k) == 1 || k == "enter" || k == "esc" || k == "tab" || k == "shift+tab" || k == " " {
				shouldUpdateViewport = false
			}
		}
		if shouldUpdateViewport {
			m.formViewport, vpCmd = m.formViewport.Update(msg)
			cmds = append(cmds, vpCmd)
		}
	}

	if m.currentState == stateProjectDetails && vpCmd == nil {
		m.detailsViewport, vpCmd = m.detailsViewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) handleSshAddFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd
	// Inlined getAddFormFocusMap logic
	focusMap := []int{0, 1, 2, 3, 4, 5}
	if m.formAuthMethod == authMethodKey {
		focusMap = append(focusMap, 6)
	} else if m.formAuthMethod == authMethodPassword {
		focusMap = append(focusMap, 7)
	}
	currentIndexInMap := -1
	for i, logicalIndex := range focusMap {
		if logicalIndex == m.formFocusIndex {
			currentIndexInMap = i
			break
		}
	}

	switch {
	case key.Matches(msg, m.keymap.Tab), key.Matches(msg, m.keymap.Down):
		if currentIndexInMap != -1 {
			nextIndexInMap := (currentIndexInMap + 1) % len(focusMap)
			m.formFocusIndex = focusMap[nextIndexInMap]
		}
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		if currentIndexInMap != -1 {
			nextIndexInMap := (currentIndexInMap - 1 + len(focusMap)) % len(focusMap)
			m.formFocusIndex = focusMap[nextIndexInMap]
	}
case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
	if m.formFocusIndex == 5 {
		if key.Matches(msg, m.keymap.Left) {
			m.formAuthMethod--
			if m.formAuthMethod < authMethodKey {
				m.formAuthMethod = authMethodPassword
			}
		} else {
			m.formAuthMethod++
			if m.formAuthMethod > authMethodPassword {
				m.formAuthMethod = authMethodKey
			}
		}
		// Redundant focus map check removed as focus index is already 5 here.
		m.formError = nil
	}

	case key.Matches(msg, m.keymap.Enter):
		if m.formFocusIndex == 5 {
			return cmds
		}
		m.formError = nil
		newHost, validationErr := m.buildHostFromForm()
		if validationErr != nil {
			m.formError = validationErr
		} else {
			cmds = append(cmds, saveNewSshHostCmd(newHost))
		}
	}

	for i := range m.formInputs {
		m.formInputs[i].Blur()
		m.formInputs[i].Prompt = "  "
		m.formInputs[i].TextStyle = lipgloss.NewStyle()
	}

	var focusedInputIndex int = -1
	switch m.formFocusIndex {
	case 0, 1, 2, 3, 4:
		focusedInputIndex = m.formFocusIndex
	case 6:
		if m.formAuthMethod == authMethodKey {
			focusedInputIndex = 5
		}
	case 7:
		if m.formAuthMethod == authMethodPassword {
			focusedInputIndex = 6
		}
	}

	if focusedInputIndex != -1 {
		cmds = append(cmds, m.formInputs[focusedInputIndex].Focus())
		m.formInputs[focusedInputIndex].Prompt = cursorStyle.Render("> ")
		m.formInputs[focusedInputIndex].TextStyle = cursorStyle
	}

	return cmds
}

func (m *model) handleSshEditFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd
	// Inlined getEditFormFocusMap logic
	focusMap := []int{0, 1, 2, 3, 4, 5}
	if m.formAuthMethod == authMethodKey {
		focusMap = append(focusMap, 6)
	} else if m.formAuthMethod == authMethodPassword {
		focusMap = append(focusMap, 7)
	}
	focusMap = append(focusMap, 8)
	// Inlined getAuthMethodFocusIndex and getDisabledToggleFocusIndex
	authMethodLogicalIndex := 5
	disabledToggleLogicalIndex := 8

	currentIndexInMap := -1
	for i, logicalIndex := range focusMap {
		if logicalIndex == m.formFocusIndex {
			currentIndexInMap = i
			break
		}
	}

	switch {
	case key.Matches(msg, m.keymap.Tab), key.Matches(msg, m.keymap.Down):
		if currentIndexInMap != -1 {
			nextIndexInMap := (currentIndexInMap + 1) % len(focusMap)
			m.formFocusIndex = focusMap[nextIndexInMap]
		}
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		if currentIndexInMap != -1 {
			nextIndexInMap := (currentIndexInMap - 1 + len(focusMap)) % len(focusMap)
			m.formFocusIndex = focusMap[nextIndexInMap]
		}
	case key.Matches(msg, m.keymap.ToggleDisabled):
		if m.formFocusIndex == disabledToggleLogicalIndex {
			m.formDisabled = !m.formDisabled
		}

	case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
		if m.formFocusIndex == authMethodLogicalIndex {
			if key.Matches(msg, m.keymap.Left) {
				m.formAuthMethod--
				if m.formAuthMethod < authMethodKey {
					m.formAuthMethod = authMethodPassword
				}
			} else {
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			// Redundant focus map check removed as focus index is already authMethodLogicalIndex here.
			m.formError = nil
		}

	case key.Matches(msg, m.keymap.Enter):
		if m.formFocusIndex == authMethodLogicalIndex || m.formFocusIndex == disabledToggleLogicalIndex {
			return cmds
		}
		m.formError = nil
		if m.hostToEdit == nil {
			m.formError = fmt.Errorf("internal error: no host selected for editing")
			return cmds
		}
		editedHost, validationErr := m.buildHostFromEditForm()
		if validationErr != nil {
			m.formError = validationErr
		} else {
			cmds = append(cmds, saveEditedSshHostCmd(m.hostToEdit.Name, editedHost))
		}
	}

	for i := range m.formInputs {
		m.formInputs[i].Blur()
		m.formInputs[i].Prompt = "  "
		m.formInputs[i].TextStyle = lipgloss.NewStyle()
	}

	var focusedInputIndex int = -1
	switch m.formFocusIndex {
	case 0, 1, 2, 3, 4:
		focusedInputIndex = m.formFocusIndex
	case 6:
		if m.formAuthMethod == authMethodKey {
			focusedInputIndex = 5
		}
	case 7:
		if m.formAuthMethod == authMethodPassword {
			focusedInputIndex = 6
		}
	}

	if focusedInputIndex != -1 {
		cmds = append(cmds, m.formInputs[focusedInputIndex].Focus())
		m.formInputs[focusedInputIndex].Prompt = cursorStyle.Render("> ")
		m.formInputs[focusedInputIndex].TextStyle = cursorStyle
	}

	return cmds
}

func (m *model) handleSshImportDetailsFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Define logical focus indices based on visual order
	const (
		remoteRootFocusIndex    = 0
		authMethodFocusIndex    = 1
		keyOrPasswordFocusIndex = 2
	)

	authNeeded := false
	if m.configuringHostIdx >= 0 && m.configuringHostIdx < len(m.importableHosts) {
		authNeeded = m.importableHosts[m.configuringHostIdx].KeyPath == ""
	}

	numFocusable := 1 // Fixed Field: Remote Root
	if authNeeded {
		numFocusable++
		if m.formAuthMethod == authMethodKey || m.formAuthMethod == authMethodPassword {
			numFocusable++
		}
	}

	switch {
	case key.Matches(msg, m.keymap.Tab), key.Matches(msg, m.keymap.Down):
		m.formFocusIndex = (m.formFocusIndex + 1) % numFocusable
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		m.formFocusIndex--
		if m.formFocusIndex < 0 {
			m.formFocusIndex = numFocusable - 1
		}
	case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
		if authNeeded && m.formFocusIndex == authMethodFocusIndex { // Check if focus is on Auth Method
			if key.Matches(msg, m.keymap.Left) {
				m.formAuthMethod--
				if m.formAuthMethod < authMethodKey {
					m.formAuthMethod = authMethodPassword
				}
			} else {
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			m.formError = nil
		}

	case key.Matches(msg, m.keymap.Enter):
		// Prevent submitting when focus is on the non-input Auth Method selector
		if authNeeded && m.formFocusIndex == authMethodFocusIndex {
			return cmds
		}

		m.formError = nil

		if m.configuringHostIdx < 0 || m.configuringHostIdx >= len(m.importableHosts) {
			m.formError = fmt.Errorf("internal error: invalid host index for import details")
			return cmds
		}
		currentPotentialHost := m.importableHosts[m.configuringHostIdx]

		remoteRoot := strings.TrimSpace(m.formInputs[4].Value())
		keyPath := strings.TrimSpace(m.formInputs[5].Value())
		password := m.formInputs[6].Value()

		hostToSave, convertErr := config.ConvertToBucketManagerHost(currentPotentialHost, currentPotentialHost.Alias, remoteRoot)
		if convertErr != nil {
			m.formError = fmt.Errorf("internal conversion error: %w", convertErr)
			return cmds
		}

		if currentPotentialHost.KeyPath == "" { // Only override auth if it wasn't set in ssh_config
			switch m.formAuthMethod {
			case authMethodKey:
				if keyPath == "" {
					m.formError = fmt.Errorf("key path is required for Key File authentication")
					return cmds
				}
				hostToSave.KeyPath = keyPath
				hostToSave.Password = ""
			case authMethodPassword:
				if password == "" {
					m.formError = fmt.Errorf("password is required for Password authentication")
					return cmds
				}
				hostToSave.Password = password
				hostToSave.KeyPath = ""
			case authMethodAgent:
				hostToSave.KeyPath = ""
				hostToSave.Password = ""
			}
		} // else: keep the KeyPath from ssh_config

		m.hostsToConfigure = append(m.hostsToConfigure, hostToSave)

		// Find the next selected host to configure
		nextSelectedIdx := -1
		for i := m.configuringHostIdx + 1; i < len(m.importableHosts); i++ {
			if _, ok := m.selectedImportIdxs[i]; ok {
				nextSelectedIdx = i
				break
			}
		}

		if nextSelectedIdx != -1 {
			// Move to the next host
			m.configuringHostIdx = nextSelectedIdx
			pHostToConfigure := m.importableHosts[m.configuringHostIdx]
			m.formInputs, m.formAuthMethod = createImportDetailsForm(pHostToConfigure)
			m.formFocusIndex = 0 // Reset focus to the first field
			m.formError = nil
		} else {
			// No more hosts selected, save and go back
			cmds = append(cmds, saveImportedSshHostsCmd(m.hostsToConfigure))
		}
	}

	// --- Update Input Focus ---
	remoteRootInputIdx := 4
	keyPathInputIdx := 5
	passwordInputIdx := 6

	// Blur all potentially focusable inputs first
	m.formInputs[remoteRootInputIdx].Blur()
	m.formInputs[keyPathInputIdx].Blur()
	m.formInputs[passwordInputIdx].Blur()
	m.formInputs[remoteRootInputIdx].Prompt = "  "
	m.formInputs[keyPathInputIdx].Prompt = "  "
	m.formInputs[passwordInputIdx].Prompt = "  "
	m.formInputs[remoteRootInputIdx].TextStyle = lipgloss.NewStyle()
	m.formInputs[keyPathInputIdx].TextStyle = lipgloss.NewStyle()
	m.formInputs[passwordInputIdx].TextStyle = lipgloss.NewStyle()

	// Apply focus based on the logical formFocusIndex
	switch m.formFocusIndex {
	case remoteRootFocusIndex:
		cmds = append(cmds, m.formInputs[remoteRootInputIdx].Focus())
		m.formInputs[remoteRootInputIdx].Prompt = cursorStyle.Render("> ")
		m.formInputs[remoteRootInputIdx].TextStyle = cursorStyle
	case keyOrPasswordFocusIndex:
		if authNeeded {
			if m.formAuthMethod == authMethodKey {
				cmds = append(cmds, m.formInputs[keyPathInputIdx].Focus())
				m.formInputs[keyPathInputIdx].Prompt = cursorStyle.Render("> ")
				m.formInputs[keyPathInputIdx].TextStyle = cursorStyle
			} else if m.formAuthMethod == authMethodPassword {
				cmds = append(cmds, m.formInputs[passwordInputIdx].Focus())
				m.formInputs[passwordInputIdx].Prompt = cursorStyle.Render("> ")
				m.formInputs[passwordInputIdx].TextStyle = cursorStyle
			}
		}
	case authMethodFocusIndex:
		// No text input to focus, focus style handled in View()
	}

	return cmds
}

func (m *model) buildHostFromForm() (config.SSHHost, error) {
	host := config.SSHHost{}

	host.Name = strings.TrimSpace(m.formInputs[0].Value())
	if host.Name == "" {
		return host, fmt.Errorf("name is required")
	}
	host.Hostname = strings.TrimSpace(m.formInputs[1].Value())
	if host.Hostname == "" {
		return host, fmt.Errorf("hostname is required")
	}
	host.User = strings.TrimSpace(m.formInputs[2].Value())
	if host.User == "" {
		return host, fmt.Errorf("user is required")
	}
	host.RemoteRoot = strings.TrimSpace(m.formInputs[4].Value())

	portStr := strings.TrimSpace(m.formInputs[3].Value())
	if portStr == "" {
		host.Port = 0
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return host, fmt.Errorf("invalid port number: %s", portStr)
		}
		if port == 22 {
			host.Port = 0
		} else {
			host.Port = port
		}
	}

	switch m.formAuthMethod {
	case authMethodKey:
		host.KeyPath = strings.TrimSpace(m.formInputs[5].Value())
		if host.KeyPath == "" {
			return host, fmt.Errorf("key path is required for Key File authentication")
		}
	case authMethodPassword:
		host.Password = m.formInputs[6].Value()
		if host.Password == "" {
			return host, fmt.Errorf("password is required for Password authentication")
		}
	case authMethodAgent:
		host.KeyPath = ""
		host.Password = ""
	default:
		return host, fmt.Errorf("invalid authentication method selected")
	}

	for _, existingHost := range m.configuredHosts {
		if existingHost.Name == host.Name {
			return host, fmt.Errorf("host name '%s' already exists", host.Name)
		}
	}

	return host, nil
}

func (m *model) buildHostFromEditForm() (config.SSHHost, error) {
	if m.hostToEdit == nil {
		return config.SSHHost{}, fmt.Errorf("internal error: hostToEdit is nil")
	}
	originalHost := *m.hostToEdit
	editedHost := config.SSHHost{}

	editedHost.Name = strings.TrimSpace(m.formInputs[0].Value())
	if editedHost.Name == "" {
		editedHost.Name = originalHost.Name
	}

	editedHost.Hostname = strings.TrimSpace(m.formInputs[1].Value())
	if editedHost.Hostname == "" {
		editedHost.Hostname = originalHost.Hostname
	}

	editedHost.User = strings.TrimSpace(m.formInputs[2].Value())
	if editedHost.User == "" {
		editedHost.User = originalHost.User
	}

	editedHost.RemoteRoot = strings.TrimSpace(m.formInputs[4].Value())
	if editedHost.RemoteRoot == "" {
		editedHost.RemoteRoot = originalHost.RemoteRoot
	}

	if editedHost.Name == "" {
		return editedHost, fmt.Errorf("name cannot be empty")
	}
	if editedHost.Hostname == "" {
		return editedHost, fmt.Errorf("hostname cannot be empty")
	}
	if editedHost.User == "" {
		return editedHost, fmt.Errorf("user cannot be empty")
	}

	portStr := strings.TrimSpace(m.formInputs[3].Value())
	if portStr == "" {
		editedHost.Port = 0
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return editedHost, fmt.Errorf("invalid port number: %s", portStr)
		}
		if port == 22 {
			editedHost.Port = 0
		} else {
			editedHost.Port = port
		}
	}

	keyPathInput := strings.TrimSpace(m.formInputs[5].Value())
	passwordInput := m.formInputs[6].Value()

	switch m.formAuthMethod {
	case authMethodKey:
		if keyPathInput == "" {
			editedHost.KeyPath = originalHost.KeyPath
		} else {
			editedHost.KeyPath = keyPathInput
		}
		if editedHost.KeyPath == "" {
			return editedHost, fmt.Errorf("key path is required for Key File authentication")
		}
		editedHost.Password = ""
	case authMethodPassword:
		if passwordInput == "" {
			if originalHost.Password != "" && originalHost.KeyPath == "" {
				editedHost.Password = originalHost.Password
			} else {
				return editedHost, fmt.Errorf("password is required for Password authentication")
			}
		} else {
			editedHost.Password = passwordInput
		}
		if editedHost.Password == "" {
			return editedHost, fmt.Errorf("password is required for Password authentication")
		}
		editedHost.KeyPath = ""
	case authMethodAgent:
		editedHost.KeyPath = ""
		editedHost.Password = ""
	default:
		return editedHost, fmt.Errorf("invalid authentication method selected")
	}

	if editedHost.Name != originalHost.Name {
		for _, existingHost := range m.configuredHosts {
			if existingHost.Name != originalHost.Name && existingHost.Name == editedHost.Name {
				return editedHost, fmt.Errorf("host name '%s' already exists", editedHost.Name)
			}
		}
	}

	editedHost.Disabled = m.formDisabled

	return editedHost, nil
}

func (m *model) handleProjectListKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd
	cursorMoved := false

	switch {
	case key.Matches(msg, m.keymap.Up):
		if m.cursor > 0 {
			m.cursor--
			cursorMoved = true
		}
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	case key.Matches(msg, m.keymap.Down):
		if m.cursor < len(m.projects)-1 {
			m.cursor++
			cursorMoved = true
		}
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	case key.Matches(msg, m.keymap.Home):
		if m.cursor != 0 {
			m.cursor = 0
			cursorMoved = true
			m.viewport.GotoTop()
		}
	case key.Matches(msg, m.keymap.End):
		lastIdx := len(m.projects) - 1
		if lastIdx >= 0 && m.cursor != lastIdx {
			m.cursor = lastIdx
			cursorMoved = true
			m.viewport.GotoBottom()
		}
	case key.Matches(msg, m.keymap.PgUp):
		m.cursor -= m.viewport.Height
		if m.cursor < 0 {
			m.cursor = 0
		}
		cursorMoved = true
		m.viewport.ViewUp()
	case key.Matches(msg, m.keymap.PgDown):
		m.cursor += m.viewport.Height
		lastIdx := len(m.projects) - 1
		if lastIdx >= 0 && m.cursor > lastIdx {
			m.cursor = lastIdx
		}
		cursorMoved = true
		m.viewport.ViewDown()
	default:
		switch {
		case key.Matches(msg, m.keymap.Select):
			if len(m.projects) > 0 && m.cursor < len(m.projects) {
				if _, ok := m.selectedProjectIdxs[m.cursor]; ok {
					delete(m.selectedProjectIdxs, m.cursor)
				} else {
					m.selectedProjectIdxs[m.cursor] = struct{}{}
				}
			}
		case key.Matches(msg, m.keymap.UpAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.UpSequence)...)
		case key.Matches(msg, m.keymap.DownAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.DownSequence)...)
		case key.Matches(msg, m.keymap.RefreshAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.RefreshSequence)...)
		case key.Matches(msg, m.keymap.Enter):
			if len(m.selectedProjectIdxs) > 0 {
				m.projectsInSequence = []*discovery.Project{}
				for idx := range m.selectedProjectIdxs {
					if idx >= 0 && idx < len(m.projects) {
						proj := m.projects[idx]
						m.projectsInSequence = append(m.projectsInSequence, &proj)
						projID := proj.Identifier()
						if _, loaded := m.projectStatuses[projID]; !loaded && !m.loadingStatus[projID] {
							m.loadingStatus[projID] = true
							cmds = append(cmds, fetchProjectStatusCmd(proj))
						}
					}
				}
				m.detailedProject = nil
				m.selectedProjectIdxs = make(map[int]struct{})
				m.currentState = stateProjectDetails
			} else if len(m.projects) > 0 && m.cursor < len(m.projects) {
				m.detailedProject = &m.projects[m.cursor]
				m.projectsInSequence = nil
				m.currentState = stateProjectDetails
				projID := m.detailedProject.Identifier()
				if !m.loadingStatus[projID] {
					m.loadingStatus[projID] = true
					cmds = append(cmds, fetchProjectStatusCmd(*m.detailedProject))
				}
			}
		}
	}

	if cursorMoved && len(m.projects) > 0 {
		selectedProject := m.projects[m.cursor]
		projID := selectedProject.Identifier()
		if _, loaded := m.projectStatuses[projID]; !loaded && !m.loadingStatus[projID] {
			m.loadingStatus[projID] = true
			cmds = append(cmds, fetchProjectStatusCmd(selectedProject))
		}
	}

	return cmds
}

func (m *model) runSequenceOnSelection(sequenceFunc func(discovery.Project) []runner.CommandStep) []tea.Cmd {
	var cmds []tea.Cmd
	var projectsToRun []*discovery.Project
	var combinedSequence []runner.CommandStep
	m.projectsInSequence = nil

	if len(m.selectedProjectIdxs) > 0 {
		for idx := range m.selectedProjectIdxs {
			if idx >= 0 && idx < len(m.projects) {
				projectsToRun = append(projectsToRun, &m.projects[idx])
			}
		}
		m.selectedProjectIdxs = make(map[int]struct{})
	} else if len(m.projects) > 0 && m.cursor < len(m.projects) {
		projectsToRun = append(projectsToRun, &m.projects[m.cursor])
	}

	if len(projectsToRun) == 0 {
		return cmds
	}

	m.projectsInSequence = projectsToRun
	for _, projPtr := range projectsToRun {
		if projPtr != nil {
			combinedSequence = slices.Concat(combinedSequence, sequenceFunc(*projPtr))
		}
	}

	if len(combinedSequence) > 0 {
		if len(projectsToRun) > 0 && projectsToRun[0] != nil {
			m.sequenceProject = projectsToRun[0]
		} else {
			m.sequenceProject = nil
		}
		m.currentSequence = combinedSequence
		m.currentState = stateRunningSequence
		m.currentStepIndex = 0
		m.outputContent = ""
		m.lastError = nil
		m.viewport.GotoTop()
		cmds = append(cmds, m.startNextStepCmd())
	}

	return cmds
}

func (m *model) startNextStepCmd() tea.Cmd {
	if m.currentSequence == nil || m.currentStepIndex >= len(m.currentSequence) {
		return nil
	}
	step := m.currentSequence[m.currentStepIndex]
	m.outputContent += stepStyle.Render(fmt.Sprintf("\n--- Starting Step: %s for %s ---", step.Name, step.Project.Identifier())) + "\n"
	m.viewport.SetContent(m.outputContent)
	m.viewport.GotoBottom()
	return runStepCmd(step)
}

func (m *model) renderProjectStatus(b *strings.Builder, proj *discovery.Project, projID string) {
	statusStr := ""
	statusInfo, loaded := m.projectStatuses[projID]
	isLoading := m.loadingStatus[projID]

	if isLoading {
		statusStr = statusLoadingStyle.Render(" [loading...]")
	} else if !loaded {
		statusStr = statusLoadingStyle.Render(" [?]")
	} else {
		switch statusInfo.OverallStatus {
		case runner.StatusUp:
			statusStr = statusUpStyle.Render(" [UP]")
		case runner.StatusDown:
			statusStr = statusDownStyle.Render(" [DOWN]")
		case runner.StatusPartial:
			statusStr = statusPartialStyle.Render(" [PARTIAL]")
		case runner.StatusError:
			statusStr = statusErrorStyle.Render(" [ERROR]")
		default:
			statusStr = statusLoadingStyle.Render(" [?]")
		}
	}
	b.WriteString(fmt.Sprintf("\nOverall Status:%s\n", statusStr))
	if !isLoading && loaded && statusInfo.Error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error fetching status: %v\n", statusInfo.Error)))
	}
	if !isLoading && loaded && len(statusInfo.Containers) > 0 {
		b.WriteString("\nContainers:\n")
		b.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "SERVICE", "CONTAINER NAME", "STATUS"))
		b.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "-------", "--------------", "------"))
		for _, c := range statusInfo.Containers {
			isUp := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
			statusRenderFunc := statusDownStyle.Render
			if isUp {
				statusRenderFunc = statusUpStyle.Render
			}
			b.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", c.Service, c.Name, statusRenderFunc(c.Status)))
		}
	} else if !isLoading && loaded && statusInfo.OverallStatus != runner.StatusError {
		b.WriteString("\n  (No containers found or running)\n")
	}
}

func (m *model) View() string {
	if !m.ready {
		return "Initializing..."
	}
	var header, bodyStr, footerStr string
	header = titleStyle.Render("Bucket Manager TUI") + "\n"
	bodyContent := strings.Builder{}

	footerContent := strings.Builder{}
	footerContent.WriteString("\n")

	switch m.currentState {
	case stateProjectList:
		if m.isDiscovering {
			footerContent.WriteString(statusLoadingStyle.Render("Discovering remote projects...") + "\n")
		}
		if len(m.discoveryErrors) > 0 {
			footerContent.WriteString(errorStyle.Render("Discovery Errors:"))
			for _, err := range m.discoveryErrors {
				footerContent.WriteString("\n  " + errorStyle.Render(err.Error()))
			}
			footerContent.WriteString("\n")
		} else if m.lastError != nil && strings.Contains(m.lastError.Error(), "discovery") {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Discovery Warning: %v", m.lastError)) + "\n")
		}

		help := strings.Builder{}
		if len(m.selectedProjectIdxs) > 0 {
			help.WriteString(fmt.Sprintf("(%d selected) ", len(m.selectedProjectIdxs)))
		}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + ": navigate | ")
		help.WriteString(m.keymap.Select.Help().Key + ": " + m.keymap.Select.Help().Desc + " | ")
		help.WriteString(m.keymap.Enter.Help().Key + ": details | ")
		help.WriteString(m.keymap.UpAction.Help().Key + ": up | ")
		help.WriteString(m.keymap.DownAction.Help().Key + ": down | ")
		help.WriteString(m.keymap.RefreshAction.Help().Key + ": refresh")
		help.WriteString(" | ")
		help.WriteString(m.keymap.Config.Help().Key + ": " + m.keymap.Config.Help().Desc + " | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateRunningSequence:
		projectIdentifier := ""
		if m.sequenceProject != nil {
			projectIdentifier = fmt.Sprintf(" for %s", m.sequenceProject.Identifier())
		}
		if m.currentSequence != nil && m.currentStepIndex < len(m.currentSequence) {
			footerContent.WriteString(statusStyle.Render(fmt.Sprintf("Running step %d/%d%s: %s...", m.currentStepIndex+1, len(m.currentSequence), projectIdentifier, m.currentSequence[m.currentStepIndex].Name)))
		} else if m.sequenceProject != nil {
			footerContent.WriteString(successStyle.Render(fmt.Sprintf("Sequence finished successfully%s.", projectIdentifier)))
		} else {
			footerContent.WriteString(successStyle.Render("Sequence finished successfully."))
		}
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + "/" + m.keymap.PgUp.Help().Key + "/" + m.keymap.PgDown.Help().Key + ": scroll | ")
		help.WriteString(m.keymap.Back.Help().Key + "/" + m.keymap.Enter.Help().Key + ": back to list | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateSequenceError:
		projectIdentifier := ""
		if m.sequenceProject != nil {
			projectIdentifier = fmt.Sprintf(" for %s", m.sequenceProject.Identifier())
		}
		if m.lastError != nil {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Error%s: %v", projectIdentifier, m.lastError)))
		} else {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("An unknown error occurred%s.", projectIdentifier)))
		}
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + "/" + m.keymap.PgUp.Help().Key + "/" + m.keymap.PgDown.Help().Key + ": scroll | ")
		help.WriteString(m.keymap.Back.Help().Key + "/" + m.keymap.Enter.Help().Key + ": back to list | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateProjectDetails:
		help := strings.Builder{}
		help.WriteString(m.keymap.Back.Help().Key + ": back to list | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateSshConfigList:
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + ": navigate | ")
		help.WriteString(m.keymap.Add.Help().Key + ": " + m.keymap.Add.Help().Desc + " | ")
		help.WriteString(m.keymap.Edit.Help().Key + ": " + m.keymap.Edit.Help().Desc + " | ")
		help.WriteString(m.keymap.Remove.Help().Key + ": " + m.keymap.Remove.Help().Desc + " | ")
		help.WriteString(m.keymap.Import.Help().Key + ": " + m.keymap.Import.Help().Desc + " | ")
		help.WriteString(m.keymap.Back.Help().Key + ": back | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)

		errorOrInfo := ""
		if m.importInfoMsg != "" {
			errorOrInfo = "\n" + successStyle.Render(m.importInfoMsg)
		} else if m.importError != nil {
			errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Import Error: %v", m.importError))
		} else if m.lastError != nil && strings.Contains(m.lastError.Error(), "ssh config") {
			errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Config Error: %v", m.lastError))
		}

		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))
		if errorOrInfo != "" {
			footerContent.WriteString(errorOrInfo)
		}

	case stateSshConfigRemoveConfirm:
		help := strings.Builder{}
		if m.hostToRemove != nil {
			help.WriteString(fmt.Sprintf("Confirm removal of '%s'? ", identifierColor.Render(m.hostToRemove.Name)))
			help.WriteString(m.keymap.Yes.Help().Key + ": " + m.keymap.Yes.Help().Desc + " | ")
			help.WriteString(m.keymap.No.Help().Key + "/" + m.keymap.Back.Help().Key + ": " + m.keymap.No.Help().Desc + "/cancel")
		} else {
			help.WriteString(errorStyle.Render("Error - no host selected. "))
			help.WriteString(m.keymap.Back.Help().Key + ": back")
		}
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateSshConfigAddForm, stateSshConfigEditForm:
		if m.formError != nil {
			footerContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
		}
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + "/" + m.keymap.Tab.Help().Key + "/" + m.keymap.ShiftTab.Help().Key + ": navigate | ")
		if m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm {
			help.WriteString(m.keymap.Left.Help().Key + "/" + m.keymap.Right.Help().Key + ": change auth | ")
		}
		if m.currentState == stateSshConfigEditForm {
			help.WriteString(m.keymap.ToggleDisabled.Help().Key + ": " + m.keymap.ToggleDisabled.Help().Desc + " | ")
		}
		help.WriteString(m.keymap.Enter.Help().Key + ": save | ")
		help.WriteString(m.keymap.Esc.Help().Key + ": " + m.keymap.Esc.Help().Desc + " | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateSshConfigImportSelect:
		help := strings.Builder{}
		if len(m.selectedImportIdxs) > 0 {
			help.WriteString(fmt.Sprintf("(%d selected) ", len(m.selectedImportIdxs)))
		}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + ": navigate | ")
		help.WriteString(m.keymap.Select.Help().Key + ": " + m.keymap.Select.Help().Desc + " | ")
		help.WriteString(m.keymap.Enter.Help().Key + ": confirm")
		help.WriteString(" | " + m.keymap.Back.Help().Key + ": cancel | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateSshConfigImportDetails:
		remaining := 0
		for i := m.configuringHostIdx + 1; i < len(m.importableHosts); i++ {
			if _, ok := m.selectedImportIdxs[i]; ok {
				remaining++
			}
		}
		hostLabel := "host"
		if remaining != 1 {
			hostLabel = "hosts"
		}
		if m.formError != nil {
			footerContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
		}
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + "/" + m.keymap.Tab.Help().Key + "/" + m.keymap.ShiftTab.Help().Key + ": navigate | ")
		help.WriteString(m.keymap.Left.Help().Key + "/" + m.keymap.Right.Help().Key + ": change auth | ")
		help.WriteString(fmt.Sprintf("%s: confirm & next (%d %s remaining) | ", m.keymap.Enter.Help().Key, remaining, hostLabel))
		help.WriteString(m.keymap.Esc.Help().Key + ": cancel import | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String()))

	default:
		help := strings.Builder{}
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))
	}
	footerStr = footerContent.String()
	actualFooterHeight := lipgloss.Height(footerStr)

	availableHeight := m.height - headerHeight - actualFooterHeight
	if availableHeight < 1 {
		availableHeight = 1
	}

	switch m.currentState {
	case stateLoadingProjects:
		bodyContent.WriteString(statusStyle.Render("Loading projects..."))
	case stateProjectList:
		listContent := strings.Builder{}
		listContent.WriteString("Select a project:\n")
		for i, project := range m.projects {
			cursor := "  "
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
			}

			checkbox := "[ ]"
			if _, selected := m.selectedProjectIdxs[i]; selected {
				checkbox = successStyle.Render("[x]")
			}

			projID := project.Identifier()
			statusStr := ""
			if m.loadingStatus[projID] {
				statusStr = statusLoadingStyle.Render(" [loading...]")
			} else if statusInfo, ok := m.projectStatuses[projID]; ok {
				switch statusInfo.OverallStatus {
				case runner.StatusUp:
					statusStr = statusUpStyle.Render(" [UP]")
				case runner.StatusDown:
					statusStr = statusDownStyle.Render(" [DOWN]")
				case runner.StatusPartial:
					statusStr = statusPartialStyle.Render(" [PARTIAL]")
				case runner.StatusError:
					statusStr = statusErrorStyle.Render(" [ERROR]")
				default:
					statusStr = statusLoadingStyle.Render(" [?]")
				}
			} else {
				statusStr = statusLoadingStyle.Render(" [?]")
			}
			listContent.WriteString(fmt.Sprintf("%s%s %s (%s)%s\n", cursor, checkbox, project.Name, serverNameStyle.Render(project.ServerName), statusStr))
		}
		m.viewport.Height = availableHeight
		m.viewport.SetContent(listContent.String())
		bodyStr = m.viewport.View()
	case stateRunningSequence, stateSequenceError:
		m.viewport.Height = availableHeight
		bodyStr = m.viewport.View()
	case stateProjectDetails:
		if m.detailedProject != nil {
			proj := m.detailedProject
			projID := proj.Identifier()
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for: %s (%s)", proj.Name, serverNameStyle.Render(proj.ServerName))) + "\n\n")
			m.renderProjectStatus(&bodyContent, proj, projID)
		} else if len(m.projectsInSequence) > 0 {
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for %d Selected Projects:", len(m.projectsInSequence))) + "\n")
			for i, proj := range m.projectsInSequence {
				if proj == nil {
					continue
				}
				projID := proj.Identifier()
				bodyContent.WriteString(fmt.Sprintf("\n--- %s (%s) ---", proj.Name, serverNameStyle.Render(proj.ServerName)))
				m.renderProjectStatus(&bodyContent, proj, projID)
				if i < len(m.projectsInSequence)-1 {
					bodyContent.WriteString("\n")
				}
			}
		} else {
			bodyContent.WriteString(errorStyle.Render("Error: No project selected for details."))
		}
		m.detailsViewport.Height = availableHeight
		m.detailsViewport.SetContent(bodyContent.String())
		bodyStr = m.detailsViewport.View()
	case stateSshConfigList:
		bodyContent.WriteString("Configured SSH Hosts:\n")
		if len(m.configuredHosts) == 0 {
			bodyContent.WriteString("\n  (No SSH hosts configured yet)")
		} else {
			for i, host := range m.configuredHosts {
				cursor := "  "
				if m.configCursor == i {
					cursor = cursorStyle.Render("> ")
				}
				details := fmt.Sprintf("%s@%s", host.User, host.Hostname)
				if host.Port != 0 && host.Port != 22 {
					details += fmt.Sprintf(":%d", host.Port)
				}
				status := ""
				if host.Disabled {
					status = errorStyle.Render(" [Disabled]")
				}
				remoteRootStr := ""
				if host.RemoteRoot != "" {
					remoteRootStr = fmt.Sprintf(" (Root: %s)", host.RemoteRoot)
				} else {
					remoteRootStr = fmt.Sprintf(" (Root: %s)", lipgloss.NewStyle().Faint(true).Render("[Default]"))
				}
				bodyContent.WriteString(fmt.Sprintf("%s%s (%s)%s%s\n", cursor, host.Name, serverNameStyle.Render(details), remoteRootStr, status))
			}
		}
		if m.lastError != nil && strings.Contains(m.lastError.Error(), "ssh config") {
			bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Config Error: %v", m.lastError)))
		}
		m.sshConfigViewport.Height = availableHeight
		m.sshConfigViewport.SetContent(bodyContent.String())
		bodyStr = m.sshConfigViewport.View()
	case stateSshConfigRemoveConfirm:
		if m.hostToRemove != nil {
			bodyContent.WriteString(fmt.Sprintf("Are you sure you want to remove the SSH host '%s'?\n\n", identifierColor.Render(m.hostToRemove.Name)))
			bodyContent.WriteString("[y] Yes, remove | [n/Esc/b] No, cancel")
		} else {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for removal. Press Esc/b to go back."))
		}
		bodyStr = lipgloss.PlaceVertical(availableHeight, lipgloss.Top, bodyContent.String())
	case stateSshConfigAddForm:
		bodyContent.WriteString(titleStyle.Render("Add New SSH Host") + "\n\n")
		for i := 0; i < 5; i++ {
			bodyContent.WriteString(m.formInputs[i].View() + "\n")
		}
		authFocus := "  "
		authStyle := lipgloss.NewStyle()
		if m.formFocusIndex == 5 {
			authFocus = cursorStyle.Render("> ")
			authStyle = cursorStyle
		}
		authMethodStr := ""
		switch m.formAuthMethod {
		case authMethodKey: authMethodStr = "SSH Key File"
		case authMethodAgent: authMethodStr = "SSH Agent"
		case authMethodPassword: authMethodStr = "Password (insecure)"
		}
		helpText := "[←/→ to change]"
		bodyContent.WriteString(fmt.Sprintf("%s%s\n", authFocus, authStyle.Render("Auth Method: "+authMethodStr+" "+helpText)))
		if m.formAuthMethod == authMethodKey {
			bodyContent.WriteString(m.formInputs[5].View() + "\n")
		} else if m.formAuthMethod == authMethodPassword {
			bodyContent.WriteString(m.formInputs[6].View() + "\n")
		}
		if m.formError != nil {
			bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
		}
		m.formViewport.Height = availableHeight
		m.formViewport.SetContent(bodyContent.String())
		bodyStr = m.formViewport.View()
	case stateSshConfigImportSelect:
		bodyContent.WriteString(titleStyle.Render("Select Hosts to Import from ~/.ssh/config") + "\n\n")
		if len(m.importableHosts) == 0 {
			bodyContent.WriteString(statusStyle.Render("No new importable hosts found."))
		} else {
			for i, pHost := range m.importableHosts {
				cursor := "  "
				if m.importCursor == i { cursor = cursorStyle.Render("> ") }
				checkbox := "[ ]"
				if _, selected := m.selectedImportIdxs[i]; selected { checkbox = successStyle.Render("[x]") }
				details := fmt.Sprintf("%s@%s", pHost.User, pHost.Hostname)
				if pHost.Port != 0 && pHost.Port != 22 { details += fmt.Sprintf(":%d", pHost.Port) }
				keyInfo := ""
				if pHost.KeyPath != "" { keyInfo = fmt.Sprintf(" (Key: %s)", lipgloss.NewStyle().Faint(true).Render(filepath.Base(pHost.KeyPath))) }
				bodyContent.WriteString(fmt.Sprintf("%s%s %s (%s)%s\n", cursor, checkbox, identifierColor.Render(pHost.Alias), serverNameStyle.Render(details), keyInfo))
			}
		}
		m.importSelectViewport.Height = availableHeight
		m.importSelectViewport.SetContent(bodyContent.String())
		bodyStr = m.importSelectViewport.View()
	case stateSshConfigImportDetails:
		if len(m.importableHosts) == 0 || m.configuringHostIdx >= len(m.importableHosts) {
			bodyContent.WriteString(errorStyle.Render("Error: Invalid state for import details."))
		} else {
			pHost := m.importableHosts[m.configuringHostIdx]
			title := fmt.Sprintf("Configure Import: %s (%s@%s)", identifierColor.Render(pHost.Alias), pHost.User, pHost.Hostname)
			bodyContent.WriteString(titleStyle.Render(title) + "\n\n")
			bodyContent.WriteString(m.formInputs[4].View() + "\n") // Remote Root Path
			authNeeded := pHost.KeyPath == ""
			if authNeeded {
				// Render Auth Method selection first
				authFocus := "  "
				authStyle := lipgloss.NewStyle()
				// Apply focus style if the logical focus is on the auth method selector
				if m.formFocusIndex == 1 { authFocus = cursorStyle.Render("> "); authStyle = cursorStyle }
				authMethodStr := ""
				switch m.formAuthMethod {
				case authMethodKey: authMethodStr = "SSH Key File"
				case authMethodAgent: authMethodStr = "SSH Agent"
				case authMethodPassword: authMethodStr = "Password (insecure)"
				}
				helpText := "[←/→ to change]"
				bodyContent.WriteString(fmt.Sprintf("%s%s\n", authFocus, authStyle.Render("Auth Method: "+authMethodStr+" "+helpText)))

				// Render Key Path or Password input *after* Auth Method
				if m.formAuthMethod == authMethodKey { bodyContent.WriteString(m.formInputs[5].View() + "\n") }
				if m.formAuthMethod == authMethodPassword { bodyContent.WriteString(m.formInputs[6].View() + "\n") }
			} else {
				bodyContent.WriteString(fmt.Sprintf("  Auth Method: SSH Key File (from ssh_config: %s)\n", lipgloss.NewStyle().Faint(true).Render(pHost.KeyPath)))
			}
			if m.formError != nil { bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError))) }
		}
		m.formViewport.Height = availableHeight
		m.formViewport.SetContent(bodyContent.String())
		bodyStr = m.formViewport.View()
	case stateSshConfigEditForm:
		if m.hostToEdit == nil {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for editing."))
		} else {
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Edit SSH Host: %s", identifierColor.Render(m.hostToEdit.Name))) + "\n\n")
			for i := range 5 { bodyContent.WriteString(m.formInputs[i].View() + "\n") }
			authFocus := "  "
			authStyle := lipgloss.NewStyle()
			if m.formFocusIndex == 5 { authFocus = cursorStyle.Render("> "); authStyle = cursorStyle } // Inlined getAuthMethodFocusIndex
			authMethodStr := ""
			switch m.formAuthMethod {
			case authMethodKey: authMethodStr = "SSH Key File"
			case authMethodAgent: authMethodStr = "SSH Agent"
			case authMethodPassword: authMethodStr = "Password (insecure)"
			}
			helpText := "[←/→ to change]"
			bodyContent.WriteString(fmt.Sprintf("%s%s\n", authFocus, authStyle.Render("Auth Method: "+authMethodStr+" "+helpText)))
			if m.formAuthMethod == authMethodKey { bodyContent.WriteString(m.formInputs[5].View() + "\n") }
			if m.formAuthMethod == authMethodPassword { bodyContent.WriteString(m.formInputs[6].View() + "\n") }
			disabledFocus := "  "
			disabledStyle := lipgloss.NewStyle()
			if m.formFocusIndex == 8 { disabledFocus = cursorStyle.Render("> "); disabledStyle = cursorStyle } // Inlined getDisabledToggleFocusIndex
			checkbox := "[ ]"
			if m.formDisabled { checkbox = successStyle.Render("[x]") }
			bodyContent.WriteString(fmt.Sprintf("%s%s\n", disabledFocus, disabledStyle.Render(checkbox+" Disabled Host [space to toggle]")))
			if m.formError != nil { bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError))) }
		}
		m.formViewport.Height = availableHeight
		m.formViewport.SetContent(bodyContent.String())
		bodyStr = m.formViewport.View()
	}

	if bodyStr == "" {
		bodyStr = lipgloss.PlaceVertical(availableHeight, lipgloss.Top, bodyContent.String())
	}

	finalView := lipgloss.JoinVertical(lipgloss.Left, header, bodyStr, footerStr)
	return finalView
}
