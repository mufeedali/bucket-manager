package ui

import (
	"fmt"
	"podman-compose-manager/internal/discovery"
	"podman-compose-manager/internal/runner"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss" // For styling
)

// Define styles
var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")) // Purple
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))             // Red
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))            // Blue
	stepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))            // Yellow
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))            // Green
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))             // Magenta
	// Status specific styles
	statusUpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // Green
	statusDownStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))   // Red
	statusPartialStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))  // Yellow
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange/Brown for status error
	statusLoadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))   // Grey
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

type projectsLoadedMsg struct {
	projects []discovery.Project
}

type outputLineMsg struct {
	line runner.OutputLine
}

type stepFinishedMsg struct {
	err error // nil if successful
}

type sequenceFinishedMsg struct{}

type projectStatusLoadedMsg struct {
	projectPath string
	statusInfo  runner.ProjectRuntimeInfo
}

// --- Commands ---

func findProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			// Return error message if root dir not found
			return stepFinishedMsg{fmt.Errorf("failed to find compose directory: %w", err)}
		}
		projs, err := discovery.FindProjects(rootDir)
		if err != nil {
			return stepFinishedMsg{fmt.Errorf("failed to load projects from %s: %w", rootDir, err)}
		}
		return projectsLoadedMsg{projs}
	}
}

func fetchProjectStatusCmd(projectPath string) tea.Cmd {
	return func() tea.Msg {
		statusInfo := runner.GetProjectStatus(projectPath)
		return projectStatusLoadedMsg{projectPath: projectPath, statusInfo: statusInfo}
	}
}

type channelsAvailableMsg struct {
	outChan <-chan runner.OutputLine
	errChan <-chan error
}

func runStepCmd(step runner.CommandStep) tea.Cmd {
	return func() tea.Msg {
		outChan, errChan := runner.StreamCommand(step)
		return channelsAvailableMsg{outChan: outChan, errChan: errChan}
	}
}

func waitForOutputCmd(outChan <-chan runner.OutputLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-outChan
		if !ok {
			return nil // Channel closed
		}
		return outputLineMsg{line}
	}
}

func waitForErrorCmd(errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		err := <-errChan
		return stepFinishedMsg{err}
	}
}

// --- Model Implementation ---

func InitialModel() model {
	return model{
		currentState:    stateLoadingProjects,
		cursor:          0,
		projectStatuses: make(map[string]runner.ProjectRuntimeInfo),
		loadingStatus:   make(map[string]bool),
		detailedProject: nil,
		sequenceProject: nil,
	}
}

