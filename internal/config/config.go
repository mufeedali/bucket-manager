// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// SSHHost defines the configuration for connecting to a remote host via SSH using the internal client.
type SSHHost struct {
	Name       string `yaml:"name"`               // User-friendly identifier (e.g., "server1")
	Hostname   string `yaml:"hostname"`
	User       string `yaml:"user"`
	Port       int    `yaml:"port,omitempty"`     // Optional SSH port (defaults to 22)
	KeyPath    string `yaml:"key_path,omitempty"` // Optional path to private key
	Password   string `yaml:"password,omitempty"` // Optional password (plaintext, discouraged)
	RemoteRoot string `yaml:"remote_root,omitempty"` // Optional root directory for projects on the remote host (defaults to ~/bucket or ~/compose-bucket)
	Disabled   bool   `yaml:"disabled,omitempty"` // Optional flag to disable this host
}

// Config holds the overall application configuration, including SSH hosts and local settings.
type Config struct {
	LocalRoot string    `yaml:"local_root,omitempty"` // Optional custom root directory for local projects
	SSHHosts  []SSHHost `yaml:"ssh_hosts"`
}

func DefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	return filepath.Join(configDir, "bucket-manager", "config.yaml"), nil
}

// LoadConfig loads the configuration from the default path.
// If the file doesn't exist, it returns an empty config without error.
func LoadConfig() (Config, error) {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil // Config file doesn't exist, return default empty config
		}
		return Config{}, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Filter out disabled hosts
	cfg.SSHHosts = slices.DeleteFunc(cfg.SSHHosts, func(h SSHHost) bool {
		return h.Disabled
	})

	return cfg, nil
}

func EnsureConfigDir() error {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return err
	}
	configDir := filepath.Dir(configPath)
	err = os.MkdirAll(configDir, 0750) // rwxr-x---
	if err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}
	return nil
}

func SaveConfig(cfg Config) error {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return err
	}

	err = EnsureConfigDir()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Write with permissions rw-r----- (0640)
	err = os.WriteFile(configPath, data, 0640)
	if err != nil {
		return fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}

	return nil
}
func ResolvePath(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		// Return unchanged if it's not a tilde path (could be absolute or relative)
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path, fmt.Errorf("could not get user home directory to resolve path '%s': %w", path, err)
	}

	return filepath.Join(homeDir, path[2:]), nil
}
