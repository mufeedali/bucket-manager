package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
)

// CommandStep defines a single command to be executed.
type CommandStep struct {
	Name    string   // User-friendly name for the step
	Command string   // The command to run (e.g., "podman")
	Args    []string // Arguments for the command
	Dir     string   // Directory to run the command in (optional)
}

// OutputLine represents a line of output from a command stream.
type OutputLine struct {
	Line    string
	IsError bool // True if the line came from stderr
}

// StreamCommand executes a command step and streams its stdout and stderr.
// It returns a channel for output lines and a channel for the final error.
func StreamCommand(step CommandStep) (<-chan OutputLine, <-chan error) {
	outChan := make(chan OutputLine)
	errChan := make(chan error, 1) // Buffered channel for the final error

	go func() {
		defer close(outChan)
		defer close(errChan)

		cmd := exec.Command(step.Command, step.Args...)
		if step.Dir != "" {
			cmd.Dir = step.Dir
		}

		// Get pipes for stdout and stderr
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			errChan <- fmt.Errorf("failed to get stdout pipe for step '%s': %w", step.Name, err)
			return
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			errChan <- fmt.Errorf("failed to get stderr pipe for step '%s': %w", step.Name, err)
			return
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			errChan <- fmt.Errorf("failed to start command for step '%s': %w", step.Name, err)
			return
		}

		// Use a WaitGroup to wait for both streams to finish
		// var wg sync.WaitGroup // Not needed as we read until EOF
		outputDone := make(chan struct{}, 2) // Signal channel for stream completion

		// Goroutine to read stdout
		go func() {
			defer func() { outputDone <- struct{}{} }() // Signal completion
			scanner := bufio.NewScanner(stdoutPipe)
			for scanner.Scan() {
				outChan <- OutputLine{Line: scanner.Text(), IsError: false}
			}
			if err := scanner.Err(); err != nil {
				// Log scanner error, but don't send it as the primary command error
				fmt.Fprintf(io.Discard, "stdout scanner error for step '%s': %v\n", step.Name, err) // Discard for now
			}
		}()

		// Goroutine to read stderr
		go func() {
			defer func() { outputDone <- struct{}{} }() // Signal completion
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				outChan <- OutputLine{Line: scanner.Text(), IsError: true}
			}
			if err := scanner.Err(); err != nil {
				// Log scanner error
				fmt.Fprintf(io.Discard, "stderr scanner error for step '%s': %v\n", step.Name, err) // Discard for now
			}
		}()

		// Wait for the command to finish
		cmdErr := cmd.Wait()

		// Wait for both stream readers to finish before closing channels/returning
		<-outputDone
		<-outputDone

		// Send the final command error (if any)
		if cmdErr != nil {
			// Try to get more specific exit code if possible
			if exitError, ok := cmdErr.(*exec.ExitError); ok {
				if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
					errChan <- fmt.Errorf("command exited with status %d: %w", status.ExitStatus(), cmdErr)
					return
				}
			}
			errChan <- fmt.Errorf("command failed: %w", cmdErr)
			return
		}

		// If no error, signal success by closing errChan without sending anything
		// The receiver will get a zero value (nil) when reading from a closed channel.
	}()

	return outChan, errChan
}

// --- Command Sequences ---

// UpSequence defines the steps for the 'up' action.
func UpSequence(projectPath string) []CommandStep {
	return []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "pull"}, // Assuming compose.yaml, adjust if needed
			Dir:     projectPath,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "up", "-d"},
			Dir:     projectPath,
		},
	}
}

// DownSequence defines the steps for the 'down' action.
func DownSequence(projectPath string) []CommandStep {
	return []CommandStep{
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "down"},
			Dir:     projectPath,
		},
	}
}

// RefreshSequence defines the steps for the 'refresh' action.
func RefreshSequence(projectPath string) []CommandStep {
	return []CommandStep{
		{
			Name:    "Pull Images",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "pull"},
			Dir:     projectPath,
		},
		{
			Name:    "Stop Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "down"},
			Dir:     projectPath,
		},
		{
			Name:    "Start Containers",
			Command: "podman",
			Args:    []string{"compose", "-f", "compose.yaml", "up", "-d"},
			Dir:     projectPath,
		},
		// Optional: Add a prune step if desired
		{
			Name:    "Prune System",
			Command: "podman",
			Args:    []string{"system", "prune", "-af"},
			// No Dir needed, system-wide command
		},
	}
}

// --- Status Logic ---

// ProjectStatus represents the overall status of a compose project.
type ProjectStatus string

const (
	StatusUp      ProjectStatus = "UP"
	StatusDown    ProjectStatus = "DOWN"
	StatusPartial ProjectStatus = "PARTIAL"
	StatusError   ProjectStatus = "ERROR"
	StatusUnknown ProjectStatus = "UNKNOWN" // Should not happen ideally
)

// ContainerState represents the state of a single container from 'podman compose ps'.
type ContainerState struct {
	Name    string `json:"Name"`
	Command string `json:"Command"`
	Service string `json:"Service"`
	Status  string `json:"Status"` // e.g., "running", "exited(0)", "created"
	Ports   string `json:"Ports"`
}

// ProjectRuntimeInfo holds the status information for a project.
type ProjectRuntimeInfo struct {
	OverallStatus ProjectStatus
	Containers    []ContainerState
	Error         error // Any error encountered during status check
}

// GetProjectStatus determines the runtime status of a Podman Compose project.
func GetProjectStatus(projectPath string) ProjectRuntimeInfo {
	info := ProjectRuntimeInfo{OverallStatus: StatusUnknown} // Default

	cmd := exec.Command("podman", "compose", "-f", "compose.yaml", "ps", "--format", "json", "-a")
	cmd.Dir = projectPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check if the error is just "no containers found" which might mean DOWN
		stderrStr := stderr.String()
		if strings.Contains(stderrStr, "no containers found") {
			info.OverallStatus = StatusDown
			return info // No containers means it's down
		}
		// Otherwise, it's a real error
		info.OverallStatus = StatusError
		info.Error = fmt.Errorf("failed to run 'podman compose ps': %w\nStderr: %s", err, stderrStr)
		return info
	}

	// Handle empty output - might happen if compose file is empty or invalid?
	if stdout.Len() == 0 {
		// Let's assume DOWN if ps runs ok but gives no output. Could refine this.
		info.OverallStatus = StatusDown
		return info
	}

	// Decode the JSON output
	// The output is typically a JSON object per line, not a single array
	var containers []ContainerState
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		var container ContainerState
		line := scanner.Bytes()
		// Skip empty lines just in case
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &container); err != nil {
			info.OverallStatus = StatusError
			info.Error = fmt.Errorf("failed to decode container status JSON line '%s': %w", string(line), err)
			return info // Stop processing on decode error
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
		// If ps ran successfully but returned no containers (after filtering empty lines)
		info.OverallStatus = StatusDown
		return info
	}

	// Determine overall status
	allRunning := true
	anyRunning := false
	for _, c := range containers {
		// Consider "running" or "healthy" states as up
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
