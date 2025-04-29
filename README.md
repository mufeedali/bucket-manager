# Bucket Manager (bm)

A command-line interface (CLI) and Text User Interface (TUI) to discover and manage multiple Podman Compose projects located in predefined directories (`~/bucket` or `~/compose-bucket`).

## Installation / Building

This project uses Go and provides a `justfile` for easier building and installation (requires `just` to be installed: https://github.com/casey/just).

1.  **Clone the repository (if you haven't already):**
    ```bash
    # git clone <repository-url>
    # cd bucket-manager
    ```
2.  **Install `bm` using Just:**
    This command builds the `bm` binary (which includes both CLI and TUI modes) and installs it to `~/.local/bin`. Ensure `~/.local/bin` is in your `$PATH`.
    ```bash
    just install
    ```
    *Alternative: Build locally without installing:*
    ```bash
    just build
    ```
    This creates the `bm` binary in the current directory.

## Usage

The single `bm` binary provides both a Command-Line Interface (CLI) and a Text User Interface (TUI).

- **To use the CLI:** Run `bm` followed by a command (e.g., `bm list`, `bm up my-project`).
- **To use the TUI:** Run `bm` with no arguments.

### CLI Commands

- `bm list`:
    - Lists all discovered Podman Compose projects found in `~/bucket` or `~/compose-bucket`.
    - Displays the project name and its full path.

- `bm up [project-name]`:
    - Runs `podman compose pull` and `podman compose up -d` for the specified project.
    - Brings the project's services online in detached mode.
    - Requires the project name as an argument. Tab completion is available for project names.

- `bm down [project-name]`:
    - Runs `podman compose down` for the specified project.
    - Stops and removes the project's containers and networks.
    - Requires the project name as an argument. Tab completion is available.

- `bm refresh [project-name]`:
    - Performs a full refresh cycle: `pull`, `down`, `up -d`, and `system prune` (implicitly via the runner sequence).
    - Useful for updating images and restarting services cleanly.
    - Requires the project name as an argument. Tab completion is available.

- `bm status [project-name]`:
    - Shows the status of containers for one or all projects.
    - If a `project-name` is provided, shows detailed status for that project's containers.
    - If no project name is given, shows the overall status (Up, Down, Partial, Error) for all discovered projects.
    - Tab completion is available for the optional project name argument.

**Example:**

```bash
# List all projects
bm list

# Bring up the 'actual' project (Actual Budget)
bm up actual

# Check the status of all projects
bm status

# Check the detailed status of the 'cup' project
bm status cup

# Stop the 'actual' project (Actual Budget)
bm down actual

# Refresh the 'beaver' project (Beaver Habits)
bm refresh beaver
```

### Shell Completion

`bm` supports generating shell completion scripts for various shells (bash, zsh, fish, powershell) using the built-in `completion` command.

For example, to install completions for Fish shell:

1.  Ensure the completions directory exists (`mkdir -p ~/.config/fish/completions`).
2.  Generate the completion script:
    ```bash
    bm completion fish > ~/.config/fish/completions/bm.fish
    ```
3.  Restart your Fish shell or source the file for the changes to take effect.

For other shells, replace `fish` with the appropriate shell name (e.g., `bash`, `zsh`) when running `bm completion` and consult your shell's documentation for the correct installation path.

### TUI Mode

Launch the interactive Text User Interface by running `bm` without any commands or arguments:

```bash
bm
```

The TUI provides an interactive way to view project status and trigger actions like up, down, and refresh. Refer to the TUI's interface for specific controls.

## Project Discovery

The tool looks for directories containing a `compose.yaml` or `compose.yml` file within `~/bucket` and `~/compose-bucket`. The first directory found containing projects will be used as the root.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
