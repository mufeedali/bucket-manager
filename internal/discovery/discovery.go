// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package discovery

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/ssh"
	"bucket-manager/internal/util"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// sshManager is a package-level variable holding the SSH connection manager.
// This should be initialized by the calling code (CLI/TUI).
var sshManager *ssh.Manager

func InitSSHManager(manager *ssh.Manager) {
	if sshManager != nil {
		return
	}
	sshManager = manager
}

// Project represents a discovered Podman Compose project, either local or remote.
type Project struct {
	Name               string          // Name of the directory (basename of the path)
	Path               string          // Full local path OR path relative to AbsoluteRemoteRoot on SSH host
	ServerName         string          // "local" or the Name field from SSHHost config
	IsRemote           bool            // True if ServerName != "local"
	HostConfig         *config.SSHHost // Pointer to the config for this host (nil if local)
	AbsoluteRemoteRoot string          // The resolved absolute root path on the remote host (empty if local)
}

func (p Project) Identifier() string {
	return fmt.Sprintf("%s (%s)", p.Name, p.ServerName)
}

// GetComposeRootDirectory finds the root directory for *local* compose projects.
// It prioritizes the `local_root` setting in the config file, then checks
// standard locations (`~/bucket`, `~/compose-bucket`) within the user's home directory.
func GetComposeRootDirectory() (string, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		// Log config load error but proceed to defaults, as config might not exist
		fmt.Fprintf(os.Stderr, "Warning: could not load config to check local_root: %v\n", err)
	} else if cfg.LocalRoot != "" {
		localRootPath, resolveErr := config.ResolvePath(cfg.LocalRoot)
		if resolveErr != nil {
			// Log the error but proceed to defaults, as ResolvePath returns original on error
			fmt.Fprintf(os.Stderr, "Warning: could not resolve configured local_root path '%s': %v\n", cfg.LocalRoot, resolveErr)
			localRootPath = cfg.LocalRoot // Use original path for Stat check
		}

		// Check if the configured path exists and is a directory
		info, statErr := os.Stat(localRootPath)
		if statErr == nil && info.IsDir() {
			return localRootPath, nil
		}

		// If configured path is invalid, return an error. Do not fall back.
		if statErr != nil {
			return "", fmt.Errorf("configured local_root '%s' is invalid: %w", cfg.LocalRoot, statErr)
		}
		// If stat succeeded but it's not a directory
		return "", fmt.Errorf("configured local_root '%s' is not a directory", cfg.LocalRoot)

	}

	// Fallback to default locations if LocalRoot was not configured or invalid
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
			// Log or handle unexpected errors during stat, but continue checking other paths
			fmt.Fprintf(os.Stderr, "Warning: error checking default directory %s: %v\n", dir, err)
		}
	}

	return "", fmt.Errorf("could not find a valid local project root directory (checked config 'local_root' and defaults: ~/bucket, ~/compose-bucket)")
}

// FindProjects discovers projects asynchronously, returning channels for results, errors, and completion.
func FindProjects() (<-chan Project, <-chan error, <-chan struct{}) {
	projectChan := make(chan Project, 10)
	errorChan := make(chan error, 5)
	doneChan := make(chan struct{})
	var wg sync.WaitGroup

	cfg, configErr := config.LoadConfig()
	if configErr != nil {
		// Send config error immediately, but don't block other discovery
		go func() { // Send in a goroutine to avoid blocking return if channel buffer is full
			errorChan <- fmt.Errorf("config load failed: %w", configErr)
		}()
	}

	// 2. Determine number of goroutines and Add to WaitGroup *before* launching any
	numGoroutines := 1
	if configErr == nil {
		numGoroutines += len(cfg.SSHHosts)
	}
	wg.Add(numGoroutines)

	// 3. Launch the goroutine to wait and close channels *after* Add
	go func() {
		wg.Wait()
		close(projectChan)
		close(errorChan)
		close(doneChan)
	}()

	go func() {
		defer wg.Done()
		localRootDir, err := GetComposeRootDirectory()
		if err == nil {
			localProjects, err := findLocalProjects(localRootDir)
			if err != nil {
				errorChan <- fmt.Errorf("local discovery failed: %w", err)
			} else {
				for _, p := range localProjects {
					projectChan <- p
				}
			}
		} else if !strings.Contains(err.Error(), "could not find") {
			errorChan <- fmt.Errorf("local root check failed: %w", err)
		}
	}()

	// 5. Launch Remote Discovery Goroutines (only if config loaded)
	if configErr == nil {
		for i := range cfg.SSHHosts {
			hostConfig := cfg.SSHHosts[i] // Create copy for the goroutine closure
			go func(hc config.SSHHost) {
				defer wg.Done()
				remoteProjects, err := findRemoteProjects(&hc)
				if err != nil {
					errorChan <- fmt.Errorf("remote discovery failed for %s: %w", hc.Name, err)
				} else {
					for _, p := range remoteProjects {
						projectChan <- p
					}
				}
			}(hostConfig)
		}
	}

	return projectChan, errorChan, doneChan
}

