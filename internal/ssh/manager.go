// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ssh

import (
	"bucket-manager/internal/config"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Manager handles persistent SSH connections.
type Manager struct {
	clients map[string]*ssh.Client // Map host name (from config.SSHHost.Name) to active client
	mu      sync.Mutex             // Protects access to the clients map
}

// NewManager creates a new SSH connection manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*ssh.Client),
	}
}

// GetClient returns an active SSH client for the given host configuration.
// It establishes a new connection if one doesn't exist.
func (m *Manager) GetClient(hostConfig config.SSHHost) (*ssh.Client, error) {
	m.mu.Lock()
	client, found := m.clients[hostConfig.Name]
	if found {
		// Basic check: Send keepalive to see if connection is still valid
		// This isn't foolproof but catches many common disconnection scenarios.
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			m.mu.Unlock()
			return client, nil // Return existing, seemingly valid client
		}
		// Connection seems dead, close it and remove from map
		client.Close()
		delete(m.clients, hostConfig.Name)
		// Proceed to create a new connection below
	}
	m.mu.Unlock() // Unlock before potentially long Dial operation

	// --- Establish New Connection ---
	authMethods, err := m.getAuthMethods(hostConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare auth methods for %s: %w", hostConfig.Name, err)
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no suitable authentication method found for %s (key, agent, or password required)", hostConfig.Name)
	}

	sshConfig := &ssh.ClientConfig{
		User:            hostConfig.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification!
		Timeout:         10 * time.Second,            // Connection timeout
	}

	port := hostConfig.Port
	if port == 0 {
		port = 22 // Default SSH port
	}
	addr := fmt.Sprintf("%s:%d", hostConfig.Hostname, port)

	newClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial ssh host %s (%s): %w", hostConfig.Name, addr, err)
	}

	// Store the new client
	m.mu.Lock()
	// Double-check if another goroutine created a client while we were dialing
	existingClient, found := m.clients[hostConfig.Name]
	if found {
		m.mu.Unlock()
		newClient.Close() // Close the redundant new client
		return existingClient, nil
	}
	m.clients[hostConfig.Name] = newClient
	m.mu.Unlock()

	return newClient, nil
}

// getAuthMethods determines the available SSH authentication methods based on config.
func (m *Manager) getAuthMethods(hostConfig config.SSHHost) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 1. Priority: Specific Key File
	if hostConfig.KeyPath != "" {
		keyPath := hostConfig.KeyPath
		// Expand ~ if necessary
		if strings.HasPrefix(keyPath, "~/") {
			homeDir, err := os.UserHomeDir()
			if err == nil { // Ignore error if home dir can't be found
				keyPath = filepath.Join(homeDir, keyPath[2:])
			}
		}

		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file %s: %w", keyPath, err)
		}

		// TODO: Add support for encrypted keys (passphrase prompt or config)
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key file %s: %w", keyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	// 2. Fallback: SSH Agent
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		conn, err := net.Dial("unix", socket)
		if err == nil { // Silently ignore agent errors if key/password might work
			agentClient := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(agentClient.Signers))
			// Note: We don't close the agent connection here, it's managed by the agent client lifecycle
		}
	}

	// 3. Fallback: Password (if provided)
	if hostConfig.Password != "" {
		methods = append(methods, ssh.Password(hostConfig.Password))
	}

	return methods, nil
}

// CloseAll closes all active SSH connections managed by the manager.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, client := range m.clients {
		client.Close()
		delete(m.clients, name) // Remove from map after closing
	}
}

// Close closes a specific connection by host name.
func (m *Manager) Close(hostName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, found := m.clients[hostName]; found {
		client.Close()
		delete(m.clients, hostName)
	}
}
