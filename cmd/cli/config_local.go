// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var configSetLocalRootCmd = &cobra.Command{
	Use:   "set-local-root <path>",
	Short: "Set the custom root directory for local projects",
	Long: `Sets the root directory where bucket-manager will look for local Podman Compose projects.
Use an absolute path or a path starting with '~/' (e.g., '~/my-compose-projects').
If set, this overrides the default search paths (~/bucket, ~/compose-bucket).
To revert to default behavior, set the path to an empty string: bm config set-local-root ""`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		localRootPath := args[0]

		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Basic validation (more thorough check happens during discovery)
		if localRootPath != "" && !strings.HasPrefix(localRootPath, "/") && !strings.HasPrefix(localRootPath, "~/") {
			errorColor.Fprintf(os.Stderr, "Error: Path must be absolute or start with '~/'\n")
			os.Exit(1)
		}

		cfg.LocalRoot = localRootPath

		err = config.SaveConfig(cfg)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		if localRootPath == "" {
			successColor.Println("Local project root reset to default search paths (~/bucket, ~/compose-bucket).")
		} else {
			successColor.Printf("Local project root set to: %s\n", localRootPath)
		}
	},
}

var configGetLocalRootCmd = &cobra.Command{
	Use:   "get-local-root",
	Short: "Show the currently configured local project root directory",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		if cfg.LocalRoot != "" {
			fmt.Printf("Configured local root: %s\n", identifierColor.Sprint(cfg.LocalRoot))
			// Optionally, we could try resolving it here for user feedback
			resolvedPath, resolveErr := config.ResolvePath(cfg.LocalRoot)
			if resolveErr == nil {
				fmt.Printf("Resolved path:         %s\n", resolvedPath)
			} else {
				errorColor.Printf("Warning: Could not resolve configured path: %v\n", resolveErr)
			}
		} else {
			fmt.Println("Local root not explicitly configured.")
			fmt.Printf("Using default search paths: %s, %s\n", identifierColor.Sprint("~/bucket"), identifierColor.Sprint("~/compose-bucket"))
			// Show the actual default found, if any
			defaultPath, defaultErr := discovery.GetComposeRootDirectory() // This will now check config first, then defaults
			if defaultErr == nil {
				// Check if the found path is actually one of the defaults or the configured one
				var resolvedConfigPath string
				if cfg.LocalRoot != "" {
					resolvedConfigPath, _ = config.ResolvePath(cfg.LocalRoot)
				}
				homeDir, _ := os.UserHomeDir()
				defaultBucket := filepath.Join(homeDir, "bucket")
				defaultComposeBucket := filepath.Join(homeDir, "compose-bucket")

				if defaultPath == defaultBucket || defaultPath == defaultComposeBucket {
					successColor.Printf("Currently active default: %s\n", defaultPath)
				} else if cfg.LocalRoot != "" && defaultPath == resolvedConfigPath {
					successColor.Printf("Currently active (from config): %s\n", defaultPath)
				} else {
					// This case might occur if GetComposeRootDirectory found a valid but unexpected path
					statusColor.Printf("Currently active path: %s\n", defaultPath)
				}
			} else if strings.Contains(defaultErr.Error(), "could not find") {
				errorColor.Println("Neither default path currently exists.")
			} else {
				errorColor.Printf("Error checking default paths: %v\n", defaultErr)
			}
		}
	},
}

func init() {
	configCmd.AddCommand(configSetLocalRootCmd)
	configCmd.AddCommand(configGetLocalRootCmd)
}
