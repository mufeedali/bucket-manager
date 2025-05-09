// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"fmt"
	"strings"
	"sync"

	"github.com/briandowns/spinner"
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
			localMatch = &potentialMatches[i]
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

	if identifier != "" {
		if strings.HasSuffix(identifier, ":") { // e.g., "server1:"
			targetServerName = strings.TrimSuffix(identifier, ":")
			if targetServerName == "" {
				return nil, []error{fmt.Errorf("invalid identifier format: '%s'. Cannot be just ':'", identifier)}
			}
		} else if parts := strings.SplitN(identifier, ":", 2); len(parts) == 2 {
			targetServerName = strings.TrimSpace(parts[0])
			targetStackName = strings.TrimSpace(parts[1])
			if targetStackName == "" || targetServerName == "" {
				return nil, []error{fmt.Errorf("invalid identifier format: '%s'. Use 'stack', 'remote:stack', or 'remote:'", identifier)}
			}
		} else {
			targetStackName = identifier
		}
	}

	cfg, configErr := config.LoadConfig()

	scanAll := identifier == ""
	discoverLocal := targetServerName == "local" || targetServerName == ""
	discoverSpecificRemote := targetServerName != "local" && targetServerName != ""
	discoverAllRemotes := targetServerName == "" // Only if ambiguous and not found locally

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
			collectedErrors = append(collectedErrors, fmt.Errorf("local root check failed: %w", err))
		}
	}

	if discoverSpecificRemote {
		if configErr != nil {
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
			if s != nil {
				originalSuffix := s.Suffix
				s.Suffix = fmt.Sprintf(" Discovering on %s...", identifierColor.Sprint(targetServerName))
				defer func() { s.Suffix = originalSuffix }()
			}
			remoteStacks, err := discovery.FindRemoteStacks(targetHost)
			if err != nil {
				collectedErrors = append(collectedErrors, fmt.Errorf("remote discovery failed for %s: %w", targetHost.Name, err))
			} else {
				if targetStackName == "" {
					stacksToCheck = append(stacksToCheck, remoteStacks...)
				} else {
					for _, rs := range remoteStacks {
						if rs.Name == targetStackName {
							stacksToCheck = append(stacksToCheck, rs)
							break
						}
					}
				}
			}
		}
	}

	if discoverAllRemotes {
		foundLocally := false
		if targetStackName != "" {
			for _, s := range stacksToCheck {
				if !s.IsRemote && s.Name == targetStackName {
					foundLocally = true
					break
				}
			}
		}

		if targetStackName != "" && !foundLocally {
			if configErr != nil {
				collectedErrors = append(collectedErrors, fmt.Errorf("stack '%s' not found locally and remote discovery skipped due to config error: %w", targetStackName, configErr))
			} else if len(cfg.SSHHosts) > 0 {
				if s != nil {
					originalSuffix := s.Suffix
					s.Suffix = fmt.Sprintf(" Discovering %s on remotes...", identifierColor.Sprint(targetStackName))
					defer func() { s.Suffix = originalSuffix }()
				}

				var remoteWg sync.WaitGroup
				remoteStackChan := make(chan discovery.Stack, len(cfg.SSHHosts))
				remoteErrorChan := make(chan error, len(cfg.SSHHosts))
				remoteWg.Add(len(cfg.SSHHosts))

				for i := range cfg.SSHHosts {
					hostConfig := cfg.SSHHosts[i]
					go func(hc config.SSHHost) {
						defer remoteWg.Done()
						remoteStacks, err := discovery.FindRemoteStacks(&hc)
						if err != nil {
							remoteErrorChan <- fmt.Errorf("remote discovery failed for %s: %w", hc.Name, err)
						} else {
							for _, rs := range remoteStacks {
								if rs.Name == targetStackName {
									remoteStackChan <- rs
									break
								}
							}
						}
					}(hostConfig)
				}

				go func() {
					remoteWg.Wait()
					close(remoteStackChan)
					close(remoteErrorChan)
				}()

				for rs := range remoteStackChan {
					stacksToCheck = append(stacksToCheck, rs)
				}
				for err := range remoteErrorChan {
					collectedErrors = append(collectedErrors, err)
				}
			}
		} else if scanAll {
			if s != nil {
				originalSuffix := s.Suffix
				s.Suffix = " Discovering all stacks..."
				defer func() { s.Suffix = originalSuffix }()
			}
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

	finalStacks := []discovery.Stack{}
	if scanAll {
		finalStacks = stacksToCheck
	} else {
		// Filter stacksToCheck based on targetStackName and targetServerName
		for _, s := range stacksToCheck {
			nameMatch := (targetStackName == "" || s.Name == targetStackName)
			serverMatch := (targetServerName == "" || s.ServerName == targetServerName)

			if nameMatch && serverMatch {
				finalStacks = append(finalStacks, s)
			}
		}

		// Resolve ambiguity if needed
		if targetServerName == "" && targetStackName != "" && len(finalStacks) > 1 {
			resolvedStack, resolveErr := findStackByIdentifier(finalStacks, identifier)
			if resolveErr == nil {
				finalStacks = []discovery.Stack{resolvedStack}
			} else {
				return nil, append(collectedErrors, resolveErr)
			}
		} else if len(finalStacks) == 0 && len(collectedErrors) == 0 {
			_, notFoundErr := findStackByIdentifier(stacksToCheck, identifier)
			if notFoundErr != nil {
				return nil, []error{notFoundErr}
			}
			return nil, []error{fmt.Errorf("no stacks found matching identifier '%s'", identifier)}
		}
	}

	return finalStacks, collectedErrors
}
