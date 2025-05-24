// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package api implements the HTTP API endpoints for the bucket manager's web interface.
// It provides handlers for fetching stack information, executing commands, and
// managing SSH configurations through a RESTful API.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
	"bucket-manager/internal/runner"

	"github.com/gorilla/mux"
)

// StackWithStatus combines Stack information with its runtime status
// for presenting complete stack information to the web UI
type StackWithStatus struct {
	discovery.Stack                    // Embedded Stack struct with stack metadata
	Status          runner.StackStatus `json:"status"` // Current running status of the stack
}

// collectStacksWithStatus retrieves status for a slice of stacks concurrently
// using goroutines for efficient parallel processing
// collectStacksWithStatus transforms a slice of Stack objects into StackWithStatus objects
// by fetching the current status of each stack in parallel using goroutines.
//
// This function:
// 1. Creates a result array to store status-enhanced stack information
// 2. Launches a goroutine for each stack to fetch its status concurrently
// 3. Waits for all status checks to complete
// 4. Returns the complete array with status information
//
// Parameters:
//   - stacks: A slice of discovery.Stack objects to enhance with status
//
// Returns:
//   - []StackWithStatus: Stack information with current status details
func collectStacksWithStatus(stacks []discovery.Stack) []StackWithStatus {
	startTime := time.Now()

	logger.Debug("Starting status collection for stacks",
		"stack_count", len(stacks))

	stacksWithStatus := make([]StackWithStatus, len(stacks))
	var wg sync.WaitGroup
	wg.Add(len(stacks))

	for i, stack := range stacks {
		go func(i int, s discovery.Stack) {
			defer wg.Done()

			stackStartTime := time.Now()
			logger.Debug("Getting status for stack",
				"stack_name", s.Name,
				"server_name", s.ServerName,
				"is_remote", s.IsRemote)

			statusInfo := runner.GetStackStatus(s)
			stacksWithStatus[i] = StackWithStatus{
				Stack:  s,
				Status: statusInfo.OverallStatus,
			}

			logger.Debug("Status retrieved for stack",
				"stack_name", s.Name,
				"server_name", s.ServerName,
				"status", statusInfo.OverallStatus,
				"duration", time.Since(stackStartTime))
		}(i, stack)
	}

	wg.Wait()

	logger.Info("Status collection completed for all stacks",
		"stack_count", len(stacks),
		"total_duration", time.Since(startTime))

	return stacksWithStatus
}

// writeJSONResponse writes a JSON response with CORS headers
func writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
}

// findSSHHost finds a host by name from the config
func findSSHHost(hostName string) (*config.SSHHost, error) {
	logger.Debug("Looking up SSH host configuration", "host_name", hostName)

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config for SSH host lookup",
			"host_name", hostName,
			"error", err)
		return nil, fmt.Errorf("error loading config: %v", err)
	}

	logger.Debug("Config loaded for SSH host lookup",
		"host_name", hostName,
		"total_ssh_hosts", len(cfg.SSHHosts))

	for i := range cfg.SSHHosts {
		if cfg.SSHHosts[i].Name == hostName {
			logger.Debug("SSH host found",
				"host_name", hostName,
				"hostname", cfg.SSHHosts[i].Hostname,
				"user", cfg.SSHHosts[i].User,
				"port", cfg.SSHHosts[i].Port)
			return &cfg.SSHHosts[i], nil
		}
	}

	logger.Warn("SSH host not found in configuration",
		"host_name", hostName,
		"available_hosts", len(cfg.SSHHosts))
	return nil, fmt.Errorf("SSH host not found")
}

// findStackByName finds a stack by name in a slice of stacks
func findStackByName(stacks []discovery.Stack, name string) (*discovery.Stack, error) {
	logger.Debug("Searching for stack by name",
		"stack_name", name,
		"total_stacks", len(stacks))

	for i, stack := range stacks {
		if stack.Name == name {
			logger.Debug("Stack found",
				"stack_name", name,
				"stack_path", stack.Path,
				"server_name", stack.ServerName,
				"is_remote", stack.IsRemote)
			return &stacks[i], nil
		}
	}

	logger.Debug("Stack not found",
		"stack_name", name,
		"searched_stacks", len(stacks))
	return nil, fmt.Errorf("stack not found")
}

