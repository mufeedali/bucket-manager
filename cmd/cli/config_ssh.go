// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/config"
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var dimColor = color.New(color.Faint)

// configCmd represents the base command for configuration management.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage bucket-manager configuration",
	Long:  `Provides subcommands to manage different aspects of the bucket-manager configuration.`,
	// PersistentPreRun ensures config dir exists, already handled by rootCmd
}

// sshCmd represents the command for managing SSH host configurations.
var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Manage SSH host configurations",
	Long:  `Add, list, edit, remove, or import SSH host configurations used by bucket-manager.`,
}

var sshListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured SSH hosts",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
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

var sshAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new SSH host configuration interactively",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Adding a new SSH host configuration...")

		var newHost config.SSHHost
		var inputErr error

		newHost.Name, inputErr = promptString("Unique Name (e.g., 'server1', 'kumo-prod'):", true)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading name: %v\n", inputErr)
			os.Exit(1)
		}
		for _, h := range cfg.SSHHosts {
			if h.Name == newHost.Name {
				errorColor.Fprintf(os.Stderr, "Error: SSH host with name '%s' already exists.\n", newHost.Name)
				os.Exit(1)
			}
		}

		newHost.Hostname, inputErr = promptString("Hostname or IP Address:", true)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading hostname: %v\n", inputErr)
			os.Exit(1)
		}

		newHost.User, inputErr = promptString("SSH Username:", true)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading username: %v\n", inputErr)
			os.Exit(1)
		}

		newHost.Port, inputErr = promptOptionalInt("SSH Port", 22)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading port: %v\n", inputErr)
			os.Exit(1)
		}

		// RemoteRoot is now optional
		prompt := "Remote Root Path (optional, defaults to ~/bucket or ~/compose-bucket):"
		newHost.RemoteRoot, inputErr = promptString(prompt, false)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading remote root: %v\n", inputErr)
			os.Exit(1)
		}

		inputErr = promptForAuthDetails(&newHost, false, "") // false for isEditing, empty original password
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error getting authentication details: %v\n", inputErr)
			os.Exit(1)
		}

		cfg.SSHHosts = append(cfg.SSHHosts, newHost)
		err = config.SaveConfig(cfg)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		successColor.Printf("Successfully added SSH host '%s'.\n", newHost.Name)
	},
}

var sshEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit an existing SSH host configuration interactively",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
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
			errorColor.Fprintf(os.Stderr, "Error reading selection: %v\n", err)
			os.Exit(1)
		}

		choice, err := strconv.Atoi(choiceStr)
		if err != nil || choice < 1 || choice > len(cfg.SSHHosts) {
			errorColor.Fprintf(os.Stderr, "Invalid selection '%s'.\n", choiceStr)
			os.Exit(1)
		}

		hostIndex := choice - 1
		originalHost := cfg.SSHHosts[hostIndex] // Keep original for comparison/defaults
		editedHost := originalHost              // Copy to modify

		fmt.Printf("\nEditing SSH host '%s'. Press Enter to keep the current value.\n", identifierColor.Sprint(originalHost.Name))

		var inputErr error

		editedHost.Name, inputErr = promptString(fmt.Sprintf("Unique Name [%s]:", originalHost.Name), false) // Name is required, but prompt shows default
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading name: %v\n", inputErr)
			os.Exit(1)
		}
		if editedHost.Name == "" { // If user just pressed Enter
			editedHost.Name = originalHost.Name
		} else if editedHost.Name != originalHost.Name {
			// Check if the new name conflicts with *other* existing hosts
			for i, h := range cfg.SSHHosts {
				if i != hostIndex && h.Name == editedHost.Name {
					errorColor.Fprintf(os.Stderr, "Error: SSH host with name '%s' already exists.\n", editedHost.Name)
					os.Exit(1)
				}
			}
		}

		editedHost.Hostname, inputErr = promptString(fmt.Sprintf("Hostname or IP Address [%s]:", originalHost.Hostname), false)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading hostname: %v\n", inputErr)
			os.Exit(1)
		}
		if editedHost.Hostname == "" {
			editedHost.Hostname = originalHost.Hostname
		}

		editedHost.User, inputErr = promptString(fmt.Sprintf("SSH Username [%s]:", originalHost.User), false)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading username: %v\n", inputErr)
			os.Exit(1)
		}
		if editedHost.User == "" {
			editedHost.User = originalHost.User
		}

		// Handle port default display correctly
		portDefault := originalHost.Port
		if portDefault == 0 {
			portDefault = 22
		} // Show 22 if original was 0/unset
		editedHost.Port, inputErr = promptOptionalInt(fmt.Sprintf("SSH Port [%d]", portDefault), portDefault)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading port: %v\n", inputErr)
			os.Exit(1)
		}
		if editedHost.Port == 22 {
			editedHost.Port = 0
		} // Store 0 if it's the default 22 for cleaner yaml

		// RemoteRoot is optional, allow clearing it
		remoteRootPrompt := "Remote Root Path (leave blank to use default: ~/bucket or ~/compose-bucket)"
		currentRemoteRootDisplay := originalHost.RemoteRoot
		if currentRemoteRootDisplay == "" {
			currentRemoteRootDisplay = dimColor.Sprint("[Default]")
		}
		editedHost.RemoteRoot, inputErr = promptString(fmt.Sprintf("%s [%s]:", remoteRootPrompt, currentRemoteRootDisplay), false)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading remote root: %v\n", inputErr)
			os.Exit(1)
		}
		// No need to restore original if blank, blank means use default

		// Use the shared helper function, passing true for isEditing and the original password
		inputErr = promptForAuthDetails(&editedHost, true, originalHost.Password)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error getting authentication details: %v\n", inputErr)
			os.Exit(1)
		}

		// Prompt for Disabled status
		disablePrompt := fmt.Sprintf("Disable this host? (Currently: %t) (y/N):", originalHost.Disabled)
		disableChoice, inputErr := promptConfirm(disablePrompt)
		if inputErr != nil {
			errorColor.Fprintf(os.Stderr, "Error reading disable choice: %v\n", inputErr)
			os.Exit(1)
		}
		editedHost.Disabled = disableChoice

		cfg.SSHHosts[hostIndex] = editedHost
		err = config.SaveConfig(cfg)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
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
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
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
			errorColor.Fprintf(os.Stderr, "Error reading selection: %v\n", err)
			os.Exit(1)
		}

		choice, err := strconv.Atoi(choiceStr)
		if err != nil || choice < 1 || choice > len(cfg.SSHHosts) {
			errorColor.Fprintf(os.Stderr, "Invalid selection '%s'.\n", choiceStr)
			os.Exit(1)
		}

		hostToRemove := cfg.SSHHosts[choice-1]

		confirmed, err := promptConfirm(fmt.Sprintf("Are you sure you want to remove host '%s'?", hostToRemove.Name))
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error reading confirmation: %v\n", err)
			os.Exit(1)
		}

		if !confirmed {
			fmt.Println("Removal cancelled.")
			return
		}

		cfg.SSHHosts = append(cfg.SSHHosts[:choice-1], cfg.SSHHosts[choice:]...)

		err = config.SaveConfig(cfg)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		successColor.Printf("Successfully removed SSH host '%s'.\n", hostToRemove.Name)
	},
}

var sshImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import hosts from ~/.ssh/config interactively",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading current configuration: %v\n", err)
			os.Exit(1)
		}

		potentialHosts, err := config.ParseSSHConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error parsing ~/.ssh/config: %v\n", err)
			os.Exit(1)
		}

		if len(potentialHosts) == 0 {
			fmt.Println("No suitable hosts found in ~/.ssh/config to import.")
			return
		}

		fmt.Println("Found potential hosts in ~/.ssh/config:")
		importableHosts := []config.PotentialHost{}
		currentConfigNames := make(map[string]bool)
		for _, h := range cfg.SSHHosts {
			currentConfigNames[h.Name] = true
		}

		for i, pHost := range potentialHosts {
			// Check if a host with the same alias already exists in bm config
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

		if len(importableHosts) == 0 {
			fmt.Println("\nNo new hosts available to import.")
			return
		}

		fmt.Println("\nEnter the numbers of the hosts you want to import (comma-separated), or 'all':")
		choiceStr, err := promptString("Import selection:", true)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error reading selection: %v\n", err)
			os.Exit(1)
		}

		hostsToImport := []config.PotentialHost{}
		if strings.ToLower(choiceStr) == "all" {
			hostsToImport = importableHosts
		} else {
			indices := strings.Split(choiceStr, ",")
			for _, indexStr := range indices {
				index, err := strconv.Atoi(strings.TrimSpace(indexStr))
				if err != nil || index < 1 || index > len(potentialHosts) { // Check against original list length for index validity
					errorColor.Fprintf(os.Stderr, "Invalid selection '%s'. Please enter numbers corresponding to the list.\n", indexStr)
					os.Exit(1)
				}
				// Find the corresponding host in the *importable* list
				selectedPotentialHost := potentialHosts[index-1]
				foundInImportable := false
				for _, ih := range importableHosts {
					if ih.Alias == selectedPotentialHost.Alias { // Match by alias
						hostsToImport = append(hostsToImport, ih)
						foundInImportable = true
						break
					}
				}
				if !foundInImportable {
					errorColor.Fprintf(os.Stderr, "Host '%s' (number %d) cannot be imported (e.g., name conflict).\n", selectedPotentialHost.Alias, index)
					// Optionally continue or exit; let's exit for simplicity
					os.Exit(1)
				}
			}
		}

		if len(hostsToImport) == 0 {
			fmt.Println("No hosts selected for import.")
			return
		}

		fmt.Println("\nFor each selected host, please provide the required details:")
		importedCount := 0
		for _, pHost := range hostsToImport {
			fmt.Printf("\nImporting host '%s' (Alias: %s)...\n", identifierColor.Sprint(pHost.Alias), pHost.Alias)

			// Use alias as default bm name, check for conflicts again just in case
			bmName := pHost.Alias
			if _, exists := currentConfigNames[bmName]; exists {
				errorColor.Fprintf(os.Stderr, "Error: Name '%s' conflicts with an existing host. Skipping import.\n", bmName)
				continue
			}

			// RemoteRoot is optional during import as well
			remoteRootPrompt := "Remote Root Path (optional, defaults to ~/bucket or ~/compose-bucket):"
			remoteRoot, err := promptString(remoteRootPrompt, false)
			if err != nil {
				errorColor.Fprintf(os.Stderr, "Error reading remote root for '%s': %v. Skipping import.\n", bmName, err)
				continue
			}

			bmHost, err := config.ConvertToBucketManagerHost(pHost, bmName, remoteRoot)
			if err != nil {
				errorColor.Fprintf(os.Stderr, "Error converting host '%s': %v. Skipping import.\n", bmName, err)
				continue
			}

			// If the imported host doesn't have a key path, prompt for auth details
			if bmHost.KeyPath == "" {
				fmt.Printf("Host '%s' imported from ssh_config has no IdentityFile specified.\n", bmName)
				err = promptForAuthDetails(&bmHost, false, "")
				if err != nil {
					errorColor.Fprintf(os.Stderr, "Error getting authentication details for '%s': %v. Skipping import.\n", bmName, err)
					continue
				}
			}

			cfg.SSHHosts = append(cfg.SSHHosts, bmHost)
			currentConfigNames[bmName] = true // Add to map to prevent duplicates within this import run
			importedCount++
			successColor.Printf("Prepared '%s' for import.\n", bmName)
		}

		if importedCount == 0 {
			fmt.Println("\nNo hosts were successfully prepared for import.")
			return
		}

		err = config.SaveConfig(cfg)
		if err != nil {
			errorColor.Fprintf(os.Stderr, "\nError saving configuration: %v\n", err)
			os.Exit(1)
		}

		successColor.Printf("\nSuccessfully imported %d SSH host(s).\n", importedCount)

	},
}

func init() {
	sshCmd.AddCommand(sshListCmd)
	sshCmd.AddCommand(sshAddCmd)
	sshCmd.AddCommand(sshEditCmd)
	sshCmd.AddCommand(sshRemoveCmd)
	sshCmd.AddCommand(sshImportCmd)

	configCmd.AddCommand(sshCmd)

	rootCmd.AddCommand(configCmd)
}

var reader = bufio.NewReader(os.Stdin)

// promptString prompts the user for a string value.
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

