// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package api implements the HTTP API endpoints for the bucket manager's web interface.
// The runner.go file specifically handles endpoints related to executing commands
// on stacks and hosts, including both synchronous and streaming execution modes.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/runner"

	"github.com/gorilla/mux"
)

// StackRunRequest represents the expected JSON body for stack runner endpoints.
// It contains the information needed to identify a specific stack for operations.
type StackRunRequest struct {
	Name       string `json:"name"`       // Name of the stack to operate on
	ServerName string `json:"serverName"` // Server where the stack is located ("local" or SSH host name)
}

// HostRunRequest represents the expected JSON body for host runner endpoints.
// It specifies which server should execute host-level operations like pruning.
type HostRunRequest struct {
	ServerName string `json:"serverName"` // Server to run the command on ("local" or SSH host name)
}

// RunOutput represents the output of a command execution.
// Used for returning command results in API responses.
type RunOutput struct {
	Output string `json:"output"`          // Standard output from the command
	Error  string `json:"error,omitempty"` // Error output if the command failed
}

// RegisterRunnerRoutes registers the API routes for running commands and actions.
// These include both synchronous (POST) and streaming (GET) endpoints for
// various stack and host operations.
func RegisterRunnerRoutes(router *mux.Router) {
	// Synchronous stack operation endpoints (return output all at once)
	router.HandleFunc("/api/run/stack/up", runStackUpHandler).Methods("POST")
	router.HandleFunc("/api/run/stack/pull", runStackPullHandler).Methods("POST")
	router.HandleFunc("/api/run/stack/down", runStackDownHandler).Methods("POST")
	router.HandleFunc("/api/run/stack/refresh", runStackRefreshHandler).Methods("POST")

	// Streaming endpoints (return output as it's generated using Server-Sent Events)
	router.HandleFunc("/api/run/stack/refresh/stream", streamStackRefreshHandler).Methods("GET")
	router.HandleFunc("/api/run/stack/up/stream", streamStackUpHandler).Methods("GET")
	router.HandleFunc("/api/run/stack/down/stream", streamStackDownHandler).Methods("GET")
	router.HandleFunc("/api/run/stack/pull/stream", streamStackPullHandler).Methods("GET")

	// Host-level operation endpoints
	router.HandleFunc("/api/run/host/prune", runHostPruneHandler).Methods("POST")
	// TODO: Add routes for running arbitrary commands or sequences
	//  - POST /api/run/stack/custom for executing custom sequences on stacks
	//  - POST /api/run/host/custom for executing arbitrary commands on hosts
}

// getStackFromRequest reads the request body and retrieves the corresponding discovery.Stack.
func getStackFromRequest(r *http.Request) (discovery.Stack, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return discovery.Stack{}, fmt.Errorf("error reading request body: %w", err)
	}
	defer r.Body.Close()

	var req StackRunRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return discovery.Stack{}, fmt.Errorf("invalid request body: %w", err)
	}

	if req.ServerName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			return discovery.Stack{}, fmt.Errorf("error getting local root directory: %w", err)
		}
		stackPath := rootDir + "/" + req.Name
		return discovery.Stack{
			Name:       req.Name,
			Path:       stackPath,
			ServerName: "local",
			IsRemote:   false,
		}, nil
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		return findRemoteStackByNameAndServer(req.Name, req.ServerName)
	}
}

// getHostTargetFromRequest reads the request body and retrieves the corresponding runner.HostTarget.
func getHostTargetFromRequest(r *http.Request) (runner.HostTarget, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return runner.HostTarget{}, fmt.Errorf("error reading request body: %w", err)
	}
	defer r.Body.Close()

	var req HostRunRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return runner.HostTarget{}, fmt.Errorf("invalid request body: %w", err)
	}

	if req.ServerName == "local" {
		return runner.HostTarget{ServerName: "local", IsRemote: false}, nil
	} else {
		// For remote hosts, find the host config
		cfg, err := config.LoadConfig()
		if err != nil {
			return runner.HostTarget{}, fmt.Errorf("error loading config: %w", err)
		}

		var targetHost *config.SSHHost
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == req.ServerName {
				targetHost = &cfg.SSHHosts[i]
				break
			}
		}

		if targetHost == nil {
			return runner.HostTarget{}, fmt.Errorf("SSH host '%s' not found", req.ServerName)
		}

		return runner.HostTarget{ServerName: req.ServerName, IsRemote: true, HostConfig: targetHost}, nil
	}
}

