// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// runLocalCommand executes a command locally.
// It streams output based on the cliMode.
// If cliMode is true, output goes directly to os.Stdout/Stderr.
// If cliMode is false, output is sent line by line over outChan.
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
