// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package cli implements the command-line interface for the bucket manager.
// It provides subcommands for managing stacks, SSH configurations, and
// executing operations on both local and remote compose stacks.
package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
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

// Package-level variables for CLI operation
var (
	// sshManager handles SSH connections to remote hosts
	sshManager *ssh.Manager

	// Color definitions for consistent CLI output formatting
	statusColor  = color.New(color.FgCyan)   // For status messages
	errorColor   = color.New(color.FgRed)    // For error messages
	stepColor    = color.New(color.FgYellow) // For step indicators
	successColor = color.New(color.FgGreen)  // For success messages

	// Colors for stack status indicators
	statusUpColor      = color.New(color.FgGreen)  // For "up" status
	statusDownColor    = color.New(color.FgRed)    // For "down" status
	statusPartialColor = color.New(color.FgYellow) // For "partial" status
	statusErrorColor   = color.New(color.FgMagenta)
	identifierColor    = color.New(color.FgBlue)
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "bm",
	Short: "Bucket Manager CLI",
	Long: `A command-line interface to manage multiple compose stacks.

Discovers stacks in standard local directories (~/bucket, ~/compose-bucket)
and on remote hosts configured via SSH (~/.config/bucket-manager/config.yaml).
Use 'bm serve' to start the web interface.`,

	// PersistentPreRunE is executed before any subcommand runs
	// It sets up the required environment and connections
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Handle verbose flag for logging
		verbose, _ := cmd.Flags().GetBool("verbose")
		silent, _ := cmd.Flags().GetBool("silent")

		// Re-initialize logger with correct verbosity settings
		logger.InitCLI(verbose, silent)

		// Ensure config directory exists
		if err := config.EnsureConfigDir(); err != nil {
			return fmt.Errorf("failed to ensure config directory: %w", err)
		}

		// Initialize SSH connection manager
		sshManager = ssh.NewManager()

		// Share SSH manager with other packages that need it
		discovery.InitSSHManager(sshManager)
		runner.InitSSHManager(sshManager)
		return nil
	},

	// PersistentPostRunE is executed after any subcommand completes
	// It handles cleanup of resources
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Clean up SSH connections when command completes
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

// init registers all CLI subcommands with the root command
func init() {
	// Add persistent flags that apply to all commands
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging to stderr")
	rootCmd.PersistentFlags().BoolP("silent", "s", false, "Suppress all output to stderr (file logging only)")

	// Stack discovery command
	rootCmd.AddCommand(listCmd)

	// Stack operation commands
	rootCmd.AddCommand(upCmd)      // Start stacks
	rootCmd.AddCommand(downCmd)    // Stop stacks
	rootCmd.AddCommand(refreshCmd) // Restart stacks
	rootCmd.AddCommand(statusCmd)  // Get stack status
	rootCmd.AddCommand(pullCmd)    // Pull latest container images

	// Host operation commands
	rootCmd.AddCommand(pruneCmd) // Clean up unused containers/images
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered compose stacks (local and remote)",
	Run: func(cmd *cobra.Command, args []string) {
		statusColor.Println("Discovering stacks...")
		stackChan, errorChan, _ := discovery.FindStacks()

		var collectedErrors []error
		var stacksFound bool
		var wg sync.WaitGroup
		wg.Add(1)

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

		for stack := range stackChan {
			s.Stop()
			stacksFound = true
			fmt.Printf("- %s (%s)\n", stack.Name, identifierColor.Sprint(stack.ServerName))
			s.Restart()
		}
		s.Stop()

		wg.Wait()

		if !stacksFound && len(collectedErrors) == 0 {
			fmt.Println("\nNo compose stacks found locally or on configured remote hosts.")
		} else if !stacksFound && len(collectedErrors) > 0 {
			fmt.Println("\nNo stacks discovered successfully.")
		}

		if len(collectedErrors) > 0 {
			os.Exit(1)
		}
	},
}

