// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
	"bucket-manager/internal/runner"
	"bucket-manager/internal/ssh"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	sshManager         *ssh.Manager
	statusColor        = color.New(color.FgCyan)
	errorColor         = color.New(color.FgRed)
	stepColor          = color.New(color.FgYellow)
	successColor       = color.New(color.FgGreen)
	statusUpColor      = color.New(color.FgGreen)
	statusDownColor    = color.New(color.FgRed)
	statusPartialColor = color.New(color.FgYellow)
	statusErrorColor   = color.New(color.FgMagenta)
	identifierColor    = color.New(color.FgBlue)
)

// findStackByIdentifier finds a specific stack based on its identifier.
// Identifier can be "stackName" (implies local preference) or "serverName:stackName".
// Returns an error if not found or if "stackName" is ambiguous.
func findStackByIdentifier(stacks []discovery.Stack, identifier string) (discovery.Stack, error) {
	identifier = strings.TrimSpace(identifier)
	targetName := identifier
	targetServer := "" // "" means user didn't specify, implies local preference unless ambiguous

	if parts := strings.SplitN(identifier, ":", 2); len(parts) == 2 {
		targetServer = strings.TrimSpace(parts[0])
		targetName = strings.TrimSpace(parts[1])
		if targetName == "" || targetServer == "" {
			// Allow "remote:" format for status command later, but not here for finding a *specific* stack
			return discovery.Stack{}, fmt.Errorf("invalid identifier format: '%s'. Use 'stack' or 'remote:stack'", identifier)
		}
	}

	var potentialMatches []discovery.Stack
	var exactMatch *discovery.Stack

	for i := range stacks {
		s := stacks[i]
		if s.Name == targetName {
			if targetServer != "" {
				if s.ServerName == targetServer {
					exactMatch = &s
					break
				}
			} else {
				potentialMatches = append(potentialMatches, s)
			}
		}
	}

	if targetServer != "" {
		if exactMatch != nil {
			return *exactMatch, nil
		}
		return discovery.Stack{}, fmt.Errorf("stack '%s:%s' not found", targetServer, targetName)
	}

	if len(potentialMatches) == 0 {
		return discovery.Stack{}, fmt.Errorf("stack '%s' not found", targetName)
	}

	if len(potentialMatches) == 1 {
		return potentialMatches[0], nil
	}

	// Ambiguous case: Multiple stacks match the name, user didn't specify server.
	// Prefer a single local match if one exists.
	var localMatch *discovery.Stack
	localCount := 0
	for i := range potentialMatches {
		if !potentialMatches[i].IsRemote {
			localCount++
			localMatch = &potentialMatches[i] // Keep track of the last local match found
		}
	}

	// If exactly one local match was found among potentials, return it.
	if localCount == 1 && localMatch != nil {
		return *localMatch, nil
	}

	// Ambiguous: Multiple matches (either all remote, multiple local, or mix)
	options := make([]string, 0, len(potentialMatches))
	for _, pm := range potentialMatches {
		options = append(options, pm.Identifier()) // Use Identifier() for clarity
	}
	return discovery.Stack{}, fmt.Errorf("stack name '%s' is ambiguous, please specify one of: %s", targetName, strings.Join(options, ", "))
}

// discoverLocalStacksForCompletion performs local discovery for completion, ignoring "not found" errors.
func discoverLocalStacksForCompletion() ([]discovery.Stack, error) {
	localRootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		if strings.Contains(err.Error(), "could not find") {
			return nil, nil // Ignore "not found" for completion purposes
		}
		return nil, err // Return other errors
	}
	return discovery.FindLocalStacks(localRootDir)
}

// discoverRemoteStacksForCompletion performs discovery on a specific remote host for completion.
func discoverRemoteStacksForCompletion(remoteName string) ([]discovery.Stack, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config for remote completion: %w", err)
	}

	var targetHost *config.SSHHost
	for i := range cfg.SSHHosts {
		if cfg.SSHHosts[i].Name == remoteName {
			targetHost = &cfg.SSHHosts[i]
			break
		}
	}

	if targetHost == nil {
		return nil, nil // No error, just no stacks for a non-existent remote during completion
	}

	// Ignore errors during discovery for completion purposes
	stacks, _ := discovery.FindRemoteStacks(targetHost)
	return stacks, nil
}

