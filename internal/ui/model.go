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
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/sync/semaphore"
)

var BubbleProgram *tea.Program

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
				isCharKey := msg.Type == tea.KeyRunes && len(msg.Runes) == 1
				if !isScrollKey && !isCharKey {
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
		// Rediscover stacks after import
		m.currentState = stateLoadingStacks
		m.isDiscovering = true
		m.stacks = nil
		m.discoveryErrors = nil
		m.stackStatuses = make(map[string]runner.StackRuntimeInfo)
		m.loadingStatus = make(map[string]bool)
		m.cursor = 0
		cmds = append(cmds, loadSshConfigCmd(), findStacksCmd())

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
				// Successful removal
				m.currentState = stateLoadingStacks // Show loading while rediscovering
				m.isDiscovering = true
				m.stacks = nil // Clear existing stacks
				m.discoveryErrors = nil
				m.stackStatuses = make(map[string]runner.StackRuntimeInfo) // Clear statuses
				m.loadingStatus = make(map[string]bool)
				m.cursor = 0
				cmds = append(cmds, loadSshConfigCmd(), findStacksCmd()) // Reload config AND rediscover stacks
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
				// Rediscover stacks after adding
				m.currentState = stateLoadingStacks
				m.isDiscovering = true
				m.stacks = nil
				m.discoveryErrors = nil
				m.stackStatuses = make(map[string]runner.StackRuntimeInfo)
				m.loadingStatus = make(map[string]bool)
				cmds = append(cmds, loadSshConfigCmd(), findStacksCmd())
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
				// Rediscover stacks after editing
				m.currentState = stateLoadingStacks
				m.isDiscovering = true
				m.stacks = nil
				m.discoveryErrors = nil
				m.stackStatuses = make(map[string]runner.StackRuntimeInfo)
				m.loadingStatus = make(map[string]bool)
				cmds = append(cmds, loadSshConfigCmd(), findStacksCmd())
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
		bodyContent.WriteString(fmt.Sprintf("%s%s (%s)\n", localCursor, "local", serverNameStyle.Render("Local Podman")))

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
