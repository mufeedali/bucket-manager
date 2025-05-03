// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package runner

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/ssh"
	"bucket-manager/internal/util"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"os/exec"
	"path/filepath"

	"strings"
	"syscall"

	gossh "golang.org/x/crypto/ssh"
)

var sshManager *ssh.Manager

// InitSSHManager sets the package-level SSH manager instance.
func InitSSHManager(manager *ssh.Manager) {
	if sshManager != nil {
		return
	}
	sshManager = manager
}

type CommandStep struct {
	Name    string
	Command string
	Args    []string
	Stack   discovery.Stack
}

type OutputLine struct {
	Line    string
	IsError bool // True if the line came from stderr
}

// HostTarget defines the target for a host-level command (local or a specific remote).
type HostTarget struct {
	IsRemote   bool
	HostConfig *config.SSHHost // Only set if IsRemote is true
	ServerName string          // "local" or the remote server name
}

// HostCommandStep defines a command to be run on a host, not within a specific stack directory.
type HostCommandStep struct {
	Name    string
	Command string
	Args    []string
	Target  HostTarget
}

// RunHostCommand executes a command directly on a target host (local or remote).
// It streams output and returns an error channel, similar to StreamCommand.
func RunHostCommand(step HostCommandStep) (<-chan OutputLine, <-chan error) {
	outChan := make(chan OutputLine)
	errChan := make(chan error, 1)

	go func() {
		defer close(outChan)
		defer close(errChan)

		cmdDesc := fmt.Sprintf("step '%s' for host %s", step.Name, step.Target.ServerName)

		if step.Target.IsRemote {
			if sshManager == nil {
				errChan <- fmt.Errorf("ssh manager not initialized for %s", cmdDesc)
				return
			}
			if step.Target.HostConfig == nil {
				errChan <- fmt.Errorf("internal error: HostConfig is nil for remote host %s", step.Target.ServerName)
				return
			}

			client, err := sshManager.GetClient(*step.Target.HostConfig)
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

			// Construct the remote command string (command args...) - No cd needed for host commands
			remoteCmdParts := []string{step.Command}
			for _, arg := range step.Args {
				remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
			}
			remoteCmdString := strings.Join(remoteCmdParts, " ")

			if err := session.Start(remoteCmdString); err != nil {
				errChan <- fmt.Errorf("failed to start remote command for %s: %w", cmdDesc, err)
				return
			}

			outputDone := make(chan struct{}, 2) // Wait for both stdout and stderr streams
			go streamPipe(stdoutPipe, outChan, outputDone, false)
			go streamPipe(stderrPipe, outChan, outputDone, true)

			cmdErr := session.Wait() // Wait for the remote command to finish

			// Wait for both pipe streaming goroutines to finish processing
			<-outputDone
			<-outputDone

			if cmdErr != nil {
				exitCode := -1
				// Try to extract the exit code from the SSH error
				if exitErr, ok := cmdErr.(*gossh.ExitError); ok {
					exitCode = exitErr.ExitStatus()
				}
				// Provide a more informative error message including the exit code if available
				if exitCode != -1 {
					errChan <- fmt.Errorf("%s exited with status %d: %w", cmdDesc, exitCode, cmdErr)
				} else {
					errChan <- fmt.Errorf("%s failed: %w", cmdDesc, cmdErr) // General failure
				}
				return // Signal error
			}
			// Command succeeded remotely
		} else {
			// --- Local Execution ---
			cmd := exec.Command(step.Command, step.Args...)
			// cmd.Dir is not set, run in the default working directory
			localCmdDesc := fmt.Sprintf("local %s", cmdDesc)

			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				errChan <- fmt.Errorf("failed to get stdout pipe for %s: %w", localCmdDesc, err)
				return
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				errChan <- fmt.Errorf("failed to get stderr pipe for %s: %w", localCmdDesc, err)
				return
			}

			if err := cmd.Start(); err != nil {
				errChan <- fmt.Errorf("failed to start %s: %w", localCmdDesc, err)
				return
			}

			outputDone := make(chan struct{}, 2)
			go streamPipe(stdoutPipe, outChan, outputDone, false)
			go streamPipe(stderrPipe, outChan, outputDone, true)

			cmdErr := cmd.Wait()

			<-outputDone
			<-outputDone

			if cmdErr != nil {
				exitCode := -1
				if exitError, ok := cmdErr.(*exec.ExitError); ok {
					if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
						exitCode = status.ExitStatus()
					}
				}
				if exitCode != -1 {
					errChan <- fmt.Errorf("%s exited with status %d: %w", localCmdDesc, exitCode, cmdErr)
				} else {
					errChan <- fmt.Errorf("%s failed: %w", localCmdDesc, cmdErr)
				}
				return
			}
		}
	}()

	return outChan, errChan
}

