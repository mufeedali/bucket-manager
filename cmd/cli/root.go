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
	sshManager         *ssh.Manager
	statusColor        = color.New(color.FgCyan)
	errorColor         = color.New(color.FgRed)
	stepColor          = color.New(color.FgYellow)
	successColor       = color.New(color.FgGreen)
	statusUpColor      = color.New(color.FgGreen)
	statusDownColor    = color.New(color.FgRed)
	statusPartialColor = color.New(color.FgYellow)
	statusErrorColor   = color.New(color.FgMagenta)
	identifierColor    = color.New(color.FgBlue)
)

// Identifier can be "projectName" (implies local preference) or "serverName:projectName".
// Returns an error if not found or if "projectName" is ambiguous.
func findProjectByIdentifier(projects []discovery.Project, identifier string) (discovery.Project, error) {
	identifier = strings.TrimSpace(identifier)
	targetName := identifier
	targetServer := "" // Empty means user didn't specify, implies local preference unless ambiguous

	if parts := strings.SplitN(identifier, ":", 2); len(parts) == 2 {
		targetServer = strings.TrimSpace(parts[0])
		targetName = strings.TrimSpace(parts[1])
		if targetName == "" || targetServer == "" {
			// Allow "remote:" format for status command later, but not here for finding a *specific* project
			return discovery.Project{}, fmt.Errorf("invalid identifier format: '%s'. Use 'project' or 'remote:project'", identifier)
		}
	}

	var potentialMatches []discovery.Project
	var exactMatch *discovery.Project

	// First pass: Look for exact matches or collect potential matches if server wasn't specified
	for i := range projects {
		p := projects[i]
		if p.Name == targetName {
			if targetServer != "" { // User specified a server (e.g., server:project or local:project)
				if p.ServerName == targetServer {
					exactMatch = &p
					break
				}
			} else { // User did *not* specify a server (e.g., just project)
				potentialMatches = append(potentialMatches, p)
			}
		}
	}

	if targetServer != "" {
		if exactMatch != nil {
			return *exactMatch, nil
		}
		// If exact match wasn't found, but server was specified, it's simply not found
		return discovery.Project{}, fmt.Errorf("project '%s:%s' not found", targetServer, targetName)
	}

	if len(potentialMatches) == 0 {
		return discovery.Project{}, fmt.Errorf("project '%s' not found", targetName)
	}

	if len(potentialMatches) == 1 {
		return potentialMatches[0], nil
	}

	// Ambiguous case: Multiple projects match the name, and user didn't specify server
	// Check if one of the matches is local - prefer that one implicitly
	var localMatch *discovery.Project
	for i := range potentialMatches {
		if !potentialMatches[i].IsRemote {
			if localMatch != nil {
				// Ambiguous if multiple local matches exist
				break
			}
			localMatch = &potentialMatches[i]
		}
	}

	if localMatch != nil && len(potentialMatches) > 1 {
		// Prefer a single local match if found among multiple name matches.
		localCount := 0
		for _, pm := range potentialMatches {
			if !pm.IsRemote {
				localCount++
			}
		}
		if localCount == 1 {
			return *localMatch, nil
		}
	}

	// Ambiguous if no single local match or multiple matches remain
	options := []string{}
	for _, pm := range potentialMatches {
		options = append(options, pm.Identifier())
	}
	return discovery.Project{}, fmt.Errorf("project name '%s' is ambiguous, please specify one of: %s", targetName, strings.Join(options, ", "))
}

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
		fmt.Fprintf(os.Stderr, "completion error finding projects: %v\n", errors[0])
	}

	var projectIdentifiers []string
	for _, p := range projects {
		identifier := p.Identifier()
		if strings.HasPrefix(identifier, toComplete) {
			projectIdentifiers = append(projectIdentifiers, identifier)
		} else if !p.IsRemote && strings.HasPrefix(p.Name, toComplete) && !strings.Contains(toComplete, ":") {
			// Also suggest implicit local name if user hasn't typed ':'
			projectIdentifiers = append(projectIdentifiers, p.Name)
		}
	}

	return projectIdentifiers, cobra.ShellCompDirectiveNoFileComp
}

