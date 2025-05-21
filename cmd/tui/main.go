// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package tui implements the Text User Interface mode for the bucket manager,
// providing an interactive terminal application for browsing and managing
// Podman Compose stacks using the Bubble Tea framework.
package tui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
	"bucket-manager/internal/runner"
	"bucket-manager/internal/ssh"
	"bucket-manager/internal/ui"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// RunTUI initializes and starts the Text User Interface application.
// This is the main entry point for the TUI mode of the bucket manager.
func RunTUI() {
	// Initialize logger for TUI mode (logs to file only to avoid cluttering the UI)
	logger.InitLogger(true)

	// Ensure configuration directory exists
	if err := config.EnsureConfigDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Error ensuring config directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize SSH connection manager
	sshManager := ssh.NewManager()
	defer sshManager.CloseAll() // Ensure all SSH connections are closed on exit

	// Share SSH manager with discovery package for remote stack operations
	discovery.InitSSHManager(sshManager)
	runner.InitSSHManager(sshManager)

	m := ui.InitialModel()
	p := tea.NewProgram(&m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	ui.BubbleProgram = p
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}
