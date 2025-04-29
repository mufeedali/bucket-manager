package ui

import (
	"fmt"
	"podman-compose-manager/internal/discovery"
	"podman-compose-manager/internal/runner"
	"path/filepath" // Added import
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type state int

const (
	stateLoadingProjects state = iota
	stateProjectList
	stateRunningCommand
	stateCommandOutput
	stateError
)

type model struct {
	projects      []discovery.Project
	cursor        int // which project is selected
	selected      int // which project is confirmed for action
	currentState  state
	commandOutput string
	err           error
	width         int
	height        int
}

// --- Messages ---

type projectsLoadedMsg struct {
	projects []discovery.Project
}
type commandFinishedMsg struct {
	output string
}
type errorMsg struct {
	err error
}

// --- Commands ---

func findProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		// Hardcoded path as requested
		projs, err := discovery.FindProjects("/home/ubuntu/bucket")
		if err != nil {
			return errorMsg{err}
		}
		return projectsLoadedMsg{projs}
	}
}

func runUpCmd(projectPath string) tea.Cmd {
	return func() tea.Msg {
		output, err := runner.Up(projectPath)
		if err != nil {
			return errorMsg{fmt.Errorf("UP failed for %s: %w\nOutput:\n%s", filepath.Base(projectPath), err, output)}
		}
		return commandFinishedMsg{output}
	}
}

func runDownCmd(projectPath string) tea.Cmd {
	return func() tea.Msg {
		output, err := runner.Down(projectPath)
		if err != nil {
			return errorMsg{fmt.Errorf("DOWN failed for %s: %w\nOutput:\n%s", filepath.Base(projectPath), err, output)}
		}
		return commandFinishedMsg{output}
	}
}

func runRefreshCmd(projectPath string) tea.Cmd {
	return func() tea.Msg {
		output, err := runner.Refresh(projectPath)
		if err != nil {
			return errorMsg{fmt.Errorf("REFRESH failed for %s: %w\nOutput:\n%s", filepath.Base(projectPath), err, output)}
		}
		return commandFinishedMsg{output}
	}
}


// --- Model Implementation ---

func InitialModel() model {
	return model{
		currentState: stateLoadingProjects,
		selected:     -1, // Nothing selected initially
	}
}

func (m model) Init() tea.Cmd {
	return findProjectsCmd() // Load projects on startup
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	// Is it a key press?
	case tea.KeyMsg:
		switch m.currentState {
		case stateProjectList:
			cmd = m.handleProjectListKeys(msg)
		case stateCommandOutput, stateError: // Allow quitting or going back from output/error view
			if msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc || msg.String() == "b" {
				m.currentState = stateProjectList
				m.commandOutput = ""
				m.err = nil
			} else if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
		case stateRunningCommand: // No key handling while command is running, except Ctrl+C
			if msg.Type == tea.KeyCtrlC {
				// TODO: Implement cancellation if possible/desired
				return m, tea.Quit // For now, just quit
			}
		default: // Loading projects state
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
		}


	// Custom messages
	case projectsLoadedMsg:
		m.projects = msg.projects
		m.currentState = stateProjectList
		if len(m.projects) == 0 {
			m.err = fmt.Errorf("no projects found in /home/ubuntu/bucket")
			m.currentState = stateError
		}

	case commandFinishedMsg:
		m.commandOutput = msg.output
		m.currentState = stateCommandOutput
		m.selected = -1 // Reset selection

	case errorMsg:
		m.err = msg.err
		m.currentState = stateError
		m.selected = -1 // Reset selection
	}

	return m, cmd
}

// handleProjectListKeys handles key presses when the project list is active.
func (m *model) handleProjectListKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	// These keys should exit the program.
	case "ctrl+c", "q":
		return tea.Quit

	// The "up" and "k" keys move the cursor up.
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	// The "down" and "j" keys move the cursor down.
	case "down", "j":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}

	// The "u" key triggers the Up action.
	case "u":
		if len(m.projects) > 0 {
			m.selected = m.cursor
			m.currentState = stateRunningCommand
			m.commandOutput = fmt.Sprintf("Running UP for %s...", m.projects[m.selected].Name)
			return runUpCmd(m.projects[m.selected].Path)
		}

	// The "d" key triggers the Down action.
	case "d":
		if len(m.projects) > 0 {
			m.selected = m.cursor
			m.currentState = stateRunningCommand
			m.commandOutput = fmt.Sprintf("Running DOWN for %s...", m.projects[m.selected].Name)
			return runDownCmd(m.projects[m.selected].Path)
		}

	// The "r" key triggers the Refresh action.
	case "r":
		if len(m.projects) > 0 {
			m.selected = m.cursor
			m.currentState = stateRunningCommand
			m.commandOutput = fmt.Sprintf("Running REFRESH for %s...", m.projects[m.selected].Name)
			return runRefreshCmd(m.projects[m.selected].Path)
		}
	}
	return nil
}


func (m model) View() string {
	s := strings.Builder{}

	switch m.currentState {
	case stateLoadingProjects:
		s.WriteString("Loading projects...\n")

	case stateProjectList:
		s.WriteString("Select a Podman Compose project:\n\n")
		for i, project := range m.projects {
			cursor := " " // no cursor
			if m.cursor == i {
				cursor = ">" // cursor!
			}
			s.WriteString(fmt.Sprintf("%s %s\n", cursor, project.Name))
		}
		s.WriteString("\nControls: [↑/k ↓/j] Navigate | [u] Up | [d] Down | [r] Refresh | [q] Quit\n")


	case stateRunningCommand:
		s.WriteString(m.commandOutput) // Show "Running..." message
		s.WriteString("\n\nPlease wait...")

	case stateCommandOutput:
		s.WriteString("--- Command Output ---\n")
		s.WriteString(m.commandOutput)
		s.WriteString("\n--- End Output ---\n")
		s.WriteString("\n[Enter/Esc/b] Back to list | [q] Quit\n")

	case stateError:
		s.WriteString("--- Error ---\n")
		s.WriteString(fmt.Sprintf("%v", m.err))
		s.WriteString("\n--- End Error ---\n")
		s.WriteString("\n[Enter/Esc/b] Back to list | [q] Quit\n")
	}

	return s.String()
}

// Helper to get base path - needed because runner functions use full path
// but error messages might be clearer with just the project name.
