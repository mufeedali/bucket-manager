// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package runner

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/util"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	gossh "golang.org/x/crypto/ssh"
)

// runSSHCommand executes a command remotely via SSH.
// It streams output based on the cliMode.
// If cliMode is true, output goes directly to os.Stdout/Stderr.
// If cliMode is false, output is sent line by line over outChan.
func runSSHCommand(
	hostConfig config.SSHHost,
	remoteCmdString string,
	cmdDesc string,
	cliMode bool,
	outChan chan<- OutputLine,
	errChan chan<- error,
) {
	if sshManager == nil {
		errChan <- fmt.Errorf("ssh manager not initialized for %s", cmdDesc)
		return
	}

	client, err := sshManager.GetClient(hostConfig)
	if err != nil {
		errChan <- fmt.Errorf("failed to get ssh client for %s: %w", cmdDesc, err)
		return
	}

	session, err := client.NewSession()
	if err != nil {
		errChan <- fmt.Errorf("failed to create ssh session for %s: %w", cmdDesc, err)
		return
	}
	defer session.Close()

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		errChan <- fmt.Errorf("failed to get ssh stdout pipe for %s: %w", cmdDesc, err)
		return
	}
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		errChan <- fmt.Errorf("failed to get ssh stderr pipe for %s: %w", cmdDesc, err)
		return
	}

	// Request a PTY for interactive commands like podman compose (enables color)
	// Only do this if in CLI mode.
	if cliMode {
		// Use sensible defaults for terminal type and size.
		modes := gossh.TerminalModes{
			gossh.ECHO:          0,     // Disable echoing input
			gossh.TTY_OP_ISPEED: 14400, // Input speed = 14.4kbaud
			gossh.TTY_OP_OSPEED: 14400, // Output speed = 14.4kbaud
		}
		// Use a common terminal type like xterm-256color
		if err := session.RequestPty("xterm-256color", 80, 40, modes); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to request pty for %s (continuing): %v\n", cmdDesc, err)
		}
	}

	if err := session.Start(remoteCmdString); err != nil {
		errChan <- fmt.Errorf("failed to start remote command for %s: %w", cmdDesc, err)
		return
	}

	var cmdErr error
	if cliMode {
		// Use io.Copy to directly pipe remote output to local stdout/stderr.
		// This handles TTY output correctly (colors, \r).
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			_, _ = io.Copy(os.Stdout, stdoutPipe)
		}()
		go func() {
			defer wg.Done()
			// Send remote stderr to local stderr for CLI mode
			_, _ = io.Copy(os.Stderr, stderrPipe)
		}()

		cmdErr = session.Wait()
		wg.Wait()
	} else {
		outputDone := make(chan struct{}, 2)

		go streamPipe(stdoutPipe, outChan, outputDone, false)
		go streamPipe(stderrPipe, outChan, outputDone, true)

		cmdErr = session.Wait()

		// Wait for pipe readers to finish *after* command Wait returns
		<-outputDone
		<-outputDone
	}

	if cmdErr != nil {
		exitCode := -1
		if exitErr, ok := cmdErr.(*gossh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		}
		if exitCode != -1 {
			errChan <- fmt.Errorf("%s exited with status %d: %w", cmdDesc, exitCode, cmdErr)
		} else {
			errChan <- fmt.Errorf("%s failed: %w", cmdDesc, cmdErr)
		}
		return
	}
}

// runSSHStatusCheck executes 'podman compose ps' remotely via SSH and returns the combined output.
func runSSHStatusCheck(stack discovery.Stack, psArgs []string, cmdDesc string) ([]byte, error) {
	if sshManager == nil {
		return nil, fmt.Errorf("ssh manager not initialized for %s", cmdDesc)
	}
	if stack.HostConfig == nil {
		return nil, fmt.Errorf("internal error: HostConfig is nil for %s", cmdDesc)
	}

	client, clientErr := sshManager.GetClient(*stack.HostConfig)
	if clientErr != nil {
		return nil, fmt.Errorf("failed to get ssh client for %s: %w", cmdDesc, clientErr)
	}

	session, sessionErr := client.NewSession()
	if sessionErr != nil {
		return nil, fmt.Errorf("failed to create ssh session for %s: %w", cmdDesc, sessionErr)
	}
	defer session.Close()

	if stack.AbsoluteRemoteRoot == "" {
		return nil, fmt.Errorf("internal error: AbsoluteRemoteRoot is empty for remote stack %s", stack.Identifier())
	}
	remoteStackPath := filepath.Join(stack.AbsoluteRemoteRoot, stack.Path)
	remoteCmdParts := []string{"cd", util.QuoteArgForShell(remoteStackPath), "&&", "podman"}
	for _, arg := range psArgs {
		remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
	}
	remoteCmdString := strings.Join(remoteCmdParts, " ")

	// CombinedOutput is suitable for short status checks.
	output, err := session.CombinedOutput(remoteCmdString)
	if err != nil {
		return output, fmt.Errorf("remote command failed for %s: %w", cmdDesc, err)
	}
	return output, nil
}
