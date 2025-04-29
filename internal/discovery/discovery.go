package discovery

import (
	"fmt"
	"os"
	"path/filepath"
)

// Project represents a discovered Podman Compose project.
type Project struct {
	Name string // Name of the directory
	Path string // Full path to the directory
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
