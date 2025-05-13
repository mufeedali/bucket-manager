// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/runner"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// --- View Helpers ---

// renderStackStatus appends the detailed status view for a given stack ID
// to the provided strings.Builder. It uses the status information stored
// in the model's stackStatuses and loadingStatus maps.
func (m *model) renderStackStatus(b *strings.Builder, stackID string) {
	statusStr := ""
	statusInfo, loaded := m.stackStatuses[stackID]
	isLoading := m.loadingStatus[stackID]

	if isLoading {
		statusStr = statusLoadingStyle.Render(" [loading...]")
	} else if !loaded {
		statusStr = statusLoadingStyle.Render(" [?]") // Status not yet loaded
	} else {
		// Status is loaded, determine display based on OverallStatus
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
			statusStr = statusLoadingStyle.Render(" [Unknown]") // Should not happen
		}
	}
	fmt.Fprintf(b, "\nOverall Status:%s\n", statusStr)

	// Display error if status fetch failed
	if !isLoading && loaded && statusInfo.Error != nil {
		fmt.Fprintf(b, "%s", errorStyle.Render(fmt.Sprintf("  Error fetching status: %v\n", statusInfo.Error)))
	}

	// Display container details if loaded and no error
	if !isLoading && loaded && statusInfo.Error == nil {
		if len(statusInfo.Containers) > 0 {
			b.WriteString("\nContainers:\n")
			// Use fmt.Sprintf for header to ensure consistent spacing
			header := fmt.Sprintf("  %-20s %-30s %s", "SERVICE", "CONTAINER NAME", "STATUS")
			separator := fmt.Sprintf("  %-20s %-30s %s", strings.Repeat("-", 7), strings.Repeat("-", 14), strings.Repeat("-", 6))
			b.WriteString(header + "\n")
			b.WriteString(separator + "\n")

			for _, c := range statusInfo.Containers {
				// Determine status color
				isUp := strings.Contains(strings.ToLower(c.Status), "running") ||
					strings.Contains(strings.ToLower(c.Status), "healthy") ||
					strings.HasPrefix(strings.ToLower(c.Status), "up")

				statusRenderFunc := statusDownStyle.Render
				if isUp {
					statusRenderFunc = statusUpStyle.Render
				}
				// Use fmt.Sprintf for container line for consistent spacing
				line := fmt.Sprintf("  %-20s %-30s %s", c.Service, c.Name, statusRenderFunc(c.Status))
				b.WriteString(line + "\n")
			}
		} else if statusInfo.OverallStatus != runner.StatusError {
			// Only show "No containers" if the overall status isn't already an error
			b.WriteString("\n  (No containers found or running)\n")
		}
	}
}

// --- State-Specific View Renderers ---
// These functions generate the body and footer content for specific UI states.
// The main View() method combines these with the header and manages viewport heights.

func (m *model) renderLoadingView() (string, string) {
	body := statusStyle.Render("Loading stacks...")
	footer := footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc)
	return body, footer
}

func (m *model) renderStackListView() (string, string) {
	bodyContent := strings.Builder{}
	bodyContent.WriteString("Select a stack:\n")
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
		bodyContent.WriteString(fmt.Sprintf("%s%s %s (%s)%s\n", cursor, checkbox, stack.Name, serverNameStyle.Render(stack.ServerName), statusStr))
	}

	footerContent := strings.Builder{}

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
		help.WriteString(footerDescStyle.Render(fmt.Sprintf("(%d selected) ", len(m.selectedStackIdxs))))
	}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerDescStyle.Render(": navigate") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Select.Help().Key) + footerDescStyle.Render(": "+m.keymap.Select.Help().Desc) + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Enter.Help().Key) + footerDescStyle.Render(": details") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.UpAction.Help().Key) + footerDescStyle.Render(": up") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.DownAction.Help().Key) + footerDescStyle.Render(": down") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.RefreshAction.Help().Key) + footerDescStyle.Render(": refresh") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.PullAction.Help().Key) + footerDescStyle.Render(": pull"))
	help.WriteString(footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Config.Help().Key) + footerDescStyle.Render(": "+m.keymap.Config.Help().Desc) + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering for wrapping

	return bodyContent.String(), footerContent.String()
}

