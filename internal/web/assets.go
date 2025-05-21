// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package web provides access to embedded web UI assets built with Next.js.
// It handles serving the web interface's static files that are embedded
// into the binary at build time using Go's embed feature.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// embeddedFiles contains the entire web UI build output embedded in the binary.
// The //go:embed directive instructs the Go compiler to include the specified files.
//
//go:embed all:assets
var embeddedFiles embed.FS

// GetFileSystem returns an http.FileSystem that serves the embedded web assets.
// This allows the application to serve the web UI without requiring external files.
func GetFileSystem() http.FileSystem {
	webUI, err := fs.Sub(embeddedFiles, "assets")
	if err != nil {
		panic(err)
	}
	return http.FS(webUI)
}
