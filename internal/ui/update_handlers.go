// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package ui's update_handlers.go file implements state-specific keyboard and input handling
// for the TUI. It contains methods that process user interactions like key presses, menu
// selections, and form submissions for each different view state of the application.

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Update Handlers ---
// These methods handle key presses and input logic for specific UI states.
// Each method corresponds to a different UI state and manages the state transitions,
// keyboard shortcuts, and user interactions appropriate for that view.

// handleStackListKeys processes keyboard input when in the main stack list view.
// It handles navigation through the stack list, selection of stacks for batch operations,
// triggering stack commands (up, down, pull), and switching to other views.
//
// Key handlers include:
// - Up/Down/Home/End: Navigation through the stack list
// - Space: Select/deselect a stack for batch operations
// - Enter: View detailed information about the selected stack
// - u/d/r/p: Shortcut keys for stack operations (up/down/refresh/pull)
// - c: Switch to SSH configuration view
// - q/Ctrl+C: Quit the application
//
// Parameters:
//   - msg: The keyboard message containing the pressed key
//
// Returns:
//   - []tea.Cmd: Commands to be executed by the Bubble Tea framework
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
		// Update viewport regardless of cursor move to handle scrolling at boundaries
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	case key.Matches(msg, m.keymap.Down):
		if m.cursor < len(m.stacks)-1 {
			m.cursor++
			cursorMoved = true
		}
		// Update viewport regardless of cursor move
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
		// Handle actions that don't involve cursor movement first
		switch {
		case key.Matches(msg, m.keymap.Select):
			if len(m.stacks) > 0 && m.cursor >= 0 && m.cursor < len(m.stacks) {
				if _, ok := m.selectedStackIdxs[m.cursor]; ok {
					delete(m.selectedStackIdxs, m.cursor)
				} else {
					m.selectedStackIdxs[m.cursor] = struct{}{}
				}
			}
		case key.Matches(msg, m.keymap.UpAction):
			cmds = slices.Concat(cmds, m.runSequenceOnSelection(runner.UpSequence))
		case key.Matches(msg, m.keymap.DownAction):
			cmds = slices.Concat(cmds, m.runSequenceOnSelection(runner.DownSequence))
		case key.Matches(msg, m.keymap.RefreshAction):
			cmds = slices.Concat(cmds, m.runSequenceOnSelection(runner.RefreshSequence))
		case key.Matches(msg, m.keymap.PullAction):
			cmds = slices.Concat(cmds, m.runSequenceOnSelection(runner.PullSequence))
		case key.Matches(msg, m.keymap.Enter):
			if len(m.selectedStackIdxs) > 0 {
				// Show details for multiple selected stacks
				m.stacksInSequence = []*discovery.Stack{} // Use this field to store stacks for detail view
				m.detailedStack = nil                     // Clear single detailed stack
				for idx := range m.selectedStackIdxs {
					if idx >= 0 && idx < len(m.stacks) {
						stack := m.stacks[idx] // Get a copy
						m.stacksInSequence = append(m.stacksInSequence, &stack)
						// Fetch status if not already loaded/loading
						stackID := stack.Identifier()
						if _, loaded := m.stackStatuses[stackID]; !loaded && !m.loadingStatus[stackID] {
							m.loadingStatus[stackID] = true
							cmds = append(cmds, m.fetchStackStatusCmd(stack))
						}
					}
				}
				m.selectedStackIdxs = make(map[int]struct{}) // Clear selection
				m.currentState = stateStackDetails
				m.detailsViewport.GotoTop()
			} else if len(m.stacks) > 0 && m.cursor >= 0 && m.cursor < len(m.stacks) {
				// Show details for the single stack under the cursor
				stack := m.stacks[m.cursor] // Get a copy
				m.detailedStack = &stack
				m.stacksInSequence = nil // Clear multi-stack selection
				m.currentState = stateStackDetails
				m.detailsViewport.GotoTop()
				// Fetch status if not already loaded/loading
				stackID := m.detailedStack.Identifier()
				if _, loaded := m.stackStatuses[stackID]; !loaded && !m.loadingStatus[stackID] {
					m.loadingStatus[stackID] = true
					cmds = append(cmds, m.fetchStackStatusCmd(*m.detailedStack))
				}
			}
		}
	}

	// If the cursor moved, fetch status for the newly highlighted stack if needed
	if cursorMoved && len(m.stacks) > 0 && m.cursor >= 0 && m.cursor < len(m.stacks) {
		selectedStack := m.stacks[m.cursor]
		stackID := selectedStack.Identifier()
		if _, loaded := m.stackStatuses[stackID]; !loaded && !m.loadingStatus[stackID] {
			m.loadingStatus[stackID] = true
			cmds = append(cmds, m.fetchStackStatusCmd(selectedStack))
		}
	}

	return cmds
}

