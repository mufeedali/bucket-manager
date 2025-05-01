// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SSHHost defines the configuration for connecting to a remote host via SSH using the internal client.
type SSHHost struct {
	Name       string `yaml:"name"`               // User-friendly identifier (e.g., "server1")
	Hostname   string `yaml:"hostname"`           // Hostname or IP address
	User       string `yaml:"user"`               // Username for SSH connection
	Port       int    `yaml:"port,omitempty"`     // Optional SSH port (defaults to 22)
	KeyPath    string `yaml:"key_path,omitempty"` // Optional path to private key
	Password   string `yaml:"password,omitempty"` // Optional password (plaintext, discouraged)
	RemoteRoot string `yaml:"remote_root"`        // Root directory for projects on the remote host
	Disabled   bool   `yaml:"disabled,omitempty"` // Optional flag to disable this host
}

// Config holds the overall application configuration, including SSH hosts.
type Config struct {
	SSHHosts []SSHHost `yaml:"ssh_hosts"`
}

// DefaultConfigPath returns the default path for the configuration file.
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
	enabledHosts := []SSHHost{}
	for _, host := range cfg.SSHHosts {
		if !host.Disabled {
			enabledHosts = append(enabledHosts, host)
		}
	}
	cfg.SSHHosts = enabledHosts

	return cfg, nil
}

// EnsureConfigDir ensures the configuration directory exists.
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

// SaveConfig saves the provided configuration struct back to the default config file path.
func SaveConfig(cfg Config) error {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return err
	}

	// Ensure the directory exists before trying to write
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