func findLocalProjects(rootDir string) ([]Project, error) {
	var projects []Project

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read local root directory %s: %w", rootDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectName := entry.Name()
		projectPath := filepath.Join(rootDir, projectName)

		composePathYaml := filepath.Join(projectPath, "compose.yaml")
		composePathYml := filepath.Join(projectPath, "compose.yml")
		_, errYaml := os.Stat(composePathYaml)
		_, errYml := os.Stat(composePathYml)

		if errYaml == nil || errYml == nil {
			projects = append(projects, Project{
				Name:       projectName,
				Path:       projectPath, // Full path for local projects
				ServerName: "local",
				IsRemote:   false,
				HostConfig: nil,
			})
		} else if !os.IsNotExist(errYaml) || !os.IsNotExist(errYml) {
			// Log errors other than "Not Exists"
			fmt.Fprintf(os.Stderr, "Warning: could not stat compose files in local project %s: %v / %v\n", projectPath, errYaml, errYml)
		}
	}

	return projects, nil
}

func findRemoteProjects(hostConfig *config.SSHHost) ([]Project, error) {
	var projects []Project

	if sshManager == nil {
		return nil, fmt.Errorf("ssh manager not initialized for discovery on %s", hostConfig.Name)
	}

	client, err := sshManager.GetClient(*hostConfig)
	if err != nil {
		// GetClient already provides context
		return nil, err
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
		session.Close()
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
				// Error creating session is serious, report it
				return nil, fmt.Errorf("failed to create ssh session for fallback discovery on %s: %w", hostConfig.Name, err)
			}
			resolveCmd := fmt.Sprintf("cd %s && pwd", util.QuoteArgForShell(fallback))
			pwdOutput, resolveErr = session.CombinedOutput(resolveCmd)
			session.Close()

			if resolveErr == nil {
				targetRemoteRoot = fallback // Store the successful fallback *name* for potential logging
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
		// This should ideally not happen if resolveErr was nil, but check anyway
		return nil, fmt.Errorf("resolved remote root path is empty for '%s' (resolved from '%s') on host %s", absoluteRemoteRoot, targetRemoteRoot, hostConfig.Name)
	}

	findSession, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create second ssh session for discovery on %s: %w", hostConfig.Name, err)
	}
	defer findSession.Close()

	// Command to find directories containing compose.y*ml one level deep using fd
	remoteFindCmd := fmt.Sprintf(
		`fd -g -d 2 'compose.y*ml' %s -x dirname {} | sort -u`,
		util.QuoteArgForShell(absoluteRemoteRoot),
	)

	output, err := findSession.CombinedOutput(remoteFindCmd)
	if err != nil {
		// Include output in error message as it might contain stderr
		return nil, fmt.Errorf("remote find command failed for host %s: %w\nOutput: %s", hostConfig.Name, err, string(output))
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))

	for scanner.Scan() {
		fullPath := scanner.Text() // This is the absolute path on the remote machine
		if fullPath == "" {
			continue
		}

		relativePath, err := filepath.Rel(absoluteRemoteRoot, fullPath)
		if err != nil {
			// This error is now more likely to indicate a logic issue or unexpected output from find
			fmt.Fprintf(os.Stderr, "Warning: could not calculate relative path for '%s' from resolved root '%s' on host %s: %v\n", fullPath, absoluteRemoteRoot, hostConfig.Name, err)
			continue
		}
		// Ensure relative path uses forward slashes for consistency
		relativePath = filepath.ToSlash(relativePath)

		projectName := filepath.Base(relativePath)
		if projectName == "." || projectName == "/" {
			continue
		}

		projects = append(projects, Project{
			Name:               projectName,
			Path:               relativePath,
			ServerName:         hostConfig.Name,
			IsRemote:           true,
			HostConfig:         hostConfig,
			AbsoluteRemoteRoot: absoluteRemoteRoot,
		})
	}
	if err := scanner.Err(); err != nil {
		return projects, fmt.Errorf("error reading ssh output for host %s: %w", hostConfig.Name, err)
	}

	return projects, nil
}
