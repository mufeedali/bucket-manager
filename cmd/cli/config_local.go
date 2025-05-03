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
			resolvedPath, resolveErr := config.ResolvePath(cfg.LocalRoot)
			if resolveErr == nil {
				fmt.Printf("Resolved path:         %s\n", resolvedPath)
			} else {
				errorColor.Printf("Warning: Could not resolve configured path: %v\n", resolveErr)
			}
		} else {
			fmt.Println("Local root not explicitly configured.")
			fmt.Printf("Default search paths: %s, %s\n", identifierColor.Sprint("~/bucket"), identifierColor.Sprint("~/compose-bucket"))
		}

		// Report the path that discovery will actually use
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
				source = "(unknown source)" // Should ideally not happen with current discovery logic
			}
			successColor.Printf("Effective path being used: %s %s\n", activePath, source)

		} else if strings.Contains(activeErr.Error(), "could not find") {
			if cfg.LocalRoot != "" {
				errorColor.Printf("Warning: Configured path '%s' not found, and no default path exists.\n", cfg.LocalRoot)
			} else {
				errorColor.Println("Warning: Neither default path exists.")
			}
		} else {
			// Report other errors encountered during discovery check
			errorColor.Printf("Error determining effective path: %v\n", activeErr)
		}
	},
}

func init() {
	configCmd.AddCommand(configSetLocalRootCmd)
	configCmd.AddCommand(configGetLocalRootCmd)
}
