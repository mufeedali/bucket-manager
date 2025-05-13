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
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
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
	sshConfigModified  bool                // Flag indicating if SSH config was changed since entering the view
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
		sshConfigModified:    false,
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

// getKeyBindings collects all key.Binding instances from the KeyMap.
func getKeyBindings(km KeyMap) []key.Binding {
	return []key.Binding{
		km.Up, km.Down, km.Left, km.Right, km.PgUp, km.PgDown, km.Home, km.End,
		km.Quit, km.Enter, km.Esc, km.Back, km.Select, km.Tab, km.ShiftTab,
		km.Yes, km.No,
		km.Config, km.UpAction, km.DownAction, km.RefreshAction, km.PullAction,
		km.Remove, km.Add, km.Import, km.Edit,
		km.ToggleDisabled, km.PruneAction,
	}
}

// getCurrentFooterString calls the appropriate render function for the current state
// to get the footer string as it would be displayed.
func (m *model) getCurrentFooterString() string {
	var footerStr string
	switch m.currentState {
	case stateLoadingStacks:
		_, footerStr = m.renderLoadingView()
	case stateStackList:
		_, footerStr = m.renderStackListView()
	case stateRunningSequence:
		_, footerStr = m.renderRunningSequenceView()
	case stateSequenceError:
		_, footerStr = m.renderSequenceErrorView()
	case stateStackDetails:
		_, footerStr = m.renderStackDetailsView()
	case stateSshConfigList:
		_, footerStr = m.renderSshConfigListView()
	case stateSshConfigRemoveConfirm:
		_, footerStr = m.renderSshConfigRemoveConfirmView()
	case statePruneConfirm:
		_, footerStr = m.renderPruneConfirmView()
	case stateRunningHostAction:
		_, footerStr = m.renderRunningHostActionView()
	case stateSshConfigAddForm:
		_, footerStr = m.renderSshConfigAddFormView()
	case stateSshConfigEditForm:
		_, footerStr = m.renderSshConfigEditFormView()
	case stateSshConfigImportSelect:
		_, footerStr = m.renderSshConfigImportSelectView()
	case stateSshConfigImportDetails:
		_, footerStr = m.renderSshConfigImportDetailsView()
	default:
		footerStr = m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc
	}
	return strings.TrimPrefix(footerStr, "\n")
}

