// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package main is the entry point for the bucket manager (bm) application,
// which provides CLI, TUI, and Web UI interfaces for managing compose stacks.
package main

import (
	"os"

	"bucket-manager/cmd/cli"
	"bucket-manager/cmd/tui"
	"bucket-manager/internal/logger"
)

// main is the entry point of the application that determines whether to run
// in CLI or TUI mode based on command-line arguments.
// If arguments are provided, CLI mode is selected; otherwise TUI mode starts.
func main() {
	// Determine mode based on command line arguments
	if len(os.Args) > 1 {
		// Initialize logger for CLI mode (clean by default)
		logger.InitCLI(false, false)
		cli.RunCLI()
	} else {
		// Initialize logger for TUI mode (file only)
		logger.InitTUI()
		tui.RunTUI()
	}
}
