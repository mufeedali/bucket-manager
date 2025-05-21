// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package ui's constants.go file defines enumeration values and other constants
// used throughout the Text User Interface. This includes view states, authentication
// methods, and layout dimensions.

package ui

// state represents the different views or modes of the TUI.
// Each state corresponds to a different screen or interaction mode.
type state int

const (
	stateLoadingStacks          state = iota // Initial loading screen when discovering stacks
	stateStackList                           // Main stack list view
	stateRunningSequence                     // View when executing stack commands
	stateSequenceError                       // Error display after a failed command
	stateStackDetails                        // Detailed view of a single stack
	stateSshConfigList                       // List of SSH configurations
	stateSshConfigRemoveConfirm              // Confirmation before removing SSH config
	stateSshConfigAddForm                    // Form for adding new SSH config
	stateSshConfigImportSelect               // Selection screen for SSH config import
	stateSshConfigImportDetails              // Details of SSH configs being imported
	stateSshConfigEditForm                   // Form for editing SSH config
	statePruneConfirm                        // Confirmation before pruning
	stateRunningHostAction                   // View when executing host-level commands
)

// Constants for SSH authentication methods used in the SSH configuration forms.
const (
	authMethodKey      = iota + 1 // SSH key-based authentication
	authMethodAgent               // SSH agent-based authentication
	authMethodPassword            // Password-based authentication (least secure)
)

// Layout and performance constants
const (
	// Limit concurrent stack status checks via SSH to avoid overwhelming connections
	maxConcurrentStatusChecks = 4
)