func (m *model) renderRunningSequenceView() (string, string) {
	bodyStr := m.outputContent // Use the raw content for setting viewport

	footerContent := strings.Builder{}

	stackIdentifier := ""
	if m.sequenceStack != nil {
		stackIdentifier = fmt.Sprintf(" for %s", m.sequenceStack.Identifier())
	}
	if m.currentSequence != nil && m.currentStepIndex < len(m.currentSequence) {
		footerContent.WriteString(statusStyle.Render(fmt.Sprintf("Running step %d/%d%s: %s...", m.currentStepIndex+1, len(m.currentSequence), stackIdentifier, m.currentSequence[m.currentStepIndex].Name)))
	} else if m.sequenceStack != nil { // Sequence finished successfully (implied, as error state is separate)
		footerContent.WriteString(successStyle.Render(fmt.Sprintf("Sequence finished successfully%s.", stackIdentifier)))
	} else {
		footerContent.WriteString(successStyle.Render("Sequence finished successfully."))
	}

	help := strings.Builder{}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.PgUp.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.PgDown.Help().Key) + footerDescStyle.Render(": scroll") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Enter.Help().Key) + footerDescStyle.Render(": back to list") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyStr, footerContent.String()
}

func (m *model) renderSequenceErrorView() (string, string) {
	bodyStr := m.outputContent // Use the raw content

	footerContent := strings.Builder{}

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
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.PgUp.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.PgDown.Help().Key) + footerDescStyle.Render(": scroll") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Enter.Help().Key) + footerDescStyle.Render(": back to list") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyStr, footerContent.String()
}

func (m *model) renderStackDetailsView() (string, string) {
	bodyContent := strings.Builder{}
	if m.detailedStack != nil {
		stack := m.detailedStack
		stackID := stack.Identifier()
		bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for: %s (%s)", stack.Name, serverNameStyle.Render(stack.ServerName))) + "\n\n")
		m.renderStackStatus(&bodyContent, stackID) // Use the existing helper
	} else if len(m.stacksInSequence) > 0 {
		bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for %d Selected Stacks:", len(m.stacksInSequence))) + "\n")
		for i, stack := range m.stacksInSequence {
			if stack == nil {
				continue
			}
			stackID := stack.Identifier()
			bodyContent.WriteString(fmt.Sprintf("\n--- %s (%s) ---", stack.Name, serverNameStyle.Render(stack.ServerName)))
			m.renderStackStatus(&bodyContent, stackID) // Use the existing helper
			if i < len(m.stacksInSequence)-1 {
				bodyContent.WriteString("\n")
			}
		}
	} else {
		bodyContent.WriteString(errorStyle.Render("Error: No stack selected for details."))
	}

	footerContent := strings.Builder{}
	help := strings.Builder{}
	help.WriteString(footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerDescStyle.Render(": back to list") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyContent.String(), footerContent.String()
}

func (m *model) renderSshConfigListView() (string, string) {
	bodyContent := strings.Builder{}
	bodyContent.WriteString("Configured Hosts:\n\n")

	// Display "local" entry first
	localCursor := "  "
	if m.configCursor == 0 {
		localCursor = cursorStyle.Render("> ")
	}
	bodyContent.WriteString(fmt.Sprintf("%s%s (%s)\n", localCursor, "local", serverNameStyle.Render("Local Podman")))

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

	footerContent := strings.Builder{}

	help := strings.Builder{}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerDescStyle.Render(": navigate") + footerSeparatorStyle.Render(" | "))
	// Show actions based on selection
	if m.configCursor == 0 { // "local" selected
		help.WriteString(footerKeyStyle.Render(m.keymap.PruneAction.Help().Key) + footerDescStyle.Render(": prune") + footerSeparatorStyle.Render(" | "))
	} else { // Remote host selected
		help.WriteString(footerKeyStyle.Render(m.keymap.Edit.Help().Key) + footerDescStyle.Render(": edit") + footerSeparatorStyle.Render(" | "))
		help.WriteString(footerKeyStyle.Render(m.keymap.Remove.Help().Key) + footerDescStyle.Render(": remove") + footerSeparatorStyle.Render(" | "))
		help.WriteString(footerKeyStyle.Render(m.keymap.PruneAction.Help().Key) + footerDescStyle.Render(": prune") + footerSeparatorStyle.Render(" | "))
	}
	// Add and Import are always available
	help.WriteString(footerKeyStyle.Render(m.keymap.Add.Help().Key) + footerDescStyle.Render(": add") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Import.Help().Key) + footerDescStyle.Render(": import") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerDescStyle.Render(": back") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))

	errorOrInfo := ""
	if m.hostActionError != nil { // Display host action error first
		errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Prune Error: %v", m.hostActionError))
	} else if m.importInfoMsg != "" { // Then import info
		errorOrInfo = "\n" + successStyle.Render(m.importInfoMsg)
	} else if m.importError != nil { // Then import error
		errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Import Error: %v", m.importError))
	} else if m.lastError != nil { // Finally general errors
		errorOrInfo = "\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.lastError))
	}

	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String()))
	if errorOrInfo != "" {
		footerContent.WriteString(errorOrInfo)
	}

	return bodyContent.String(), footerContent.String()
}

