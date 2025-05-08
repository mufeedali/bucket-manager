// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

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

type Manager struct {
	clients map[string]*ssh.Client
	mu      sync.Mutex
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*ssh.Client),
	}
}

func (m *Manager) GetClient(hostConfig config.SSHHost) (*ssh.Client, error) {
	m.mu.Lock()
	client, found := m.clients[hostConfig.Name]
	if found {
		// Send keepalive to check if cached client is still valid (not foolproof).
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
		User: hostConfig.User,
		Auth: authMethods,
		Timeout: 10 * time.Second,
	}
	// Add proper host key verification
	hostKeyCallback, khErr := createHostKeyCallback()
	if khErr != nil {
		// Log the error but potentially continue if it's just a missing file
		// Allowing connection without verification is risky, but might be acceptable for some tools.
		// We'll log and proceed without strict verification if the file is missing/unparsable.
		// A better approach might involve prompting the user or failing.
		logger.Warnf("Could not create known_hosts callback for %s: %v. Host key will not be verified.", hostConfig.Name, khErr)
		sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey() // Fallback to insecure
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

func (m *Manager) getAuthMethods(hostConfig config.SSHHost) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if hostConfig.KeyPath != "" {
		keyPath, resolveErr := config.ResolvePath(hostConfig.KeyPath)
		if resolveErr != nil {
			logger.Errorf("Warning: could not resolve key path '%s': %v", hostConfig.KeyPath, resolveErr)
			keyPath = hostConfig.KeyPath
		}

		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file %s: %w", keyPath, err)
		}

		// Attempt to parse the private key
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			// Check if the error is due to encryption
			if _, ok := err.(*ssh.PassphraseMissingError); ok {
				// Log a warning that encrypted keys are not yet supported by this method.
				// We won't add this key as an auth method in this case.
				logger.Warnf("Private key file %s is encrypted and passphrase prompting is not yet supported here. Skipping key.", keyPath)
				// Continue to check other auth methods (agent, password)
			} else {
				// Return other parsing errors
				return nil, fmt.Errorf("failed to parse private key file %s: %w", keyPath, err)
			}
		} else {
			// If parsing succeeded (unencrypted key), add it as an auth method
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
func createHostKeyCallback() (ssh.HostKeyCallback, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory for known_hosts: %w", err)
	}
	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")

	// knownhosts.New also handles creating the file if it doesn't exist,
	// but it's better practice to check existence or handle the error specifically.
	// We'll let it try to create/read and handle potential errors.
	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		// Don't treat file not found as a fatal error for the callback creation itself,
		// but log it. The actual connection attempt will fail later if the key is unknown.
		// Other errors (like permission issues) might be more critical.
		if os.IsNotExist(err) {
			logger.Warnf("known_hosts file (%s) not found. Will attempt connection without verification.", knownHostsPath)
			// Return InsecureIgnoreHostKey as a fallback ONLY if file doesn't exist.
			// This allows first-time connections but is less secure.
			// Consider prompting user in a real application.
			return ssh.InsecureIgnoreHostKey(), nil // Return nil error as we handled the specific case
		}
		// For other errors (permissions, format issues), return the error.
		return nil, fmt.Errorf("failed to load or parse known_hosts file %s: %w", knownHostsPath, err)
	}
	return callback, nil
}