// runStackSequence streams the output of a given stack command sequence using Server-Sent Events.
// runStackSequence executes a sequence of commands and streams the output
// to the client using Server-Sent Events (SSE). This function is used by
// all streaming API endpoints to provide real-time command execution updates.
//
// The function:
// 1. Sets up proper headers for SSE communication
// 2. Executes each command in the sequence sequentially
// 3. Streams command outputs, errors, and step transitions as events
// 4. Handles flushing the response buffer to ensure timely updates
// 5. Terminates the stream when all commands complete or an error occurs
//
// Parameters:
//   - w: HTTP response writer to send the SSE stream
//   - sequence: Ordered list of commands to execute
func runStackSequence(w http.ResponseWriter, sequence []runner.CommandStep) {
	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow cross-origin for development

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// For simplicity, run steps sequentially and stream output
	for _, step := range sequence {
		// Send step name as an event
		fmt.Fprintf(w, "event: step\ndata: %s\n\n", step.Name)
		flusher.Flush()

		outChan, errChan := runner.StreamCommand(step, false) // Use cliMode false for channel output

		// Collect output and errors from channels and stream them
		for outputLine := range outChan {
			// Escape newlines in data to ensure proper SSE formatting
			// Remove extra spaces before newlines and normalize line endings
			line := strings.TrimRight(outputLine.Line, " \t\r\n")
			if line == "" {
				continue
			}
			escapedLine := strings.ReplaceAll(line, "\n", "\\n")
			if outputLine.IsError {
				fmt.Fprintf(w, "event: stderr\ndata: %s\n\n", escapedLine)
			} else {
				fmt.Fprintf(w, "event: stdout\ndata: %s\n\n", escapedLine)
			}
			flusher.Flush()
		}

		// Check for errors after the command finishes
		if err := <-errChan; err != nil {
			errMsg := strings.TrimRight(err.Error(), " \t\r\n")
			escapedError := strings.ReplaceAll(errMsg, "\n", "\\n")
			fmt.Fprintf(w, "event: error\ndata: Error during step '%s': %s\n\n", step.Name, escapedError)
			flusher.Flush()
		}
	}

	// Send a done event when the sequence is finished
	fmt.Fprintf(w, "event: done\ndata: Sequence finished\n\n")
	flusher.Flush()
}

// runHostCommand streams the output of a given host command using Server-Sent Events.
func runHostCommand(w http.ResponseWriter, step runner.HostCommandStep) {
	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow cross-origin for development

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send step name as an event
	fmt.Fprintf(w, "event: step\ndata: %s\n\n", step.Name)
	flusher.Flush()

	outChan, errChan := runner.RunHostCommand(step, false) // Use cliMode false for channel output

	// Collect output and errors from channels and stream them
	for outputLine := range outChan { // Normalize line endings
		lines := strings.Split(strings.TrimRight(outputLine.Line, " \t\r\n"), "\n")
		for _, line := range lines {
			if trimmed := strings.TrimRight(line, " \t\r"); trimmed != "" {
				escapedLine := strings.ReplaceAll(trimmed, "\n", "\\n")
				if outputLine.IsError {
					fmt.Fprintf(w, "event: stderr\ndata: %s\n\n", escapedLine)
				} else {
					fmt.Fprintf(w, "event: stdout\ndata: %s\n\n", escapedLine)
				}
			}
		}
		flusher.Flush()
	}

	// Check for errors after the command finishes
	if err := <-errChan; err != nil {
		escapedError := strings.ReplaceAll(err.Error(), "\n", "\\n")
		fmt.Fprintf(w, "event: error\ndata: Error during step '%s': %s\n\n", step.Name, escapedError)
		flusher.Flush()
	}

	// Send a done event when the command is finished
	fmt.Fprintf(w, "event: done\ndata: Command finished\n\n")
	flusher.Flush()
}