// --- Form Navigation and Styling Helpers ---

// handleFormNavigation manages keyboard-based navigation between form fields
// in any form view. It supports moving focus up, down, or with Tab/Shift+Tab.
//
// The function uses a focus map which specifies the logical order and accessibility
// of form fields. This allows for complex layouts where not all fields might be
// visible or enabled based on the current form state.
//
// Parameters:
//   - msg: The keyboard message containing the pressed key
//   - focusMap: An ordered slice of logical field indices that are currently accessible
//
// Returns:
//   - bool: True if navigation was handled (focus changed), false otherwise
func (m *model) handleFormNavigation(msg tea.KeyMsg, focusMap []int) bool {
	currentIndexInMap := -1
	for i, logicalIndex := range focusMap {
		if logicalIndex == m.formFocusIndex {
			currentIndexInMap = i
			break
		}
	}
	if currentIndexInMap == -1 {
		return false // Current focus not in the map, shouldn't happen
	}

	navigated := false
	switch {
	case key.Matches(msg, m.keymap.Tab), key.Matches(msg, m.keymap.Down):
		nextIndexInMap := (currentIndexInMap + 1) % len(focusMap)
		m.formFocusIndex = focusMap[nextIndexInMap]
		navigated = true
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		nextIndexInMap := (currentIndexInMap - 1 + len(focusMap)) % len(focusMap)
		m.formFocusIndex = focusMap[nextIndexInMap]
		navigated = true
	}

	if navigated {
		m.formError = nil // Clear error on navigation
	}
	return navigated
}

// updateFormFocusStyles applies focus styling to the correct input field based on logical focus index.
// Returns the tea.Cmd needed to actually focus the text input.
func (m *model) updateFormFocusStyles() tea.Cmd {
	var focusCmd tea.Cmd

	// Define logical focus indices (constants could be defined globally if preferred)
	const (
		nameFocusIndex           = 0
		hostnameFocusIndex       = 1
		userFocusIndex           = 2
		portFocusIndex           = 3
		remoteRootFocusIndex     = 4
		authMethodFocusIndex     = 5 // The selector itself
		keyPathFocusIndex        = 6 // Logical index for key path input
		passwordFocusIndex       = 7 // Logical index for password input
		disabledToggleFocusIndex = 8 // Logical index for the disabled toggle (Edit form)
	)

	// Map logical focus index to actual m.formInputs index
	// This needs to know the current state to determine the correct mapping
	focusedInputIndex := -1 // Index within m.formInputs
	switch m.currentState {
	case stateSshConfigAddForm, stateSshConfigEditForm:
		switch m.formFocusIndex {
		case nameFocusIndex, hostnameFocusIndex, userFocusIndex, portFocusIndex, remoteRootFocusIndex:
			focusedInputIndex = m.formFocusIndex // Direct mapping for 0-4
		case keyPathFocusIndex:
			if m.formAuthMethod == authMethodKey {
				focusedInputIndex = 5 // Actual index for KeyPath input
			}
		case passwordFocusIndex:
			if m.formAuthMethod == authMethodPassword {
				focusedInputIndex = 6 // Actual index for Password input
			}
			// No input focus for authMethodFocusIndex or disabledToggleFocusIndex
		}
	case stateSshConfigImportDetails:
		const (
			impRemoteRootFocusIndex    = 0
			impAuthMethodFocusIndex    = 1
			impKeyOrPasswordFocusIndex = 2
		)
		switch m.formFocusIndex {
		case impRemoteRootFocusIndex:
			focusedInputIndex = 4 // Actual index for Remote Root
		case impKeyOrPasswordFocusIndex:
			switch m.formAuthMethod {
			case authMethodKey:
				focusedInputIndex = 5 // Actual index for Key Path
			case authMethodPassword:
				focusedInputIndex = 6 // Actual index for Password
			}
		}
	}

	// Blur all inputs first
	for i := range m.formInputs {
		if m.formInputs[i].Focused() { // Only blur if actually focused
			m.formInputs[i].Blur()
		}
		m.formInputs[i].Prompt = "  " // Reset prompt
		m.formInputs[i].TextStyle = lipgloss.NewStyle()
	}

	// Apply focus style to the correct input if one should be focused
	if focusedInputIndex != -1 && focusedInputIndex < len(m.formInputs) {
		focusCmd = m.formInputs[focusedInputIndex].Focus()
		m.formInputs[focusedInputIndex].Prompt = cursorStyle.Render("> ")
		m.formInputs[focusedInputIndex].TextStyle = cursorStyle
	}
	// If focus is on a non-input element (auth selector, disabled toggle),
	// no text input gets focus styling, but the element itself will be styled in View().

	return focusCmd
}

