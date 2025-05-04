// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package main

import (
	"os"

	"bucket-manager/cmd/cli"
	"bucket-manager/cmd/tui"
	"bucket-manager/internal/logger"
)

func main() {
	// Initialize logger for CLI mode (logs to file and stderr)
	logger.InitLogger(false)

	if len(os.Args) <= 1 {
		tui.RunTUI()
	} else {
		cli.RunCLI()
	}
}
