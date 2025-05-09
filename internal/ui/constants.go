// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

// state represents the different views or modes of the TUI.
type state int

const (
	stateLoadingStacks state = iota
	stateStackList
	stateRunningSequence
	stateSequenceError
	stateStackDetails
	stateSshConfigList
	stateSshConfigRemoveConfirm
	stateSshConfigAddForm
	stateSshConfigImportSelect
	stateSshConfigImportDetails
	stateSshConfigEditForm
	statePruneConfirm
	stateRunningHostAction
)

// Constants for SSH authentication methods.
const (
	authMethodKey = iota + 1
	authMethodAgent
	authMethodPassword
)

const (
	headerHeight              = 1 // Height reserved for the main title header (single line, JoinVertical adds newline).
	maxConcurrentStatusChecks = 4 // Limit concurrent stack status checks via SSH.
)
