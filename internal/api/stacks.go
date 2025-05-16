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
type StackWithStatus struct {
	discovery.Stack
	Status runner.StackStatus `json:"status"`
}

// collectStacksWithStatus retrieves status for a slice of stacks concurrently
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

func RegisterStackRoutes(router *mux.Router) {
	router.HandleFunc("/api/stacks/local", listLocalStacksHandler).Methods("GET")
	router.HandleFunc("/api/stacks/local/{name}/status", getLocalStackStatusHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts/{hostName}/stacks", listRemoteStacksHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts/{hostName}/stacks/{name}/status", getRemoteStackStatusHandler).Methods("GET")
}

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
