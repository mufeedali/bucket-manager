// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
)

// --- Messages ---
// These types define the events that drive the TUI's state updates.

type stackDiscoveredMsg struct{ stack discovery.Stack }
type discoveryErrorMsg struct{ err error }
type discoveryFinishedMsg struct{}
type sshConfigLoadedMsg struct {
	hosts []config.SSHHost
	Err   error
}
type sshHostAddedMsg struct{ err error }
type sshHostEditedMsg struct{ err error }
type sshConfigParsedMsg struct {
	potentialHosts []config.PotentialHost
	err            error
}
type sshHostsImportedMsg struct {
	importedCount int
	skippedCount  int
	err           error
}
type outputLineMsg struct{ line runner.OutputLine }
type stepFinishedMsg struct{ err error }
type stackStatusLoadedMsg struct {
	stackIdentifier string
	statusInfo      runner.StackRuntimeInfo
}
type channelsAvailableMsg struct {
	outChan <-chan runner.OutputLine
	errChan <-chan error
}
