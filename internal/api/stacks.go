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

	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
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
	stacksWithStatus := make([]StackWithStatus, len(stacks))
	var wg sync.WaitGroup
	wg.Add(len(stacks))

	for i, stack := range stacks {
		go func(i int, s discovery.Stack) {
			defer wg.Done()
			statusInfo := runner.GetStackStatus(s)
			stacksWithStatus[i] = StackWithStatus{
				Stack:  s,
				Status: statusInfo.OverallStatus,
			}
		}(i, stack)
	}

	wg.Wait()
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
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error loading config: %v", err)
	}

	for i := range cfg.SSHHosts {
		if cfg.SSHHosts[i].Name == hostName {
			return &cfg.SSHHosts[i], nil
		}
	}

	return nil, fmt.Errorf("SSH host not found")
}

// findStackByName finds a stack by name in a slice of stacks
func findStackByName(stacks []discovery.Stack, name string) (*discovery.Stack, error) {
	for i, stack := range stacks {
		if stack.Name == name {
			return &stacks[i], nil
		}
	}
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
	targetHost, err := findSSHHost(serverName)
	if err != nil {
		return discovery.Stack{}, err
	}

	// TODO: In a future improvement, we should cache discovered stacks to avoid
	// rediscovery for every operation. For now, we'll fetch them each time.
	stacks, err := discovery.FindRemoteStacks(targetHost)
	if err != nil {
		return discovery.Stack{}, fmt.Errorf("error finding remote stacks: %w", err)
	}

	for _, stack := range stacks {
		if stack.Name == stackName {
			return stack, nil
		}
	}

	return discovery.Stack{}, fmt.Errorf("stack '%s' not found on host '%s'", stackName, serverName)
}

func RegisterStackRoutes(router *mux.Router) {
	router.HandleFunc("/api/stacks/local", listLocalStacksHandler).Methods("GET")
	router.HandleFunc("/api/stacks/local/{name}/status", getLocalStackStatusHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts/{hostName}/stacks", listRemoteStacksHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts/{hostName}/stacks/{name}/status", getRemoteStackStatusHandler).Methods("GET")
}

// listLocalStacksHandler serves the GET /api/stacks/local endpoint, which returns
// all Podman Compose stacks found in the local filesystem. This endpoint discovers
// stacks by searching for docker-compose.yml or podman-compose.yml files.
//
// Response:
// - 200 OK: Returns an array of stack objects with their status information
// - 500 Internal Server Error: If an error occurs during stack discovery
//
// If no local root directory is configured or found, an empty array is returned
// rather than an error.
func listLocalStacksHandler(w http.ResponseWriter, r *http.Request) {
	rootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		// If no local root is found, return an empty list, not an error
		if strings.Contains(err.Error(), "could not find") {
			writeJSONResponse(w, []StackWithStatus{})
			return
		}
		http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
		return
	}

	stacks, err := discovery.FindLocalStacks(rootDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error finding local stacks: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, collectStacksWithStatus(stacks))
}

// listRemoteStacksHandler serves the GET /api/stacks/remote/{host} endpoint, which returns
// all Podman Compose stacks found on a specific remote SSH host. The host must be
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
	vars := mux.Vars(r)
	hostName := vars["hostName"]

	targetHost, err := findSSHHost(hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	stacks, err := discovery.FindRemoteStacks(targetHost)
	if err != nil {
		// If no remote root is found, return an empty list, not an error
		if strings.Contains(err.Error(), "could not find") {
			writeJSONResponse(w, []StackWithStatus{})
			return
		}
		http.Error(w, fmt.Sprintf("Error finding remote stacks for host %s: %v", hostName, err), http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, collectStacksWithStatus(stacks))
}

// getLocalStackStatusHandler serves the GET /api/stacks/local/{path}/status endpoint,
// which returns the current status of a specific local Podman Compose stack.
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
	vars := mux.Vars(r)
	stackName := vars["name"]

	rootDir, err := discovery.GetComposeRootDirectory()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
		return
	}

	stacks, err := discovery.FindLocalStacks(rootDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error finding local stacks: %v", err), http.StatusInternalServerError)
		return
	}

	targetStack, err := findStackByName(stacks, stackName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	statusInfo := runner.GetStackStatus(*targetStack)
	response := map[string]interface{}{
		"name":   targetStack.Name,
		"status": statusInfo.OverallStatus,
	}

	writeJSONResponse(w, response)
}

// getRemoteStackStatusHandler serves the GET /api/stacks/remote/{host}/{path}/status endpoint,
// which returns the current status of a specific Podman Compose stack on a remote host.
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
	vars := mux.Vars(r)
	hostName := vars["hostName"]
	stackName := vars["name"]

	targetHost, err := findSSHHost(hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	stacks, err := discovery.FindRemoteStacks(targetHost)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error finding remote stacks: %v", err), http.StatusInternalServerError)
		return
	}

	targetStack, err := findStackByName(stacks, stackName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	statusInfo := runner.GetStackStatus(*targetStack)
	response := map[string]interface{}{
		"name":   targetStack.Name,
		"status": statusInfo.OverallStatus,
	}

	writeJSONResponse(w, response)
}
