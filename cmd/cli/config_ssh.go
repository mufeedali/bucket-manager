// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package cli's config_ssh.go file implements CLI commands for managing SSH host
// configurations. It provides interactive commands for adding, listing, editing,
// removing, and importing SSH hosts.

package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/logger"
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// sshCmd is the parent command for SSH-specific configuration subcommands
var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Manage SSH host configurations",
	Long: `Add, list, edit, remove, or import SSH host configurations used by bucket-manager.
These configurations are used to connect to remote hosts for stack discovery and management.`,
}

var sshListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured SSH hosts",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		if len(cfg.SSHHosts) == 0 {
			fmt.Println("No SSH hosts configured.")
			return
		}

		statusColor.Println("Configured SSH Hosts:")
		for i, host := range cfg.SSHHosts {
			details := fmt.Sprintf("%s@%s", host.User, host.Hostname)
			if host.Port != 0 && host.Port != 22 {
				details += fmt.Sprintf(":%d", host.Port)
			}
			fmt.Printf("%d: %s (%s)\n", i+1, identifierColor.Sprint(host.Name), details)
			if host.RemoteRoot != "" {
				fmt.Printf("   Remote Root: %s\n", host.RemoteRoot)
			} else {
				fmt.Printf("   Remote Root: %s\n", dimColor.Sprint("[Default: ~/bucket or ~/compose-bucket]"))
			}
			if host.KeyPath != "" {
				fmt.Printf("   Key Path:    %s\n", host.KeyPath)
			}
			if host.Password != "" {
				fmt.Printf("   Password:    %s\n", errorColor.Sprint("[set, stored insecurely]"))
			}
			if host.Disabled {
				fmt.Printf("   Status:      %s\n", errorColor.Sprint("Disabled"))
			}
		}
	},
}

// promptForNewHostDetails handles the interactive prompts for adding a new host.
func promptForNewHostDetails(existingHosts []config.SSHHost) (config.SSHHost, error) {
	var newHost config.SSHHost
	var err error

	newHost.Name, err = promptString("Unique Name (e.g., 'server1', 'kumo-prod'):", true)
	if err != nil {
		return newHost, fmt.Errorf("error reading name: %w", err)
	}
	for _, h := range existingHosts {
		if h.Name == newHost.Name {
			return newHost, fmt.Errorf("SSH host with name '%s' already exists", newHost.Name)
		}
	}

	newHost.Hostname, err = promptString("Hostname or IP Address:", true)
	if err != nil {
		return newHost, fmt.Errorf("error reading hostname: %w", err)
	}

	newHost.User, err = promptString("SSH Username:", true)
	if err != nil {
		return newHost, fmt.Errorf("error reading username: %w", err)
	}

	newHost.Port, err = promptOptionalInt("SSH Port", 22)
	if err != nil {
		return newHost, fmt.Errorf("error reading port: %w", err)
	}
	if newHost.Port == 22 {
		newHost.Port = 0 // Store 0 for default
	}

	prompt := "Remote Root Path (optional, defaults to ~/bucket or ~/compose-bucket):"
	newHost.RemoteRoot, err = promptString(prompt, false)
	if err != nil {
		return newHost, fmt.Errorf("error reading remote root: %w", err)
	}

	err = promptForAuthDetails(&newHost, false, "")
	if err != nil {
		return newHost, fmt.Errorf("error getting authentication details: %w", err)
	}

	newHost.Disabled = false
	return newHost, nil
}

var sshAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new SSH host configuration interactively",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		fmt.Println("Adding a new SSH host configuration...")

		newHost, err := promptForNewHostDetails(cfg.SSHHosts)
		if err != nil {
			logger.Errorf("Failed to get host details: %v", err)
			os.Exit(1)
		}

		cfg.SSHHosts = append(cfg.SSHHosts, newHost)
		err = config.SaveConfig(cfg)
		if err != nil {
			logger.Errorf("Error saving configuration: %v", err)
			os.Exit(1)
		}

		successColor.Printf("Successfully added SSH host '%s'.\n", newHost.Name)
	},
}

