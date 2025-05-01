// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package tui

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"bucket-manager/internal/ssh"
	"bucket-manager/internal/ui"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// RunTUI initializes and runs the Bubble Tea TUI application.
func RunTUI() {
	if err := config.EnsureConfigDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Error ensuring config directory: %v\n", err)
		os.Exit(1)
	}

	sshManager := ssh.NewManager()
	defer sshManager.CloseAll() // Ensure connections are closed when TUI exits

	discovery.InitSSHManager(sshManager)
	runner.InitSSHManager(sshManager)

	m := ui.InitialModel()
	p := tea.NewProgram(&m, tea.WithAltScreen()) // Use AltScreen
	ui.BubbleProgram = p
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}
