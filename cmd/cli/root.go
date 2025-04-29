// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	statusColor        = color.New(color.FgCyan)
	errorColor         = color.New(color.FgRed)
	stepColor          = color.New(color.FgYellow)
	successColor       = color.New(color.FgGreen)
	statusUpColor      = color.New(color.FgGreen)
	statusDownColor    = color.New(color.FgRed)
	statusPartialColor = color.New(color.FgYellow)
	statusErrorColor   = color.New(color.FgMagenta)
)

// getComposeRootOrExit gets the compose root directory or prints an error and exits.
func getComposeRootOrExit() string {
	rootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		errorColor.Fprintf(os.Stderr, "Error finding compose directory: %v\n", err)
		os.Exit(1)
	}
	return rootDir
}

// projectCompletionFunc provides dynamic completion for project names.
func projectCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	rootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		fmt.Fprintf(os.Stderr, "completion error getting root dir: %v\n", err)
		return nil, cobra.ShellCompDirectiveError
	}
	projects, err := discovery.FindProjects(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "completion error finding projects in %s: %v\n", rootDir, err)
		return nil, cobra.ShellCompDirectiveError
	}

	var projectNames []string
	for _, p := range projects {
		// Only suggest projects that start with the currently typed string
		if strings.HasPrefix(p.Name, toComplete) {
			projectNames = append(projectNames, p.Name)
		}
	}

	return projectNames, cobra.ShellCompDirectiveNoFileComp
}

var rootCmd = &cobra.Command{
	Use:   "bm",
	Short: "Bucket Manager CLI",
	Long:  `A command-line interface to manage multiple Podman Compose projects found in ~/bucket or ~/compose-bucket.`,
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
	Short: "List discovered Podman Compose projects",
	Run: func(cmd *cobra.Command, args []string) {
		rootDir := getComposeRootOrExit()
		projects, err := discovery.FindProjects(rootDir)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error finding projects in %s: %v\n", rootDir, err)
			os.Exit(1)
		}
		if len(projects) == 0 {
			fmt.Printf("No Podman Compose projects found in %s.\n", rootDir)
			return
		}
		statusColor.Println("Discovered projects:")
		for _, p := range projects {
			fmt.Printf("- %s (%s)\n", p.Name, p.Path)
		}
	},
}

var upCmd = &cobra.Command{
	Use:               "up [project-name]",
	Short:             "Run 'pull' and 'up -d' for a project",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		rootDir := getComposeRootOrExit()
		projectName := args[0]
		projectPath := filepath.Join(rootDir, projectName)
		// Verify project exists before running sequence
		if _, err := os.Stat(filepath.Join(projectPath, "compose.yaml")); os.IsNotExist(err) {
			if _, errYml := os.Stat(filepath.Join(projectPath, "compose.yml")); os.IsNotExist(errYml) {
				errorColor.Fprintf(os.Stderr, "Error: Project '%s' not found or missing compose file in %s.\n", projectName, projectPath)
				os.Exit(1)
			}
		}
		statusColor.Printf("Executing 'up' action for project: %s (in %s)\n", projectName, rootDir)
		sequence := runner.UpSequence(projectPath)
		err := runSequence(projectName, rootDir, sequence)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "\n'up' action failed for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		successColor.Printf("'up' action completed successfully for %s.\n", projectName)
	},
}

var downCmd = &cobra.Command{
	Use:               "down [project-name]",
	Short:             "Run 'podman compose down' for a project",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		rootDir := getComposeRootOrExit()
		projectName := args[0]
		projectPath := filepath.Join(rootDir, projectName)
		// Verify project exists
		if _, err := os.Stat(filepath.Join(projectPath, "compose.yaml")); os.IsNotExist(err) {
			if _, errYml := os.Stat(filepath.Join(projectPath, "compose.yml")); os.IsNotExist(errYml) {
				errorColor.Fprintf(os.Stderr, "Error: Project '%s' not found or missing compose file in %s.\n", projectName, projectPath)
				os.Exit(1)
			}
		}
		statusColor.Printf("Executing 'down' action for project: %s (in %s)\n", projectName, rootDir)
		sequence := runner.DownSequence(projectPath)
		err := runSequence(projectName, rootDir, sequence)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "\n'down' action failed for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		successColor.Printf("'down' action completed successfully for %s.\n", projectName)
	},
}

