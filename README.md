# Docktop

**A lightweight, btop-inspired Docker terminal dashboard with MCP catalog browsing.**

Docktop uses the Docker CLI already installed on your machine. It works with the active `docker context` (including rootless and remote contexts); it does **not** require Docker Desktop, its VM, a web UI, or a resident Docktop daemon.

> **Status: early MVP.** Containers, images, networks, volumes, resource usage, bounded logs, and MCP catalog listing are implemented. Compose management, image pull/push, and advanced MCP profile management are not yet implemented.

## Requirements

- Linux or macOS terminal (developed with Fedora in mind)
- Go 1.23+
- Docker CLI with access to a working Docker Engine
- Optional: `docker mcp` plugin and the Docker MCP catalog for the MCP screen

## Run

```bash
git clone https://github.com/tobiasGuta/Docktop.git
cd Docktop
go mod tidy
go run ./cmd/docktop
```

For a standalone binary:

```bash
go build -o docktop ./cmd/docktop
./docktop
```

The first build downloads the Bubble Tea and Lip Gloss Go modules. No Docker state is changed on startup.

## Keyboard controls

| Key | Function |
| --- | --- |
| `1`–`6`, `Tab` / `Shift+Tab` | Switch dashboard, containers, images, networks, volumes, MCP catalog |
| `↑`/`↓` or `j`/`k` | Navigate rows or scroll catalog/logs |
| `r` | Refresh active view |
| `l` | Read selected container's last 100 log lines |
| `s` | Start or stop selected container, with confirmation |
| `R` | Restart selected container, with confirmation |
| `y` / `n` | Confirm / cancel a container action |
| `/` | Filter MCP catalog servers by name; Enter to search |
| `?` | Help |
| `Esc` | Dismiss logs, help, or prompts |
| `q`, `Ctrl+C` | Exit |

The dashboard refreshes every five seconds. MCP catalog data loads on demand (when entering its tab or pressing `r`) and is **read-only**; Docktop does not install or launch MCP servers.

To prepare MCP catalog browsing on Docker Engine:

```bash
docker mcp --help
docker mcp catalog pull mcp/docker-mcp-catalog:latest
```

## Design and security

- Executes Docker CLI arguments directly, **never through a shell**, preserving the user's Docker context and configuration.
- Performs read-only inspection by default. Container `start`, `stop`, and `restart` require an explicit `y` confirmation, and use validated container IDs.
- No Docker socket forwarding, credential storage, automatic MCP execution, or telemetry.
- Truncates logs and terminal output, strips ANSI control codes, and bounds CLI calls with timeouts.
- Docker daemon access is privileged. **Do not run Docktop as root** or expose the Docker socket to untrusted workloads. Docktop cannot enforce isolation beyond permissions Docker already grants.

## Roadmap

1. Improve layout responsiveness, selection and resource graphs.
2. Add safe container inspections, Compose projects, and image management.
3. Add MCP server inspection, profiles, tools and gateway management with granular permissions.
4. Add regression testing against a disposable Docker fixture and benchmark idle resource usage against other TUIs.

Contributions and bug reports are welcome.
