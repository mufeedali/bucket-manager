// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package config handles application configuration including reading and writing
// configuration files, managing SSH host definitions, and providing access to
// application settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// SSHHost represents a remote SSH host configuration for connecting to
// and managing remote Podman Compose stacks.
type SSHHost struct {
	// Name is the unique identifier for this host configuration
	Name string `yaml:"name"`

	// Hostname is the server address (IP or domain)
	Hostname string `yaml:"hostname"`

	// User is the SSH username for authentication
	User string `yaml:"user"`

	// Port is the SSH port number (optional, defaults to standard SSH port)
	Port int `yaml:"port,omitempty"`

	// KeyPath is the path to the SSH private key file
	KeyPath string `yaml:"key_path,omitempty"`

	// Password is an optional authentication method (plaintext, discouraged)
	Password string `yaml:"password,omitempty"`

	// RemoteRoot is the directory path to search for stacks on the remote host
	RemoteRoot string `yaml:"remote_root,omitempty"`

	// Disabled indicates whether this host should be skipped during discovery
	Disabled bool `yaml:"disabled,omitempty"`
}

// Config represents the top-level application configuration
type Config struct {
	// LocalRoot is the custom directory to search for stacks locally (optional)
	LocalRoot string `yaml:"local_root,omitempty"`

	// SSHHosts is a list of remote SSH host configurations
	SSHHosts []SSHHost `yaml:"ssh_hosts"`
}

func DefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	return filepath.Join(configDir, "bucket-manager", "config.yaml"), nil
}

func LoadConfig() (Config, error) {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

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
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path, fmt.Errorf("could not get user home directory to resolve path '%s': %w", path, err)
	}

	return filepath.Join(homeDir, path[2:]), nil
}