// promptForEditedHostDetails handles the interactive prompts for editing an existing host.
func promptForEditedHostDetails(originalHost config.SSHHost, allHosts []config.SSHHost, hostIndex int) (config.SSHHost, error) {
	editedHost := originalHost // Start with a copy
	var err error

	fmt.Printf("\nEditing SSH host '%s'. Press Enter to keep the current value.\n", identifierColor.Sprint(originalHost.Name))

	editedHost.Name, err = promptString(fmt.Sprintf("Unique Name [%s]:", originalHost.Name), false)
	if err != nil {
		return editedHost, fmt.Errorf("error reading name: %w", err)
	}
	if editedHost.Name == "" {
		editedHost.Name = originalHost.Name
	} else if editedHost.Name != originalHost.Name {
		for i, h := range allHosts {
			if i != hostIndex && h.Name == editedHost.Name {
				return editedHost, fmt.Errorf("SSH host with name '%s' already exists", editedHost.Name)
			}
		}
	}

	editedHost.Hostname, err = promptString(fmt.Sprintf("Hostname or IP Address [%s]:", originalHost.Hostname), false)
	if err != nil {
		return editedHost, fmt.Errorf("error reading hostname: %w", err)
	}
	if editedHost.Hostname == "" {
		editedHost.Hostname = originalHost.Hostname
	}

	editedHost.User, err = promptString(fmt.Sprintf("SSH Username [%s]:", originalHost.User), false)
	if err != nil {
		return editedHost, fmt.Errorf("error reading username: %w", err)
	}
	if editedHost.User == "" {
		editedHost.User = originalHost.User
	}

	portDefault := originalHost.Port
	if portDefault == 0 {
		portDefault = 22
	}
	editedHost.Port, err = promptOptionalInt(fmt.Sprintf("SSH Port [%d]", portDefault), portDefault)
	if err != nil {
		return editedHost, fmt.Errorf("error reading port: %w", err)
	}
	if editedHost.Port == 22 {
		editedHost.Port = 0 // Store 0 if it's the default 22
	}

	remoteRootPrompt := "Remote Root Path (leave blank to use default: ~/bucket or ~/compose-bucket)"
	currentRemoteRootDisplay := originalHost.RemoteRoot
	if currentRemoteRootDisplay == "" {
		currentRemoteRootDisplay = dimColor.Sprint("[Default]")
	}
	editedHost.RemoteRoot, err = promptString(fmt.Sprintf("%s [%s]:", remoteRootPrompt, currentRemoteRootDisplay), false)
	if err != nil {
		return editedHost, fmt.Errorf("error reading remote root: %w", err)
	}
	// Note: promptString returns trimmed space, so empty input becomes "" which is desired for clearing RemoteRoot

	err = promptForAuthDetails(&editedHost, true, originalHost.Password)
	if err != nil {
		return editedHost, fmt.Errorf("error getting authentication details: %w", err)
	}

	disablePrompt := fmt.Sprintf("Disable this host? (Currently: %t) (y/N):", originalHost.Disabled)
	disableChoice, err := promptConfirm(disablePrompt)
	if err != nil {
		return editedHost, fmt.Errorf("error reading disable choice: %w", err)
	}
	editedHost.Disabled = disableChoice

	return editedHost, nil
}

var sshEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit an existing SSH host configuration interactively",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		if len(cfg.SSHHosts) == 0 {
			fmt.Println("No SSH hosts configured to edit.")
			return
		}

		fmt.Println("Select the SSH host to edit:")
		for i, host := range cfg.SSHHosts {
			fmt.Printf("  %d: %s\n", i+1, identifierColor.Sprint(host.Name))
		}
		choiceStr, err := promptString("Enter the number of the host to edit:", true)
		if err != nil {
			logger.Errorf("Error reading selection: %v", err)
			os.Exit(1)
		}
		choice, err := strconv.Atoi(choiceStr)
		if err != nil || choice < 1 || choice > len(cfg.SSHHosts) {
			logger.Errorf("Invalid selection '%s'.", choiceStr)
			os.Exit(1)
		}
		hostIndex := choice - 1
		originalHost := cfg.SSHHosts[hostIndex]

		editedHost, err := promptForEditedHostDetails(originalHost, cfg.SSHHosts, hostIndex)
		if err != nil {
			logger.Errorf("Failed to get updated host details: %v", err)
			os.Exit(1)
		}

		cfg.SSHHosts[hostIndex] = editedHost
		err = config.SaveConfig(cfg)
		if err != nil {
			logger.Errorf("Error saving configuration: %v", err)
			os.Exit(1)
		}

		successColor.Printf("Successfully updated SSH host '%s'.\n", editedHost.Name)
	},
}

var sshRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove an SSH host configuration interactively",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading configuration: %v", err)
			os.Exit(1)
		}

		if len(cfg.SSHHosts) == 0 {
			fmt.Println("No SSH hosts configured to remove.")
			return
		}

		fmt.Println("Select the SSH host to remove:")
		for i, host := range cfg.SSHHosts {
			fmt.Printf("  %d: %s\n", i+1, identifierColor.Sprint(host.Name))
		}

		choiceStr, err := promptString("Enter the number of the host to remove:", true)
		if err != nil {
			logger.Errorf("Error reading selection: %v", err)
			os.Exit(1)
		}

		choice, err := strconv.Atoi(choiceStr)
		if err != nil || choice < 1 || choice > len(cfg.SSHHosts) {
			logger.Errorf("Invalid selection '%s'.", choiceStr)
			os.Exit(1)
		}

		hostToRemove := cfg.SSHHosts[choice-1]

		confirmed, err := promptConfirm(fmt.Sprintf("Are you sure you want to remove host '%s'?", hostToRemove.Name))
		if err != nil {
			logger.Errorf("Error reading confirmation: %v", err)
			os.Exit(1)
		}

		if !confirmed {
			fmt.Println("Removal cancelled.")
			return
		}

		cfg.SSHHosts = append(cfg.SSHHosts[:choice-1], cfg.SSHHosts[choice:]...)

		err = config.SaveConfig(cfg)
		if err != nil {
			logger.Errorf("Error saving configuration: %v", err)
			os.Exit(1)
		}

		successColor.Printf("Successfully removed SSH host '%s'.\n", hostToRemove.Name)
	},
}

// filterAndDisplayPotentialHosts filters hosts from ssh_config against existing bm config and displays them.
// Returns the list of hosts that are actually importable.
func filterAndDisplayPotentialHosts(potentialHosts []config.PotentialHost, currentConfigHosts []config.SSHHost) []config.PotentialHost {
	fmt.Println("Found potential hosts in ~/.ssh/config:")
	importableHosts := []config.PotentialHost{}
	currentConfigNames := make(map[string]bool)
	for _, h := range currentConfigHosts {
		currentConfigNames[h.Name] = true
	}

	for i, pHost := range potentialHosts {
		if _, exists := currentConfigNames[pHost.Alias]; exists {
			fmt.Printf("  %d: %s (Alias: %s) - %s\n", i+1, identifierColor.Sprint(pHost.Alias), pHost.Hostname, errorColor.Sprint("[Skipped: Name already exists in bm config]"))
			continue
		}
		fmt.Printf("  %d: %s (Hostname: %s, User: %s, Port: %d)\n", i+1, identifierColor.Sprint(pHost.Alias), pHost.Hostname, pHost.User, pHost.Port)
		if pHost.KeyPath != "" {
			fmt.Printf("     Key: %s\n", pHost.KeyPath)
		}
		importableHosts = append(importableHosts, pHost)
	}
	return importableHosts
}

// promptForImportSelection prompts the user to select hosts from the importable list.
// potentialHosts is the original list used for index validation.
func promptForImportSelection(potentialHosts, importableHosts []config.PotentialHost) ([]config.PotentialHost, error) {
	if len(importableHosts) == 0 {
		fmt.Println("\nNo new hosts available to import.")
		return nil, nil
	}

	fmt.Println("\nEnter the numbers of the hosts you want to import (comma-separated), or 'all':")
	choiceStr, err := promptString("Import selection:", true)
	if err != nil {
		return nil, fmt.Errorf("error reading selection: %w", err)
	}

	hostsToImport := []config.PotentialHost{}
	if strings.ToLower(choiceStr) == "all" {
		hostsToImport = importableHosts
	} else {
		indices := strings.Split(choiceStr, ",")
		selectedAliases := make(map[string]bool) // Track selected aliases to avoid duplicates from input

		for _, indexStr := range indices {
			index, err := strconv.Atoi(strings.TrimSpace(indexStr))
			if err != nil || index < 1 || index > len(potentialHosts) {
				return nil, fmt.Errorf("invalid selection '%s'. Please enter numbers corresponding to the list", indexStr)
			}

			selectedPotentialHost := potentialHosts[index-1]
			foundInImportable := false
			for _, ih := range importableHosts {
				if ih.Alias == selectedPotentialHost.Alias {
					if !selectedAliases[ih.Alias] {
						hostsToImport = append(hostsToImport, ih)
						selectedAliases[ih.Alias] = true
					}
					foundInImportable = true
					break
				}
			}
			if !foundInImportable {
				return nil, fmt.Errorf("host '%s' (number %d) cannot be imported (e.g., name conflict)", selectedPotentialHost.Alias, index)
			}
		}
	}

	if len(hostsToImport) == 0 {
		fmt.Println("No hosts selected for import.")
		return nil, nil
	}
	return hostsToImport, nil
}

