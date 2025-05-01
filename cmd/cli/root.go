// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"bucket-manager/internal/ssh"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	sshManager         *ssh.Manager // SSH Manager instance
	statusColor        = color.New(color.FgCyan)
	errorColor         = color.New(color.FgRed)
	stepColor          = color.New(color.FgYellow)
	successColor       = color.New(color.FgGreen)
	statusUpColor      = color.New(color.FgGreen)
	statusDownColor    = color.New(color.FgRed)
	statusPartialColor = color.New(color.FgYellow)
	statusErrorColor   = color.New(color.FgMagenta)
	identifierColor    = color.New(color.FgBlue) // For project identifiers
)

// findProjectByIdentifier searches the list of projects for one matching the identifier.
// Identifier can be "projectName" or "projectName (serverName)".
// Returns an error if not found or if the short name is ambiguous.
func findProjectByIdentifier(projects []discovery.Project, identifier string) (discovery.Project, error) {
	identifier = strings.TrimSpace(identifier)
	targetName := identifier
	targetServer := ""

	// Check if identifier includes server name like "project (server)"
	if strings.HasSuffix(identifier, ")") {
		lastOpenParen := strings.LastIndex(identifier, " (")
		if lastOpenParen > 0 {
			targetName = strings.TrimSpace(identifier[:lastOpenParen])
			targetServer = strings.TrimSpace(identifier[lastOpenParen+2 : len(identifier)-1])
		}
	}

	var foundProject *discovery.Project
	var potentialMatches []discovery.Project

	for i := range projects {
		p := projects[i] // Create a local copy for the loop iteration
		if p.Name == targetName {
			if targetServer == "" {
				// No server specified in identifier, add to potential matches
				potentialMatches = append(potentialMatches, p)
				foundProject = &p // Tentatively set foundProject
			} else if p.ServerName == targetServer {
				// Exact match for name and server
				return p, nil
			}
		}
	}

	// Post-loop evaluation for cases where server wasn't specified
	if targetServer == "" {
		if len(potentialMatches) == 1 {
			return *foundProject, nil // Exactly one match found
		}
		if len(potentialMatches) > 1 {
			// Ambiguous short name
			options := []string{}
			for _, pm := range potentialMatches {
				options = append(options, fmt.Sprintf("%s (%s)", pm.Name, pm.ServerName))
			}
			return discovery.Project{}, fmt.Errorf("project name '%s' is ambiguous, please specify one of: %s", targetName, strings.Join(options, ", "))
		}
	}

	// If we reach here, no match was found (either specific or general)
	return discovery.Project{}, fmt.Errorf("project '%s' not found", identifier)
}

// projectCompletionFunc provides dynamic completion for project identifiers "projectName (serverName)".
func projectCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Completion needs to be synchronous. Read from channels until closed.
	projectChan, errorChan, _ := discovery.FindProjects()
	var projects []discovery.Project
	var errors []error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for p := range projectChan {
			projects = append(projects, p)
		}
	}()
	go func() {
		defer wg.Done()
		for e := range errorChan {
			errors = append(errors, e)
		}
	}()
	wg.Wait()

	if len(errors) > 0 {
		// Log first error to stderr for debugging completion issues
		fmt.Fprintf(os.Stderr, "completion error finding projects: %v\n", errors[0])
	}

	var projectIdentifiers []string
	for _, p := range projects {
		identifier := fmt.Sprintf("%s (%s)", p.Name, p.ServerName)
		// Only suggest projects that start with the currently typed string
		if strings.HasPrefix(identifier, toComplete) {
			projectIdentifiers = append(projectIdentifiers, identifier)
		} else if strings.HasPrefix(p.Name, toComplete) && !strings.Contains(toComplete, "(") {
			// Also suggest if just the name matches and user hasn't started typing server
			projectIdentifiers = append(projectIdentifiers, identifier)
		}
	}

	return projectIdentifiers, cobra.ShellCompDirectiveNoFileComp
}

