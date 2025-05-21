// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package ssh provides functionality for establishing and managing SSH connections
// to remote hosts. It handles authentication, connection pooling, and provides
// methods to execute commands on remote systems.
package ssh

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/logger"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Manager handles SSH connections to remote hosts.
// It maintains a pool of connections to avoid repeatedly establishing new connections
// to the same hosts and provides thread-safe access to these connections.
type Manager struct {
	clients map[string]*ssh.Client // Map of host names to active SSH clients
	mu      sync.Mutex             // Mutex to protect concurrent access to clients map
}

// NewManager creates and initializes a new SSH connection manager
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*ssh.Client),
	}
}

// GetClient returns an established SSH client for the specified host configuration.
// It reuses existing connections when possible, and creates new ones when necessary.
// The method includes connection validation and reconnection logic for robustness.
func (m *Manager) GetClient(hostConfig config.SSHHost) (*ssh.Client, error) {
	m.mu.Lock()
	client, found := m.clients[hostConfig.Name]
	if found {
		// Send keepalive to check if cached client is still valid (not foolproof).
		// This helps detect stale connections without a full reconnect attempt
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			m.mu.Unlock()
			return client, nil
		}
		if err := client.Close(); err != nil {
			logger.Errorf("Error closing existing SSH client for %s during reconnect: %v", hostConfig.Name, err)
		}
		delete(m.clients, hostConfig.Name)
	}
	m.mu.Unlock() // Unlock before potentially long Dial operation

	authMethods, err := m.getAuthMethods(hostConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare auth methods for %s: %w", hostConfig.Name, err)
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no suitable authentication method found for %s (key, agent, or password required)", hostConfig.Name)
	}

	sshConfig := &ssh.ClientConfig{
		User:    hostConfig.User,
		Auth:    authMethods,
		Timeout: 10 * time.Second,
	}
	// Add proper host key verification
	hostKeyCallback, khErr := createHostKeyCallback()
	if khErr != nil {
		// Log the error but potentially continue if it's just a missing file.
		// Allowing connection without verification is risky, but might be acceptable for some tools.
		// We'll log and proceed without strict verification if the file is missing/unparsable.
		logger.Warnf("Could not create known_hosts callback for %s: %v. Host key will not be verified.", hostConfig.Name, khErr)
		sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		sshConfig.HostKeyCallback = hostKeyCallback
	}

	port := hostConfig.Port
	if port == 0 {
		port = 22
	}
	addr := fmt.Sprintf("%s:%d", hostConfig.Hostname, port)

	newClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial ssh host %s (%s): %w", hostConfig.Name, addr, err)
	}

	m.mu.Lock()
	// Double-check if another goroutine created a client while we were dialing
	existingClient, found := m.clients[hostConfig.Name]
	if found {
		m.mu.Unlock()
		if err := newClient.Close(); err != nil {
			logger.Errorf("Error closing redundant SSH client for %s: %v", hostConfig.Name, err)
		}
		return existingClient, nil
	}
	m.clients[hostConfig.Name] = newClient
	m.mu.Unlock()

	return newClient, nil
}

// getAuthMethods prepares authentication methods for SSH connection based on the host configuration.
// It tries multiple authentication methods in this order:
// 1. SSH key authentication if KeyPath is provided
// 2. SSH agent authentication if SSH_AUTH_SOCK environment variable is available
// 3. Password authentication if Password is provided in the host config
func (m *Manager) getAuthMethods(hostConfig config.SSHHost) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if hostConfig.KeyPath != "" {
		// Attempt to resolve potential relative paths or ~ expansion
		keyPath, resolveErr := config.ResolvePath(hostConfig.KeyPath)
		if resolveErr != nil {
			logger.Errorf("Warning: could not resolve key path '%s': %v", hostConfig.KeyPath, resolveErr)
			keyPath = hostConfig.KeyPath
		}

		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file %s: %w", keyPath, err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			if _, ok := err.(*ssh.PassphraseMissingError); ok {
				// Log a warning that encrypted keys are not yet supported by this method.
				// We won't add this key as an auth method in this case.
				logger.Warnf("Private key file %s is encrypted and passphrase prompting is not yet supported here. Skipping key.", keyPath)
				// Continue to check other auth methods (agent, password)
			} else {
				return nil, fmt.Errorf("failed to parse private key file %s: %w", keyPath, err)
			}
		} else {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}

	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		conn, err := net.Dial("unix", socket)
		if err == nil { // Silently ignore agent errors if key/password might work
			agentClient := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(agentClient.Signers))
			// Note: We don't close the agent connection here, it's managed by the agent client lifecycle
		}
	}

	if hostConfig.Password != "" {
		methods = append(methods, ssh.Password(hostConfig.Password))
	}

	return methods, nil
}

// CloseAll closes all active SSH connections managed by this Manager.
// This should be called when the application is shutting down or when
// all SSH connections need to be refreshed.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			logger.Errorf("Error closing SSH client for %s: %v", name, err)
		}
		delete(m.clients, name)
	}
}

// Close closes a specific SSH connection by hostname.
// This can be used when a specific connection needs to be refreshed or
// when the connection to a specific host is no longer needed.
func (m *Manager) Close(hostName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, found := m.clients[hostName]; found {
		if err := client.Close(); err != nil {
			logger.Errorf("Error closing SSH client for %s: %v", hostName, err)
		}
		delete(m.clients, hostName)
	}
}

// createHostKeyCallback attempts to load the user's known_hosts file.
// This function creates a host key verification callback based on the standard
// ~/.ssh/known_hosts file for security-conscious SSH connectivity.
// If the known_hosts file doesn't exist, a warning is logged and an insecure
// callback is returned as a fallback (accepting any host key).
func createHostKeyCallback() (ssh.HostKeyCallback, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory for known_hosts: %w", err)
	}
	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")

	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warnf("known_hosts file (%s) not found. Will attempt connection without verification.", knownHostsPath)
			// Return InsecureIgnoreHostKey as a fallback ONLY if file doesn't exist.
			return ssh.InsecureIgnoreHostKey(), nil
		}
		return nil, fmt.Errorf("failed to load or parse known_hosts file %s: %w", knownHostsPath, err)
	}
	return callback, nil
}
