// Package web provides access to embedded web UI assets
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:assets
var embeddedFiles embed.FS

// GetFileSystem returns an http.FileSystem that serves the embedded web assets
func GetFileSystem() http.FileSystem {
	webUI, err := fs.Sub(embeddedFiles, "assets")
	if err != nil {
		panic(err)
	}
	return http.FS(webUI)
}
