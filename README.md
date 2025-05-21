# Bucket Manager (bm)

Bucket Manager is a tool for managing multiple Podman Compose stacks across local and remote machines. It provides three interfaces: a command-line (CLI), a text-based UI (TUI), and a web interface.

## Quick Start

1. **Install:**
   ```bash
   just install
   ```
   This installs the `bm` binary to `~/.local/bin/` (ensure this is in your `$PATH`).

2. **Choose an interface:**
   - **CLI:** `bm list`, `bm up my-stack`
   - **TUI:** `bm` (with no arguments)
   - **Web UI:** `bm serve` and open http://localhost:8080

## Core Features

- Manage Podman Compose stacks locally and on remote SSH hosts
- Control stacks (up, down, pull, refresh) individually or in bulk
- View real-time stack status (Up, Down, Partial, Error)
- Configure SSH hosts for remote management
- System cleanup with podman prune

## Stack Commands

| Command | Description |
|---------|-------------|
| `bm list` | List all stacks |
| `bm up <stack>` | Start a stack |
| `bm down <stack>` | Stop a stack |
| `bm pull <stack>` | Pull latest images |
| `bm refresh <stack>` | Full refresh (pull, down, up) |
| `bm status [stack]` | Show status of all or specific stacks |
| `bm prune [hosts]` | Clean up Docker resources |

## Stack Naming

Stacks can be referenced in three ways:

1. **Full name:** `server:stack-name` (e.g., `local:app` or `server1:api`)
2. **Short name:** `stack-name` (tries local first, then remote)
3. **Server only:** `server:` (only for `bm status`, e.g., `bm status server1:`)

Tab completion helps find the right names.

## Additional Features

### Web Interface

Run `bm serve` to start the web interface on http://localhost:8080, offering:
- Modern graphical interface for stack management
- Real-time status updates
- Remote host configuration
- Command output streaming

### Shell Completion

Install tab completion for your shell:
```bash
# For Fish shell
mkdir -p ~/.config/fish/completions
bm completion fish > ~/.config/fish/completions/bm.fish
```

For other shells, replace `fish` with `bash`, `zsh`, or `powershell` and the appropriate path.

### TUI Mode

The text interface (`bm` with no arguments) provides:
- Interactive navigation with keyboard shortcuts
- Multi-stack selection and operations
- Real-time status updates
- SSH configuration management (`c` key)
- Host pruning

### SSH Configuration

Manage remote hosts:
- `bm config ssh list` - Show all hosts
- `bm config ssh add` - Add a new host
- `bm config ssh edit` - Edit an existing host
- `bm config ssh import` - Import from ~/.ssh/config

### Stack Discovery

- **Local:** Finds `compose.yaml`/`compose.yml` files in `~/bucket` or `~/compose-bucket`
- **Remote:** Searches the same paths on configured SSH hosts
- **Custom paths:** Set with `bm config set-local-root <path>`

## Examples

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
