// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package runner's local.go file implements functions for executing commands
// on the local system. This includes running podman compose commands within stack
// directories and system-level commands like prune operations.

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// runLocalCommand executes a command locally on the host system.
// It provides two output modes based on the cliMode parameter:
// - If cliMode is true: output is sent directly to os.Stdout/Stderr (terminal)
// - If cliMode is false: output is captured and sent through channels for TUI/API use
//
// Parameters:
//   - cmd: The prepared exec.Cmd to execute
//   - cmdDesc: Description of the command for error messages
//   - cliMode: Whether to use direct terminal output or channel-based output
//   - outChan: Channel to send command output lines
//   - errChan: Channel to send execution errors
func runLocalCommand(cmd *exec.Cmd, cmdDesc string, cliMode bool, outChan chan<- OutputLine, errChan chan<- error) {
	var cmdErr error
	if cliMode {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			errChan <- fmt.Errorf("failed to start %s: %w", cmdDesc, err)
			return
		}
		cmdErr = cmd.Wait()
	} else {
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			errChan <- fmt.Errorf("failed to get stdout pipe for %s: %w", cmdDesc, err)
			return
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			errChan <- fmt.Errorf("failed to get stderr pipe for %s: %w", cmdDesc, err)
			return
		}

		if err := cmd.Start(); err != nil {
			errChan <- fmt.Errorf("failed to start %s: %w", cmdDesc, err)
			return
		}

		outputDone := make(chan struct{}, 2) // Wait for both streamPipe goroutines
		go streamPipe(stdoutPipe, outChan, outputDone, false)
		go streamPipe(stderrPipe, outChan, outputDone, true)

		cmdErr = cmd.Wait()

		// Wait for pipe readers to finish *after* command Wait returns
		<-outputDone
		<-outputDone
	}

	if cmdErr != nil {
		exitCode := -1
		if exitError, ok := cmdErr.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		}
		if exitCode != -1 {
			errChan <- fmt.Errorf("%s exited with status %d: %w", cmdDesc, exitCode, cmdErr)
		} else {
			errChan <- fmt.Errorf("%s failed: %w", cmdDesc, cmdErr)
		}
		return
	}
}