// handleSubmitAddForm validates and attempts to save a new SSH host.
func (m *model) handleSubmitAddForm() tea.Cmd {
	// Prevent submitting when focus is on the non-input Auth Method selector
	if m.formFocusIndex == 5 { // 5 is authMethodFocusIndex for Add form
		return nil
	}
	m.formError = nil // Clear previous error
	newHost, validationErr := m.buildHostFromForm()
	if validationErr != nil {
		m.formError = validationErr
		return nil
	}
	// Validation passed, attempt to save
	return saveNewSshHostCmd(newHost)
}

func (m *model) handleSshAddFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Define logical focus indices and build navigable map
	const (
		nameFocusIndex       = 0
		hostnameFocusIndex   = 1
		userFocusIndex       = 2
		portFocusIndex       = 3
		remoteRootFocusIndex = 4
		authMethodFocusIndex = 5
		keyPathFocusIndex    = 6
		passwordFocusIndex   = 7
	)
	focusMap := []int{nameFocusIndex, hostnameFocusIndex, userFocusIndex, portFocusIndex, remoteRootFocusIndex, authMethodFocusIndex}
	switch m.formAuthMethod {
	case authMethodKey:
		focusMap = append(focusMap, keyPathFocusIndex)
	case authMethodPassword:
		focusMap = append(focusMap, passwordFocusIndex)
	}

	// Handle navigation first
	navigated := m.handleFormNavigation(msg, focusMap)

	// Handle other keys if navigation didn't occur
	if !navigated {
		switch {
		case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
			// Handle auth method switching only when the selector is focused
			if m.formFocusIndex == authMethodFocusIndex {
				if key.Matches(msg, m.keymap.Left) {
					m.formAuthMethod--
					if m.formAuthMethod < authMethodKey {
						m.formAuthMethod = authMethodPassword // Wrap around
					}
				} else { // Right key
					m.formAuthMethod++
					if m.formAuthMethod > authMethodPassword {
						m.formAuthMethod = authMethodKey // Wrap around
					}
				}
				m.formError = nil // Clear error when changing auth method
			}
		case key.Matches(msg, m.keymap.Enter):
			cmds = append(cmds, m.handleSubmitAddForm())
		}
	}

	// Update focus styles after handling keys
	cmds = append(cmds, m.updateFormFocusStyles())

	return cmds
}

// handleSubmitEditForm validates and attempts to save an edited SSH host.
func (m *model) handleSubmitEditForm() tea.Cmd {
	// Define logical focus indices (constants could be defined globally if preferred)
	const (
		authMethodFocusIndex     = 5
		disabledToggleFocusIndex = 8
	)
	// Prevent submitting when focus is on non-input selectors
	if m.formFocusIndex == authMethodFocusIndex || m.formFocusIndex == disabledToggleFocusIndex {
		return nil
	}

	m.formError = nil // Clear previous error
	if m.hostToEdit == nil {
		m.formError = fmt.Errorf("internal error: no host selected for editing")
		return nil
	}
	editedHost, validationErr := m.buildHostFromEditForm()
	if validationErr != nil {
		m.formError = validationErr
		return nil
	}
	// Validation passed, attempt to save
	return saveEditedSshHostCmd(m.hostToEdit.Name, editedHost)
}

