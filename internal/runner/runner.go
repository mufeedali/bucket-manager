// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package runner provides functionality for executing commands on local and remote hosts.
// It handles command execution, output streaming, and error handling for Podman Compose
// operations across both local and SSH-connected remote environments.
package runner

import (
	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/ssh"
	"bucket-manager/internal/util"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sshManager is a package-level reference to the SSH connection manager
// Used by both runner.go and ssh.go to access SSH connections
var sshManager *ssh.Manager

// InitSSHManager sets the package-level SSH manager instance.
func InitSSHManager(manager *ssh.Manager) {
	if sshManager != nil {
		return
	}
	sshManager = manager
}

// CommandStep represents a single command to be executed within a stack's directory
// Used for operations like 'podman compose up', 'podman compose down', etc.
type CommandStep struct {
	Name    string          // User-friendly name/description of the command
	Command string          // The executable command (e.g., 'podman')
	Args    []string        // Command arguments (e.g., ['compose', 'up', '-d'])
	Stack   discovery.Stack // The target stack where the command will be executed
}

// OutputLine represents a single line of command output with its source indicator
// Used to differentiate between stdout and stderr when presenting command output
type OutputLine struct {
	Line    string // The actual output text
	IsError bool   // True if the line came from stderr, false if from stdout
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
// It streams output based on the cliMode.
// If cliMode is true, output goes directly to os.Stdout/Stderr.
// If cliMode is false, output is sent line by line over outChan.
func RunHostCommand(step HostCommandStep, cliMode bool) (<-chan OutputLine, <-chan error) {
	// Buffer channel slightly for TUI mode to prevent blocking on rapid output
	outChan := make(chan OutputLine, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(outChan)
		defer close(errChan)

		cmdDesc := fmt.Sprintf("step '%s' for host %s", step.Name, step.Target.ServerName)

		if step.Target.IsRemote {
			if step.Target.HostConfig == nil {
				errChan <- fmt.Errorf("internal error: HostConfig is nil for remote host %s", step.Target.ServerName)
				return
			}
			// Construct the remote command string (command args...) - No cd needed for host commands
			remoteCmdParts := []string{step.Command}
			for _, arg := range step.Args {
				remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
			}
			remoteCmdString := strings.Join(remoteCmdParts, " ")

			runSSHCommand(*step.Target.HostConfig, remoteCmdString, cmdDesc, cliMode, outChan, errChan)
		} else {
			cmd := exec.Command(step.Command, step.Args...)
			// cmd.Dir is not set for host commands, run in the default working directory
			localCmdDesc := fmt.Sprintf("local %s", cmdDesc)

			runLocalCommand(cmd, localCmdDesc, cliMode, outChan, errChan)
		}
	}()

	return outChan, errChan
}

// streamPipe reads raw chunks from the pipe and sends them over the outChan.
// This is used for TUI mode where raw output (including control characters) is needed.
func streamPipe(pipe io.Reader, outChan chan<- OutputLine, doneChan chan<- struct{}, isError bool) {
	defer func() { doneChan <- struct{}{} }()
	buf := make([]byte, 1024)
	for {
		n, err := pipe.Read(buf)
		if n > 0 {
			outChan <- OutputLine{Line: string(buf[:n]), IsError: isError}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				fmt.Fprintf(os.Stderr, "Pipe read error (%v): %v\n", isError, err)
			}
			break
		}
	}
}

// StreamCommand executes a sequence of commands within a specific stack's context.
// It streams output based on the cliMode.
// If cliMode is true, output goes directly to os.Stdout/Stderr.
// If cliMode is false, output is sent line by line over outChan.
func StreamCommand(step CommandStep, cliMode bool) (<-chan OutputLine, <-chan error) {
	// Buffer channel slightly for TUI mode to prevent blocking on rapid output
	outChan := make(chan OutputLine, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(outChan)
		defer close(errChan)

		cmdDesc := fmt.Sprintf("step '%s' for stack %s", step.Name, step.Stack.Identifier())

		if step.Stack.IsRemote {
			if step.Stack.HostConfig == nil {
				errChan <- fmt.Errorf("internal error: HostConfig is nil for remote stack %s", step.Stack.Identifier())
				return
			}
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

			runSSHCommand(*step.Stack.HostConfig, remoteCmdString, cmdDesc, cliMode, outChan, errChan)
		} else {
			cmd := exec.Command(step.Command, step.Args...)
			cmd.Dir = step.Stack.Path
			localCmdDesc := fmt.Sprintf("local %s", cmdDesc)

			runLocalCommand(cmd, localCmdDesc, cliMode, outChan, errChan)
		}
	}()

	return outChan, errChan
}

func UpSequence(stack discovery.Stack) []CommandStep {
	return []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "pull"},
			Stack:   stack,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "up", "-d"},
			Stack:   stack,
		},
	}
}
func PullSequence(stack discovery.Stack) []CommandStep {
	return []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "pull"},
			Stack:   stack,
		},
	}
}