// discoverAllRemoteStacksForCompletion performs discovery only on all configured remote hosts for completion.
func discoverAllRemoteStacksForCompletion() ([]discovery.Stack, []error) {
	var remoteStacks []discovery.Stack
	var discoveryErrors []error

	cfg, configErr := config.LoadConfig()
	if configErr != nil {
		// Can't discover remotes if config fails
		return nil, []error{fmt.Errorf("failed to load config for remote completion: %w", configErr)}
	}
	if len(cfg.SSHHosts) == 0 {
		return nil, nil // No remotes configured
	}

	var wg sync.WaitGroup
	stackChan := make(chan discovery.Stack, len(cfg.SSHHosts)) // Buffer size based on hosts
	errorChan := make(chan error, len(cfg.SSHHosts))           // Buffer size based on hosts
	wg.Add(len(cfg.SSHHosts))

	for i := range cfg.SSHHosts {
		hostConfig := cfg.SSHHosts[i] // Capture loop variable
		go func(hc config.SSHHost) {
			defer wg.Done()
			// Ignore errors during discovery for completion purposes
			stacks, err := discovery.FindRemoteStacks(&hc)
			if err != nil {
				// Still collect errors, even if ignored for suggestions
				errorChan <- fmt.Errorf("remote discovery failed for %s: %w", hc.Name, err)
				return
			}
			for _, s := range stacks {
				stackChan <- s
			}
		}(hostConfig)
	}

	// Goroutine to close channels once all discovery goroutines are done
	go func() {
		wg.Wait()
		close(stackChan)
		close(errorChan)
	}()

	// Collect results
	var collectWg sync.WaitGroup
	collectWg.Add(2)

	go func() { // Collect stacks
		defer collectWg.Done()
		for s := range stackChan {
			remoteStacks = append(remoteStacks, s)
		}
	}()
	go func() { // Collect errors
		defer collectWg.Done()
		for e := range errorChan {
			discoveryErrors = append(discoveryErrors, e)
		}
	}()

	collectWg.Wait()

	// Errors are collected but typically ignored by the caller for completion
	return remoteStacks, discoveryErrors
}

// stackCompletionFunc provides dynamic completion for stack identifiers.
func stackCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	suggestionMap := make(map[string]struct{}) // Use map for deduplication
	var stacksToSearch []discovery.Stack
	var discoveryErrors []error // Collect errors silently

	targetServer := ""
	targetStack := toComplete
	hasColon := strings.Contains(toComplete, ":")

	if hasColon {
		parts := strings.SplitN(toComplete, ":", 2)
		targetServer = parts[0]
		targetStack = parts[1] // Can be empty if completing server name (e.g., "remote:")
	}

	// --- Discovery Strategy ---
	switch {
	case targetServer == "local":
		// "local:" prefix: Only suggest local stacks
		stacksToSearch, _ = discoverLocalStacksForCompletion() // Ignore errors for completion
	case targetServer != "":
		// "remote:" prefix: Only suggest stacks from that specific remote
		stacksToSearch, _ = discoverRemoteStacksForCompletion(targetServer) // Ignore errors for completion
	default:
		// No prefix or just "stack": Suggest local first, then remotes if no local match
		var localStacks []discovery.Stack
		localStacks, _ = discoverLocalStacksForCompletion() // Ignore errors for completion
		stacksToSearch = localStacks                        // Start with local

		// Check if any local stack name matches the prefix
		localMatchFound := false
		if len(localStacks) > 0 {
			for _, s := range localStacks {
				// Only check stack name for prefix match when no server is specified
				if strings.HasPrefix(s.Name, targetStack) {
					suggestionMap[s.Name] = struct{}{} // Add the plain name
					localMatchFound = true
				}
			}
		}

		// If local matches were found, *only* return those plain names
		if localMatchFound {
			suggestions := make([]string, 0, len(suggestionMap))
			for suggestion := range suggestionMap {
				suggestions = append(suggestions, suggestion)
			}
			// sort.Strings(suggestions) // Optional: Sort suggestions alphabetically
			return suggestions, cobra.ShellCompDirectiveNoFileComp
		}

		// No local matches found, proceed to discover all remotes
		var remoteStacks []discovery.Stack
		remoteStacks, discoveryErrors = discoverAllRemoteStacksForCompletion()
		stacksToSearch = append(stacksToSearch, remoteStacks...) // Add remotes to the search list
		// We collected remote discovery errors, but won't show them during completion
		_ = discoveryErrors // Explicitly ignore collected errors for completion
	}

	// --- Generate Suggestions from discovered stacks ---
	for _, s := range stacksToSearch {
		identifier := s.Identifier() // e.g., "local:stack" or "remote:stack"
		name := s.Name               // e.g., "stack"

		// If completing a full identifier (e.g., "remote:st")
		if hasColon && strings.HasPrefix(identifier, toComplete) {
			suggestionMap[identifier] = struct{}{}
		}

		// If completing just a name (e.g., "st") or a server (e.g., "remote:")
		if !hasColon {
			if strings.HasPrefix(name, targetStack) {
				// Suggest the plain name if it matches
				suggestionMap[name] = struct{}{}
			}
			// Also suggest the full identifier if the server part matches
			if targetServer == "" && strings.HasPrefix(identifier, toComplete) {
				suggestionMap[identifier] = struct{}{}
			}
		}

		// Special case: If user typed "remote:", suggest all stacks for that remote
		if hasColon && targetStack == "" && s.ServerName == targetServer {
			suggestionMap[identifier] = struct{}{}
		}
	}

	suggestions := make([]string, 0, len(suggestionMap))
	for suggestion := range suggestionMap {
		suggestions = append(suggestions, suggestion)
	}
	// sort.Strings(suggestions) // Optional: Sort suggestions alphabetically

	return suggestions, cobra.ShellCompDirectiveNoFileComp
}

