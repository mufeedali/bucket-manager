// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BubbleProgram is a package-level variable to hold the program instance.
// This is needed so background goroutines can send messages.
// It's assigned in cmd/tui/main.go before the program runs.
var BubbleProgram *tea.Program

// Define styles
var (
	titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))   // Purple
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))               // Red
	statusStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))              // Blue
	stepStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))              // Yellow
	successStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))              // Green
	cursorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))               // Magenta
	statusUpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))              // Green
	statusDownStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))               // Red
	statusPartialStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))              // Yellow
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))             // Orange/Brown for status error
	statusLoadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))               // Grey
	serverNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Italic(true) // Blue Italic for server name
	identifierColor    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))               // Cyan for identifiers like hostnames
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
	stateSshConfigImportSelect  // State for selecting hosts to import (skipping form for now)
	stateSshConfigImportDetails // State for providing details for selected hosts
	stateSshConfigEditForm      // State for editing an existing host
)

// Constants for auth methods
const (
	authMethodKey = iota + 1
	authMethodAgent
	authMethodPassword
)

// KeyMap defines the application's keybindings.
type KeyMap struct {
	// General
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PgUp     key.Binding
	PgDown   key.Binding
	Home     key.Binding
	End      key.Binding
	Help     key.Binding // Might use later for a dedicated help view
	Quit     key.Binding
	Enter    key.Binding
	Esc      key.Binding
	Back     key.Binding // Often same as Esc
	Select   key.Binding // e.g., Spacebar
	Tab      key.Binding
	ShiftTab key.Binding
	Yes      key.Binding
	No       key.Binding

	// Project List Specific
	Config        key.Binding
	UpAction      key.Binding
	DownAction    key.Binding
	RefreshAction key.Binding

	// SSH Config List Specific
	Remove key.Binding
	Add    key.Binding
	Import key.Binding
	Edit   key.Binding

	// Form Specific (Add/Edit/Import Details)
	ToggleDisabled key.Binding // Spacebar in Edit form
}

// DefaultKeyMap defines the default keybindings.
var DefaultKeyMap = KeyMap{
	// General
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
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
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
	Back: key.NewBinding( // Often same as Esc, but can be context-specific
		key.WithKeys("esc", "b"), // Add 'b' for back consistency
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

	// Project List Specific
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

	// SSH Config List Specific
	Remove: key.NewBinding(
		key.WithKeys("delete", "backspace"), // More standard delete keys
		key.WithHelp("del/bksp", "remove host"),
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

	// Form Specific
	ToggleDisabled: key.NewBinding(
		key.WithKeys(" "), // Spacebar used for this toggle in edit form
		key.WithHelp("space", "toggle disabled"),
	),
}

type model struct {
	keymap              KeyMap
	projects            []discovery.Project
	cursor              int              // project list cursor
	selectedProjectIdxs map[int]struct{} // Indices of selected projects
	configCursor        int              // config list cursor
	hostToRemove        *config.SSHHost  // For remove confirmation
	hostToEdit          *config.SSHHost  // For edit form
	configuredHosts     []config.SSHHost
	viewport            viewport.Model
	currentState        state
	isDiscovering       bool // Flag to track discovery process
	currentSequence     []runner.CommandStep
	currentStepIndex    int
	outputContent       string
	lastError           error
	discoveryErrors     []error // Store multiple discovery errors
	ready               bool
	width               int
	height              int
	outputChan          <-chan runner.OutputLine
	errorChan           <-chan error
	projectStatuses     map[string]runner.ProjectRuntimeInfo
	loadingStatus       map[string]bool
	detailedProject     *discovery.Project
	sequenceProject     *discovery.Project   // Tracks the *first* project for display during sequence
	projectsInSequence  []*discovery.Project // Tracks *all* projects in the current sequence run

	// SSH Config Add Form State
	formInputs     []textinput.Model
	formFocusIndex int
	formAuthMethod int  // 1=Key, 2=Agent, 3=Password
	formDisabled   bool // For edit form disabled toggle
	formError      error

	// SSH Config Import State
	importableHosts    []config.PotentialHost // Hosts parsed from file, filtered for conflicts
	selectedImportIdxs map[int]struct{}       // Indices of importableHosts selected by user
	importCursor       int                    // Cursor for import selection list
	importError        error                     // Errors during import process (parsing, saving)
	importInfoMsg      string                    // Informational messages from import (e.g., success, skipped)
	hostsToConfigure   []config.SSHHost          // Hosts built after gathering details, ready to save
	configuringHostIdx int                       // Index of the host currently being configured in Details state
}

// --- Messages ---

// projectDiscoveredMsg is sent when a single project is found.
type projectDiscoveredMsg struct {
	project discovery.Project
}

// discoveryErrorMsg is sent when an error occurs during discovery (local or remote).
type discoveryErrorMsg struct {
	err error
}

// discoveryFinishedMsg is sent when all discovery goroutines have completed.
type discoveryFinishedMsg struct{}

type sshConfigLoadedMsg struct {
	hosts []config.SSHHost
	Err   error
}

type sshHostAddedMsg struct { // Used by Add form
	err error
}

type sshHostEditedMsg struct { // Used by Edit form
	err error
}

type sshConfigParsedMsg struct { // Used by Import: Step 1 result
	potentialHosts []config.PotentialHost
	err            error
}

type sshHostsImportedMsg struct { // Used by Import: Final result
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

// --- Commands ---

// findProjectsCmd now launches goroutines to read from discovery channels
// and send incremental messages back to the Update loop via ui.BubbleProgram.
func findProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		// This command now only *starts* the discovery process.
		// It returns nil immediately, and the results will come via messages
		// sent from the goroutines below.
		projectChan, errorChan, doneChan := discovery.FindProjects()

		// Goroutine to read projects and send messages
		go func() {
			for p := range projectChan {
				if BubbleProgram != nil {
					BubbleProgram.Send(projectDiscoveredMsg{project: p})
				}
			}
		}()

		// Goroutine to read errors and send messages
		go func() {
			for e := range errorChan {
				if BubbleProgram != nil {
					BubbleProgram.Send(discoveryErrorMsg{err: e})
				}
			}
		}()

		// Goroutine to wait for discovery to finish (via doneChan) and send final message
		go func() {
			<-doneChan // Wait for the discovery process to close doneChan
			if BubbleProgram != nil {
				BubbleProgram.Send(discoveryFinishedMsg{}) // Signal completion
			}
		}()

		return nil // Command returns immediately, messages follow
	}
}

// listenForProjectsCmd returns a command that waits for a project from the channel
// and returns a projectDiscoveredMsg. It returns nil if the channel closes.
func listenForProjectsCmd(projectChan <-chan discovery.Project) tea.Cmd {
	return func() tea.Msg {
		project, ok := <-projectChan
		if !ok {
			return nil
		}
		return projectDiscoveredMsg{project: project}
	}
}

// listenForDiscoveryErrorsCmd returns a command that waits for an error from the channel
// and returns a discoveryErrorMsg. It returns nil if the channel closes.
func listenForDiscoveryErrorsCmd(errorChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-errorChan
		if !ok {
			return nil
		}
		return discoveryErrorMsg{err: err}
	}
}

