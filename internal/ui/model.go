// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"bucket-manager/internal/sshconfig"
	"fmt"
	"os"
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

// KeyMap defines additional keybindings.
type KeyMap struct {
	Config key.Binding
	Remove key.Binding
	Add    key.Binding
	Import key.Binding
	Edit   key.Binding
}

// DefaultKeyMap defines the default keybindings.
var DefaultKeyMap = KeyMap{
	Config: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "manage ssh config"),
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
}

type model struct {
	keymap           KeyMap
	projects         []discovery.Project
	cursor           int             // project list cursor
	configCursor     int             // config list cursor
	hostToRemove     *config.SSHHost // For remove confirmation
	hostToEdit       *config.SSHHost // For edit form
	configuredHosts  []config.SSHHost
	viewport         viewport.Model
	currentState     state
	isDiscovering    bool // Flag to track discovery process
	currentSequence  []runner.CommandStep
	currentStepIndex int
	outputContent    string
	lastError        error
	discoveryErrors  []error // Store multiple discovery errors
	ready            bool
	width            int
	height           int
	outputChan       <-chan runner.OutputLine
	errorChan        <-chan error
	projectStatuses  map[string]runner.ProjectRuntimeInfo
	loadingStatus    map[string]bool
	detailedProject  *discovery.Project
	sequenceProject  *discovery.Project

	// SSH Config Add Form State
	formInputs     []textinput.Model
	formFocusIndex int
	formAuthMethod int  // 1=Key, 2=Agent, 3=Password
	formDisabled   bool // For edit form disabled toggle
	formError      error

	// SSH Config Import State
	importableHosts    []sshconfig.PotentialHost // Hosts parsed from file, filtered for conflicts
	selectedImportIdxs map[int]struct{}          // Indices of importableHosts selected by user
	importCursor       int                       // Cursor for import selection list
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
	potentialHosts []sshconfig.PotentialHost
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
				if BubbleProgram != nil { // Check if program is initialized
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
			return nil // Channel closed
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
			return nil // Channel closed
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
			// Find the host by its original name
			if cfg.SSHHosts[i].Name == originalName {
				// Check for name conflict if the name was changed
				if originalName != editedHost.Name {
					for j, otherHost := range cfg.SSHHosts {
						if i != j && otherHost.Name == editedHost.Name {
							return sshHostEditedMsg{fmt.Errorf("host name '%s' already exists", editedHost.Name)}
						}
					}
				}
				// Update the host in the slice
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
		return sshHostEditedMsg{nil} // Success
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
		return sshHostAddedMsg{nil} // Success
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
			// Return stepFinishedMsg with the error field set
			return stepFinishedMsg{err: fmt.Errorf("host '%s' not found in config during removal", hostToRemove.Name)}
		}
		cfg.SSHHosts = newHosts
		err = config.SaveConfig(cfg)
		if err != nil {
			// Return stepFinishedMsg with the error field set
			return stepFinishedMsg{err: fmt.Errorf("failed to save config after remove: %w", err)}
		}
		return stepFinishedMsg{nil} // nil error signals success
	}
}

// parseSshConfigCmd attempts to parse hosts from the default SSH config file path.
func parseSshConfigCmd() tea.Cmd {
	return func() tea.Msg {
		// sshconfig.ParseSSHConfig already handles default path (~/.ssh/config)
		potentialHosts, err := sshconfig.ParseSSHConfig()
		return sshConfigParsedMsg{potentialHosts: potentialHosts, err: err}
	}
}

