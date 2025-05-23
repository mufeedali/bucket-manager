// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package cli

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

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
at http://localhost:8080.

Use --dev flag for development mode, which proxies frontend requests to the Next.js
dev server running on localhost:3000 for live reloading.`,
	Run: func(cmd *cobra.Command, args []string) {
		devMode, _ := cmd.Flags().GetBool("dev")
		runWebServer(devMode)
	},
}

// runWebServer starts the HTTP server for the web UI.
// It initializes the router, registers API endpoints, and serves either the embedded
// Next.js web application or proxies to the dev server based on devMode.
func runWebServer(devMode bool) {
	// Note: SSH manager is already initialized in PersistentPreRunE of rootCmd

	router := mux.NewRouter()

	// Register API routes
	api.RegisterStackRoutes(router)
	api.RegisterSSHRoutes(router)
	api.RegisterRunnerRoutes(router)

	// Serve frontend - either embedded files or proxy to dev server
	// Must be registered after API routes to avoid conflicts
	if devMode {
		fmt.Println("Development mode: proxying frontend requests to localhost:3000")
		// Create reverse proxy to Next.js dev server
		nextJSURL, err := url.Parse("http://localhost:3000")
		if err != nil {
			log.Fatal("Failed to parse Next.js dev server URL:", err)
		}
		proxy := httputil.NewSingleHostReverseProxy(nextJSURL)
		router.PathPrefix("/").Handler(proxy)
	} else {
		// Serve static files from the embedded Next.js build output
		staticFileServer := http.FileServer(web.GetFileSystem())
		router.PathPrefix("/").Handler(staticFileServer)
	}

	port := "8080" // TODO: Make this configurable via --port flag and in config.yaml under server.port
	fmt.Printf("Starting web server on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func init() {
	serveCmd.Flags().Bool("dev", false, "Enable development mode (proxy to Next.js dev server on localhost:3000)")
	rootCmd.AddCommand(serveCmd)
}