// hostCompletionFunc provides dynamic completion for host identifiers ("local" or remote names).
func hostCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	suggestions := []string{"local"} // Always suggest local

	cfg, err := config.LoadConfig()
	// Ignore config load errors during completion
	if err == nil {
		for _, host := range cfg.SSHHosts {
			if strings.HasPrefix(host.Name, toComplete) {
				suggestions = append(suggestions, host.Name)
			}
		}
	}

	// Filter suggestions based on toComplete prefix if it wasn't used in the loop
	// (e.g., if completing "loc" for "local")
	finalSuggestions := []string{}
	for _, s := range suggestions {
		if strings.HasPrefix(s, toComplete) {
			finalSuggestions = append(finalSuggestions, s)
		}
	}

	return finalSuggestions, cobra.ShellCompDirectiveNoFileComp
}

var rootCmd = &cobra.Command{
	Use:   "bm",
	Short: "Bucket Manager CLI",
	Long: `A command-line interface to manage multiple Podman Compose stacks.

Discovers stacks in standard local directories (~/bucket, ~/compose-bucket)
and on remote hosts configured via SSH (~/.config/bucket-manager/config.yaml).`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := config.EnsureConfigDir(); err != nil {
			return fmt.Errorf("failed to ensure config directory: %w", err)
		}
		sshManager = ssh.NewManager()
		discovery.InitSSHManager(sshManager)
		runner.InitSSHManager(sshManager)
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if sshManager != nil {
			sshManager.CloseAll()
		}
		return nil
	},
}

func RunCLI() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(refreshCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pruneCmd) // Add prune command
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered Podman Compose stacks (local and remote)",
	Run: func(cmd *cobra.Command, args []string) {
		statusColor.Println("Discovering stacks...")
		stackChan, errorChan, _ := discovery.FindStacks()

		var collectedErrors []error
		var stacksFound bool
		var wg sync.WaitGroup
		wg.Add(1) // Only wait for the error collection goroutine

		// Goroutine to collect errors and print them immediately
		go func() {
			defer wg.Done()
			for err := range errorChan {
				collectedErrors = append(collectedErrors, err)
				errorColor.Fprintf(os.Stderr, "Error during discovery: %v\n", err)
			}
		}()

		fmt.Println("\nDiscovered stacks:")

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Color("cyan")
		s.Suffix = " Loading remote stacks..."
		s.Start()

		for stack := range stackChan {
			s.Stop()
			stacksFound = true
			fmt.Printf("- %s (%s)\n", stack.Name, identifierColor.Sprint(stack.ServerName))
			s.Restart()
		}
		s.Stop()

		wg.Wait()

		if !stacksFound && len(collectedErrors) == 0 {
			fmt.Println("\nNo Podman Compose stacks found locally or on configured remote hosts.")
		} else if !stacksFound && len(collectedErrors) > 0 {
			fmt.Println("\nNo stacks discovered successfully.")
		}

		if len(collectedErrors) > 0 {
			os.Exit(1)
		}
	},
}