var rootCmd = &cobra.Command{
	Use:   "bm",
	Short: "Bucket Manager CLI",
	Long: `A command-line interface to manage multiple Podman Compose projects.

Discovers projects in standard local directories (~/bucket, ~/compose-bucket)
and on remote hosts configured via SSH (~/.config/bucket-manager/config.yaml).`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := config.EnsureConfigDir(); err != nil {
			return fmt.Errorf("failed to ensure config directory: %w", err)
		}
		sshManager = ssh.NewManager()
		discovery.InitSSHManager(sshManager)
		runner.InitSSHManager(sshManager)
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if sshManager != nil {
			sshManager.CloseAll()
		}
		return nil
	},
}

// RunCLI executes the Cobra CLI application.
func RunCLI() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(refreshCmd)
	rootCmd.AddCommand(statusCmd)
}

// --- Subcommands ---

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered Podman Compose projects (local and remote)",
	Run: func(cmd *cobra.Command, args []string) {
		statusColor.Println("Discovering projects...")
		projectChan, errorChan, _ := discovery.FindProjects()

		var collectedErrors []error
		var projectsFound bool
		var wg sync.WaitGroup
		wg.Add(1)

		// Goroutine to collect errors and print them as they arrive
		go func() {
			defer wg.Done()
			for err := range errorChan {
				collectedErrors = append(collectedErrors, err)
				errorColor.Fprintf(os.Stderr, "Error during discovery: %v\n", err)
			}
		}()

		// Process projects as they arrive
		fmt.Println("\nDiscovered projects:")

		// Start spinner while waiting for all projects
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Color("cyan")
		s.Suffix = " Loading remote projects..."
		s.Start()

		for project := range projectChan {
			s.Stop()
			projectsFound = true
			fmt.Printf("- %s (%s)\n", project.Name, identifierColor.Sprint(project.ServerName))
			s.Restart()
		}
		s.Stop()

		wg.Wait()

		// Final status messages
		if !projectsFound && len(collectedErrors) == 0 {
			fmt.Println("\nNo Podman Compose projects found locally or on configured remote hosts.")
		} else if !projectsFound && len(collectedErrors) > 0 {
			// Error message was already printed when the error arrived
			fmt.Println("\nNo projects discovered successfully.")
		}

		// Exit with non-zero status if errors occurred during discovery
		if len(collectedErrors) > 0 {
			os.Exit(1)
		}
	},
}

// runProjectAction is a helper to reduce repetition in up/down/refresh commands
func runProjectAction(action string, args []string) {
	if len(args) != 1 {
		errorColor.Fprintf(os.Stderr, "Error: requires exactly one project identifier argument.\n")
		os.Exit(1)
	}
	projectIdentifier := args[0]

	statusColor.Println("Discovering projects...")
	// Collect all projects and errors first for action commands
	projectChan, errorChan, _ := discovery.FindProjects()
	var allProjects []discovery.Project
	var errors []error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for p := range projectChan {
			allProjects = append(allProjects, p)
		}
	}()
	go func() {
		defer wg.Done()
		for e := range errorChan {
			errors = append(errors, e)
		}
	}()
	wg.Wait()

	if len(errors) > 0 {
		errorColor.Fprintln(os.Stderr, "\nErrors during project discovery:")
		for _, err := range errors {
			errorColor.Fprintf(os.Stderr, "- %v\n", err)
		}
		// Exit even if some projects were found, as the target might be missing due to error
		os.Exit(1)
	}
	if len(allProjects) == 0 {
		errorColor.Fprintf(os.Stderr, "\nError: No projects found.\n")
		os.Exit(1)
	}

	targetProject, err := findProjectByIdentifier(allProjects, projectIdentifier)
	if err != nil {
		errorColor.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	statusColor.Printf("Executing '%s' action for project: %s (%s)\n", action, targetProject.Name, identifierColor.Sprint(targetProject.ServerName))

	var sequence []runner.CommandStep
	switch action {
	case "up":
		sequence = runner.UpSequence(targetProject)
	case "down":
		sequence = runner.DownSequence(targetProject)
	case "refresh":
		sequence = runner.RefreshSequence(targetProject)
	default:
		errorColor.Fprintf(os.Stderr, "Internal Error: Invalid action '%s'\n", action)
		os.Exit(1)
	}

	err = runSequence(targetProject, sequence)
	if err != nil {
		errorColor.Fprintf(os.Stderr, "\n'%s' action failed for %s (%s): %v\n", action, targetProject.Name, targetProject.ServerName, err)
		os.Exit(1)
	}
	successColor.Printf("'%s' action completed successfully for %s (%s).\n", action, targetProject.Name, identifierColor.Sprint(targetProject.ServerName))
}