func streamPipe(pipe io.Reader, outChan chan<- OutputLine, doneChan chan<- struct{}, isError bool) {
	defer func() { doneChan <- struct{}{} }()
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		outChan <- OutputLine{Line: scanner.Text(), IsError: isError}
	}
	if err := scanner.Err(); err != nil {
		// Use io.Discard to avoid printing errors directly if not needed,
		// but log them internally or handle them appropriately if debugging is required.
		// fmt.Fprintf(os.Stderr, "%s pipe scanner error for %s: %v\n", map[bool]string{false: "stdout", true: "stderr"}[isError], cmdDesc, err)
		_ = err // Avoid unused variable error if not logging
	}
}

// StreamCommand executes a sequence of commands within a specific stack's context.
func StreamCommand(step CommandStep) (<-chan OutputLine, <-chan error) {
	outChan := make(chan OutputLine)
	errChan := make(chan error, 1)

	go func() {
		defer close(outChan)
		defer close(errChan)

		cmdDesc := fmt.Sprintf("step '%s' for stack %s", step.Name, step.Stack.Identifier())

		if step.Stack.IsRemote {
			if sshManager == nil {
				errChan <- fmt.Errorf("ssh manager not initialized for %s", cmdDesc)
				return
			}
			if step.Stack.HostConfig == nil {
				errChan <- fmt.Errorf("internal error: HostConfig is nil for remote stack %s", step.Stack.Identifier())
				return
			}

			client, err := sshManager.GetClient(*step.Stack.HostConfig)
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

			// Construct the remote command string (cd /resolved/absolute/root/relative/path && command args...)
			if step.Stack.AbsoluteRemoteRoot == "" {
				errChan <- fmt.Errorf("internal error: AbsoluteRemoteRoot is empty for remote stack %s", step.Stack.Identifier())
				return
			}
			remoteStackPath := filepath.Join(step.Stack.AbsoluteRemoteRoot, step.Stack.Path)
			remoteCmdParts := []string{"cd", util.QuoteArgForShell(remoteStackPath), "&&", step.Command}
			for _, arg := range step.Args {
				remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
			}
			remoteCmdString := strings.Join(remoteCmdParts, " ")

			if err := session.Start(remoteCmdString); err != nil {
				errChan <- fmt.Errorf("failed to start remote command for %s: %w", cmdDesc, err)
				return
			}

			outputDone := make(chan struct{}, 2) // Wait for both stdout and stderr streams
			go streamPipe(stdoutPipe, outChan, outputDone, false)
			go streamPipe(stderrPipe, outChan, outputDone, true)

			cmdErr := session.Wait() // Wait for the remote command to finish

			// Wait for both pipe streaming goroutines to finish processing
			<-outputDone
			<-outputDone

			if cmdErr != nil {
				exitCode := -1
				// Try to extract the exit code from the SSH error
				if exitErr, ok := cmdErr.(*gossh.ExitError); ok {
					exitCode = exitErr.ExitStatus()
				}
				// Provide a more informative error message including the exit code if available
				if exitCode != -1 {
					errChan <- fmt.Errorf("%s exited with status %d: %w", cmdDesc, exitCode, cmdErr)
				} else {
					errChan <- fmt.Errorf("%s failed: %w", cmdDesc, cmdErr) // General failure
				}
				return // Signal error
			}
			// Command succeeded remotely
		} else {
			// --- Local Execution ---
			cmd := exec.Command(step.Command, step.Args...)
			cmd.Dir = step.Stack.Path // Run in the stack's directory
			localCmdDesc := fmt.Sprintf("local %s", cmdDesc)

			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				errChan <- fmt.Errorf("failed to get stdout pipe for %s: %w", localCmdDesc, err)
				return
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				errChan <- fmt.Errorf("failed to get stderr pipe for %s: %w", localCmdDesc, err)
				return
			}

			if err := cmd.Start(); err != nil {
				errChan <- fmt.Errorf("failed to start %s: %w", localCmdDesc, err)
				return
			}

			outputDone := make(chan struct{}, 2)
			go streamPipe(stdoutPipe, outChan, outputDone, false)
			go streamPipe(stderrPipe, outChan, outputDone, true)

			cmdErr := cmd.Wait()

			<-outputDone
			<-outputDone

			if cmdErr != nil {
				exitCode := -1
				if exitError, ok := cmdErr.(*exec.ExitError); ok {
					if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
						exitCode = status.ExitStatus()
					}
				}
				if exitCode != -1 {
					errChan <- fmt.Errorf("%s exited with status %d: %w", localCmdDesc, exitCode, cmdErr)
				} else {
					errChan <- fmt.Errorf("%s failed: %w", localCmdDesc, cmdErr)
				}
				return
			}
		}
	}()

	return outChan, errChan
}