// discoverTargetStacks finds stacks based on an identifier, handling local/remote discovery.
// identifier: The stack identifier (e.g., "my-app", "server1:my-app", "local:my-app").
//
//	Can also be "server1:" to discover all stacks on server1 (used by status).
//	If empty, discovers all stacks.
//
// s: Optional spinner for feedback during remote discovery.
func discoverTargetStacks(identifier string, s *spinner.Spinner) ([]discovery.Stack, []error) {
	var stacksToCheck []discovery.Stack
	var collectedErrors []error
	targetStackName := ""
	targetServerName := "" // "local", specific remote name, or "" for ambiguous/all

	// 1. Parse Identifier
	if identifier != "" {
		if strings.HasSuffix(identifier, ":") { // e.g., "server1:"
			targetServerName = strings.TrimSuffix(identifier, ":")
			if targetServerName == "" { // Just ":" is invalid
				return nil, []error{fmt.Errorf("invalid identifier format: '%s'. Cannot be just ':'", identifier)}
			}
			// targetStackName remains "" -> find all on this server
		} else if parts := strings.SplitN(identifier, ":", 2); len(parts) == 2 { // e.g., "server1:app"
			targetServerName = strings.TrimSpace(parts[0])
			targetStackName = strings.TrimSpace(parts[1])
			if targetStackName == "" || targetServerName == "" {
				return nil, []error{fmt.Errorf("invalid identifier format: '%s'. Use 'stack', 'remote:stack', or 'remote:'", identifier)}
			}
		} else { // e.g., "app"
			targetStackName = identifier
			// targetServerName remains "" -> implies local preference or ambiguous
		}
	}
	// If identifier is "", scanAll case: targetServerName and targetStackName remain ""

	// 2. Load Config (needed for any remote operation)
	cfg, configErr := config.LoadConfig()
	// Defer returning config error until we know we need remotes

	// 3. Perform Discovery based on parsed identifier
	scanAll := identifier == ""
	discoverLocal := targetServerName == "local" || targetServerName == ""
	discoverSpecificRemote := targetServerName != "local" && targetServerName != ""
	discoverAllRemotes := targetServerName == "" // Only if ambiguous and not found locally

	// --- Local Discovery ---
	if discoverLocal {
		localRootDir, err := discovery.GetComposeRootDirectory()
		if err == nil {
			localStacks, err := discovery.FindLocalStacks(localRootDir)
			if err != nil {
				collectedErrors = append(collectedErrors, fmt.Errorf("local discovery failed: %w", err))
			} else {
				stacksToCheck = append(stacksToCheck, localStacks...)
			}
		} else if !strings.Contains(err.Error(), "could not find") {
			// Report errors other than "not found"
			collectedErrors = append(collectedErrors, fmt.Errorf("local root check failed: %w", err))
		}
	}

	// --- Specific Remote Discovery ---
	if discoverSpecificRemote {
		if configErr != nil { // Now we definitely need the config
			return nil, []error{fmt.Errorf("error loading config needed for remote discovery: %w", configErr)}
		}
		var targetHost *config.SSHHost
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == targetServerName {
				targetHost = &cfg.SSHHosts[i]
				break
			}
		}
		if targetHost == nil {
			collectedErrors = append(collectedErrors, fmt.Errorf("remote host '%s' not found in configuration", targetServerName))
		} else {
			if s != nil { // Update spinner if provided
				originalSuffix := s.Suffix
				s.Suffix = fmt.Sprintf(" Discovering on %s...", identifierColor.Sprint(targetServerName))
				defer func() { s.Suffix = originalSuffix }()
			}
			remoteStacks, err := discovery.FindRemoteStacks(targetHost)
			if err != nil {
				collectedErrors = append(collectedErrors, fmt.Errorf("remote discovery failed for %s: %w", targetHost.Name, err))
			} else {
				// Filter results based on whether we need all stacks or a specific one
				if targetStackName == "" { // "remote:" case
					stacksToCheck = append(stacksToCheck, remoteStacks...)
				} else { // "remote:stack" case
					for _, rs := range remoteStacks {
						if rs.Name == targetStackName {
							stacksToCheck = append(stacksToCheck, rs)
							break // Found the specific stack on this remote
						}
					}
				}
			}
		}
	}

	// --- Ambiguous Case: Discover All Remotes (if needed) ---
	if discoverAllRemotes {
		// Check if the target stack was already found locally
		foundLocally := false
		if targetStackName != "" { // Only relevant if looking for a specific stack
			for _, s := range stacksToCheck {
				if !s.IsRemote && s.Name == targetStackName {
					foundLocally = true
					break
				}
			}
		}

		// If looking for a specific stack AND it wasn't found locally, search remotes
		if targetStackName != "" && !foundLocally {
			if configErr != nil { // Check config error before remote scan
				collectedErrors = append(collectedErrors, fmt.Errorf("stack '%s' not found locally and remote discovery skipped due to config error: %w", targetStackName, configErr))
			} else if len(cfg.SSHHosts) > 0 {
				if s != nil { // Update spinner
					originalSuffix := s.Suffix
					s.Suffix = fmt.Sprintf(" Discovering %s on remotes...", identifierColor.Sprint(targetStackName))
					defer func() { s.Suffix = originalSuffix }()
				}

				var remoteWg sync.WaitGroup
				remoteStackChan := make(chan discovery.Stack, len(cfg.SSHHosts))
				remoteErrorChan := make(chan error, len(cfg.SSHHosts))
				remoteWg.Add(len(cfg.SSHHosts))

				for i := range cfg.SSHHosts {
					hostConfig := cfg.SSHHosts[i] // Capture loop variable
					go func(hc config.SSHHost) {
						defer remoteWg.Done()
						remoteStacks, err := discovery.FindRemoteStacks(&hc)
						if err != nil {
							remoteErrorChan <- fmt.Errorf("remote discovery failed for %s: %w", hc.Name, err)
						} else {
							// Add remote stack only if it matches the target name
							for _, rs := range remoteStacks {
								if rs.Name == targetStackName {
									remoteStackChan <- rs
									break // Found the one we need on this remote
								}
							}
						}
					}(hostConfig)
				}

				// Closer goroutine
				go func() {
					remoteWg.Wait()
					close(remoteStackChan)
					close(remoteErrorChan)
				}()

				// Collect remote stacks matching the name
				for rs := range remoteStackChan {
					stacksToCheck = append(stacksToCheck, rs)
				}
				// Collect remote discovery errors
				for err := range remoteErrorChan {
					collectedErrors = append(collectedErrors, err)
				}
			}
		} else if scanAll { // If scanning all, use the main FindStacks function
			// This block replaces the original `if scanAll` block logic
			if s != nil { // Update spinner
				originalSuffix := s.Suffix
				s.Suffix = " Discovering all stacks..."
				defer func() { s.Suffix = originalSuffix }()
			}
			// Clear potentially added local stacks if we are rescanning all
			stacksToCheck = nil
			stackChan, errorChan, _ := discovery.FindStacks()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				for s := range stackChan {
					stacksToCheck = append(stacksToCheck, s)
				}
			}()
			go func() {
				defer wg.Done()
				for e := range errorChan {
					collectedErrors = append(collectedErrors, e)
				}
			}()
			wg.Wait()
		}
	}

	// --- Final Filtering & Ambiguity Resolution ---
	finalStacks := []discovery.Stack{}
	if scanAll {
		finalStacks = stacksToCheck // No filtering needed for scan all
	} else {
		// Filter based on explicit parts of the identifier
		for _, s := range stacksToCheck {
			nameMatch := (targetStackName == "" || s.Name == targetStackName)
			serverMatch := (targetServerName == "" || s.ServerName == targetServerName)

			if nameMatch && serverMatch {
				finalStacks = append(finalStacks, s)
			}
		}

		// If the identifier was ambiguous ("stack") and multiple matches remain, resolve ambiguity
		if targetServerName == "" && targetStackName != "" && len(finalStacks) > 1 {
			resolvedStack, resolveErr := findStackByIdentifier(finalStacks, identifier) // Use original identifier
			if resolveErr == nil {
				// If resolved successfully (e.g. preferred local), return only that one
				finalStacks = []discovery.Stack{resolvedStack}
			} else {
				// If ambiguous and couldn't resolve, return the ambiguity error
				// Combine with any previous discovery errors
				return nil, append(collectedErrors, resolveErr)
			}
		} else if len(finalStacks) == 0 && len(collectedErrors) == 0 {
			// If filtering resulted in no stacks and there were no prior discovery errors,
			// return a specific "not found" error based on the original identifier.
			// Use findStackByIdentifier to generate the appropriate error message.
			_, notFoundErr := findStackByIdentifier(stacksToCheck, identifier) // Check against original discovered set
			if notFoundErr != nil {
				return nil, []error{notFoundErr} // Return the specific not found/ambiguous error
			}
			// If findStackByIdentifier somehow doesn't error, return generic not found
			return nil, []error{fmt.Errorf("no stacks found matching identifier '%s'", identifier)}
		}
	}

	return finalStacks, collectedErrors
}

