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
	"time"

	"bucket-manager/internal/config"
	"bucket-manager/internal/discovery"
	"bucket-manager/internal/logger"
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
	startTime := time.Now()

	logger.Debug("Processing stack request from request body",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body for stack request",
			"error", err,
			"remote_addr", r.RemoteAddr)
		return discovery.Stack{}, fmt.Errorf("error reading request body: %w", err)
	}
	defer r.Body.Close()

	var req StackRunRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error("Failed to unmarshal stack request body",
			"error", err,
			"body_length", len(body),
			"remote_addr", r.RemoteAddr)
		return discovery.Stack{}, fmt.Errorf("invalid request body: %w", err)
	}

	logger.Debug("Parsed stack request",
		"stack_name", req.Name,
		"server_name", req.ServerName,
		"duration", time.Since(startTime))

	if req.ServerName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			logger.Error("Failed to get local compose root directory for stack request",
				"stack_name", req.Name,
				"error", err)
			return discovery.Stack{}, fmt.Errorf("error getting local root directory: %w", err)
		}
		stackPath := rootDir + "/" + req.Name

		logger.Info("Created local stack from request",
			"stack_name", req.Name,
			"stack_path", stackPath,
			"duration", time.Since(startTime))

		return discovery.Stack{
			Name:       req.Name,
			Path:       stackPath,
			ServerName: "local",
			IsRemote:   false,
		}, nil
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		logger.Debug("Looking up remote stack",
			"stack_name", req.Name,
			"server_name", req.ServerName)

		stack, err := findRemoteStackByNameAndServer(req.Name, req.ServerName)
		if err != nil {
			logger.Error("Failed to find remote stack",
				"stack_name", req.Name,
				"server_name", req.ServerName,
				"error", err)
			return discovery.Stack{}, err
		}

		logger.Info("Found remote stack from request",
			"stack_name", req.Name,
			"server_name", req.ServerName,
			"stack_path", stack.Path,
			"duration", time.Since(startTime))

		return stack, nil
	}
}

// getHostTargetFromRequest reads the request body and retrieves the corresponding runner.HostTarget.
func getHostTargetFromRequest(r *http.Request) (runner.HostTarget, error) {
	startTime := time.Now()

	logger.Debug("Processing host target request from request body",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body for host target request",
			"error", err,
			"remote_addr", r.RemoteAddr)
		return runner.HostTarget{}, fmt.Errorf("error reading request body: %w", err)
	}
	defer r.Body.Close()

	var req HostRunRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error("Failed to unmarshal host target request body",
			"error", err,
			"body_length", len(body),
			"remote_addr", r.RemoteAddr)
		return runner.HostTarget{}, fmt.Errorf("invalid request body: %w", err)
	}

	logger.Debug("Parsed host target request",
		"server_name", req.ServerName,
		"duration", time.Since(startTime))

	if req.ServerName == "local" {
		logger.Info("Created local host target from request",
			"server_name", req.ServerName,
			"duration", time.Since(startTime))
		return runner.HostTarget{ServerName: "local", IsRemote: false}, nil
	} else {
		// For remote hosts, find the host config
		logger.Debug("Loading config for remote host target",
			"server_name", req.ServerName)

		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Error("Failed to load config for remote host target",
				"server_name", req.ServerName,
				"error", err)
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
			logger.Error("SSH host not found for host target request",
				"server_name", req.ServerName,
				"available_hosts", len(cfg.SSHHosts))
			return runner.HostTarget{}, fmt.Errorf("SSH host '%s' not found", req.ServerName)
		}

		logger.Info("Created remote host target from request",
			"server_name", req.ServerName,
			"host_address", targetHost.Hostname,
			"duration", time.Since(startTime))

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
	startTime := time.Now()

	logger.Info("Starting stack command sequence stream",
		"sequence_length", len(sequence),
		"steps", func() []string {
			steps := make([]string, len(sequence))
			for i, step := range sequence {
				steps[i] = step.Name
			}
			return steps
		}())

	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow cross-origin for development

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("HTTP response writer does not support flushing for SSE stream")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	logger.Debug("SSE headers set, starting command sequence execution")

	// For simplicity, run steps sequentially and stream output
	for i, step := range sequence {
		stepStartTime := time.Now()

		logger.Debug("Starting sequence step",
			"step_index", i+1,
			"step_name", step.Name,
			"total_steps", len(sequence))

		// Send step name as an event
		fmt.Fprintf(w, "event: step\ndata: %s\n\n", step.Name)
		flusher.Flush()

		outChan, errChan := runner.StreamCommand(step, false) // Use cliMode false for channel output

		outputLines := 0
		errorLines := 0

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
				errorLines++
			} else {
				fmt.Fprintf(w, "event: stdout\ndata: %s\n\n", escapedLine)
				outputLines++
			}
			flusher.Flush()
		}

		// Check for errors after the command finishes
		if err := <-errChan; err != nil {
			logger.Error("Error during sequence step execution",
				"step_index", i+1,
				"step_name", step.Name,
				"error", err,
				"step_duration", time.Since(stepStartTime))

			errMsg := strings.TrimRight(err.Error(), " \t\r\n")
			escapedError := strings.ReplaceAll(errMsg, "\n", "\\n")
			fmt.Fprintf(w, "event: error\ndata: Error during step '%s': %s\n\n", step.Name, escapedError)
			flusher.Flush()
		} else {
			logger.Debug("Completed sequence step successfully",
				"step_index", i+1,
				"step_name", step.Name,
				"output_lines", outputLines,
				"error_lines", errorLines,
				"step_duration", time.Since(stepStartTime))
		}
	}

	// Send a done event when the sequence is finished
	fmt.Fprintf(w, "event: done\ndata: Sequence finished\n\n")
	flusher.Flush()

	logger.Info("Completed stack command sequence stream",
		"total_steps", len(sequence),
		"total_duration", time.Since(startTime))
}