func (m *model) renderSshConfigRemoveConfirmView() (string, string) {
	bodyContent := strings.Builder{}
	if m.hostToRemove != nil {
		bodyContent.WriteString(fmt.Sprintf("Are you sure you want to remove the SSH host '%s'?\n\n", identifierColor.Render(m.hostToRemove.Name)))
		bodyContent.WriteString("[y] Yes, remove | [n/Esc/b] No, cancel")
	} else {
		bodyContent.WriteString(errorStyle.Render("Error: No host selected for removal. Press Esc/b to go back."))
	}

	footerContent := strings.Builder{}
	help := strings.Builder{}
	if m.hostToRemove != nil {
		help.WriteString(footerDescStyle.Render(fmt.Sprintf("Confirm removal of '%s'? ", identifierColor.Render(m.hostToRemove.Name))))
		help.WriteString(footerKeyStyle.Render(m.keymap.Yes.Help().Key) + footerDescStyle.Render(": "+m.keymap.Yes.Help().Desc) + footerSeparatorStyle.Render(" | "))
		help.WriteString(footerKeyStyle.Render(m.keymap.No.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerDescStyle.Render(": "+m.keymap.No.Help().Desc+"/cancel"))
	} else {
		help.WriteString(errorStyle.Render("Error - no host selected. "))
		help.WriteString(footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerDescStyle.Render(": back"))
	}
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	// For simple confirmation views, body is often placed without a viewport
	return bodyContent.String(), footerContent.String()
}

func (m *model) renderPruneConfirmView() (string, string) {
	bodyContent := strings.Builder{}
	if len(m.hostsToPrune) > 0 {
		targetName := m.hostsToPrune[0].ServerName // TUI currently only prunes one host
		bodyContent.WriteString(fmt.Sprintf("Are you sure you want to run 'podman system prune -af' on host '%s'?\n\n", identifierColor.Render(targetName)))
		bodyContent.WriteString("This will remove all unused containers, networks, images, and build cache.\n\n")
		bodyContent.WriteString("[y] Yes, prune | [n/Esc/b] No, cancel")
	} else {
		bodyContent.WriteString(errorStyle.Render("Error: No host selected for prune. Press Esc/b to go back."))
	}

	footerContent := strings.Builder{}
	help := strings.Builder{}
	if len(m.hostsToPrune) > 0 {
		targetName := m.hostsToPrune[0].ServerName
		help.WriteString(footerDescStyle.Render(fmt.Sprintf("Confirm prune action for host '%s'? ", identifierColor.Render(targetName))))
		help.WriteString(footerKeyStyle.Render(m.keymap.Yes.Help().Key) + footerDescStyle.Render(": "+m.keymap.Yes.Help().Desc) + footerSeparatorStyle.Render(" | "))
		help.WriteString(footerKeyStyle.Render(m.keymap.No.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerDescStyle.Render(": "+m.keymap.No.Help().Desc+"/cancel"))
	} else {
		help.WriteString(errorStyle.Render("Error - no host selected. "))
		help.WriteString(footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerDescStyle.Render(": back"))
	}
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyContent.String(), footerContent.String()
}

func (m *model) renderRunningHostActionView() (string, string) {
	bodyStr := m.outputContent

	footerContent := strings.Builder{}

	targetName := "unknown host"
	actionName := "action"
	if m.currentHostActionStep.Name != "" {
		actionName = m.currentHostActionStep.Name
	}
	if len(m.hostsToPrune) > 0 {
		targetName = m.hostsToPrune[0].ServerName
	}
	footerContent.WriteString(statusStyle.Render(fmt.Sprintf("Running %s on '%s'...", actionName, identifierColor.Render(targetName))))

	help := strings.Builder{}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.PgUp.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.PgDown.Help().Key) + footerDescStyle.Render(": scroll") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString("\n" + lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyStr, footerContent.String()
}

func (m *model) renderSshConfigAddFormView() (string, string) {
	bodyContent := strings.Builder{}
	bodyContent.WriteString(titleStyle.Render("Add New SSH Host") + "\n\n")
	// Render basic inputs (Name, Hostname, User, Port, RemoteRoot)
	for i := 0; i < 5; i++ {
		bodyContent.WriteString(m.formInputs[i].View() + "\n")
	}
	// Render Auth Method selector
	authFocus := "  "
	authStyle := lipgloss.NewStyle()
	if m.formFocusIndex == 5 { // Logical index for auth selector
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
	// Render conditional inputs (Key Path or Password)
	switch m.formAuthMethod {
	case authMethodKey:
		bodyContent.WriteString(m.formInputs[5].View() + "\n") // Index 5 is Key Path
	case authMethodPassword:
		bodyContent.WriteString(m.formInputs[6].View() + "\n") // Index 6 is Password
	}

	footerContent := strings.Builder{}

	if m.formError != nil {
		footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)) + "\n")
	}
	help := strings.Builder{}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Tab.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.ShiftTab.Help().Key) + footerDescStyle.Render(": navigate") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Left.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Right.Help().Key) + footerDescStyle.Render(": change auth") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Enter.Help().Key) + footerDescStyle.Render(": save") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Esc.Help().Key) + footerDescStyle.Render(": "+m.keymap.Esc.Help().Desc) + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyContent.String(), footerContent.String()
}