func runStackAction(action string, args []string) {
	if len(args) != 1 {
		errorColor.Fprintf(os.Stderr, "Error: requires exactly one stack identifier argument.\n")
		os.Exit(1)
	}
	stackIdentifier := args[0]

	statusColor.Printf("Locating stack '%s'...\n", stackIdentifier)

	// Use discoverTargetStacks which handles finding relevant stacks based on the identifier
	// Pass nil for spinner as this is a non-interactive phase
	stacksToCheck, collectedErrors := discoverTargetStacks(stackIdentifier, nil)

	// Handle Discovery Errors first
	if len(collectedErrors) > 0 {
		errorColor.Fprintln(os.Stderr, "\nErrors during stack discovery:")
		for _, err := range collectedErrors {
			errorColor.Fprintf(os.Stderr, "- %v\n", err)
		}
		// Exit even if some stacks were found, as the discovery wasn't fully successful
		os.Exit(1)
	}

	// Now, try to find the specific stack from the discovered ones
	targetStack, err := findStackByIdentifier(stacksToCheck, stackIdentifier)
	if err != nil {
		// This handles "not found" or "ambiguous" errors based on the discovered stacks
		errorColor.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	// Proceed with the action on the uniquely identified target stack
	statusColor.Printf("Executing '%s' action for stack: %s (%s)\n", action, targetStack.Name, identifierColor.Sprint(targetStack.ServerName))

	var sequence []runner.CommandStep
	switch action {
	case "up":
		sequence = runner.UpSequence(targetStack)
	case "down":
		sequence = runner.DownSequence(targetStack)
	case "refresh":
		sequence = runner.RefreshSequence(targetStack)
	case "pull":
		sequence = runner.PullSequence(targetStack)
	default:
		errorColor.Fprintf(os.Stderr, "Internal Error: Invalid action '%s'\n", action)
		os.Exit(1)
	}

	err = runSequence(targetStack, sequence)
	if err != nil {
		logger.Errorf("\n'%s' action failed for %s (%s): %v", action, targetStack.Name, targetStack.ServerName, err)
		os.Exit(1)
	}
	successColor.Printf("'%s' action completed successfully for %s (%s).\n", action, targetStack.Name, identifierColor.Sprint(targetStack.ServerName))
}

var upCmd = &cobra.Command{
	Use:               "up <stack-identifier>",
	Short:             "Run 'pull' and 'up -d' for a stack",
	Example:           "  bm up my-local-app\n  bm up server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("up", args)
	},
}