// runHostCommand streams the output of a given host command using Server-Sent Events.
func runHostCommand(w http.ResponseWriter, step runner.HostCommandStep) {
	startTime := time.Now()

	logger.Info("Starting host command stream",
		"command_name", step.Name,
		"server_name", step.Target.ServerName,
		"is_remote", step.Target.IsRemote)

	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow cross-origin for development

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("HTTP response writer does not support flushing for host command SSE stream")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	logger.Debug("SSE headers set, starting host command execution",
		"command_name", step.Name)

	// Send step name as an event
	fmt.Fprintf(w, "event: step\ndata: %s\n\n", step.Name)
	flusher.Flush()

	outChan, errChan := runner.RunHostCommand(step, false) // Use cliMode false for channel output

	outputLines := 0
	errorLines := 0

	// Collect output and errors from channels and stream them
	for outputLine := range outChan { // Normalize line endings
		lines := strings.Split(strings.TrimRight(outputLine.Line, " \t\r\n"), "\n")
		for _, line := range lines {
			if trimmed := strings.TrimRight(line, " \t\r"); trimmed != "" {
				escapedLine := strings.ReplaceAll(trimmed, "\n", "\\n")
				if outputLine.IsError {
					fmt.Fprintf(w, "event: stderr\ndata: %s\n\n", escapedLine)
					errorLines++
				} else {
					fmt.Fprintf(w, "event: stdout\ndata: %s\n\n", escapedLine)
					outputLines++
				}
			}
		}
		flusher.Flush()
	}

	// Check for errors after the command finishes
	if err := <-errChan; err != nil {
		logger.Error("Error during host command execution",
			"command_name", step.Name,
			"server_name", step.Target.ServerName,
			"error", err,
			"duration", time.Since(startTime))

		escapedError := strings.ReplaceAll(err.Error(), "\n", "\\n")
		fmt.Fprintf(w, "event: error\ndata: Error during step '%s': %s\n\n", step.Name, escapedError)
		flusher.Flush()
	} else {
		logger.Info("Completed host command successfully",
			"command_name", step.Name,
			"server_name", step.Target.ServerName,
			"output_lines", outputLines,
			"error_lines", errorLines,
			"duration", time.Since(startTime))
	}

	// Send a done event when the command is finished
	fmt.Fprintf(w, "event: done\ndata: Command finished\n\n")
	flusher.Flush()
}