func (m *model) handleSshEditFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Define logical focus indices and build navigable map
	const (
		nameFocusIndex           = 0
		hostnameFocusIndex       = 1
		userFocusIndex           = 2
		portFocusIndex           = 3
		remoteRootFocusIndex     = 4
		authMethodFocusIndex     = 5
		keyPathFocusIndex        = 6
		passwordFocusIndex       = 7
		disabledToggleFocusIndex = 8
	)
	focusMap := []int{nameFocusIndex, hostnameFocusIndex, userFocusIndex, portFocusIndex, remoteRootFocusIndex, authMethodFocusIndex}
	switch m.formAuthMethod {
	case authMethodKey:
		focusMap = append(focusMap, keyPathFocusIndex)
	case authMethodPassword:
		focusMap = append(focusMap, passwordFocusIndex)
	}
	focusMap = append(focusMap, disabledToggleFocusIndex) // Always add the disabled toggle

	// Handle navigation first
	navigated := m.handleFormNavigation(msg, focusMap)

	// Handle other keys if navigation didn't occur
	if !navigated {
		switch {
		case key.Matches(msg, m.keymap.ToggleDisabled): // Spacebar
			// Toggle disabled status only when the toggle itself is focused
			if m.formFocusIndex == disabledToggleFocusIndex {
				m.formDisabled = !m.formDisabled
			}
		case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
			// Handle auth method switching only when the selector is focused
			if m.formFocusIndex == authMethodFocusIndex {
				if key.Matches(msg, m.keymap.Left) {
					m.formAuthMethod--
					if m.formAuthMethod < authMethodKey {
						m.formAuthMethod = authMethodPassword // Wrap around
					}
				} else { // Right key
					m.formAuthMethod++
					if m.formAuthMethod > authMethodPassword {
						m.formAuthMethod = authMethodKey // Wrap around
					}
				}
				m.formError = nil // Clear error when changing auth method
			}
		case key.Matches(msg, m.keymap.Enter):
			cmds = append(cmds, m.handleSubmitEditForm())
		}
	}

	// Update focus styles after handling keys
	cmds = append(cmds, m.updateFormFocusStyles())

	return cmds
}

