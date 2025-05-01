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

// InitSSHManager allows the main application to set the SSH manager instance.
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
			// --- Remote Execution via Internal SSH Client ---
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

			// Construct the remote command string (cd && command args...)
			remoteCmdParts := []string{"cd", util.QuoteArgForShell(filepath.Join(step.Project.HostConfig.RemoteRoot, step.Project.Path)), "&&", step.Command}
			for _, arg := range step.Args {
				remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
			}
			remoteCmdString := strings.Join(remoteCmdParts, " ")

			// Start the remote command
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
			// --- Local Execution (using os/exec) ---
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

// streamPipe reads from an io.Reader (like stdout/stderr pipe) and sends lines to outChan.
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

// --- Command Sequences ---
// These now take the Project struct to associate steps with the target.

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
	// Only prune the *local* system if the project is local
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

// --- Status Logic ---

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

// GetProjectStatus retrieves the status of a project, using internal SSH client if remote.
func GetProjectStatus(project discovery.Project) ProjectRuntimeInfo {
	// Initialize info with the project itself
	info := ProjectRuntimeInfo{Project: project, OverallStatus: StatusUnknown}
	cmdDesc := fmt.Sprintf("status check for project %s", project.Identifier())
	psArgs := []string{"compose", "-f", "compose.yaml", "ps", "--format", "json", "-a"}

	var output []byte
	var err error
	var stderrStr string

	if project.IsRemote {
		// --- Remote Status via Internal SSH Client ---
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

		// Construct remote command
		remoteCmdParts := []string{"cd", util.QuoteArgForShell(filepath.Join(project.HostConfig.RemoteRoot, project.Path)), "&&", "podman"}
		for _, arg := range psArgs {
			remoteCmdParts = append(remoteCmdParts, util.QuoteArgForShell(arg))
		}
		remoteCmdString := strings.Join(remoteCmdParts, " ")

		// Run the command and capture combined output
		output, err = session.CombinedOutput(remoteCmdString)
		stderrStr = string(output)

	} else {
		// --- Local Status (using os/exec) ---
		cmd := exec.Command("podman", psArgs...)
		cmd.Dir = project.Path
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		err = cmd.Run()
		output = stdoutBuf.Bytes()
		stderrStr = stderrBuf.String()
	}

	// --- Process Results (Common for Local and Remote) ---
	stderrTrimmed := strings.TrimSpace(stderrStr)
	if err != nil {
		if strings.Contains(stderrTrimmed, "no containers found") || strings.Contains(stderrTrimmed, "no such file or directory") {
			info.OverallStatus = StatusDown
			return info
		}

		info.OverallStatus = StatusError
		errMsg := fmt.Sprintf("failed to run %s", cmdDesc)
		if stderrTrimmed != "" {
			errMsg = fmt.Sprintf("%s: %s", errMsg, stderrTrimmed)
		}
		info.Error = fmt.Errorf("%s: %w", errMsg, err)
		return info
	}

	if len(bytes.TrimSpace(output)) == 0 {
		info.OverallStatus = StatusDown
		return info
	}

	// Decode the JSON output
	var containers []ContainerState
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var container ContainerState
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &container); err != nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("failed to decode container status JSON line '%s': %w", string(line), err)
			return info
		}
		containers = append(containers, container)
	}
	if err := scanner.Err(); err != nil {
		info.OverallStatus = StatusError
		info.Error = fmt.Errorf("error reading container status output: %w", err)
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