func UpSequence(stack discovery.Stack) []CommandStep {
	return []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "pull"}, // Remove -f compose.yaml
			Stack:   stack,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "up", "-d"}, // Remove -f compose.yaml
			Stack:   stack,
		},
	}
}
func PullSequence(stack discovery.Stack) []CommandStep {
	return []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "pull"}, // Remove -f compose.yaml
			Stack:   stack,
		},
	}
}

func DownSequence(stack discovery.Stack) []CommandStep {
	return []CommandStep{
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "down"}, // Remove -f compose.yaml
			Stack:   stack,
		},
	}
}

func RefreshSequence(stack discovery.Stack) []CommandStep {
	steps := []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "pull"}, // Remove -f compose.yaml
			Stack:   stack,
		},
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "down"}, // Remove -f compose.yaml
			Stack:   stack,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "up", "-d"}, // Remove -f compose.yaml
			Stack:   stack,
		},
	}
	// Prune local system only if the stack is local
	if !stack.IsRemote {
		steps = append(steps, CommandStep{
			Name:    "Prune Local System",
			Command: "podman",
			Args:    []string{"system", "prune", "-af"},
			Stack:   stack,
		})
	}
	return steps
}

// PruneHostStep creates a command step to prune the Podman system on a target host.
func PruneHostStep(target HostTarget) HostCommandStep {
	return HostCommandStep{
		Name:    "Prune System",
		Command: "podman",
		Args:    []string{"system", "prune", "-af"},
		Target:  target,
	}
}

type StackStatus string

const (
	StatusUp      StackStatus = "UP"
	StatusDown    StackStatus = "DOWN"
	StatusPartial StackStatus = "PARTIAL"
	StatusError   StackStatus = "ERROR"
	StatusUnknown StackStatus = "UNKNOWN"
)

type ContainerState struct {
	Name    string `json:"Name"`
	Command string `json:"Command"`
	Service string `json:"Service"`
	Status  string `json:"Status"` // e.g., "running", "exited(0)", "created"
	Ports   string `json:"Ports"`
}

// StackRuntimeInfo holds the status information for a stack.
type StackRuntimeInfo struct {
	Stack         discovery.Stack
	OverallStatus StackStatus
	Containers    []ContainerState
	Error         error
}

