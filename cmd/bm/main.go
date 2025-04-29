package main

import (
	"os"

	"bucket-manager/cmd/cli"
	"bucket-manager/cmd/tui"
)

func main() {
	// If no arguments (or just the program name) are provided, run the TUI.
	// Otherwise, run the CLI (which will handle the arguments).
	if len(os.Args) <= 1 {
		tui.RunTUI()
	} else {
		cli.RunCLI()
	}
}
