# Bucket Manager (bm)

A command-line interface (CLI) and Text User Interface (TUI) to discover and manage multiple Podman Compose projects located locally or on remote hosts via SSH.

## Features

- Discover Podman Compose projects in standard (well, my standard) local directories (`~/bucket`, `~/compose-bucket`) or a custom local root.
- Discover projects in configured remote directories on SSH hosts.
- Manage SSH host configurations (add, edit, list, remove, import from `~/.ssh/config`).
- View project status (Up, Down, Partial, Error) for local and remote projects.
- Run common `podman compose` actions (`up`, `down`, `pull`, `refresh`) on individual or multiple selected projects (local or remote).
- Interactive TUI for easy navigation and management.
- CLI for scripting and quick actions.
- Shell completion support (bash, zsh, fish, powershell).

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

### Project Identifier Format

Commands like `up`, `down`, `refresh`, and `status` accept a `<project-identifier>` argument to target specific projects. This identifier can take several forms:

1.  **`project-name@server-name`**: This is the most explicit format.
    - Use `project-name@local` to target a project named `project-name` on the local machine.
    - Use `project-name@remote-host-name` to target a project named `project-name` on the configured SSH host `remote-host-name`.
    - This format guarantees targeting a single, specific project instance.

2.  **`project-name`**: When only the project name is provided (e.g., `bm status my-app`, `bm up my-app`), `bm` follows this logic to find the target project:
    - It searches the local project directory first.
    - If one or more projects named `project-name` are found locally, the **first local match** is targeted. Remote hosts are *not* searched.
    - If *no* project named `project-name` is found locally, it then searches all configured remote hosts.
    - If `project-name` is found on exactly *one* remote host, that project is targeted.
    - If `project-name` is found on *multiple* remote hosts (but not locally), an ambiguity error is reported, and you must use the explicit `project-name@server-name` format.
    - If `project-name` is not found locally or on any remote host, a "not found" error is reported.

**Note:** The TUI and `bm list` command display projects using the format `project-name (server-name)` for clarity, but the input format for commands uses `@` as the separator (e.g., `my-app@local`). Tab completion can help find the correct identifier.

### Configuration

Bucket Manager stores its configuration in `~/.config/bucket-manager/config.yaml`. This file is created automatically when needed.

You can manage the configuration using the `bm config` command.

**SSH Host Management:**

- `bm config ssh list`: Show all configured SSH hosts.
- `bm config ssh add`: Interactively add a new SSH host. You'll be prompted for:
    - Unique Name (e.g., `server1`)
    - Hostname/IP
    - SSH User
    - Port (defaults to 22)
    - Remote Root Path (optional, defaults to `~/bucket` or `~/compose-bucket` on the remote host)
    - Authentication Method (SSH Key, Agent, or Password)
- `bm config ssh edit`: Interactively edit an existing SSH host configuration.
- `bm config ssh remove`: Interactively remove an SSH host configuration.
- `bm config ssh import`: Interactively import hosts from your `~/.ssh/config` file.

**Local Project Root Management:**

By default, `bm` searches for local projects in `~/bucket` and `~/compose-bucket`. You can override this:

- `bm config set-local-root <path>`: Set a custom absolute path (or `~/path`) for local project discovery. Set to `""` (empty string) to revert to default search paths.
- `bm config get-local-root`: Show the currently configured local project root directory (or indicates default paths are being used).

### CLI Commands

- `bm list`:
    - Lists all discovered Podman Compose projects, both local and remote.
    - Displays the project name and server identifier: `project-name (server-name)`.

- `bm up <project-identifier>`:
    - Runs `podman compose pull` and `podman compose up -d` for the specified project (local or remote).
    - Use the full identifier, e.g., `bm up my-app` or `bm up api@server1`.
    - Tab completion is available.

- `bm down <project-identifier>`:
    - Runs `podman compose down` for the specified project (local or remote).
    - Use the full identifier. Tab completion is available.

- `bm refresh <project-identifier>` (alias: `bm re ...`):
    - Performs a full refresh cycle: `pull`, `down`, `up -d`.
    - If the project is local, it also runs `podman system prune -af`.
    - Use the full identifier. Tab completion is available.

- `bm status [project-identifier]`:
    - Shows the status of containers for one or all projects. The identifier is optional here. If an identifier isn't specified, status for all projects is displayed.
    - Tab completion is available for project identifiers.

- `bm config ...`: (See [Configuration](#configuration) section above)

**Example:**

```bash
# List all local and remote projects
bm list

# Bring up the 'actual' project on the local machine
bm up actual@local

# Bring up the 'api' project on the remote host named 'server1'
bm up api@server1

# Check the status of all projects
bm status

# Check the detailed status of the 'cup' project on 'server1'
bm status cup@server1

# Stop the 'actual' project
bm down actual@local

# Refresh the 'beaver' project on 'server1'
bm refresh beaver@server1

# List configured SSH hosts
bm config ssh list

# Add a new SSH host interactively
bm config ssh add
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

The TUI provides:
- An interactive list of all discovered local and remote projects.
- Real-time status updates for projects (Up, Down, Partial, Error, Loading).
- Ability to select single or multiple projects.
- Actions (Up, Down, Refresh) applicable to the selected project(s).
- Detailed view showing individual container status for a project.
- SSH configuration management screen (add, edit, list, remove, import) accessible via the `c` key.

Refer to the TUI's help bar at the bottom for specific controls within each view.

## Project Discovery

- **Local:** `bm` looks for directories containing a `compose.yaml` file within `~/bucket` and `~/compose-bucket`, or a custom path set via `bm config set-local-root`. The first directory found containing projects will be used.
- **Remote:** `bm` connects to each configured (and enabled) SSH host and searches for projects within the host's `remote_root` path (defaulting to `~/bucket` or `~/compose-bucket` on the remote machine if not specified).

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
