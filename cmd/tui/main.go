package main

import (
	"fmt"
	"os"
	"podman-compose-manager/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	m := ui.InitialModel()
	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}