func (m *model) renderSshConfigEditFormView() (string, string) {
	bodyContent := strings.Builder{}
	if m.hostToEdit == nil {
		bodyContent.WriteString(errorStyle.Render("Error: No host selected for editing."))
	} else {
		bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Edit SSH Host: %s", identifierColor.Render(m.hostToEdit.Name))) + "\n\n")
		// Render basic inputs
		for i := 0; i < 5; i++ {
			bodyContent.WriteString(m.formInputs[i].View() + "\n")
		}
		// Render Auth Method selector
		authFocus := "  "
		authStyle := lipgloss.NewStyle()
		if m.formFocusIndex == 5 { // Logical index for auth selector
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
		// Render conditional inputs
		if m.formAuthMethod == authMethodKey {
			bodyContent.WriteString(m.formInputs[5].View() + "\n") // Index 5 is Key Path
		}
		if m.formAuthMethod == authMethodPassword {
			bodyContent.WriteString(m.formInputs[6].View() + "\n") // Index 6 is Password
		}
		// Render Disabled toggle
		disabledFocus := "  "
		disabledStyle := lipgloss.NewStyle()
		if m.formFocusIndex == 8 { // Logical index for disabled toggle
			disabledFocus = cursorStyle.Render("> ")
			disabledStyle = cursorStyle
		}
		checkbox := "[ ]"
		if m.formDisabled {
			checkbox = successStyle.Render("[x]")
		}
		bodyContent.WriteString(fmt.Sprintf("%s%s\n", disabledFocus, disabledStyle.Render(checkbox+" Disabled Host [space to toggle]")))
	}

	// Footer generation
	footerContent := strings.Builder{}

	if m.formError != nil {
		footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)) + "\n")
	}
	help := strings.Builder{}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Tab.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.ShiftTab.Help().Key) + footerDescStyle.Render(": navigate") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Left.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Right.Help().Key) + footerDescStyle.Render(": change auth") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.ToggleDisabled.Help().Key) + footerDescStyle.Render(": "+m.keymap.ToggleDisabled.Help().Desc) + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Enter.Help().Key) + footerDescStyle.Render(": save") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Esc.Help().Key) + footerDescStyle.Render(": "+m.keymap.Esc.Help().Desc) + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyContent.String(), footerContent.String()
}

