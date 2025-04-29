package main // Changed from cli to main

import (
	"fmt"
	"os"
	"path/filepath"
	"podman-compose-manager/internal/discovery"
	"podman-compose-manager/internal/runner"

	"github.com/fatih/color" // Added for colored output
	"github.com/spf13/cobra"
)

// Define some colors for status messages
var (
	statusColor  = color.New(color.FgCyan)
	errorColor   = color.New(color.FgRed)
	stepColor    = color.New(color.FgYellow)
	successColor = color.New(color.FgGreen)
)

var rootCmd = &cobra.Command{
	Use:   "pcm-cli",
	Short: "Podman Compose Manager CLI",
	Long:  `A command-line interface to manage multiple Podman Compose projects found in /home/ubuntu/bucket.`,
	// Run: func(cmd *cobra.Command, args []string) { }, // Root command doesn't do anything by itself
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Add subcommands here
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(refreshCmd)
}

// --- Subcommands ---

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered Podman Compose projects",
	Run: func(cmd *cobra.Command, args []string) {
		// NOTE: Using "/home/ubuntu/bucket" as the canonical path for discovery logic,
		// even though the user mentioned ~/bucket. The runner will use the correct project paths.
		projects, err := discovery.FindProjects("/home/ubuntu/bucket")
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error finding projects: %v\n", err) // Use errorColor
			os.Exit(1)
		}
		if len(projects) == 0 {
			fmt.Println("No Podman Compose projects found in /home/ubuntu/bucket.")
			return
		}
		statusColor.Println("Discovered projects:") // Use statusColor
		for _, p := range projects {
			fmt.Printf("- %s (%s)\n", p.Name, p.Path) // Keep this plain for now
		}
	},
}

var upCmd = &cobra.Command{
	Use:   "up [project-name]",
	Short: "Run 'podman compose pull' and 'podman compose up -d' for a project",
	Args: cobra.ExactArgs(1), // Requires exactly one argument
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		// Use /home/ubuntu/bucket for consistency with discovery, runner handles actual path logic
		projectPath := filepath.Join("/home/ubuntu/bucket", projectName)
		statusColor.Printf("Executing 'up' action for project: %s\n", projectName)
		sequence := runner.UpSequence(projectPath) // Get the steps
		err := runSequence(projectName, sequence)  // Run the sequence
		if err != nil {
			errorColor.Fprintf(os.Stderr, "\n'up' action failed for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		successColor.Printf("'up' action completed successfully for %s.\n", projectName)
	},
}

var downCmd = &cobra.Command{
	Use:   "down [project-name]",
	Short: "Run 'podman compose down' for a project",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		projectPath := filepath.Join("/home/ubuntu/bucket", projectName)
		statusColor.Printf("Executing 'down' action for project: %s\n", projectName)
		sequence := runner.DownSequence(projectPath) // Get the steps
		err := runSequence(projectName, sequence)   // Run the sequence
		if err != nil {
			errorColor.Fprintf(os.Stderr, "\n'down' action failed for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		successColor.Printf("'down' action completed successfully for %s.\n", projectName)
	},
}

var refreshCmd = &cobra.Command{
	Use:   "refresh [project-name]",
	Short: "Run 'pull', 'down', 'up', and 'prune' for a project",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		projectPath := filepath.Join("/home/ubuntu/bucket", projectName)
		statusColor.Printf("Executing 'refresh' action for project: %s\n", projectName)
		sequence := runner.RefreshSequence(projectPath) // Get the steps
		err := runSequence(projectName, sequence)      // Run the sequence
		if err != nil {
			errorColor.Fprintf(os.Stderr, "\n'refresh' action failed for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		successColor.Printf("'refresh' action completed successfully for %s.\n", projectName)
	},
}

// runSequence executes a series of command steps, streaming output.
func runSequence(projectName string, sequence []runner.CommandStep) error {
	for _, step := range sequence {
		stepColor.Printf("\n--- Running Step: %s ---\n", step.Name)

		// Adjust step directory if it's project-specific but path is /home/ubuntu/bucket based
		if step.Dir != "" { // Only adjust if a specific dir is set (e.g., not for global prune)
			step.Dir = filepath.Join("/home/ubuntu/bucket", projectName)
		}


		outChan, errChan := runner.StreamCommand(step)

		var stepErr error
		outputDone := make(chan struct{})

		// Goroutine to process output lines
		go func() {
			defer close(outputDone)
			for line := range outChan {
				if line.IsError {
					// Print stderr lines in red, but don't treat them as fatal errors here
					// The final error comes from errChan
					errorColor.Fprintln(os.Stderr, line.Line)
				} else {
					fmt.Println(line.Line)
				}
			}
		}()

		// Wait for the command to finish and get the final error status
		stepErr = <-errChan
		// Wait for all output lines to be printed before proceeding
		<-outputDone

		if stepErr != nil {
			// Don't print the error here again, StreamCommand already includes it
			// errorColor.Fprintf(os.Stderr, "Step '%s' failed: %v\n", step.Name, stepErr)
			// Decide whether to stop or continue based on the action?
			// For now, let's stop on any error during up/down/refresh.
			return fmt.Errorf("step '%s' failed: %w", step.Name, stepErr)
		}
		successColor.Printf("--- Step '%s' completed successfully ---\n", step.Name)
	}
	return nil // All steps succeeded
}
