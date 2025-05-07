// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import (
	"bucket-manager/internal/config"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

// --- Form Creation ---

func createAddForm() []textinput.Model {
	inputs := make([]textinput.Model, 7)
	var t textinput.Model

	t = textinput.New()
	t.Placeholder = "Unique Name (e.g., server1)"
	t.Focus() // Initial focus
	t.CharLimit = 50
	t.Width = 40
	inputs[0] = t

	t = textinput.New()
	t.Placeholder = "Hostname or IP Address"
	t.CharLimit = 100
	t.Width = 40
	inputs[1] = t

	t = textinput.New()
	t.Placeholder = "SSH Username"
	t.CharLimit = 50
	t.Width = 40
	inputs[2] = t

	t = textinput.New()
	t.Placeholder = "Port (default 22)"
	t.CharLimit = 5
	t.Width = 20
	t.Validate = func(s string) error {
		if s == "" {
			return nil // Allow empty for default port
		}
		_, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("port must be a number")
		}
		return nil
	}
	inputs[3] = t

	t = textinput.New()
	t.Placeholder = "Remote Root Path (optional, defaults: ~/bucket or ~/compose-bucket)"
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	t = textinput.New()
	t.Placeholder = "Path to Private Key (e.g., ~/.ssh/id_rsa)"
	t.CharLimit = 200
	t.Width = 60
	inputs[5] = t

	t = textinput.New()
	t.Placeholder = "Password (stored insecurely!)"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	return inputs
}

func createEditForm(host config.SSHHost) ([]textinput.Model, int, bool) {
	inputs := make([]textinput.Model, 7) // Same fields as add form
	var t textinput.Model
	initialAuthMethod := authMethodAgent // Default to agent if no specific method found
	if host.KeyPath != "" {
		initialAuthMethod = authMethodKey
	} else if host.Password != "" {
		initialAuthMethod = authMethodPassword
	}

	t = textinput.New()
	t.Placeholder = "Unique Name"
	t.SetValue(host.Name)
	t.Focus() // Initial focus
	t.CharLimit = 50
	t.Width = 40
	inputs[0] = t

	t = textinput.New()
	t.Placeholder = "Hostname or IP Address"
	t.SetValue(host.Hostname)
	t.CharLimit = 100
	t.Width = 40
	inputs[1] = t

	t = textinput.New()
	t.Placeholder = "SSH Username"
	t.SetValue(host.User)
	t.CharLimit = 50
	t.Width = 40
	inputs[2] = t

	t = textinput.New()
	t.Placeholder = "Port (default 22)"
	portStr := ""
	if host.Port != 0 { // Only set value if port is non-default
		portStr = strconv.Itoa(host.Port)
	}
	t.SetValue(portStr)
	t.CharLimit = 5
	t.Width = 20
	t.Validate = func(s string) error {
		if s == "" {
			return nil // Allow empty for default port
		}
		_, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("port must be a number")
		}
		return nil
	}
	inputs[3] = t

	t = textinput.New()
	t.Placeholder = "Remote Root Path (leave blank for default)"
	t.SetValue(host.RemoteRoot)
	t.CharLimit = 200
	t.Width = 60
	inputs[4] = t

	t = textinput.New()
	t.Placeholder = "Path to Private Key"
	t.SetValue(host.KeyPath)
	t.CharLimit = 200
	t.Width = 60
	inputs[5] = t

	t = textinput.New()
	t.Placeholder = "Password (leave blank to keep current)" // Don't show existing password
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	return inputs, initialAuthMethod, host.Disabled
}

func createImportDetailsForm(pHost config.PotentialHost) ([]textinput.Model, int) {
	// We still create the full 7 inputs for consistency in indexing,
	// but only populate/use RemoteRoot, KeyPath, and Password.
	inputs := make([]textinput.Model, 7)
	var t textinput.Model
	initialAuthMethod := authMethodAgent // Default if no key found

	t = textinput.New()
	t.Placeholder = "Remote Root Path (optional, defaults: ~/bucket or ~/compose-bucket)"
	t.Focus() // Initial focus for this form
	t.CharLimit = 200
	t.Width = 70
	inputs[4] = t

	t = textinput.New()
	t.Placeholder = "Path to Private Key"
	t.CharLimit = 200
	t.Width = 60
	if pHost.KeyPath != "" {
		t.SetValue(pHost.KeyPath)
		initialAuthMethod = authMethodKey // Set initial method if key exists
	}
	inputs[5] = t

	t = textinput.New()
	t.Placeholder = "Password (stored insecurely!)"
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '*'
	t.CharLimit = 100
	t.Width = 40
	inputs[6] = t

	// Add placeholders for unused fields (0-3) to maintain array size consistency
	for i := 0; i < 4; i++ {
		inputs[i] = textinput.New() // Create empty inputs
	}

	return inputs, initialAuthMethod
}

// --- Form Processing ---