func (m *model) Init() tea.Cmd {
	return findProjectsCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2
		footerHeight := 2
		if !m.ready {
			m.viewport = viewport.New(m.width, m.height-headerHeight-footerHeight)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = m.height - headerHeight - footerHeight
		}
		if m.currentState == stateRunningSequence || m.currentState == stateSequenceError {
			m.viewport.SetContent(m.outputContent)
		}

	case tea.KeyMsg:
		switch m.currentState {
		case stateProjectList:
			cmds = append(cmds, m.handleProjectListKeys(msg)...)

		case stateRunningSequence, stateSequenceError:
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit // TODO: Add command cancellation later?
			}
			if msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc || msg.String() == "b" {
				projectPathToRefresh := ""
				if m.sequenceProject != nil {
					projectPathToRefresh = m.sequenceProject.Path
				}
				m.currentState = stateProjectList
				m.outputContent = ""
				m.lastError = nil
				m.currentSequence = nil
				m.currentStepIndex = 0
				m.sequenceProject = nil
				m.viewport.GotoTop()

				if projectPathToRefresh != "" && !m.loadingStatus[projectPathToRefresh] {
					m.loadingStatus[projectPathToRefresh] = true
					cmds = append(cmds, fetchProjectStatusCmd(projectPathToRefresh))
				}
			}
			// Viewport scrolling is handled by the viewport update below

		case stateProjectDetails:
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
			if msg.Type == tea.KeyEsc || msg.String() == "b" {
				m.currentState = stateProjectList
				m.detailedProject = nil
			}

		default: // Loading state
			if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
		}

	// --- Custom Message Handlers ---
	case projectsLoadedMsg:
		m.projects = msg.projects
		m.currentState = stateProjectList
		if len(m.projects) == 0 {
			rootDir, err := discovery.GetComposeRootDirectory()
			if err != nil { // Handle error getting root dir for message
				rootDir = "compose directory" // Fallback message
			}
			m.lastError = fmt.Errorf("no projects found in %s", rootDir)
			m.currentState = stateSequenceError
		} else {
			// Trigger initial status fetch for all projects
			for _, p := range m.projects {
				if !m.loadingStatus[p.Path] {
					m.loadingStatus[p.Path] = true
					cmds = append(cmds, fetchProjectStatusCmd(p.Path))
				}
			}
		}

	case projectStatusLoadedMsg:
		m.loadingStatus[msg.projectPath] = false
		m.projectStatuses[msg.projectPath] = msg.statusInfo

	case stepFinishedMsg:
		if m.currentState == stateLoadingProjects { // Project loading failed
			if msg.err != nil {
				m.lastError = msg.err
				m.currentState = stateSequenceError
			}
		} else if m.currentState == stateRunningSequence { // Sequence step finished
			m.outputChan = nil // Stop listening
			m.errorChan = nil

			if msg.err != nil { // Step failed
				m.lastError = msg.err
				m.currentState = stateSequenceError
				m.outputContent += errorStyle.Render(fmt.Sprintf("\n--- STEP FAILED: %v ---", msg.err)) + "\n"
				m.viewport.SetContent(m.outputContent)
				m.viewport.GotoBottom()
			} else { // Step succeeded
				m.outputContent += successStyle.Render(fmt.Sprintf("\n--- Step '%s' Succeeded ---", m.currentSequence[m.currentStepIndex].Name)) + "\n"
				m.currentStepIndex++
				if m.currentStepIndex >= len(m.currentSequence) { // Sequence complete
					m.outputContent += successStyle.Render("\n--- Action Sequence Completed Successfully ---") + "\n"
					m.viewport.SetContent(m.outputContent)
					m.viewport.GotoBottom()
					if m.sequenceProject != nil && !m.loadingStatus[m.sequenceProject.Path] {
						m.loadingStatus[m.sequenceProject.Path] = true
						cmds = append(cmds, fetchProjectStatusCmd(m.sequenceProject.Path))
					}
				} else { // Start next step
					cmds = append(cmds, m.startNextStepCmd())
				}
			}
		}

	case channelsAvailableMsg: // Received channels for a new step
		if m.currentState == stateRunningSequence {
			m.outputChan = msg.outChan
			m.errorChan = msg.errChan
			cmds = append(cmds, waitForOutputCmd(m.outputChan), waitForErrorCmd(m.errorChan))
		}

	case outputLineMsg: // Received output from running step
		if m.currentState == stateRunningSequence && m.outputChan != nil {
			if msg.line.IsError {
				m.outputContent += errorStyle.Render(msg.line.Line) + "\n"
			} else {
				m.outputContent += msg.line.Line + "\n"
			}
			m.viewport.SetContent(m.outputContent)
			m.viewport.GotoBottom()
			cmds = append(cmds, waitForOutputCmd(m.outputChan))
		}

	} // End main message switch

	// Update viewport - this handles scrolling etc. based on the msg
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *model) handleProjectListKeys(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	switch msg.String() {
	case "ctrl+c", "q":
		cmds = append(cmds, tea.Quit)

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			selectedProject := m.projects[m.cursor]
			if _, loaded := m.projectStatuses[selectedProject.Path]; !loaded && !m.loadingStatus[selectedProject.Path] {
				m.loadingStatus[selectedProject.Path] = true
				cmds = append(cmds, fetchProjectStatusCmd(selectedProject.Path))
			}
		}

	case "down", "j":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
			selectedProject := m.projects[m.cursor]
			if _, loaded := m.projectStatuses[selectedProject.Path]; !loaded && !m.loadingStatus[selectedProject.Path] {
				m.loadingStatus[selectedProject.Path] = true
				cmds = append(cmds, fetchProjectStatusCmd(selectedProject.Path))
			}
		}

	case "u", "d", "r": // Project actions
		if len(m.projects) > 0 {
			selectedProject := m.projects[m.cursor]
			switch msg.String() {
			case "u":
				m.currentSequence = runner.UpSequence(selectedProject.Path)
			case "d":
				m.currentSequence = runner.DownSequence(selectedProject.Path)
			case "r":
				m.currentSequence = runner.RefreshSequence(selectedProject.Path)
			}
			m.currentState = stateRunningSequence
			m.currentStepIndex = 0
			m.outputContent = ""
			m.lastError = nil
			m.viewport.GotoTop()
			cmds = append(cmds, m.startNextStepCmd())
		}
	case "enter": // Show project details
		if len(m.projects) > 0 {
			m.detailedProject = &m.projects[m.cursor]
			m.currentState = stateProjectDetails
			if _, loaded := m.projectStatuses[m.detailedProject.Path]; !loaded && !m.loadingStatus[m.detailedProject.Path] {
				m.loadingStatus[m.detailedProject.Path] = true
				cmds = append(cmds, fetchProjectStatusCmd(m.detailedProject.Path))
			}
		}
	}
	return cmds
}

