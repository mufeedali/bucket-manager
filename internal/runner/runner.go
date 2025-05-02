// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package runner

import (
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

// sshManager is a package-level variable holding the SSH connection manager.
// This should be initialized by the calling code (CLI/TUI).
var sshManager *ssh.Manager

func InitSSHManager(manager *ssh.Manager) {
	if sshManager != nil {
		return
	}
	sshManager = manager
}

// CommandStep defines a single command to be executed for a specific project.
type CommandStep struct {
	Name    string            // User-friendly name for the step
	Command string            // The command to run (e.g., "podman")
	Args    []string          // Arguments for the command
	Project discovery.Project // The target project (local or remote)
}

// OutputLine represents a line of output from a command stream.
type OutputLine struct {
	Line    string
	IsError bool // True if the line came from stderr
}

// StreamCommand executes a command step (local or remote) and streams its stdout/stderr.
func StreamCommand(step CommandStep) (<-chan OutputLine, <-chan error) {
	outChan := make(chan OutputLine)
	errChan := make(chan error, 1)

	go func() {
		defer close(outChan)
		defer close(errChan)

		cmdDesc := fmt.Sprintf("step '%s' for project %s", step.Name, step.Project.Identifier())

		if step.Project.IsRemote {
			if sshManager == nil {
				errChan <- fmt.Errorf("ssh manager not initialized")
				return
			}
			if step.Project.HostConfig == nil {
				errChan <- fmt.Errorf("internal error: HostConfig is nil for remote project %s", step.Project.Identifier())
				return
			}

			client, err := sshManager.GetClient(*step.Project.HostConfig)
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
			if step.Project.AbsoluteRemoteRoot == "" {
				errChan <- fmt.Errorf("internal error: AbsoluteRemoteRoot is empty for remote project %s", step.Project.Identifier())
				return
			}
			remoteProjectPath := filepath.Join(step.Project.AbsoluteRemoteRoot, step.Project.Path)
			remoteCmdParts := []string{"cd", util.QuoteArgForShell(remoteProjectPath), "&&", step.Command}
			for _, arg := range step.Args {
				remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
			}
			remoteCmdString := strings.Join(remoteCmdParts, " ")

			if err := session.Start(remoteCmdString); err != nil {
				errChan <- fmt.Errorf("failed to start remote command for %s: %w", cmdDesc, err)
				return
			}

			outputDone := make(chan struct{}, 2)
			go streamPipe(stdoutPipe, outChan, outputDone, false, cmdDesc)
			go streamPipe(stderrPipe, outChan, outputDone, true, cmdDesc)

			cmdErr := session.Wait()

			<-outputDone
			<-outputDone

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

		} else {
			cmd := exec.Command(step.Command, step.Args...)
			cmd.Dir = step.Project.Path
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
			go streamPipe(stdoutPipe, outChan, outputDone, false, localCmdDesc)
			go streamPipe(stderrPipe, outChan, outputDone, true, localCmdDesc)

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

func streamPipe(pipe io.Reader, outChan chan<- OutputLine, doneChan chan<- struct{}, isError bool, cmdDesc string) {
	defer func() { doneChan <- struct{}{} }()
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		outChan <- OutputLine{Line: scanner.Text(), IsError: isError}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(io.Discard, "%s pipe scanner error for %s: %v\n", map[bool]string{false: "stdout", true: "stderr"}[isError], cmdDesc, err)
	}
}

func UpSequence(project discovery.Project) []CommandStep {
	return []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "pull"},
			Project: project,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "up", "-d"},
			Project: project,
		},
	}
}

func DownSequence(project discovery.Project) []CommandStep {
	return []CommandStep{
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "down"},
			Project: project,
		},
	}
}

func RefreshSequence(project discovery.Project) []CommandStep {
	steps := []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "pull"},
			Project: project,
		},
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "down"},
			Project: project,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "up", "-d"},
			Project: project,
		},
	}
	// Prune local system only if the project is local
	if !project.IsRemote {
		steps = append(steps, CommandStep{
			Name:    "Prune Local System",
			Command: "podman",
			Args:    []string{"system", "prune", "-af"},
			Project: project,
		})
	}
	return steps
}

type ProjectStatus string

const (
	StatusUp      ProjectStatus = "UP"
	StatusDown    ProjectStatus = "DOWN"
	StatusPartial ProjectStatus = "PARTIAL"
	StatusError   ProjectStatus = "ERROR"
	StatusUnknown ProjectStatus = "UNKNOWN"
)

type ContainerState struct {
	Name    string `json:"Name"`
	Command string `json:"Command"`
	Service string `json:"Service"`
	Status  string `json:"Status"` // e.g., "running", "exited(0)", "created"
	Ports   string `json:"Ports"`
}

// ProjectRuntimeInfo holds the status information for a project.
type ProjectRuntimeInfo struct {
	Project       discovery.Project
	OverallStatus ProjectStatus
	Containers    []ContainerState
	Error         error
}

func GetProjectStatus(project discovery.Project) ProjectRuntimeInfo {
	info := ProjectRuntimeInfo{Project: project, OverallStatus: StatusUnknown}
	cmdDesc := fmt.Sprintf("status check for project %s", project.Identifier())
	psArgs := []string{"compose", "-f", "compose.yaml", "ps", "--format", "json", "-a"}

	var output []byte
	var err error
	var stderrStr string

	if project.IsRemote {
		if sshManager == nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("ssh manager not initialized for %s", cmdDesc)
			return info
		}
		if project.HostConfig == nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("internal error: HostConfig is nil for %s", cmdDesc)
			return info
		}

		client, clientErr := sshManager.GetClient(*project.HostConfig)
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

		if project.AbsoluteRemoteRoot == "" {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("internal error: AbsoluteRemoteRoot is empty for remote project %s", project.Identifier())
			return info
		}
		remoteProjectPath := filepath.Join(project.AbsoluteRemoteRoot, project.Path)
		remoteCmdParts := []string{"cd", util.QuoteArgForShell(remoteProjectPath), "&&", "podman"}
		for _, arg := range psArgs {
			remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
		}
		remoteCmdString := strings.Join(remoteCmdParts, " ")

		output, err = session.CombinedOutput(remoteCmdString)
		// Note: CombinedOutput includes both stdout and stderr.
		// We rely on the error check below to see if the command failed.

	} else {
		cmd := exec.Command("podman", psArgs...)
		cmd.Dir = project.Path
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		err = cmd.Run()
		output = stdoutBuf.Bytes()
		stderrStr = stderrBuf.String()
	}

	// Check for errors after execution
	if err != nil {
		// Check common errors indicating the project is simply down or doesn't exist
		errMsgLower := strings.ToLower(err.Error())
		stderrLower := ""
		if !project.IsRemote { // Only rely on stderrStr if it was explicitly captured locally
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
		if !project.IsRemote && stderrStr != "" {
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
			continue // Skip empty lines
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