// promptOptionalInt prompts for an optional integer value.
func promptOptionalInt(prompt string, defaultValue int) (int, error) {
	fmt.Printf("%s (default: %d): ", prompt, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue, err // Return default on read error
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

// promptConfirm prompts for a yes/no confirmation.
func promptConfirm(prompt string) (bool, error) {
	fmt.Print(prompt + " (y/N): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes", nil
}

// promptForAuthDetails handles the interactive prompting for SSH authentication details.
// It modifies the passed host struct directly.
// originalPassword is only relevant when isEditing is true and current method is password.
func promptForAuthDetails(host *config.SSHHost, isEditing bool, originalPassword string) error {
	var inputErr error
	currentAuthMethod := 2 // Default to agent if no key/password
	if host.KeyPath != "" {
		currentAuthMethod = 1
	} else if host.Password != "" {
		currentAuthMethod = 3
	}

	fmt.Println("\nAuthentication Method:")
	if isEditing {
		fmt.Print("Current: ")
		switch currentAuthMethod {
		case 1:
			fmt.Printf("SSH Key File (%s)\n", host.KeyPath)
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
	defaultChoiceStr := strconv.Itoa(currentAuthMethod)
	if isEditing {
		promptMsg += fmt.Sprintf(" (leave blank to keep current - %s):", defaultChoiceStr)
	} else {
		promptMsg += fmt.Sprintf(" (default %s):", defaultChoiceStr)
	}

	authChoiceStr, inputErr := promptString(promptMsg, false)
	if inputErr != nil {
		return fmt.Errorf("error reading auth choice: %w", inputErr)
	}

	newAuthChoice := currentAuthMethod // Default to keeping current or the initial default
	if authChoiceStr != "" {
		choice, err := strconv.Atoi(authChoiceStr)
		if err != nil || choice < 1 || choice > 3 {
			errorColor.Fprintf(os.Stderr, "Invalid choice '%s', using default/current method (%d).\n", authChoiceStr, currentAuthMethod)
			// Keep newAuthChoice as currentAuthMethod
		} else {
			newAuthChoice = choice
		}
	}

	// Clear old auth details only if the method actually changes
	if newAuthChoice != currentAuthMethod {
		host.KeyPath = ""
		host.Password = ""
	}

	// Prompt for details based on the *final* choice
	switch newAuthChoice {
	case 1:
		defaultKey := host.KeyPath // Use potentially cleared value
		prompt := "Path to Private Key File"
		required := true // Key path is required if this method is chosen
		if isEditing && defaultKey != "" {
			prompt += fmt.Sprintf(" [%s]", defaultKey)
			required = false // Not required if editing and already set
		} else {
			prompt += ":"
		}

		host.KeyPath, inputErr = promptString(prompt, required)
		if inputErr != nil {
			return fmt.Errorf("error reading key path: %w", inputErr)
		}
		if host.KeyPath == "" && isEditing {
			host.KeyPath = defaultKey
		} // Keep old if editing and user entered blank

		// Final check: If method 1 is chosen, key path cannot be empty.
		if host.KeyPath == "" {
			return fmt.Errorf("key path cannot be empty when Key File method is selected")
		}
	case 3:
		fmt.Println(errorColor.Sprint("Warning: Password will be stored in plaintext in the config file!"))
		prompt := "SSH Password"
		required := true // Password is required if this method is chosen
		if isEditing && host.Password != "" {
			prompt += " (leave blank to keep current):" // Don't show default password
			required = false
		} else {
			prompt += ":"
		}

		host.Password, inputErr = promptString(prompt, required)
		if inputErr != nil {
			return fmt.Errorf("error reading password: %w", inputErr)
		}
		// If editing and user entered blank, keep the original password *only if* the method didn't change
		if host.Password == "" && isEditing && newAuthChoice == 3 && currentAuthMethod == 3 {
			host.Password = originalPassword
		}

		// Final check: If method 3 is chosen, password cannot be empty.
		if host.Password == "" && newAuthChoice == 3 {
			return fmt.Errorf("password cannot be empty when Password method is selected")
		}
	case 2:
		// Ensure other fields are cleared if we switched to Agent
		host.KeyPath = ""
		host.Password = ""
	}
	return nil
}