var rootCmd = &cobra.Command{
	Use:   "bm",
	Short: "Bucket Manager CLI",
	Long: `A command-line interface to manage multiple Podman Compose stacks.

Discovers stacks in standard local directories (~/bucket, ~/compose-bucket)
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

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered Podman Compose stacks (local and remote)",
	Run: func(cmd *cobra.Command, args []string) {
		statusColor.Println("Discovering stacks...")
		projectChan, errorChan, _ := discovery.FindProjects()

		var collectedErrors []error
		var stacksFound bool
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

		fmt.Println("\nDiscovered stacks:")

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Color("cyan")
		s.Suffix = " Loading remote stacks..."
		s.Start()

		for stack := range projectChan {
			s.Stop()
			stacksFound = true
			fmt.Printf("- %s (%s)\n", stack.Name, identifierColor.Sprint(stack.ServerName))
			s.Restart()
		}
		s.Stop()

		wg.Wait()

		if !stacksFound && len(collectedErrors) == 0 {
			fmt.Println("\nNo Podman Compose stacks found locally or on configured remote hosts.")
		} else if !stacksFound && len(collectedErrors) > 0 {
			fmt.Println("\nNo stacks discovered successfully.")
		}

		if len(collectedErrors) > 0 {
			os.Exit(1)
		}
	},
}

// discoverTargetProjects finds projects based on an identifier, handling local/remote discovery.
// identifier: The project identifier (e.g., "my-app", "server1:my-app", "local:my-app").
//
//	Can also be "server1:" to discover all projects on server1 (used by status).
//	If empty, discovers all projects.
//
// s: Optional spinner for feedback during remote discovery.
func discoverTargetProjects(identifier string, s *spinner.Spinner) ([]discovery.Project, []error) {
	var projectsToCheck []discovery.Project
	var collectedErrors []error
	targetProjectName := ""
	targetServerName := "" // "local", specific remote name, or "" for ambiguous/all

	// 1. Parse Identifier (if provided)
	if identifier != "" {
		if strings.HasSuffix(identifier, ":") { // Handle "remote:" case for status
			targetServerName = strings.TrimSuffix(identifier, ":")
			targetProjectName = "" // Signal to find all projects on this server
			if targetServerName == "" {
				return nil, []error{fmt.Errorf("invalid identifier format: '%s'. Cannot be just ':'", identifier)}
			}
		} else if parts := strings.SplitN(identifier, ":", 2); len(parts) == 2 { // Handle "remote:project"
			targetServerName = strings.TrimSpace(parts[0])
			targetProjectName = strings.TrimSpace(parts[1])
			if targetProjectName == "" || targetServerName == "" {
				return nil, []error{fmt.Errorf("invalid identifier format: '%s'. Use 'project', 'remote:project', or 'remote:'", identifier)}
			}
		} else { // Handle "project"
			targetProjectName = identifier
			// targetServerName remains "" -> implies local preference or ambiguous
		}
	}
	// If identifier is "", scanAll case: targetServerName remains ""

	// 2. Load Config (conditionally needed for remote)
	cfg, configErr := config.LoadConfig()
	// We only fail *immediately* if config is needed for a *specific* remote host.
	if configErr != nil && targetServerName != "local" && targetServerName != "" {
		return nil, []error{fmt.Errorf("error loading config needed for remote discovery: %w", configErr)}
	}
	// configErr might still be non-nil, but we handle it later if remote scan becomes necessary.

	// 3. Discovery Logic
	scanAll := identifier == ""

	if scanAll {
		// --- Discover All Projects ---
		projectChan, errorChan, _ := discovery.FindProjects()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for p := range projectChan {
				projectsToCheck = append(projectsToCheck, p)
			}
		}()
		go func() {
			defer wg.Done()
			for e := range errorChan {
				collectedErrors = append(collectedErrors, e)
			}
		}()
		wg.Wait()
	} else {
		// --- Targeted Discovery ---

		// a. Discover Local (if target is local or ambiguous)
		if targetServerName == "local" || targetServerName == "" {
			localRootDir, err := discovery.GetComposeRootDirectory()
			if err == nil {
				localProjects, err := discovery.FindLocalProjects(localRootDir)
				if err != nil {
					collectedErrors = append(collectedErrors, fmt.Errorf("local discovery failed: %w", err))
				} else {
					projectsToCheck = append(projectsToCheck, localProjects...)
				}
			} else if !strings.Contains(err.Error(), "could not find") {
				collectedErrors = append(collectedErrors, fmt.Errorf("local root check failed: %w", err))
			}
		}

		// b. Discover Specific Remote (if target is specific remote)
		if targetServerName != "local" && targetServerName != "" {
			// Check configErr now, as we definitely need the config
			if configErr != nil {
				return nil, []error{fmt.Errorf("error loading config needed for remote discovery: %w", configErr)}
			}
			var targetHost *config.SSHHost
			for i := range cfg.SSHHosts {
				if cfg.SSHHosts[i].Name == targetServerName {
					targetHost = &cfg.SSHHosts[i]
					break
				}
			}
			if targetHost == nil {
				collectedErrors = append(collectedErrors, fmt.Errorf("remote host '%s' not found in configuration", targetServerName))
			} else {
				remoteProjects, err := discovery.FindRemoteProjects(targetHost)
				if err != nil {
					collectedErrors = append(collectedErrors, fmt.Errorf("remote discovery failed for %s: %w", targetHost.Name, err))
				} else {
					// If targetProjectName is empty ("remote:" case), add all projects from this remote.
					// Otherwise, only add the specific project.
					if targetProjectName == "" {
						projectsToCheck = append(projectsToCheck, remoteProjects...)
					} else {
						for _, p := range remoteProjects {
							if p.Name == targetProjectName {
								projectsToCheck = append(projectsToCheck, p)
								break
							}
						}
					}
				}
			}
		}

		// c. Discover All Remotes (if ambiguous and not found locally)
		if targetServerName == "" {
			foundLocally := false
			for _, p := range projectsToCheck { // Check projects found locally so far
				if p.Name == targetProjectName {
					foundLocally = true
					break
				}
			}

			if !foundLocally {
				// Check configErr now before attempting remote discovery
				if configErr != nil {
					collectedErrors = append(collectedErrors, fmt.Errorf("project '%s' not found locally and remote discovery skipped due to config error: %w", targetProjectName, configErr))
				} else if len(cfg.SSHHosts) > 0 { // Only discover remotes if config is ok and hosts exist
					if s != nil {
						originalSuffix := s.Suffix
						s.Suffix = fmt.Sprintf(" Discovering %s on remotes...", identifierColor.Sprint(targetProjectName))
						defer func() { s.Suffix = originalSuffix }() // Restore suffix after function returns
					}

					var remoteWg sync.WaitGroup
					remoteProjectChan := make(chan discovery.Project, 10)
					remoteErrorChan := make(chan error, 5)
					remoteWg.Add(len(cfg.SSHHosts))

					for i := range cfg.SSHHosts {
						hostConfig := cfg.SSHHosts[i]
						go func(hc config.SSHHost) {
							defer remoteWg.Done()
							remoteProjs, err := discovery.FindRemoteProjects(&hc)
							if err != nil {
								remoteErrorChan <- fmt.Errorf("remote discovery failed for %s: %w", hc.Name, err)
							} else {
								// Only add remote projects if they match the target name (if specified)
								// If targetProjectName is empty ("remote:" case), this loop won't run, which is correct.
								for _, p := range remoteProjs {
									if p.Name == targetProjectName {
										remoteProjectChan <- p
										break
									}
								}
							}
						}(hostConfig)
					}

					go func() {
						remoteWg.Wait()
						close(remoteProjectChan)
						close(remoteErrorChan)
					}()

					for p := range remoteProjectChan {
						projectsToCheck = append(projectsToCheck, p)
					}
					for e := range remoteErrorChan {
						collectedErrors = append(collectedErrors, e)
					}
				}
			}
		}
	}

	return projectsToCheck, collectedErrors
}

func runProjectAction(action string, args []string) {
	if len(args) != 1 {
		errorColor.Fprintf(os.Stderr, "Error: requires exactly one project identifier argument.\n")
		os.Exit(1)
	}
	projectIdentifier := args[0]

	statusColor.Printf("Locating project '%s'...\n", projectIdentifier)

	projectsToCheck, collectedErrors := discoverTargetProjects(projectIdentifier, nil)

	// Handle Discovery Errors
	if len(collectedErrors) > 0 {
		errorColor.Fprintln(os.Stderr, "\nErrors during stack discovery:")
		for _, err := range collectedErrors {
			errorColor.Fprintf(os.Stderr, "- %v\n", err)
		}
		os.Exit(1)
	}
	if len(projectsToCheck) == 0 {
		// discoverTargetProjects doesn't inherently know the context (local only, specific remote, or all)
		// to provide a super specific "not found" message here.
		// findProjectByIdentifier will give a more specific error if needed.
		errorColor.Fprintf(os.Stderr, "\nError: No stacks found matching identifier '%s'.\n", projectIdentifier)
		os.Exit(1)
	}

	// Find the specific target project using the identifier logic
	targetProject, err := findProjectByIdentifier(projectsToCheck, projectIdentifier)
	if err != nil {
		errorColor.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	statusColor.Printf("Executing '%s' action for stack: %s (%s)\n", action, targetProject.Name, identifierColor.Sprint(targetProject.ServerName))

	// Determine sequence based on action
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
	Use:               "up <stack-identifier>",
	Short:             "Run 'pull' and 'up -d' for a stack",
	Example:           "  bm up my-local-app\n  bm up server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runProjectAction("up", args)
	},
}

var downCmd = &cobra.Command{
	Use:               "down <stack-identifier>",
	Short:             "Run 'podman compose down' for a stack",
	Example:           "  bm down my-local-app\n  bm down server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runProjectAction("down", args)
	},
}

var refreshCmd = &cobra.Command{
	Use:               "refresh <stack-identifier>",
	Aliases:           []string{"re"},
	Short:             "Run 'pull', 'down', 'up', and maybe 'prune' for a stack (alias: re)",
	Long:              `Runs 'pull', 'down', 'up -d' for the specified stack. Additionally runs 'podman system prune -af' locally if the target stack is local.`,
	Example:           "  bm refresh my-local-app\n  bm re server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runProjectAction("refresh", args)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status [stack-identifier]",
	Short: "Show the status of containers for one or all stacks",
	Long: `Shows the status of Podman Compose containers for local and remote stacks.
If a stack identifier (e.g., my-app or server1:remote-app) is provided, shows status for that specific stack.
If a remote identifier ending with ':' (e.g., server1:) is provided, shows status for all stacks on that remote.
Otherwise, shows status for all discovered stacks.`,
	Example:           "  bm status\n  bm status my-local-app\n  bm status server1:remote-app\n  bm status server1:",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		var collectedErrors []error
		var projectsToCheck []discovery.Project
		var specificProjectIdentifier string
		scanAll := len(args) == 0
		targetServerName := "" // "local", specific remote name, or "" for all/ambiguous
		targetProjectName := ""

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Color("cyan")

		if !scanAll {
			specificProjectIdentifier = args[0]
			// Parse the identifier to determine target (project, remote:project, or remote:)
			if strings.HasSuffix(specificProjectIdentifier, ":") {
				targetServerName = strings.TrimSuffix(specificProjectIdentifier, ":")
				targetProjectName = "" // Signal to get all projects for this server
				if targetServerName == "" {
					errorColor.Fprintf(os.Stderr, "Error: invalid identifier format: '%s'. Cannot be just ':'\n", specificProjectIdentifier)
					os.Exit(1)
				}
				statusColor.Printf("Checking status for all projects on %s...\n", identifierColor.Sprint(targetServerName))
				s.Suffix = fmt.Sprintf(" Discovering projects on %s...", identifierColor.Sprint(targetServerName))
			} else if parts := strings.SplitN(specificProjectIdentifier, ":", 2); len(parts) == 2 {
				targetServerName = strings.TrimSpace(parts[0])
				targetProjectName = strings.TrimSpace(parts[1])
				if targetProjectName == "" || targetServerName == "" {
					errorColor.Fprintf(os.Stderr, "Error: invalid identifier format: '%s'. Use 'project', 'remote:project', or 'remote:'\n", specificProjectIdentifier)
					os.Exit(1)
				}
				statusColor.Printf("Checking status for %s...\n", identifierColor.Sprint(specificProjectIdentifier))
				s.Suffix = fmt.Sprintf(" Discovering %s...", identifierColor.Sprint(specificProjectIdentifier))
			} else {
				// Only project name given, implies local preference but could be ambiguous
				targetProjectName = specificProjectIdentifier
				targetServerName = "" // Signal to check local first, then handle ambiguity
				statusColor.Printf("Checking status for %s...\n", identifierColor.Sprint(specificProjectIdentifier))
				s.Suffix = fmt.Sprintf(" Discovering %s...", identifierColor.Sprint(specificProjectIdentifier))
			}
		} else {
			statusColor.Println("Discovering all stacks and checking status...")
			s.Suffix = " Discovering stacks..."
		}
		s.Start()

		// --- Use Shared Discovery Logic ---
		discoveryIdentifier := "" // Empty means discover all
		if !scanAll {
			discoveryIdentifier = specificProjectIdentifier
		}
		projectsToCheck, collectedErrors = discoverTargetProjects(discoveryIdentifier, s)
		s.Stop()

		// --- Handle Discovery Errors ---
		// Print errors collected during discovery
		if len(collectedErrors) > 0 {
			errorColor.Fprintln(os.Stderr, "\nErrors during stack discovery:")
			for _, err := range collectedErrors {
				errorColor.Fprintf(os.Stderr, "- %v\n", err)
			}
			// Decide whether to exit or continue based on whether *any* projects were found
			if len(projectsToCheck) == 0 {
				os.Exit(1)
			}
			// If some projects were found despite errors, continue to show status for those.
		}

		// --- Filter Projects and Check Status ---
		var finalProjectsToProcess []discovery.Project
		var projectFound bool

		// Filter the results based on the request (all, specific project, or all on a remote)
		if scanAll {
			// Process all discovered projects
			finalProjectsToProcess = projectsToCheck
			projectFound = len(finalProjectsToProcess) > 0
		} else if targetProjectName == "" && targetServerName != "" { // Case: "remote:"
			// Filter projectsToCheck to only include those from the targetServerName
			for _, p := range projectsToCheck {
				if p.ServerName == targetServerName {
					finalProjectsToProcess = append(finalProjectsToProcess, p)
				}
			}
			projectFound = len(finalProjectsToProcess) > 0
			if !projectFound && len(collectedErrors) == 0 {
				// If discovery was successful but no stacks found for this specific remote
				errorColor.Fprintf(os.Stderr, "\nError: No stacks found on remote '%s'.\n", targetServerName)
				os.Exit(1)
			}
		} else { // Case: "project" or "remote:project"
			// Use findProjectByIdentifier to handle ambiguity and select the single target
			targetProject, err := findProjectByIdentifier(projectsToCheck, specificProjectIdentifier)
			if err != nil {
				// If findProjectByIdentifier failed (not found, ambiguous), and we didn't have prior discovery errors
				if len(collectedErrors) == 0 {
					errorColor.Fprintf(os.Stderr, "\nError: %v\n", err)
					os.Exit(1)
				}
				// If we had prior discovery errors, those were more critical.
				// We might have found *some* projects but not the specific one, or ambiguity exists.
				// Proceed to show status for any projects found, but acknowledge the specific target issue.
				errorColor.Fprintf(os.Stderr, "\nWarning: Could not uniquely identify target '%s': %v\n", specificProjectIdentifier, err)
				// Continue with projectsToCheck if any were found, otherwise exit if discovery completely failed earlier.
				if len(projectsToCheck) == 0 {
					os.Exit(1) // Exit if discovery yielded nothing and target wasn't found/ambiguous
				}
				projectFound = false // Mark as not found to prevent status check loop for the failed target
			} else {
				finalProjectsToProcess = []discovery.Project{targetProject}
				projectFound = true
			}
		}

		// --- Perform Status Checks ---
		if len(finalProjectsToProcess) > 0 {
			statusChan := make(chan runner.ProjectRuntimeInfo, len(finalProjectsToProcess))
			var statusWg sync.WaitGroup
			statusWg.Add(len(finalProjectsToProcess))

			s.Suffix = " Checking stack status..."
			s.Start()

			for _, project := range finalProjectsToProcess {
				go func(p discovery.Project) {
					defer statusWg.Done() // Corrected back to statusWg
					statusInfo := runner.GetProjectStatus(p)
					statusChan <- statusInfo
				}(project)
			}

			go func() {
				statusWg.Wait()
				close(statusChan)
			}()

			for statusInfo := range statusChan {
				s.Stop()

				fmt.Printf("\nStack: %s (%s) ", statusInfo.Project.Name, identifierColor.Sprint(statusInfo.Project.ServerName))
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
						collectedErrors = append(collectedErrors, fmt.Errorf("status check for %s failed: %w", statusInfo.Project.Identifier(), statusInfo.Error))
					} else {
						errorColor.Fprintf(os.Stderr, "  Unknown error checking status.\n")
						collectedErrors = append(collectedErrors, fmt.Errorf("unknown status check error for %s", statusInfo.Project.Identifier()))
					}
				default:
					fmt.Printf("[%s]\n", statusInfo.OverallStatus)
				}

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
		}

		if !projectFound && len(collectedErrors) == 0 {
			if scanAll {
				fmt.Println("\nNo Podman Compose stacks found locally or on configured remote hosts.")
			}
		} else if !projectFound && len(collectedErrors) > 0 {
			fmt.Println("\nStack discovery or status check failed.")
		}

		if len(collectedErrors) > 0 {
			if !scanAll && !projectFound {
				// Error already shown
			} else {
				errorColor.Fprintln(os.Stderr, "\nEncountered errors:")
				for _, err := range collectedErrors {
					errorColor.Fprintf(os.Stderr, "- %v\n", err)
				}
			}
			os.Exit(1)
		}
	},
}

func runSequence(project discovery.Project, sequence []runner.CommandStep) error {
	for _, step := range sequence {
		stepColor.Printf("\n--- Running Step: %s for %s (%s) ---\n", step.Name, project.Name, identifierColor.Sprint(project.ServerName))

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

		stepErr = <-errChan // Blocks until an error is sent or the channel is closed

		<-outputDone

		if stepErr != nil {
			return fmt.Errorf("step '%s' failed: %w", step.Name, stepErr)
		}
		successColor.Printf("--- Step '%s' completed successfully for %s (%s) ---\n", step.Name, project.Name, identifierColor.Sprint(project.ServerName))
	}
	return nil
}