var refreshCmd = &cobra.Command{
	Use:               "refresh [project-name]",
	Short:             "Run 'pull', 'down', 'up', and 'prune' for a project",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		rootDir := getComposeRootOrExit()
		projectName := args[0]
		projectPath := filepath.Join(rootDir, projectName)
		// Verify project exists
		if _, err := os.Stat(filepath.Join(projectPath, "compose.yaml")); os.IsNotExist(err) {
			if _, errYml := os.Stat(filepath.Join(projectPath, "compose.yml")); os.IsNotExist(errYml) {
				errorColor.Fprintf(os.Stderr, "Error: Project '%s' not found or missing compose file in %s.\n", projectName, projectPath)
				os.Exit(1)
			}
		}
		statusColor.Printf("Executing 'refresh' action for project: %s (in %s)\n", projectName, rootDir)
		sequence := runner.RefreshSequence(projectPath)
		err := runSequence(projectName, rootDir, sequence)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "\n'refresh' action failed for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		successColor.Printf("'refresh' action completed successfully for %s.\n", projectName)
	},
}

// --- statusCmd ---
var statusCmd = &cobra.Command{
	Use:   "status [project-name]",
	Short: "Show the status of containers for one or all projects",
	Long: `Shows the status of Podman Compose containers.
If a project name is provided, shows status for that specific project.
Otherwise, shows status for all discovered projects.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: projectCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		rootDir := getComposeRootOrExit()
		var projectsToScan []discovery.Project

		allProjects, findErr := discovery.FindProjects(rootDir)
		if findErr != nil {
			errorColor.Fprintf(os.Stderr, "Error finding projects in %s: %v\n", rootDir, findErr)
			os.Exit(1)
		}

		if len(args) == 1 {
			projectName := args[0]
			found := false
			for _, p := range allProjects {
				if p.Name == projectName {
					projectsToScan = append(projectsToScan, p)
					found = true
					break
				}
			}
			if !found {
				errorColor.Fprintf(os.Stderr, "Error: Project '%s' not found in %s.\n", projectName, rootDir)
				os.Exit(1)
			}
		} else {
			if len(allProjects) == 0 {
				fmt.Printf("No Podman Compose projects found in %s.\n", rootDir)
				return
			}
			projectsToScan = allProjects // Scan all found projects
		}

		statusColor.Printf("Checking project status in %s...\n", rootDir)
		for _, p := range projectsToScan {
			statusInfo := runner.GetProjectStatus(p.Path)

			fmt.Printf("\nProject: %s ", p.Name)
			switch statusInfo.OverallStatus {
			case runner.StatusUp:
				statusUpColor.Printf("[%s]\n", statusInfo.OverallStatus)
			case runner.StatusDown:
				statusDownColor.Printf("[%s]\n", statusInfo.OverallStatus)
			case runner.StatusPartial:
				statusPartialColor.Printf("[%s]\n", statusInfo.OverallStatus)
			case runner.StatusError:
				statusErrorColor.Printf("[%s]\n", statusInfo.OverallStatus)
				errorColor.Fprintf(os.Stderr, "  Error checking status: %v\n", statusInfo.Error)
			default:
				fmt.Printf("[%s]\n", statusInfo.OverallStatus)
			}

			if statusInfo.OverallStatus != runner.StatusDown && len(statusInfo.Containers) > 0 {
				fmt.Println("  Containers:")
				for _, c := range statusInfo.Containers {
					isUp := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
					if isUp {
						statusUpColor.Printf("  - %s (%s): %s\n", c.Service, c.Name, c.Status)
					} else {
						statusDownColor.Printf("  - %s (%s): %s\n", c.Service, c.Name, c.Status)
					}
				}
			}
		}
	},
}

// runSequence executes a series of command steps, streaming output.
// It now accepts rootDir to correctly construct project paths.
func runSequence(projectName, rootDir string, sequence []runner.CommandStep) error {
	for _, step := range sequence {
		stepColor.Printf("\n--- Running Step: %s ---\n", step.Name)

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

		stepErr = <-errChan
		<-outputDone // Wait for output processing to finish

		if stepErr != nil {
			// Error is already formatted by StreamCommand or the wait logic
			return fmt.Errorf("step '%s' failed: %w", step.Name, stepErr)
		}
		successColor.Printf("--- Step '%s' completed successfully ---\n", step.Name)
	}
	return nil
}
