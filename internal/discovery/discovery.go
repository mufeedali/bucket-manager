package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Project represents a discovered Podman Compose project.
type Project struct {
	Name string // Name of the directory
	Path string // Full path to the directory
}

// GetComposeRootDirectory finds the root directory for compose projects by checking
// standard locations within the user's home directory.
func GetComposeRootDirectory() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}

	possibleDirs := []string{
		filepath.Join(homeDir, "bucket"),
		filepath.Join(homeDir, "compose-bucket"),
	}

	for _, dir := range possibleDirs {
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			return dir, nil // Found a valid directory
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			// Log or handle unexpected errors during stat, but continue checking other paths
			fmt.Fprintf(os.Stderr, "Warning: error checking directory %s: %v\n", dir, err)
		}
	}

	return "", fmt.Errorf("could not find 'bucket' or 'compose-bucket' directory in home directory (%s)", homeDir)
}

// FindProjects scans the given root directory for subdirectories containing
// compose.yaml or compose.yml files.
func FindProjects(rootDir string) ([]Project, error) {
	var projects []Project

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read root directory %s: %w", rootDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectName := entry.Name()
		projectPath := filepath.Join(rootDir, projectName)

		composePathYaml := filepath.Join(projectPath, "compose.yaml")
		composePathYml := filepath.Join(projectPath, "compose.yml")

		_, errYaml := os.Stat(composePathYaml)
		_, errYml := os.Stat(composePathYml)

		if errYaml == nil || errYml == nil {
			projects = append(projects, Project{
				Name: projectName,
				Path: projectPath,
			})
		} else if !os.IsNotExist(errYaml) || !os.IsNotExist(errYml) {
			// Log errors other than "Not Exists" when checking compose files
			fmt.Fprintf(os.Stderr, "Warning: could not stat compose files in %s: %v / %v\n", projectPath, errYaml, errYml)
		}
	}

	return projects, nil
}
