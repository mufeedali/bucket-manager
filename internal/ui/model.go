// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"context"
	"fmt"
	"slices"

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
		ctx := context.Background()
		if err := m.statusCheckSem.Acquire(ctx, 1); err != nil {
			return stackStatusLoadedMsg{
				stackIdentifier: stack.Identifier(),
				statusInfo: runner.StackRuntimeInfo{
					Stack:         stack,
					OverallStatus: runner.StatusError,
					Error:         fmt.Errorf("failed to acquire status check semaphore: %w", err),
				},
			}
		}
		defer m.statusCheckSem.Release(1)

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

// refreshFormInputStyles updates prompts, text styles, and blurs form inputs.
// It styles the input at 'focusedIdx' as active and others as inactive.
// 'checkPlaceholder' (optional) returns true if an input at a given index
// is a placeholder and should not have its style reset (its style is preserved).
func (m *model) refreshFormInputStyles(focusedIdx int, checkPlaceholder func(idx int) bool) {
	for i := range m.formInputs {
		if i == focusedIdx {
			m.formInputs[i].Prompt = cursorStyle.Render("> ")
			m.formInputs[i].TextStyle = cursorStyle
		} else {
			m.formInputs[i].Prompt = "  " // Common for unfocused and placeholders
			isPlaceholder := false
			if checkPlaceholder != nil {
				isPlaceholder = checkPlaceholder(i)
			}

			if !isPlaceholder {
				m.formInputs[i].TextStyle = lipgloss.NewStyle() // Reset style only if not placeholder
			}
			m.formInputs[i].Blur() // Blur all non-focused inputs (placeholders included)
		}
	}
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	viewportActive := m.currentState == stateRunningSequence

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		cmd := handleWindowSizeMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		if viewportActive {
			return m.handleViewportKeys(msg)
		}

		// --- Handle Form Input Updates First (if applicable) ---
		isFormState := m.currentState == stateSshConfigAddForm || m.currentState == stateSshConfigEditForm || m.currentState == stateSshConfigImportDetails
		if isFormState {
			inputCmd := m.handleFormInputUpdates(msg)
			if inputCmd != nil {
				cmds = append(cmds, inputCmd)
			}
			// Note: handleFormInputUpdates modifies m.formInputs directly
		}

		// --- Handle State-Specific Navigation and Actions ---
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
				cmds = slices.Concat(cmds, m.handleStackListKeys(msg))
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
			totalItems := len(m.configuredHosts) + 1 // Includes "local"

			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back):
				m.currentState = stateStackList
				m.lastError = nil
				m.importError = nil
				m.importInfoMsg = ""
			case key.Matches(msg, m.keymap.Up):
				if m.configCursor > 0 {
					m.configCursor--
				}
				m.sshConfigViewport.ScrollUp(1)
			case key.Matches(msg, m.keymap.Down):
				if m.configCursor < totalItems-1 {
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
				if m.configCursor > 0 && m.configCursor < totalItems { // cursor > 0 means not "local"
					remoteHostIndex := m.configCursor - 1 // Adjust for configuredHosts slice
					m.hostToRemove = &m.configuredHosts[remoteHostIndex]
					m.currentState = stateSshConfigRemoveConfirm
					m.lastError = nil
				} else {
					m.lastError = fmt.Errorf("cannot remove 'local' host")
				}
			case key.Matches(msg, m.keymap.Add):
				m.formInputs = createAddForm()
				m.formFocusIndex = 0
				m.formAuthMethod = authMethodAgent
				m.formError = nil
				m.currentState = stateSshConfigAddForm
				m.formViewport.GotoTop()
				// m.formFocusIndex is already 0, which is the first input
				m.refreshFormInputStyles(m.formFocusIndex, nil)
				if len(m.formInputs) > m.formFocusIndex && m.formFocusIndex >= 0 { // Ensure input exists
					cmds = append(cmds, m.formInputs[m.formFocusIndex].Focus())
				}
			case key.Matches(msg, m.keymap.Import):
				m.currentState = stateLoadingStacks // Show loading while parsing
				m.importError = nil
				m.lastError = nil
				cmds = append(cmds, parseSshConfigCmd())
			case key.Matches(msg, m.keymap.Edit):
				if m.configCursor > 0 && m.configCursor < totalItems { // cursor > 0 means not "local"
					remoteHostIndex := m.configCursor - 1 // Adjust for configuredHosts slice
					m.hostToEdit = &m.configuredHosts[remoteHostIndex]
					m.formInputs, m.formAuthMethod, m.formDisabled = createEditForm(*m.hostToEdit)
					m.formFocusIndex = 0
					m.formError = nil
					m.currentState = stateSshConfigEditForm
					m.formViewport.GotoTop()
					// m.formFocusIndex is already 0, which is the first input
					m.refreshFormInputStyles(m.formFocusIndex, nil)
					if len(m.formInputs) > m.formFocusIndex && m.formFocusIndex >= 0 { // Ensure input exists
						cmds = append(cmds, m.formInputs[m.formFocusIndex].Focus())
					}
				} else {
					m.lastError = fmt.Errorf("cannot edit 'local' host")
				}
			case key.Matches(msg, m.keymap.PruneAction):
				m.hostsToPrune = nil
				m.hostActionError = nil
				m.lastError = nil

				if m.configCursor == 0 { // "local" selected
					m.hostsToPrune = []runner.HostTarget{{IsRemote: false, ServerName: "local"}}
				} else if m.configCursor > 0 && m.configCursor < totalItems { // A remote host selected
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

				if len(m.hostsToPrune) > 0 && m.lastError == nil {
					m.currentState = statePruneConfirm
				}
			}
			if vpCmd == nil { // Update viewport only if no specific command was generated for it yet
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
				cmds = slices.Concat(cmds, m.handleSshAddFormKeys(msg))
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
				cmds = slices.Concat(cmds, m.handleSshEditFormKeys(msg))
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
						// m.formFocusIndex is 0 (logical for Remote Root). Actual input index is 4.
						actualFocusedInputIndex := 4 // Remote Root Path is input at index 4
						m.refreshFormInputStyles(actualFocusedInputIndex, func(idx int) bool {
							return idx < 4 // Inputs 0-3 are placeholders
						})
						if len(m.formInputs) > actualFocusedInputIndex {
							cmds = append(cmds, m.formInputs[actualFocusedInputIndex].Focus())
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
					cmds = slices.Concat(cmds, m.handleSshImportDetailsFormKeys(msg))
				}
			}

		case statePruneConfirm:
			switch {
			case key.Matches(msg, m.keymap.Yes):
				if len(m.hostsToPrune) > 0 {
					m.outputContent = statusStyle.Render(fmt.Sprintf("Initiating prune for %s...", m.hostsToPrune[0].ServerName)) + "\n"
					m.currentState = stateRunningHostAction
					m.hostActionError = nil
					step := runner.PruneHostStep(m.hostsToPrune[0])
					m.currentHostActionStep = step
					m.viewport.SetContent(m.outputContent) // Ensure viewport shows the initial message
					m.viewport.GotoBottom()
					cmds = append(cmds, runHostActionCmd(step))
				} else {
					// This case should ideally not be reached if logic is correct
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
		cmd := handleSshConfigParsedMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case sshHostsImportedMsg:
		cmd := handleSshHostsImportedMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case stackDiscoveredMsg:
		cmd := handleStackDiscoveredMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case discoveryErrorMsg:
		cmd := handleDiscoveryErrorMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case discoveryFinishedMsg:
		// The 'msg' parameter is implicitly used by handleDiscoveryFinishedMsg
		// if it needs to access fields from discoveryFinishedMsg.
		// If not, it can be simplified further.
		cmd := handleDiscoveryFinishedMsg(m)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case sshConfigLoadedMsg:
		cmd := handleSshConfigLoadedMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case stackStatusLoadedMsg:
		cmd := handleStackStatusLoadedMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case stepFinishedMsg:
		cmd := handleStepFinishedMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case channelsAvailableMsg:
		cmd := handleChannelsAvailableMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case outputLineMsg:
		cmd := handleOutputLineMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case sshHostAddedMsg:
		cmd := handleSshHostAddedMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case sshHostEditedMsg:
		cmd := handleSshHostEditedMsg(m, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// --- Viewport and Form Input Updates ---
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

	var header, bodyStr, footerStr, bodyContent string
	header = titleStyle.Render("Bucket Manager") + "\n"

	// Call state-specific render function
	switch m.currentState {
	case stateLoadingStacks:
		bodyContent, footerStr = m.renderLoadingView()
	case stateStackList:
		bodyContent, footerStr = m.renderStackListView()
	case stateRunningSequence:
		bodyContent, footerStr = m.renderRunningSequenceView()
	case stateSequenceError:
		bodyContent, footerStr = m.renderSequenceErrorView()
	case stateStackDetails:
		bodyContent, footerStr = m.renderStackDetailsView()
	case stateSshConfigList:
		bodyContent, footerStr = m.renderSshConfigListView()
	case stateSshConfigRemoveConfirm:
		bodyContent, footerStr = m.renderSshConfigRemoveConfirmView()
	case statePruneConfirm:
		bodyContent, footerStr = m.renderPruneConfirmView()
	case stateRunningHostAction:
		bodyContent, footerStr = m.renderRunningHostActionView()
	case stateSshConfigAddForm:
		bodyContent, footerStr = m.renderSshConfigAddFormView()
	case stateSshConfigEditForm:
		bodyContent, footerStr = m.renderSshConfigEditFormView()
	case stateSshConfigImportSelect:
		bodyContent, footerStr = m.renderSshConfigImportSelectView()
	case stateSshConfigImportDetails:
		bodyContent, footerStr = m.renderSshConfigImportDetailsView()
	default:
		bodyContent = errorStyle.Render(fmt.Sprintf("Error: Unknown view state %d", m.currentState))
		footerStr = "\n" + m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc
	}

	// Calculate available height for the body
	actualFooterHeight := lipgloss.Height(footerStr)
	availableHeight := m.height - headerHeight - actualFooterHeight
	if availableHeight < 1 {
		availableHeight = 1 // Ensure at least 1 line
	}

	// Determine which viewport to use and render its content
	// Default to placing content directly if no specific viewport is used for the state.
	bodyStr = lipgloss.PlaceVertical(availableHeight, lipgloss.Top, bodyContent)

	switch m.currentState {
	case stateStackList, stateRunningSequence, stateSequenceError, stateRunningHostAction:
		m.viewport.Height = availableHeight
		m.viewport.SetContent(bodyContent)
		bodyStr = m.viewport.View()
	case stateStackDetails:
		m.detailsViewport.Height = availableHeight
		m.detailsViewport.SetContent(bodyContent)
		bodyStr = m.detailsViewport.View()
	case stateSshConfigList:
		m.sshConfigViewport.Height = availableHeight
		m.sshConfigViewport.SetContent(bodyContent)
		bodyStr = m.sshConfigViewport.View()
	case stateSshConfigAddForm, stateSshConfigEditForm, stateSshConfigImportDetails:
		m.formViewport.Height = availableHeight
		m.formViewport.SetContent(bodyContent)
		bodyStr = m.formViewport.View()
	case stateSshConfigImportSelect:
		m.importSelectViewport.Height = availableHeight
		m.importSelectViewport.SetContent(bodyContent)
		bodyStr = m.importSelectViewport.View()
		// For confirmation states (Remove, Prune), bodyStr is already set by PlaceVertical
	}

	// --- Combine header, body (rendered viewport or placed content), and footer ---
	finalView := lipgloss.JoinVertical(lipgloss.Left, header, bodyStr, footerStr)
	return finalView
}
