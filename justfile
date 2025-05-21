# Justfile for Bucket Manager - provides easy build, install, and clean commands
# Use 'just <command>' to run commands, e.g., 'just install'

# Default task (runs when 'just' is called with no arguments)
default: install

# Build the bm binary and install it to the user's preferred directory

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
    cp bm {{ INSTALL_DIR }}/
    @echo "Installation complete. 'bm' is now available in {{ INSTALL_DIR }}"
    @{{ just_executable() }} clean
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
    go build -o bm ./cmd/bm
    @echo "Build complete: ./bm"

# Clean build artifacts and temporary files
clean:
    @echo "Cleaning build artifacts..."
    rm -f bm
    find internal/web/assets -mindepth 1 -not -name ".gitkeep" -delete
    @echo "Build artifacts cleaned"
