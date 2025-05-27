// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package cli's execution.go contains the implementation logic for CLI commands
// that execute actions on stacks or hosts. It handles targeting stacks by name,
// running compose actions, and managing multiple concurrent operations.

package cli

import (
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
	"bucket-manager/internal/runner"
	"fmt"
	"os"
	"sync"
)

// runStackAction locates the target stacks and executes a predefined sequence of runner steps.
// It handles parsing multiple stack identifiers, discovering the stacks, and executing the
// specified action (up, down, refresh, or pull) on each stack.
func runStackAction(action string, args []string) {
	if len(args) == 0 {
		errorColor.Fprintf(os.Stderr, "Error: requires at least one stack identifier argument.\n")
		os.Exit(1)
	}

	// Log the start of the action
	logger.Info("Stack action started",
		"action", action,
		"stack_identifiers", args,
		"stack_count", len(args))

	if len(args) == 1 {
		statusColor.Printf("Locating stack '%s'...\n", args[0])
	} else {
		statusColor.Printf("Locating %d stacks...\n", len(args))
	}

	var targetStacks []discovery.Stack
	var allErrors []error

	// Discover each stack individually
	for _, stackIdentifier := range args {
		stacksToCheck, collectedErrors := discoverTargetStacks(stackIdentifier, nil)

		if len(collectedErrors) > 0 {
			logger.Error("Stack discovery failed",
				"action", action,
				"stack_identifier", stackIdentifier,
				"error_count", len(collectedErrors))
			for _, err := range collectedErrors {
				allErrors = append(allErrors, fmt.Errorf("stack '%s': %w", stackIdentifier, err))
			}
			continue
		}

		targetStack, err := findStackByIdentifier(stacksToCheck, stackIdentifier)
		if err != nil {
			logger.Error("Stack not found",
				"action", action,
				"stack_identifier", stackIdentifier,
				"error", err)
			allErrors = append(allErrors, fmt.Errorf("stack '%s': %w", stackIdentifier, err))
			continue
		}

		targetStacks = append(targetStacks, targetStack)
		logger.Info("Stack located successfully",
			"action", action,
			"stack_name", targetStack.Name,
			"server_name", targetStack.ServerName,
			"is_remote", targetStack.IsRemote,
			"path", targetStack.Path)
	}

	// Report any discovery errors
	if len(allErrors) > 0 {
		errorColor.Fprintln(os.Stderr, "\nErrors during stack discovery:")
		for _, err := range allErrors {
			errorColor.Fprintf(os.Stderr, "- %v\n", err)
		}
	}

	// Exit if no stacks were found
	if len(targetStacks) == 0 {
		logger.Error("No stacks found",
			"action", action,
			"stack_identifiers", args)
		errorColor.Fprintf(os.Stderr, "\nNo stacks were found or accessible.\n")
		os.Exit(1)
	}

	// Execute action on each stack
	var executionErrors []error
	for i, targetStack := range targetStacks {
		if len(targetStacks) > 1 {
			statusColor.Printf("\n[%d/%d] Executing '%s' action for stack: %s (%s)\n",
				i+1, len(targetStacks), action, targetStack.Name, identifierColor.Sprint(targetStack.ServerName))
		} else {
			statusColor.Printf("Executing '%s' action for stack: %s (%s)\n",
				action, targetStack.Name, identifierColor.Sprint(targetStack.ServerName))
		}

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
			logger.Error("Invalid action requested",
				"action", action,
				"stack_name", targetStack.Name)
			errorColor.Fprintf(os.Stderr, "Internal Error: Invalid action '%s'\n", action)
			os.Exit(1)
		}

		logger.Debug("Action sequence prepared",
			"action", action,
			"stack_name", targetStack.Name,
			"step_count", len(sequence))

		err := runSequence(targetStack, sequence)
		if err != nil {
			logger.Error("Stack action failed",
				"action", action,
				"stack_name", targetStack.Name,
				"server_name", targetStack.ServerName,
				"error", err)
			executionErrors = append(executionErrors, fmt.Errorf("'%s' action failed for %s (%s): %w",
				action, targetStack.Name, targetStack.ServerName, err))
			continue
		}

		logger.Info("Stack action completed successfully",
			"action", action,
			"stack_name", targetStack.Name,
			"server_name", targetStack.ServerName)
		successColor.Printf("'%s' action completed successfully for %s (%s).\n",
			action, targetStack.Name, identifierColor.Sprint(targetStack.ServerName))
	}

	// Report execution summary
	if len(executionErrors) > 0 {
		errorColor.Fprintf(os.Stderr, "\n%d stack(s) failed:\n", len(executionErrors))
		for _, err := range executionErrors {
			errorColor.Fprintf(os.Stderr, "- %v\n", err)
		}

		if len(executionErrors) < len(targetStacks) {
			successColor.Printf("\n%d stack(s) completed successfully.\n", len(targetStacks)-len(executionErrors))
		}
		os.Exit(1)
	} else {
		if len(targetStacks) > 1 {
			successColor.Printf("\nAll %d stack(s) completed successfully.\n", len(targetStacks))
		}
	}
}

