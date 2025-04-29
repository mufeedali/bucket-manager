package ui

import (
	"fmt"
	"podman-compose-manager/internal/discovery"
	"podman-compose-manager/internal/runner" // Ensure runner is imported
	"strings"

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
	// Status specific styles
	statusUpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green
	statusDownStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // Red
	statusPartialStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange/Brown for status error
	statusLoadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Grey
)


type state int

const (
	stateLoadingProjects state = iota // Initial state, fetching projects
	stateProjectList                  // Displaying the list of projects
	stateRunningSequence              // An action sequence (up/down/refresh) is running
	stateSequenceError                // An error occurred during a sequence
	stateProjectDetails               // Displaying details for a single project
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
	// Status tracking
	projectStatuses map[string]runner.ProjectRuntimeInfo // Map project path to its status
	loadingStatus   map[string]bool                      // Map project path to loading state
	// Details view
	detailedProject *discovery.Project // Project being viewed in detail
	sequenceProject *discovery.Project // Project the current sequence is running for
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

type projectStatusLoadedMsg struct { // New message for when a project's status is loaded
	projectPath string
	statusInfo  runner.ProjectRuntimeInfo
}


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

// fetchProjectStatusCmd fetches the status for a single project asynchronously.
func fetchProjectStatusCmd(projectPath string) tea.Cmd {
	return func() tea.Msg {
		statusInfo := runner.GetProjectStatus(projectPath)
		return projectStatusLoadedMsg{projectPath: projectPath, statusInfo: statusInfo}
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
		currentState:    stateLoadingProjects,
		cursor:          0,
		projectStatuses: make(map[string]runner.ProjectRuntimeInfo),
		loadingStatus:   make(map[string]bool),
		detailedProject: nil,
		sequenceProject: nil,
	}
}

func (m *model) Init() tea.Cmd { // Changed to pointer receiver
	// Still load projects on startup
	return findProjectsCmd()
}

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
				// Store the project path before clearing sequence info
				projectPathToRefresh := ""
				if m.sequenceProject != nil {
					projectPathToRefresh = m.sequenceProject.Path
				}
				// Clear sequence state
				m.currentState = stateProjectList
				m.outputContent = "" // Clear output
				m.lastError = nil
				m.currentSequence = nil
				m.currentStepIndex = 0
				m.sequenceProject = nil // Clear the sequence project
				m.viewport.GotoTop() // Reset viewport scroll
				// TODO: Cancel running command if returning while running

				// Trigger status refresh for the project that was acted upon
				if projectPathToRefresh != "" && !m.loadingStatus[projectPathToRefresh] {
					m.loadingStatus[projectPathToRefresh] = true
					cmds = append(cmds, fetchProjectStatusCmd(projectPathToRefresh))
				}
			} else {
				// Pass other keys (like arrows, pgup/pgdn) to viewport
				var vpCmd tea.Cmd
				m.viewport, vpCmd = m.viewport.Update(msg)
				cmds = append(cmds, vpCmd)
			}
		case stateProjectDetails:
			// Handle keys for the details view
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
			if msg.Type == tea.KeyEsc || msg.String() == "b" {
				// Return to project list
				m.currentState = stateProjectList
				m.detailedProject = nil // Clear detailed project
			}
			// Potentially add scrolling later if details become long
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
		} else {
			// Trigger status fetch for all loaded projects initially
			for _, p := range m.projects {
				if !m.loadingStatus[p.Path] { // Check if not already loading (shouldn't be, but safe)
					m.loadingStatus[p.Path] = true
					cmds = append(cmds, fetchProjectStatusCmd(p.Path))
				}
			}
		}

	// A project's status has been loaded
	case projectStatusLoadedMsg:
		m.loadingStatus[msg.projectPath] = false // Mark as not loading anymore
		m.projectStatuses[msg.projectPath] = msg.statusInfo
		// No command needed, just update state

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

					// Trigger status refresh for the completed project
					if m.sequenceProject != nil && !m.loadingStatus[m.sequenceProject.Path] {
						m.loadingStatus[m.sequenceProject.Path] = true
						cmds = append(cmds, fetchProjectStatusCmd(m.sequenceProject.Path))
					}
					// Don't clear sequenceProject here, needed if user presses 'b'
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
			// Fetch status for newly selected project if needed
			selectedProject := m.projects[m.cursor]
			if _, loaded := m.projectStatuses[selectedProject.Path]; !loaded && !m.loadingStatus[selectedProject.Path] {
				m.loadingStatus[selectedProject.Path] = true
				cmds = append(cmds, fetchProjectStatusCmd(selectedProject.Path))
			}
		}

	case "down", "j":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
			// Fetch status for newly selected project if needed
			selectedProject := m.projects[m.cursor]
			if _, loaded := m.projectStatuses[selectedProject.Path]; !loaded && !m.loadingStatus[selectedProject.Path] {
				m.loadingStatus[selectedProject.Path] = true
				cmds = append(cmds, fetchProjectStatusCmd(selectedProject.Path))
			}
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
	case "enter": // Show project details
		if len(m.projects) > 0 {
			m.detailedProject = &m.projects[m.cursor] // Store pointer to the selected project
			m.currentState = stateProjectDetails
			// Ensure status is fetched if not already available/loading
			if _, loaded := m.projectStatuses[m.detailedProject.Path]; !loaded && !m.loadingStatus[m.detailedProject.Path] {
				m.loadingStatus[m.detailedProject.Path] = true
				cmds = append(cmds, fetchProjectStatusCmd(m.detailedProject.Path))
			}
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

	// Store the project this sequence is for, only when starting the first step
	if m.currentStepIndex == 0 && m.cursor >= 0 && m.cursor < len(m.projects) {
		m.sequenceProject = &m.projects[m.cursor]
	}

	// Adjust step directory if needed (consistency with CLI)
	if step.Dir != "" && m.sequenceProject != nil {
		step.Dir = m.sequenceProject.Path // Use the actual project path from stored sequenceProject
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
			// Determine status string
			statusStr := ""
			if m.loadingStatus[project.Path] {
				statusStr = statusLoadingStyle.Render(" [loading...]")
			} else if statusInfo, ok := m.projectStatuses[project.Path]; ok {
				switch statusInfo.OverallStatus {
				case runner.StatusUp:
					statusStr = statusUpStyle.Render(" [UP]")
				case runner.StatusDown:
					statusStr = statusDownStyle.Render(" [DOWN]")
				case runner.StatusPartial:
					statusStr = statusPartialStyle.Render(" [PARTIAL]")
				case runner.StatusError:
					// Maybe add tooltip or detail later for the error
					statusStr = statusErrorStyle.Render(" [ERROR]")
				default: // Unknown or empty status
					statusStr = statusLoadingStyle.Render(" [?]") // Use loading style for unknown
				}
			} else {
				// Not loading and no status yet (should be fetched on load/scroll)
				statusStr = statusLoadingStyle.Render(" [?]")
			}

			bodyContent.WriteString(fmt.Sprintf("%s%s%s\n", cursor, project.Name, statusStr))
		}

	case stateRunningSequence, stateSequenceError:
		// The viewport handles the main body content in these states
		body = m.viewport.View() // Use viewport directly for body

	case stateProjectDetails:
		if m.detailedProject == nil {
			bodyContent.WriteString(errorStyle.Render("Error: No project selected for details."))
		} else {
			bodyContent.WriteString(titleStyle.Render(fmt.Sprintf("Details for: %s", m.detailedProject.Name)) + "\n\n")
			statusStr := ""
			statusInfo, loaded := m.projectStatuses[m.detailedProject.Path]
			isLoading := m.loadingStatus[m.detailedProject.Path]

			if isLoading {
				statusStr = statusLoadingStyle.Render(" [loading...]")
				bodyContent.WriteString(fmt.Sprintf("Overall Status:%s\n", statusStr))
			} else if !loaded {
				statusStr = statusLoadingStyle.Render(" [?]")
				bodyContent.WriteString(fmt.Sprintf("Overall Status:%s\n", statusStr))
			} else {
				// Status is loaded
				switch statusInfo.OverallStatus {
				case runner.StatusUp:
					statusStr = statusUpStyle.Render(" [UP]")
				case runner.StatusDown:
					statusStr = statusDownStyle.Render(" [DOWN]")
				case runner.StatusPartial:
					statusStr = statusPartialStyle.Render(" [PARTIAL]")
				case runner.StatusError:
					statusStr = statusErrorStyle.Render(" [ERROR]")
				default:
					statusStr = statusLoadingStyle.Render(" [?]")
				}
				bodyContent.WriteString(fmt.Sprintf("Overall Status:%s\n", statusStr))

				if statusInfo.Error != nil {
					bodyContent.WriteString(errorStyle.Render(fmt.Sprintf("  Error fetching status: %v\n", statusInfo.Error)))
				}

				if len(statusInfo.Containers) > 0 {
					bodyContent.WriteString("\nContainers:\n")
					// Simple header for now
					bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "SERVICE", "CONTAINER NAME", "STATUS"))
					bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "-------", "--------------", "------"))
					for _, c := range statusInfo.Containers {
						// Determine container status color
						isUp := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
						statusRenderFunc := statusDownStyle.Render // Default to down/red
						if isUp {
							statusRenderFunc = statusUpStyle.Render
						}
						bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", c.Service, c.Name, statusRenderFunc(c.Status)))
					}
				} else if statusInfo.OverallStatus != runner.StatusError {
					// Only show "no containers" if status isn't already error
					bodyContent.WriteString("\n  (No containers found or running)\n")
				}
			}
		}

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
		footerContent.WriteString("[↑/k ↓/j] Navigate | [Enter] Details | [u] Up | [d] Down | [r] Refresh | [q] Quit") // Added Enter
	case stateRunningSequence:
		// Show status of the running sequence
		projectName := ""
		if m.sequenceProject != nil {
			projectName = fmt.Sprintf(" for %s", m.sequenceProject.Name)
		}
		if m.currentSequence != nil && m.currentStepIndex < len(m.currentSequence) {
			footerContent.WriteString(statusStyle.Render(fmt.Sprintf("Running step %d/%d%s: %s...",
				m.currentStepIndex+1,
				len(m.currentSequence),
				projectName,
				m.currentSequence[m.currentStepIndex].Name)))
		} else {
			// Sequence finished successfully
			footerContent.WriteString(successStyle.Render(fmt.Sprintf("Sequence finished successfully%s.", projectName)))
		}
		footerContent.WriteString("\n[↑/↓ PgUp/PgDn] Scroll | [b/Esc/Enter] Back | [q] Quit")
	case stateSequenceError:
		// Show error message
		projectName := ""
		if m.sequenceProject != nil {
			projectName = fmt.Sprintf(" for %s", m.sequenceProject.Name)
		}
		if m.lastError != nil {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("Error%s: %v", projectName, m.lastError)))
		} else {
			footerContent.WriteString(errorStyle.Render(fmt.Sprintf("An unknown error occurred%s.", projectName)))
		}
		footerContent.WriteString("\n[↑/↓ PgUp/PgDn] Scroll | [b/Esc/Enter] Back | [q] Quit")
	case stateProjectDetails:
		footerContent.WriteString("[b/Esc] Back to List | [q] Quit")
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