func (m *model) startNextStepCmd() tea.Cmd {
	if m.currentSequence == nil || m.currentStepIndex >= len(m.currentSequence) {
		return nil
	}
	step := m.currentSequence[m.currentStepIndex]

	if m.currentStepIndex == 0 && m.cursor >= 0 && m.cursor < len(m.projects) {
		m.sequenceProject = &m.projects[m.cursor]
	}

	if step.Dir != "" && m.sequenceProject != nil {
		step.Dir = m.sequenceProject.Path
	}

	m.outputContent += stepStyle.Render(fmt.Sprintf("\n--- Starting Step: %s ---", step.Name)) + "\n"
	m.viewport.SetContent(m.outputContent)
	m.viewport.GotoBottom()

	return runStepCmd(step)
}

func (m *model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var header, body, footer string

	header = titleStyle.Render("Podman Compose Manager TUI") + "\n"

	bodyContent := strings.Builder{}
	switch m.currentState {
	case stateLoadingProjects:
		bodyContent.WriteString(statusStyle.Render("Loading projects..."))

	case stateProjectList:
		bodyContent.WriteString("Select a project:\n")
		for i, project := range m.projects {
			cursor := "  "
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
			}
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
					statusStr = statusErrorStyle.Render(" [ERROR]")
				default:
					statusStr = statusLoadingStyle.Render(" [?]")
				}
			} else {
				statusStr = statusLoadingStyle.Render(" [?]")
			}
			bodyContent.WriteString(fmt.Sprintf("%s%s%s\n", cursor, project.Name, statusStr))
		}

	case stateRunningSequence, stateSequenceError:
		body = m.viewport.View()

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
					bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "SERVICE", "CONTAINER NAME", "STATUS"))
					bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", "-------", "--------------", "------"))
					for _, c := range statusInfo.Containers {
						isUp := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
						statusRenderFunc := statusDownStyle.Render
						if isUp {
							statusRenderFunc = statusUpStyle.Render
						}
						bodyContent.WriteString(fmt.Sprintf("  %-20s %-30s %s\n", c.Service, c.Name, statusRenderFunc(c.Status)))
					}
				} else if statusInfo.OverallStatus != runner.StatusError {
					bodyContent.WriteString("\n  (No containers found or running)\n")
				}
			}
		}

	}
	if body == "" {
		body = bodyContent.String()
	}

	footerContent := strings.Builder{}
	footerContent.WriteString("\n")
	switch m.currentState {
	case stateProjectList:
		footerContent.WriteString("[↑/k ↓/j] Navigate | [Enter] Details | [u] Up | [d] Down | [r] Refresh | [q] Quit")
	case stateRunningSequence:
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
			footerContent.WriteString(successStyle.Render(fmt.Sprintf("Sequence finished successfully%s.", projectName)))
		}
		footerContent.WriteString("\n[↑/↓ PgUp/PgDn] Scroll | [b/Esc/Enter] Back | [q] Quit")
	case stateSequenceError:
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

	// Combine header, body, footer using lipgloss
	finalView := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	return finalView
}