var upCmd = &cobra.Command{
	Use:               "up <project-identifier>",
	Short:             "Run 'pull' and 'up -d' for a project",
	Example:           "  bm up my-local-app\n  bm up 'remote-app (server1)'",
	Args:              cobra.ExactArgs(1), // Validated in runProjectAction
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runProjectAction("up", args)
	},
}

var downCmd = &cobra.Command{
	Use:               "down <project-identifier>",
	Short:             "Run 'podman compose down' for a project",
	Example:           "  bm down my-local-app\n  bm down 'remote-app (server1)'",
	Args:              cobra.ExactArgs(1), // Validated in runProjectAction
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runProjectAction("down", args)
	},
}

var refreshCmd = &cobra.Command{
	Use:               "refresh <project-identifier>",
	Short:             "Run 'pull', 'down', 'up', and maybe 'prune' for a project",
	Long:              `Runs 'pull', 'down', 'up -d' for the specified project. Additionally runs 'podman system prune -af' locally if the target project is local.`,
	Example:           "  bm refresh my-local-app\n  bm refresh 'remote-app (server1)'",
	Args:              cobra.ExactArgs(1), // Validated in runProjectAction
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runProjectAction("refresh", args)
	},
}

// --- statusCmd ---
var statusCmd = &cobra.Command{
	Use:   "status [project-identifier]",
	Short: "Show the status of containers for one or all projects",
	Long: `Shows the status of Podman Compose containers for local and remote projects.
If a project identifier (e.g., my-app or 'remote-app (server1)') is provided, shows status for that specific project.
Otherwise, shows status for all discovered projects.`,
	Example:           "  bm status\n  bm status my-local-app\n  bm status 'remote-app (server1)'",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		var collectedErrors []error
		var projectsFound bool
		var specificProjectIdentifier string
		scanAll := len(args) == 0
		if !scanAll {
			specificProjectIdentifier = args[0]
		}

		statusColor.Println("Discovering projects and checking status...")
		projectChan, errorChan, _ := discovery.FindProjects()
		statusChan := make(chan runner.ProjectRuntimeInfo, 10)
		var errWg sync.WaitGroup
		var statusWg sync.WaitGroup
		errWg.Add(1)

		// Goroutine to collect discovery errors
		go func() {
			defer errWg.Done()
			for err := range errorChan {
				collectedErrors = append(collectedErrors, err)
				// Print error directly without stopping spinner
				errorColor.Fprintf(os.Stderr, "\nError during discovery: %v\n", err)
			}
		}()

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Color("cyan")
		s.Suffix = " Discovering and checking status..."
		s.Start()

		// Launch status check goroutines as projects are discovered
		go func() {
			for project := range projectChan {
				projectsFound = true
				processThisProject := scanAll
				if !scanAll {
					if project.Identifier() == specificProjectIdentifier || project.Name == specificProjectIdentifier {
						processThisProject = true
					}
				}

				if processThisProject {
					statusWg.Add(1)
					go func(p discovery.Project) { // Pass project copy to goroutine
						defer statusWg.Done()
						statusInfo := runner.GetProjectStatus(p)
						statusChan <- statusInfo
					}(project)
				}
			}

			// All projects discovered, wait for status checks to finish, then close statusChan
			statusWg.Wait()
			close(statusChan)
		}()

		// Process status results as they arrive
		for statusInfo := range statusChan {
			s.Stop()

			fmt.Printf("\nProject: %s (%s) ", statusInfo.Project.Name, identifierColor.Sprint(statusInfo.Project.ServerName))
			switch statusInfo.OverallStatus {
			case runner.StatusUp:
				statusUpColor.Printf("[%s]\n", statusInfo.OverallStatus)
			case runner.StatusDown:
				statusDownColor.Printf("[%s]\n", statusInfo.OverallStatus)
			case runner.StatusPartial:
				statusPartialColor.Printf("[%s]\n", statusInfo.OverallStatus)
			case runner.StatusError:
				statusErrorColor.Printf("[%s]\n", statusInfo.OverallStatus)
				if statusInfo.Error != nil {
					errorColor.Fprintf(os.Stderr, "  Error checking status: %v\n", statusInfo.Error)
				} else {
					errorColor.Fprintf(os.Stderr, "  Unknown error checking status.\n")
				}
			default:
				fmt.Printf("[%s]\n", statusInfo.OverallStatus)
			}

			// Display container details
			if statusInfo.OverallStatus != runner.StatusDown && len(statusInfo.Containers) > 0 {
				fmt.Println("  Containers:")
				fmt.Printf("    %-25s %-35s %s\n", "SERVICE", "CONTAINER NAME", "STATUS")
				fmt.Printf("    %-25s %-35s %s\n", strings.Repeat("-", 25), strings.Repeat("-", 35), strings.Repeat("-", 6))
				for _, c := range statusInfo.Containers {
					isUp := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
					statusPrinter := statusDownColor
					if isUp {
						statusPrinter = statusUpColor
					}
					fmt.Printf("    %-25s %-35s %s\n", c.Service, c.Name, statusPrinter.Sprint(c.Status))
				}
			}

			s.Restart()
		}
		s.Stop()

		// Wait for error collection goroutine to finish *after* processing all statuses
		errWg.Wait()

		if !projectsFound && len(collectedErrors) == 0 {
			fmt.Println("\nNo Podman Compose projects found locally or on configured remote hosts.")
		} else if !projectsFound && len(collectedErrors) > 0 {
			fmt.Println("\nNo projects discovered successfully.")
		} else if !scanAll && !projectsFound { // Specific project requested but not found
			errorColor.Fprintf(os.Stderr, "\nError: Project '%s' not found.\n", specificProjectIdentifier)
			os.Exit(1) // Exit non-zero if specific project not found
		}

		// Exit with non-zero code if any discovery error occurred
		if len(collectedErrors) > 0 {
			os.Exit(1)
		}
	},
}

// runSequence executes a series of command steps for a given project, streaming output.
func runSequence(project discovery.Project, sequence []runner.CommandStep) error {
	for _, step := range sequence {
		// Include project identifier in step message
		stepColor.Printf("\n--- Running Step: %s for %s (%s) ---\n", step.Name, project.Name, identifierColor.Sprint(project.ServerName))

		// StreamCommand now correctly handles local/remote based on step.Project
		outChan, errChan := runner.StreamCommand(step)

		var stepErr error
		outputDone := make(chan struct{})

		go func() {
			defer close(outputDone)
			for line := range outChan {
				if line.IsError {
					errorColor.Fprintln(os.Stderr, line.Line)
				} else {
					fmt.Println(line.Line)
				}
			}
		}()

		// Wait for the error channel OR the output channel to close (signaling command end)
		stepErr = <-errChan // Blocks until an error is sent or the channel is closed

		<-outputDone

		if stepErr != nil {
			return fmt.Errorf("step '%s' failed: %w", step.Name, stepErr)
		}
		successColor.Printf("--- Step '%s' completed successfully for %s (%s) ---\n", step.Name, project.Name, identifierColor.Sprint(project.ServerName))
	}
	return nil
}