// saveImportedSshHostsCmd attempts to save multiple new SSH hosts to the config.
func saveImportedSshHostsCmd(hostsToSave []config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		if len(hostsToSave) == 0 {
			return sshHostsImportedMsg{importedCount: 0, err: nil} // Nothing to save
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
			currentNames[newHost.Name] = true // Add to map for checks within this batch
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
		keymap:          DefaultKeyMap,
		currentState:    stateLoadingProjects, // Start in loading state
		isDiscovering:   true,                 // Mark discovery as active
		cursor:          0,
		configCursor:    0,
		projectStatuses: make(map[string]runner.ProjectRuntimeInfo),
		loadingStatus:   make(map[string]bool),
		configuredHosts: []config.SSHHost{},
		discoveryErrors: []error{}, // Initialize error slice
		detailedProject: nil,
		sequenceProject: nil,
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
	t.Focus() // Start with the first field focused
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
	t.Validate = func(s string) error { // Basic validation for integer
		if s == "" {
			return nil
		} // Allow empty for default
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
	t.Focus() // Start with the first field focused
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
	if host.Port != 0 { // Only set value if not default
		portStr = strconv.Itoa(host.Port)
	}
	t.SetValue(portStr)
	t.CharLimit = 5
	t.Width = 20
	t.Validate = func(s string) error { // Basic validation for integer
		if s == "" {
			return nil
		} // Allow empty for default
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
func createImportDetailsForm(pHost sshconfig.PotentialHost) ([]textinput.Model, int) {
	// We only need RemoteRoot, and potentially KeyPath/Password if not in pHost.
	// Let's reuse the indices from createAddForm for consistency:
	// 4: RemoteRoot
	// 5: KeyPath
	// 6: Password
	inputs := make([]textinput.Model, 7) // Keep size consistent, but only use needed ones
	var t textinput.Model
	initialAuthMethod := authMethodAgent // Default to agent

	// Remote Root (Index 4) - Always needed
	t = textinput.New()
	t.Placeholder = "Remote Root Path (e.g., /home/user/projects)"
	t.Focus() // Start focus here
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

// updateInputs updates the focused text input component for the Add form
func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	// Only update the focused input
	// Check if formInputs is initialized and we are in a form state
	if (m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm) && m.formInputs != nil && m.formFocusIndex >= 0 && m.formFocusIndex < len(m.formInputs) {
		var cmd tea.Cmd
		// Only pass key messages to the input field if it's one of the text inputs
		// (formFocusIndex might point to Auth Method or Disabled toggle)
		isTextInputFocused := false
		numVisibleInputs := 5 // Base: Name, Hostname, User, Port, RemoteRoot
		if m.formAuthMethod == authMethodKey {
			numVisibleInputs++
		} // KeyPath
		if m.formAuthMethod == authMethodPassword {
			numVisibleInputs++
		} // Password

		if m.formFocusIndex < numVisibleInputs {
			isTextInputFocused = true
		}

		if isTextInputFocused {
			// Only pass key messages to the input field
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				// Skip Tab/Shift+Tab/Enter/Esc/Space as they are handled separately for navigation/submission/toggles
				k := keyMsg.String()
				if k != "tab" && k != "shift+tab" && k != "enter" && k != "esc" && k != " " {
					// Determine the actual input index based on auth method
					inputIndex := m.formFocusIndex
					if m.formAuthMethod != authMethodKey && inputIndex == 5 { // Skip KeyPath index if not visible
						// This logic might need refinement if indices shift significantly
						// For now, assume indices 0-4 are always present, 5 is KeyPath, 6 is Password
						// If focus is on 5 but KeyPath isn't visible, something is wrong, but let's prevent panic
						// The navigation logic should prevent focus from landing here incorrectly.
					} else if m.formAuthMethod != authMethodPassword && inputIndex == 6 { // Skip Password index if not visible
						// Similar to above
					}

					if inputIndex < len(m.formInputs) {
						t := m.formInputs[inputIndex]
						m.formInputs[inputIndex], cmd = t.Update(msg)
						cmds = append(cmds, cmd)
					}
				}
			}
		}
	} else if m.currentState == stateSshConfigImportDetails && m.formInputs != nil {
		// Similar logic for the Import Details form, but might focus on different indices
		var cmd tea.Cmd // Declare cmd here
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

			// Allow 'm' key through now
			if isFocusableField && actualInputIndex != -1 && k != "tab" && k != "shift+tab" && k != "enter" && k != "esc" {
				// Ensure the input exists before updating
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
	// Start discovery process; results will arrive via messages
	return findProjectsCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd // Viewport command

	// Determine if the viewport should be considered active for input handling
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
		// Ensure viewport content is updated if it's active
		if m.currentState == stateRunningSequence || m.currentState == stateSequenceError {
			m.viewport.SetContent(m.outputContent)
		}

	case tea.KeyMsg:
		// Give viewport first dibs on keys when it's active
		if viewportActive {
			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
			// If viewport handled the key (e.g., scrolling), don't process further general keys.
			// This assumes viewport.Update indicates consumption, which might need adjustment
			// based on bubbletea/viewport behavior. For now, let's assume if vpCmd is not nil,
			// the viewport might have handled it. A more robust check might be needed.
			// However, allowing fallthrough for now to handle Back/Quit even during scroll.
		}

		switch m.currentState {
		case stateProjectList:
			switch {
			case key.Matches(msg, m.keymap.Config):
				m.currentState = stateSshConfigList
				m.configCursor = 0
				cmds = append(cmds, loadSshConfigCmd())
			default:
				// Let handleProjectListKeys handle navigation and actions
				// It returns cmds, so append them
				cmds = append(cmds, m.handleProjectListKeys(msg)...)
			}

		case stateRunningSequence, stateSequenceError:
			// Only handle quit/back keys if the viewport is active.
			// The viewport itself handles scrolling keys via the update above.
			if viewportActive {
				// Check for Back/Quit keys regardless of whether viewport handled scrolling.
				if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
					return m, tea.Quit
				}
				if key.Matches(msg, key.NewBinding(key.WithKeys("enter", "esc", "b"))) {
					projectToRefresh := m.sequenceProject
					m.currentState = stateProjectList
					m.outputContent = ""
					m.lastError = nil
					m.currentSequence = nil
					m.currentStepIndex = 0
					m.sequenceProject = nil
					m.viewport.GotoTop()
					if projectToRefresh != nil {
						projID := projectToRefresh.Identifier()
						if !m.loadingStatus[projID] {
							m.loadingStatus[projID] = true
							cmds = append(cmds, fetchProjectStatusCmd(*projectToRefresh))
						}
					}
				}
			}

		case stateProjectDetails:
			if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
				return m, tea.Quit
			}
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "b"))) {
				m.currentState = stateProjectList
				m.detailedProject = nil
			}

		case stateSshConfigList:
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
				return m, tea.Quit
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "b"))):
				m.currentState = stateProjectList
				m.lastError = nil
				m.importError = nil  // Clear import messages
				m.importInfoMsg = "" // Clear import messages
			case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
				if m.configCursor > 0 {
					m.configCursor--
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
				if m.configCursor < len(m.configuredHosts)-1 {
					m.configCursor++
				}
			case key.Matches(msg, m.keymap.Remove):
				if len(m.configuredHosts) > 0 && m.configCursor < len(m.configuredHosts) {
					m.hostToRemove = &m.configuredHosts[m.configCursor]
					m.currentState = stateSshConfigRemoveConfirm
					m.lastError = nil // Clear general error when entering confirmation
				}
			case key.Matches(msg, m.keymap.Add):
				m.formInputs = createAddForm()
				m.formFocusIndex = 0
				m.formAuthMethod = authMethodAgent // Default to Agent
				m.formError = nil
				m.currentState = stateSshConfigAddForm
			case key.Matches(msg, m.keymap.Import):
				m.currentState = stateLoadingProjects // Use loading state temporarily
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
				}
			}

		case stateSshConfigRemoveConfirm:
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("y"))):
				if m.hostToRemove != nil {
					cmds = append(cmds, removeSshHostCmd(*m.hostToRemove)) // Call as function
					m.hostToRemove = nil
					m.lastError = nil
					// State will be changed back by stepFinishedMsg handler
				} else {
					m.currentState = stateSshConfigList // Go back if somehow no host was selected
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("n", "esc", "b"))):
				m.currentState = stateSshConfigList
				m.hostToRemove = nil
				m.lastError = nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
				return m, tea.Quit
			}

		case stateSshConfigAddForm:
			// Handle form navigation, input, submission, cancellation
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
				m.currentState = stateSshConfigList
				m.formError = nil    // Clear form error on cancel
				m.formInputs = nil   // Clear form inputs
				m.importError = nil  // Clear import messages when returning
				m.importInfoMsg = "" // Clear import messages when returning
			} else if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
				return m, tea.Quit
			} else {
				// Handle form navigation and input
				cmds = append(cmds, m.handleSshAddFormKeys(msg)...)
			}

		case stateSshConfigEditForm:
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
				m.currentState = stateSshConfigList
				m.formError = nil
				m.formInputs = nil
				m.hostToEdit = nil
				m.importError = nil  // Clear import messages when returning
				m.importInfoMsg = "" // Clear import messages when returning
			} else if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
				return m, tea.Quit
			} else {
				// Handle form navigation, input, submission
				cmds = append(cmds, m.handleSshEditFormKeys(msg)...)
			}

		case stateSshConfigImportSelect:
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
				return m, tea.Quit
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "b"))):
				m.currentState = stateSshConfigList
				m.importableHosts = nil // Clear import state
				m.selectedImportIdxs = nil
				m.importError = nil  // Clear import messages
				m.importInfoMsg = "" // Clear import messages
				m.lastError = nil
			case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
				if m.importCursor > 0 {
					m.importCursor--
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
				if m.importCursor < len(m.importableHosts)-1 {
					m.importCursor++
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys(" "))): // Spacebar to toggle selection
				if _, ok := m.selectedImportIdxs[m.importCursor]; ok {
					delete(m.selectedImportIdxs, m.importCursor)
				} else {
					m.selectedImportIdxs[m.importCursor] = struct{}{}
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				if len(m.selectedImportIdxs) > 0 {
					// Prepare for detail gathering
					m.currentState = stateSshConfigImportDetails
					m.hostsToConfigure = []config.SSHHost{} // Reset list of hosts ready to save
					m.configuringHostIdx = 0                // Start with the first selected host
					m.formError = nil                       // Clear any previous form errors
					// TODO: Need to find the *actual* first selected index
					firstSelectedIdx := -1
					for i := 0; i < len(m.importableHosts); i++ {
						if _, ok := m.selectedImportIdxs[i]; ok {
							firstSelectedIdx = i
							break
						}
					}
					if firstSelectedIdx != -1 {
						// TODO: Prepare the form for the first host (e.g., create inputs)
						// This logic will go into the stateSshConfigImportDetails handler/view
						m.configuringHostIdx = firstSelectedIdx // Set the index to configure
						// Prepare the form for the first selected host
						m.configuringHostIdx = firstSelectedIdx // Set the index of the PotentialHost
						pHostToConfigure := m.importableHosts[m.configuringHostIdx]
						m.formInputs, m.formAuthMethod = createImportDetailsForm(pHostToConfigure)
						m.formFocusIndex = 0 // Focus the first field (Remote Root)
						m.formError = nil    // Clear form error
					} else {
						// Should not happen if len(selectedImportIdxs) > 0, but handle defensively
						m.importError = fmt.Errorf("internal error: no selected host index found")
						m.currentState = stateSshConfigList
						m.importableHosts = nil
						m.selectedImportIdxs = nil
					}
				} else {
					// No hosts selected, just go back
					m.importError = fmt.Errorf("no hosts selected for import")
					m.currentState = stateSshConfigList
					m.importableHosts = nil
					m.selectedImportIdxs = nil
				}
			}

		case stateSshConfigImportDetails:
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
				// Cancel the entire import process
				m.currentState = stateSshConfigList
				m.importError = fmt.Errorf("import cancelled") // Keep this specific error
				m.importInfoMsg = ""                           // Clear info msg
				m.importableHosts = nil
				m.selectedImportIdxs = nil
				m.hostsToConfigure = nil
				m.formInputs = nil
			} else if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
				return m, tea.Quit
			} else {
				// Handle form navigation and input using a dedicated handler
				cmds = append(cmds, m.handleSshImportDetailsFormKeys(msg)...)
			}

		default: // Loading projects state
			if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
				return m, tea.Quit
			}
		}

	case sshConfigParsedMsg:
		// Received potential hosts from parser
		if msg.err != nil {
			m.lastError = fmt.Errorf("failed to parse ssh config: %w", msg.err)
			m.currentState = stateSshConfigList // Go back to list on error
		} else {
			// Filter out hosts that already exist in current config
			cfg, loadErr := config.LoadConfig() // Load current config
			if loadErr != nil {
				m.lastError = fmt.Errorf("failed to load current config for import filtering: %w", loadErr)
				m.currentState = stateSshConfigList
			} else {
				currentConfigNames := make(map[string]bool)
				for _, h := range cfg.SSHHosts {
					currentConfigNames[h.Name] = true
				}

				m.importableHosts = []sshconfig.PotentialHost{}
				for _, pHost := range msg.potentialHosts {
					if _, exists := currentConfigNames[pHost.Alias]; !exists {
						m.importableHosts = append(m.importableHosts, pHost)
					}
				}

				if len(m.importableHosts) == 0 {
					m.importError = fmt.Errorf("no new importable hosts found in ssh config") // Use importError
					m.currentState = stateSshConfigList                                       // Go back to list
				} else {
					// Transition to selection state
					m.currentState = stateSshConfigImportSelect
					m.importCursor = 0
					m.selectedImportIdxs = make(map[int]struct{}) // Reset selections
					m.importError = nil                           // Clear specific import error
					m.lastError = nil                             // Clear general error
				}
			}
		}
	case sshHostsImportedMsg:
		// Result of attempting to save imported hosts
		m.currentState = stateSshConfigList // Go back to list regardless of outcome
		m.importError = nil                 // Clear previous error first
		m.importInfoMsg = ""                // Clear previous info message

		// Check if the error message indicates only non-fatal info (like skipped hosts)
		isInfoOnly := false
		if msg.err != nil {
			errMsgStr := msg.err.Error()
			// Check if the error message *only* contains the "skipped" part or is empty after trimming spaces
			isInfoOnly = strings.HasPrefix(strings.TrimSpace(errMsgStr), "(") && strings.HasSuffix(strings.TrimSpace(errMsgStr), ")") || strings.TrimSpace(errMsgStr) == ""
		}

		if msg.err != nil && !isInfoOnly {
			// Actual error occurred during save or loading config
			m.importError = fmt.Errorf("import failed: %w", msg.err)
		} else {
			// Success or info only (like skipped hosts)
			info := fmt.Sprintf("Import finished: %d hosts added.", msg.importedCount)
			if msg.err != nil && isInfoOnly { // Append the non-fatal info (e.g., skipped count)
				info += fmt.Sprintf(" %s", msg.err.Error()) // Append the original non-fatal error string
			}
			m.importInfoMsg = info
		}

		// Clear import-specific state
		m.importableHosts = nil
		m.selectedImportIdxs = nil
		m.hostsToConfigure = nil
		m.formInputs = nil
		// Reload the config list to show changes
		cmds = append(cmds, loadSshConfigCmd())

	// --- Custom Message Handlers ---
	case projectDiscoveredMsg:
		// If we were in the initial loading state, switch to the list view
		if m.currentState == stateLoadingProjects {
			m.currentState = stateProjectList // Switch to list view on first project
		}
		// Append the newly discovered project
		m.projects = append(m.projects, msg.project)
		// TODO: Consider sorting projects (e.g., local first, then by name)
		// Fetch status for the new project
		projID := msg.project.Identifier()
		if !m.loadingStatus[projID] {
			m.loadingStatus[projID] = true
			cmds = append(cmds, fetchProjectStatusCmd(msg.project))
		}
		// No need to re-queue listener, findProjectsCmd handles the loop

	case discoveryErrorMsg:
		// Store the error; might display the latest or all in the view/footer
		m.discoveryErrors = append(m.discoveryErrors, msg.err)
		// Maybe update lastError to show the latest?
		m.lastError = msg.err // Update lastError to show the most recent one
		// No need to re-queue listener

	case discoveryFinishedMsg:
		fmt.Fprintf(os.Stderr, "[Debug TUI] Update: Received discoveryFinishedMsg. Current isDiscovering: %t\n", m.isDiscovering)
		// Handle state transitions first
		if m.currentState == stateLoadingProjects {
			m.currentState = stateProjectList // Go to list view anyway
			if len(m.discoveryErrors) == 0 {
				// If no errors occurred either, set a specific message
				m.lastError = fmt.Errorf("no projects found")
			} else {
				// Use the last discovery error as the primary message
				m.lastError = fmt.Errorf("discovery finished with errors")
			}
		}
		// Set the flag *after* other state changes in this handler
		m.isDiscovering = false
		fmt.Fprintf(os.Stderr, "[Debug TUI] Update: Set isDiscovering=false\n")

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
		if m.currentState == stateSshConfigRemoveConfirm { // Check if it was a remove operation
			if msg.err != nil {
				m.lastError = fmt.Errorf("failed to remove host: %w", msg.err)
				m.currentState = stateSshConfigList     // Go back to list on error
				cmds = append(cmds, loadSshConfigCmd()) // Reload config list
			} else {
				// Removal succeeded, reload the list
				m.currentState = stateSshConfigList
				cmds = append(cmds, loadSshConfigCmd())
			}
		} else if m.currentState == stateRunningSequence { // Sequence step finished
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
					if m.sequenceProject != nil {
						projID := m.sequenceProject.Identifier()
						if !m.loadingStatus[projID] {
							m.loadingStatus[projID] = true
							cmds = append(cmds, fetchProjectStatusCmd(*m.sequenceProject))
						}
					}
					// Stay in sequence view until user presses back
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
		if m.currentState == stateSshConfigAddForm { // Ensure this message is for the add form
			if msg.err != nil {
				// Save failed, display error in the form
				m.formError = msg.err
			} else {
				// Save succeeded, go back to list and reload
				m.currentState = stateSshConfigList
				m.formError = nil
				m.formInputs = nil                      // Clear form state
				m.configCursor = 0                      // Reset cursor potentially
				cmds = append(cmds, loadSshConfigCmd()) // Reload the list
			}
		}

	case sshHostEditedMsg:
		if m.currentState == stateSshConfigEditForm { // Ensure this message is for the edit form
			if msg.err != nil {
				// Save failed, display error in the form
				m.formError = msg.err
			} else {
				// Save succeeded, go back to list and reload
				m.currentState = stateSshConfigList
				m.formError = nil
				m.formInputs = nil                      // Clear form state
				m.hostToEdit = nil                      // Clear host being edited
				m.configCursor = 0                      // Reset cursor potentially
				cmds = append(cmds, loadSshConfigCmd()) // Reload the list
			}
		}

	} // End main message switch

	// Handle viewport updates if it was active and received keys
	if viewportActive {
		// Already updated above, just collect potential command
	}

	// Update form inputs if a form state is active (Add, Edit, ImportDetails)
	if m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm || m.currentState == stateSshConfigImportDetails {
		cmds = append(cmds, m.updateInputs(msg))
	}

	return m, tea.Batch(cmds...)
}

