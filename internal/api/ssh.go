package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bucket-manager/internal/config"

	"github.com/gorilla/mux"
)

// RegisterSSHRoutes registers the API routes for SSH configurations.
func RegisterSSHRoutes(router *mux.Router) {
	router.HandleFunc("/api/ssh/hosts", listSSHHostsHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts", addSSHHostHandler).Methods("POST")
	router.HandleFunc("/api/ssh/hosts/{name}", getSSHHostHandler).Methods("GET")
	router.HandleFunc("/api/ssh/hosts/{name}", updateSSHHostHandler).Methods("PUT")
	router.HandleFunc("/api/ssh/hosts/{name}", deleteSSHHostHandler).Methods("DELETE")
	router.HandleFunc("/api/ssh/import", importSSHHostsHandler).Methods("POST")
}

// listSSHHostsHandler handles requests to list all SSH hosts.
func listSSHHostsHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg.SSHHosts)
}

// addSSHHostHandler handles requests to add a new SSH host.
func addSSHHostHandler(w http.ResponseWriter, r *http.Request) {
	var newHost config.SSHHost
	if err := json.NewDecoder(r.Body).Decode(&newHost); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	// TODO: Add validation for the new host

	cfg.SSHHosts = append(cfg.SSHHosts, newHost)

	if err := config.SaveConfig(cfg); err != nil {
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newHost)
}

// getSSHHostHandler handles requests to get details of a specific SSH host.
func getSSHHostHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostName := vars["name"]

	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	for _, host := range cfg.SSHHosts {
		if host.Name == hostName {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(host)
			return
		}
	}

	http.Error(w, "SSH host not found", http.StatusNotFound)
}

// updateSSHHostHandler handles requests to update an existing SSH host.
func updateSSHHostHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostName := vars["name"]

	var updatedHost config.SSHHost
	if err := json.NewDecoder(r.Body).Decode(&updatedHost); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	found := false
	for i, host := range cfg.SSHHosts {
		if host.Name == hostName {
			// TODO: Add validation for the updated host
			cfg.SSHHosts[i] = updatedHost
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "SSH host not found", http.StatusNotFound)
		return
	}

	if err := config.SaveConfig(cfg); err != nil {
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedHost)
}

// deleteSSHHostHandler handles requests to delete an SSH host.
func deleteSSHHostHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostName := vars["name"]

	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading config: %v", err), http.StatusInternalServerError)
		return
	}

	newSSHHosts := []config.SSHHost{}
	found := false
	for _, host := range cfg.SSHHosts {
		if host.Name == hostName {
			found = true
			continue
		}
		newSSHHosts = append(newSSHHosts, host)
	}

	if !found {
		http.Error(w, "SSH host not found", http.StatusNotFound)
		return
	}

	cfg.SSHHosts = newSSHHosts

	if err := config.SaveConfig(cfg); err != nil {
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// importSSHHostsHandler handles requests to import SSH hosts from a file.
func importSSHHostsHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement SSH host import logic
	http.Error(w, "SSH import not yet implemented", http.StatusNotImplemented)
}
