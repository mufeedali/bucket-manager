// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package config

import (
	"bucket-manager/internal/logger"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kevinburke/ssh_config"
)

type PotentialHost struct {
	Alias    string
	Hostname string
	User     string
	Port     int
	KeyPath  string
}

func DefaultSSHConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".ssh", "config"), nil
}

func ParseSSHConfig() ([]PotentialHost, error) {
	sshConfigPath, err := DefaultSSHConfigPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(sshConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []PotentialHost{}, nil
		}
		return nil, fmt.Errorf("failed to open ssh config file %s: %w", sshConfigPath, err)
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ssh config file %s: %w", sshConfigPath, err)
	}

	var potentialHosts []PotentialHost

	for _, host := range cfg.Hosts {
		// Skip global ("*") or empty patterns
		if len(host.Patterns) == 0 || host.Patterns[0].String() == "*" {
			continue
		}

		// Use the first pattern as the alias for import suggestion
		alias := host.Patterns[0].String()

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
			}
			// Ignore conversion errors, keep default port 22
		}

		// Resolve ~ in IdentityFile path using the shared function
		if keyPath != "" {
			resolvedKeyPath, resolveErr := ResolvePath(keyPath)
			if resolveErr == nil {
				keyPath = resolvedKeyPath
			} else {
				// Log warning but keep original path if resolution fails
				logger.Errorf("Warning: could not resolve key path '%s' for host '%s': %v", keyPath, alias, resolveErr)
			}
		}

		// Only consider hosts with both a hostname and user specified
		if hostname != "" && user != "" {
			potentialHosts = append(potentialHosts, PotentialHost{
				Alias:    alias,
				Hostname: hostname,
				User:     user,
				Port:     port,
				KeyPath:  keyPath,
			})
		}
	}

	return potentialHosts, nil
}

func ConvertToBucketManagerHost(p PotentialHost, uniqueName, remoteRoot string) (SSHHost, error) {
	if p.Hostname == "" || p.User == "" {
		return SSHHost{}, fmt.Errorf("cannot convert potential host '%s' with missing hostname or user", p.Alias)
	}
	if uniqueName == "" {
		return SSHHost{}, fmt.Errorf("a unique name is required for the bucket-manager host")
	}

	return SSHHost{
		Name:       uniqueName,
		Hostname:   p.Hostname,
		User:       p.User,
		Port:       p.Port,
		KeyPath:    p.KeyPath,
		RemoteRoot: remoteRoot,
	}, nil
}
