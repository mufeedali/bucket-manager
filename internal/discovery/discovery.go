// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package discovery provides functionality for finding compose stack directories
// in both local and remote environments. It handles scanning directories,
// detecting compose files, and determining stack status.
package discovery

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/logger"
	"bucket-manager/internal/ssh"
	"bucket-manager/internal/util"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
)

// maxConcurrentDiscoveries limits the number of concurrent discovery operations
// to prevent overwhelming local or remote systems
const maxConcurrentDiscoveries = 8

// sshManager provides access to SSH connections for remote discovery operations
var sshManager *ssh.Manager

// InitSSHManager sets the package-level SSH manager instance.
// This must be called before performing any remote discovery operations.
func InitSSHManager(manager *ssh.Manager) {
	if sshManager != nil {
		return
	}
	sshManager = manager
}

// Stack represents a discovered compose stack, which is a directory
// containing compose files (compose.yaml, compose.yml, docker-compose.yaml, docker-compose.yml, etc.)
// The Stack can be either local or on a remote SSH host.
type Stack struct {
	Name               string          // Name of the stack (derived from directory name)
	Path               string          // Full local path OR path relative to AbsoluteRemoteRoot on SSH host
	ServerName         string          // "local" or the Name field from SSHHost config
	IsRemote           bool            // True if stack is on a remote server, false if local
	HostConfig         *config.SSHHost // SSH host configuration (nil if local)
	AbsoluteRemoteRoot string          // Root directory on remote host (empty if local)
}

// Identifier returns the unique string representation (e.g., "my-app" or "server1:my-app").
func (s Stack) Identifier() string {
	if !s.IsRemote {
		// Always return the explicit "local:" prefix for clarity and completion consistency
		return fmt.Sprintf("local:%s", s.Name)
	}
	return fmt.Sprintf("%s:%s", s.ServerName, s.Name)
}

// GetComposeRootDirectory finds the root directory for local compose stacks,
// checking config override first, then defaults.
func GetComposeRootDirectory() (string, error) {
	logger.Debug("Determining compose root directory")

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Warn("Could not load config to check local_root", "error", err)
	} else if cfg.LocalRoot != "" {
		logger.Debug("Using configured local root", "configured_path", cfg.LocalRoot)

		localRootPath, resolveErr := config.ResolvePath(cfg.LocalRoot)
		if resolveErr != nil {
			logger.Warn("Could not resolve configured local_root path",
				"configured_path", cfg.LocalRoot,
				"error", resolveErr)
			localRootPath = cfg.LocalRoot // Use original path for Stat check
		}

		info, statErr := os.Stat(localRootPath)
		if statErr == nil && info.IsDir() {
			logger.Info("Using configured local root directory",
				"path", localRootPath,
				"resolved_from", cfg.LocalRoot)
			return localRootPath, nil
		}

		// If configured path is invalid, return an error. Do not fall back.
		if statErr != nil {
			logger.Error("Configured local_root is invalid",
				"configured_path", cfg.LocalRoot,
				"resolved_path", localRootPath,
				"error", statErr)
			return "", fmt.Errorf("configured local_root '%s' is invalid: %w", cfg.LocalRoot, statErr)
		}
		logger.Error("Configured local_root is not a directory",
			"configured_path", cfg.LocalRoot,
			"resolved_path", localRootPath)
		return "", fmt.Errorf("configured local_root '%s' is not a directory", cfg.LocalRoot)
	}

	logger.Debug("No local root configured, checking default locations")

	// Fallback to default locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Error("Could not get user home directory for default lookup", "error", err)
		return "", fmt.Errorf("could not get user home directory for default lookup: %w", err)
	}

	possibleDirs := []string{
		filepath.Join(homeDir, "bucket"),
		filepath.Join(homeDir, "compose-bucket"),
	}

	logger.Debug("Checking default directories", "candidates", possibleDirs)

	for _, dir := range possibleDirs {
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			logger.Info("Using default local root directory", "path", dir)
			return dir, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("Error checking default directory", "directory", dir, "error", err)
		}
	}

	logger.Error("No valid local stack root directory found",
		"checked_config", cfg.LocalRoot != "",
		"checked_defaults", possibleDirs)
	return "", fmt.Errorf("could not find a valid local stack root directory (checked config 'local_root' and defaults: ~/bucket, ~/compose-bucket)")
}