var downCmd = &cobra.Command{
	Use:               "down <stack-identifier>",
	Short:             "Run 'podman compose down' for a stack",
	Example:           "  bm down my-local-app\n  bm down server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("down", args)
	},
}

var refreshCmd = &cobra.Command{
	Use:               "refresh <stack-identifier>",
	Aliases:           []string{"re"},
	Short:             "Run 'pull', 'down', 'up', and maybe 'prune' for a stack (alias: re)",
	Long:              `Runs 'pull', 'down', 'up -d' for the specified stack. Additionally runs 'podman system prune -af' locally if the target stack is local.`,
	Example:           "  bm refresh my-local-app\n  bm re server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("refresh", args)
	},
}

var pullCmd = &cobra.Command{
	Use:               "pull <stack-identifier>",
	Short:             "Run 'podman compose pull' for a stack",
	Example:           "  bm pull my-local-app\n  bm pull server1:remote-app",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		runStackAction("pull", args)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status [stack-identifier]",
	Short: "Show the status of containers for one or all stacks",
	Long: `Shows the status of Podman Compose containers for local and remote stacks.
If a stack identifier (e.g., my-app or server1:remote-app) is provided, shows status for that specific stack.
If a remote identifier ending with ':' (e.g., server1:) is provided, shows status for all stacks on that remote.
Otherwise, shows status for all discovered stacks.`,
	Example:           "  bm status\n  bm status my-local-app\n  bm status server1:remote-app\n  bm status server1:",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: stackCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		var collectedErrors []error
		// var specificStackIdentifier string // Not needed directly, use discoveryIdentifier
		scanAll := len(args) == 0

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Color("cyan")

		discoveryIdentifier := ""
		if !scanAll {
			discoveryIdentifier = args[0] // Use the provided identifier directly
			statusColor.Printf("Checking status for %s...\n", identifierColor.Sprint(discoveryIdentifier))
			s.Suffix = fmt.Sprintf(" Discovering %s...", identifierColor.Sprint(discoveryIdentifier))
		} else {
			statusColor.Println("Discovering all stacks and checking status...")
			s.Suffix = " Discovering stacks..."
		}
		s.Start()

		// Use the refactored discoverTargetStacks with the correctly determined identifier
		stacksToProcess, collectedErrors := discoverTargetStacks(discoveryIdentifier, s)
		s.Stop()

		// Handle Discovery Errors
		if len(collectedErrors) > 0 {
			logger.Error("\nErrors during stack discovery:")
			for _, err := range collectedErrors {
				logger.Errorf("- %v", err)
			}
			// Exit if discovery failed completely, otherwise continue with found stacks
			if len(stacksToProcess) == 0 {
				os.Exit(1)
			}
			errorColor.Fprintln(os.Stderr, "Continuing with successfully discovered stacks...")
		}

		// If a specific identifier was given but resulted in ambiguity or "not found"
		// after discovery, findStackByIdentifier (called within discoverTargetStacks)
		// would have returned an error, handled above.
		// If discoverTargetStacks returns stacks, they are the ones to process.

		if len(stacksToProcess) == 0 {
			if scanAll {
				fmt.Println("\nNo Podman Compose stacks found locally or on configured remote hosts.")
			}
			// Exit if no stacks to process AND no prior discovery errors occurred
			if len(collectedErrors) == 0 {
				os.Exit(1)
			}
		}

		// --- Perform Status Checks on the final list of stacks ---
		if len(stacksToProcess) > 0 {
			statusChan := make(chan runner.StackRuntimeInfo, len(stacksToProcess))
			var statusWg sync.WaitGroup
			statusWg.Add(len(stacksToProcess))

			s.Suffix = " Checking stack status..."
			s.Start()

			for _, stack := range stacksToProcess {
				go func(s discovery.Stack) {
					defer statusWg.Done()
					statusInfo := runner.GetStackStatus(s)
					statusChan <- statusInfo
				}(stack) // Pass stack by value
			}

			// Closer goroutine
			go func() {
				statusWg.Wait()
				close(statusChan)
			}()

			// Process status results as they arrive
			for statusInfo := range statusChan {
				s.Stop() // Stop spinner briefly for printing

				fmt.Printf("\nStack: %s (%s) ", statusInfo.Stack.Name, identifierColor.Sprint(statusInfo.Stack.ServerName))
				switch statusInfo.OverallStatus {
				case runner.StatusUp:
					statusUpColor.Printf("[%s]\n", statusInfo.OverallStatus)
				case runner.StatusDown:
					statusDownColor.Printf("[%s]\n", statusInfo.OverallStatus)
				case runner.StatusPartial:
					statusPartialColor.Printf("[%s]\n", statusInfo.OverallStatus)
				case runner.StatusError:
					statusErrorColor.Printf("[%s]\n", statusInfo.OverallStatus)
					// Add error to collectedErrors for final summary
					err := fmt.Errorf("status check for %s failed: %w", statusInfo.Stack.Identifier(), statusInfo.Error)
					collectedErrors = append(collectedErrors, err)
					if statusInfo.Error != nil {
						logger.Errorf("  Error checking status: %v", statusInfo.Error)
					} else {
						logger.Error("  Unknown error checking status.")
					}
				default: // Should not happen, but handle defensively
					fmt.Printf("[%s]\n", statusInfo.OverallStatus)
				}

				// Print container details if stack is not fully down
				if statusInfo.OverallStatus != runner.StatusDown && len(statusInfo.Containers) > 0 {
					fmt.Println("  Containers:")
					// TODO: Consider using text/tabwriter for better alignment if needed
					fmt.Printf("    %-25s %-35s %s\n", "SERVICE", "CONTAINER NAME", "STATUS")
					fmt.Printf("    %-25s %-35s %s\n", strings.Repeat("-", 25), strings.Repeat("-", 35), strings.Repeat("-", 6))
					for _, c := range statusInfo.Containers {
						// Simplified status check
						isUp := strings.Contains(strings.ToLower(c.Status), "running") ||
							strings.Contains(strings.ToLower(c.Status), "healthy") ||
							strings.HasPrefix(c.Status, "Up")

						statusPrinter := statusDownColor
						if isUp {
							statusPrinter = statusUpColor
						}
						fmt.Printf("    %-25s %-35s %s\n", c.Service, c.Name, statusPrinter.Sprint(c.Status))
					}
				}
				s.Restart() // Restart spinner for subsequent checks
			}
			s.Stop() // Final stop
		}

		// Final error check
		if len(collectedErrors) > 0 {
			// Errors were already printed during discovery or status check phase
			// Just exit with non-zero status
			os.Exit(1)
		}
	},
}

