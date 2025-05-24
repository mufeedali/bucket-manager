# 🪣 Bucket Manager (bm)

**Bucket Manager** is a tool for managing compose stacks across local and remote machines. It offers three interfaces to fit your workflow: CLI for automation, TUI for interactive management, and a web interface for visual control.

## Features

- **Simple & Intuitive** - Works just like you'd expect it to
- **Local & Remote** - Manage stacks anywhere via SSH
- **Multiple Interfaces** - CLI, TUI, and Web UI
- **Real-time Updates** - Live status monitoring
- **Zero Configuration** - Auto-discovers your compose stacks
- **Tab Completion** - Fast command completion for the CLI

## Quick Start

1. **Install:**
   ```bash
   just install
   ```
   This installs the `bm` binary to `~/.local/bin/` (make sure this is in your `$PATH`). Actual path reference [here](https://docs.rs/dirs/latest/dirs/fn.executable_dir.html).

2. **Choose your interface:**
   - **CLI:** `bm list`, `bm up my-stack`
   - **TUI:** `bm` (with no arguments for interactive mode)
   - **Web UI:** `bm serve` then visit http://localhost:8080

## Core Features

- Control stacks (start, stop, update, refresh) individually or in bulk
- View current stack status (Up, Down, Partial, Error)
- SSH support for remote management

## Stack Commands

| Command | Description |
|---------|-------------|
| `bm list` | List all stacks |
| `bm up <stack>` | Start a stack |
| `bm down <stack>` | Stop a stack |
| `bm pull <stack>` | Pull latest images |
| `bm refresh <stack>` | Full refresh (pull, down, up) |
| `bm status [stack]` | Show status of all or specific stacks |
| `bm prune [hosts]` | Clean up unused resources |

## Stack Naming

Stacks can be referenced in three ways:

1. **Full name:** `server:stack-name` (e.g., `local:app` or `server1:api`)
2. **Short name:** `stack-name` (tries local first, then remote)
3. **Server only:** `server:` (only for `bm status`, e.g., `bm status server1:`)

Tab completion helps find the right names.

## Stack Discovery

Bucket Manager automatically discovers compose stacks in the following locations:

**Default Paths:**
- **Local:** `~/bucket` or `~/compose-bucket`
- **Remote:** Same paths on configured SSH hosts

**Custom Paths:**
- **Local:** Use `bm config set-local-root <path>` to change the search directory
- **Remote:** Configure per-host paths when adding hosts with `bm config ssh add` or `bm config ssh edit`

All locations are searched for `compose.yaml`, `compose.yml`, `docker-compose.yaml`, and `docker-compose.yml` files.

## Interfaces

### Web Interface

Run `bm serve` to start the web interface on http://localhost:8080, offering:
- Modern graphical interface for stack management
- Real-time status updates
- Remote host configuration
- Command output streaming

### TUI

The text interface (`bm` with no arguments) provides:
- Interactive navigation with keyboard shortcuts
- Multi-stack selection and operations
- Real-time status updates
- SSH configuration management (`c` key)
- Host pruning

### CLI

#### Shell Completion

Install tab completion for your shell:

```bash
# For Fish shell
mkdir -p ~/.config/fish/completions
bm completion fish > ~/.config/fish/completions/bm.fish
```

For other shells, replace `fish` with `bash`, `zsh`, or `powershell` and the appropriate path.

#### Container Runtime

Bucket Manager supports both Podman (default) and Docker as container runtimes:

```bash
# Set runtime to Docker
bm config set-runtime docker

# Set runtime to Podman (default)
bm config set-runtime podman

# Check current runtime
bm config get-runtime
```

The runtime affects all stack operations. Make sure your compose files are compatible with the chosen runtime.

#### SSH Configuration

Manage remote hosts:
- `bm config ssh list` - Show all hosts
- `bm config ssh add` - Add a new host
- `bm config ssh edit` - Edit an existing host
- `bm config ssh import` - Import from ~/.ssh/config

#### Examples

```bash
# Start a local stack
bm up myapp

# Start a stack on a remote server
bm up server1:api

# Check all stack statuses
bm status

# Check statuses on just one server
bm status server1:

# Complete refresh of a stack (pull, down, up)
bm refresh myapp

# Clean up Docker resources locally
bm prune local
```

## License

[Apache License 2.0](LICENSE)