// runStackUpHandler handles requests to start a stack.
func runStackUpHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	logger.Info("Received stack up request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	stack, err := getStackFromRequest(r)
	if err != nil {
		logger.Error("Failed to get stack info for stack up request",
			"error", err,
			"remote_addr", r.RemoteAddr)
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	logger.Info("Starting stack up operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path)

	sequence := runner.UpSequence(stack)

	logger.Debug("Generated stack up sequence",
		"stack_name", stack.Name,
		"sequence_length", len(sequence),
		"preparation_duration", time.Since(startTime))

	runStackSequence(w, sequence) // Stream output
}

// runStackPullHandler handles requests to pull images for a stack.
func runStackPullHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	logger.Info("Received stack pull request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	stack, err := getStackFromRequest(r)
	if err != nil {
		logger.Error("Failed to get stack info for stack pull request",
			"error", err,
			"remote_addr", r.RemoteAddr)
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	logger.Info("Starting stack pull operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path)

	sequence := runner.PullSequence(stack)

	logger.Debug("Generated stack pull sequence",
		"stack_name", stack.Name,
		"sequence_length", len(sequence),
		"preparation_duration", time.Since(startTime))

	runStackSequence(w, sequence) // Stream output
}

// runStackDownHandler handles requests to stop a stack.
func runStackDownHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	logger.Info("Received stack down request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	stack, err := getStackFromRequest(r)
	if err != nil {
		logger.Error("Failed to get stack info for stack down request",
			"error", err,
			"remote_addr", r.RemoteAddr)
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	logger.Info("Starting stack down operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path)

	sequence := runner.DownSequence(stack)

	logger.Debug("Generated stack down sequence",
		"stack_name", stack.Name,
		"sequence_length", len(sequence),
		"preparation_duration", time.Since(startTime))

	runStackSequence(w, sequence) // Stream output
}

// runStackRefreshHandler handles requests to run the 'refresh' sequence on a stack.
func runStackRefreshHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	logger.Info("Received stack refresh request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	stack, err := getStackFromRequest(r)
	if err != nil {
		logger.Error("Failed to get stack info for stack refresh request",
			"error", err,
			"remote_addr", r.RemoteAddr)
		http.Error(w, fmt.Sprintf("Error getting stack info: %v", err), http.StatusBadRequest)
		return
	}

	logger.Info("Starting stack refresh operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path)

	sequence := runner.RefreshSequence(stack)

	logger.Debug("Generated stack refresh sequence",
		"stack_name", stack.Name,
		"sequence_length", len(sequence),
		"preparation_duration", time.Since(startTime))

	runStackSequence(w, sequence) // Stream output
}

// streamStackRefreshHandler handles GET requests to stream the 'refresh' sequence output on a stack.
// streamStackRefreshHandler serves the GET /api/stream/stack/refresh endpoint, which
// streams real-time output from the `compose ps` command to check stack status.
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
	startTime := time.Now()

	logger.Info("Received stream stack refresh request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	logger.Debug("Parsing stream stack refresh query parameters",
		"stack_name", stackName,
		"server_name", serverName)

	if stackName == "" || serverName == "" {
		logger.Error("Missing required query parameters for stream stack refresh",
			"stack_name", stackName,
			"server_name", serverName,
			"remote_addr", r.RemoteAddr)
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			logger.Error("Failed to get local root directory for stream stack refresh",
				"stack_name", stackName,
				"error", err)
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

		logger.Debug("Created local stack for stream refresh",
			"stack_name", stackName,
			"stack_path", stackPath)
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		logger.Debug("Looking up remote stack for stream refresh",
			"stack_name", stackName,
			"server_name", serverName)

		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			logger.Error("Failed to find remote stack for stream refresh",
				"stack_name", stackName,
				"server_name", serverName,
				"error", err)
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	logger.Info("Starting stream stack refresh operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path,
		"preparation_duration", time.Since(startTime))

	sequence := runner.RefreshSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackUpHandler serves the GET /api/stream/stack/up endpoint, which
// streams real-time output from the sequence of commands used to start a stack.
//
// This handler uses Server-Sent Events (SSE) to provide a continuous stream of
// command execution updates to the client as the stack is being started. The stream
// includes all output from the stack startup process.
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
	startTime := time.Now()

	logger.Info("Received stream stack up request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	logger.Debug("Parsing stream stack up query parameters",
		"stack_name", stackName,
		"server_name", serverName)

	if stackName == "" || serverName == "" {
		logger.Error("Missing required query parameters for stream stack up",
			"stack_name", stackName,
			"server_name", serverName,
			"remote_addr", r.RemoteAddr)
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			logger.Error("Failed to get local root directory for stream stack up",
				"stack_name", stackName,
				"error", err)
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

		logger.Debug("Created local stack for stream up",
			"stack_name", stackName,
			"stack_path", stackPath)
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		logger.Debug("Looking up remote stack for stream up",
			"stack_name", stackName,
			"server_name", serverName)

		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			logger.Error("Failed to find remote stack for stream up",
				"stack_name", stackName,
				"server_name", serverName,
				"error", err)
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	logger.Info("Starting stream stack up operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path,
		"preparation_duration", time.Since(startTime))

	sequence := runner.UpSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackDownHandler handles GET requests to stream output from stopping a stack.
// streamStackDownHandler serves the GET /api/stream/stack/down endpoint, which
// streams real-time output from the sequence of commands used to stop a stack.
//
// This handler uses Server-Sent Events (SSE) to provide a continuous stream of
// command execution updates to the client as the stack is being stopped. The stream
// includes all output from the stack shutdown process.
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
	startTime := time.Now()

	logger.Info("Received stream stack down request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	logger.Debug("Parsing stream stack down query parameters",
		"stack_name", stackName,
		"server_name", serverName)

	if stackName == "" || serverName == "" {
		logger.Error("Missing required query parameters for stream stack down",
			"stack_name", stackName,
			"server_name", serverName,
			"remote_addr", r.RemoteAddr)
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			logger.Error("Failed to get local root directory for stream stack down",
				"stack_name", stackName,
				"error", err)
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

		logger.Debug("Created local stack for stream down",
			"stack_name", stackName,
			"stack_path", stackPath)
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		logger.Debug("Looking up remote stack for stream down",
			"stack_name", stackName,
			"server_name", serverName)

		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			logger.Error("Failed to find remote stack for stream down",
				"stack_name", stackName,
				"server_name", serverName,
				"error", err)
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	logger.Info("Starting stream stack down operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path,
		"preparation_duration", time.Since(startTime))

	sequence := runner.DownSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackPullHandler handles GET requests to stream output from pulling images for a stack.
// streamStackPullHandler serves the GET /api/stream/stack/pull endpoint, which
// streams real-time output from the sequence of commands used to pull updated
// container images for a stack.
//
// This handler uses Server-Sent Events (SSE) to provide a continuous stream of
// command execution updates to the client as images are being pulled. The stream
// includes all output from `compose pull` and any related commands.
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
	startTime := time.Now()

	logger.Info("Received stream stack pull request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	query := r.URL.Query()
	stackName := query.Get("name")
	serverName := query.Get("serverName")

	logger.Debug("Parsing stream stack pull query parameters",
		"stack_name", stackName,
		"server_name", serverName)

	if stackName == "" || serverName == "" {
		logger.Error("Missing required query parameters for stream stack pull",
			"stack_name", stackName,
			"server_name", serverName,
			"remote_addr", r.RemoteAddr)
		http.Error(w, "Missing 'name' or 'serverName' query parameter", http.StatusBadRequest)
		return
	}

	// Adapted logic from getStackFromRequest to get stack details from query params
	var stack discovery.Stack

	if serverName == "local" {
		rootDir, err := discovery.GetComposeRootDirectory()
		if err != nil {
			logger.Error("Failed to get local root directory for stream stack pull",
				"stack_name", stackName,
				"error", err)
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

		logger.Debug("Created local stack for stream pull",
			"stack_name", stackName,
			"stack_path", stackPath)
	} else {
		// Get complete remote stack with AbsoluteRemoteRoot properly populated
		logger.Debug("Looking up remote stack for stream pull",
			"stack_name", stackName,
			"server_name", serverName)

		completeStack, err := findRemoteStackByNameAndServer(stackName, serverName)
		if err != nil {
			logger.Error("Failed to find remote stack for stream pull",
				"stack_name", stackName,
				"server_name", serverName,
				"error", err)
			http.Error(w, fmt.Sprintf("Error finding stack: %v", err), http.StatusNotFound)
			return
		}

		stack = completeStack
	}

	logger.Info("Starting stream stack pull operation",
		"stack_name", stack.Name,
		"server_name", stack.ServerName,
		"is_remote", stack.IsRemote,
		"stack_path", stack.Path,
		"preparation_duration", time.Since(startTime))

	sequence := runner.PullSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// runHostPruneHandler handles requests to clean up unused resources on a host.
// runHostPruneHandler serves the POST /api/host/prune endpoint, which executes
// the prune command on a host to clean up unused resources.
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
	startTime := time.Now()

	logger.Info("Received host prune request",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	target, err := getHostTargetFromRequest(r)
	if err != nil {
		logger.Error("Failed to get host info for host prune request",
			"error", err,
			"remote_addr", r.RemoteAddr)
		http.Error(w, fmt.Sprintf("Error getting host info: %v", err), http.StatusBadRequest)
		return
	}

	logger.Info("Starting host prune operation",
		"server_name", target.ServerName,
		"is_remote", target.IsRemote,
		"preparation_duration", time.Since(startTime))

	step := runner.PruneHostStep(target)

	logger.Debug("Generated host prune step",
		"server_name", target.ServerName,
		"command_name", step.Name)

	runHostCommand(w, step) // Stream output
}

// TODO: Implement handlers for running arbitrary commands or sequences.
// These handlers should accept JSON payloads with custom command sequences
// and execute them in the same way as the predefined operations.
