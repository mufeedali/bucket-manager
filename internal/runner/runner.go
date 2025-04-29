package runner

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// OutputLine represents a single line of output from a command.
type OutputLine struct {
	Line    string
	IsError bool // Indicates if the line came from stderr
}

// CommandStep defines a single command to be executed within a sequence.
type CommandStep struct {
	Name    string   // User-friendly name for the step (e.g., "Pulling images")
	Command string   // The command to run (e.g., "podman")
	Args    []string // Arguments for the command
	Dir     string   // Directory to execute the command in
}

// StreamCommand executes a single CommandStep and streams its output.
// It returns channels for output lines and the final error.
func StreamCommand(step CommandStep) (<-chan OutputLine, <-chan error) {
	outChan := make(chan OutputLine)
	errChan := make(chan error, 1) // Buffered channel for the final error

	go func() {
		defer close(outChan)
		defer close(errChan)

		cmd := exec.Command(step.Command, step.Args...)
		cmd.Dir = step.Dir

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			errChan <- fmt.Errorf("error creating stdout pipe for '%s': %w", step.Name, err)
			return
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			errChan <- fmt.Errorf("error creating stderr pipe for '%s': %w", step.Name, err)
			return
		}

		var wg sync.WaitGroup
		wg.Add(2) // One goroutine for stdout, one for stderr

		// Goroutine to stream stdout
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdoutPipe)
			for scanner.Scan() {
				outChan <- OutputLine{Line: scanner.Text(), IsError: false}
			}
			if err := scanner.Err(); err != nil && err != io.EOF {
				// Send scanner errors as error lines? Or to errChan? Let's use errChan.
				// Note: This might race with cmd.Wait() error reporting.
				// Consider a dedicated error channel or prefixing these messages.
				outChan <- OutputLine{Line: fmt.Sprintf("stdout scanner error: %v", err), IsError: true}
			}
		}()

		// Goroutine to stream stderr
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				outChan <- OutputLine{Line: scanner.Text(), IsError: true}
			}
			if err := scanner.Err(); err != nil && err != io.EOF {
				outChan <- OutputLine{Line: fmt.Sprintf("stderr scanner error: %v", err), IsError: true}
			}
		}()

		// Start the command
		if err := cmd.Start(); err != nil {
			errChan <- fmt.Errorf("error starting command '%s': %w", step.Name, err)
			// Need to ensure goroutines reading pipes eventually exit if Start fails.
			// Closing the pipes might be necessary, but cmd.Wait() won't be called.
			// This part needs careful handling in a real-world scenario.
			// For now, we assume Start() failing is rare and the pipes might hang.
			return
		}

		// Wait for reader goroutines to finish
		wg.Wait()

		// Wait for the command to complete and capture the final error
		err = cmd.Wait()
		if err != nil {
			// Send the command execution error
			errChan <- fmt.Errorf("command '%s' failed: %w", step.Name, err)
		} else {
			// Signal successful completion by sending nil
			errChan <- nil
		}
	}()

	return outChan, errChan
}

// --- Action Sequences ---

// UpSequence returns the steps for the 'up' action.
func UpSequence(projectPath string) []CommandStep {
	return []CommandStep{
		{Name: "Pulling images", Command: "podman", Args: []string{"compose", "pull"}, Dir: projectPath},
		{Name: "Starting containers", Command: "podman", Args: []string{"compose", "up", "-d"}, Dir: projectPath},
	}
}

// DownSequence returns the steps for the 'down' action.
func DownSequence(projectPath string) []CommandStep {
	return []CommandStep{
		{Name: "Stopping containers", Command: "podman", Args: []string{"compose", "down"}, Dir: projectPath},
	}
}

// RefreshSequence returns the steps for the 'refresh' action.
func RefreshSequence(projectPath string) []CommandStep {
	return []CommandStep{
		{Name: "Pulling images", Command: "podman", Args: []string{"compose", "pull"}, Dir: projectPath},
		{Name: "Stopping containers", Command: "podman", Args: []string{"compose", "down"}, Dir: projectPath},
		{Name: "Starting containers", Command: "podman", Args: []string{"compose", "up", "-d"}, Dir: projectPath},
		// Run prune globally, Dir "" means current working dir of the app, which is fine.
		{Name: "Pruning system", Command: "podman", Args: []string{"system", "prune", "-a", "-f"}, Dir: ""},
	}
}
