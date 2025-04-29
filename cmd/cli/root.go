package main // Changed from cli to main

import (
	"fmt"
	"os"
	"path/filepath"
	"podman-compose-manager/internal/discovery"
	"podman-compose-manager/internal/runner"

	"github.com/spf13/cobra"
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
		projects, err := discovery.FindProjects("/home/ubuntu/bucket") // Hardcoded as requested
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding projects: %v\n", err)
			os.Exit(1)
		}
		if len(projects) == 0 {
			fmt.Println("No Podman Compose projects found in /home/ubuntu/bucket.")
			return
		}
		fmt.Println("Discovered projects:")
		for _, p := range projects {
			fmt.Printf("- %s (%s)\n", p.Name, p.Path)
		}
	},
}

var upCmd = &cobra.Command{
	Use:   "up [project-name]",
	Short: "Run 'podman compose pull' and 'podman compose up -d' for a project",
	Args:  cobra.ExactArgs(1), // Requires exactly one argument
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		projectPath := filepath.Join("/home/ubuntu/bucket", projectName) // Assume project name is the directory name
		fmt.Printf("Executing 'up' action for project: %s\n", projectName)
		output, err := runner.Up(projectPath)
		fmt.Println(output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing 'up' for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		fmt.Printf("'up' action completed for %s.\n", projectName)
	},
}

var downCmd = &cobra.Command{
	Use:   "down [project-name]",
	Short: "Run 'podman compose down' for a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		projectPath := filepath.Join("/home/ubuntu/bucket", projectName)
		fmt.Printf("Executing 'down' action for project: %s\n", projectName)
		output, err := runner.Down(projectPath)
		fmt.Println(output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing 'down' for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		fmt.Printf("'down' action completed for %s.\n", projectName)
	},
}

var refreshCmd = &cobra.Command{
	Use:   "refresh [project-name]",
	Short: "Run 'pull', 'down', 'up', and 'prune' for a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		projectPath := filepath.Join("/home/ubuntu/bucket", projectName)
		fmt.Printf("Executing 'refresh' action for project: %s\n", projectName)
		output, err := runner.Refresh(projectPath)
		fmt.Println(output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing 'refresh' for %s: %v\n", projectName, err)
			os.Exit(1)
		}
		fmt.Printf("'refresh' action completed for %s.\n", projectName)
	},
}
