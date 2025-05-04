// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Update Handlers ---
// These methods handle key presses and logic for specific UI states.

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
		m.viewport.PageUp() // Use bubble's PageUp
	case key.Matches(msg, m.keymap.PgDown):
		m.cursor += m.viewport.Height
		lastIdx := len(m.stacks) - 1
		if lastIdx >= 0 && m.cursor > lastIdx {
			m.cursor = lastIdx
		}
		cursorMoved = true
		m.viewport.PageDown() // Use bubble's PageDown
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
			cmds = append(cmds, m.runSequenceOnSelection(runner.UpSequence)...)
		case key.Matches(msg, m.keymap.DownAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.DownSequence)...)
		case key.Matches(msg, m.keymap.RefreshAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.RefreshSequence)...)
		case key.Matches(msg, m.keymap.PullAction):
			cmds = append(cmds, m.runSequenceOnSelection(runner.PullSequence)...)
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

func (m *model) handleSshAddFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Define logical focus indices for the Add form
	const (
		nameFocusIndex       = 0
		hostnameFocusIndex   = 1
		userFocusIndex       = 2
		portFocusIndex       = 3
		remoteRootFocusIndex = 4
		authMethodFocusIndex = 5 // The selector itself
		keyPathFocusIndex    = 6 // Logical index for key path input
		passwordFocusIndex   = 7 // Logical index for password input
	)

	// Build the map of currently navigable logical indices based on auth method
	focusMap := []int{nameFocusIndex, hostnameFocusIndex, userFocusIndex, portFocusIndex, remoteRootFocusIndex, authMethodFocusIndex}
	switch m.formAuthMethod {
	case authMethodKey:
		focusMap = append(focusMap, keyPathFocusIndex)
	case authMethodPassword:
		focusMap = append(focusMap, passwordFocusIndex)
	}

	// Find the current position in the focus map
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
			m.formError = nil // Clear error on navigation
		}
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		if currentIndexInMap != -1 {
			nextIndexInMap := (currentIndexInMap - 1 + len(focusMap)) % len(focusMap)
			m.formFocusIndex = focusMap[nextIndexInMap]
			m.formError = nil // Clear error on navigation
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
		// Prevent submitting when focus is on the non-input Auth Method selector
		if m.formFocusIndex == authMethodFocusIndex {
			return cmds // Do nothing
		}
		// Attempt to build and save the host
		m.formError = nil // Clear previous error
		newHost, validationErr := m.buildHostFromForm()
		if validationErr != nil {
			m.formError = validationErr
		} else {
			// Validation passed, attempt to save
			cmds = append(cmds, saveNewSshHostCmd(newHost))
		}
	}

	// --- Update Input Focus Styles ---
	// Blur all inputs first
	for i := range m.formInputs {
		m.formInputs[i].Blur()
		m.formInputs[i].Prompt = "  " // Reset prompt
		m.formInputs[i].TextStyle = lipgloss.NewStyle()
	}

	// Determine the actual textinput.Model index to focus
	focusedInputIndex := -1
	switch m.formFocusIndex {
	case nameFocusIndex, hostnameFocusIndex, userFocusIndex, portFocusIndex, remoteRootFocusIndex:
		focusedInputIndex = m.formFocusIndex // Direct mapping for these
	case keyPathFocusIndex:
		if m.formAuthMethod == authMethodKey {
			focusedInputIndex = 5 // Actual index for KeyPath input
		}
	case passwordFocusIndex:
		if m.formAuthMethod == authMethodPassword {
			focusedInputIndex = 6 // Actual index for Password input
		}
	}

	// Apply focus style to the correct input if one should be focused
	if focusedInputIndex != -1 {
		cmds = append(cmds, m.formInputs[focusedInputIndex].Focus())
		m.formInputs[focusedInputIndex].Prompt = cursorStyle.Render("> ")
		m.formInputs[focusedInputIndex].TextStyle = cursorStyle
	}
	// If focus is on the authMethod selector (m.formFocusIndex == authMethodFocusIndex),
	// no text input gets focus styling, but the selector itself will be styled in View().

	return cmds
}