// createSimulatedKeyCmd generates a tea.Cmd that simulates a key press for the given binding.
// It uses the first key defined in the binding.
func (m *model) createSimulatedKeyCmd(binding key.Binding) tea.Cmd {
	if len(binding.Keys()) == 0 {
		return nil // No keys to simulate
	}
	keyPress := binding.Keys()[0] // Use the first key defined for the binding
	var simKeyMsg tea.KeyMsg

	switch keyPress {
	case "enter":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyEscape}
	case "ctrl+c":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyCtrlC}
	case "tab":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "up":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyRight}
	case "pgup":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		simKeyMsg = tea.KeyMsg{Type: tea.KeyEnd}
	case " ": // space
		simKeyMsg = tea.KeyMsg{Type: tea.KeySpace}
	default:
		// For single rune keys like 'q', 'y', 'n', etc.
		if utf8.RuneCountInString(keyPress) == 1 {
			simKeyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(keyPress)}
		}
		// Note: Complex key combinations not handled by simple runes/types (e.g. "alt+s")
		// would require more sophisticated mapping if they were used as primary keys for bindings.
		// Currently, the system relies on these standard tea.KeyType and runes.
	}

	if simKeyMsg.Type != 0 || len(simKeyMsg.Runes) > 0 {
		return func() tea.Msg { return simKeyMsg }
	}
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	viewportActive := m.currentState == stateRunningSequence

	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Pass mouse messages to viewports for scrolling, etc.
		switch m.currentState {
		case stateStackList, stateRunningSequence, stateSequenceError, stateRunningHostAction:
			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
		case stateStackDetails:
			m.detailsViewport, vpCmd = m.detailsViewport.Update(msg)
			cmds = append(cmds, vpCmd)
		case stateSshConfigList:
			m.sshConfigViewport, vpCmd = m.sshConfigViewport.Update(msg)
			cmds = append(cmds, vpCmd)
		case stateSshConfigAddForm, stateSshConfigEditForm, stateSshConfigImportDetails:
			m.formViewport, vpCmd = m.formViewport.Update(msg)
			cmds = append(cmds, vpCmd)
		case stateSshConfigImportSelect:
			m.importSelectViewport, vpCmd = m.importSelectViewport.Update(msg)
			cmds = append(cmds, vpCmd)
		}

		// Click handling
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			// --- Body Content Click Handling (existing logic) ---
			// screenLineOffsetToBodyContent = headerRenderHeight (1) + lipgloss.JoinVertical newline (1) + bodyTopBorder (1)
			const listContentTopOffset = 1 + 1 + 1
			const checkboxMinX = 1
			const checkboxMaxX = 6

			clickedInBodyRelativeY := msg.Y - listContentTopOffset
			bodyClicked := false

			switch m.currentState {
			case stateStackList:
				if clickedInBodyRelativeY >= 0 && clickedInBodyRelativeY < m.viewport.Height {
					bodyClicked = true
					clickedItemIndex := m.viewport.YOffset + clickedInBodyRelativeY
					if clickedItemIndex >= 0 && clickedItemIndex < len(m.stacks) {
						m.cursor = clickedItemIndex
						if msg.X >= checkboxMinX && msg.X <= checkboxMaxX {
							if _, ok := m.selectedStackIdxs[m.cursor]; ok {
								delete(m.selectedStackIdxs, m.cursor)
							} else {
								m.selectedStackIdxs[m.cursor] = struct{}{}
							}
						} else {
							enterKeyMsg := tea.KeyMsg{Type: tea.KeyEnter}
							keyCmds := m.handleStackListKeys(enterKeyMsg)
							cmds = append(cmds, keyCmds...)
						}
					}
				}
			case stateSshConfigImportSelect:
				if clickedInBodyRelativeY >= 0 && clickedInBodyRelativeY < m.importSelectViewport.Height {
					bodyClicked = true
					clickedItemIndex := m.importSelectViewport.YOffset + clickedInBodyRelativeY
					if clickedItemIndex >= 0 && clickedItemIndex < len(m.importableHosts) {
						m.importCursor = clickedItemIndex
						if msg.X >= checkboxMinX && msg.X <= checkboxMaxX {
							if _, ok := m.selectedImportIdxs[m.importCursor]; ok {
								delete(m.selectedImportIdxs, m.importCursor)
							} else {
								m.selectedImportIdxs[m.importCursor] = struct{}{}
							}
						}
					}
				}
			}

			// --- Footer Click Handling ---
			if !bodyClicked {
				currentFooterStr := m.getCurrentFooterString()
				if currentFooterStr != "" {
					actualFooterRenderHeight := lipgloss.Height(currentFooterStr)
					footerStartY := m.height - actualFooterRenderHeight

					if msg.Y >= footerStartY && msg.Y < m.height { // Click is in footer Y range
						clickedFooterLineIndex := msg.Y - footerStartY
						footerLines := strings.Split(currentFooterStr, "\n")

						if clickedFooterLineIndex >= 0 && clickedFooterLineIndex < len(footerLines) {
							lineTextWithANSI := footerLines[clickedFooterLineIndex]
							plainLineText := xansi.Strip(lineTextWithANSI)
							allBindings := getKeyBindings(m.keymap)
							var simKeyCmd tea.Cmd

							// Convert mouse X to rune index
							clickedRuneIdx := -1
							currentCellPos := 0
							for i, r := range plainLineText {
								runeW := runewidth.RuneWidth(r)
								if msg.X >= currentCellPos && msg.X < currentCellPos+runeW {
									clickedRuneIdx = i
									break
								}
								currentCellPos += runeW
							}
							if clickedRuneIdx == -1 && msg.X == currentCellPos && len(plainLineText) > 0 {
								// Click was at the very end of the line
								clickedRuneIdx = len(plainLineText) - 1
							}

							if clickedRuneIdx != -1 { // Only proceed if click is within text bounds
								var bindingActionFound bool // Flag to break outer loop once an action is decided
								for _, binding := range allBindings {
									if bindingActionFound {
										break // Exit loop over bindings if action already found
									}
									if !binding.Enabled() {
										continue
									}
									helpKeyDisplay := binding.Help().Key
									if helpKeyDisplay == "" {
										continue
									}

									searchStartIdx := 0
									for {
										// Find the key display string (e.g., "enter", "↑/k")
										relativeKeyIdx := strings.Index(plainLineText[searchStartIdx:], helpKeyDisplay)
										if relativeKeyIdx == -1 {
											break // Key display not found further in this line
										}
										absoluteKeyIdx := searchStartIdx + relativeKeyIdx
										keyEndIdx := absoluteKeyIdx + len(helpKeyDisplay)

										// 1. Check if click is on the key display itself
										if clickedRuneIdx >= absoluteKeyIdx && clickedRuneIdx < keyEndIdx {
											simKeyCmd = m.createSimulatedKeyCmd(binding)
											bindingActionFound = true
											break // Action decided, break from inner key display search loop
										}

										// 2. Check if click is on the description part
										// Description usually follows "key: description" or "key: desc |"
										descMarker := ": "
										markerStartIdx := keyEndIdx
										if strings.HasPrefix(plainLineText[markerStartIdx:], descMarker) {
											descTextStartIdx := markerStartIdx + len(descMarker)

											// Find end of description (next " | " or end of line)
											descEndIdx := len(plainLineText)
											nextSeparatorIdx := strings.Index(plainLineText[descTextStartIdx:], " | ")
											if nextSeparatorIdx != -1 {
												descEndIdx = descTextStartIdx + nextSeparatorIdx
											}

											descriptionText := strings.TrimSpace(plainLineText[descTextStartIdx:descEndIdx])

											if clickedRuneIdx >= descTextStartIdx && clickedRuneIdx < descEndIdx {
												// Special handling for "navigate" and "change auth":
												// These footer texts are informational; their actions are tied to specific key presses,
												// not generic click simulation on their help text.
												if descriptionText == "navigate" || descriptionText == "change auth" {
													// Do nothing for these specific descriptions
													simKeyCmd = nil
												} else {
													// For other descriptions, simulate the key press
													simKeyCmd = m.createSimulatedKeyCmd(binding)
												}
												bindingActionFound = true
												break // Action decided (or no action), break from inner key display search loop
											}
										}
										// If not clicked on key or its description, continue search for key display
										searchStartIdx = absoluteKeyIdx + 1
										if searchStartIdx >= len(plainLineText) {
											break
										}
									}
								}
							}
							if simKeyCmd != nil {
								cmds = append(cmds, simKeyCmd)
							}
						}
					}
				}
			}
		}

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
		case stateLoadingStacks:
			// Allow quitting or going back from the loading screen
			switch {
			case key.Matches(msg, m.keymap.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keymap.Back), key.Matches(msg, m.keymap.Esc):
				// Go back to the stack list and stop the discovery indication
				m.currentState = stateStackList
				m.isDiscovering = false
				// Clear any errors that might have occurred during the aborted discovery
				m.lastError = nil
				m.discoveryErrors = nil
				// TODO: Implement cancellation for the background findStacksCmd if possible
			}
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
				// Check if config was modified before deciding where to go/what to do
				if m.sshConfigModified {
					m.sshConfigModified = false // Reset the flag
					// Trigger the full refresh which will change state to loading
					cmds = append(cmds, m.triggerConfigAndStackRefresh())
				} else {
					// No changes, just go back to stack list
					m.currentState = stateStackList
				}
				// Clear errors specific to the config view when leaving
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
	header = titleStyle.Render("Bucket Manager")

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
		footerStr = m.keymap.Quit.Help().Key + ": " + m.keymap.Quit.Help().Desc
	}

	actualHeaderRenderHeight := lipgloss.Height(header) // Should be 1 if titleStyle is single line
	actualFooterRenderHeight := lipgloss.Height(footerStr)
	joinerNewlines := 2 // Newlines added by JoinVertical for 3 elements

	// Calculate the total height available for the main body block (content + border)
	// bodyBlockHeight is the height for the bodyStr component itself.
	bodyBlockHeight := m.height - actualHeaderRenderHeight - actualFooterRenderHeight - joinerNewlines
	if bodyBlockHeight < 0 {
		bodyBlockHeight = 0 // Cannot be negative
	}

	var renderedBodyContent string

	// Check if there's enough space for the border itself
	// Since borderVerticalPadding is effectively 0, this check simplifies.
	// We need at least some height to render anything.
	if bodyBlockHeight < 0 { // Should not happen due to earlier clamp, but defensive.
		// Not enough space for a border, render content directly if possible
		if bodyBlockHeight > 0 {
			bodyStr = lipgloss.PlaceVertical(bodyBlockHeight, lipgloss.Top, bodyContent)
		} else {
			bodyStr = "" // No space for anything
		}
	} else {
		// Enough space for border and potentially content
		contentHeight := bodyBlockHeight // borderVerticalPadding is effectively 0
		if contentHeight < 0 {
			contentHeight = 0 // Content area inside border can be zero
		}

		// Width for content inside the border (accounts for left/right border characters)
		// Assuming border characters take 1 unit of width on each side
		contentWidth := m.width - 2
		if contentWidth < 0 {
			contentWidth = 0
		}

		switch m.currentState {
		case stateStackList, stateRunningSequence, stateSequenceError, stateRunningHostAction:
			m.viewport.Height = contentHeight
			m.viewport.Width = contentWidth
			m.viewport.SetContent(bodyContent)
			renderedBodyContent = m.viewport.View()
		case stateStackDetails:
			m.detailsViewport.Height = contentHeight
			m.detailsViewport.Width = contentWidth
			m.detailsViewport.SetContent(bodyContent)
			renderedBodyContent = m.detailsViewport.View()
		case stateSshConfigList:
			m.sshConfigViewport.Height = contentHeight
			m.sshConfigViewport.Width = contentWidth
			m.sshConfigViewport.SetContent(bodyContent)
			renderedBodyContent = m.sshConfigViewport.View()
		case stateSshConfigAddForm, stateSshConfigEditForm, stateSshConfigImportDetails:
			m.formViewport.Height = contentHeight
			m.formViewport.Width = contentWidth
			m.formViewport.SetContent(bodyContent)
			renderedBodyContent = m.formViewport.View()
		case stateSshConfigImportSelect:
			m.importSelectViewport.Height = contentHeight
			m.importSelectViewport.Width = contentWidth
			m.importSelectViewport.SetContent(bodyContent)
			renderedBodyContent = m.importSelectViewport.View()
		default:
			// For states without a dedicated viewport or simple text content
			if contentHeight > 0 && contentWidth > 0 {
				renderedBodyContent = lipgloss.NewStyle().Width(contentWidth).Render(
					lipgloss.PlaceVertical(contentHeight, lipgloss.Top, bodyContent),
				)
			} else {
				renderedBodyContent = "" // Not enough space for content
			}
		}

		// Apply the border to the rendered body content
		bodyStr = mainContentBorderStyle.
			Height(bodyBlockHeight). // Let lipgloss determine height of bordered box based on content + border
			Render(renderedBodyContent)
	}

	// --- Combine header, body (rendered viewport or placed content), and footer ---
	finalView := lipgloss.JoinVertical(lipgloss.Left, header, bodyStr, footerStr)
	return finalView
}