func (m *model) renderSshConfigImportSelectView() (string, string) {
	bodyContent := strings.Builder{}
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

	footerContent := strings.Builder{}

	help := strings.Builder{}
	if len(m.selectedImportIdxs) > 0 {
		help.WriteString(footerDescStyle.Render(fmt.Sprintf("(%d selected) ", len(m.selectedImportIdxs))))
	}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerDescStyle.Render(": navigate") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Select.Help().Key) + footerDescStyle.Render(": "+m.keymap.Select.Help().Desc) + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Enter.Help().Key) + footerDescStyle.Render(": confirm"))
	help.WriteString(footerSeparatorStyle.Render(" | ") + footerKeyStyle.Render(m.keymap.Back.Help().Key) + footerDescStyle.Render(": cancel") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyContent.String(), footerContent.String()
}

func (m *model) renderSshConfigImportDetailsView() (string, string) {
	bodyContent := strings.Builder{}
	if len(m.importableHosts) == 0 || m.configuringHostIdx >= len(m.importableHosts) || m.configuringHostIdx < 0 {
		bodyContent.WriteString(errorStyle.Render("Error: Invalid state for import details."))
	} else {
		pHost := m.importableHosts[m.configuringHostIdx]
		title := fmt.Sprintf("Configure Import: %s (%s@%s)", identifierColor.Render(pHost.Alias), pHost.User, pHost.Hostname)
		bodyContent.WriteString(titleStyle.Render(title) + "\n\n")
		bodyContent.WriteString(m.formInputs[4].View() + "\n") // Remote Root Path (index 4)

		authNeeded := pHost.KeyPath == "" // Determine if auth details were missing in ssh_config
		if authNeeded {
			// Render Auth Method selection
			authFocus := "  "
			authStyle := lipgloss.NewStyle()
			if m.formFocusIndex == 1 { // Logical index for auth selector
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

			// Render Key Path or Password input based on selection
			if m.formAuthMethod == authMethodKey {
				bodyContent.WriteString(m.formInputs[5].View() + "\n") // Index 5 is Key Path
			}
			if m.formAuthMethod == authMethodPassword {
				bodyContent.WriteString(m.formInputs[6].View() + "\n") // Index 6 is Password
			}
		} else {
			// Auth details were present in ssh_config, just display them
			bodyContent.WriteString(fmt.Sprintf("  Auth Method: SSH Key File (from ssh_config: %s)\n", lipgloss.NewStyle().Faint(true).Render(pHost.KeyPath)))
		}
	}

	footerContent := strings.Builder{}

	remaining := 0
	if m.configuringHostIdx >= 0 { // Check index validity
		for i := m.configuringHostIdx + 1; i < len(m.importableHosts); i++ {
			if _, ok := m.selectedImportIdxs[i]; ok {
				remaining++
			}
		}
	}
	hostLabel := "host"
	if remaining != 1 {
		hostLabel = "hosts"
	}
	if m.formError != nil {
		footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.formError)) + "\n")
	}
	help := strings.Builder{}
	help.WriteString(footerKeyStyle.Render(m.keymap.Up.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Down.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Tab.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.ShiftTab.Help().Key) + footerDescStyle.Render(": navigate") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Left.Help().Key) + footerSeparatorStyle.Render("/") + footerKeyStyle.Render(m.keymap.Right.Help().Key) + footerDescStyle.Render(": change auth") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Enter.Help().Key) + footerDescStyle.Render(fmt.Sprintf(": confirm & next (%d %s remaining)", remaining, hostLabel)) + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Esc.Help().Key) + footerDescStyle.Render(": cancel import") + footerSeparatorStyle.Render(" | "))
	help.WriteString(footerKeyStyle.Render(m.keymap.Quit.Help().Key) + footerDescStyle.Render(": "+m.keymap.Quit.Help().Desc))
	footerContent.WriteString(lipgloss.NewStyle().Width(m.width).Render(help.String())) // Keep lipgloss width rendering

	return bodyContent.String(), footerContent.String()
}