func FindStacks() (<-chan Stack, <-chan error, <-chan struct{}) {
	logger.Info("Starting stack discovery")

	stackChan := make(chan Stack, 10)
	errorChan := make(chan error, 5)
	doneChan := make(chan struct{})
	var wg sync.WaitGroup

	cfg, configErr := config.LoadConfig()
	if configErr != nil {
		logger.Error("Failed to load configuration for stack discovery", "error", configErr)
		go func() {
			errorChan <- fmt.Errorf("config load failed: %w", configErr)
		}()
	}

	logger.Debug("Configuration loaded for discovery",
		"ssh_host_count", func() int {
			if cfg.SSHHosts == nil {
				return 0
			}
			return len(cfg.SSHHosts)
		}(),
		"local_root_configured", cfg.LocalRoot != "")

	numGoroutines := 1
	if configErr == nil {
		numGoroutines += len(cfg.SSHHosts)
	}
	wg.Add(numGoroutines)

	go func() {
		wg.Wait()
		close(stackChan)
		close(errorChan)
		close(doneChan)
	}()

	go func() {
		defer wg.Done()
		logger.Debug("Starting local stack discovery")

		localRootDir, err := GetComposeRootDirectory()
		if err == nil {
			logger.Debug("Local root directory found, searching for stacks", "root_dir", localRootDir)

			localStacks, err := FindLocalStacks(localRootDir)
			if err != nil {
				logger.Error("Local stack discovery failed", "root_dir", localRootDir, "error", err)
				errorChan <- fmt.Errorf("local discovery failed: %w", err)
			} else {
				logger.Info("Local stack discovery completed",
					"root_dir", localRootDir,
					"stack_count", len(localStacks))
				for _, s := range localStacks {
					logger.Debug("Local stack found", "stack_name", s.Name, "path", s.Path)
					stackChan <- s
				}
			}
		} else if !strings.Contains(err.Error(), "could not find") {
			logger.Error("Local root directory check failed", "error", err)
			errorChan <- fmt.Errorf("local root check failed: %w", err)
		} else {
			logger.Debug("No local root directory configured or found")
		}
	}()

	if configErr == nil && len(cfg.SSHHosts) > 0 {
		logger.Debug("Starting remote stack discovery", "host_count", len(cfg.SSHHosts))

		sem := semaphore.NewWeighted(maxConcurrentDiscoveries)
		ctx := context.Background()

		for i := range cfg.SSHHosts {
			hostConfig := cfg.SSHHosts[i] // Create copy for the goroutine closure
			go func(hc config.SSHHost) {
				defer wg.Done()

				logger.Debug("Starting remote discovery for host",
					"host_name", hc.Name,
					"hostname", hc.Hostname,
					"remote_root", hc.RemoteRoot,
					"disabled", hc.Disabled)

				if hc.Disabled {
					logger.Debug("Skipping disabled host", "host_name", hc.Name)
					return
				}

				if err := sem.Acquire(ctx, 1); err != nil {
					logger.Error("Failed to acquire semaphore for remote discovery",
						"host_name", hc.Name, "error", err)
					errorChan <- fmt.Errorf("failed to acquire semaphore for %s: %w", hc.Name, err)
					return
				}
				defer sem.Release(1)

				remoteStacks, err := FindRemoteStacks(&hc)
				if err != nil {
					logger.Error("Remote stack discovery failed",
						"host_name", hc.Name,
						"hostname", hc.Hostname,
						"error", err)
					errorChan <- fmt.Errorf("remote discovery failed for %s: %w", hc.Name, err)
				} else {
					logger.Info("Remote stack discovery completed",
						"host_name", hc.Name,
						"hostname", hc.Hostname,
						"stack_count", len(remoteStacks))
					for _, s := range remoteStacks {
						logger.Debug("Remote stack found",
							"stack_name", s.Name,
							"host_name", s.ServerName,
							"path", s.Path)
						stackChan <- s
					}
				}
			}(hostConfig)
		}
	}

	return stackChan, errorChan, doneChan
}

func FindLocalStacks(rootDir string) ([]Stack, error) {
	var stacks []Stack

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read local root directory %s: %w", rootDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		stackName := entry.Name()
		stackPath := filepath.Join(rootDir, stackName)

		// Check for common compose file names
		composeFiles := []string{
			"compose.yaml",
			"compose.yml",
			"docker-compose.yaml",
			"docker-compose.yml",
		}

		hasComposeFile := false
		var statErrors []error

		for _, composeFile := range composeFiles {
			composePath := filepath.Join(stackPath, composeFile)
			_, err := os.Stat(composePath)
			if err == nil {
				hasComposeFile = true
				break
			} else if !os.IsNotExist(err) {
				statErrors = append(statErrors, err)
			}
		}

		// If any compose file exists, consider it a valid stack
		if hasComposeFile {
			stacks = append(stacks, Stack{
				Name:       stackName,
				Path:       stackPath,
				ServerName: "local",
				IsRemote:   false,
				HostConfig: nil,
				// AbsoluteRemoteRoot is empty for local stacks
			})
		} else if len(statErrors) > 0 {
			// Only log warnings if there were non-NotExist errors
			for _, statErr := range statErrors {
				logger.Errorf("Warning: could not stat compose files in local stack %s: %v", stackPath, statErr)
			}
		}
	}

	return stacks, nil
}

