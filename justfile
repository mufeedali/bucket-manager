# Justfile for Bucket Manager - provides easy build, install, and clean commands
# Use 'just <command>' to run commands, e.g., 'just install'

# Default task (runs when 'just' is called with no arguments)
default: install

# Build the bm binary and install it to the user's preferred directory

BUILD_BINARY := "build/bm"
INSTALL_DIR := executable_directory()
PATH_VALUE := env("PATH")
SHELL_PATH := env("SHELL", "/bin/sh")
CURRENT_SHELL_NAME := file_name(SHELL_PATH)

# Regex pattern to check if INSTALL_DIR is in PATH

path_contains_pattern := ".*" + INSTALL_DIR + ".*"

# Message to be displayed when INSTALL_DIR is in PATH

path_success_message := GREEN + "✓ All done! You can now simply run " + BOLD + CYAN + "bm" + NORMAL + GREEN + " from any directory!" + NORMAL

# Message to assist users in adding the INSTALL_DIR to their PATH.
# I use and like fish, so I made this a bit more fish-friendly.

path_setup_instruction := if CURRENT_SHELL_NAME == "fish" { BOLD + CYAN + "fish_add_path \"" + INSTALL_DIR + "\"" + NORMAL } else { BOLD + CYAN + "export PATH=\"" + INSTALL_DIR + ":$PATH\"" + NORMAL + "\n" + ITALIC + "You may want to add this to your shell's configuration file (e.g., ~/.bashrc, ~/.zshrc) for persistence." + NORMAL }

# Message for when INSTALL_DIR is NOT in PATH

path_failure_message := YELLOW + "⚠ All done! But " + BOLD + INSTALL_DIR + NORMAL + YELLOW + " is not in your PATH." + NORMAL + "\n" + WHITE + "To use " + BOLD + CYAN + "bm" + NORMAL + WHITE + " from any directory, add it to your PATH by running:" + NORMAL + "\n" + path_setup_instruction
conditional_output_message := if PATH_VALUE =~ path_contains_pattern { path_success_message } else { path_failure_message }

# This is the main installation command that builds both web UI and Go binary
install: build
    @echo "Using installation directory: {{ INSTALL_DIR }}"
    @echo "Ensuring directory exists..."
    mkdir -p {{ INSTALL_DIR }}
    @echo "Copying bm to {{ INSTALL_DIR }}..."
    cp {{ BUILD_BINARY }} {{ INSTALL_DIR }}/
    @echo "Installation complete. 'bm' is now available in {{ INSTALL_DIR }}"
    @{{ just_executable() }} cleanup
    @echo "{{ conditional_output_message }}"

# Build the Next.js web UI for embedding in the Go binary
build-web:
    @echo "Building web UI..."
    cd web && bun install && bun run build
    @echo "Copying web UI build output to embedded directory..."
    cp -r web/out/* internal/web/assets/

# Simple build task (creates binary in current directory)
build: build-web
    @echo "Building bm binary..."
    go build -o {{ BUILD_BINARY }} ./cmd/bm
    @echo "Build complete: ./{{ BUILD_BINARY }}"

# Development commands for the web UI
# ===================================

# Development with tmux (starts both servers in one terminal)
dev:
    @echo "Starting development environment with tmux..."
    go build -o {{ BUILD_BINARY }} ./cmd/bm
    -tmux -V
    -tmux kill-session -t bucket-manager-dev 2>/dev/null
    tmux new-session -d -s bucket-manager-dev -c "web" "bun dev"
    @echo "Development environment will start at http://localhost:8080"
    @echo "- Use Ctrl+B then 0/1 to switch windows, Ctrl+B then d to detach"
    @echo "- Commands: tmux attach/kill-session -t bucket-manager-dev"
    sleep 3
    tmux new-window -t bucket-manager-dev -c "." "./{{ BUILD_BINARY }} serve --dev"
    tmux select-window -t bucket-manager-dev:0
    tmux attach -t bucket-manager-dev
    @echo "Make sure you run 'just dev-cleanup' once done :)"

# Start just the Next.js dev server (for use in separate terminal)
dev-frontend:
    @echo "Starting Next.js dev server on http://localhost:3000"
    @echo "Make sure you run 'just dev-cleanup' once done :)"
    cd web && bun dev

# Start just the Go backend in dev mode (for use in separate terminal)
dev-backend:
    @echo "Building Go backend for development..."
    go build -o {{ BUILD_BINARY }} ./cmd/bm
    @echo "Starting Go backend in dev mode (proxying to http://localhost:3000)"
    @echo "Access the application at http://localhost:8080"
    @echo "Make sure you run 'just dev-cleanup' once done :)"
    ./{{ BUILD_BINARY }} serve --dev

# Show development mode instructions
dev-help:
    @echo "Development Workflow Options"
    @echo "==========================="
    @echo ""
    @echo "Option 1: Using tmux (recommended)"
    @echo "  Command: just dev"
    @echo "  Requirements: tmux must be installed"
    @echo "  Benefits: Both servers in one terminal with split windows"
    @echo ""
    @echo "  Controls:"
    @echo "  - Ctrl+B then 0 = Next.js window"
    @echo "  - Ctrl+B then 1 = Go backend window"
    @echo "  - Ctrl+B then d = Detach (servers keep running)"
    @echo ""
    @echo "  Management commands:"
    @echo "  - tmux attach -t bucket-manager-dev    (reattach to session)"
    @echo "  - tmux kill-session -t bucket-manager-dev  (stop servers)"
    @echo ""
    @echo "Option 2: Separate terminals"
    @echo "  Terminal 1: just dev-frontend    (starts Next.js dev server)"
    @echo "  Terminal 2: just dev-backend     (starts Go backend in dev mode)"
    @echo ""
    @echo "Both options provide:"
    @echo "- Live UI reloading (changes visible immediately)"
    @echo "- Working API endpoints"
    @echo "- Access via http://localhost:8080"
    @echo ""
    @echo "To install tmux if needed:"
    @echo "  - Debian/Ubuntu: sudo apt install tmux"
    @echo "  - Fedora: sudo dnf install tmux"
    @echo "  - Arch: sudo pacman -S tmux"
    @echo "  - macOS: brew install tmux"

# Quick development build without installation
dev-build:
    @echo "Quick development build (no install)..."
    cd web && bun install && bun run build
    cp -r web/out/* internal/web/assets/
    go build -o {{ BUILD_BINARY }} ./cmd/bm
    @echo "Development build complete. Run './{{ BUILD_BINARY }} serve' to test."

# Clean up build artifacts and temporary files
cleanup:
    @echo "Cleaning up build artifacts..."
    rm -f {{ BUILD_BINARY }}
    find internal/web/assets -mindepth 1 -not -name ".gitkeep" -delete
    @echo "Build artifacts cleaned"

# Clean up after a development session
dev-cleanup:
    @echo "Cleaning up development artifacts and environment..."
    -tmux kill-session -t bucket-manager-dev 2>/dev/null
    rm -f {{ BUILD_BINARY }}
    @echo "Development artifacts and environment cleaned"
