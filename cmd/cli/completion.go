// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// discoverLocalStacksForCompletion performs local discovery for completion, ignoring "not found" errors.
func discoverLocalStacksForCompletion() ([]discovery.Stack, error) {
	localRootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		if strings.Contains(err.Error(), "could not find") {
			return nil, nil
		}
		return nil, err
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

	go func() {
		wg.Wait()
		close(stackChan)
		close(errorChan)
	}()

	var collectWg sync.WaitGroup
	collectWg.Add(2)

	go func() {
		defer collectWg.Done()
		for s := range stackChan {
			remoteStacks = append(remoteStacks, s)
		}
	}()
	go func() {
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
			return suggestions, cobra.ShellCompDirectiveNoFileComp
		}

		// No local matches found, proceed to discover all remotes
		var remoteStacks []discovery.Stack
		remoteStacks, discoveryErrors = discoverAllRemoteStacksForCompletion()
		stacksToSearch = append(stacksToSearch, remoteStacks...)
		// We collected remote discovery errors, but won't show them during completion
		_ = discoveryErrors
	}

	// Generate Suggestions from discovered stacks
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
