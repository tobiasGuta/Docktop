# Docktop

**A lightweight, btop-inspired terminal dashboard for Docker Engine and the Docker MCP Catalog.**

Docktop runs on your existing Docker CLI and active Docker context. Unlike Docker Desktop for Linux, it does not start a separate VM or background Docktop service. Designed for Fedora; compatible with other Linux and macOS terminals.

## What works (v0.2)

- Full-screen, responsive two-panel interface with keyboard navigation, item details, and scrollable lists.
- Docker containers, images, networks, and volumes, with full identifiers and metadata in the details panel.
- Recent Docker container CPU and memory history in bounded sparklines (60 samples; refresh ~5 seconds). CPU is the sum of running-container percentages: 100% equals one CPU core, **not** total host CPU. Memory is the sum of Docker-reported running-container usage.
- Container CPU, memory, network I/O and block I/O readings where Docker supplies them.
- Last 100 container log lines (bounded and sanitized), start/stop/restart **only after explicit confirmation**.
- Read-only MCP catalog browser with client-side search of server names, titles, and descriptions, and a detail pane showing descriptions, tool counts, image/endpoint information.
- Responsive single-panel mode for narrow terminals. No Docker Desktop or web interface required.

Not yet implemented: Compose operations, image pulls, container creation, MCP activation/installation, full per-container metric plots, and benchmarks against alternative Docker tools.

## Requirements

- Go 1.23+ for building
- `docker` CLI with an accessible Docker Engine (including rootless and remote contexts)
- Optional: Docker MCP CLI plugin and pulled `mcp/docker-mcp-catalog:latest`

## Run

```bash
git clone https://github.com/tobiasGuta/Docktop.git
cd Docktop
go mod tidy
go run ./cmd/docktop
```

Or build a standalone binary:

```bash
go build -o docktop ./cmd/docktop
./docktop
```

No Docker state is modified during startup or by browsing dashboards. The first Go build downloads Bubble Tea and Lip Gloss dependencies.

## Keyboard controls

| Key | Function |
| --- | --- |
| `1`–`6`, `Tab` / `Shift+Tab` | Switch dashboard, containers, images, networks, volumes and MCP |
| `↑`/`↓` or `j`/`k`, `PgUp`/`PgDn`, `Home`/`End` | Navigate lists and logs |
| `Enter` | Open selected item details (useful in narrow terminals) |
| `r` | Refresh Docker view or reload MCP catalog |
| `l` | Show last 100 log lines for selected container (Containers tab) |
| `s` | Start or stop selected container, with explicit confirmation |
| `R` | Restart selected container, with explicit confirmation |
| `y` / `n` | Confirm or cancel a state change |
| `/` | Search MCP names, titles and descriptions; `Ctrl+U` clears the query |
| `?` | Show help; `Esc` closes prompts or details |
| `q`, `Ctrl+C` | Exit |

Search on the MCP page is local and does not re-fetch the full catalog on every keystroke. To access MCP browsing, prepare the catalog separately:

```bash
docker mcp --help
docker mcp catalog pull mcp/docker-mcp-catalog:latest
```

## Design and security

- Docker CLI arguments are passed directly without a shell. The active Docker context and Docker permissions are honored.
- Read-only discovery is the default. Container state changes use validated IDs and ask for a `y` confirmation.
- No telemetry, credential storage, background Docktop daemon, socket forwarding, or MCP execution.
- External text is sanitized; CLI commands are timeout-limited, logs are bounded, and metric history is capped to 60 samples.
- **Docker daemon access is highly privileged.** Do not run Docktop as root or mount the Docker socket into untrusted workloads. Confirmation prompts are guardrails, not an OS security boundary.
- Docker resource metrics are sourced from `docker stats` and do not represent whole-host CPU or RAM usage.

## Development

```bash
go test ./...
go vet ./...
go build ./cmd/docktop
```

The unit tests use fake Docker responses; they never interact with your local Docker daemon or running containers.