// findRemoteStackByNameAndServer finds a remote stack on a specific host by name.
//
// This function:
// 1. Locates the SSH host configuration by name
// 2. Attempts to use cached stacks from a recent discovery if available
// 3. Otherwise performs a fresh discovery of stacks on the remote host
// 4. Searches for the requested stack by name in the discovered stacks
// 5. Returns the complete stack information with AbsoluteRemoteRoot populated
//
// Using this function avoids rediscovering all stacks unnecessarily when
// performing operations on a specific stack.
//
// Parameters:
//   - stackName: The name of the stack to find
//   - serverName: The name of the SSH host where the stack is located
//
// Returns:
//   - discovery.Stack: The complete stack information if found
//   - error: An error if the host or stack wasn't found, or if discovery failed
func findRemoteStackByNameAndServer(stackName, serverName string) (discovery.Stack, error) {
	startTime := time.Now()

	logger.Debug("Finding remote stack by name and server",
		"stack_name", stackName,
		"server_name", serverName)

	targetHost, err := findSSHHost(serverName)
	if err != nil {
		logger.Error("Failed to find SSH host for remote stack lookup",
			"stack_name", stackName,
			"server_name", serverName,
			"error", err)
		return discovery.Stack{}, err
	}

	logger.Debug("SSH host found, discovering remote stacks",
		"stack_name", stackName,
		"server_name", serverName,
		"hostname", targetHost.Hostname)

	// TODO: In a future improvement, we should cache discovered stacks to avoid
	// rediscovery for every operation. For now, we'll fetch them each time.
	stacks, err := discovery.FindRemoteStacks(targetHost)
	if err != nil {
		logger.Error("Failed to discover remote stacks for stack lookup",
			"stack_name", stackName,
			"server_name", serverName,
			"error", err,
			"duration", time.Since(startTime))
		return discovery.Stack{}, fmt.Errorf("error finding remote stacks: %w", err)
	}

	logger.Debug("Remote stacks discovered, searching for target stack",
		"stack_name", stackName,
		"server_name", serverName,
		"total_stacks", len(stacks))

	for _, stack := range stacks {
		if stack.Name == stackName {
			logger.Info("Remote stack found successfully",
				"stack_name", stackName,
				"server_name", serverName,
				"stack_path", stack.Path,
				"duration", time.Since(startTime))
			return stack, nil
		}
	}

	logger.Error("Remote stack not found",
		"stack_name", stackName,
		"server_name", serverName,
		"searched_stacks", len(stacks),
		"duration", time.Since(startTime))
	return discovery.Stack{}, fmt.Errorf("stack '%s' not found on host '%s'", stackName, serverName)
}

func RegisterStackRoutes(router *mux.Router) {
	router.HandleFunc("/api/stacks/local", listLocalStacksHandler).Methods("GET")
	router.HandleFunc("/api/stacks/local/{name}/status", getLocalStackStatusHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts/{hostName}/stacks", listRemoteStacksHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts/{hostName}/stacks/{name}/status", getRemoteStackStatusHandler).Methods("GET")
}

