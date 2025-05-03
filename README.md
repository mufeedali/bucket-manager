# Bucket Manager (bm)

A command-line interface (CLI) and Text User Interface (TUI) to discover and manage multiple Podman Compose stacks located locally or on remote hosts via SSH.

## Features

- Discover Podman Compose stacks in standard (well, my standard) local directories (`~/bucket`, `~/compose-bucket`) or a custom local root.
- Discover stacks in configured remote directories on SSH hosts.
- Manage SSH host configurations (add, edit, list, remove, import from `~/.ssh/config`).
- View stack status (Up, Down, Partial, Error) for local and remote stacks.
- Run common `podman compose` actions (`up`, `down`, `pull`, `refresh`) on individual or multiple selected stacks (local or remote).
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

- **To use the CLI:** Run `bm` followed by a command (e.g., `bm list`, `bm up my-stack`).
- **To use the TUI:** Run `bm` with no arguments.

### Stack Identifier Format

Commands like `up`, `down`, `refresh`, and `status` accept a `<stack-identifier>` argument to target specific stacks. This identifier can take several forms:

1.  **`server-name:stack-name`**: This is the most explicit format for remote stacks.
    - Use `local:stack-name` to target a stack named `stack-name` on the local machine.
    - Use `remote-host-name:stack-name` to target a stack named `stack-name` on the configured SSH host `remote-host-name`.
    - This format guarantees targeting a single, specific stack instance.

2.  **`stack-name`**: When only the stack name is provided (e.g., `bm status my-app`, `bm up my-app`), `bm` follows this logic to find the target stack:
    - It searches the local stack directory first.
    - If one or more stacks named `stack-name` are found locally, the **first local match** is targeted. Remote hosts are *not* searched.
    - If *no* stack named `stack-name` is found locally, it then searches all configured remote hosts.
    - If `stack-name` is found on exactly *one* remote host, that stack is targeted.
    - If `stack-name` is found on *multiple* remote hosts (but not locally), an ambiguity error is reported, and you must use the explicit `server-name:stack-name` format.
    - If `stack-name` is not found locally or on any remote host, a "not found" error is reported.

3.  **`server-name:`** (Note the trailing colon): Used *only* with the `bm status` command to show the status of *all* stacks on a specific remote host (e.g., `bm status server1:`).

**Note:** The TUI and `bm list` command display stacks using the format `stack-name (server-name)` for clarity, but the input format for commands uses `:` as the separator (e.g., `local:my-app`, `server1:api`). Tab completion can help find the correct identifier.

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

**Local Stack Root Management:**

By default, `bm` searches for local stacks in `~/bucket` and `~/compose-bucket`. You can override this:

- `bm config set-local-root <path>`: Set a custom absolute path (or `~/path`) for local stack discovery. Set to `""` (empty string) to revert to default search paths.
- `bm config get-local-root`: Show the currently configured local stack root directory (or indicates default paths are being used).

### CLI Commands

- `bm list`:
    - Lists all discovered Podman Compose stacks, both local and remote.
    - Displays the stack name and server identifier: `stack-name (server-name)`.

- `bm up <stack-identifier>`:
    - Runs `podman compose pull` and `podman compose up -d` for the specified stack (local or remote).
    - Use the identifier format, e.g., `bm up my-app` or `bm up server1:api`.
    - Tab completion is available.

- `bm down <stack-identifier>`:
    - Runs `podman compose down` for the specified stack (local or remote).
    - Use the identifier format. Tab completion is available.

- `bm refresh <stack-identifier>` (alias: `bm re ...`):
    - Performs a full refresh cycle: `pull`, `down`, `up -d`.
    - If the stack is local, it also runs `podman system prune -af`.
    - Use the identifier format. Tab completion is available.

- `bm status [stack-identifier]`:
    - Shows the status of containers for one or all stacks.
    - If no identifier is provided, shows status for all discovered stacks.
    - If `stack-name` or `server:stack-name` is provided, shows status for that specific stack.
    - If `server:` (with a trailing colon) is provided, shows status for all stacks on that specific remote server.
    - Tab completion is available for stack identifiers.

- `bm config ...`: (See [Configuration](#configuration) section above)

**Example:**

```bash
# List all local and remote stacks
bm list

# Bring up the 'actual' stack on the local machine (implicitly local)
bm up actual
# Or explicitly:
bm up local:actual

# Bring up the 'api' stack on the remote host named 'server1'
bm up server1:api

# Check the status of all stacks
bm status

# Check the detailed status of the 'cup' stack on 'server1'
bm status server1:cup

# Check the status of ALL stacks on 'server1'
bm status server1:

# Stop the 'actual' stack
bm down local:actual

# Refresh the 'beaver' stack on 'server1'
bm refresh server1:beaver

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
- An interactive list of all discovered local and remote stacks.
- Real-time status updates for stacks (Up, Down, Partial, Error, Loading).
- Ability to select single or multiple stacks.
- Actions (Up, Down, Refresh) applicable to the selected stack(s).
- Detailed view showing individual container status for a stack.
- SSH configuration management screen (add, edit, list, remove, import) accessible via the `c` key.

Refer to the TUI's help bar at the bottom for specific controls within each view.

## Stack Discovery

- **Local:** `bm` looks for directories containing a `compose.yaml` file within `~/bucket` and `~/compose-bucket`, or a custom path set via `bm config set-local-root`. Each such directory represents a stack. The first root directory found containing stacks will be used.
- **Remote:** `bm` connects to each configured (and enabled) SSH host and searches for stacks within the host's `remote_root` path (defaulting to `~/bucket` or `~/compose-bucket` on the remote machine if not specified).

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