// listenForDiscoveryDoneCmd returns a command that waits for the done channel to close
// and returns a discoveryFinishedMsg.
func listenForDiscoveryDoneCmd(doneChan <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-doneChan // Wait until channel is closed
		return discoveryFinishedMsg{}
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

// saveEditedSshHostCmd attempts to save changes to an existing SSH host configuration.
func saveEditedSshHostCmd(originalName string, editedHost config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostEditedMsg{fmt.Errorf("failed to load config before saving edit: %w", err)}
		}

		found := false
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == originalName {
				// Check for name conflict if the name was changed
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

// saveNewSshHostCmd attempts to save a new SSH host configuration.
func saveNewSshHostCmd(newHost config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostAddedMsg{fmt.Errorf("failed to load config before saving: %w", err)}
		}

		// Final validation check for name conflict just before saving
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

// parseSshConfigCmd attempts to parse hosts from the default SSH config file path.
func parseSshConfigCmd() tea.Cmd {
	return func() tea.Msg {
		// config.ParseSSHConfig already handles default path (~/.ssh/config)
		potentialHosts, err := config.ParseSSHConfig()
		return sshConfigParsedMsg{potentialHosts: potentialHosts, err: err}
	}
}

// saveImportedSshHostsCmd attempts to save multiple new SSH hosts to the config.
func saveImportedSshHostsCmd(hostsToSave []config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		if len(hostsToSave) == 0 {
			return sshHostsImportedMsg{importedCount: 0, err: nil}
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostsImportedMsg{err: fmt.Errorf("failed to load config before saving imports: %w", err)}
		}

		// Check for name conflicts again just before saving (defensive check)
		currentNames := make(map[string]bool)
		for _, h := range cfg.SSHHosts {
			currentNames[h.Name] = true
		}

		finalHostsToAdd := []config.SSHHost{}
		skippedCount := 0
		for _, newHost := range hostsToSave {
			if _, exists := currentNames[newHost.Name]; exists {
				// This shouldn't happen if filtering worked, but handle anyway
				skippedCount++
				continue
			}
			finalHostsToAdd = append(finalHostsToAdd, newHost)
			currentNames[newHost.Name] = true
		}

		if len(finalHostsToAdd) == 0 && skippedCount > 0 {
			return sshHostsImportedMsg{err: fmt.Errorf("all selected hosts conflicted with existing names")}
		}

		cfg.SSHHosts = append(cfg.SSHHosts, finalHostsToAdd...)
		err = config.SaveConfig(cfg)
		if err != nil {
			return sshHostsImportedMsg{err: fmt.Errorf("failed to save config after import: %w", err)}
		}

		errMsg := ""
		if skippedCount > 0 {
			errMsg = fmt.Sprintf(" (skipped %d due to conflicts)", skippedCount)
		}

		// Return success, potentially with a note about skipped hosts
		return sshHostsImportedMsg{
			importedCount: len(finalHostsToAdd),
			err:           fmt.Errorf("import failed: %s", errMsg), // Use error field for non-fatal info
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

// --- Model Implementation ---

func InitialModel() model {
	m := model{
		keymap:              DefaultKeyMap,
		currentState:        stateLoadingProjects,
		isDiscovering:       true,
		cursor:              0,
		selectedProjectIdxs: make(map[int]struct{}),
		configCursor:        0,
		projectStatuses:     make(map[string]runner.ProjectRuntimeInfo),
		loadingStatus:       make(map[string]bool),
		configuredHosts:     []config.SSHHost{},
		discoveryErrors:     []error{},
		detailedProject:     nil,
		sequenceProject:     nil,
		projectsInSequence:  nil,
	}
	return m
}

// createAddForm initializes the text input fields for the add SSH host form.
func createAddForm() []textinput.Model {
	inputs := make([]textinput.Model, 7) // Name, Hostname, User, Port, RemoteRoot, KeyPath, Password
	var t textinput.Model

	// Name
	t = textinput.New()
	t.Placeholder = "Unique Name (e.g., server1)"
	t.Focus()
	t.CharLimit = 50
	t.Width = 40
	inputs[0] = t

	// Hostname
	t = textinput.New()
	t.Placeholder = "Hostname or IP Address"
	t.CharLimit = 100
	t.Width = 40
	inputs[1] = t

	// User
	t = textinput.New()
	t.Placeholder = "SSH Username"
	t.CharLimit = 50
	t.Width = 40
	inputs[2] = t

	// Port
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

	// Remote Root
	t = textinput.New()
	t.Placeholder = "Remote Root Path (e.g., /home/user/projects)"
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	// Key Path (initially hidden, shown based on auth method)
	t = textinput.New()
	t.Placeholder = "Path to Private Key (e.g., ~/.ssh/id_rsa)"
	t.CharLimit = 200
	t.Width = 60
	inputs[5] = t

	// Password (masked, initially hidden)
	t = textinput.New()
	t.Placeholder = "Password (stored insecurely!)"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	return inputs
}

// createEditForm initializes the text input fields for the edit SSH host form, pre-filling values.
func createEditForm(host config.SSHHost) ([]textinput.Model, int, bool) {
	inputs := make([]textinput.Model, 7) // Name, Hostname, User, Port, RemoteRoot, KeyPath, Password
	var t textinput.Model
	initialAuthMethod := authMethodAgent // Default to agent
	if host.KeyPath != "" {
		initialAuthMethod = authMethodKey
	} else if host.Password != "" {
		initialAuthMethod = authMethodPassword
	}

	// Name
	t = textinput.New()
	t.Placeholder = "Unique Name"
	t.SetValue(host.Name)
	t.Focus()
	t.CharLimit = 50
	t.Width = 40
	inputs[0] = t

	// Hostname
	t = textinput.New()
	t.Placeholder = "Hostname or IP Address"
	t.SetValue(host.Hostname)
	t.CharLimit = 100
	t.Width = 40
	inputs[1] = t

	// User
	t = textinput.New()
	t.Placeholder = "SSH Username"
	t.SetValue(host.User)
	t.CharLimit = 50
	t.Width = 40
	inputs[2] = t

	// Port
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

	// Remote Root
	t = textinput.New()
	t.Placeholder = "Remote Root Path"
	t.SetValue(host.RemoteRoot)
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	// Key Path
	t = textinput.New()
	t.Placeholder = "Path to Private Key"
	t.SetValue(host.KeyPath)
	t.CharLimit = 200
	t.Width = 60
	inputs[5] = t

	// Password
	t = textinput.New()
	t.Placeholder = "Password (leave blank to keep current)"
	// Do not set value for password, user must re-enter if changing method or password
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	return inputs, initialAuthMethod, host.Disabled
}

// createImportDetailsForm initializes the text input fields for the import details form.
// It pre-fills some fields and determines which auth fields are needed.
func createImportDetailsForm(pHost config.PotentialHost) ([]textinput.Model, int) {
	// We only need RemoteRoot, and potentially KeyPath/Password if not in pHost.
	// Let's reuse the indices from createAddForm for consistency:
	// 4: RemoteRoot
	// 5: KeyPath
	// 6: Password
	inputs := make([]textinput.Model, 7) // Keep size consistent, but only use needed ones
	var t textinput.Model
	initialAuthMethod := authMethodAgent // Default to agent

	// Remote Root (Index 4)
	t = textinput.New()
	t.Placeholder = "Remote Root Path (e.g., /home/user/projects)"
	t.Focus()
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	// Key Path (Index 5) - Needed if pHost has it or if user selects Key auth
	t = textinput.New()
	t.Placeholder = "Path to Private Key"
	t.CharLimit = 200
	t.Width = 60
	if pHost.KeyPath != "" {
		t.SetValue(pHost.KeyPath)
		initialAuthMethod = authMethodKey
	}
	inputs[5] = t

	// Password (Index 6) - Needed if user selects Password auth
	t = textinput.New()
	t.Placeholder = "Password (stored insecurely!)"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	// Initialize unused inputs to prevent nil panics, though they won't be shown/used directly
	for i := 0; i < 4; i++ {
		inputs[i] = textinput.New()
	}

	return inputs, initialAuthMethod
}

// updateInputs updates the focused text input component for the Add/Edit/ImportDetails forms
func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	if (m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm) && m.formInputs != nil {
		var cmd tea.Cmd
		actualInputIndex := -1
		isTextInputFocused := false

		switch m.formFocusIndex {
		case 0, 1, 2, 3, 4: // Name, Hostname, User, Port, RemoteRoot (Add/Edit)
			actualInputIndex = m.formFocusIndex
			isTextInputFocused = true
		case 5: // Could be Auth Method (Add/Edit), KeyPath (Edit), or Password (Edit) depending on context
			if m.currentState == stateSshConfigAddForm {
				// In Add form, index 5 is Auth Method if Agent/Password, or KeyPath if Key
				if m.formAuthMethod == authMethodKey {
					actualInputIndex = 5 // KeyPath
					isTextInputFocused = true
				}
				// Otherwise, it's Auth Method (handled by navigation, not input update)
			} else { // Edit form
				// In Edit form, index 5 is Auth Method
				// Index 6 is KeyPath (if visible)
				// Index 7 is Password (if visible)
				// Index 8 is Disabled toggle
				// Input update is handled below based on visibility
			}
		case 6: // Could be KeyPath (Add), Password (Add), Auth Method (Edit), Disabled (Edit)
			if m.currentState == stateSshConfigAddForm {
				// In Add form, index 6 is Password if Password auth, or Auth Method if Key auth
				if m.formAuthMethod == authMethodPassword {
					actualInputIndex = 6 // Password
					isTextInputFocused = true
				}
				// Otherwise, it's Auth Method (handled by navigation)
			} else { // Edit form
				// Index 6 is KeyPath if visible
				if m.formAuthMethod == authMethodKey {
					actualInputIndex = 5 // KeyPath maps to input index 5
					isTextInputFocused = true
				}
			}
		case 7: // Could be Password (Edit)
			if m.currentState == stateSshConfigEditForm {
				if m.formAuthMethod == authMethodPassword {
					actualInputIndex = 6 // Password maps to input index 6
					isTextInputFocused = true
				}
			}
			// case 8: // Disabled toggle (Edit form) - not a text input
		}

		if isTextInputFocused && actualInputIndex >= 0 && actualInputIndex < len(m.formInputs) {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				k := keyMsg.String()
				isSpaceAllowed := k != " " || !(m.currentState == stateSshConfigEditForm && m.formFocusIndex == m.getDisabledToggleFocusIndex())

				if k != "tab" && k != "shift+tab" && k != "enter" && k != "esc" && isSpaceAllowed {
					t := m.formInputs[actualInputIndex]
					m.formInputs[actualInputIndex], cmd = t.Update(msg)
					cmds = append(cmds, cmd)
				}
			}
		}
	} else if m.currentState == stateSshConfigImportDetails && m.formInputs != nil {
		// Logic for Import Details form
		var cmd tea.Cmd
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			k := keyMsg.String()
			// Allow input only for the visible fields in the import details form
			// Indices: 4 (RemoteRoot), 5 (KeyPath), 6 (Password)
			isFocusableField := false
			if m.formFocusIndex == 0 { // Remote Root is always focusable (relative index 0)
				isFocusableField = true
			} else if m.formAuthMethod == authMethodKey && m.formFocusIndex == 1 { // KeyPath (relative index 1)
				isFocusableField = true
			} else if m.formAuthMethod == authMethodPassword && m.formFocusIndex == 1 { // Password (relative index 1)
				isFocusableField = true
			}

			// Map relative formFocusIndex (0, 1) to actual m.formInputs index (4, 5, or 6)
			actualInputIndex := -1
			if m.formFocusIndex == 0 {
				actualInputIndex = 4 // Remote Root
			} else if m.formFocusIndex == 1 {
				if m.formAuthMethod == authMethodKey {
					actualInputIndex = 5 // Key Path
				} else if m.formAuthMethod == authMethodPassword {
					actualInputIndex = 6 // Password
				}
			}

			if isFocusableField && actualInputIndex != -1 && k != "tab" && k != "shift+tab" && k != "enter" && k != "esc" {
				if actualInputIndex < len(m.formInputs) {
					t := m.formInputs[actualInputIndex]
					m.formInputs[actualInputIndex], cmd = t.Update(msg)
					cmds = append(cmds, cmd)
				}
			}
		}
	}
	return tea.Batch(cmds...)
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
		headerHeight := 2
		footerHeight := 2
		if !m.ready {
			m.viewport = viewport.New(m.width, m.height-headerHeight-footerHeight)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = m.height - headerHeight - footerHeight
		}
		if m.currentState == stateRunningSequence || m.currentState == stateSequenceError {
			m.viewport.SetContent(m.outputContent)
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
			// We might need a more robust way to check if viewport consumed the key.
			// For now, if vpCmd is not nil, assume it might have been handled.
			if vpCmd != nil {
				return m, tea.Batch(cmds...)
			}
		}

		// --- State-Specific Key Handling (if viewport not active or didn't handle) ---
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

		case stateRunningSequence, stateSequenceError:
			// Handled by viewportActive check above
			if key.Matches(msg, m.keymap.Quit) {
				return m, tea.Quit
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
				m.importError = nil  // Clear import messages
				m.importInfoMsg = "" // Clear import messages
			case key.Matches(msg, m.keymap.Up):
				if m.configCursor > 0 {
					m.configCursor--
				}
			case key.Matches(msg, m.keymap.Down):
				if m.configCursor < len(m.configuredHosts)-1 {
					m.configCursor++
				}
			case key.Matches(msg, m.keymap.Remove):
				if len(m.configuredHosts) > 0 && m.configCursor < len(m.configuredHosts) {
					m.hostToRemove = &m.configuredHosts[m.configCursor]
					m.currentState = stateSshConfigRemoveConfirm
					m.lastError = nil
				}
			case key.Matches(msg, m.keymap.Add):
				m.formInputs = createAddForm()
				m.formFocusIndex = 0
				m.formAuthMethod = authMethodAgent // Default to Agent
				m.formError = nil
				m.currentState = stateSshConfigAddForm
				return m, tea.Batch(cmds...)
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
					return m, tea.Batch(cmds...)
				}
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
			case key.Matches(msg, m.keymap.Down):
				if m.importCursor < len(m.importableHosts)-1 {
					m.importCursor++
				}
			case key.Matches(msg, m.keymap.Select):
				if _, ok := m.selectedImportIdxs[m.importCursor]; ok {
					delete(m.selectedImportIdxs, m.importCursor)
				} else {
					m.selectedImportIdxs[m.importCursor] = struct{}{}
				}
			case key.Matches(msg, m.keymap.Enter):
				if len(m.selectedImportIdxs) > 0 {
					m.currentState = stateSshConfigImportDetails
					m.hostsToConfigure = []config.SSHHost{}
					m.configuringHostIdx = 0
					m.formError = nil

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
				cmds = append(cmds, m.handleSshImportDetailsFormKeys(msg)...)
			}

		default: // Loading projects state
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
		// TODO: Consider sorting projects (e.g., local first, then by name)
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
					statusStr = statusUpStyle.Render(" [loaded]") // Placeholder status
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

	} // End main message switch

	if viewportActive {
		// Viewport update handled earlier
	}

	if m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm || m.currentState == stateSshConfigImportDetails {
		cmds = append(cmds, m.updateInputs(msg))
	}

	return m, tea.Batch(cmds...)
}

