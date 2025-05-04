// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/runner"
	"fmt"
	"strings"
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
		// Render the error message using the errorStyle
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