var upCmd = &cobra.Command{
	Use:               "up <stack-identifier> [stack-identifier...]",
	Short:             "Start one or more stacks",
	Example:           "  bm up my-local-app\n  bm up server1:remote-app\n  bm up app1 app2 server1:app3",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("up", args)
	},
}

var downCmd = &cobra.Command{
	Use:               "down <stack-identifier> [stack-identifier...]",
	Short:             "Stop one or more stacks",
	Example:           "  bm down my-local-app\n  bm down server1:remote-app\n  bm down app1 app2 server1:app3",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("down", args)
	},
}

var refreshCmd = &cobra.Command{
	Use:               "refresh <stack-identifier> [stack-identifier...]",
	Aliases:           []string{"re"},
	Short:             "Fully refresh one or more stacks (alias: re)",
	Long:              `Pulls latest images, stops the stack, and starts it again. Also cleans up unused resources on local stacks.`,
	Example:           "  bm refresh my-local-app\n  bm re server1:remote-app\n  bm refresh app1 app2 server1:app3",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("refresh", args)
	},
}

var pullCmd = &cobra.Command{
	Use:               "pull <stack-identifier> [stack-identifier...]",
	Short:             "Pull latest images for one or more stacks",
	Example:           "  bm pull my-local-app\n  bm pull server1:remote-app\n  bm pull app1 app2 server1:app3",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("pull", args)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status [stack-identifier]",
	Short: "Show the status of containers for one or all stacks",
	Long: `Shows the status of compose containers for local and remote stacks.
If a stack identifier (e.g., my-app or server1:remote-app) is provided, shows status for that specific stack.
If a remote identifier ending with ':' (e.g., server1:) is provided, shows status for all stacks on that remote.
Otherwise, shows status for all discovered stacks.`,
	Example:           "  bm status\n  bm status my-local-app\n  bm status server1:remote-app\n  bm status server1:",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		var collectedErrors []error
		scanAll := len(args) == 0

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Color("cyan")

		discoveryIdentifier := ""
		if !scanAll {
			discoveryIdentifier = args[0]
			statusColor.Printf("Checking status for %s...\n", identifierColor.Sprint(discoveryIdentifier))
			s.Suffix = fmt.Sprintf(" Discovering %s...", identifierColor.Sprint(discoveryIdentifier))
		} else {
			statusColor.Println("Discovering all stacks and checking status...")
			s.Suffix = " Discovering stacks..."
		}
		s.Start()

		stacksToProcess, collectedErrors := discoverTargetStacks(discoveryIdentifier, s)
		s.Stop()

		if len(collectedErrors) > 0 {
			logger.Error("\nErrors during stack discovery:")
			for _, err := range collectedErrors {
				logger.Errorf("- %v", err)
			}
			if len(stacksToProcess) == 0 {
				os.Exit(1)
			}
			errorColor.Fprintln(os.Stderr, "Continuing with successfully discovered stacks...")
		}

		if len(stacksToProcess) == 0 {
			if scanAll {
				fmt.Println("\nNo compose stacks found locally or on configured remote hosts.")
			}
			if len(collectedErrors) == 0 {
				os.Exit(1)
			}
		}

		if len(stacksToProcess) > 0 {
			statusChan := make(chan runner.StackRuntimeInfo, len(stacksToProcess))
			var statusWg sync.WaitGroup
			statusWg.Add(len(stacksToProcess))

			s.Suffix = " Checking stack status..."
			s.Start()

			for _, stack := range stacksToProcess {
				go func(s discovery.Stack) {
					defer statusWg.Done()
					statusInfo := runner.GetStackStatus(s)
					statusChan <- statusInfo
				}(stack)
			}

			go func() {
				statusWg.Wait()
				close(statusChan)
			}()

			for statusInfo := range statusChan {
				s.Stop()

				fmt.Printf("\nStack: %s (%s) ", statusInfo.Stack.Name, identifierColor.Sprint(statusInfo.Stack.ServerName))
				switch statusInfo.OverallStatus {
				case runner.StatusUp:
					statusUpColor.Printf("[%s]\n", statusInfo.OverallStatus)
				case runner.StatusDown:
					statusDownColor.Printf("[%s]\n", statusInfo.OverallStatus)
				case runner.StatusPartial:
					statusPartialColor.Printf("[%s]\n", statusInfo.OverallStatus)
				case runner.StatusError:
					statusErrorColor.Printf("[%s]\n", statusInfo.OverallStatus)
					err := fmt.Errorf("status check for %s failed: %w", statusInfo.Stack.Identifier(), statusInfo.Error)
					collectedErrors = append(collectedErrors, err)
					if statusInfo.Error != nil {
						logger.Errorf("  Error checking status: %v", statusInfo.Error)
					} else {
						logger.Error("  Unknown error checking status.")
					}
				default:
					fmt.Printf("[%s]\n", statusInfo.OverallStatus)
				}

				if statusInfo.OverallStatus != runner.StatusDown && len(statusInfo.Containers) > 0 {
					fmt.Println("  Containers:")
					fmt.Printf("    %-25s %-35s %s\n", "SERVICE", "CONTAINER NAME", "STATUS")
					fmt.Printf("    %-25s %-35s %s\n", strings.Repeat("-", 25), strings.Repeat("-", 35), strings.Repeat("-", 6))
					for _, c := range statusInfo.Containers {
						isUp := strings.Contains(strings.ToLower(c.Status), "running") ||
							strings.Contains(strings.ToLower(c.Status), "healthy") ||
							strings.HasPrefix(c.Status, "Up")

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

		if len(collectedErrors) > 0 {
			os.Exit(1)
		}
	},
}

var pruneCmd = &cobra.Command{
	Use:   "prune [host-identifier...]",
	Short: "Clean up unused resources on specified hosts",
	Long: `Removes unused containers, networks, images, and volumes on the specified hosts.
Targets can be 'local', specific remote host names, or left empty to target ALL configured hosts (local + remotes).`,
	Example: `  bm prune          # Clean up local system AND all configured remote hosts
	 bm prune local       # Clean up only the local system
	 bm prune server1     # Clean up only the remote host 'server1'
	 bm prune local server1 server2 # Clean up local, server1, and server2`,
	ValidArgsFunction: hostCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		targetsToPrune := []runner.HostTarget{}
		targetMap := make(map[string]bool)

		if len(args) == 0 {
			statusColor.Println("Targeting local host and all configured remote hosts for prune...")
			targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: false, ServerName: "local"})
			targetMap["local"] = true
			for _, host := range cfg.SSHHosts {
				if !host.Disabled {
					targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: true, HostConfig: &host, ServerName: host.Name})
					targetMap[host.Name] = true
				}
			}
		} else {
			statusColor.Printf("Targeting specified hosts for prune: %s...\n", strings.Join(args, ", "))
			for _, targetName := range args {
				if targetMap[targetName] {
					continue
				}

				if targetName == "local" {
					targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: false, ServerName: "local"})
					targetMap["local"] = true
				} else {
					found := false
					for i := range cfg.SSHHosts {
						host := cfg.SSHHosts[i]
						if host.Name == targetName {
							if host.Disabled {
								errorColor.Fprintf(os.Stderr, "Warning: Skipping disabled host '%s'\n", targetName)
							} else {
								targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: true, HostConfig: &host, ServerName: host.Name})
								targetMap[host.Name] = true
							}
							found = true
							break
						}
					}
					if !found {
						errorColor.Fprintf(os.Stderr, "Error: Host identifier '%s' not found in configuration.\n", targetName)
						os.Exit(1)
					}
				}
			}
		}

		if len(targetsToPrune) == 0 {
			errorColor.Fprintln(os.Stderr, "No valid targets specified or found for prune.")
			os.Exit(1)
		}

		err = runHostAction("prune", targetsToPrune)
		if err != nil {
			logger.Errorf("\nPrune action failed for one or more hosts: %v", err)
			os.Exit(1)
		}

		successColor.Println("\nPrune action completed for all targeted hosts.")
	},
}
