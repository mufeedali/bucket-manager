// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"fmt"
	"log"
	"net/http"

	"bucket-manager/internal/api"
	"bucket-manager/internal/web"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
)

// serveCmd represents the command to start the web server for the bucket manager
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server for Bucket Manager",
	Long: `Starts an HTTP server that serves the Bucket Manager web UI and API.
This provides a modern web interface for managing all your Podman Compose stacks
from any browser. The server runs on localhost by default and can be accessed
at http://localhost:8080.`,
	Run: func(cmd *cobra.Command, args []string) {
		runWebServer()
	},
}

// runWebServer starts the HTTP server for the web UI.
// It initializes the router, registers API endpoints, and serves the embedded
// Next.js web application.
func runWebServer() {
	// Note: SSH manager is already initialized in PersistentPreRunE of rootCmd

	router := mux.NewRouter()

	// Register API routes
	api.RegisterStackRoutes(router)
	api.RegisterSSHRoutes(router)
	api.RegisterRunnerRoutes(router)

	// Serve static files from the embedded Next.js build output
	// Must be registered after API routes to avoid conflicts
	staticFileServer := http.FileServer(web.GetFileSystem())
	router.PathPrefix("/").Handler(staticFileServer)

	port := "8080" // TODO: Make this configurable via --port flag and in config.yaml under server.port
	fmt.Printf("Starting web server on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