// configureAndConvertImportedHost prompts for additional details and converts a PotentialHost.
func configureAndConvertImportedHost(pHost config.PotentialHost, currentConfigNames map[string]bool) (*config.SSHHost, error) {
	fmt.Printf("\nConfiguring import for host '%s' (Alias: %s)...\n", identifierColor.Sprint(pHost.Alias), pHost.Alias)

	bmName := pHost.Alias
	if _, exists := currentConfigNames[bmName]; exists {
		return nil, fmt.Errorf("name '%s' conflicts with an existing host", bmName)
	}

	remoteRootPrompt := "Remote Root Path (optional, defaults to ~/bucket or ~/compose-bucket):"
	remoteRoot, err := promptString(remoteRootPrompt, false)
	if err != nil {
		return nil, fmt.Errorf("error reading remote root: %w", err)
	}

	bmHost, err := config.ConvertToBucketManagerHost(pHost, bmName, remoteRoot)
	if err != nil {
		return nil, fmt.Errorf("error converting host: %w", err)
	}

	if bmHost.KeyPath == "" {
		fmt.Printf("Host '%s' imported from ssh_config has no IdentityFile specified.\n", bmName)
		err = promptForAuthDetails(&bmHost, false, "")
		if err != nil {
			return nil, fmt.Errorf("error getting authentication details: %w", err)
		}
	}

	return &bmHost, nil
}

var sshImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import hosts from ~/.ssh/config interactively",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Errorf("Error loading current configuration: %v", err)
			os.Exit(1)
		}

		potentialHosts, err := config.ParseSSHConfig()
		if err != nil {
			logger.Errorf("Error parsing ~/.ssh/config: %v", err)
			os.Exit(1)
		}
		if len(potentialHosts) == 0 {
			fmt.Println("No suitable hosts found in ~/.ssh/config to import.")
			return
		}

		importableHosts := filterAndDisplayPotentialHosts(potentialHosts, cfg.SSHHosts)

		hostsToConfigure, err := promptForImportSelection(potentialHosts, importableHosts)
		if err != nil {
			logger.Errorf("Import selection failed: %v", err)
			os.Exit(1)
		}
		if len(hostsToConfigure) == 0 {
			return
		}

		fmt.Println("\nFor each selected host, please provide any required details:")
		successfullyConfiguredHosts := []config.SSHHost{}
		currentConfigNames := make(map[string]bool) // Rebuild map for checks during configuration loop
		for _, h := range cfg.SSHHosts {
			currentConfigNames[h.Name] = true
		}

		for _, pHost := range hostsToConfigure {
			bmHostPtr, configErr := configureAndConvertImportedHost(pHost, currentConfigNames)
			if configErr != nil {
				logger.Errorf("Skipping import for '%s': %v", pHost.Alias, configErr)
				continue
			}
			if bmHostPtr != nil {
				successfullyConfiguredHosts = append(successfullyConfiguredHosts, *bmHostPtr)
				currentConfigNames[bmHostPtr.Name] = true // Add name to map to prevent duplicates within this import run
				successColor.Printf("Prepared '%s' for import.\n", bmHostPtr.Name)
			}
		}

		if len(successfullyConfiguredHosts) == 0 {
			fmt.Println("\nNo hosts were successfully configured for import.")
			return
		}

		cfg.SSHHosts = append(cfg.SSHHosts, successfullyConfiguredHosts...)
		err = config.SaveConfig(cfg)
		if err != nil {
			logger.Errorf("\nError saving configuration: %v", err)
			os.Exit(1)
		}

		successColor.Printf("\nSuccessfully imported %d SSH host(s).\n", len(successfullyConfiguredHosts))
	},
}

func init() {
	sshCmd.AddCommand(sshListCmd)
	sshCmd.AddCommand(sshAddCmd)
	sshCmd.AddCommand(sshEditCmd)
	sshCmd.AddCommand(sshRemoveCmd)
	sshCmd.AddCommand(sshImportCmd)

	configCmd.AddCommand(sshCmd)
}

var reader = bufio.NewReader(os.Stdin)

func promptString(prompt string, required bool) (string, error) {
	fmt.Print(prompt + " ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if required && input == "" {
		return "", fmt.Errorf("input is required")
	}
	return input, nil
}

func promptOptionalInt(prompt string, defaultValue int) (int, error) {
	fmt.Printf("%s (default: %d): ", prompt, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue, err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	val, err := strconv.Atoi(input)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid integer input: %w", err)
	}
	return val, nil
}