// handleSshAddFormKeys handles key presses when the SSH Add form is active.
func (m *model) handleSshAddFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	numInputs := len(m.formInputs)
	numVisibleInputs := 5 // Name, Hostname, User, Port, RemoteRoot are always visible
	if m.formAuthMethod == authMethodKey {
		numVisibleInputs++ // KeyPath is visible
	} else if m.formAuthMethod == authMethodPassword {
		numVisibleInputs++ // Password is visible
	}
	totalFocusableItems := numVisibleInputs + 1 // +1 for the Auth Method selector

	switch msg.String() {
	case "tab", "down":
		m.formFocusIndex = (m.formFocusIndex + 1) % totalFocusableItems
	case "shift+tab", "up":
		m.formFocusIndex--
		if m.formFocusIndex < 0 {
			m.formFocusIndex = totalFocusableItems - 1
		}
	case "left", "right":
		// Only cycle auth method if the auth method selector itself is focused
		isAuthMethodFocused := m.formFocusIndex == numVisibleInputs
		if isAuthMethodFocused {
			if msg.String() == "left" {
				m.formAuthMethod--
				if m.formAuthMethod < authMethodKey {
					m.formAuthMethod = authMethodPassword
				}
			} else { // right
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			m.formError = nil // Clear potential errors related to old auth method
		}
		// If not focused on auth method, let the input field handle left/right normally (via updateInputs)

	case "enter":
		// Validate and attempt to save
		m.formError = nil // Clear previous error
		newHost, validationErr := m.buildHostFromForm()
		if validationErr != nil {
			m.formError = validationErr
		} else {
			// Validation passed, dispatch save command
			cmds = append(cmds, saveNewSshHostCmd(newHost))
		}
	}

	// Update focus state for inputs
	for i := 0; i < numInputs; i++ {
		isFocusableIndex := i < numVisibleInputs // Check if the input *index* corresponds to a visible field

		if isFocusableIndex && i == m.formFocusIndex {
			// Set focus
			cmds = append(cmds, m.formInputs[i].Focus())
			m.formInputs[i].Prompt = cursorStyle.Render("> ")
			m.formInputs[i].TextStyle = cursorStyle
		} else {
			// Remove focus
			m.formInputs[i].Blur()
			m.formInputs[i].Prompt = "  "
			m.formInputs[i].TextStyle = lipgloss.NewStyle()
		}
	}

	return cmds
}

