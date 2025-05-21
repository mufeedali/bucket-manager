// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package ui's messages.go file defines the message types used in the Bubble Tea
// Model-View-Update architecture. These messages are sent between components
// to communicate state changes and trigger UI updates.

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
)

// --- Message Types ---
// These types define the events that drive the TUI's state updates.
// In the Bubble Tea framework, messages are sent to the Update method
// which then updates the model state accordingly.

// Stack discovery messages
type stackDiscoveredMsg struct{ stack discovery.Stack } // Sent when a stack is found
type discoveryErrorMsg struct{ err error }              // Sent when an error occurs during discovery
type discoveryFinishedMsg struct{}                      // Sent when all stack discovery is complete

// SSH configuration messages
type sshConfigLoadedMsg struct {
	hosts []config.SSHHost
	Err   error
}
type sshHostAddedMsg struct{ err error }  // Result of adding a new SSH host
type sshHostEditedMsg struct{ err error } // Result of editing an SSH host
type sshConfigParsedMsg struct {
	potentialHosts []config.PotentialHost // Hosts found in ~/.ssh/config
	err            error
}
type sshHostsImportedMsg struct {
	importedCount int   // Number of hosts successfully imported
	skippedCount  int   // Number of hosts skipped (already exist or errors)
	err           error // Any error that occurred during import
}

// Command execution messages
type outputLineMsg struct{ line runner.OutputLine } // Single line of command output
type stepFinishedMsg struct{ err error }            // Notification that a command step finished
type stackStatusLoadedMsg struct {
	stackIdentifier string                  // Identifier of the stack that was checked
	statusInfo      runner.StackRuntimeInfo // Status information for the stack
}
type channelsAvailableMsg struct {
	outChan <-chan runner.OutputLine // Channel for receiving command output
	errChan <-chan error             // Channel for receiving command errors
}