// handleSubmitImportDetailsForm processes the details for the currently configuring host,
// moves to the next host, or triggers the final save.
func (m *model) handleSubmitImportDetailsForm() tea.Cmd {
	// Define logical focus indices
	const (
		remoteRootFocusIndex = 0
		authMethodFocusIndex = 1
	)
	// Prevent submitting when focus is on the non-input Auth Method selector
	if m.formFocusIndex == authMethodFocusIndex {
		return nil
	}

	m.formError = nil // Clear previous error

	if m.configuringHostIdx < 0 || m.configuringHostIdx >= len(m.importableHosts) {
		m.formError = fmt.Errorf("internal error: invalid host index for import details")
		return nil
	}
	currentPotentialHost := m.importableHosts[m.configuringHostIdx]

	// Extract values from the relevant form inputs
	remoteRoot := strings.TrimSpace(m.formInputs[4].Value()) // Index 4 is Remote Root
	keyPath := strings.TrimSpace(m.formInputs[5].Value())    // Index 5 is Key Path
	password := m.formInputs[6].Value()                      // Index 6 is Password

	// Convert potential host to our config format
	hostToSave, convertErr := config.ConvertToBucketManagerHost(currentPotentialHost, currentPotentialHost.Alias, remoteRoot)
	if convertErr != nil {
		m.formError = fmt.Errorf("internal conversion error: %w", convertErr)
		return nil
	}

	// Always apply auth details based on the form selection, overriding ssh_config if needed.
	switch m.formAuthMethod {
	case authMethodKey:
		if keyPath == "" {
			m.formError = fmt.Errorf("key path is required for Key File authentication")
			return nil
		}
		hostToSave.KeyPath = keyPath
		hostToSave.Password = "" // Ensure password is clear
	case authMethodPassword:
		if password == "" {
			m.formError = fmt.Errorf("password is required for Password authentication")
			return nil
		}
		hostToSave.Password = password
		hostToSave.KeyPath = "" // Ensure key path is clear
	case authMethodAgent:
		// Agent selected, ensure both are clear, overriding any ssh_config key path
		hostToSave.KeyPath = ""
		hostToSave.Password = ""
	}

	// Add the configured host to the list to be saved
	m.hostsToConfigure = append(m.hostsToConfigure, hostToSave)

	// --- Move to the next selected host or finish ---
	nextSelectedIdx := -1
	// Start searching from the index *after* the current one
	for i := m.configuringHostIdx + 1; i < len(m.importableHosts); i++ {
		if _, ok := m.selectedImportIdxs[i]; ok {
			nextSelectedIdx = i
			break // Found the next one
		}
	}

	if nextSelectedIdx != -1 {
		// Move to the next host: update index, create new form, reset focus
		m.configuringHostIdx = nextSelectedIdx
		pHostToConfigure := m.importableHosts[m.configuringHostIdx]
		m.formInputs, m.formAuthMethod = createImportDetailsForm(pHostToConfigure)
		m.formFocusIndex = remoteRootFocusIndex // Reset focus to the first field (Remote Root)
		m.formError = nil
		m.formViewport.GotoTop() // Scroll form viewport to top for the new host
		// Return command to focus the first input of the new form
		return m.updateFormFocusStyles()
	}

	// No more selected hosts left, trigger the save command
	return saveImportedSshHostsCmd(m.hostsToConfigure)
	// State change will happen upon receiving sshHostsImportedMsg
}

func (m *model) handleSshImportDetailsFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Define logical focus indices and build navigable map
	const (
		remoteRootFocusIndex    = 0
		authMethodFocusIndex    = 1
		keyOrPasswordFocusIndex = 2
	)
	focusMap := []int{remoteRootFocusIndex, authMethodFocusIndex}
	switch m.formAuthMethod {
	case authMethodKey, authMethodPassword:
		focusMap = append(focusMap, keyOrPasswordFocusIndex)
	}

	// Handle navigation first
	navigated := m.handleFormNavigation(msg, focusMap)

	// Handle other keys if navigation didn't occur
	if !navigated {
		switch {
		case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
			// Handle auth method switching only when the selector is focused
			if m.formFocusIndex == authMethodFocusIndex {
				if key.Matches(msg, m.keymap.Left) {
					m.formAuthMethod--
					if m.formAuthMethod < authMethodKey {
						m.formAuthMethod = authMethodPassword // Wrap around
					}
				} else { // Right key
					m.formAuthMethod++
					if m.formAuthMethod > authMethodPassword {
						m.formAuthMethod = authMethodKey // Wrap around
					}
				}
				m.formError = nil // Clear error when changing auth method
			}
		case key.Matches(msg, m.keymap.Enter):
			cmds = append(cmds, m.handleSubmitImportDetailsForm())
		}
	}

	// Update focus styles after handling keys
	cmds = append(cmds, m.updateFormFocusStyles())

	return cmds
}