func promptConfirm(prompt string) (bool, error) {
	fmt.Print(prompt + " (y/N): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes", nil
}

// chooseAuthMethod prompts the user to select an authentication method.
func chooseAuthMethod(currentMethod int, isEditing bool) (int, error) {
	fmt.Println("\nAuthentication Method:")
	if isEditing {
		fmt.Print("Current: ")
		switch currentMethod {
		case 1:
			fmt.Printf("SSH Key File\n")
		case 2:
			fmt.Println("SSH Agent")
		case 3:
			fmt.Println("Password (insecure)")
		}
		fmt.Println("Change Authentication Method?")
	}
	fmt.Println("  1. SSH Key File")
	fmt.Println("  2. SSH Agent (requires agent running with keys loaded)")
	fmt.Println("  3. Password (stored insecurely in config)")

	promptMsg := "Choose auth method [1, 2, 3]"
	defaultChoiceStr := strconv.Itoa(currentMethod)
	if isEditing {
		promptMsg += fmt.Sprintf(" (leave blank to keep current - %s):", defaultChoiceStr)
	} else {
		promptMsg += fmt.Sprintf(" (default %s):", defaultChoiceStr)
	}

	authChoiceStr, err := promptString(promptMsg, false)
	if err != nil {
		return currentMethod, fmt.Errorf("error reading auth choice: %w", err)
	}

	newAuthChoice := currentMethod // Default to keeping current or the initial default
	if authChoiceStr != "" {
		choice, err := strconv.Atoi(authChoiceStr)
		if err != nil || choice < 1 || choice > 3 {
			fmt.Fprintf(os.Stderr, "Invalid choice '%s', using default/current method (%d).\n", authChoiceStr, currentMethod)
		} else {
			newAuthChoice = choice
		}
	}
	return newAuthChoice, nil
}

// promptForKeyFile prompts for the SSH private key file path.
func promptForKeyFile(currentPath string, isEditing bool) (string, error) {
	prompt := "Path to Private Key File"
	required := true

	if isEditing && currentPath != "" {
		prompt += fmt.Sprintf(" [%s]", currentPath)
		required = false
	} else {
		prompt += ":"
	}

	keyPath, err := promptString(prompt, required)
	if err != nil {
		return "", fmt.Errorf("error reading key path: %w", err)
	}

	if keyPath == "" && isEditing {
		keyPath = currentPath
	}

	if keyPath == "" {
		return "", fmt.Errorf("key path cannot be empty when Key File method is selected")
	}
	return keyPath, nil
}

// promptForPassword prompts for the SSH password.
func promptForPassword(currentPassword string, isEditing bool) (string, error) {
	fmt.Println(errorColor.Sprint("Warning: Password will be stored in plaintext in the config file!"))
	prompt := "SSH Password"
	required := true

	if isEditing && currentPassword != "" {
		prompt += " (leave blank to keep current):"
		required = false
	} else {
		prompt += ":"
	}

	password, err := promptString(prompt, required)
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}

	if password == "" && isEditing {
		password = currentPassword
	}

	if password == "" {
		return "", fmt.Errorf("password cannot be empty when Password method is selected")
	}
	return password, nil
}

// promptForAuthDetails handles the interactive prompting for SSH authentication details.
// It modifies the passed host struct directly.
// originalPassword is only relevant when isEditing is true and current method is password.
func promptForAuthDetails(host *config.SSHHost, isEditing bool, originalPassword string) error {
	currentAuthMethod := 2
	if host.KeyPath != "" {
		currentAuthMethod = 1
	} else if host.Password != "" {
		currentAuthMethod = 3
	}

	newAuthChoice, err := chooseAuthMethod(currentAuthMethod, isEditing)
	if err != nil {
		return err
	}

	methodChanged := newAuthChoice != currentAuthMethod
	if methodChanged {
		host.KeyPath = ""
		host.Password = ""
	}

	switch newAuthChoice {
	case 1:
		// Determine current key path for prompt, considering if method changed
		currentKeyForPrompt := host.KeyPath
		if !methodChanged && isEditing { // If editing and method is still key, use current host's key path
			currentKeyForPrompt = host.KeyPath
		}
		keyPath, keyErr := promptForKeyFile(currentKeyForPrompt, isEditing)
		if keyErr != nil {
			return keyErr
		}
		host.KeyPath = keyPath
		host.Password = ""
	case 3:
		// Determine current password for prompt (only if editing and method is still password)
		currentPassForPrompt := ""
		if !methodChanged && isEditing {
			currentPassForPrompt = originalPassword
		}
		password, passErr := promptForPassword(currentPassForPrompt, isEditing)
		if passErr != nil {
			return passErr
		}
		host.Password = password
		host.KeyPath = ""
	case 2:
		host.KeyPath = ""
		host.Password = ""
	}
	return nil
}