// runSequence executes a series of command steps for a given stack.
func runSequence(stack discovery.Stack, sequence []runner.CommandStep) error {
	for _, step := range sequence {
		stepColor.Printf("\n--- Running Step: %s for %s (%s) ---\n", step.Name, stack.Name, identifierColor.Sprint(stack.ServerName))

		// CLI always uses cliMode: true for direct output
		outChan, errChan := runner.StreamCommand(step, true)

		var stepErr error
		var wg sync.WaitGroup

		if !step.Stack.IsRemote {
			// --- Local Execution ---
			// Output is connected directly in runner.go.
			// We only need to wait for the command to finish via errChan.
			stepErr = <-errChan
			// Print a newline after direct output finishes.
			fmt.Println()
		} else {
			// --- Remote Execution ---
			// Process output via the outChan as before.
			wg.Add(1)
			go func() {
				defer wg.Done()
				for outputLine := range outChan {
					// Print raw output directly to stdout. Let the terminal handle \r and colors.
					fmt.Fprint(os.Stdout, outputLine.Line)
				}
			}()

			// Wait for the error channel to return the final error status
			stepErr = <-errChan

			// Wait for the output processing goroutine to finish
			wg.Wait()

			// Print a newline AFTER all remote output is done.
			fmt.Println()
		}

		// --- Handle Step Completion ---
		if stepErr != nil {
			// Error details might have been printed by the command itself.
			return fmt.Errorf("step '%s' failed", step.Name) // Simplified error message
		}
		successColor.Printf("--- Step '%s' completed successfully for %s (%s) ---\n", step.Name, stack.Name, identifierColor.Sprint(stack.ServerName))
	}
	return nil
}

