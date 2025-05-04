// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"context"
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
	"golang.org/x/sync/semaphore"
)

const (
	headerHeight = 2
)

const maxConcurrentStatusChecks = 4 // Limit concurrent status checks
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
	stateLoadingStacks state = iota
	stateStackList
	stateRunningSequence
	stateSequenceError
	stateStackDetails
	stateSshConfigList
	stateSshConfigRemoveConfirm
	stateSshConfigAddForm
	stateSshConfigImportSelect
	stateSshConfigImportDetails
	stateSshConfigEditForm
	statePruneConfirm
	stateRunningHostAction
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
	PullAction    key.Binding

	Remove key.Binding
	Add    key.Binding
	Import key.Binding
	Edit   key.Binding

	ToggleDisabled key.Binding
	PruneAction    key.Binding
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
		key.WithHelp("c", "configure hosts"),
	),
	UpAction: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "up stack(s)"),
	),
	DownAction: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "down stack(s)"),
	),
	RefreshAction: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh stack(s)"),
	),
	PullAction: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pull images"),
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
	PruneAction: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "prune host"),
	),
}

type model struct {
	keymap               KeyMap
	stacks               []discovery.Stack
	cursor               int
	selectedStackIdxs    map[int]struct{}
	configCursor         int
	hostToRemove         *config.SSHHost
	hostToEdit           *config.SSHHost
	configuredHosts      []config.SSHHost
	viewport             viewport.Model
	sshConfigViewport    viewport.Model
	detailsViewport      viewport.Model
	formViewport         viewport.Model
	importSelectViewport viewport.Model
	currentState         state
	isDiscovering        bool
	currentSequence      []runner.CommandStep
	currentStepIndex     int
	outputContent        string
	lastError            error
	discoveryErrors      []error
	ready                bool
	width                int
	height               int
	outputChan           <-chan runner.OutputLine
	errorChan            <-chan error
	stackStatuses        map[string]runner.StackRuntimeInfo
	loadingStatus        map[string]bool
	detailedStack        *discovery.Stack
	sequenceStack        *discovery.Stack   // The primary stack for the current sequence (used for display)
	stacksInSequence     []*discovery.Stack // All stacks involved in the current sequence

	// Host action state
	hostsToPrune          []runner.HostTarget // Hosts targeted for prune action
	currentHostActionStep runner.HostCommandStep
	hostActionError       error

	// Form state (Add/Edit/Import Details)
	formInputs     []textinput.Model
	formFocusIndex int  // Logical focus index within the current form
	formAuthMethod int  // Selected auth method (authMethodKey, etc.)
	formDisabled   bool // For edit form's disabled toggle
	formError      error

	// Import state
	importableHosts    []config.PotentialHost
	selectedImportIdxs map[int]struct{}
	importCursor       int
	importError        error
	importInfoMsg      string
	hostsToConfigure   []config.SSHHost    // Hosts built from import details form
	configuringHostIdx int                 // Index in importableHosts currently being configured
	statusCheckSem     *semaphore.Weighted // Semaphore for limiting status checks
}

// --- Messages ---

type stackDiscoveredMsg struct{ stack discovery.Stack }
type discoveryErrorMsg struct{ err error }
type discoveryFinishedMsg struct{}
type sshConfigLoadedMsg struct {
	hosts []config.SSHHost
	Err   error
}
type sshHostAddedMsg struct{ err error }
type sshHostEditedMsg struct{ err error }
type sshConfigParsedMsg struct {
	potentialHosts []config.PotentialHost
	err            error
}
type sshHostsImportedMsg struct {
	importedCount int
	skippedCount  int
	err           error
}
type outputLineMsg struct{ line runner.OutputLine }
type stepFinishedMsg struct{ err error }
type stackStatusLoadedMsg struct {
	stackIdentifier string
	statusInfo      runner.StackRuntimeInfo
}
type channelsAvailableMsg struct {
	outChan <-chan runner.OutputLine
	errChan <-chan error
}

