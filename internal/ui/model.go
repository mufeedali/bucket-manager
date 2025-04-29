package ui

import (
	"fmt"
	"podman-compose-manager/internal/discovery"
	"podman-compose-manager/internal/runner"
	// "path/filepath" // Removed unused import
	"strings"
	// "time" // Removed unused import

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss" // For styling
)

// Define styles
var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")) // Purple
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // Red
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Blue
	stepStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // Magenta
)


type state int

const (
	stateLoadingProjects state = iota // Initial state, fetching projects
	stateProjectList                  // Displaying the list of projects
	stateRunningSequence              // An action sequence (up/down/refresh) is running
	stateSequenceError                // An error occurred during a sequence
)

type model struct {
	projects         []discovery.Project
	cursor           int // which project is selected in the list
	viewport         viewport.Model
	currentState     state
	currentSequence  []runner.CommandStep // Steps for the current action (up/down/refresh)
	currentStepIndex int                  // Index of the step currently running
	outputContent    string               // Accumulated output for the viewport
	lastError        error                // Store the last error that occurred
	ready            bool                 // Flag to indicate if viewport is ready
	width            int
	height           int
	// Channels for the currently running step
	outputChan <-chan runner.OutputLine
	errorChan  <-chan error
}

// --- Messages ---

type projectsLoadedMsg struct { // Existing message
	projects []discovery.Project
}

type outputLineMsg struct { // New message for a single line of output
	line runner.OutputLine
}

type stepFinishedMsg struct { // New message indicating a step completed (success or fail)
	err error // nil if successful
}

type sequenceFinishedMsg struct{} // New message indicating all steps in sequence are done


// --- Commands ---

// findProjectsCmd returns projects or an error via stepFinishedMsg
func findProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		// Use /home/ubuntu/bucket consistently
		projs, err := discovery.FindProjects("/home/ubuntu/bucket")
		if err != nil {
			// Return stepFinishedMsg with error to signify loading failure
			return stepFinishedMsg{fmt.Errorf("failed to load projects: %w", err)}
		}
		return projectsLoadedMsg{projs}
	}
}

// New message to pass channels back to Update loop
type channelsAvailableMsg struct {
	outChan <-chan runner.OutputLine
	errChan <-chan error
}

// runStepCmd is the command that starts a step and returns channels via channelsAvailableMsg
func runStepCmd(step runner.CommandStep) tea.Cmd {
	return func() tea.Msg {
		outChan, errChan := runner.StreamCommand(step)
		return channelsAvailableMsg{outChan: outChan, errChan: errChan}
	}
}

// waitForOutputCmd listens on the output channel for one message
func waitForOutputCmd(outChan <-chan runner.OutputLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-outChan
		if !ok {
			// Channel closed, might indicate step finished or error occurred.
			// The stepFinishedMsg will handle the final state.
			return nil
		}
		return outputLineMsg{line}
	}
}

// waitForErrorCmd listens on the error channel for the final result
func waitForErrorCmd(errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		err := <-errChan // Blocks until error (or nil for success) is sent
		return stepFinishedMsg{err}
	}
}


// --- Model Implementation ---

func InitialModel() model {
	// Initialize viewport later once dimensions are known
	return model{
		currentState: stateLoadingProjects,
		cursor:       0,
	}
}

func (m *model) Init() tea.Cmd { // Changed to pointer receiver
	// Still load projects on startup
	return findProjectsCmd()
}

