package runner

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RunCommand executes a command in the specified directory and returns its combined output and any error.
func RunCommand(dir string, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	output := outb.String() + errb.String() // Combine stdout and stderr

	if err != nil {
		// Include command and output in the error message for better context
		return output, fmt.Errorf("command '%s %s' failed in %s: %w\nOutput:\n%s", command, strings.Join(args, " "), dir, err, output)
	}
	return output, nil
}

// Up runs 'podman compose pull' followed by 'podman compose up -d'.
func Up(projectPath string) (string, error) {
	var combinedOutput strings.Builder

	// 1. podman compose pull
	output, err := RunCommand(projectPath, "podman", "compose", "pull")
	combinedOutput.WriteString("--- podman compose pull ---\n")
	combinedOutput.WriteString(output)
	combinedOutput.WriteString("\n")
	if err != nil {
		return combinedOutput.String(), fmt.Errorf("failed during pull: %w", err)
	}

	// 2. podman compose up -d
	output, err = RunCommand(projectPath, "podman", "compose", "up", "-d")
	combinedOutput.WriteString("--- podman compose up -d ---\n")
	combinedOutput.WriteString(output)
	combinedOutput.WriteString("\n")
	if err != nil {
		return combinedOutput.String(), fmt.Errorf("failed during up: %w", err)
	}

	return combinedOutput.String(), nil
}

// Down runs 'podman compose down'.
func Down(projectPath string) (string, error) {
	var combinedOutput strings.Builder

	// 1. podman compose down
	output, err := RunCommand(projectPath, "podman", "compose", "down")
	combinedOutput.WriteString("--- podman compose down ---\n")
	combinedOutput.WriteString(output)
	combinedOutput.WriteString("\n")
	if err != nil {
		return combinedOutput.String(), fmt.Errorf("failed during down: %w", err)
	}

	return combinedOutput.String(), nil
}

// Refresh runs 'pull', 'down', 'up -d', and 'system prune -a -f'.
func Refresh(projectPath string) (string, error) {
	var combinedOutput strings.Builder

	// 1. podman compose pull
	output, err := RunCommand(projectPath, "podman", "compose", "pull")
	combinedOutput.WriteString("--- podman compose pull ---\n")
	combinedOutput.WriteString(output)
	combinedOutput.WriteString("\n")
	if err != nil {
		return combinedOutput.String(), fmt.Errorf("failed during pull: %w", err)
	}

	// 2. podman compose down
	output, err = RunCommand(projectPath, "podman", "compose", "down")
	combinedOutput.WriteString("--- podman compose down ---\n")
	combinedOutput.WriteString(output)
	combinedOutput.WriteString("\n")
	// We continue even if down fails, as 'up' might fix things or be needed anyway.
	// Log the error? For now, just append output.

	// 3. podman compose up -d
	output, err = RunCommand(projectPath, "podman", "compose", "up", "-d")
	combinedOutput.WriteString("--- podman compose up -d ---\n")
	combinedOutput.WriteString(output)
	combinedOutput.WriteString("\n")
	if err != nil {
		return combinedOutput.String(), fmt.Errorf("failed during up: %w", err)
	}

	// 4. podman system prune -a -f (Run globally, not specific to project dir)
	// Note: Pruning is system-wide, so running it from the project dir doesn't change its scope.
	output, err = RunCommand("", "podman", "system", "prune", "-a", "-f") // Run from default dir
	combinedOutput.WriteString("--- podman system prune -a -f ---\n")
	combinedOutput.WriteString(output)
	combinedOutput.WriteString("\n")
	if err != nil {
		// Log prune failure but don't necessarily fail the whole refresh?
		// For now, let's return the error.
		return combinedOutput.String(), fmt.Errorf("failed during system prune: %w", err)
	}

	return combinedOutput.String(), nil
}
