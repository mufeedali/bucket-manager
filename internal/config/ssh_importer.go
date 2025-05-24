// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package config provides functionality for SSH host configuration importing.
// This file specifically handles importing SSH hosts from the user's ~/.ssh/config file,
// allowing users to easily add their existing SSH configurations to the bucket manager.

package config

import (
	"bucket-manager/internal/logger"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kevinburke/ssh_config"
)

// PotentialHost represents an SSH host configuration parsed from the user's SSH config file.
// These entries can be imported into the bucket manager's configuration.
type PotentialHost struct {
	Alias    string // Host alias as defined in SSH config (e.g., "my-server")
	Hostname string // Actual hostname or IP address to connect to
	User     string // Username for SSH connection
	Port     int    // Port number for SSH connection
	KeyPath  string // Path to the identity file (private key)
}

// DefaultSSHConfigPath returns the standard location of the user's SSH config file.
func DefaultSSHConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Error("Failed to get user home directory for SSH config path", "error", err)
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	sshConfigPath := filepath.Join(homeDir, ".ssh", "config")
	logger.Debug("Determined SSH config path",
		"home_dir", homeDir,
		"ssh_config_path", sshConfigPath)

	return sshConfigPath, nil
}

// ParseSSHConfig reads the user's SSH config file and extracts host configurations.
// It returns a slice of PotentialHost objects that can be imported into the bucket manager.
// If the SSH config file doesn't exist, it returns an empty slice without an error.
func ParseSSHConfig() ([]PotentialHost, error) {
	startTime := time.Now()

	sshConfigPath, err := DefaultSSHConfigPath()
	if err != nil {
		logger.Error("Failed to get SSH config path", "error", err)
		return nil, err
	}

	logger.Debug("Starting SSH config parsing", "ssh_config_path", sshConfigPath)

	f, err := os.Open(sshConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("SSH config file not found, returning empty host list",
				"ssh_config_path", sshConfigPath,
				"duration", time.Since(startTime))
			// Not an error if the file doesn't exist, just return empty results
			return []PotentialHost{}, nil
		}
		logger.Error("Failed to open SSH config file",
			"ssh_config_path", sshConfigPath,
			"error", err,
			"duration", time.Since(startTime))
		return nil, fmt.Errorf("failed to open ssh config file %s: %w", sshConfigPath, err)
	}
	defer f.Close()

	logger.Debug("SSH config file opened successfully", "ssh_config_path", sshConfigPath)

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		logger.Error("Failed to parse SSH config file",
			"ssh_config_path", sshConfigPath,
			"error", err,
			"duration", time.Since(startTime))
		return nil, fmt.Errorf("failed to parse ssh config file %s: %w", sshConfigPath, err)
	}

	logger.Debug("SSH config decoded successfully",
		"ssh_config_path", sshConfigPath,
		"total_hosts", len(cfg.Hosts))

	var potentialHosts []PotentialHost
	processedCount := 0
	skippedCount := 0

	for _, host := range cfg.Hosts {
		// Skip global ("*") or empty patterns
		if len(host.Patterns) == 0 || host.Patterns[0].String() == "*" {
			skippedCount++
			continue
		}

		// Use the first pattern as the alias for import suggestion
		alias := host.Patterns[0].String()

		logger.Debug("Processing SSH host entry", "alias", alias)

		// Get relevant config values for this host alias
		// Ignore errors from cfg.Get, as missing values are handled below
		hostname, _ := cfg.Get(alias, "HostName")
		user, _ := cfg.Get(alias, "User")
		portStr, _ := cfg.Get(alias, "Port")
		keyPath, _ := cfg.Get(alias, "IdentityFile")

		// If HostName is not specified, use the alias itself
		if hostname == "" {
			hostname = alias
		}

		// Default port is 22
		port := 22
		if portStr != "" {
			p, err := strconv.Atoi(portStr)
			if err == nil { // Only use parsed port if conversion is successful
				port = p
			} else {
				logger.Debug("Invalid port value, using default",
					"alias", alias,
					"port_string", portStr,
					"default_port", 22)
			}
			// Ignore conversion errors, keep default port 22
		}

		// Resolve ~ in IdentityFile path using the shared function
		if keyPath != "" {
			resolvedKeyPath, resolveErr := ResolvePath(keyPath)
			if resolveErr == nil {
				keyPath = resolvedKeyPath
				logger.Debug("Resolved SSH key path",
					"alias", alias,
					"original_path", keyPath,
					"resolved_path", resolvedKeyPath)
			} else {
				// Log warning but keep original path if resolution fails
				logger.Warn("Could not resolve SSH key path",
					"alias", alias,
					"key_path", keyPath,
					"error", resolveErr)
			}
		}

		// Only consider hosts with both a hostname and user specified
		if hostname != "" && user != "" {
			potentialHost := PotentialHost{
				Alias:    alias,
				Hostname: hostname,
				User:     user,
				Port:     port,
				KeyPath:  keyPath,
			}
			potentialHosts = append(potentialHosts, potentialHost)
			processedCount++

			logger.Debug("Added potential host for import",
				"alias", alias,
				"hostname", hostname,
				"user", user,
				"port", port,
				"key_path", keyPath)
		} else {
			skippedCount++
			logger.Debug("Skipped host due to missing hostname or user",
				"alias", alias,
				"hostname", hostname,
				"user", user)
		}
	}

	logger.Info("SSH config parsing completed",
		"ssh_config_path", sshConfigPath,
		"total_hosts", len(cfg.Hosts),
		"processed_hosts", processedCount,
		"skipped_hosts", skippedCount,
		"potential_hosts", len(potentialHosts),
		"duration", time.Since(startTime))

	return potentialHosts, nil
}

func ConvertToBucketManagerHost(p PotentialHost, uniqueName, remoteRoot string) (SSHHost, error) {
	logger.Debug("Converting potential host to bucket manager host",
		"alias", p.Alias,
		"hostname", p.Hostname,
		"user", p.User,
		"port", p.Port,
		"unique_name", uniqueName,
		"remote_root", remoteRoot)

	if p.Hostname == "" || p.User == "" {
		logger.Error("Cannot convert potential host with missing required fields",
			"alias", p.Alias,
			"hostname", p.Hostname,
			"user", p.User)
		return SSHHost{}, fmt.Errorf("cannot convert potential host '%s' with missing hostname or user", p.Alias)
	}
	if uniqueName == "" {
		logger.Error("Unique name is required for conversion", "alias", p.Alias)
		return SSHHost{}, fmt.Errorf("a unique name is required for the bucket-manager host")
	}

	host := SSHHost{
		Name:       uniqueName,
		Hostname:   p.Hostname,
		User:       p.User,
		Port:       p.Port,
		KeyPath:    p.KeyPath,
		RemoteRoot: remoteRoot,
	}

	logger.Info("Successfully converted potential host to bucket manager host",
		"original_alias", p.Alias,
		"new_name", uniqueName,
		"hostname", host.Hostname,
		"user", host.User,
		"port", host.Port,
		"key_path", host.KeyPath,
		"remote_root", host.RemoteRoot)

	return host, nil
}