// listLocalStacksHandler serves the GET /api/stacks/local endpoint, which returns
// all compose stacks found in the local filesystem. This endpoint discovers
// stacks by searching for compose.yaml, compose.yml, docker-compose.yaml, and docker-compose.yml files.
//
// Response:
// - 200 OK: Returns an array of stack objects with their status information
// - 500 Internal Server Error: If an error occurs during stack discovery
//
// If no local root directory is configured or found, an empty array is returned
// rather than an error.
func listLocalStacksHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	logger.Info("API request received",
		"endpoint", "/api/stacks/local",
		"method", r.Method,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent())

	rootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		// If no local root is found, return an empty list, not an error
		if strings.Contains(err.Error(), "could not find") {
			logger.Info("No local root directory found, returning empty stack list",
				"duration", time.Since(startTime))
			writeJSONResponse(w, []StackWithStatus{})
			return
		}
		logger.Error("Failed to get local root directory",
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Debug("Local root directory found", "root_dir", rootDir)

	stacks, err := discovery.FindLocalStacks(rootDir)
	if err != nil {
		logger.Error("Failed to find local stacks",
			"root_dir", rootDir,
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, fmt.Sprintf("Error finding local stacks: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Debug("Local stacks discovered",
		"stack_count", len(stacks),
		"root_dir", rootDir)

	stacksWithStatus := collectStacksWithStatus(stacks)
	writeJSONResponse(w, stacksWithStatus)

	logger.Info("API request completed successfully",
		"endpoint", "/api/stacks/local",
		"stack_count", len(stacks),
		"duration", time.Since(startTime))
}

// listRemoteStacksHandler serves the GET /api/stacks/remote/{host} endpoint, which returns
// all compose stacks found on a specific remote SSH host. The host must be
// configured in the application's SSH configuration.
//
// URL Parameters:
// - host: The name of the SSH host as configured in the application
//
// Response:
// - 200 OK: Returns an array of stack objects with their status information
// - 400 Bad Request: If the host parameter is missing or invalid
// - 404 Not Found: If the specified host is not configured
// - 500 Internal Server Error: If an error occurs during stack discovery or SSH connection
func listRemoteStacksHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	vars := mux.Vars(r)
	hostName := vars["hostName"]

	logger.Info("API request received",
		"endpoint", "/api/ssh/hosts/stacks",
		"method", r.Method,
		"host_name", hostName,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent())

	targetHost, err := findSSHHost(hostName)
	if err != nil {
		logger.Error("SSH host not found",
			"host_name", hostName,
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logger.Debug("SSH host configuration found",
		"host_name", hostName,
		"hostname", targetHost.Hostname,
		"user", targetHost.User,
		"port", targetHost.Port)

	stacks, err := discovery.FindRemoteStacks(targetHost)
	if err != nil {
		// If no remote root is found, return an empty list, not an error
		if strings.Contains(err.Error(), "could not find") {
			logger.Info("No remote root directory found, returning empty stack list",
				"host_name", hostName,
				"duration", time.Since(startTime))
			writeJSONResponse(w, []StackWithStatus{})
			return
		}
		logger.Error("Failed to find remote stacks",
			"host_name", hostName,
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, fmt.Sprintf("Error finding remote stacks for host %s: %v", hostName, err), http.StatusInternalServerError)
		return
	}

	logger.Debug("Remote stacks discovered",
		"host_name", hostName,
		"stack_count", len(stacks))

	stacksWithStatus := collectStacksWithStatus(stacks)
	writeJSONResponse(w, stacksWithStatus)

	logger.Info("API request completed successfully",
		"endpoint", "/api/ssh/hosts/stacks",
		"host_name", hostName,
		"stack_count", len(stacks),
		"duration", time.Since(startTime))
}

// getLocalStackStatusHandler serves the GET /api/stacks/local/{path}/status endpoint,
// which returns the current status of a specific local compose stack.
//
// This endpoint returns detailed container status information for the requested stack,
// including which containers are running, their names, and any errors encountered.
//
// URL Parameters:
// - path: The URL-encoded relative path to the stack directory from the compose root
//
// Response:
// - 200 OK: Returns a status object with container details and overall status
// - 400 Bad Request: If the path parameter is missing or malformed
// - 404 Not Found: If the specified stack does not exist
// - 500 Internal Server Error: If an error occurs while fetching the status
func getLocalStackStatusHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	vars := mux.Vars(r)
	stackName := vars["name"]

	logger.Info("API request received",
		"endpoint", "/api/stacks/local/status",
		"method", r.Method,
		"stack_name", stackName,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent())

	rootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		logger.Error("Failed to get local root directory",
			"stack_name", stackName,
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Debug("Local root directory found",
		"stack_name", stackName,
		"root_dir", rootDir)

	stacks, err := discovery.FindLocalStacks(rootDir)
	if err != nil {
		logger.Error("Failed to find local stacks",
			"stack_name", stackName,
			"root_dir", rootDir,
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, fmt.Sprintf("Error finding local stacks: %v", err), http.StatusInternalServerError)
		return
	}

	targetStack, err := findStackByName(stacks, stackName)
	if err != nil {
		logger.Error("Stack not found",
			"stack_name", stackName,
			"available_stacks", len(stacks),
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logger.Debug("Stack found, getting status",
		"stack_name", stackName,
		"stack_path", targetStack.Path)

	statusInfo := runner.GetStackStatus(*targetStack)
	response := map[string]interface{}{
		"name":   targetStack.Name,
		"status": statusInfo.OverallStatus,
	}

	writeJSONResponse(w, response)

	logger.Info("API request completed successfully",
		"endpoint", "/api/stacks/local/status",
		"stack_name", stackName,
		"status", statusInfo.OverallStatus,
		"duration", time.Since(startTime))
}

// getRemoteStackStatusHandler serves the GET /api/stacks/remote/{host}/{path}/status endpoint,
// which returns the current status of a specific compose stack on a remote host.
//
// This endpoint retrieves detailed container status information for the requested stack
// on the specified SSH host, including which containers are running, their names, and
// any errors encountered.
//
// URL Parameters:
// - host: The name of the SSH host as configured in the application
// - path: The URL-encoded relative path to the stack directory from the remote root
//
// Response:
// - 200 OK: Returns a status object with container details and overall status
// - 400 Bad Request: If any parameters are missing or malformed
// - 404 Not Found: If the host or stack does not exist
// - 500 Internal Server Error: If an error occurs during SSH connection or status fetching
func getRemoteStackStatusHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	vars := mux.Vars(r)
	hostName := vars["hostName"]
	stackName := vars["name"]

	logger.Info("API request received",
		"endpoint", "/api/ssh/hosts/stacks/status",
		"method", r.Method,
		"host_name", hostName,
		"stack_name", stackName,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent())

	targetHost, err := findSSHHost(hostName)
	if err != nil {
		logger.Error("SSH host not found",
			"host_name", hostName,
			"stack_name", stackName,
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logger.Debug("SSH host configuration found",
		"host_name", hostName,
		"stack_name", stackName,
		"hostname", targetHost.Hostname,
		"user", targetHost.User,
		"port", targetHost.Port)

	stacks, err := discovery.FindRemoteStacks(targetHost)
	if err != nil {
		logger.Error("Failed to find remote stacks",
			"host_name", hostName,
			"stack_name", stackName,
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, fmt.Sprintf("Error finding remote stacks: %v", err), http.StatusInternalServerError)
		return
	}

	targetStack, err := findStackByName(stacks, stackName)
	if err != nil {
		logger.Error("Stack not found on remote host",
			"host_name", hostName,
			"stack_name", stackName,
			"available_stacks", len(stacks),
			"error", err,
			"duration", time.Since(startTime))
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logger.Debug("Remote stack found, getting status",
		"host_name", hostName,
		"stack_name", stackName,
		"stack_path", targetStack.Path)

	statusInfo := runner.GetStackStatus(*targetStack)
	response := map[string]interface{}{
		"name":   targetStack.Name,
		"status": statusInfo.OverallStatus,
	}

	writeJSONResponse(w, response)

	logger.Info("API request completed successfully",
		"endpoint", "/api/ssh/hosts/stacks/status",
		"host_name", hostName,
		"stack_name", stackName,
		"status", statusInfo.OverallStatus,
		"duration", time.Since(startTime))
}