// runHostAction executes a host-level action (like prune) on one or more targets.
func runHostAction(actionName string, targets []runner.HostTarget) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(targets)) // Buffered channel for errors

	for _, target := range targets {
		wg.Add(1)
		go func(t runner.HostTarget) {
			defer wg.Done()

			var step runner.HostCommandStep
			switch actionName {
			case "prune":
				step = runner.PruneHostStep(t)
			default:
				errChan <- fmt.Errorf("internal error: unknown host action '%s'", actionName)
				return
			}

			stepColor.Printf("\n--- Running Step: %s for host %s ---\n", step.Name, identifierColor.Sprint(t.ServerName))
			// CLI always uses cliMode: true for direct output
			outChan, stepErrChan := runner.RunHostCommand(step, true)

			var stepErr error
			var outputWg sync.WaitGroup

			if !t.IsRemote {
				// --- Local Execution ---
				// Output is connected directly in runner.go.
				// We only need to wait for the command to finish via stepErrChan.
				stepErr = <-stepErrChan
				// Print a newline after direct output finishes.
				fmt.Println()
			} else {
				// --- Remote Execution ---
				// Process output via the outChan as before.
				outputWg.Add(1)
				go func() {
					defer outputWg.Done()
					for outputLine := range outChan {
						// Print raw output directly to stdout. Let the terminal handle \r and colors.
						fmt.Fprint(os.Stdout, outputLine.Line)
					}
				}()

				stepErr = <-stepErrChan
				outputWg.Wait() // Ensure all remote output is processed before checking error

				// Print a newline AFTER all remote output is done.
				fmt.Println()
			}

			// --- Handle Step Completion ---
			if stepErr != nil {
				err := fmt.Errorf("step '%s' failed for host %s", step.Name, t.ServerName) // Simplified error
				// Avoid printing error details again if they were part of command output
				logger.Errorf("%v", err) // Print simplified error summary
				errChan <- err           // Send error to the channel
				return
			}
			successColor.Printf("--- Step '%s' completed successfully for host %s ---\n", step.Name, identifierColor.Sprint(t.ServerName))
		}(target)
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errChan) // Close the error channel after all goroutines are done

	// Check if any errors occurred
	var collectedErrors []error
	for err := range errChan {
		collectedErrors = append(collectedErrors, err)
	}

	if len(collectedErrors) > 0 {
		return fmt.Errorf("%d host action(s) failed", len(collectedErrors)) // Return a summary error
	}

	return nil
}

var pruneCmd = &cobra.Command{
	Use:   "prune [host-identifier...]",
	Short: "Run 'podman system prune -af' on specified hosts (local or remote)",
	Long: `Runs 'podman system prune -af' to remove unused data (containers, networks, images, volumes).
Targets can be 'local', specific remote host names, or left empty to target ALL configured hosts (local + remotes).`,
	Example: `  bm prune          # Prune local system AND all configured remote hosts
	 bm prune local       # Prune only the local system
	 bm prune server1     # Prune only the remote host 'server1'
	 bm prune local server1 server2 # Prune local, server1, and server2`,
	ValidArgsFunction: hostCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			errorColor.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		targetsToPrune := []runner.HostTarget{}
		targetMap := make(map[string]bool) // To avoid duplicates

		if len(args) == 0 {
			// No args: target local + all remotes
			statusColor.Println("Targeting local host and all configured remote hosts for prune...")
			targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: false, ServerName: "local"})
			targetMap["local"] = true
			for _, host := range cfg.SSHHosts {
				if !host.Disabled { // Skip disabled hosts
					targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: true, HostConfig: &host, ServerName: host.Name})
					targetMap[host.Name] = true
				}
			}
		} else {
			// Specific args provided
			statusColor.Printf("Targeting specified hosts for prune: %s...\n", strings.Join(args, ", "))
			for _, targetName := range args {
				if targetMap[targetName] {
					continue // Skip duplicates
				}

				if targetName == "local" {
					targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: false, ServerName: "local"})
					targetMap["local"] = true
				} else {
					found := false
					for i := range cfg.SSHHosts {
						host := cfg.SSHHosts[i] // Use index to get addressable copy for pointer
						if host.Name == targetName {
							if host.Disabled {
								errorColor.Fprintf(os.Stderr, "Warning: Skipping disabled host '%s'\n", targetName)
							} else {
								targetsToPrune = append(targetsToPrune, runner.HostTarget{IsRemote: true, HostConfig: &host, ServerName: host.Name})
								targetMap[host.Name] = true
							}
							found = true
							break
						}
					}
					if !found {
						errorColor.Fprintf(os.Stderr, "Error: Host identifier '%s' not found in configuration.\n", targetName)
						os.Exit(1)
					}
				}
			}
		}

		if len(targetsToPrune) == 0 {
			errorColor.Fprintln(os.Stderr, "No valid targets specified or found for prune.")
			os.Exit(1)
		}

		err = runHostAction("prune", targetsToPrune)
		if err != nil {
			logger.Errorf("\nPrune action failed for one or more hosts: %v", err)
			os.Exit(1)
		}

		successColor.Println("\nPrune action completed for all targeted hosts.")
	},
}