// NOTE: The model struct definition was already updated earlier.
// The duplicate definition below is removed.

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { // Changed to pointer receiver
	var cmds []tea.Cmd // Use a slice for batching commands

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Define header/footer heights for viewport calculation
		headerHeight := 2 // Title + blank line
		footerHeight := 2 // Status/Error + Controls
		if !m.ready {
			// Initialize viewport on first WindowSizeMsg
			m.viewport = viewport.New(m.width, m.height-headerHeight-footerHeight)
			// m.viewport.YPosition = headerHeight // Start drawing below header (handled by View func)
			m.ready = true
		} else {
			// Update viewport size on subsequent resizes
			m.viewport.Width = m.width
			m.viewport.Height = m.height - headerHeight - footerHeight
		}
		// Re-wrap content if viewport size changed
		if m.currentState == stateRunningSequence || m.currentState == stateSequenceError {
			m.viewport.SetContent(m.outputContent)
		}


	case tea.KeyMsg:
		switch m.currentState {
		case stateProjectList:
			// handleProjectListKeys now returns []tea.Cmd
			cmds = append(cmds, m.handleProjectListKeys(msg)...)
		case stateRunningSequence, stateSequenceError:
			// Handle viewport scrolling and returning to list
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				// TODO: Implement command cancellation if possible
				return m, tea.Quit
			}
			if msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc || msg.String() == "b" {
				// Return to project list
				m.currentState = stateProjectList
				m.outputContent = "" // Clear output
				m.lastError = nil
				m.currentSequence = nil
				m.currentStepIndex = 0
				m.viewport.GotoTop() // Reset viewport scroll
				// TODO: Cancel running command if returning while running
			} else {
				// Pass other keys (like arrows, pgup/pgdn) to viewport
				var vpCmd tea.Cmd
				m.viewport, vpCmd = m.viewport.Update(msg)
				cmds = append(cmds, vpCmd)
			}
		default: // Loading projects state
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
		}


	// --- Custom Messages ---

	// Project loading results
	case projectsLoadedMsg:
		m.projects = msg.projects
		m.currentState = stateProjectList
		if len(m.projects) == 0 {
			m.lastError = fmt.Errorf("no projects found in /home/ubuntu/bucket")
			m.currentState = stateSequenceError // Treat as error state
		}

	// Step finished (could be project loading error or sequence step error/success)
	case stepFinishedMsg:
		if m.currentState == stateLoadingProjects { // Handle project loading error
			if msg.err != nil {
				m.lastError = msg.err
				m.currentState = stateSequenceError
			}
			// No command needed here, state transition is enough
		} else if m.currentState == stateRunningSequence { // Handle sequence step completion
			m.outputChan = nil // Stop listening for output from this step
			m.errorChan = nil  // Stop listening for error from this step

			if msg.err != nil {
				// Step failed
				m.lastError = msg.err
				m.currentState = stateSequenceError
				m.outputContent += errorStyle.Render(fmt.Sprintf("\n--- STEP FAILED: %v ---", msg.err)) + "\n"
				m.viewport.SetContent(m.outputContent)
				m.viewport.GotoBottom()
				// No further commands needed, stay in error state
			} else {
				// Step succeeded
				m.outputContent += successStyle.Render(fmt.Sprintf("\n--- Step '%s' Succeeded ---", m.currentSequence[m.currentStepIndex].Name)) + "\n"
				m.currentStepIndex++
				if m.currentStepIndex >= len(m.currentSequence) {
					// Entire sequence finished successfully
					m.outputContent += successStyle.Render("\n--- Action Sequence Completed Successfully ---") + "\n"
					m.viewport.SetContent(m.outputContent)
					m.viewport.GotoBottom()
					// Stay in stateRunningSequence, user presses 'b'/'esc'/etc to go back
				} else {
					// Start the next step in the sequence
					cmds = append(cmds, m.startNextStepCmd())
				}
			}
		}

	// Received channels for the step that just started
	case channelsAvailableMsg:
		if m.currentState == stateRunningSequence {
			m.outputChan = msg.outChan
			m.errorChan = msg.errChan
			// Start listening on both channels immediately
			cmds = append(cmds, waitForOutputCmd(m.outputChan), waitForErrorCmd(m.errorChan))
		}

	// Received a line of output from the current step
	case outputLineMsg:
		if m.currentState == stateRunningSequence && m.outputChan != nil {
			if msg.line.IsError {
				m.outputContent += errorStyle.Render(msg.line.Line) + "\n"
			} else {
				m.outputContent += msg.line.Line + "\n"
			}
			m.viewport.SetContent(m.outputContent)
			m.viewport.GotoBottom()
			// Continue listening for the *next* output line from the same step
			cmds = append(cmds, waitForOutputCmd(m.outputChan))
		}

	} // End switch msg.(type)


	// If viewport isn't ready, don't process its updates yet
	// (This check might be redundant if WindowSizeMsg always comes first)
	// if !m.ready {
	// 	return m, tea.Batch(cmds...)
	// }

	// Pass messages to viewport AFTER potential content updates
	// Note: We already handled viewport updates for KeyMsg within the stateRunningSequence block.
	// Avoid double-updating. Maybe only update viewport here if it wasn't a KeyMsg handled above?
	// Let's simplify: update viewport always, except for the messages we explicitly handle above.
	// if _, ok := msg.(tea.KeyMsg); !ok || (m.currentState != stateRunningSequence && m.currentState != stateSequenceError) {
	// 	var vpCmd tea.Cmd
	// 	m.viewport, vpCmd = m.viewport.Update(msg)
	// 	cmds = append(cmds, vpCmd)
	// }
	// Simpler: Let viewport handle all messages it cares about. Bubble Tea is efficient.
	// But ensure content is set *before* viewport potentially processes scrolling keys.
	// The current structure seems okay as viewport update happens after content setting.


	return m, tea.Batch(cmds...) // Batch all generated commands
} // <<< Added missing closing brace for Update function
// handleProjectListKeys handles key presses when the project list is active.
// Now returns a slice of commands.
func (m *model) handleProjectListKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	switch msg.String() {
	case "ctrl+c", "q":
		cmds = append(cmds, tea.Quit)

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}

	case "u", "d", "r": // Handle actions
		if len(m.projects) > 0 {
			selectedProject := m.projects[m.cursor]
			// Determine the sequence based on the key pressed
			switch msg.String() {
			case "u":
				m.currentSequence = runner.UpSequence(selectedProject.Path)
			case "d":
				m.currentSequence = runner.DownSequence(selectedProject.Path)
			case "r":
				m.currentSequence = runner.RefreshSequence(selectedProject.Path)
			}
			// Transition state and prepare for sequence execution
			m.currentState = stateRunningSequence
			m.currentStepIndex = 0
			m.outputContent = "" // Clear previous output
			m.lastError = nil
			m.viewport.GotoTop() // Reset scroll
			// Start the first step of the sequence
			cmds = append(cmds, m.startNextStepCmd())
		}
	}
	return cmds // Return slice of commands
}

