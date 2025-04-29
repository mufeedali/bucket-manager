package main // CLI entry point is in package main

// Main entry point for the CLI application.
// This is kept separate to allow for testing and potentially
// different main packages (e.g., one for CLI, one for TUI).
// However, for building a single binary, this might be moved
// to the root main.go later. For now, we build separate binaries.

func main() {
	Execute()
}