// handleSshEditFormKeys handles key presses when the SSH Edit form is active.
func (m *model) handleSshEditFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	numInputs := len(m.formInputs)
	// Determine number of focusable items: Visible Inputs + Auth Method Selector + Disabled Toggle
	numVisibleInputs := 5 // Base: Name, Hostname, User, Port, RemoteRoot
	if m.formAuthMethod == authMethodKey {
		numVisibleInputs++
	} // KeyPath
	if m.formAuthMethod == authMethodPassword {
		numVisibleInputs++
	} // Password
	authMethodFocusIndex := numVisibleInputs
	disabledFocusIndex := numVisibleInputs + 1
	totalFocusableItems := numVisibleInputs + 2 // Inputs + Auth Method + Disabled Toggle

	switch msg.String() {
	case "tab", "down":
		m.formFocusIndex = (m.formFocusIndex + 1) % totalFocusableItems
	case "shift+tab", "up":
		m.formFocusIndex--
		if m.formFocusIndex < 0 {
			m.formFocusIndex = totalFocusableItems - 1
		}
	case " ": // Spacebar - Toggle Disabled status if focused
		if m.formFocusIndex == disabledFocusIndex {
			m.formDisabled = !m.formDisabled
		}
		// Otherwise, let input handle space if focused

	case "left", "right":
		// Only cycle auth method if the auth method selector itself is focused
		if m.formFocusIndex == authMethodFocusIndex {
			if msg.String() == "left" {
				m.formAuthMethod--
				if m.formAuthMethod < authMethodKey {
					m.formAuthMethod = authMethodPassword
				}
			} else { // right
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			m.formError = nil // Clear potential errors related to old auth method
		}
		// If not focused on auth method or disabled toggle, let the input field handle left/right normally (via updateInputs)

	case "enter":
		// Validate and attempt to save
		m.formError = nil        // Clear previous error
		if m.hostToEdit == nil { // Should not happen, but safety check
			m.formError = fmt.Errorf("internal error: no host selected for editing")
			return cmds
		}
		editedHost, validationErr := m.buildHostFromEditForm()
		if validationErr != nil {
			m.formError = validationErr
		} else {
			// Validation passed, dispatch save command
			cmds = append(cmds, saveEditedSshHostCmd(m.hostToEdit.Name, editedHost))
		}
	}

	// Update focus state for inputs
	for i := 0; i < numInputs; i++ {
		isVisibleIndex := false
		if i < 5 {
			isVisibleIndex = true
		} // First 5 always visible conceptually
		if i == 5 && m.formAuthMethod == authMethodKey {
			isVisibleIndex = true
		} // KeyPath
		if i == 6 && m.formAuthMethod == authMethodPassword {
			isVisibleIndex = true
		} // Password

		if isVisibleIndex && i == m.formFocusIndex {
			// Set focus
			cmds = append(cmds, m.formInputs[i].Focus())
			m.formInputs[i].Prompt = cursorStyle.Render("> ")
			m.formInputs[i].TextStyle = cursorStyle
		} else {
			// Remove focus
			m.formInputs[i].Blur()
			m.formInputs[i].Prompt = "  "
			m.formInputs[i].TextStyle = lipgloss.NewStyle()
		}
	}

	return cmds
}