func FindRemoteStacks(hostConfig *config.SSHHost) ([]Stack, error) {
	var stacks []Stack

	if sshManager == nil {
		return nil, fmt.Errorf("ssh manager not initialized for discovery on %s", hostConfig.Name)
	}

	client, err := sshManager.GetClient(*hostConfig)
	if err != nil {
		return nil, err // GetClient already provides context
	}

	var targetRemoteRoot string
	var resolveErr error
	var pwdOutput []byte

	if hostConfig.RemoteRoot != "" {
		targetRemoteRoot = hostConfig.RemoteRoot
		session, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("failed to create ssh session for discovery on %s: %w", hostConfig.Name, err)
		}
		resolveCmd := fmt.Sprintf("cd %s && pwd", util.QuoteArgForShell(targetRemoteRoot))
		pwdOutput, resolveErr = session.CombinedOutput(resolveCmd)
		if err := session.Close(); err != nil {
			logger.Errorf("Error closing SSH session for %s (resolve path): %v", hostConfig.Name, err)
		}
		if resolveErr != nil {
			return nil, fmt.Errorf("failed to resolve configured remote root path '%s' on host %s: %w\nOutput: %s", targetRemoteRoot, hostConfig.Name, resolveErr, string(pwdOutput))
		}
	} else {
		// Configured root is empty, try fallbacks
		fallbacks := []string{"~/bucket", "~/compose-bucket"}
		foundFallback := false
		for _, fallback := range fallbacks {
			session, err := client.NewSession()
			if err != nil {
				return nil, fmt.Errorf("failed to create ssh session for fallback discovery on %s: %w", hostConfig.Name, err)
			}
			resolveCmd := fmt.Sprintf("cd %s && pwd", util.QuoteArgForShell(fallback))
			pwdOutput, resolveErr = session.CombinedOutput(resolveCmd)

			if resolveErr == nil {
				targetRemoteRoot = fallback
				foundFallback = true
				break
			}
		}

		if !foundFallback {
			return nil, fmt.Errorf("remote_root not configured for host %s, and default fallbacks ('~/bucket', '~/compose-bucket') could not be resolved", hostConfig.Name)
		}
	}

	absoluteRemoteRoot := strings.TrimSpace(string(pwdOutput))
	if absoluteRemoteRoot == "" {
		return nil, fmt.Errorf("resolved remote root path is empty for '%s' (resolved from '%s') on host %s", absoluteRemoteRoot, targetRemoteRoot, hostConfig.Name)
	}

	findSession, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create second ssh session for discovery on %s: %w", hostConfig.Name, err)
	}
	// CombinedOutput handles the session lifecycle for findSession.

	// Command to find directories containing any supported compose files one level deep using find (representing stack roots)
	remoteFindCmd := fmt.Sprintf(
		`find %s -maxdepth 2 \( -name 'compose.y*ml' -o -name 'docker-compose.y*ml' \) -printf '%%h\\n' | sort -u`,
		util.QuoteArgForShell(absoluteRemoteRoot),
	)

	output, err := findSession.CombinedOutput(remoteFindCmd)
	if err != nil {
		return nil, fmt.Errorf("remote find command failed for host %s: %w\nOutput: %s", hostConfig.Name, err, string(output))
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))

	for scanner.Scan() {
		fullPath := scanner.Text()
		if fullPath == "" {
			continue
		}

		relativePath, err := filepath.Rel(absoluteRemoteRoot, fullPath)
		if err != nil {
			logger.Errorf("Warning: could not calculate relative path for '%s' from resolved root '%s' on host %s: %v", fullPath, absoluteRemoteRoot, hostConfig.Name, err)
			continue
		}
		relativePath = filepath.ToSlash(relativePath) // Ensure forward slashes

		stackName := filepath.Base(relativePath)
		if stackName == "." || stackName == "/" {
			continue
		}

		stacks = append(stacks, Stack{
			Name:               stackName,
			Path:               relativePath,
			ServerName:         hostConfig.Name,
			IsRemote:           true,
			HostConfig:         hostConfig,
			AbsoluteRemoteRoot: absoluteRemoteRoot,
		})
	}
	if err := scanner.Err(); err != nil {
		return stacks, fmt.Errorf("error reading ssh output for host %s: %w", hostConfig.Name, err)
	}

	return stacks, nil
}