// runStackUpHandler handles requests to run the 'up' sequence on a stack.
func runStackUpHandler(w http.ResponseWriter, r *http.Request) {
	stack, err := getStackFromRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	sequence := runner.UpSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// runStackPullHandler handles requests to run the 'pull' sequence on a stack.
func runStackPullHandler(w http.ResponseWriter, r *http.Request) {
	stack, err := getStackFromRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	sequence := runner.PullSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// runStackDownHandler handles requests to run the 'down' sequence on a stack.
func runStackDownHandler(w http.ResponseWriter, r *http.Request) {
	stack, err := getStackFromRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	sequence := runner.DownSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// runStackRefreshHandler handles requests to run the 'refresh' sequence on a stack.
func runStackRefreshHandler(w http.ResponseWriter, r *http.Request) {
	stack, err := getStackFromRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	sequence := runner.RefreshSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackRefreshHandler handles GET requests to stream the 'refresh' sequence output on a stack.
// streamStackRefreshHandler serves the GET /api/stream/stack/refresh endpoint, which
// streams real-time output from the `podman-compose ps` command to check stack status.
//
// This handler uses Server-Sent Events (SSE) to provide a continuous stream of
// command execution updates to the client, including command output and error messages.
// The connection remains open until the command completes or an error occurs.
//
// Query Parameters:
// - name: The name of the stack to refresh
// - serverName: The server name where the stack is located ("local" or an SSH host name)
//
// Response:
// - 200 OK with text/event-stream content type for successful connections
// - 400 Bad Request if required parameters are missing
// - 404 Not Found if the stack or host doesn't exist
// - 500 Internal Server Error if command execution fails
func streamStackRefreshHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	if stackName == "" || serverName == "" {
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
			return
		}
		stackPath := rootDir + "/" + stackName
		stack = discovery.Stack{
			Name:       stackName,
			Path:       stackPath,
			ServerName: "local",
			IsRemote:   false,
		}
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	sequence := runner.RefreshSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackUpHandler serves the GET /api/stream/stack/up endpoint, which
// streams real-time output from the sequence of commands used to start a stack.
//
// This handler uses Server-Sent Events (SSE) to provide a continuous stream of
// command execution updates to the client as the stack is being started. The stream
// includes all output from `podman-compose up -d` and any related commands.
//
// The connection remains open until:
// - All commands complete successfully
// - An error occurs during execution
// - The client disconnects
//
// Query Parameters:
// - name: The name of the stack to start
// - serverName: The server name where the stack is located ("local" or an SSH host name)
//
// Response:
// - 200 OK with text/event-stream content type for successful connections
// - 400 Bad Request if required parameters are missing
// - 404 Not Found if the stack or host doesn't exist
// - 500 Internal Server Error if command execution fails
func streamStackUpHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	if stackName == "" || serverName == "" {
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
			return
		}
		stackPath := rootDir + "/" + stackName
		stack = discovery.Stack{
			Name:       stackName,
			Path:       stackPath,
			ServerName: "local",
			IsRemote:   false,
		}
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	sequence := runner.UpSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackDownHandler handles GET requests to stream the 'down' sequence output on a stack.
// streamStackDownHandler serves the GET /api/stream/stack/down endpoint, which
// streams real-time output from the sequence of commands used to stop a stack.
//
// This handler uses Server-Sent Events (SSE) to provide a continuous stream of
// command execution updates to the client as the stack is being stopped. The stream
// includes all output from `podman-compose down` and any related commands.
//
// The connection remains open until:
// - All commands complete successfully
// - An error occurs during execution
// - The client disconnects
//
// Query Parameters:
// - name: The name of the stack to stop
// - serverName: The server name where the stack is located ("local" or an SSH host name)
//
// Response:
// - 200 OK with text/event-stream content type for successful connections
// - 400 Bad Request if required parameters are missing
// - 404 Not Found if the stack or host doesn't exist
// - 500 Internal Server Error if command execution fails
func streamStackDownHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	if stackName == "" || serverName == "" {
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
			return
		}
		stackPath := rootDir + "/" + stackName
		stack = discovery.Stack{
			Name:       stackName,
			Path:       stackPath,
			ServerName: "local",
			IsRemote:   false,
		}
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	sequence := runner.DownSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackPullHandler handles GET requests to stream the 'pull' sequence output on a stack.
// streamStackPullHandler serves the GET /api/stream/stack/pull endpoint, which
// streams real-time output from the sequence of commands used to pull updated
// container images for a stack.
//
// This handler uses Server-Sent Events (SSE) to provide a continuous stream of
// command execution updates to the client as images are being pulled. The stream
// includes all output from `podman-compose pull` and any related commands.
//
// The connection remains open until:
// - All commands complete successfully
// - An error occurs during execution
// - The client disconnects
//
// Query Parameters:
// - name: The name of the stack to pull images for
// - serverName: The server name where the stack is located ("local" or an SSH host name)
//
// Response:
// - 200 OK with text/event-stream content type for successful connections
// - 400 Bad Request if required parameters are missing
// - 404 Not Found if the stack or host doesn't exist
// - 500 Internal Server Error if command execution fails
func streamStackPullHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	if stackName == "" || serverName == "" {
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting local root directory: %v", err), http.StatusInternalServerError)
			return
		}
		stackPath := rootDir + "/" + stackName
		stack = discovery.Stack{
			Name:       stackName,
			Path:       stackPath,
			ServerName: "local",
			IsRemote:   false,
		}
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	sequence := runner.PullSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// runHostPruneHandler handles requests to run the 'prune' action on a host.
// runHostPruneHandler serves the POST /api/host/prune endpoint, which executes
// the podman system prune command on a host to clean up unused resources.
//
// This handler runs the prune command on either the local system or a remote SSH host
// to remove unused containers, networks, images, and volumes. The output of the command
// is returned in the response.
//
// Request Body (JSON):
// - serverName: The name of the server to prune ("local" or an SSH host name)
// - pruneVolumes: Boolean flag indicating whether to prune volumes as well (optional)
//
// Response:
// - 200 OK with JSON containing command output and success status
// - 400 Bad Request if the serverName is missing or invalid
// - 404 Not Found if the host doesn't exist
// - 500 Internal Server Error if command execution fails
func runHostPruneHandler(w http.ResponseWriter, r *http.Request) {
	target, err := getHostTargetFromRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting host info: %v", err), http.StatusBadRequest)
		return
	}

	step := runner.PruneHostStep(target)
	runHostCommand(w, step) // Stream output
}

// TODO: Implement handlers for running arbitrary commands or sequences.
// These handlers should accept JSON payloads with custom command sequences
// and execute them in the same way as the predefined operations.
