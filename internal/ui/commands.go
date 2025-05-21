// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package ui's commands.go file contains Bubble Tea commands that perform
// asynchronous operations in the TUI. These commands handle long-running tasks
// like discovering stacks, executing podman compose operations, and managing
// configurations without blocking the UI.

package ui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Bubble Tea Commands ---
// These functions create tea.Cmds to perform asynchronous operations.
// Each command runs in its own goroutine and communicates back to the main
// UI loop by sending messages through the Bubble Tea program.

// findStacksCmd creates a command to discover all available stacks.
// It handles both local and remote stack discovery in the background.
func findStacksCmd() tea.Cmd {
	return func() tea.Msg {
		stackChan, errorChan, doneChan := discovery.FindStacks()

		go func() {
			for s := range stackChan {
				if BubbleProgram != nil {
					BubbleProgram.Send(stackDiscoveredMsg{stack: s})
				}
			}
		}()

		go func() {
			for e := range errorChan {
				if BubbleProgram != nil {
					BubbleProgram.Send(discoveryErrorMsg{err: e})
				}
			}
		}()

		go func() {
			<-doneChan
			if BubbleProgram != nil {
				BubbleProgram.Send(discoveryFinishedMsg{})
			}
		}()

		return nil
	}
}

func loadSshConfigCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		return sshConfigLoadedMsg{hosts: cfg.SSHHosts, Err: err}
	}
}

func saveEditedSshHostCmd(originalName string, editedHost config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostEditedMsg{fmt.Errorf("failed to load config before saving edit: %w", err)}
		}

		found := false
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == originalName {
				if originalName != editedHost.Name {
					// Check for name collision only if the name was changed
					for j, otherHost := range cfg.SSHHosts {
						if i != j && otherHost.Name == editedHost.Name {
							return sshHostEditedMsg{fmt.Errorf("host name '%s' already exists", editedHost.Name)}
						}
					}
				}
				cfg.SSHHosts[i] = editedHost
				found = true
				break
			}
		}

		if !found {
			return sshHostEditedMsg{fmt.Errorf("original host '%s' not found during save", originalName)}
		}

		err = config.SaveConfig(cfg)
		if err != nil {
			return sshHostEditedMsg{fmt.Errorf("failed to save config after edit: %w", err)}
		}
		return sshHostEditedMsg{nil}
	}
}

func saveNewSshHostCmd(newHost config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostAddedMsg{fmt.Errorf("failed to load config before saving: %w", err)}
		}

		// Check for existing name
		for _, h := range cfg.SSHHosts {
			if h.Name == newHost.Name {
				return sshHostAddedMsg{fmt.Errorf("host name '%s' already exists", newHost.Name)}
			}
		}

		cfg.SSHHosts = append(cfg.SSHHosts, newHost)
		err = config.SaveConfig(cfg)
		if err != nil {
			return sshHostAddedMsg{fmt.Errorf("failed to save config: %w", err)}
		}
		return sshHostAddedMsg{nil}
	}
}

func removeSshHostCmd(hostToRemove config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return stepFinishedMsg{fmt.Errorf("failed to load config before remove: %w", err)}
		}
		newHosts := []config.SSHHost{}
		found := false
		for _, h := range cfg.SSHHosts {
			if h.Name != hostToRemove.Name {
				newHosts = append(newHosts, h)
			} else {
				found = true
			}
		}
		if !found {
			// This case should ideally not happen if the UI prevents it, but handle defensively.
			return stepFinishedMsg{err: fmt.Errorf("host '%s' not found in config during removal", hostToRemove.Name)}
		}
		cfg.SSHHosts = newHosts
		err = config.SaveConfig(cfg)
		if err != nil {
			return stepFinishedMsg{err: fmt.Errorf("failed to save config after remove: %w", err)}
		}
		return stepFinishedMsg{nil} // Signal success (no error)
	}
}

func parseSshConfigCmd() tea.Cmd {
	return func() tea.Msg {
		potentialHosts, err := config.ParseSSHConfig()
		return sshConfigParsedMsg{potentialHosts: potentialHosts, err: err}
	}
}

func saveImportedSshHostsCmd(hostsToSave []config.SSHHost) tea.Cmd {
	return func() tea.Msg {
		if len(hostsToSave) == 0 {
			// Nothing to save, return success with zero counts
			return sshHostsImportedMsg{importedCount: 0, skippedCount: 0, err: nil}
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			return sshHostsImportedMsg{err: fmt.Errorf("failed to load config before saving imports: %w", err)}
		}

		currentNames := make(map[string]bool)
		for _, h := range cfg.SSHHosts {
			currentNames[h.Name] = true
		}

		finalHostsToAdd := []config.SSHHost{}
		skippedCount := 0
		for _, newHost := range hostsToSave {
			if _, exists := currentNames[newHost.Name]; exists {
				skippedCount++
				continue // Skip host with conflicting name
			}
			// Add the host if no conflict and mark the name as used for subsequent checks within this batch
			finalHostsToAdd = append(finalHostsToAdd, newHost)
			currentNames[newHost.Name] = true
		}

		// If all selected hosts already existed or conflicted, return a specific error message
		if len(finalHostsToAdd) == 0 && skippedCount > 0 {
			return sshHostsImportedMsg{
				importedCount: 0,
				skippedCount:  skippedCount,
				err:           fmt.Errorf("all %d selected host(s) already exist or conflict", skippedCount),
			}
		}

		// Only save if there are actually new hosts to add
		if len(finalHostsToAdd) > 0 {
			cfg.SSHHosts = slices.Concat(cfg.SSHHosts, finalHostsToAdd)
			err = config.SaveConfig(cfg)
			if err != nil {
				// Return a real save error
				return sshHostsImportedMsg{
					importedCount: 0, // Indicate failure by setting count to 0
					skippedCount:  skippedCount,
					err:           fmt.Errorf("failed to save config after import: %w", err),
				}
			}
		}

		// Success: return counts and nil error
		return sshHostsImportedMsg{
			importedCount: len(finalHostsToAdd),
			skippedCount:  skippedCount,
			err:           nil, // Explicitly nil on success
		}
	}
}

// runHostActionCmd triggers the execution of a host-level command step (like prune) in TUI mode.
func runHostActionCmd(step runner.HostCommandStep) tea.Cmd {
	return func() tea.Msg {
		// TUI always uses cliMode: false for channel-based output
		outChan, errChan := runner.RunHostCommand(step, false)
		return channelsAvailableMsg{outChan: outChan, errChan: errChan}
	}
}

// runStepCmd triggers the execution of a stack-level command step in TUI mode.
func runStepCmd(step runner.CommandStep) tea.Cmd {
	return func() tea.Msg {
		// TUI always uses cliMode: false for channel-based output
		outChan, errChan := runner.StreamCommand(step, false)
		return channelsAvailableMsg{outChan: outChan, errChan: errChan}
	}
}

// waitForOutputCmd waits for the next line of output from a command's output channel.
func waitForOutputCmd(outChan <-chan runner.OutputLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-outChan
		if !ok {
			// Channel closed, no more output for this step
			return nil
		}
		return outputLineMsg{line}
	}
}

// waitForErrorCmd waits for the final error result from a command's error channel.
func waitForErrorCmd(errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		err := <-errChan // Blocks until the command finishes and sends an error (or nil)
		return stepFinishedMsg{err}
	}
}