// runSequenceOnSelection prepares and initiates the execution of command sequences
// on stacks. It handles both single-stack operations (using the cursor position) and
// batch operations on multiple stacks (using the selection map).
//
// This function:
// 1. Determines which stacks to operate on (selected or just cursor position)
// 2. Creates a combined command sequence by applying the provided function to each stack
// 3. Prepares the UI for sequence execution and output display
// 4. Initiates the execution of the first command in the sequence
//
// Parameters:
//   - sequenceFunc: A function that generates the appropriate command steps for a given stack
//
// Returns:
//   - []tea.Cmd: Commands to be executed by the Bubble Tea framework
func (m *model) runSequenceOnSelection(sequenceFunc func(discovery.Stack) []runner.CommandStep) []tea.Cmd {
	var cmds []tea.Cmd
	var stacksToRun []*discovery.Stack
	var combinedSequence []runner.CommandStep
	m.stacksInSequence = nil // Reset the list of stacks involved in the current sequence

	// Determine target stacks: either selected or the one under the cursor
	if len(m.selectedStackIdxs) > 0 {
		for idx := range m.selectedStackIdxs {
			if idx >= 0 && idx < len(m.stacks) {
				stacksToRun = append(stacksToRun, &m.stacks[idx]) // Add pointer to the stack
			}
		}
		m.selectedStackIdxs = make(map[int]struct{}) // Clear selection after use
	} else if len(m.stacks) > 0 && m.cursor >= 0 && m.cursor < len(m.stacks) {
		// If no selection, use the stack under the cursor
		stacksToRun = append(stacksToRun, &m.stacks[m.cursor])
	}

	// If no valid stacks were targeted, do nothing
	if len(stacksToRun) == 0 {
		return cmds
	}

	// Store the stacks involved and build the combined command sequence
	m.stacksInSequence = stacksToRun
	for _, stackPtr := range stacksToRun {
		if stackPtr != nil {
			// Generate the sequence steps for the current stack and concatenate
			combinedSequence = slices.Concat(combinedSequence, sequenceFunc(*stackPtr))
		}
	}

	// If any commands were generated, start the sequence
	if len(combinedSequence) > 0 {
		// Set the primary stack for display (usually the first one)
		if len(stacksToRun) > 0 && stacksToRun[0] != nil {
			m.sequenceStack = stacksToRun[0]
		} else {
			m.sequenceStack = nil // Should not happen if combinedSequence is not empty
		}
		// Update model state for running a sequence
		m.currentSequence = combinedSequence
		m.currentState = stateRunningSequence
		m.currentStepIndex = 0
		m.outputContent = "" // Clear previous output
		m.lastError = nil    // Clear previous error
		m.viewport.GotoTop() // Scroll output viewport to top
		// Start the first step
		cmds = append(cmds, m.startNextStepCmd())
	}

	return cmds
}

// startNextStepCmd creates a command that will execute the next step in the
// current command sequence. It handles sequential execution of multi-step
// operations like starting, stopping, or pulling stacks.
//
// This function:
// 1. Validates that a sequence exists and has remaining steps
// 2. Retrieves the next step to execute
// 3. Creates a command that will run the step and update the UI with output
// 4. Manages sequence progress tracking
//
// The command created will update the output content with execution results
// and will trigger subsequent steps when completed successfully.
//
// Returns:
//   - tea.Cmd: A command that executes the next step in the sequence
func (m *model) startNextStepCmd() tea.Cmd {
	// Ensure there is a sequence and the index is valid
	if m.currentSequence == nil || m.currentStepIndex >= len(m.currentSequence) {
		return nil // No more steps or no sequence active
	}
	// Get the current step
	step := m.currentSequence[m.currentStepIndex]
	// Add a header to the output indicating the step start
	m.outputContent += stepStyle.Render(fmt.Sprintf("\n--- Starting Step: %s for %s ---", step.Name, step.Stack.Identifier())) + "\n"
	// Update the viewport content and scroll to bottom
	m.viewport.SetContent(m.outputContent)
	m.viewport.GotoBottom()
	// Return the command to execute the step
	return runStepCmd(step)
}