// startNextStepCmd prepares and returns the command to run the next step.
func (m *model) startNextStepCmd() tea.Cmd {
	if m.currentSequence == nil || m.currentStepIndex >= len(m.currentSequence) {
		return nil // Should not happen if called correctly
	}
	step := m.currentSequence[m.currentStepIndex]

	// Adjust step directory if needed (consistency with CLI)
	// Use the project path associated with the *currently selected* project (cursor)
	// This assumes the cursor hasn't changed since the action was initiated.
	// A safer approach might be to store the selected project index when starting the sequence.
	if step.Dir != "" && m.cursor >= 0 && m.cursor < len(m.projects) {
		step.Dir = m.projects[m.cursor].Path // Use the actual project path
	}


	m.outputContent += stepStyle.Render(fmt.Sprintf("\n--- Starting Step: %s ---", step.Name)) + "\n"
	m.viewport.SetContent(m.outputContent)
	m.viewport.GotoBottom()

	// Return the command that will eventually yield channelsAvailableMsg
	return runStepCmd(step)
}


func (m *model) View() string { // Changed to pointer receiver
	if !m.ready {
		return "Initializing..." // Don't render until viewport is ready
	}

	var header, body, footer string

	// --- Header ---
	header = titleStyle.Render("Podman Compose Manager TUI") + "\n"


	// --- Body ---
	bodyContent := strings.Builder{}
	switch m.currentState {
	case stateLoadingProjects:
		bodyContent.WriteString(statusStyle.Render("Loading projects..."))

	case stateProjectList:
		bodyContent.WriteString("Select a project:\n")
		for i, project := range m.projects {
			cursor := "  " // Two spaces for alignment
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
			}
			bodyContent.WriteString(fmt.Sprintf("%s%s\n", cursor, project.Name))
		}

	case stateRunningSequence, stateSequenceError:
		// The viewport handles the main body content in these states
		body = m.viewport.View() // Use viewport directly for body

	}
	// If body wasn't set directly by viewport, use the builder content
	if body == "" {
		body = bodyContent.String()
	}


	// --- Footer ---
	footerContent := strings.Builder{}
	footerContent.WriteString("\n") // Separator line
	switch m.currentState {
	case stateProjectList:
		footerContent.WriteString("[↑/k ↓/j] Navigate | [u] Up | [d] Down | [r] Refresh | [q] Quit")
	case stateRunningSequence:
		// Show status of the running sequence
		if m.currentSequence != nil && m.currentStepIndex < len(m.currentSequence) {
			footerContent.WriteString(statusStyle.Render(fmt.Sprintf("Running step %d/%d: %s...",
				m.currentStepIndex+1,
				len(m.currentSequence),
				m.currentSequence[m.currentStepIndex].Name)))
		} else {
			// Sequence finished successfully
			footerContent.WriteString(successStyle.Render("Sequence finished successfully."))
		}
		footerContent.WriteString("\n[↑/↓ PgUp/PgDn] Scroll | [b/Esc/Enter] Back | [q] Quit")
	case stateSequenceError:
		// Show error message
		if m.lastError != nil {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.lastError)))
		} else {
			footerContent.WriteString(errorStyle.Render("An unknown error occurred."))
		}
		footerContent.WriteString("\n[↑/↓ PgUp/PgDn] Scroll | [b/Esc/Enter] Back | [q] Quit")
	default: // Loading
		footerContent.WriteString("[q] Quit")
	}
	footer = footerContent.String()


	// Combine header, body, footer
	// Ensure body doesn't exceed available height when not using viewport directly
	// This simple join might cause issues if body is too long in list view.
	// A more robust approach uses lipgloss.JoinVertical.
	finalView := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	return finalView
}

// Removed unused helper comment related to filepath