// getAddFormFocusMap returns the sequence of logical focus indices for the Add form.
func (m *model) getAddFormFocusMap() []int {
	// Logical indices:
	// 0-4: Name, Hostname, User, Port, RemoteRoot (Input indices 0-4)
	// 5: Auth Method
	// 6: KeyPath (Input index 5) - if authMethodKey
	// 7: Password (Input index 6) - if authMethodPassword
	focusMap := []int{0, 1, 2, 3, 4, 5}
	if m.formAuthMethod == authMethodKey {
		focusMap = append(focusMap, 6)
	} else if m.formAuthMethod == authMethodPassword {
		focusMap = append(focusMap, 7)
	}
	return focusMap
}

// handleSshAddFormKeys handles key presses when the SSH Add form is active.
func (m *model) handleSshAddFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd
	focusMap := m.getAddFormFocusMap()
	currentIndexInMap := -1
	for i, logicalIndex := range focusMap {
		if logicalIndex == m.formFocusIndex {
			currentIndexInMap = i
			break
		}
	}

	switch {
	// Navigation
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
			} else { // Right
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			// Ensure focus stays valid after auth method change
			newFocusMap := m.getAddFormFocusMap()
			found := false
			for _, logicalIndex := range newFocusMap {
				if logicalIndex == m.formFocusIndex {
					found = true
					break
				}
			}
			if !found { // If current focus index is no longer valid (e.g., was KeyPath), move to Auth Method
				m.formFocusIndex = 5
			}
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

	// --- Update Focus State ---
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

// --- Edit Form Navigation Helpers ---

// getEditFormFocusMap returns the sequence of logical focus indices for the Edit form.
func (m *model) getEditFormFocusMap() []int {
	// Logical indices:
	// 0-4: Name, Hostname, User, Port, RemoteRoot (Input indices 0-4)
	// 5: Auth Method
	// 6: KeyPath (Input index 5) - if authMethodKey
	// 7: Password (Input index 6) - if authMethodPassword
	// 8: Disabled Toggle
	focusMap := []int{0, 1, 2, 3, 4, 5}
	if m.formAuthMethod == authMethodKey {
		focusMap = append(focusMap, 6)
	} else if m.formAuthMethod == authMethodPassword {
		focusMap = append(focusMap, 7)
	}
	focusMap = append(focusMap, 8)
	return focusMap
}

// getAuthMethodFocusIndex returns the logical focus index for the Auth Method selector in the Edit form.
func (m *model) getAuthMethodFocusIndex() int {
	return 5
}

// getDisabledToggleFocusIndex returns the logical focus index for the Disabled toggle in the Edit form.
func (m *model) getDisabledToggleFocusIndex() int {
	return 8
}

// handleSshEditFormKeys handles key presses when the SSH Edit form is active.
func (m *model) handleSshEditFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd
	focusMap := m.getEditFormFocusMap()
	authMethodLogicalIndex := m.getAuthMethodFocusIndex()
	disabledToggleLogicalIndex := m.getDisabledToggleFocusIndex()

	currentIndexInMap := -1
	for i, logicalIndex := range focusMap {
		if logicalIndex == m.formFocusIndex {
			currentIndexInMap = i
			break
		}
	}

	switch {
	// Navigation
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
			} else { // Right
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			// Ensure focus stays valid after auth method change
			newFocusMap := m.getEditFormFocusMap()
			found := false
			for _, logicalIndex := range newFocusMap {
				if logicalIndex == m.formFocusIndex {
					found = true
					break
				}
			}
			if !found { // If current focus index is no longer valid, move to Auth Method
				m.formFocusIndex = authMethodLogicalIndex
			}
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

	// --- Update Focus State ---
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

// handleSshImportDetailsFormKeys handles key presses when the SSH Import Details form is active.
func (m *model) handleSshImportDetailsFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Determine number of focusable items: RemoteRoot + (KeyPath or Password if needed) + AuthMethod
	numFocusable := 1 // Remote Root
	authNeeded := false
	// Check if configuringHostIdx is valid before accessing importableHosts
	if m.configuringHostIdx >= 0 && m.configuringHostIdx < len(m.importableHosts) {
		authNeeded = m.importableHosts[m.configuringHostIdx].KeyPath == ""
	}
	if authNeeded {
		numFocusable++ // KeyPath or Password field
		numFocusable++ // Auth Method selector
	}

	switch {
	// Navigation
	case key.Matches(msg, m.keymap.Tab), key.Matches(msg, m.keymap.Down):
		m.formFocusIndex = (m.formFocusIndex + 1) % numFocusable
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		m.formFocusIndex--
		if m.formFocusIndex < 0 {
			m.formFocusIndex = numFocusable - 1
		}
	// Auth Method Cycling
	case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
		isAuthMethodFocused := authNeeded && m.formFocusIndex == 2 // Index 2 is auth method selector here
		if isAuthMethodFocused {
			if key.Matches(msg, m.keymap.Left) {
				m.formAuthMethod--
				if m.formAuthMethod < authMethodKey {
					m.formAuthMethod = authMethodPassword
				}
			} else { // Right
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			m.formError = nil
		}

	case key.Matches(msg, m.keymap.Enter):
		m.formError = nil

		if m.configuringHostIdx < 0 || m.configuringHostIdx >= len(m.importableHosts) {
			m.formError = fmt.Errorf("internal error: invalid host index for import details")
			return cmds
		}
		currentPotentialHost := m.importableHosts[m.configuringHostIdx]

		remoteRoot := strings.TrimSpace(m.formInputs[4].Value())
		keyPath := strings.TrimSpace(m.formInputs[5].Value())
		password := m.formInputs[6].Value()

		if remoteRoot == "" {
			m.formError = fmt.Errorf("remote root path is required")
			return cmds
		}

		hostToSave, convertErr := config.ConvertToBucketManagerHost(currentPotentialHost, currentPotentialHost.Alias, remoteRoot)
		if convertErr != nil {
			m.formError = fmt.Errorf("internal conversion error: %w", convertErr)
			return cmds
		}

		if currentPotentialHost.KeyPath == "" {
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
		}

		m.hostsToConfigure = append(m.hostsToConfigure, hostToSave)

		nextSelectedIdx := -1
		for i := m.configuringHostIdx + 1; i < len(m.importableHosts); i++ {
			if _, ok := m.selectedImportIdxs[i]; ok {
				nextSelectedIdx = i
				break
			}
		}

		if nextSelectedIdx != -1 {
			m.configuringHostIdx = nextSelectedIdx
			pHostToConfigure := m.importableHosts[m.configuringHostIdx]
			m.formInputs, m.formAuthMethod = createImportDetailsForm(pHostToConfigure)
			m.formFocusIndex = 0
			m.formError = nil
		} else {
			cmds = append(cmds, saveImportedSshHostsCmd(m.hostsToConfigure))
		}
	}

	// --- Update Focus State ---
	remoteRootIdx := 4
	keyPathIdx := 5
	passwordIdx := 6

	m.formInputs[remoteRootIdx].Blur()
	m.formInputs[keyPathIdx].Blur()
	m.formInputs[passwordIdx].Blur()
	m.formInputs[remoteRootIdx].Prompt = "  "
	m.formInputs[keyPathIdx].Prompt = "  "
	m.formInputs[passwordIdx].Prompt = "  "
	m.formInputs[remoteRootIdx].TextStyle = lipgloss.NewStyle()
	m.formInputs[keyPathIdx].TextStyle = lipgloss.NewStyle()
	m.formInputs[passwordIdx].TextStyle = lipgloss.NewStyle()

	if m.formFocusIndex == 0 {
		cmds = append(cmds, m.formInputs[remoteRootIdx].Focus())
		m.formInputs[remoteRootIdx].Prompt = cursorStyle.Render("> ")
		m.formInputs[remoteRootIdx].TextStyle = cursorStyle
	} else if authNeeded {
		if m.formFocusIndex == 1 {
			if m.formAuthMethod == authMethodKey {
				cmds = append(cmds, m.formInputs[keyPathIdx].Focus())
				m.formInputs[keyPathIdx].Prompt = cursorStyle.Render("> ")
				m.formInputs[keyPathIdx].TextStyle = cursorStyle
			} else if m.formAuthMethod == authMethodPassword {
				cmds = append(cmds, m.formInputs[passwordIdx].Focus())
				m.formInputs[passwordIdx].Prompt = cursorStyle.Render("> ")
				m.formInputs[passwordIdx].TextStyle = cursorStyle
			}
		}
	}

	return cmds
}

// buildHostFromForm creates a config.SSHHost struct from the current form state.
// It also performs basic validation.
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
	if host.RemoteRoot == "" {
		return host, fmt.Errorf("remote root path is required")
	}

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

	// Auth method specific validation
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
		// No specific fields required for agent
		host.KeyPath = ""
		host.Password = ""
	default:
		return host, fmt.Errorf("invalid authentication method selected")
	}

	// Check for name conflict with *currently loaded* hosts (final check done in command)
	for _, existingHost := range m.configuredHosts {
		if existingHost.Name == host.Name {
			return host, fmt.Errorf("host name '%s' already exists", host.Name)
		}
	}

	return host, nil
}

