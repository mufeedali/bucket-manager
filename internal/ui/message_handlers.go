// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/runner"
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// --- Message Handlers ---
// These methods handle specific message types received by the model's Update function.

func handleWindowSizeMsg(m *model, msg tea.WindowSizeMsg) tea.Cmd {
	m.width = msg.Width
	m.height = msg.Height

	// Ensure viewports are initialized or resized correctly
	// This logic seems fine here, but could be a helper if it grows
	if !m.ready {
		// Initialize viewports if not ready
		// Assuming viewport.New creates viewports with default settings
		// We might need to adjust this if specific configurations are needed at init
		m.viewport = viewport.New(m.width, 1) // Placeholder height, adjust as needed
		m.sshConfigViewport = viewport.New(m.width, 1)
		m.detailsViewport = viewport.New(m.width, 1)
		m.formViewport = viewport.New(m.width, 1)
		m.importSelectViewport = viewport.New(m.width, 1)
		m.ready = true
	} else {
		// Resize existing viewports
		m.viewport.Width = m.width
		m.sshConfigViewport.Width = m.width
		m.detailsViewport.Width = m.width
		m.formViewport.Width = m.width
		m.importSelectViewport.Width = m.width
		// Note: Height is often set dynamically in the View() method based on available space
	}
	return nil
}

func handleSshConfigParsedMsg(m *model, msg sshConfigParsedMsg) tea.Cmd {
	if msg.err != nil {
		m.lastError = fmt.Errorf("failed to parse ssh config: %w", msg.err)
		m.currentState = stateSshConfigList // Go back to list on error
		return loadSshConfigCmd()           // Reload potentially partially loaded config
	}

	cfg, loadErr := config.LoadConfig()
	if loadErr != nil {
		m.lastError = fmt.Errorf("failed to load current config for import filtering: %w", loadErr)
		m.currentState = stateSshConfigList
		return loadSshConfigCmd()
	}

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
		return loadSshConfigCmd() // Still need to reload config state
	}

	// Transition to import selection state
	m.currentState = stateSshConfigImportSelect
	m.importCursor = 0
	m.selectedImportIdxs = make(map[int]struct{})
	m.importError = nil
	m.lastError = nil
	m.importSelectViewport.GotoTop() // Ensure viewport starts at the top

	return nil
}

func handleSshHostsImportedMsg(m *model, msg sshHostsImportedMsg) tea.Cmd {
	m.currentState = stateSshConfigList // Always return to list after import attempt
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
	m.currentState = stateLoadingStacks // Show loading while rediscovering
	m.isDiscovering = true
	m.stacks = nil
	m.discoveryErrors = nil
	m.stackStatuses = make(map[string]runner.StackRuntimeInfo)
	m.loadingStatus = make(map[string]bool)
	m.cursor = 0
	// Return commands to reload config AND rediscover stacks
	return tea.Batch(loadSshConfigCmd(), findStacksCmd())
}

func handleStackDiscoveredMsg(m *model, msg stackDiscoveredMsg) tea.Cmd {
	// If we were in the initial loading state, transition to the list view
	if m.currentState == stateLoadingStacks {
		m.currentState = stateStackList
	}

	// Add the discovered stack
	m.stacks = append(m.stacks, msg.stack)

	// Fetch status for the newly discovered stack if not already loading/loaded
	stackID := msg.stack.Identifier()
	if !m.loadingStatus[stackID] {
		if _, loaded := m.stackStatuses[stackID]; !loaded {
			m.loadingStatus[stackID] = true
			return m.fetchStackStatusCmd(msg.stack) // Return command to fetch status
		}
	}
	return nil // No command needed if status is already loading or loaded
}

func handleDiscoveryErrorMsg(m *model, msg discoveryErrorMsg) tea.Cmd {
	m.discoveryErrors = append(m.discoveryErrors, msg.err)
	// Optionally update lastError to show the most recent discovery error
	m.lastError = msg.err
	// Potentially transition state if needed, but often just collecting errors is fine
	// If still loading, remain in loading state.
	return nil
}

