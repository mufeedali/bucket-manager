// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// dimColor is used for less important/secondary text in the CLI output
var dimColor = color.New(color.Faint)

// configCmd is the parent command for all configuration-related subcommands
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage bucket-manager configuration",
	Long: `Provides subcommands to manage different aspects of the bucket-manager configuration.
This includes SSH host configurations, local root path settings, and container runtime selection.`,
}

// Local root configuration commands
var configSetLocalRootCmd = &cobra.Command{
	Use:   "set-local-root <path>",
	Short: "Set the custom root directory for local stacks",
	Long: `Sets the root directory where bucket-manager will look for local compose stacks.
Use an absolute path or a path starting with '~/' (e.g., '~/my-compose-stacks').
If set, this overrides the default search paths (~/bucket, ~/compose-bucket).
To revert to default behavior, set the path to an empty string: bm config set-local-root ""`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		localRootPath := args[0]

		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		if localRootPath != "" && !strings.HasPrefix(localRootPath, "/") && !strings.HasPrefix(localRootPath, "~/") {
			logger.Error("Error: Path must be absolute or start with '~/'")
			os.Exit(1)
		}

		cfg.LocalRoot = localRootPath

		err = config.SaveConfig(cfg)
		if err != nil {
			logger.Errorf("Error saving configuration: %v", err)
			os.Exit(1)
		}

		if localRootPath == "" {
			successColor.Println("Local stack root reset to default search paths (~/bucket, ~/compose-bucket).")
		} else {
			successColor.Printf("Local stack root set to: %s\n", localRootPath)
		}
	},
}

var configGetLocalRootCmd = &cobra.Command{
	Use:   "get-local-root",
	Short: "Show the currently configured local stack root directory",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		if cfg.LocalRoot != "" {
			fmt.Printf("Configured local root: %s\n", identifierColor.Sprint(cfg.LocalRoot))
			resolvedPath, resolveErr := config.ResolvePath(cfg.LocalRoot)
			if resolveErr == nil {
				fmt.Printf("Resolved path:         %s\n", resolvedPath)
			} else {
				fmt.Printf("Warning: Could not resolve configured path: %v\n", resolveErr)
			}
		} else {
			fmt.Println("Local root not explicitly configured.")
			fmt.Printf("Default search paths: %s, %s\n", identifierColor.Sprint("~/bucket"), identifierColor.Sprint("~/compose-bucket"))
		}

		activePath, activeErr := discovery.GetComposeRootDirectory()
		if activeErr == nil {
			// Determine if the active path came from config or default
			resolvedConfigPath, _ := config.ResolvePath(cfg.LocalRoot) // Resolve even if empty
			homeDir, _ := os.UserHomeDir()
			defaultBucket := filepath.Join(homeDir, "bucket")
			defaultComposeBucket := filepath.Join(homeDir, "compose-bucket")

			source := ""
			if cfg.LocalRoot != "" && activePath == resolvedConfigPath {
				source = "(from config)"
			} else if activePath == defaultBucket || activePath == defaultComposeBucket {
				source = "(default)"
			} else {
				source = "(unknown source)"
			}
			successColor.Printf("Effective path being used: %s %s\n", activePath, source)

		} else if strings.Contains(activeErr.Error(), "could not find") {
			if cfg.LocalRoot != "" {
				fmt.Printf("Warning: Configured path '%s' not found, and no default path exists.\n", cfg.LocalRoot)
			} else {
				fmt.Println("Warning: Neither default path exists.")
			}
		} else {
			logger.Errorf("Error determining effective path: %v", activeErr)
		}
	},
}

// Runtime configuration commands
var configSetRuntimeCmd = &cobra.Command{
	Use:   "set-runtime <runtime>",
	Short: "Set the container runtime (podman or docker)",
	Long: `Sets the container runtime to use for compose operations.
Valid values are 'podman' or 'docker'. This affects all stack operations.

Examples:
  bm config set-runtime docker    # Use Docker
  bm config set-runtime podman    # Use Podman (default)`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runtime := strings.ToLower(args[0])

		// Validate runtime
		if runtime != "podman" && runtime != "docker" {
			logger.Error("Error: Runtime must be either 'podman' or 'docker'")
			os.Exit(1)
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		cfg.ContainerRuntime = runtime

		err = config.SaveConfig(cfg)
		if err != nil {
			logger.Errorf("Error saving configuration: %v", err)
			os.Exit(1)
		}

		successColor.Printf("Container runtime set to: %s\n", runtime)

		// Show a helpful tip about compose files
		fmt.Println("\nTip: Make sure your compose files are compatible with the runtime chosen.")
		fmt.Println("Some features may be specific to a runtime, Docker for example.")
	},
}

var configGetRuntimeCmd = &cobra.Command{
	Use:   "get-runtime",
	Short: "Show the currently configured container runtime",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		currentRuntime := cfg.ContainerRuntime
		fmt.Printf("Current container runtime: %s", identifierColor.Sprint(currentRuntime))
		if cfg.ContainerRuntime == "" {
			fmt.Print(" (default)")
		}
		fmt.Println()

		// Show which binary will be used
		fmt.Printf("Commands will use: %s compose\n", currentRuntime)
	},
}

func init() {
	// Add local root commands
	configCmd.AddCommand(configSetLocalRootCmd)
	configCmd.AddCommand(configGetLocalRootCmd)

	// Add runtime commands
	configCmd.AddCommand(configSetRuntimeCmd)
	configCmd.AddCommand(configGetRuntimeCmd)

	// Add the config command to root
	rootCmd.AddCommand(configCmd)
}