// buildHostFromEditForm creates a config.SSHHost struct from the current edit form state.
// It uses m.hostToEdit for original values if inputs are left blank.
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
	if editedHost.RemoteRoot == "" {
		return editedHost, fmt.Errorf("remote root path cannot be empty")
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

	// Auth method specific validation
	keyPathInput := strings.TrimSpace(m.formInputs[5].Value())
	passwordInput := m.formInputs[6].Value()

	switch m.formAuthMethod {
	case authMethodKey:
		if keyPathInput == "" {
			// If blank, keep original key path
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
	case key.Matches(msg, m.keymap.PgUp), key.Matches(msg, m.keymap.PgDown), key.Matches(msg, m.keymap.Home), key.Matches(msg, m.keymap.End):
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
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
			if len(m.projects) > 0 && m.cursor < len(m.projects) {
				m.detailedProject = &m.projects[m.cursor]
				m.currentState = stateProjectDetails
				projID := m.detailedProject.Identifier()
				if !m.loadingStatus[projID] {
					m.loadingStatus[projID] = true
					cmds = append(cmds, fetchProjectStatusCmd(*m.detailedProject))
				}
			}
			// Config key is handled in the main Update switch
		}
	}

	// Fetch status if cursor moved to an unloaded project
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

// runSequenceOnSelection prepares and starts a sequence for selected projects or the current project.
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
			combinedSequence = append(combinedSequence, sequenceFunc(*projPtr)...)
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

func (m *model) View() string {
	if !m.ready {
		return "Initializing..."
	}
	var header, body, footer string
	header = titleStyle.Render("Bucket Manager TUI") + "\n"
	bodyContent := strings.Builder{}

	headerHeight := 2
	footerHeight := 2
	newViewportHeight := m.height - headerHeight - footerHeight
	if newViewportHeight < 1 {
		newViewportHeight = 1
	}
	m.viewport.Height = newViewportHeight

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
		m.viewport.SetContent(listContent.String())
		body = m.viewport.View()
	case stateRunningSequence, stateSequenceError:
		body = m.viewport.View()
	case stateProjectDetails:
		if m.detailedProject == nil {
			bodyContent.WriteString(errorStyle.Render("Error: No project selected for details."))
		} else {
			projID := m.detailedProject.Identifier()
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for: %s (%s)", m.detailedProject.Name, serverNameStyle.Render(m.detailedProject.ServerName))) + "\n\n")
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
			bodyContent.WriteString(fmt.Sprintf("Overall Status:%s\n", statusStr))
			if !isLoading && loaded && statusInfo.Error != nil {
				bodyContent.WriteString(errorStyle.Render(fmt.Sprintf("  Error fetching status: %v\n", statusInfo.Error)))
			}
			if !isLoading && loaded && len(statusInfo.Containers) > 0 {
				bodyContent.WriteString("\nContainers:\n")
				bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "SERVICE", "CONTAINER NAME", "STATUS"))
				bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "-------", "--------------", "------"))
				for _, c := range statusInfo.Containers {
					isUp := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
					statusRenderFunc := statusDownStyle.Render
					if isUp {
						statusRenderFunc = statusUpStyle.Render
					}
					bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", c.Service, c.Name, statusRenderFunc(c.Status)))
				}
			} else if !isLoading && loaded && statusInfo.OverallStatus != runner.StatusError {
				bodyContent.WriteString("\n  (No containers found or running)\n")
			}
		}
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
				bodyContent.WriteString(fmt.Sprintf("%s%s (%s)%s\n", cursor, host.Name, serverNameStyle.Render(details), status))
			}
		}
		if m.lastError != nil && strings.Contains(m.lastError.Error(), "ssh config") {
			bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Config Error: %v", m.lastError)))
		}
	case stateSshConfigRemoveConfirm:
		if m.hostToRemove != nil {
			bodyContent.WriteString(fmt.Sprintf("Are you sure you want to remove the SSH host '%s'?\n\n", identifierColor.Render(m.hostToRemove.Name)))
			bodyContent.WriteString("[y] Yes, remove | [n/Esc/b] No, cancel")
		} else {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for removal. Press Esc/b to go back."))
		}
	case stateSshConfigAddForm:
		bodyContent.WriteString(titleStyle.Render("Add New SSH Host") + "\n\n")

		for i := 0; i < 5; i++ {
			bodyContent.WriteString(m.formInputs[i].View() + "\n")
		}

		// Render Auth Method Selection FIRST
		authFocus := "  "
		authStyle := lipgloss.NewStyle()
		numVisibleInputs := 5
		if m.formAuthMethod == authMethodKey {
			numVisibleInputs++
		} else if m.formAuthMethod == authMethodPassword {
			numVisibleInputs++
		}
		// Check if auth method selector (logical index 5) is focused
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
		} else if m.formAuthMethod == authMethodPassword {
			bodyContent.WriteString(m.formInputs[6].View() + "\n")
		}

		if m.formError != nil {
			bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
		}

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
	case stateSshConfigImportDetails:
		if len(m.importableHosts) == 0 || m.configuringHostIdx >= len(m.importableHosts) {
			bodyContent.WriteString(errorStyle.Render("Error: Invalid state for import details."))
		} else {
			pHost := m.importableHosts[m.configuringHostIdx]
			title := fmt.Sprintf("Configure Import: %s (%s@%s)", identifierColor.Render(pHost.Alias), pHost.User, pHost.Hostname)
			bodyContent.WriteString(titleStyle.Render(title) + "\n\n")

			bodyContent.WriteString(m.formInputs[4].View() + "\n")

			authNeeded := pHost.KeyPath == ""
			if authNeeded {
				if m.formAuthMethod == authMethodKey {
					bodyContent.WriteString(m.formInputs[5].View() + "\n")
				} else if m.formAuthMethod == authMethodPassword {
					bodyContent.WriteString(m.formInputs[6].View() + "\n")
				}

				authFocus := "  "
				authStyle := lipgloss.NewStyle()
				// Focus index 2 corresponds to the auth method selector in this context
				if m.formFocusIndex == 2 {
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
			} else {
				// Display the key path found in ssh_config
				bodyContent.WriteString(fmt.Sprintf("  Auth Method: SSH Key File (from ssh_config: %s)\n", lipgloss.NewStyle().Faint(true).Render(pHost.KeyPath)))
			}

			if m.formError != nil {
				bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
			}
		}
	case stateSshConfigEditForm:
		if m.hostToEdit == nil {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for editing."))
		} else {
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Edit SSH Host: %s", identifierColor.Render(m.hostToEdit.Name))) + "\n\n")

			for i := range 5 {
				bodyContent.WriteString(m.formInputs[i].View() + "\n")
			}

			// Render Auth Method Selection FIRST
			authFocus := "  "
			authStyle := lipgloss.NewStyle()
			numVisibleInputs := 5
			if m.formAuthMethod == authMethodKey {
				numVisibleInputs++
			} else if m.formAuthMethod == authMethodPassword {
				numVisibleInputs++
			}
			// Check if auth method selector (logical index 5) is focused
			if m.formFocusIndex == m.getAuthMethodFocusIndex() {
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
			} else if m.formAuthMethod == authMethodPassword {
				bodyContent.WriteString(m.formInputs[6].View() + "\n")
			}

			disabledFocus := "  "
			disabledStyle := lipgloss.NewStyle()
			// Check if disabled toggle (logical index 8) is focused
			if m.formFocusIndex == m.getDisabledToggleFocusIndex() {
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
	}

	// Use bodyContent only if body wasn't set by viewport
	if body == "" {
		body = bodyContent.String()
	}

	// --- Footer ---
	footerContent := strings.Builder{}
	footerContent.WriteString("\n")

	switch m.currentState {
	case stateProjectList:
		hasDiscoveryMessage := false
		if m.isDiscovering {
			footerContent.WriteString(statusLoadingStyle.Render("Discovering remote projects...") + "\n")
			hasDiscoveryMessage = true
		}
		hasErrors := false
		if len(m.discoveryErrors) > 0 {
			footerContent.WriteString(errorStyle.Render("Discovery Errors:"))
			for _, err := range m.discoveryErrors {
				footerContent.WriteString("\n  " + errorStyle.Render(err.Error()))
			}
			footerContent.WriteString("\n") // Add separator before keys
			hasErrors = true
		} else if m.lastError != nil && strings.Contains(m.lastError.Error(), "discovery") {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Discovery Warning: %v", m.lastError)) + "\n")
			hasErrors = true
		}

		if !hasDiscoveryMessage && !hasErrors {
			footerContent.WriteString("\n")
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
			help.WriteString(fmt.Sprintf(" (%d selected)", len(m.selectedImportIdxs)))
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

	default: // Loading projects state
		help := strings.Builder{}
		help.WriteString(m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc)
		footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))
	}
	footer = footerContent.String()

	finalView := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return finalView
}