// handleSshImportDetailsFormKeys handles key presses when the SSH Import Details form is active.
func (m *model) handleSshImportDetailsFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Determine number of focusable items: RemoteRoot + (KeyPath or Password if needed) + AuthMethod
	numFocusable := 1 // Remote Root
	authNeeded := m.importableHosts[m.configuringHostIdx].KeyPath == ""
	if authNeeded {
		numFocusable++ // KeyPath or Password field
		numFocusable++ // Auth Method selector
	}

	switch msg.String() {
	case "tab", "down":
		m.formFocusIndex = (m.formFocusIndex + 1) % numFocusable
	case "shift+tab", "up":
		m.formFocusIndex--
		if m.formFocusIndex < 0 {
			m.formFocusIndex = numFocusable - 1
		}
	case "left", "right": // Cycle auth method (only if auth is needed and focused)
		isAuthMethodFocused := authNeeded && m.formFocusIndex == 2 // Index 2 is auth method selector here
		if isAuthMethodFocused {
			if msg.String() == "left" {
				m.formAuthMethod--
				if m.formAuthMethod < authMethodKey {
					m.formAuthMethod = authMethodPassword
				}
			} else { // right
				m.formAuthMethod++
				if m.formAuthMethod > authMethodPassword {
					m.formAuthMethod = authMethodKey
				}
			}
			m.formError = nil // Clear potential errors related to old auth method
		}
		// If not focused on auth method, let the input field handle left/right normally (via updateInputs)

	case "enter":
		// Validate current host details and move to next or save
		m.formError = nil // Clear previous error
		currentPotentialHost := m.importableHosts[m.configuringHostIdx]

		// Extract details from form
		remoteRoot := strings.TrimSpace(m.formInputs[4].Value())
		keyPath := strings.TrimSpace(m.formInputs[5].Value())
		password := m.formInputs[6].Value() // Don't trim

		// Basic validation
		if remoteRoot == "" {
			m.formError = fmt.Errorf("remote root path is required")
			return cmds // Stay on current form
		}

		// Build the host config
		hostToSave, convertErr := sshconfig.ConvertToBucketManagerHost(currentPotentialHost, currentPotentialHost.Alias, remoteRoot)
		if convertErr != nil { // Should not happen if PotentialHost was valid, but check anyway
			m.formError = fmt.Errorf("internal conversion error: %w", convertErr)
			return cmds
		}

		// Apply auth details if needed
		if currentPotentialHost.KeyPath == "" { // Only override auth if not specified in ssh_config
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

		// Add successfully configured host to the list
		m.hostsToConfigure = append(m.hostsToConfigure, hostToSave)

		// Find the next selected host index
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
			m.formFocusIndex = 0 // Reset focus
			m.formError = nil
		} else {
			// No more hosts selected, trigger save command
			cmds = append(cmds, saveImportedSshHostsCmd(m.hostsToConfigure))
			// State transition and cleanup will be handled by sshHostsImportedMsg handler
		}
	}

	// --- Update Focus State ---
	// Map relative focus index (0, 1, 2) to actual input indices (4, 5/6) and auth selector
	remoteRootIdx := 4
	keyPathIdx := 5
	passwordIdx := 6

	// Blur all potentially focusable inputs first
	m.formInputs[remoteRootIdx].Blur()
	m.formInputs[keyPathIdx].Blur()
	m.formInputs[passwordIdx].Blur()
	m.formInputs[remoteRootIdx].Prompt = "  "
	m.formInputs[keyPathIdx].Prompt = "  "
	m.formInputs[passwordIdx].Prompt = "  "
	m.formInputs[remoteRootIdx].TextStyle = lipgloss.NewStyle()
	m.formInputs[keyPathIdx].TextStyle = lipgloss.NewStyle()
	m.formInputs[passwordIdx].TextStyle = lipgloss.NewStyle()

	// Set focus based on relative index and auth method
	if m.formFocusIndex == 0 { // Focus Remote Root (index 4)
		cmds = append(cmds, m.formInputs[remoteRootIdx].Focus())
		m.formInputs[remoteRootIdx].Prompt = cursorStyle.Render("> ")
		m.formInputs[remoteRootIdx].TextStyle = cursorStyle
	} else if authNeeded {
		if m.formFocusIndex == 1 { // Focus KeyPath or Password
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
		// Focus state for the Auth Method selector (index 2) is handled in the View function
	}

	return cmds
}