func (m *model) handleSshEditFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Define logical focus indices for the Edit form
	const (
		nameFocusIndex           = 0
		hostnameFocusIndex       = 1
		userFocusIndex           = 2
		portFocusIndex           = 3
		remoteRootFocusIndex     = 4
		authMethodFocusIndex     = 5 // The selector itself
		keyPathFocusIndex        = 6 // Logical index for key path input
		passwordFocusIndex       = 7 // Logical index for password input
		disabledToggleFocusIndex = 8 // Logical index for the disabled toggle
	)

	// Build the map of currently navigable logical indices based on auth method
	focusMap := []int{nameFocusIndex, hostnameFocusIndex, userFocusIndex, portFocusIndex, remoteRootFocusIndex, authMethodFocusIndex}
	switch m.formAuthMethod {
	case authMethodKey:
		focusMap = append(focusMap, keyPathFocusIndex)
	case authMethodPassword:
		focusMap = append(focusMap, passwordFocusIndex)
	}
	focusMap = append(focusMap, disabledToggleFocusIndex) // Always add the disabled toggle

	// Find the current position in the focus map
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
			m.formError = nil // Clear error on navigation
		}
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		if currentIndexInMap != -1 {
			nextIndexInMap := (currentIndexInMap - 1 + len(focusMap)) % len(focusMap)
			m.formFocusIndex = focusMap[nextIndexInMap]
			m.formError = nil // Clear error on navigation
		}
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
		// Prevent submitting when focus is on non-input selectors
		if m.formFocusIndex == authMethodFocusIndex || m.formFocusIndex == disabledToggleFocusIndex {
			return cmds // Do nothing
		}
		// Attempt to build and save the edited host
		m.formError = nil // Clear previous error
		if m.hostToEdit == nil {
			m.formError = fmt.Errorf("internal error: no host selected for editing")
			return cmds
		}
		editedHost, validationErr := m.buildHostFromEditForm()
		if validationErr != nil {
			m.formError = validationErr
		} else {
			// Validation passed, attempt to save
			cmds = append(cmds, saveEditedSshHostCmd(m.hostToEdit.Name, editedHost))
		}
	}

	// --- Update Input Focus Styles ---
	// Blur all inputs first
	for i := range m.formInputs {
		m.formInputs[i].Blur()
		m.formInputs[i].Prompt = "  " // Reset prompt
		m.formInputs[i].TextStyle = lipgloss.NewStyle()
	}

	// Determine the actual textinput.Model index to focus
	focusedInputIndex := -1
	switch m.formFocusIndex {
	case nameFocusIndex, hostnameFocusIndex, userFocusIndex, portFocusIndex, remoteRootFocusIndex:
		focusedInputIndex = m.formFocusIndex // Direct mapping
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

	// Apply focus style to the correct input if one should be focused
	if focusedInputIndex != -1 {
		cmds = append(cmds, m.formInputs[focusedInputIndex].Focus())
		m.formInputs[focusedInputIndex].Prompt = cursorStyle.Render("> ")
		m.formInputs[focusedInputIndex].TextStyle = cursorStyle
	}
	// If focus is on the authMethod selector or disabled toggle,
	// no text input gets focus styling, but the elements themselves will be styled in View().

	return cmds
}

func (m *model) handleSshImportDetailsFormKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Define logical focus indices for the Import Details form
	const (
		remoteRootFocusIndex    = 0 // Logical index for Remote Root input
		authMethodFocusIndex    = 1 // Logical index for Auth Method selector
		keyOrPasswordFocusIndex = 2 // Logical index for Key Path OR Password input
	)

	// Determine if auth fields are needed based on the potential host
	authNeeded := false
	if m.configuringHostIdx >= 0 && m.configuringHostIdx < len(m.importableHosts) {
		authNeeded = m.importableHosts[m.configuringHostIdx].KeyPath == ""
	}

	// Build the map of currently navigable logical indices
	focusMap := []int{remoteRootFocusIndex} // Remote Root is always present
	if authNeeded {
		focusMap = append(focusMap, authMethodFocusIndex) // Add auth selector if needed
		// Add the specific input based on the selected auth method
		if m.formAuthMethod == authMethodKey || m.formAuthMethod == authMethodPassword {
			focusMap = append(focusMap, keyOrPasswordFocusIndex)
		}
	}

	// Find the current position in the focus map
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
			m.formError = nil // Clear error on navigation
		}
	case key.Matches(msg, m.keymap.ShiftTab), key.Matches(msg, m.keymap.Up):
		if currentIndexInMap != -1 {
			nextIndexInMap := (currentIndexInMap - 1 + len(focusMap)) % len(focusMap)
			m.formFocusIndex = focusMap[nextIndexInMap]
			m.formError = nil // Clear error on navigation
		}
	case key.Matches(msg, m.keymap.Left), key.Matches(msg, m.keymap.Right):
		// Handle auth method switching only if auth is needed and the selector is focused
		if authNeeded && m.formFocusIndex == authMethodFocusIndex {
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
		// Prevent submitting when focus is on the non-input Auth Method selector
		if authNeeded && m.formFocusIndex == authMethodFocusIndex {
			return cmds // Do nothing
		}

		// --- Process the current host's details ---
		m.formError = nil // Clear previous error

		if m.configuringHostIdx < 0 || m.configuringHostIdx >= len(m.importableHosts) {
			m.formError = fmt.Errorf("internal error: invalid host index for import details")
			return cmds
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
			return cmds
		}

		// Apply auth details only if they were needed (i.e., not provided in ssh_config)
		if authNeeded {
			switch m.formAuthMethod {
			case authMethodKey:
				if keyPath == "" {
					m.formError = fmt.Errorf("key path is required for Key File authentication")
					return cmds
				}
				hostToSave.KeyPath = keyPath
				hostToSave.Password = "" // Ensure password is clear
			case authMethodPassword:
				if password == "" {
					m.formError = fmt.Errorf("password is required for Password authentication")
					return cmds
				}
				hostToSave.Password = password
				hostToSave.KeyPath = "" // Ensure key path is clear
			case authMethodAgent:
				// Agent selected, ensure both are clear
				hostToSave.KeyPath = ""
				hostToSave.Password = ""
			}
		} // else: Keep the KeyPath from ssh_config (already set by ConvertToBucketManagerHost)

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
		} else {
			// No more selected hosts left, trigger the save command
			cmds = append(cmds, saveImportedSshHostsCmd(m.hostsToConfigure))
			// State change will happen upon receiving sshHostsImportedMsg
		}
	}

	// --- Update Input Focus Styles ---
	// Define actual indices in m.formInputs
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
	focusedInputIndex := -1
	switch m.formFocusIndex {
	case remoteRootFocusIndex:
		focusedInputIndex = remoteRootInputIdx
	case keyOrPasswordFocusIndex:
		// Only focus if auth is needed
		if authNeeded {
			switch m.formAuthMethod {
			case authMethodKey:
				focusedInputIndex = keyPathInputIdx
			case authMethodPassword:
				focusedInputIndex = passwordInputIdx
			}
		}
		// No input focus for authMethodFocusIndex
	}

	// Apply focus style if an input should be focused
	if focusedInputIndex != -1 {
		cmds = append(cmds, m.formInputs[focusedInputIndex].Focus())
		m.formInputs[focusedInputIndex].Prompt = cursorStyle.Render("> ")
		m.formInputs[focusedInputIndex].TextStyle = cursorStyle
	}

	return cmds
}

// runSequenceOnSelection determines which stacks to run a sequence on (selected or cursor)
// and initiates the sequence execution.
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

// startNextStepCmd prepares and returns the command to run the next step in the current sequence.
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