// handleViewportKeys handles key presses when the main output viewport is active (e.g., during sequence execution).
// handleViewportKeys processes keyboard input for views that use a viewport
// for scrolling content, such as the sequence output view and stack details view.
// It handles both scrolling controls and navigation to other views.
//
// This function handles:
// - Up/Down/PgUp/PgDown: Scrolling the viewport content
// - Enter/Esc/Back: Returning to the previous view
// - q/Ctrl+C: Quitting the application
//
// Parameters:
//   - msg: The keyboard message containing the pressed key
//
// Returns:
//   - tea.Model: The updated model
//   - tea.Cmd: Command to be executed by the Bubble Tea framework
func (m *model) handleViewportKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	switch {
	case key.Matches(msg, m.keymap.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keymap.Back), key.Matches(msg, m.keymap.Enter):
		// Return to stack list and refresh statuses
		for _, stack := range m.stacksInSequence {
			if stack != nil {
				stackID := stack.Identifier()
				// Check if status is not already loading or loaded to avoid redundant fetches
				if !m.loadingStatus[stackID] {
					if _, loaded := m.stackStatuses[stackID]; !loaded {
						m.loadingStatus[stackID] = true
						cmds = append(cmds, m.fetchStackStatusCmd(*stack))
					}
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
		return m, tea.Batch(cmds...) // Return immediately after state change and commands
	}

	// Default: Update the viewport for scrolling etc.
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...) // Return model and any viewport commands
}

// handleFormInputUpdates processes keyboard input for text input fields in forms.
// It maps the logical focus index to the actual input field index and routes
// input to the appropriate field.
//
// This function:
// 1. Determines which input field is currently focused based on the application state
// 2. Updates the focused input field with the keyboard input
// 3. Handles special inputs like Enter for form submission
//
// The function supports different form layouts across various application states
// (SSH host add/edit, import configuration, etc.) and maps the logical focus index
// to the appropriate physical input field.
//
// Parameters:
//   - msg: The keyboard message containing the pressed key
//
// Returns:
//   - tea.Cmd: Command to be executed by the Bubble Tea framework
func (m *model) handleFormInputUpdates(msg tea.KeyMsg) tea.Cmd {
	var cmds []tea.Cmd
	focusedInputIndex := -1 // Index within m.formInputs

	// Determine which input field has logical focus based on the current state and formFocusIndex
	switch m.currentState {
	case stateSshConfigAddForm:
		switch m.formFocusIndex {
		case 0, 1, 2, 3, 4: // Name, Hostname, User, Port, RemoteRoot
			focusedInputIndex = m.formFocusIndex
		case 6: // Key Path (only focusable if authMethodKey)
			if m.formAuthMethod == authMethodKey {
				focusedInputIndex = 5 // Actual index in m.formInputs
			}
		case 7: // Password (only focusable if authMethodPassword)
			if m.formAuthMethod == authMethodPassword {
				focusedInputIndex = 6 // Actual index in m.formInputs
			}
		}
	case stateSshConfigEditForm:
		switch m.formFocusIndex {
		case 0, 1, 2, 3, 4: // Name, Hostname, User, Port, RemoteRoot
			focusedInputIndex = m.formFocusIndex
		case 6: // Key Path (only focusable if authMethodKey)
			if m.formAuthMethod == authMethodKey {
				focusedInputIndex = 5 // Actual index in m.formInputs
			}
		case 7: // Password (only focusable if authMethodPassword)
			if m.formAuthMethod == authMethodPassword {
				focusedInputIndex = 6 // Actual index in m.formInputs
			}
		}
		// Note: Disabled toggle (index 8) doesn't have a text input
	case stateSshConfigImportDetails:
		const (
			remoteRootFocusIndex = 0
			// authMethodFocusIndex = 1 (no text input)
			keyOrPasswordFocusIndex = 2
		)
		authNeeded := false
		if m.configuringHostIdx >= 0 && m.configuringHostIdx < len(m.importableHosts) {
			// Check if the *original* potential host needed auth details
			authNeeded = m.importableHosts[m.configuringHostIdx].KeyPath == ""
		}

		switch m.formFocusIndex {
		case remoteRootFocusIndex:
			focusedInputIndex = 4 // Actual index in m.formInputs for Remote Root
		case keyOrPasswordFocusIndex:
			if authNeeded { // Only allow input if auth was needed initially
				switch m.formAuthMethod {
				case authMethodKey:
					focusedInputIndex = 5 // Actual index for Key Path
				case authMethodPassword:
					focusedInputIndex = 6 // Actual index for Password
				}
			}
		}
	}

	// If a valid text input is focused, update it
	if focusedInputIndex != -1 && focusedInputIndex < len(m.formInputs) {
		var inputCmd tea.Cmd
		// Create a temporary variable for the update result
		var updatedInput textinput.Model
		updatedInput, inputCmd = m.formInputs[focusedInputIndex].Update(msg)
		// Assign the updated input back to the slice
		m.formInputs[focusedInputIndex] = updatedInput
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}
	}

	return tea.Batch(cmds...)
}