func DownSequence(stack discovery.Stack) []CommandStep {
	return []CommandStep{
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "down"},
			Stack:   stack,
		},
	}
}

func RefreshSequence(stack discovery.Stack) []CommandStep {
	steps := []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "pull"},
			Stack:   stack,
		},
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "down"},
			Stack:   stack,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "up", "-d"},
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

// parseContainerStatusOutput processes the JSON stream output from 'podman compose ps'.
func parseContainerStatusOutput(output []byte) ([]ContainerState, error) {
	var containers []ContainerState
	var firstUnmarshalError error

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
			continue
		}
		containers = append(containers, container)
	}

	if errScan := scanner.Err(); errScan != nil {
		if firstUnmarshalError == nil { // Prioritize unmarshal errors over scan errors
			firstUnmarshalError = fmt.Errorf("error scanning command output: %w", errScan)
		}
	}

	// Check if the error might be the "no containers found" case by examining the raw output
	// This check is done here because even if parsing fails, the output might indicate no containers.
	if firstUnmarshalError != nil {
		outputLower := strings.ToLower(string(output))
		if strings.Contains(outputLower, "no containers found") {
			// Treat "no containers found" as a successful parse yielding zero containers, clear the error.
			return []ContainerState{}, nil
		}
		return nil, firstUnmarshalError
	}

	return containers, nil
}

// aggregateOverallStatus determines the overall stack status based on container states.
func aggregateOverallStatus(containers []ContainerState) StackStatus {
	if len(containers) == 0 {
		return StatusDown
	}

	allRunning := true
	anyRunning := false
	for _, c := range containers {
		// Consider variations in status strings (case-insensitive)
		statusLower := strings.ToLower(c.Status)
		isRunning := strings.Contains(statusLower, "running") ||
			strings.Contains(statusLower, "healthy") ||
			strings.HasPrefix(statusLower, "up")

		if isRunning {
			anyRunning = true
		} else {
			allRunning = false
		}
		// Optimization: if we find one running and one not running, it's Partial
		if anyRunning && !allRunning {
			return StatusPartial
		}
	}

	if allRunning {
		return StatusUp
	}
	// If loop finishes and !allRunning, but anyRunning was true, it's Partial (handled above)
	// If loop finishes and !allRunning and !anyRunning, it means all are down/stopped/exited.
	if !anyRunning {
		return StatusDown
	}

	// Fallback/Should not happen if logic above is correct
	return StatusUnknown
}

func GetStackStatus(stack discovery.Stack) StackRuntimeInfo {
	info := StackRuntimeInfo{Stack: stack, OverallStatus: StatusUnknown}
	cmdDesc := fmt.Sprintf("status check for stack %s", stack.Identifier())
	psArgs := []string{"compose", "ps", "--format", "json", "-a"}

	var output []byte
	var cmdErr error
	var stderrStr string // Only relevant for local execution

	// 1. Execute command (local or remote)
	if stack.IsRemote {
		output, cmdErr = runSSHStatusCheck(stack, psArgs, cmdDesc)
		// runSSHStatusCheck returns combined output and the command error
	} else {
		cmd := exec.Command("podman", psArgs...)
		cmd.Dir = stack.Path
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		cmdErr = cmd.Run()
		output = stdoutBuf.Bytes()
		stderrStr = stderrBuf.String() // Capture stderr for local
	}

	// 2. Handle command execution errors
	if cmdErr != nil {
		// Check common errors indicating the stack is simply down or doesn't exist
		errMsgLower := strings.ToLower(cmdErr.Error())
		// Check stderr for local, combined output for remote
		outputToCheck := stderrStr
		if stack.IsRemote {
			outputToCheck = string(output)
		}
		outputToCheckLower := strings.ToLower(outputToCheck)

		if strings.Contains(errMsgLower, "exit status") ||
			strings.Contains(outputToCheckLower, "no containers found") ||
			strings.Contains(outputToCheckLower, "no such file or directory") { // If compose file is missing
			info.OverallStatus = StatusDown
			return info // Not a failure, just down.
		}

		// It's a real command execution error
		info.OverallStatus = StatusError
		errMsg := fmt.Sprintf("failed to run %s", cmdDesc)
		// Append stderr from local execution if available and provides context
		if !stack.IsRemote && stderrStr != "" {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(stderrStr))
		}
		info.Error = fmt.Errorf("%s: %w", errMsg, cmdErr)
		return info
	}

	// 3. Handle empty output (implies down)
	if len(bytes.TrimSpace(output)) == 0 {
		info.OverallStatus = StatusDown
		return info
	}

	// 4. Parse the output
	containers, parseErr := parseContainerStatusOutput(output)
	if parseErr != nil {
		info.OverallStatus = StatusError
		info.Error = parseErr // Use the specific parsing error
		return info
	}

	// 5. Aggregate status from parsed containers
	info.Containers = containers
	info.OverallStatus = aggregateOverallStatus(containers)

	return info
}
