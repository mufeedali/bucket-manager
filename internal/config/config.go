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
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"bucket-manager/internal/logger"
)

// SSHHost represents a remote SSH host configuration for connecting to
// and managing remote compose stacks.
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

	// ContainerRuntime specifies which container runtime to use (podman or docker)
	// Defaults to "podman" if not specified
	ContainerRuntime string `yaml:"container_runtime,omitempty"`

	// SSHHosts is a list of remote SSH host configurations
	SSHHosts []SSHHost `yaml:"ssh_hosts"`
}

func DefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		logger.Error("Failed to get user config directory", "error", err)
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "bucket-manager", "config.yaml")
	logger.Debug("Determined default config path",
		"config_dir", configDir,
		"config_path", configPath)

	return configPath, nil
}

func LoadConfig() (Config, error) {
	startTime := time.Now()

	configPath, err := DefaultConfigPath()
	if err != nil {
		logger.Error("Failed to get default config path", "error", err)
		return Config{}, err
	}

	logger.Debug("Loading configuration", "config_path", configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("Configuration file not found, using defaults",
				"config_path", configPath,
				"duration", time.Since(startTime))
			// Return empty config - defaults will be set below
			return Config{}, nil
		}
		logger.Error("Failed to read config file",
			"config_path", configPath,
			"error", err,
			"duration", time.Since(startTime))
		return Config{}, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	logger.Debug("Configuration file read successfully",
		"config_path", configPath,
		"file_size", len(data))

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		logger.Error("Failed to parse YAML config",
			"config_path", configPath,
			"error", err,
			"duration", time.Since(startTime))
		return Config{}, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Set default container runtime if not specified
	if cfg.ContainerRuntime == "" {
		cfg.ContainerRuntime = "podman"
		logger.Debug("Applied default container runtime", "runtime", "podman")
	}

	logger.Info("Configuration loaded successfully",
		"config_path", configPath,
		"container_runtime", cfg.ContainerRuntime,
		"local_root", cfg.LocalRoot,
		"ssh_hosts_count", len(cfg.SSHHosts),
		"duration", time.Since(startTime))

	return cfg, nil
}

func EnsureConfigDir() error {
	configPath, err := DefaultConfigPath()
	if err != nil {
		logger.Error("Failed to get default config path for directory creation", "error", err)
		return err
	}

	configDir := filepath.Dir(configPath)
	logger.Debug("Ensuring config directory exists", "config_dir", configDir)

	// Check if directory already exists
	if info, statErr := os.Stat(configDir); statErr == nil {
		if info.IsDir() {
			logger.Debug("Config directory already exists", "config_dir", configDir)
			return nil
		} else {
			logger.Error("Config path exists but is not a directory",
				"config_dir", configDir,
				"file_mode", info.Mode())
			return fmt.Errorf("config path %s exists but is not a directory", configDir)
		}
	}

	err = os.MkdirAll(configDir, 0750) // rwxr-x---
	if err != nil {
		logger.Error("Failed to create config directory",
			"config_dir", configDir,
			"error", err,
			"permissions", "0750")
		return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	logger.Info("Config directory created successfully",
		"config_dir", configDir,
		"permissions", "0750")
	return nil
}

func SaveConfig(cfg Config) error {
	startTime := time.Now()

	configPath, err := DefaultConfigPath()
	if err != nil {
		logger.Error("Failed to get default config path for saving", "error", err)
		return err
	}

	logger.Debug("Saving configuration",
		"config_path", configPath,
		"container_runtime", cfg.ContainerRuntime,
		"local_root", cfg.LocalRoot,
		"ssh_hosts_count", len(cfg.SSHHosts))

	err = EnsureConfigDir()
	if err != nil {
		logger.Error("Failed to ensure config directory exists", "error", err)
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		logger.Error("Failed to marshal config to YAML",
			"error", err,
			"duration", time.Since(startTime))
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	logger.Debug("Configuration marshaled to YAML",
		"yaml_size", len(data),
		"config_path", configPath)

	// Write with permissions rw-r----- (0640)
	err = os.WriteFile(configPath, data, 0640)
	if err != nil {
		logger.Error("Failed to write config file",
			"config_path", configPath,
			"error", err,
			"permissions", "0640",
			"duration", time.Since(startTime))
		return fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}

	logger.Info("Configuration saved successfully",
		"config_path", configPath,
		"yaml_size", len(data),
		"permissions", "0640",
		"duration", time.Since(startTime))

	return nil
}

// GetContainerRuntime returns the configured container runtime, defaulting to "podman"
func GetContainerRuntime() string {
	logger.Debug("Getting container runtime from configuration")

	cfg, err := LoadConfig()
	if err != nil {
		logger.Warn("Failed to load config for runtime check, using default",
			"error", err,
			"default_runtime", "podman")
		// Fallback to podman if config can't be loaded
		return "podman"
	}

	runtime := cfg.ContainerRuntime
	if runtime == "" {
		runtime = "podman"
		logger.Debug("No runtime configured, using default", "runtime", runtime)
	} else {
		logger.Debug("Retrieved configured runtime", "runtime", runtime)
	}

	return runtime
}

func ResolvePath(path string) (string, error) {
	logger.Debug("Resolving path", "input_path", path)

	if !strings.HasPrefix(path, "~/") {
		logger.Debug("Path does not need resolution", "path", path)
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Error("Failed to get user home directory for path resolution",
			"input_path", path,
			"error", err)
		return path, fmt.Errorf("could not get user home directory to resolve path '%s': %w", path, err)
	}

	resolvedPath := filepath.Join(homeDir, path[2:])
	logger.Debug("Path resolved successfully",
		"input_path", path,
		"home_dir", homeDir,
		"resolved_path", resolvedPath)

	return resolvedPath, nil
}
