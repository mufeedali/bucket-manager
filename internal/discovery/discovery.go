// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

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

const maxConcurrentDiscoveries = 8

var sshManager *ssh.Manager

// InitSSHManager sets the package-level SSH manager instance.
func InitSSHManager(manager *ssh.Manager) {
	if sshManager != nil {
		return
	}
	sshManager = manager
}

type Stack struct {
	Name               string
	Path               string // Full local path OR path relative to AbsoluteRemoteRoot on SSH host
	ServerName         string // "local" or the Name field from SSHHost config
	IsRemote           bool
	HostConfig         *config.SSHHost // nil if local
	AbsoluteRemoteRoot string          // empty if local
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
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Errorf("Warning: could not load config to check local_root: %v", err)
	} else if cfg.LocalRoot != "" {
		localRootPath, resolveErr := config.ResolvePath(cfg.LocalRoot)
		if resolveErr != nil {
			logger.Errorf("Warning: could not resolve configured local_root path '%s': %v", cfg.LocalRoot, resolveErr)
			localRootPath = cfg.LocalRoot // Use original path for Stat check
		}

		info, statErr := os.Stat(localRootPath)
		if statErr == nil && info.IsDir() {
			return localRootPath, nil
		}

		// If configured path is invalid, return an error. Do not fall back.
		if statErr != nil {
			return "", fmt.Errorf("configured local_root '%s' is invalid: %w", cfg.LocalRoot, statErr)
		}
		return "", fmt.Errorf("configured local_root '%s' is not a directory", cfg.LocalRoot)
	}

	// Fallback to default locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory for default lookup: %w", err)
	}

	possibleDirs := []string{
		filepath.Join(homeDir, "bucket"),
		filepath.Join(homeDir, "compose-bucket"),
	}

	for _, dir := range possibleDirs {
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			return dir, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Errorf("Warning: error checking default directory %s: %v", dir, err)
		}
	}

	return "", fmt.Errorf("could not find a valid local stack root directory (checked config 'local_root' and defaults: ~/bucket, ~/compose-bucket)")
}

func FindStacks() (<-chan Stack, <-chan error, <-chan struct{}) {
	stackChan := make(chan Stack, 10)
	errorChan := make(chan error, 5)
	doneChan := make(chan struct{})
	var wg sync.WaitGroup

	cfg, configErr := config.LoadConfig()
	if configErr != nil {
		go func() {
			errorChan <- fmt.Errorf("config load failed: %w", configErr)
		}()
	}

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
		localRootDir, err := GetComposeRootDirectory()
		if err == nil {
			localStacks, err := FindLocalStacks(localRootDir)
			if err != nil {
				errorChan <- fmt.Errorf("local discovery failed: %w", err)
			} else {
				for _, s := range localStacks {
					stackChan <- s
				}
			}
		} else if !strings.Contains(err.Error(), "could not find") {
			errorChan <- fmt.Errorf("local root check failed: %w", err)
		}
	}()

	if configErr == nil && len(cfg.SSHHosts) > 0 {
		sem := semaphore.NewWeighted(maxConcurrentDiscoveries)
		ctx := context.Background()

		for i := range cfg.SSHHosts {
			hostConfig := cfg.SSHHosts[i] // Create copy for the goroutine closure
			go func(hc config.SSHHost) {
				defer wg.Done()

				if err := sem.Acquire(ctx, 1); err != nil {
					errorChan <- fmt.Errorf("failed to acquire semaphore for %s: %w", hc.Name, err)
					return
				}
				defer sem.Release(1)

				remoteStacks, err := FindRemoteStacks(&hc)
				if err != nil {
					errorChan <- fmt.Errorf("remote discovery failed for %s: %w", hc.Name, err)
				} else {
					for _, s := range remoteStacks {
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

		// Check for both compose.yaml and compose.yml
		composePathYaml := filepath.Join(stackPath, "compose.yaml")
		composePathYml := filepath.Join(stackPath, "compose.yml")

		_, errYaml := os.Stat(composePathYaml)
		_, errYml := os.Stat(composePathYml)

		// If either file exists, consider it a valid stack
		if errYaml == nil || errYml == nil {
			stacks = append(stacks, Stack{
				Name:       stackName,
				Path:       stackPath,
				ServerName: "local",
				IsRemote:   false,
				HostConfig: nil,
				// AbsoluteRemoteRoot is empty for local stacks
			})
		} else if !os.IsNotExist(errYaml) || !os.IsNotExist(errYml) {
			if !os.IsNotExist(errYaml) {
				logger.Errorf("Warning: could not stat compose files in local stack %s: %v", stackPath, errYaml)
			} else if !os.IsNotExist(errYml) {
				// If yaml was NotExist, but yml had a different error, report that.
				logger.Errorf("Warning: could not stat compose files in local stack %s: %v", stackPath, errYml)
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

	// Command to find directories containing compose.y*ml one level deep using find (representing stack roots)
	remoteFindCmd := fmt.Sprintf(
		`find %s -maxdepth 2 -name 'compose.y*ml' -printf '%%h\\n' | sort -u`,
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
