// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
	"bucket-manager/internal/runner"
	"fmt"
	"os"
	"sync"
)

// runStackAction locates the target stack and executes a predefined sequence of runner steps.
func runStackAction(action string, args []string) {
	if len(args) != 1 {
		errorColor.Fprintf(os.Stderr, "Error: requires exactly one stack identifier argument.\n")
		os.Exit(1)
	}
	stackIdentifier := args[0]

	statusColor.Printf("Locating stack '%s'...\n", stackIdentifier)

	// Note: discoverTargetStacks is assumed to be moved to discovery.go or similar
	stacksToCheck, collectedErrors := discoverTargetStacks(stackIdentifier, nil)

	if len(collectedErrors) > 0 {
		errorColor.Fprintln(os.Stderr, "\nErrors during stack discovery:")
		for _, err := range collectedErrors {
			errorColor.Fprintf(os.Stderr, "- %v\n", err)
		}
		os.Exit(1)
	}

	// Note: findStackByIdentifier is assumed to be moved to discovery.go or similar
	targetStack, err := findStackByIdentifier(stacksToCheck, stackIdentifier)
	if err != nil {
		errorColor.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

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

// runSequence executes a series of command steps for a given stack.
func runSequence(stack discovery.Stack, sequence []runner.CommandStep) error {
	for _, step := range sequence {
		stepColor.Printf("\n--- Running Step: %s for %s (%s) ---\n", step.Name, stack.Name, identifierColor.Sprint(stack.ServerName))

		// CLI always uses cliMode: true for direct output
		outChan, errChan := runner.StreamCommand(step, true)

		var stepErr error
		var wg sync.WaitGroup

		if !step.Stack.IsRemote {
			stepErr = <-errChan
			fmt.Println()
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for outputLine := range outChan {
					fmt.Fprint(os.Stdout, outputLine.Line)
				}
			}()

			stepErr = <-errChan
			wg.Wait()
			fmt.Println()
		}

		if stepErr != nil {
			return fmt.Errorf("step '%s' failed", step.Name)
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
				stepErr = <-stepErrChan
				fmt.Println()
			} else {
				outputWg.Add(1)
				go func() {
					defer outputWg.Done()
					for outputLine := range outChan {
						fmt.Fprint(os.Stdout, outputLine.Line)
					}
				}()

				stepErr = <-stepErrChan
				outputWg.Wait()
				fmt.Println()
			}

			if stepErr != nil {
				err := fmt.Errorf("step '%s' failed for host %s", step.Name, t.ServerName)
				logger.Errorf("%v", err) // Log the error
				errChan <- err           // Send the error to the channel
				return
			}
			successColor.Printf("--- Step '%s' completed successfully for host %s ---\n", step.Name, identifierColor.Sprint(t.ServerName))
		}(target)
	}

	wg.Wait()
	close(errChan)

	var collectedErrors []error
	for err := range errChan {
		collectedErrors = append(collectedErrors, err)
	}

	if len(collectedErrors) > 0 {
		// Combine errors or return the first one, or a summary error
		// Returning a summary error here
		return fmt.Errorf("%d host action(s) failed", len(collectedErrors))
	}

	return nil
}
