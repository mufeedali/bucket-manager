// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

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
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pruneCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered Podman Compose stacks (local and remote)",
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
			fmt.Println("\nNo Podman Compose stacks found locally or on configured remote hosts.")
		} else if !stacksFound && len(collectedErrors) > 0 {
			fmt.Println("\nNo stacks discovered successfully.")
		}

		if len(collectedErrors) > 0 {
			os.Exit(1)
		}
	},
}

var upCmd = &cobra.Command{
	Use:               "up <stack-identifier>",
	Short:             "Run 'pull' and 'up -d' for a stack",
	Example:           "  bm up my-local-app\n  bm up server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("up", args)
	},
}

var downCmd = &cobra.Command{
	Use:               "down <stack-identifier>",
	Short:             "Run 'podman compose down' for a stack",
	Example:           "  bm down my-local-app\n  bm down server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("down", args)
	},
}

var refreshCmd = &cobra.Command{
	Use:               "refresh <stack-identifier>",
	Aliases:           []string{"re"},
	Short:             "Run 'pull', 'down', 'up', and maybe 'prune' for a stack (alias: re)",
	Long:              `Runs 'pull', 'down', 'up -d' for the specified stack. Additionally runs 'podman system prune -af' locally if the target stack is local.`,
	Example:           "  bm refresh my-local-app\n  bm re server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("refresh", args)
	},
}

var pullCmd = &cobra.Command{
	Use:               "pull <stack-identifier>",
	Short:             "Run 'podman compose pull' for a stack",
	Example:           "  bm pull my-local-app\n  bm pull server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("pull", args)
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
				fmt.Println("\nNo Podman Compose stacks found locally or on configured remote hosts.")
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
	Short: "Run 'podman system prune -af' on specified hosts (local or remote)",
	Long: `Runs 'podman system prune -af' to remove unused data (containers, networks, images, volumes).
Targets can be 'local', specific remote host names, or left empty to target ALL configured hosts (local + remotes).`,
	Example: `  bm prune          # Prune local system AND all configured remote hosts
	 bm prune local       # Prune only the local system
	 bm prune server1     # Prune only the remote host 'server1'
	 bm prune local server1 server2 # Prune local, server1, and server2`,
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