// buildHostFromForm creates a config.SSHHost struct from the current form state.
// It also performs basic validation.
func (m *model) buildHostFromForm() (config.SSHHost, error) {
	host := config.SSHHost{}

	// Basic required field checks
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

	// Port (optional, defaults to 22 if empty, validate if not empty)
	portStr := strings.TrimSpace(m.formInputs[3].Value())
	if portStr == "" {
		host.Port = 0 // Store 0 for default 22
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return host, fmt.Errorf("invalid port number: %s", portStr)
		}
		if port == 22 {
			host.Port = 0 // Store 0 for default 22
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
		host.Password = m.formInputs[6].Value() // Don't trim password
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

	return host, nil // Validation passed
}

// buildHostFromEditForm creates a config.SSHHost struct from the current edit form state.
// It uses m.hostToEdit for original values if inputs are left blank.
func (m *model) buildHostFromEditForm() (config.SSHHost, error) {
	if m.hostToEdit == nil {
		return config.SSHHost{}, fmt.Errorf("internal error: hostToEdit is nil")
	}
	originalHost := *m.hostToEdit
	editedHost := config.SSHHost{} // Start fresh

	// Get values, falling back to original if blank (after trim)
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

	// Basic required field checks (ensure they didn't become empty)
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

	// Port (optional, defaults to 22 if empty, validate if not empty)
	portStr := strings.TrimSpace(m.formInputs[3].Value())
	if portStr == "" {
		editedHost.Port = 0 // Store 0 for default 22
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return editedHost, fmt.Errorf("invalid port number: %s", portStr)
		}
		if port == 22 {
			editedHost.Port = 0 // Store 0 for default 22
		} else {
			editedHost.Port = port
		}
	}

	// Auth method specific validation
	keyPathInput := strings.TrimSpace(m.formInputs[5].Value())
	passwordInput := m.formInputs[6].Value() // Don't trim password input

	switch m.formAuthMethod {
	case authMethodKey:
		if keyPathInput == "" {
			// If blank, keep original key path
			editedHost.KeyPath = originalHost.KeyPath
		} else {
			editedHost.KeyPath = keyPathInput
		}
		// Final check: Key path cannot be empty if this method is selected
		if editedHost.KeyPath == "" {
			return editedHost, fmt.Errorf("key path is required for Key File authentication")
		}
		editedHost.Password = "" // Ensure password is clear
	case authMethodPassword:
		if passwordInput == "" {
			// If blank, keep original password *only if* auth method didn't change from password
			if originalHost.Password != "" && originalHost.KeyPath == "" {
				editedHost.Password = originalHost.Password
			} else {
				// Auth method changed to password, or original wasn't password, and input is blank
				return editedHost, fmt.Errorf("password is required for Password authentication")
			}
		} else {
			editedHost.Password = passwordInput
		}
		// Final check: Password cannot be empty if this method is selected
		if editedHost.Password == "" {
			return editedHost, fmt.Errorf("password is required for Password authentication")
		}
		editedHost.KeyPath = "" // Ensure key path is clear
	case authMethodAgent:
		// No specific fields required for agent
		editedHost.KeyPath = ""
		editedHost.Password = ""
	default:
		return editedHost, fmt.Errorf("invalid authentication method selected")
	}

	// Check for name conflict only if the name was actually changed
	if editedHost.Name != originalHost.Name {
		for _, existingHost := range m.configuredHosts {
			// Ensure we are not comparing the host against its original entry if name didn't change
			// (The check should be against *other* hosts)
			if existingHost.Name != originalHost.Name && existingHost.Name == editedHost.Name {
				return editedHost, fmt.Errorf("host name '%s' already exists", editedHost.Name)
			}
		}
	}

	// Set the disabled status from the form toggle state
	editedHost.Disabled = m.formDisabled

	return editedHost, nil // Validation passed
}