func findStacksCmd() tea.Cmd {
	return func() tea.Msg {
		stackChan, errorChan, doneChan := discovery.FindStacks()

		go func() {
			for s := range stackChan {
				if BubbleProgram != nil {
					BubbleProgram.Send(stackDiscoveredMsg{stack: s})
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

// fetchStackStatusCmd fetches the status for a single stack, respecting concurrency limits.
func (m *model) fetchStackStatusCmd(stack discovery.Stack) tea.Cmd {
	return func() tea.Msg {
		// Acquire semaphore
		ctx := context.Background()
		if err := m.statusCheckSem.Acquire(ctx, 1); err != nil {
			// If acquiring fails, return an error status immediately
			return stackStatusLoadedMsg{
				stackIdentifier: stack.Identifier(),
				statusInfo: runner.StackRuntimeInfo{
					Stack:         stack,
					OverallStatus: runner.StatusError,
					Error:         fmt.Errorf("failed to acquire status check semaphore: %w", err),
				},
			}
		}
		defer m.statusCheckSem.Release(1) // Release semaphore when done

		// Semaphore acquired, proceed with status check
		statusInfo := runner.GetStackStatus(stack)
		return stackStatusLoadedMsg{
			stackIdentifier: stack.Identifier(),
			statusInfo:      statusInfo,
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

		// If all selected hosts already existed, return a specific error
		if len(finalHostsToAdd) == 0 && skippedCount > 0 {
			return sshHostsImportedMsg{
				importedCount: 0,
				skippedCount:  skippedCount,
				err:           fmt.Errorf("all %d selected host(s) already exist or conflict", skippedCount),
			}
		}

		// Only save if there are hosts to add
		if len(finalHostsToAdd) > 0 {
			cfg.SSHHosts = slices.Concat(cfg.SSHHosts, finalHostsToAdd)
			err = config.SaveConfig(cfg)
			if err != nil {
				// Return a real save error
				return sshHostsImportedMsg{
					importedCount: 0, // Indicate failure
					skippedCount:  skippedCount,
					err:           fmt.Errorf("failed to save config after import: %w", err),
				}
			}
		}

		// Success: return counts and nil error
		return sshHostsImportedMsg{
			importedCount: len(finalHostsToAdd),
			skippedCount:  skippedCount,
			err:           nil, // Explicitly nil on success
		}
	}
}

// runHostActionCmd triggers the execution of a host-level command step in TUI mode.
func runHostActionCmd(step runner.HostCommandStep) tea.Cmd {
	return func() tea.Msg {
		// TUI always uses cliMode: false for channel-based output
		outChan, errChan := runner.RunHostCommand(step, false)
		return channelsAvailableMsg{outChan: outChan, errChan: errChan}
	}
}

func runStepCmd(step runner.CommandStep) tea.Cmd {
	return func() tea.Msg {
		// TUI always uses cliMode: false for channel-based output
		outChan, errChan := runner.StreamCommand(step, false)
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
		currentState:         stateLoadingStacks,
		isDiscovering:        true,
		cursor:               0,
		selectedStackIdxs:    make(map[int]struct{}),
		configCursor:         0,
		stackStatuses:        make(map[string]runner.StackRuntimeInfo),
		loadingStatus:        make(map[string]bool),
		configuredHosts:      []config.SSHHost{},
		discoveryErrors:      []error{},
		detailedStack:        nil,
		sequenceStack:        nil,
		stacksInSequence:     nil,
		viewport:             vp,
		sshConfigViewport:    vp,
		detailsViewport:      vp,
		formViewport:         vp,
		importSelectViewport: vp,
		statusCheckSem:       semaphore.NewWeighted(maxConcurrentStatusChecks),
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

	// Add placeholders for unused fields to maintain array size consistency
	for i := 0; i < 4; i++ {
		inputs[i] = textinput.New()
	}

	return inputs, initialAuthMethod
}

func (m *model) Init() tea.Cmd {
	return findStacksCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	viewportActive := m.currentState == stateRunningSequence

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
				for _, stack := range m.stacksInSequence {
					if stack != nil {
						stackID := stack.Identifier()
						if !m.loadingStatus[stackID] {
							m.loadingStatus[stackID] = true
							cmds = append(cmds, m.fetchStackStatusCmd(*stack))
						}
					}
				}
				m.currentState = stateStackList
				m.outputContent = ""
				m.lastError = nil
				m.currentSequence = nil
				m.currentStepIndex = 0
				m.sequenceStack = nil
				m.stacksInSequence = nil
				m.viewport.GotoTop()
				return m, tea.Batch(cmds...)
			}

			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
		} else if isFormState := m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm || m.currentState == stateSshConfigImportDetails; isFormState {
			// Handle text input updates *before* navigation logic
			var focusedInputIndex = -1
			switch m.currentState {
			case stateSshConfigAddForm:
				switch m.formFocusIndex {
				case 0, 1, 2, 3, 4: // Name, Hostname, User, Port, RemoteRoot
					focusedInputIndex = m.formFocusIndex
				case 6: // Key Path (only focusable if authMethodKey)
					if m.formAuthMethod == authMethodKey {
						focusedInputIndex = 5
					}
				case 7: // Password (only focusable if authMethodPassword)
					if m.formAuthMethod == authMethodPassword {
						focusedInputIndex = 6
					}
				}
			case stateSshConfigEditForm:
				switch m.formFocusIndex {
				case 0, 1, 2, 3, 4: // Name, Hostname, User, Port, RemoteRoot
					focusedInputIndex = m.formFocusIndex
				case 6: // Key Path (only focusable if authMethodKey)
					if m.formAuthMethod == authMethodKey {
						focusedInputIndex = 5
					}
				case 7: // Password (only focusable if authMethodPassword)
					if m.formAuthMethod == authMethodPassword {
						focusedInputIndex = 6
					}
				}
			case stateSshConfigImportDetails:
				const (
					remoteRootFocusIndex    = 0
					authMethodFocusIndex    = 1
					keyOrPasswordFocusIndex = 2
				)
				authNeeded := false
				if m.configuringHostIdx >= 0 && m.configuringHostIdx < len(m.importableHosts) {
					authNeeded = m.importableHosts[m.configuringHostIdx].KeyPath == ""
				}

				switch m.formFocusIndex {
				case remoteRootFocusIndex:
					focusedInputIndex = 4 // Actual index in m.formInputs for Remote Root
				case keyOrPasswordFocusIndex:
					if authNeeded {
						switch m.formAuthMethod {
						case authMethodKey:
							focusedInputIndex = 5 // Actual index for Key Path
						case authMethodPassword:
							focusedInputIndex = 6 // Actual index for Password
						}
					}
				}
				// Focus index 1 (Auth Method) doesn't have a text input
			}

			if focusedInputIndex != -1 && focusedInputIndex < len(m.formInputs) {
				var inputCmd tea.Cmd
				m.formInputs[focusedInputIndex], inputCmd = m.formInputs[focusedInputIndex].Update(msg)
				cmds = append(cmds, inputCmd)
			}
		}

		// Handle navigation and other keys based on state
		switch m.currentState {
		case stateStackList:
			switch {
			case key.Matches(msg, m.keymap.Config):
				m.currentState = stateSshConfigList
				m.configCursor = 0
				cmds = append(cmds, loadSshConfigCmd())
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			default:
				cmds = append(cmds, m.handleStackListKeys(msg)...)
			}

		case stateStackDetails:
			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back):
				m.currentState = stateStackList
				m.detailedStack = nil
			}

		case stateSshConfigList:
			// Calculate the total number of items including "local"
			totalItems := len(m.configuredHosts) + 1

			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back):
				m.currentState = stateStackList
				m.lastError = nil
				m.importError = nil
				m.importInfoMsg = ""
			case key.Matches(msg, m.keymap.Up):
				if m.configCursor > 0 { // Stop at "local" (index 0)
					m.configCursor--
				}
				m.sshConfigViewport.ScrollUp(1)
			case key.Matches(msg, m.keymap.Down):
				if m.configCursor < totalItems-1 { // Stop at the last item
					m.configCursor++
				}
				m.sshConfigViewport.ScrollDown(1)
			case key.Matches(msg, m.keymap.PgUp), key.Matches(msg, m.keymap.Home):
				m.sshConfigViewport, vpCmd = m.sshConfigViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			case key.Matches(msg, m.keymap.PgDown), key.Matches(msg, m.keymap.End):
				m.sshConfigViewport, vpCmd = m.sshConfigViewport.Update(msg)
				cmds = append(cmds, vpCmd)
			case key.Matches(msg, m.keymap.Remove):
				// Only allow removing remote hosts (cursor > 0)
				if m.configCursor > 0 && m.configCursor < totalItems {
					remoteHostIndex := m.configCursor - 1 // Adjust index for configuredHosts slice
					m.hostToRemove = &m.configuredHosts[remoteHostIndex]
					m.currentState = stateSshConfigRemoveConfirm
					m.lastError = nil
				} else {
					m.lastError = fmt.Errorf("cannot remove 'local' host")
				}
			case key.Matches(msg, m.keymap.Add):
				// Add is always allowed, doesn't depend on selection
				m.formInputs = createAddForm()
				m.formFocusIndex = 0
				m.formAuthMethod = authMethodAgent
				m.formError = nil
				m.currentState = stateSshConfigAddForm
				m.formViewport.GotoTop()
				if len(m.formInputs) > 0 {
					m.formInputs[0].Prompt = cursorStyle.Render("> ")
					m.formInputs[0].TextStyle = cursorStyle
					cmds = append(cmds, m.formInputs[0].Focus())
				}
			case key.Matches(msg, m.keymap.Import):
				// Import is always allowed
				m.currentState = stateLoadingStacks // Show loading while parsing
				m.importError = nil
				m.lastError = nil
				cmds = append(cmds, parseSshConfigCmd())
			case key.Matches(msg, m.keymap.Edit):
				// Only allow editing remote hosts (cursor > 0)
				if m.configCursor > 0 && m.configCursor < totalItems {
					remoteHostIndex := m.configCursor - 1 // Adjust index for configuredHosts slice
					m.hostToEdit = &m.configuredHosts[remoteHostIndex]
					m.formInputs, m.formAuthMethod, m.formDisabled = createEditForm(*m.hostToEdit)
					m.formFocusIndex = 0
					m.formError = nil
					m.currentState = stateSshConfigEditForm
					m.formViewport.GotoTop()
					if len(m.formInputs) > 0 {
						m.formInputs[0].Prompt = cursorStyle.Render("> ")
						m.formInputs[0].TextStyle = cursorStyle
						cmds = append(cmds, m.formInputs[0].Focus())
					}
				} else {
					m.lastError = fmt.Errorf("cannot edit 'local' host")
				}
			case key.Matches(msg, m.keymap.PruneAction):
				m.hostsToPrune = nil // Reset before setting
				m.hostActionError = nil
				m.lastError = nil

				if m.configCursor == 0 {
					// Target local host
					m.hostsToPrune = []runner.HostTarget{{IsRemote: false, ServerName: "local"}}
				} else if m.configCursor > 0 && m.configCursor < totalItems {
					// Target the selected remote host
					remoteHostIndex := m.configCursor - 1
					host := m.configuredHosts[remoteHostIndex]
					if !host.Disabled {
						m.hostsToPrune = append(m.hostsToPrune, runner.HostTarget{IsRemote: true, HostConfig: &host, ServerName: host.Name})
					} else {
						m.lastError = fmt.Errorf("cannot prune disabled host: %s", host.Name)
					}
				} else {
					m.lastError = fmt.Errorf("invalid selection for prune action")
				}

				// Proceed to confirmation only if a valid target was set and no error occurred
				if len(m.hostsToPrune) > 0 && m.lastError == nil {
					m.currentState = statePruneConfirm
				}
			}
			// Update viewport only if no specific command was generated for it yet
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
				m.importSelectViewport.ScrollUp(1)
			case key.Matches(msg, m.keymap.Down):
				if m.importCursor < len(m.importableHosts)-1 {
					m.importCursor++
				}
				m.importSelectViewport.ScrollDown(1)
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
							cmds = append(cmds, m.formInputs[4].Focus())
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

		case statePruneConfirm:
			switch {
			case key.Matches(msg, m.keymap.Yes):
				if len(m.hostsToPrune) > 0 {
					// Add an initial message immediately
					m.outputContent = statusStyle.Render(fmt.Sprintf("Initiating prune for %s...", m.hostsToPrune[0].ServerName)) + "\n"
					m.currentState = stateRunningHostAction
					m.hostActionError = nil
					// For now, TUI only prunes one host at a time
					step := runner.PruneHostStep(m.hostsToPrune[0])
					m.currentHostActionStep = step // Store the step being run
					// Ensure viewport shows the initial message before command starts
					m.viewport.SetContent(m.outputContent)
					m.viewport.GotoBottom()
					cmds = append(cmds, runHostActionCmd(step))
				} else {
					// Should not happen, but reset state if it does
					m.currentState = stateSshConfigList
					m.lastError = fmt.Errorf("internal error: no hosts targeted for prune")
				}
			case key.Matches(msg, m.keymap.No), key.Matches(msg, m.keymap.Back):
				m.currentState = stateSshConfigList
				m.hostsToPrune = nil
				m.lastError = nil
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
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

		// Check for actual errors (save failed, or all hosts conflicted)
		if msg.err != nil {
			m.importError = msg.err
		} else {
			// Build success/info message based on counts
			info := fmt.Sprintf("Import finished: %d host(s) added.", msg.importedCount)
			if msg.skippedCount > 0 {
				info += fmt.Sprintf(" Skipped %d host(s) due to existing names.", msg.skippedCount)
			}
			m.importInfoMsg = info
		}

		// Clean up import state regardless of success/failure
		m.importableHosts = nil
		m.selectedImportIdxs = nil
		m.hostsToConfigure = nil
		m.formInputs = nil
		cmds = append(cmds, loadSshConfigCmd())

	case stackDiscoveredMsg:
		if m.currentState == stateLoadingStacks {
			m.currentState = stateStackList
		}
		m.stacks = append(m.stacks, msg.stack)
		stackID := msg.stack.Identifier()
		if !m.loadingStatus[stackID] {
			m.loadingStatus[stackID] = true
			cmds = append(cmds, m.fetchStackStatusCmd(msg.stack))
		}

	case discoveryErrorMsg:
		m.discoveryErrors = append(m.discoveryErrors, msg.err)
		m.lastError = msg.err

	case discoveryFinishedMsg:
		stateChanged := false
		if m.currentState == stateLoadingStacks {
			m.currentState = stateStackList
			stateChanged = true
			if len(m.stacks) == 0 {
				if len(m.discoveryErrors) == 0 {
					m.lastError = fmt.Errorf("no stacks found")
				} else {
					m.lastError = fmt.Errorf("discovery finished with errors, no stacks found")
				}
			} else {
				m.lastError = nil
				if len(m.discoveryErrors) > 0 {
					m.lastError = fmt.Errorf("discovery finished with errors")
				}
			}
		}
		m.isDiscovering = false

		if stateChanged && m.currentState == stateStackList {
			listContent := strings.Builder{}
			listContent.WriteString("Select a stack:\n")
			for i, stack := range m.stacks {
				cursor := "  "
				if m.cursor == i {
					cursor = cursorStyle.Render("> ")
				}
				stackID := stack.Identifier()
				statusStr := ""
				if m.loadingStatus[stackID] {
					statusStr = statusLoadingStyle.Render(" [loading...]")
				} else if _, ok := m.stackStatuses[stackID]; ok {
					// Status is loaded, determine how to display it later in View()
				} else {
					statusStr = statusLoadingStyle.Render(" [?]")
				}
				listContent.WriteString(fmt.Sprintf("%s%s (%s)%s\n", cursor, stack.Name, serverNameStyle.Render(stack.ServerName), statusStr))
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
		// Ensure cursor stays within bounds (0 to len(hosts)) after loading
		totalItems := len(m.configuredHosts) + 1
		if m.configCursor >= totalItems {
			m.configCursor = max(0, totalItems-1)
		}

	case stackStatusLoadedMsg:
		m.loadingStatus[msg.stackIdentifier] = false
		m.stackStatuses[msg.stackIdentifier] = msg.statusInfo

	case stepFinishedMsg:
		switch m.currentState {
		case stateSshConfigRemoveConfirm:
			if msg.err != nil {
				m.lastError = fmt.Errorf("failed to remove host: %w", msg.err)
				m.currentState = stateSshConfigList
				cmds = append(cmds, loadSshConfigCmd())
			} else {
				m.currentState = stateSshConfigList
				cmds = append(cmds, loadSshConfigCmd())
			}
		case stateRunningSequence:
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
					for _, stack := range m.stacksInSequence {
						if stack != nil {
							stackID := stack.Identifier()
							if !m.loadingStatus[stackID] {
								m.loadingStatus[stackID] = true
								cmds = append(cmds, m.fetchStackStatusCmd(*stack))
							}
						}
					}
				} else {
					cmds = append(cmds, m.startNextStepCmd())
				}
			}
		case stateRunningHostAction:
			// Handle host action completion
			m.outputChan = nil
			m.errorChan = nil
			if msg.err != nil {
				m.hostActionError = msg.err // Store specific host action error
				m.lastError = msg.err       // Also update general lastError for display
				m.currentState = stateSshConfigList
				m.outputContent += errorStyle.Render(fmt.Sprintf("\n--- HOST ACTION '%s' FAILED: %v ---", m.currentHostActionStep.Name, msg.err)) + "\n"
				m.viewport.SetContent(m.outputContent)
				m.viewport.GotoBottom()
				cmds = append(cmds, loadSshConfigCmd()) // Reload config state
			} else {
				// Success
				m.outputContent += successStyle.Render(fmt.Sprintf("\n--- Host Action '%s' Completed Successfully ---", m.currentHostActionStep.Name)) + "\n"
				m.viewport.SetContent(m.outputContent)
				m.viewport.GotoBottom()
				m.currentState = stateSshConfigList // Go back to list on success
				m.hostsToPrune = nil
				m.hostActionError = nil
				m.lastError = nil                       // Clear last error on success
				cmds = append(cmds, loadSshConfigCmd()) // Reload config state
			}
		}

	case channelsAvailableMsg:
		switch m.currentState {
		case stateRunningSequence:
			m.outputChan = msg.outChan
			m.errorChan = msg.errChan
			cmds = append(cmds, waitForOutputCmd(m.outputChan), waitForErrorCmd(m.errorChan))
		case stateRunningHostAction:
			m.outputChan = msg.outChan
			m.errorChan = msg.errChan
			// Use the same waitForOutputCmd, but need a way to distinguish error source if needed
			// For now, stepFinishedMsg handles the final error.
			cmds = append(cmds, waitForOutputCmd(m.outputChan), waitForErrorCmd(m.errorChan))
		}

	case outputLineMsg:
		if (m.currentState == stateRunningSequence || m.currentState == stateRunningHostAction) && m.outputChan != nil {
			// Append the raw line content (which might be a chunk) directly.
			// This preserves original ANSI colors and control characters like \r.
			// The terminal and lipgloss viewport should handle rendering correctly.
			m.outputContent += msg.line.Line // Append the raw chunk/line
			m.viewport.SetContent(m.outputContent)
			m.viewport.GotoBottom()
			cmds = append(cmds, waitForOutputCmd(m.outputChan)) // Continue waiting for output
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

	if m.currentState == stateStackDetails && vpCmd == nil {
		m.detailsViewport, vpCmd = m.detailsViewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) handleSshAddFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd
	focusMap := []int{0, 1, 2, 3, 4, 5} // Logical indices: Name, Host, User, Port, Root, AuthMethod
	switch m.formAuthMethod {
	case authMethodKey:
		focusMap = append(focusMap, 6) // Add KeyPath index
	case authMethodPassword:
		focusMap = append(focusMap, 7) // Add Password index
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
		if m.formFocusIndex == 5 { // Focus is on Auth Method selector
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

	// Update Input Focus Styles
	for i := range m.formInputs {
		m.formInputs[i].Blur()
		m.formInputs[i].Prompt = "  "
		m.formInputs[i].TextStyle = lipgloss.NewStyle()
	}

	focusedInputIndex := -1
	switch m.formFocusIndex {
	case 0, 1, 2, 3, 4: // Name, Hostname, User, Port, RemoteRoot
		focusedInputIndex = m.formFocusIndex
	case 6: // Key Path (only focusable if authMethodKey)
		if m.formAuthMethod == authMethodKey {
			focusedInputIndex = 5
		}
	case 7: // Password (only focusable if authMethodPassword)
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
	focusMap := []int{0, 1, 2, 3, 4, 5} // Logical indices: Name, Host, User, Port, Root, AuthMethod
	switch m.formAuthMethod {
	case authMethodKey:
		focusMap = append(focusMap, 6) // Add KeyPath index
	case authMethodPassword:
		focusMap = append(focusMap, 7) // Add Password index
	}
	focusMap = append(focusMap, 8) // Add Disabled index
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
		if m.formFocusIndex == authMethodLogicalIndex { // Focus is on Auth Method selector
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
		// Prevent submitting when focus is on non-input selectors
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

	// Update Input Focus Styles
	for i := range m.formInputs {
		m.formInputs[i].Blur()
		m.formInputs[i].Prompt = "  "
		m.formInputs[i].TextStyle = lipgloss.NewStyle()
	}

	focusedInputIndex := -1
	switch m.formFocusIndex {
	case 0, 1, 2, 3, 4: // Name, Hostname, User, Port, RemoteRoot
		focusedInputIndex = m.formFocusIndex
	case 6: // Key Path (only focusable if authMethodKey)
		if m.formAuthMethod == authMethodKey {
			focusedInputIndex = 5
		}
	case 7: // Password (only focusable if authMethodPassword)
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

	const (
		remoteRootFocusIndex    = 0 // Logical index for Remote Root
		authMethodFocusIndex    = 1 // Logical index for Auth Method selector
		keyOrPasswordFocusIndex = 2
	)

	authNeeded := false
	if m.configuringHostIdx >= 0 && m.configuringHostIdx < len(m.importableHosts) {
		authNeeded = m.importableHosts[m.configuringHostIdx].KeyPath == ""
	}

	numFocusable := 1 // Fixed Field: Remote Root
	if authNeeded {
		numFocusable++ // Auth Method Selector
		if m.formAuthMethod == authMethodKey || m.formAuthMethod == authMethodPassword {
			numFocusable++ // Key Path or Password Input
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

	// Update Input Focus Styles
	remoteRootInputIdx := 4 // Actual index in m.formInputs
	keyPathInputIdx := 5    // Actual index in m.formInputs
	passwordInputIdx := 6   // Actual index in m.formInputs

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
			switch m.formAuthMethod {
			case authMethodKey:
				cmds = append(cmds, m.formInputs[keyPathInputIdx].Focus())
				m.formInputs[keyPathInputIdx].Prompt = cursorStyle.Render("> ")
				m.formInputs[keyPathInputIdx].TextStyle = cursorStyle
			case authMethodPassword:
				cmds = append(cmds, m.formInputs[passwordInputIdx].Focus())
				m.formInputs[passwordInputIdx].Prompt = cursorStyle.Render("> ")
				m.formInputs[passwordInputIdx].TextStyle = cursorStyle
			}
		}
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

	// Note: Name conflict check is performed in saveNewSshHostCmd

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
		editedHost.Name = originalHost.Name // Keep original if empty
	}

	editedHost.Hostname = strings.TrimSpace(m.formInputs[1].Value())
	if editedHost.Hostname == "" {
		editedHost.Hostname = originalHost.Hostname // Keep original if empty
	}

	editedHost.User = strings.TrimSpace(m.formInputs[2].Value())
	if editedHost.User == "" {
		editedHost.User = originalHost.User // Keep original if empty
	}

	editedHost.RemoteRoot = strings.TrimSpace(m.formInputs[4].Value())
	// Allow empty RemoteRoot to clear it

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
		editedHost.Port = 0 // Default port
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return editedHost, fmt.Errorf("invalid port number: %s", portStr)
		}
		if port == 22 {
			editedHost.Port = 0 // Store 0 for default port 22
		} else {
			editedHost.Port = port
		}
	}

	keyPathInput := strings.TrimSpace(m.formInputs[5].Value())
	passwordInput := m.formInputs[6].Value()

	switch m.formAuthMethod {
	case authMethodKey:
		if keyPathInput == "" {
			// If input is empty, keep original key path if it existed
			if originalHost.KeyPath != "" && originalHost.Password == "" {
				editedHost.KeyPath = originalHost.KeyPath
			} else {
				return editedHost, fmt.Errorf("key path is required for Key File authentication")
			}
		} else {
			editedHost.KeyPath = keyPathInput
		}
		editedHost.Password = "" // Clear password if key is set
	case authMethodPassword:
		if passwordInput == "" {
			// If input is empty, keep original password if it existed and no key was set
			if originalHost.Password != "" && originalHost.KeyPath == "" {
				editedHost.Password = originalHost.Password
			} else {
				return editedHost, fmt.Errorf("password is required for Password authentication")
			}
		} else {
			editedHost.Password = passwordInput
		}
		editedHost.KeyPath = "" // Clear key path if password is set
	case authMethodAgent:
		editedHost.KeyPath = ""
		editedHost.Password = ""
	default:
		return editedHost, fmt.Errorf("invalid authentication method selected")
	}

	// Note: Name conflict check is performed in saveEditedSshHostCmd

	editedHost.Disabled = m.formDisabled

	return editedHost, nil
}

func (m *model) handleStackListKeys(msg tea.KeyMsg) []tea.Cmd {
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
		if m.cursor < len(m.stacks)-1 {
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
		lastIdx := len(m.stacks) - 1
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
		m.viewport.PageUp()
	case key.Matches(msg, m.keymap.PgDown):
		m.cursor += m.viewport.Height
		lastIdx := len(m.stacks) - 1
		if lastIdx >= 0 && m.cursor > lastIdx {
			m.cursor = lastIdx
		}
		cursorMoved = true
		m.viewport.PageDown()
	default:
		switch {
		case key.Matches(msg, m.keymap.Select):
			if len(m.stacks) > 0 && m.cursor < len(m.stacks) {
				if _, ok := m.selectedStackIdxs[m.cursor]; ok {
					delete(m.selectedStackIdxs, m.cursor)
				} else {
					m.selectedStackIdxs[m.cursor] = struct{}{}
				}
			}
		case key.Matches(msg, m.keymap.UpAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.UpSequence)...)
		case key.Matches(msg, m.keymap.DownAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.DownSequence)...)
		case key.Matches(msg, m.keymap.RefreshAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.RefreshSequence)...)
		case key.Matches(msg, m.keymap.PullAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.PullSequence)...)
		case key.Matches(msg, m.keymap.Enter):
			if len(m.selectedStackIdxs) > 0 {
				m.stacksInSequence = []*discovery.Stack{}
				for idx := range m.selectedStackIdxs {
					if idx >= 0 && idx < len(m.stacks) {
						stack := m.stacks[idx]
						m.stacksInSequence = append(m.stacksInSequence, &stack)
						stackID := stack.Identifier()
						if _, loaded := m.stackStatuses[stackID]; !loaded && !m.loadingStatus[stackID] {
							m.loadingStatus[stackID] = true
							cmds = append(cmds, m.fetchStackStatusCmd(stack))
						}
					}
				}
				m.detailedStack = nil
				m.selectedStackIdxs = make(map[int]struct{})
				m.currentState = stateStackDetails
			} else if len(m.stacks) > 0 && m.cursor < len(m.stacks) {
				m.detailedStack = &m.stacks[m.cursor]
				m.stacksInSequence = nil
				m.currentState = stateStackDetails
				stackID := m.detailedStack.Identifier()
				if !m.loadingStatus[stackID] {
					m.loadingStatus[stackID] = true
					cmds = append(cmds, m.fetchStackStatusCmd(*m.detailedStack))
				}
			}
		}
	}

	if cursorMoved && len(m.stacks) > 0 {
		selectedStack := m.stacks[m.cursor]
		stackID := selectedStack.Identifier()
		if _, loaded := m.stackStatuses[stackID]; !loaded && !m.loadingStatus[stackID] {
			m.loadingStatus[stackID] = true
			cmds = append(cmds, m.fetchStackStatusCmd(selectedStack))
		}
	}

	return cmds
}

func (m *model) runSequenceOnSelection(sequenceFunc func(discovery.Stack) []runner.CommandStep) []tea.Cmd {
	var cmds []tea.Cmd
	var stacksToRun []*discovery.Stack
	var combinedSequence []runner.CommandStep
	m.stacksInSequence = nil

	if len(m.selectedStackIdxs) > 0 {
		for idx := range m.selectedStackIdxs {
			if idx >= 0 && idx < len(m.stacks) {
				stacksToRun = append(stacksToRun, &m.stacks[idx])
			}
		}
		m.selectedStackIdxs = make(map[int]struct{})
	} else if len(m.stacks) > 0 && m.cursor < len(m.stacks) {
		stacksToRun = append(stacksToRun, &m.stacks[m.cursor])
	}

	if len(stacksToRun) == 0 {
		return cmds
	}

	m.stacksInSequence = stacksToRun
	for _, stackPtr := range stacksToRun {
		if stackPtr != nil {
			combinedSequence = slices.Concat(combinedSequence, sequenceFunc(*stackPtr))
		}
	}

	if len(combinedSequence) > 0 {
		if len(stacksToRun) > 0 && stacksToRun[0] != nil {
			m.sequenceStack = stacksToRun[0] // Use the first selected stack for display purposes
		} else {
			m.sequenceStack = nil
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
	m.outputContent += stepStyle.Render(fmt.Sprintf("\n--- Starting Step: %s for %s ---", step.Name, step.Stack.Identifier())) + "\n"
	m.viewport.SetContent(m.outputContent)
	m.viewport.GotoBottom()
	return runStepCmd(step)
}

func (m *model) renderStackStatus(b *strings.Builder, stackID string) {
	statusStr := ""
	statusInfo, loaded := m.stackStatuses[stackID]
	isLoading := m.loadingStatus[stackID]

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
	fmt.Fprintf(b, "\nOverall Status:%s\n", statusStr)
	if !isLoading && loaded && statusInfo.Error != nil {
		// Pass the rendered string as an argument to avoid non-constant format string error
		fmt.Fprintf(b, "%s", errorStyle.Render(fmt.Sprintf("  Error fetching status: %v\n", statusInfo.Error)))
	}
	if !isLoading && loaded && len(statusInfo.Containers) > 0 {
		b.WriteString("\nContainers:\n")
		fmt.Fprintf(b, "  %-20s %-30s %s\n", "SERVICE", "CONTAINER NAME", "STATUS")
		fmt.Fprintf(b, "  %-20s %-30s %s\n", "-------", "--------------", "------")
		for _, c := range statusInfo.Containers {
			isUp := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
			statusRenderFunc := statusDownStyle.Render
			if isUp {
				statusRenderFunc = statusUpStyle.Render
			}
			fmt.Fprintf(b, "  %-20s %-30s %s\n", c.Service, c.Name, statusRenderFunc(c.Status))
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
	header = titleStyle.Render("Bucket Manager") + "\n"
	bodyContent := strings.Builder{}

	footerContent := strings.Builder{}
	footerContent.WriteString("\n")

	switch m.currentState {
	case stateStackList:
		if m.isDiscovering {
			footerContent.WriteString(statusLoadingStyle.Render("Discovering remote stacks...") + "\n")
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
		if len(m.selectedStackIdxs) > 0 {
			help.WriteString(fmt.Sprintf("(%d selected) ", len(m.selectedStackIdxs)))
		}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + ": navigate | ")
		help.WriteString(m.keymap.Select.Help().Key + ": " + m.keymap.Select.Help().Desc + " | ")
		help.WriteString(m.keymap.Enter.Help().Key + ": details | ")
		help.WriteString(m.keymap.UpAction.Help().Key + ": up | ")
		help.WriteString(m.keymap.DownAction.Help().Key + ": down | ")
		help.WriteString(m.keymap.RefreshAction.Help().Key + ": refresh | ")
		help.WriteString(m.keymap.PullAction.Help().Key + ": pull")
		help.WriteString(" | ")
		help.WriteString(m.keymap.Config.Help().Key + ": " + m.keymap.Config.Help().Desc + " | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateRunningSequence:
		stackIdentifier := ""
		if m.sequenceStack != nil {
			stackIdentifier = fmt.Sprintf(" for %s", m.sequenceStack.Identifier())
		}
		if m.currentSequence != nil && m.currentStepIndex < len(m.currentSequence) {
			footerContent.WriteString(statusStyle.Render(fmt.Sprintf("Running step %d/%d%s: %s...", m.currentStepIndex+1, len(m.currentSequence), stackIdentifier, m.currentSequence[m.currentStepIndex].Name)))
		} else if m.sequenceStack != nil {
			footerContent.WriteString(successStyle.Render(fmt.Sprintf("Sequence finished successfully%s.", stackIdentifier)))
		} else {
			footerContent.WriteString(successStyle.Render("Sequence finished successfully."))
		}
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + "/" + m.keymap.PgUp.Help().Key + "/" + m.keymap.PgDown.Help().Key + ": scroll | ")
		help.WriteString(m.keymap.Back.Help().Key + "/" + m.keymap.Enter.Help().Key + ": back to list | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateSequenceError:
		stackIdentifier := ""
		if m.sequenceStack != nil {
			stackIdentifier = fmt.Sprintf(" for %s", m.sequenceStack.Identifier())
		}
		if m.lastError != nil {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Error%s: %v", stackIdentifier, m.lastError)))
		} else {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("An unknown error occurred%s.", stackIdentifier)))
		}
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + "/" + m.keymap.PgUp.Help().Key + "/" + m.keymap.PgDown.Help().Key + ": scroll | ")
		help.WriteString(m.keymap.Back.Help().Key + "/" + m.keymap.Enter.Help().Key + ": back to list | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateStackDetails:
		help := strings.Builder{}
		help.WriteString(m.keymap.Back.Help().Key + ": back to list | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))

	case stateSshConfigList:
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + ": navigate | ")
		// Show actions based on selection
		if m.configCursor == 0 { // "local" selected
			// Local only allows Prune
			help.WriteString(m.keymap.PruneAction.Help().Key + ": prune | ")
		} else { // Remote host selected
			// Remote allows Edit, Remove, Prune
			help.WriteString(m.keymap.Edit.Help().Key + ": edit | ")
			help.WriteString(m.keymap.Remove.Help().Key + ": remove | ")
			help.WriteString(m.keymap.PruneAction.Help().Key + ": prune | ")
		}
		// Add and Import are always available regardless of selection
		help.WriteString(m.keymap.Add.Help().Key + ": add | ")
		help.WriteString(m.keymap.Import.Help().Key + ": import | ")
		help.WriteString(m.keymap.Back.Help().Key + ": back | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)

		errorOrInfo := ""
		if m.hostActionError != nil { // Display host action error if present
			errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Prune Error: %v", m.hostActionError))
		} else if m.importInfoMsg != "" {
			errorOrInfo = "\n" + successStyle.Render(m.importInfoMsg)
		} else if m.importError != nil {
			errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Import Error: %v", m.importError))
		} else if m.lastError != nil { // Display general errors if no specific import/prune error
			errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.lastError))
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

	case statePruneConfirm:
		help := strings.Builder{}
		if len(m.hostsToPrune) > 0 {
			targetName := m.hostsToPrune[0].ServerName // TUI currently only prunes one host
			help.WriteString(fmt.Sprintf("Confirm prune action for host '%s'? ", identifierColor.Render(targetName)))
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
	case stateLoadingStacks:
		bodyContent.WriteString(statusStyle.Render("Loading stacks..."))
	case stateStackList:
		listContent := strings.Builder{}
		listContent.WriteString("Select a stack:\n")
		for i, stack := range m.stacks {
			cursor := "  "
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
			}

			checkbox := "[ ]"
			if _, selected := m.selectedStackIdxs[i]; selected {
				checkbox = successStyle.Render("[x]")
			}

			stackID := stack.Identifier()
			statusStr := ""
			if m.loadingStatus[stackID] {
				statusStr = statusLoadingStyle.Render(" [loading...]")
			} else if statusInfo, ok := m.stackStatuses[stackID]; ok {
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
			listContent.WriteString(fmt.Sprintf("%s%s %s (%s)%s\n", cursor, checkbox, stack.Name, serverNameStyle.Render(stack.ServerName), statusStr))
		}
		m.viewport.Height = availableHeight
		m.viewport.SetContent(listContent.String())
		bodyStr = m.viewport.View()
	case stateRunningSequence, stateSequenceError:
		m.viewport.Height = availableHeight
		bodyStr = m.viewport.View()
	case stateStackDetails:
		if m.detailedStack != nil {
			stack := m.detailedStack
			stackID := stack.Identifier()
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for: %s (%s)", stack.Name, serverNameStyle.Render(stack.ServerName))) + "\n\n")
			m.renderStackStatus(&bodyContent, stackID)
		} else if len(m.stacksInSequence) > 0 {
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for %d Selected Stacks:", len(m.stacksInSequence))) + "\n")
			for i, stack := range m.stacksInSequence {
				if stack == nil {
					continue
				}
				stackID := stack.Identifier()
				bodyContent.WriteString(fmt.Sprintf("\n--- %s (%s) ---", stack.Name, serverNameStyle.Render(stack.ServerName)))
				m.renderStackStatus(&bodyContent, stackID)
				if i < len(m.stacksInSequence)-1 {
					bodyContent.WriteString("\n")
				}
			}
		} else {
			bodyContent.WriteString(errorStyle.Render("Error: No stack selected for details."))
		}
		m.detailsViewport.Height = availableHeight
		m.detailsViewport.SetContent(bodyContent.String())
		bodyStr = m.detailsViewport.View()
	case stateSshConfigList:
		bodyContent.WriteString("Configured Hosts:\n\n")

		// Display "local" entry first
		localCursor := "  "
		if m.configCursor == 0 {
			localCursor = cursorStyle.Render("> ")
		}
		bodyContent.WriteString(fmt.Sprintf("%s%s (%s)\n", localCursor, "local", serverNameStyle.Render("Local Docker/Podman")))

		// Display configured remote hosts
		if len(m.configuredHosts) == 0 {
			bodyContent.WriteString("\n  (No remote SSH hosts configured yet)")
		} else {
			for i, host := range m.configuredHosts {
				cursor := "  "
				// Adjust cursor check for remote hosts (index starts from 1 in the view)
				if m.configCursor == i+1 {
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
		// Display general errors at the bottom if they exist and aren't specific import/prune errors handled in footer
		if m.lastError != nil && m.importError == nil && m.hostActionError == nil {
			bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.lastError)))
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

	case statePruneConfirm:
		if len(m.hostsToPrune) > 0 {
			targetName := m.hostsToPrune[0].ServerName
			bodyContent.WriteString(fmt.Sprintf("Are you sure you want to run 'podman system prune -af' on host '%s'?\n\n", identifierColor.Render(targetName)))
			bodyContent.WriteString("This will remove all unused containers, networks, images, and build cache.\n\n")
			bodyContent.WriteString("[y] Yes, prune | [n/Esc/b] No, cancel")
		} else {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for prune. Press Esc/b to go back."))
		}
		bodyStr = lipgloss.PlaceVertical(availableHeight, lipgloss.Top, bodyContent.String())

	case stateRunningHostAction:
		// Display output similar to stateRunningSequence
		m.viewport.Height = availableHeight
		bodyStr = m.viewport.View() // Viewport already contains output content

		// --- Explicitly build footer for this state ---
		runningFooter := strings.Builder{}
		targetName := "unknown host"
		if len(m.hostsToPrune) > 0 {
			targetName = m.hostsToPrune[0].ServerName
		}
		// Add the status line
		runningFooter.WriteString("\n" + statusStyle.Render(fmt.Sprintf("Running prune action on '%s'...", identifierColor.Render(targetName))))

		// Add basic help line
		help := strings.Builder{}
		help.WriteString(m.keymap.Up.Help().Key + "/" + m.keymap.Down.Help().Key + "/" + m.keymap.PgUp.Help().Key + "/" + m.keymap.PgDown.Help().Key + ": scroll | ")
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		runningFooter.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String()))

		// --- Assign directly to footerStr, overriding the default footer building ---
		footerStr = runningFooter.String()

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
		case authMethodKey:
			authMethodStr = "SSH Key File"
		case authMethodAgent:
			authMethodStr = "SSH Agent"
		case authMethodPassword:
			authMethodStr = "Password (insecure)"
		}
		helpText := "[←/→ to change]"
		bodyContent.WriteString(fmt.Sprintf("%s%s\n", authFocus, authStyle.Render("Auth Method: "+authMethodStr+" "+helpText)))
		switch m.formAuthMethod {
		case authMethodKey:
			bodyContent.WriteString(m.formInputs[5].View() + "\n")
		case authMethodPassword:
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
				if m.importCursor == i {
					cursor = cursorStyle.Render("> ")
				}
				checkbox := "[ ]"
				if _, selected := m.selectedImportIdxs[i]; selected {
					checkbox = successStyle.Render("[x]")
				}
				details := fmt.Sprintf("%s@%s", pHost.User, pHost.Hostname)
				if pHost.Port != 0 && pHost.Port != 22 {
					details += fmt.Sprintf(":%d", pHost.Port)
				}
				keyInfo := ""
				if pHost.KeyPath != "" {
					keyInfo = fmt.Sprintf(" (Key: %s)", lipgloss.NewStyle().Faint(true).Render(filepath.Base(pHost.KeyPath)))
				}
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
				if m.formFocusIndex == 1 {
					authFocus = cursorStyle.Render("> ")
					authStyle = cursorStyle
				}
				authMethodStr := ""
				switch m.formAuthMethod {
				case authMethodKey:
					authMethodStr = "SSH Key File"
				case authMethodAgent:
					authMethodStr = "SSH Agent"
				case authMethodPassword:
					authMethodStr = "Password (insecure)"
				}
				helpText := "[←/→ to change]"
				bodyContent.WriteString(fmt.Sprintf("%s%s\n", authFocus, authStyle.Render("Auth Method: "+authMethodStr+" "+helpText)))

				// Render Key Path or Password input *after* Auth Method
				if m.formAuthMethod == authMethodKey {
					bodyContent.WriteString(m.formInputs[5].View() + "\n")
				}
				if m.formAuthMethod == authMethodPassword {
					bodyContent.WriteString(m.formInputs[6].View() + "\n")
				}
			} else {
				bodyContent.WriteString(fmt.Sprintf("  Auth Method: SSH Key File (from ssh_config: %s)\n", lipgloss.NewStyle().Faint(true).Render(pHost.KeyPath)))
			}
			if m.formError != nil {
				bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
			}
		}
		m.formViewport.Height = availableHeight
		m.formViewport.SetContent(bodyContent.String())
		bodyStr = m.formViewport.View()
	case stateSshConfigEditForm:
		if m.hostToEdit == nil {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for editing."))
		} else {
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Edit SSH Host: %s", identifierColor.Render(m.hostToEdit.Name))) + "\n\n")
			for i := range 5 {
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
			case authMethodKey:
				authMethodStr = "SSH Key File"
			case authMethodAgent:
				authMethodStr = "SSH Agent"
			case authMethodPassword:
				authMethodStr = "Password (insecure)"
			}
			helpText := "[←/→ to change]"
			bodyContent.WriteString(fmt.Sprintf("%s%s\n", authFocus, authStyle.Render("Auth Method: "+authMethodStr+" "+helpText)))
			if m.formAuthMethod == authMethodKey {
				bodyContent.WriteString(m.formInputs[5].View() + "\n")
			}
			if m.formAuthMethod == authMethodPassword {
				bodyContent.WriteString(m.formInputs[6].View() + "\n")
			}
			disabledFocus := "  "
			disabledStyle := lipgloss.NewStyle()
			if m.formFocusIndex == 8 {
				disabledFocus = cursorStyle.Render("> ")
				disabledStyle = cursorStyle
			}
			checkbox := "[ ]"
			if m.formDisabled {
				checkbox = successStyle.Render("[x]")
			}
			bodyContent.WriteString(fmt.Sprintf("%s%s\n", disabledFocus, disabledStyle.Render(checkbox+" Disabled Host [space to toggle]")))
			if m.formError != nil {
				bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
			}
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