// buildHostFromForm creates a config.SSHHost from the add form inputs.
// It performs basic validation.
// NOTE: This is defined as a method on the model because it needs access to
// m.formInputs and m.formAuthMethod.
func (m *model) buildHostFromForm() (config.SSHHost, error) {
	host := config.SSHHost{}

	host.Name = strings.TrimSpace(m.formInputs[0].Value())
	if host.Name == "" {
		return host, fmt.Errorf("name is required")
	}
	host.Hostname = strings.TrimSpace(m.formInputs[1].Value())
	if host.Hostname == "" {
		return host, fmt.Errorf("hostname is required")
	}
	host.User = strings.TrimSpace(m.formInputs[2].Value())
	if host.User == "" {
		return host, fmt.Errorf("user is required")
	}
	host.RemoteRoot = strings.TrimSpace(m.formInputs[4].Value())

	// Validate and parse Port
	portStr := strings.TrimSpace(m.formInputs[3].Value())
	if portStr == "" {
		host.Port = 0 // Use default port (handled by SSH client)
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return host, fmt.Errorf("invalid port number: %s", portStr)
		}
		if port == 22 {
			host.Port = 0 // Store 0 for default port 22
		} else {
			host.Port = port
		}
	}

	// Set auth method fields based on selection
	switch m.formAuthMethod {
	case authMethodKey:
		host.KeyPath = strings.TrimSpace(m.formInputs[5].Value())
		if host.KeyPath == "" {
			return host, fmt.Errorf("key path is required for Key File authentication")
		}
		host.Password = "" // Ensure password is empty
	case authMethodPassword:
		host.Password = m.formInputs[6].Value()
		if host.Password == "" {
			return host, fmt.Errorf("password is required for Password authentication")
		}
		host.KeyPath = "" // Ensure key path is empty
	case authMethodAgent:
		host.KeyPath = ""
		host.Password = ""
	default:
		return host, fmt.Errorf("invalid authentication method selected")
	}

	host.Disabled = false // New hosts are enabled by default

	// Note: Name conflict check is performed later in saveNewSshHostCmd
	return host, nil
}

// buildHostFromEditForm creates a config.SSHHost from the edit form inputs.
// It performs basic validation and handles keeping original values if inputs are left blank.
// NOTE: This is defined as a method on the model because it needs access to
// m.hostToEdit, m.formInputs, m.formAuthMethod, and m.formDisabled.
func (m *model) buildHostFromEditForm() (config.SSHHost, error) {
	if m.hostToEdit == nil {
		return config.SSHHost{}, fmt.Errorf("internal error: hostToEdit is nil")
	}
	originalHost := *m.hostToEdit
	editedHost := config.SSHHost{}

	// Get values, keeping original if the field is left empty (except for RemoteRoot and auth fields)
	editedHost.Name = strings.TrimSpace(m.formInputs[0].Value())
	if editedHost.Name == "" {
		editedHost.Name = originalHost.Name // Keep original if empty
	}

	editedHost.Hostname = strings.TrimSpace(m.formInputs[1].Value())
	if editedHost.Hostname == "" {
		editedHost.Hostname = originalHost.Hostname // Keep original if empty
	}

	editedHost.User = strings.TrimSpace(m.formInputs[2].Value())
	if editedHost.User == "" {
		editedHost.User = originalHost.User // Keep original if empty
	}

	// Allow empty RemoteRoot to clear it
	editedHost.RemoteRoot = strings.TrimSpace(m.formInputs[4].Value())

	// Basic validation for required fields after potentially keeping originals
	if editedHost.Name == "" {
		return editedHost, fmt.Errorf("name cannot be empty")
	}
	if editedHost.Hostname == "" {
		return editedHost, fmt.Errorf("hostname cannot be empty")
	}
	if editedHost.User == "" {
		return editedHost, fmt.Errorf("user cannot be empty")
	}

	// Validate and parse Port
	portStr := strings.TrimSpace(m.formInputs[3].Value())
	if portStr == "" {
		editedHost.Port = 0 // Default port
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return editedHost, fmt.Errorf("invalid port number: %s", portStr)
		}
		if port == 22 {
			editedHost.Port = 0 // Store 0 for default port 22
		} else {
			editedHost.Port = port
		}
	}

	// Handle auth fields based on selected method and inputs
	keyPathInput := strings.TrimSpace(m.formInputs[5].Value())
	passwordInput := m.formInputs[6].Value() // Don't trim password

	switch m.formAuthMethod {
	case authMethodKey:
		if keyPathInput == "" {
			// If input is empty, keep original key path ONLY if the original method was also key-based
			if originalHost.KeyPath != "" && originalHost.Password == "" {
				editedHost.KeyPath = originalHost.KeyPath
			} else {
				// If original wasn't key-based or had no key, require a new path
				return editedHost, fmt.Errorf("key path is required for Key File authentication")
			}
		} else {
			editedHost.KeyPath = keyPathInput
		}
		editedHost.Password = "" // Clear password if key is set/kept
	case authMethodPassword:
		if passwordInput == "" {
			// If input is empty, keep original password ONLY if the original method was also password-based
			if originalHost.Password != "" && originalHost.KeyPath == "" {
				editedHost.Password = originalHost.Password
			} else {
				// If original wasn't password-based or had no password, require a new one
				return editedHost, fmt.Errorf("password is required for Password authentication (leave blank only to keep existing)")
			}
		} else {
			editedHost.Password = passwordInput
		}
		editedHost.KeyPath = "" // Clear key path if password is set/kept
	case authMethodAgent:
		// Agent auth selected, clear both specific fields
		editedHost.KeyPath = ""
		editedHost.Password = ""
	default:
		return editedHost, fmt.Errorf("invalid authentication method selected")
	}

	// Apply the disabled status from the form
	editedHost.Disabled = m.formDisabled

	// Note: Name conflict check (if name changed) is performed later in saveEditedSshHostCmd
	return editedHost, nil
}