func handleDiscoveryFinishedMsg(m *model) tea.Cmd {
	m.isDiscovering = false // Mark discovery as finished

	// If we were loading stacks, transition to the list state now.
	if m.currentState == stateLoadingStacks {
		m.currentState = stateStackList
		// Set error/info message based on discovery results
		if len(m.stacks) == 0 {
			if len(m.discoveryErrors) == 0 {
				m.lastError = fmt.Errorf("no stacks found")
			} else {
				// Combine errors or show a generic message
				m.lastError = fmt.Errorf("discovery finished with %d errors, no stacks found", len(m.discoveryErrors))
			}
		} else {
			// Stacks found, clear general 'no stacks' error, but keep discovery errors if any
			m.lastError = nil
			if len(m.discoveryErrors) > 0 {
				// Maybe set an info message instead of lastError?
				// m.infoMessage = fmt.Sprintf("Discovery finished with %d errors.", len(m.discoveryErrors))
				m.lastError = fmt.Errorf("discovery finished with errors") // Keep as error for now
			}
		}
		m.viewport.GotoTop() // Reset viewport scroll for the list
	}
	// If discovery finished while already in another state (e.g., config), just update isDiscovering flag.
	return nil
}

func handleSshConfigLoadedMsg(m *model, msg sshConfigLoadedMsg) tea.Cmd {
	if msg.Err != nil {
		m.lastError = fmt.Errorf("failed to load ssh config: %w", msg.Err)
		m.configuredHosts = []config.SSHHost{} // Clear hosts on error
	} else {
		m.configuredHosts = msg.hosts
		m.lastError = nil // Clear error on success
	}

	// Ensure cursor stays within bounds after loading/reloading config
	totalItems := len(m.configuredHosts) + 1 // +1 for "local"
	if m.configCursor >= totalItems {
		m.configCursor = max(0, totalItems-1)
	}
	m.sshConfigViewport.GotoTop() // Reset scroll on config load/reload
	return nil
}

func handleStackStatusLoadedMsg(m *model, msg stackStatusLoadedMsg) tea.Cmd {
	m.loadingStatus[msg.stackIdentifier] = false // Mark as no longer loading
	m.stackStatuses[msg.stackIdentifier] = msg.statusInfo
	// No state transition needed, View() will pick up the new status
	return nil
}

func handleStepFinishedMsg(m *model, msg stepFinishedMsg) tea.Cmd {
	var cmds []tea.Cmd

	switch m.currentState {
	case stateSshConfigRemoveConfirm: // This state implies a 'remove host' step was run
		m.hostToRemove = nil // Clear the host targeted for removal
		if msg.err != nil {
			m.lastError = fmt.Errorf("failed to remove host: %w", msg.err)
			m.currentState = stateSshConfigList // Go back to list on error
			cmds = append(cmds, loadSshConfigCmd())
		} else {
			// Successful removal: Reload config and rediscover stacks
			m.currentState = stateLoadingStacks // Show loading while rediscovering
			m.isDiscovering = true
			m.stacks = nil // Clear existing stacks
			m.discoveryErrors = nil
			m.stackStatuses = make(map[string]runner.StackRuntimeInfo) // Clear statuses
			m.loadingStatus = make(map[string]bool)
			m.cursor = 0
			cmds = append(cmds, loadSshConfigCmd(), findStacksCmd())
		}

	case stateRunningSequence:
		m.outputChan = nil // Stop listening for output/errors for this step
		m.errorChan = nil
		if msg.err != nil {
			// Step failed
			m.lastError = msg.err
			m.currentState = stateSequenceError
			m.outputContent += errorStyle.Render(fmt.Sprintf("\n--- STEP FAILED: %v ---", msg.err)) + "\n"
			m.viewport.SetContent(m.outputContent)
			m.viewport.GotoBottom()
		} else {
			// Step succeeded
			stepName := "Unknown Step"
			if m.currentSequence != nil && m.currentStepIndex < len(m.currentSequence) {
				stepName = m.currentSequence[m.currentStepIndex].Name
			}
			m.outputContent += successStyle.Render(fmt.Sprintf("\n--- Step '%s' Succeeded ---", stepName)) + "\n"
			m.currentStepIndex++ // Move to the next step index

			if m.currentStepIndex >= len(m.currentSequence) {
				// Sequence finished successfully
				m.outputContent += successStyle.Render("\n--- Action Sequence Completed Successfully ---") + "\n"
				m.viewport.SetContent(m.outputContent)
				m.viewport.GotoBottom()
				// Optionally, refresh status of involved stacks after sequence completion
				for _, stack := range m.stacksInSequence {
					if stack != nil {
						stackID := stack.Identifier()
						if !m.loadingStatus[stackID] {
							m.loadingStatus[stackID] = true
							cmds = append(cmds, m.fetchStackStatusCmd(*stack))
						}
					}
				}
				// Note: We stay in stateRunningSequence view until user presses Back/Enter
			} else {
				// Start the next step
				cmds = append(cmds, m.startNextStepCmd())
			}
		}

	case stateRunningHostAction: // e.g., Prune
		m.outputChan = nil
		m.errorChan = nil
		stepName := "Unknown Action"
		if m.currentHostActionStep.Name != "" {
			stepName = m.currentHostActionStep.Name
		}

		if msg.err != nil {
			// Host action failed
			m.hostActionError = msg.err // Store specific host action error
			m.lastError = msg.err       // Also update general lastError for display
			m.outputContent += errorStyle.Render(fmt.Sprintf("\n--- HOST ACTION '%s' FAILED: %v ---", stepName, msg.err)) + "\n"
			m.viewport.SetContent(m.outputContent)
			m.viewport.GotoBottom()
			m.currentState = stateSshConfigList     // Go back to config list
			cmds = append(cmds, loadSshConfigCmd()) // Reload config state
		} else {
			// Host action succeeded
			m.outputContent += successStyle.Render(fmt.Sprintf("\n--- Host Action '%s' Completed Successfully ---", stepName)) + "\n"
			m.viewport.SetContent(m.outputContent)
			m.viewport.GotoBottom()
			m.currentState = stateSshConfigList // Go back to config list
			m.hostsToPrune = nil                // Clear prune targets
			m.hostActionError = nil
			m.lastError = nil                       // Clear last error on success
			cmds = append(cmds, loadSshConfigCmd()) // Reload config state
		}
		m.currentHostActionStep = runner.HostCommandStep{} // Clear the current host step

		// Add cases for other states if steps can finish there
	}
	return tea.Batch(cmds...)
}