func (m *model) handleProjectListKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd // Command from viewport update

	// Handle navigation first (cursor and viewport)
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if m.cursor > 0 {
			m.cursor--
		}
		// Send key to viewport regardless of cursor change for scrolling
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}
		// Send key to viewport regardless of cursor change for scrolling
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	// Add PageUp/PageDown/Home/End for viewport scrolling
	case key.Matches(msg, key.NewBinding(key.WithKeys("pgup", "pgdown", "home", "end"))):
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	// Handle actions only if not a navigation key handled above
	default:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			return []tea.Cmd{tea.Quit}
		case key.Matches(msg, key.NewBinding(key.WithKeys("u"))):
			if len(m.projects) > 0 && m.cursor < len(m.projects) {
				m.sequenceProject = &m.projects[m.cursor]
				m.currentSequence = runner.UpSequence(*m.sequenceProject)
				m.currentState = stateRunningSequence
				m.currentStepIndex = 0
				m.outputContent = ""
				m.lastError = nil
				m.viewport.GotoTop()
				cmds = append(cmds, m.startNextStepCmd())
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			if len(m.projects) > 0 && m.cursor < len(m.projects) {
				m.sequenceProject = &m.projects[m.cursor]
				m.currentSequence = runner.DownSequence(*m.sequenceProject)
				m.currentState = stateRunningSequence
				m.currentStepIndex = 0
				m.outputContent = ""
				m.lastError = nil
				m.viewport.GotoTop()
				cmds = append(cmds, m.startNextStepCmd())
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			if len(m.projects) > 0 && m.cursor < len(m.projects) {
				m.sequenceProject = &m.projects[m.cursor]
				m.currentSequence = runner.RefreshSequence(*m.sequenceProject)
				m.currentState = stateRunningSequence
				m.currentStepIndex = 0
				m.outputContent = ""
				m.lastError = nil
				m.viewport.GotoTop()
				cmds = append(cmds, m.startNextStepCmd())
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(m.projects) > 0 && m.cursor < len(m.projects) {
				m.detailedProject = &m.projects[m.cursor]
				m.currentState = stateProjectDetails
				projID := m.detailedProject.Identifier()
				if !m.loadingStatus[projID] {
					m.loadingStatus[projID] = true
					cmds = append(cmds, fetchProjectStatusCmd(*m.detailedProject))
				}
			}
		}
	}

	// Fetch status if cursor moved to an unloaded project (after potential cursor update)
	if (key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))) || key.Matches(msg, key.NewBinding(key.WithKeys("down", "j")))) && len(m.projects) > 0 {
		selectedProject := m.projects[m.cursor]
		projID := selectedProject.Identifier()
		if _, loaded := m.projectStatuses[projID]; !loaded && !m.loadingStatus[projID] {
			m.loadingStatus[projID] = true
			cmds = append(cmds, fetchProjectStatusCmd(selectedProject))
		}
	}

	return cmds
}