// runSequence executes a series of command steps for a given stack.
func runSequence(stack discovery.Stack, sequence []runner.CommandStep) error {
	logger.Debug("Command sequence started",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"step_count", len(sequence))

	for i, step := range sequence {
		logger.Debug("Step starting",
			"step_index", i+1,
			"step_name", step.Name,
			"stack_name", stack.Name,
			"server_name", stack.ServerName,
			"command", step.Command,
			"args", step.Args)

		stepColor.Printf("\n--- Running Step: %s for %s (%s) ---\n", step.Name, stack.Name, identifierColor.Sprint(stack.ServerName))

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
			logger.Error("Step failed",
				"step_index", i+1,
				"step_name", step.Name,
				"stack_name", stack.Name,
				"server_name", stack.ServerName,
				"error", stepErr)
			return fmt.Errorf("step '%s' failed", step.Name)
		}

		logger.Debug("Step completed successfully",
			"step_index", i+1,
			"step_name", step.Name,
			"stack_name", stack.Name,
			"server_name", stack.ServerName)
		successColor.Printf("--- Step '%s' completed successfully for %s (%s) ---\n", step.Name, stack.Name, identifierColor.Sprint(stack.ServerName))
	}

	logger.Debug("Command sequence completed",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"step_count", len(sequence))
	return nil
}

// runHostAction executes a host-level action (like prune) on one or more targets.
func runHostAction(actionName string, targets []runner.HostTarget) error {
	logger.Info("Host action started",
		"action", actionName,
		"target_count", len(targets))

	var wg sync.WaitGroup
	errChan := make(chan error, len(targets)) // Buffered channel for errors

	for _, target := range targets {
		wg.Add(1)
		go func(t runner.HostTarget) {
			defer wg.Done()

			logger.Debug("Host action starting for target",
				"action", actionName,
				"server_name", t.ServerName,
				"is_remote", t.IsRemote)

			var step runner.HostCommandStep
			switch actionName {
			case "prune":
				step = runner.PruneHostStep(t)
			default:
				err := fmt.Errorf("internal error: unknown host action '%s'", actionName)
				logger.Error("Unknown host action",
					"action", actionName,
					"server_name", t.ServerName,
					"error", err)
				errChan <- err
				return
			}

			stepColor.Printf("\n--- Running Step: %s for host %s ---\n", step.Name, identifierColor.Sprint(t.ServerName))
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
				logger.Error("Host action step failed",
					"action", actionName,
					"step_name", step.Name,
					"server_name", t.ServerName,
					"is_remote", t.IsRemote,
					"error", stepErr)
				logger.Errorf("%v", err)
				errChan <- err
				return
			}

			logger.Debug("Host action completed for target",
				"action", actionName,
				"server_name", t.ServerName,
				"is_remote", t.IsRemote)
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
		logger.Error("Host action completed with errors",
			"action", actionName,
			"target_count", len(targets),
			"error_count", len(collectedErrors))
		return fmt.Errorf("%d host action(s) failed", len(collectedErrors))
	}

	logger.Info("Host action completed successfully",
		"action", actionName,
		"target_count", len(targets))
	return nil
}