func handleChannelsAvailableMsg(m *model, msg channelsAvailableMsg) tea.Cmd {
	// Check the state to ensure we should be expecting channels
	if m.currentState == stateRunningSequence || m.currentState == stateRunningHostAction {
		m.outputChan = msg.outChan
		m.errorChan = msg.errChan
		// Return commands to start listening on these channels
		return tea.Batch(
			waitForOutputCmd(m.outputChan),
			waitForErrorCmd(m.errorChan), // You might want a different error message type for host actions vs sequence steps
		)
	}
	// If not in a state expecting channels, ignore the message
	return nil
}

func handleOutputLineMsg(m *model, msg outputLineMsg) tea.Cmd {
	// Check if we are in a state that displays streaming output and have an active channel
	if (m.currentState == stateRunningSequence || m.currentState == stateRunningHostAction) && m.outputChan != nil {
		// Append the raw line content. Lipgloss/terminal handles ANSI.
		m.outputContent += msg.line.Line
		m.viewport.SetContent(m.outputContent)
		m.viewport.GotoBottom()
		// Continue waiting for more output on the same channel
		return waitForOutputCmd(m.outputChan)
	}
	// Ignore if not in the right state or channel is closed
	return nil
}

func handleSshHostAddedMsg(m *model, msg sshHostAddedMsg) tea.Cmd {
	// This message should only be relevant if we were in the AddForm state
	if m.currentState == stateSshConfigAddForm {
		if msg.err != nil {
			m.formError = msg.err // Display error on the form
			return nil            // Stay in the form state
		}
		// Success: Clean up form, reload config, rediscover stacks
		m.formError = nil
		m.formInputs = nil
		m.configCursor = 0 // Reset cursor in config list

		m.currentState = stateLoadingStacks // Show loading while rediscovering
		m.isDiscovering = true
		m.stacks = nil
		m.discoveryErrors = nil
		m.stackStatuses = make(map[string]runner.StackRuntimeInfo)
		m.loadingStatus = make(map[string]bool)
		return tea.Batch(loadSshConfigCmd(), findStacksCmd())
	}
	// Ignore if received in an unexpected state
	return nil
}

func handleSshHostEditedMsg(m *model, msg sshHostEditedMsg) tea.Cmd {
	// This message should only be relevant if we were in the EditForm state
	if m.currentState == stateSshConfigEditForm {
		if msg.err != nil {
			m.formError = msg.err // Display error on the form
			return nil            // Stay in the form state
		}
		// Success: Clean up form, reload config, rediscover stacks
		m.formError = nil
		m.formInputs = nil
		m.hostToEdit = nil // Clear the host being edited
		m.configCursor = 0 // Reset cursor in config list

		m.currentState = stateLoadingStacks // Show loading while rediscovering
		m.isDiscovering = true
		m.stacks = nil
		m.discoveryErrors = nil
		m.stackStatuses = make(map[string]runner.StackRuntimeInfo)
		m.loadingStatus = make(map[string]bool)
		return tea.Batch(loadSshConfigCmd(), findStacksCmd())
	}
	// Ignore if received in an unexpected state
	return nil
}

// Add other message handlers here as needed...
