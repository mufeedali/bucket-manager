package tui

import (
	"fmt"
	"os"
	"bucket-manager/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// RunTUI initializes and runs the Bubble Tea TUI application.
func RunTUI() {
	m := ui.InitialModel()
	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}
