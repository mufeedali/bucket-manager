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

var sshManager *ssh.Manager

// InitSSHManager sets the package-level SSH manager instance.
func InitSSHManager(manager *ssh.Manager) {
	if sshManager != nil {
		return
	}
	sshManager = manager
}

type Project struct {
	Name               string
	Path               string // Full local path OR path relative to AbsoluteRemoteRoot on SSH host
	ServerName         string // "local" or the Name field from SSHHost config
	IsRemote           bool
	HostConfig         *config.SSHHost // nil if local
	AbsoluteRemoteRoot string          // empty if local
}

// Identifier returns the unique string representation (e.g., "my-app" or "server1:my-app").
func (p Project) Identifier() string {
	if !p.IsRemote {
		return p.Name // Implicit local
	}
	return fmt.Sprintf("%s:%s", p.ServerName, p.Name)
}

// GetComposeRootDirectory finds the root directory for local compose stacks,
// checking config override first, then defaults.
func GetComposeRootDirectory() (string, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config to check local_root: %v\n", err)
	} else if cfg.LocalRoot != "" {
		localRootPath, resolveErr := config.ResolvePath(cfg.LocalRoot)
		if resolveErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not resolve configured local_root path '%s': %v\n", cfg.LocalRoot, resolveErr)
			localRootPath = cfg.LocalRoot // Use original path for Stat check
		}

		info, statErr := os.Stat(localRootPath)
		if statErr == nil && info.IsDir() {
			return localRootPath, nil // Configured path is valid
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
			fmt.Fprintf(os.Stderr, "Warning: error checking default directory %s: %v\n", dir, err)
		}
	}

	return "", fmt.Errorf("could not find a valid local stack root directory (checked config 'local_root' and defaults: ~/bucket, ~/compose-bucket)")
}

func FindProjects() (<-chan Project, <-chan error, <-chan struct{}) {
	projectChan := make(chan Project, 10)
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
		close(projectChan)
		close(errorChan)
		close(doneChan)
	}()

	go func() {
		defer wg.Done()
		localRootDir, err := GetComposeRootDirectory()
		if err == nil {
			localProjects, err := FindLocalProjects(localRootDir)
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

	if configErr == nil {
		for i := range cfg.SSHHosts {
			hostConfig := cfg.SSHHosts[i] // Create copy for the goroutine closure
			go func(hc config.SSHHost) {
				defer wg.Done()
				remoteProjects, err := FindRemoteProjects(&hc)
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

func FindLocalProjects(rootDir string) ([]Project, error) {
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
		_, errYaml := os.Stat(composePathYaml)

		if errYaml == nil {
			projects = append(projects, Project{
				Name:       projectName,
				Path:       projectPath,
				ServerName: "local",
				IsRemote:   false,
				HostConfig: nil,
			})
		} else if !os.IsNotExist(errYaml) {
			fmt.Fprintf(os.Stderr, "Warning: could not stat compose files in local stack %s: %v\n", projectPath, errYaml)
		}
	}

	return projects, nil
}

func FindRemoteProjects(hostConfig *config.SSHHost) ([]Project, error) {
	var projects []Project

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
				return nil, fmt.Errorf("failed to create ssh session for fallback discovery on %s: %w", hostConfig.Name, err)
			}
			resolveCmd := fmt.Sprintf("cd %s && pwd", util.QuoteArgForShell(fallback))
			pwdOutput, resolveErr = session.CombinedOutput(resolveCmd)
			session.Close()

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
	defer findSession.Close()

	// Command to find directories containing compose.y*ml one level deep using fd (representing stack roots)
	remoteFindCmd := fmt.Sprintf(
		`fd -g -d 2 'compose.y*ml' %s -x dirname {} | sort -u`,
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
			fmt.Fprintf(os.Stderr, "Warning: could not calculate relative path for '%s' from resolved root '%s' on host %s: %v\n", fullPath, absoluteRemoteRoot, hostConfig.Name, err)
			continue
		}
		relativePath = filepath.ToSlash(relativePath) // Ensure forward slashes

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