func (m *model) startNextStepCmd() tea.Cmd {
	if m.currentSequence == nil || m.currentStepIndex >= len(m.currentSequence) {
		return nil
	}
	step := m.currentSequence[m.currentStepIndex]
	m.outputContent += stepStyle.Render(fmt.Sprintf("\n--- Starting Step: %s for %s ---", step.Name, m.sequenceProject.Identifier())) + "\n"
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

	switch m.currentState {
	case stateLoadingProjects:
		bodyContent.WriteString(statusStyle.Render("Loading projects..."))
	case stateProjectList:
		// Build the project list content string
		listContent := strings.Builder{}
		listContent.WriteString("Select a project:\n") // Header for the list itself
		for i, project := range m.projects {
			cursor := "  "
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
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
			listContent.WriteString(fmt.Sprintf("%s%s (%s)%s\n", cursor, project.Name, serverNameStyle.Render(project.ServerName), statusStr))
		}
		// Set the built string as viewport content
		m.viewport.SetContent(listContent.String())
		// Use the viewport's view for the body
		body = m.viewport.View()
		// Indicator logic moved to footer rendering below
	case stateRunningSequence, stateSequenceError:
		// Ensure viewport content is updated (already done in Update for these states)
		body = m.viewport.View() // Use viewport directly
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
				} // Use Render
				bodyContent.WriteString(fmt.Sprintf("%s%s (%s)%s\n", cursor, host.Name, serverNameStyle.Render(details), status))
			}
		}
		if m.lastError != nil && strings.Contains(m.lastError.Error(), "ssh config") {
			bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Config Error: %v", m.lastError)))
		}
	case stateSshConfigRemoveConfirm:
		if m.hostToRemove != nil {
			bodyContent.WriteString(fmt.Sprintf("Are you sure you want to remove the SSH host '%s'?\n\n", identifierColor.Render(m.hostToRemove.Name))) // Use Render
			bodyContent.WriteString("[y] Yes, remove | [n/Esc/b] No, cancel")
		} else {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for removal. Press Esc/b to go back."))
		}
	case stateSshConfigAddForm:
		bodyContent.WriteString(titleStyle.Render("Add New SSH Host") + "\n\n")

		// Render inputs
		for i := 0; i < 5; i++ { // Always render first 5 inputs
			bodyContent.WriteString(m.formInputs[i].View() + "\n")
		}

		// Conditionally render Key Path or Password input
		if m.formAuthMethod == authMethodKey {
			bodyContent.WriteString(m.formInputs[5].View() + "\n") // Key Path
		} else if m.formAuthMethod == authMethodPassword {
			bodyContent.WriteString(m.formInputs[6].View() + "\n") // Password
		}

		// Render Auth Method Selection
		authFocus := "  "
		authStyle := lipgloss.NewStyle()
		numVisibleInputs := 5
		if m.formAuthMethod == authMethodKey {
			numVisibleInputs++
		} else if m.formAuthMethod == authMethodPassword {
			numVisibleInputs++
		}
		if m.formFocusIndex == numVisibleInputs { // Check if auth method selector is focused
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

		// Render Error
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
					checkbox = successStyle.Render("[x]") // Green check
				}

				details := fmt.Sprintf("%s@%s", pHost.User, pHost.Hostname)
				if pHost.Port != 0 && pHost.Port != 22 {
					details += fmt.Sprintf(":%d", pHost.Port)
				}
				keyInfo := ""
				if pHost.KeyPath != "" {
					// Use filepath.Base to show only the filename
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

			// Render Remote Root input (index 4)
			bodyContent.WriteString(m.formInputs[4].View() + "\n")

			// Conditionally render Key Path or Password input if needed
			authNeeded := pHost.KeyPath == ""
			if authNeeded {
				if m.formAuthMethod == authMethodKey {
					bodyContent.WriteString(m.formInputs[5].View() + "\n") // Key Path
				} else if m.formAuthMethod == authMethodPassword {
					bodyContent.WriteString(m.formInputs[6].View() + "\n") // Password
				}

				// Render Auth Method Selection
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

			// Render Error
			if m.formError != nil {
				bodyContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
			}
		}
	case stateSshConfigEditForm:
		if m.hostToEdit == nil {
			bodyContent.WriteString(errorStyle.Render("Error: No host selected for editing."))
		} else {
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Edit SSH Host: %s", identifierColor.Render(m.hostToEdit.Name))) + "\n\n")

			// Render inputs (pre-filled by createEditForm)
			for i := 0; i < 5; i++ { // Always render first 5 inputs
				bodyContent.WriteString(m.formInputs[i].View() + "\n")
			}

			// Conditionally render Key Path or Password input
			if m.formAuthMethod == authMethodKey {
				bodyContent.WriteString(m.formInputs[5].View() + "\n") // Key Path
			} else if m.formAuthMethod == authMethodPassword {
				bodyContent.WriteString(m.formInputs[6].View() + "\n") // Password
			}

			// Render Auth Method Selection
			authFocus := "  "
			authStyle := lipgloss.NewStyle()
			numVisibleInputs := 5
			if m.formAuthMethod == authMethodKey {
				numVisibleInputs++
			} else if m.formAuthMethod == authMethodPassword {
				numVisibleInputs++
			}
			authMethodFocusIndex := numVisibleInputs
			if m.formFocusIndex == authMethodFocusIndex { // Check if auth method selector is focused
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

			// Render Disabled Toggle
			disabledFocus := "  "
			disabledStyle := lipgloss.NewStyle()
			disabledFocusIndex := numVisibleInputs + 1
			if m.formFocusIndex == disabledFocusIndex {
				disabledFocus = cursorStyle.Render("> ")
				disabledStyle = cursorStyle
			}
			checkbox := "[ ]"
			if m.formDisabled {
				checkbox = successStyle.Render("[x]")
			}
			bodyContent.WriteString(fmt.Sprintf("%s%s\n", disabledFocus, disabledStyle.Render(checkbox+" Disabled Host [space to toggle]")))

			// Render Error
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
		// Add loading indicator to footer if discovery is ongoing
		if m.isDiscovering {
			footerContent.WriteString(statusLoadingStyle.Render("Discovering remote projects...") + "\n")
		}
		footerContent.WriteString(m.keymap.Config.Help().Key + ": " + m.keymap.Config.Help().Desc + " | ")
		footerContent.WriteString("[↑/k ↓/j] Navigate | [Enter] Details | [u] Up | [d] Down | [r] Refresh | [q] Quit")
		// Display discovery errors in the footer as well
		if len(m.discoveryErrors) > 0 {
			footerContent.WriteString("\n" + errorStyle.Render("Discovery Errors:"))
			for _, err := range m.discoveryErrors {
				footerContent.WriteString("\n  " + errorStyle.Render(err.Error()))
			}
		} else if m.lastError != nil && strings.Contains(m.lastError.Error(), "discovery failed") {
			// Fallback for general discovery failure message
			footerContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Discovery Warning: %v", m.lastError)))
		}
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
		footerContent.WriteString("\n[↑/↓ PgUp/PgDn] Scroll | [b/Esc/Enter] Back to List | [q] Quit")
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
		footerContent.WriteString("\n[↑/↓ PgUp/PgDn] Scroll | [b/Esc/Enter] Back to List | [q] Quit")
	case stateProjectDetails:
		footerContent.WriteString("[b/Esc] Back to List | [q] Quit")
	case stateSshConfigList:
		footerContent.WriteString(m.keymap.Add.Help().Key + ": " + m.keymap.Add.Help().Desc + " | ")
		footerContent.WriteString(m.keymap.Edit.Help().Key + ": " + m.keymap.Edit.Help().Desc + " | ")
		footerContent.WriteString(m.keymap.Remove.Help().Key + ": " + m.keymap.Remove.Help().Desc + " | ")
		footerContent.WriteString(m.keymap.Import.Help().Key + ": " + m.keymap.Import.Help().Desc + " | ")
		footerContent.WriteString("[↑/k ↓/j] Navigate | [b/Esc] Back | [q] Quit")
		// Show import info (green) or error (red) first, then general config error
		if m.importInfoMsg != "" {
			footerContent.WriteString("\n" + successStyle.Render(m.importInfoMsg))
		} else if m.importError != nil {
			// Display specific import errors (like failure to save, parse errors, cancellation)
			footerContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Import Error: %v", m.importError)))
		} else if m.lastError != nil && strings.Contains(m.lastError.Error(), "ssh config") {
			// Display general config loading errors if no import messages are present
			footerContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Config Error: %v", m.lastError)))
		}
	case stateSshConfigRemoveConfirm:
		if m.hostToRemove != nil {
			footerContent.WriteString(fmt.Sprintf("Confirm removal of '%s'? [y/N]", identifierColor.Render(m.hostToRemove.Name))) // Use Render
		} else {
			footerContent.WriteString(errorStyle.Render("Error - no host selected. [Esc/b] Back"))
		}
	case stateSshConfigAddForm:
		footerContent.WriteString("[↑/↓/Tab/Shift+Tab] Navigate | [Enter] Save | [Esc] Cancel | [q] Quit")
		// Inline help text "[←/→ to change]" added in the form view itself
		if m.formError != nil {
			footerContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
		}
	case stateSshConfigImportSelect:
		footerContent.WriteString("[↑/k ↓/j] Navigate | [space] Toggle Selection | [Enter] Confirm | [Esc/b] Cancel | [q] Quit")
		if len(m.selectedImportIdxs) > 0 {
			footerContent.WriteString(fmt.Sprintf(" (%d selected)", len(m.selectedImportIdxs)))
		}
	case stateSshConfigImportDetails:
		// Calculate remaining hosts
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
		footerContent.WriteString(fmt.Sprintf("[↑/↓/Tab/Shift+Tab] Navigate | [Enter] Confirm & Next (%d %s remaining) | [Esc] Cancel Import | [q] Quit", remaining, hostLabel))
		// Inline help text "[←/→ to change]" added in the form view itself
		if m.formError != nil {
			footerContent.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)))
		}

	default: // Loading projects state
		footerContent.WriteString("[q] Quit")
	}
	footer = footerContent.String()

	// Combine header, body, footer using lipgloss
	finalView := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return finalView
}
