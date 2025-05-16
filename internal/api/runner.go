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
type StackRunRequest struct {
	Name       string `json:"name"`
	ServerName string `json:"serverName"`
}

// HostRunRequest represents the expected JSON body for host runner endpoints.
type HostRunRequest struct {
	ServerName string `json:"serverName"`
}

// RunOutput represents the output of a command execution.
type RunOutput struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// RegisterRunnerRoutes registers the API routes for running commands and actions.
func RegisterRunnerRoutes(router *mux.Router) {
	router.HandleFunc("/api/run/stack/up", runStackUpHandler).Methods("POST")
	router.HandleFunc("/api/run/stack/pull", runStackPullHandler).Methods("POST")
	router.HandleFunc("/api/run/stack/down", runStackDownHandler).Methods("POST")
	router.HandleFunc("/api/run/stack/refresh", runStackRefreshHandler).Methods("POST")

	// Streaming endpoints
	router.HandleFunc("/api/run/stack/refresh/stream", streamStackRefreshHandler).Methods("GET")
	router.HandleFunc("/api/run/stack/up/stream", streamStackUpHandler).Methods("GET")
	router.HandleFunc("/api/run/stack/down/stream", streamStackDownHandler).Methods("GET")
	router.HandleFunc("/api/run/stack/pull/stream", streamStackPullHandler).Methods("GET")

	router.HandleFunc("/api/run/host/prune", runHostPruneHandler).Methods("POST")
	// TODO: Add routes for running arbitrary commands or sequences
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
		// For remote stacks, find the host config
		cfg, err := config.LoadConfig()
		if err != nil {
			return discovery.Stack{}, fmt.Errorf("error loading config: %w", err)
		}

		var targetHost *config.SSHHost
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == req.ServerName {
				targetHost = &cfg.SSHHosts[i]
				break
			}
		}

		if targetHost == nil {
			return discovery.Stack{}, fmt.Errorf("SSH host '%s' not found", req.ServerName)
		}

		// We need the remote stack's path and absolute remote root.
		// This information is available during discovery.
		// A better approach might be to pass the full Stack object from the frontend,
		// or have a backend endpoint to get a single stack's details.
		// For now, let's create a dummy remote stack with minimal info needed by runner.
		// This is a simplification and might need refinement.
		return discovery.Stack{
			Name:       req.Name,
			ServerName: req.ServerName,
			IsRemote:   true,
			HostConfig: targetHost,
			// Path and AbsoluteRemoteRoot are needed by runner.StreamCommand
			// We might need to fetch the stack details first or pass them from frontend.
			// Let's assume for now that runner can work with just Name, ServerName, and HostConfig for remote.
			// This is a potential area for future improvement.
		}, nil
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
		cfg, err := config.LoadConfig()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
			return
		}

		var targetHost *config.SSHHost
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == serverName {
				targetHost = &cfg.SSHHosts[i]
				break
			}
		}

		if targetHost == nil {
			http.Error(w, fmt.Sprintf("SSH host '%s' not found", serverName), http.StatusBadRequest)
			return
		}

		// Assuming runner.StreamCommand can work with just Name, ServerName, and HostConfig for remote
		stack = discovery.Stack{
			Name:       stackName,
			ServerName: serverName,
			IsRemote:   true,
			HostConfig: targetHost,
		}
	}

	sequence := runner.RefreshSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackUpHandler handles GET requests to stream the 'up' sequence output on a stack.
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
		cfg, err := config.LoadConfig()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
			return
		}

		var targetHost *config.SSHHost
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == serverName {
				targetHost = &cfg.SSHHosts[i]
				break
			}
		}

		if targetHost == nil {
			http.Error(w, fmt.Sprintf("SSH host '%s' not found", serverName), http.StatusBadRequest)
			return
		}

		stack = discovery.Stack{
			Name:       stackName,
			ServerName: serverName,
			IsRemote:   true,
			HostConfig: targetHost,
		}
	}

	sequence := runner.UpSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackDownHandler handles GET requests to stream the 'down' sequence output on a stack.
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
		cfg, err := config.LoadConfig()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
			return
		}

		var targetHost *config.SSHHost
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == serverName {
				targetHost = &cfg.SSHHosts[i]
				break
			}
		}

		if targetHost == nil {
			http.Error(w, fmt.Sprintf("SSH host '%s' not found", serverName), http.StatusBadRequest)
			return
		}

		stack = discovery.Stack{
			Name:       stackName,
			ServerName: serverName,
			IsRemote:   true,
			HostConfig: targetHost,
		}
	}

	sequence := runner.DownSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// streamStackPullHandler handles GET requests to stream the 'pull' sequence output on a stack.
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
		cfg, err := config.LoadConfig()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
			return
		}

		var targetHost *config.SSHHost
		for i := range cfg.SSHHosts {
			if cfg.SSHHosts[i].Name == serverName {
				targetHost = &cfg.SSHHosts[i]
				break
			}
		}

		if targetHost == nil {
			http.Error(w, fmt.Sprintf("SSH host '%s' not found", serverName), http.StatusBadRequest)
			return
		}

		stack = discovery.Stack{
			Name:       stackName,
			ServerName: serverName,
			IsRemote:   true,
			HostConfig: targetHost,
		}
	}

	sequence := runner.PullSequence(stack)
	runStackSequence(w, sequence) // Stream output
}

// runHostPruneHandler handles requests to run the 'prune' action on a host.
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