func GetStackStatus(stack discovery.Stack) StackRuntimeInfo {
	info := StackRuntimeInfo{Stack: stack, OverallStatus: StatusUnknown}
	cmdDesc := fmt.Sprintf("status check for stack %s", stack.Identifier())
	// Remove -f compose.yaml, let podman-compose find compose.yaml or compose.yml
	psArgs := []string{"compose", "ps", "--format", "json", "-a"}

	var output []byte
	var err error
	var stderrStr string

	if stack.IsRemote {
		if sshManager == nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("ssh manager not initialized for %s", cmdDesc)
			return info
		}
		if stack.HostConfig == nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("internal error: HostConfig is nil for %s", cmdDesc)
			return info
		}

		client, clientErr := sshManager.GetClient(*stack.HostConfig)
		if clientErr != nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("failed to get ssh client for %s: %w", cmdDesc, clientErr)
			return info
		}

		session, sessionErr := client.NewSession()
		if sessionErr != nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("failed to create ssh session for %s: %w", cmdDesc, sessionErr)
			return info
		}
		defer session.Close()

		if stack.AbsoluteRemoteRoot == "" {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("internal error: AbsoluteRemoteRoot is empty for remote stack %s", stack.Identifier())
			return info
		}
		remoteStackPath := filepath.Join(stack.AbsoluteRemoteRoot, stack.Path)
		remoteCmdParts := []string{"cd", util.QuoteArgForShell(remoteStackPath), "&&", "podman"}
		for _, arg := range psArgs {
			remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
		}
		remoteCmdString := strings.Join(remoteCmdParts, " ")

		// Use CombinedOutput for status check as it's typically short
		output, err = session.CombinedOutput(remoteCmdString)
		// CombinedOutput returns stdout and stderr combined. Error checking below handles failures.

	} else {
		// --- Local Status Check ---
		cmd := exec.Command("podman", psArgs...)
		cmd.Dir = stack.Path
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		err = cmd.Run()
		output = stdoutBuf.Bytes()
		stderrStr = stderrBuf.String()
	}

	// Check for errors after execution
	if err != nil {
		// Check common errors indicating the stack is simply down or doesn't exist
		errMsgLower := strings.ToLower(err.Error())
		stderrLower := ""
		if !stack.IsRemote { // Only rely on stderrStr if it was explicitly captured locally
			stderrLower = strings.ToLower(stderrStr)
		} else { // For remote, check the combined output string as stderr isn't separate
			stderrLower = strings.ToLower(string(output))
		}

		if strings.Contains(errMsgLower, "exit status") || // Generic exit error
			strings.Contains(stderrLower, "no containers found") || // Podman compose message
			strings.Contains(stderrLower, "no such file or directory") { // If compose file is missing
			info.OverallStatus = StatusDown
			return info // Not a failure, just down.
		}

		// Otherwise, it's a real error
		info.OverallStatus = StatusError
		errMsg := fmt.Sprintf("failed to run %s", cmdDesc)
		// Append stderr from local execution if available and provides context
		if !stack.IsRemote && stderrStr != "" {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(stderrStr))
		}
		info.Error = fmt.Errorf("%s: %w", errMsg, err) // Wrap original error
		return info
	}

	if len(bytes.TrimSpace(output)) == 0 {
		info.OverallStatus = StatusDown
		return info
	}

	var containers []ContainerState
	var firstUnmarshalError error

	// Process the output line by line, as podman compose ps --format json outputs a stream
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		if len(bytes.TrimSpace(lineBytes)) == 0 {
			continue
		}

		var container ContainerState
		if errUnmarshal := json.Unmarshal(lineBytes, &container); errUnmarshal != nil {
			// Store the first error encountered, but continue trying to parse other lines
			if firstUnmarshalError == nil {
				firstUnmarshalError = fmt.Errorf("failed to decode container status JSON line: %w\nLine: %s", errUnmarshal, string(lineBytes))
			}
			continue // Skip lines that fail to parse
		}
		containers = append(containers, container)
	}

	// Check for scanner errors
	if errScan := scanner.Err(); errScan != nil {
		if firstUnmarshalError == nil { // Prioritize unmarshal errors over scan errors
			firstUnmarshalError = fmt.Errorf("error scanning command output: %w", errScan)
		}
	}

	// If any line failed to unmarshal, report the error
	if firstUnmarshalError != nil {
		// Check if the error might be the "no containers found" case by examining the raw output
		outputLower := strings.ToLower(string(output))
		if strings.Contains(outputLower, "no containers found") {
			info.OverallStatus = StatusDown
			return info
		}
		// Otherwise, report the parsing error
		info.OverallStatus = StatusError
		info.Error = firstUnmarshalError // Use the stored first error
		return info
	}

	info.Containers = containers

	if len(containers) == 0 {
		info.OverallStatus = StatusDown
		return info
	}

	allRunning := true
	anyRunning := false
	for _, c := range containers {
		isRunning := strings.Contains(strings.ToLower(c.Status), "running") || strings.Contains(strings.ToLower(c.Status), "healthy") || strings.HasPrefix(c.Status, "Up")
		if isRunning {
			anyRunning = true
		} else {
			allRunning = false
		}
	}

	if allRunning {
		info.OverallStatus = StatusUp
	} else if anyRunning {
		info.OverallStatus = StatusPartial
	} else {
		info.OverallStatus = StatusDown
	}

	return info
}
