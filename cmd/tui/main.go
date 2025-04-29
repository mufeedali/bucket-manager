package main // TUI entry point is in package main

import (
	"fmt"
	"os"
	"podman-compose-manager/internal/ui" // Import the internal ui package

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Initialize the model
	m := ui.InitialModel()

	// Create and run the Bubble Tea program
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}
