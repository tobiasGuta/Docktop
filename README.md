# Docktop

**A lightweight, btop-inspired terminal dashboard for Docker Engine and the Docker MCP Catalog.**

Docktop runs on your existing Docker CLI and active Docker context. Unlike Docker Desktop for Linux, it does not start a separate VM or background Docktop service. Designed for Fedora; compatible with other Linux and macOS terminals.

## What works (v0.4 visual refresh)

- btop-inspired multi-panel dashboard with a CPU history graph, memory history, cumulative network/block I/O counters, Docker Engine summary, and a dense container list. Two-panel inspection pages remain available for detailed browsing.
- Transparent panels that inherit the terminal background, with high-contrast borders and an adaptive compact dashboard on narrow terminals. Graph y-axes scale to observed peaks and show their true maxima.
- Docker containers, images, networks, and volumes, with full identifiers and metadata in the details panel.
- Recent Docker container CPU and memory history in bounded plots (60 samples; refresh ~5 seconds). CPU is the sum of running-container percentages: 100% equals one CPU core, **not** total host CPU. Memory is the sum of Docker-reported running-container usage.
- Container CPU, memory, network I/O and block I/O readings where Docker supplies them.
- Last 100 container log lines (bounded and sanitized), start/stop/restart **only after explicit confirmation**.
- Read-only MCP catalog browser with client-side search of server names, titles, and descriptions, and a detail pane showing descriptions, tool counts, image/endpoint information.
- Responsive single-panel mode for narrow terminals. No Docker Desktop or web interface required.

Also supported since v0.3:

- **Compose discovery** (`7`): on-demand, read-only `docker compose ls --all` project list, statuses and config paths; requires Docker Compose CLI plugin.
- **Docker Hub search** (`8`): press `/` to search the public catalog on demand and browse descriptions, stars and official status. Press `p` to request an image pull, which displays the target and requires explicit `y` confirmation. Pulls may take several minutes and download sizable data.
- **Deep container inspection** (`2`, then `Enter`): health, mount attachments and networks. Environment *keys* are displayed with **all values redacted**, not only those with obvious secret names. `Enter` also discovers selected network attachments and volume usage on demand. These queries do not modify Docker resources.
- **MCP profile visibility** (`6`, then `p`): on-demand, read-only list of configured profiles; no activation, credential access or server execution.
- **Measurement helper**: `scripts/measure-process.sh PID` reads the RSS and process-lifetime CPU average of an already running Docktop process. Does not measure Docker containers, and does not claim a comparative benchmark.

The Docker I/O panel shows cumulative counters as reported by `docker stats`, **not bytes per second**. The CPU figure sums running-container usage (100% = one CPU core), not whole-host utilization. A dashboard graph with a low but nonzero reading is autoscaled and labelled; its apparent height is not the percent of total host CPU.

Not yet implemented: Compose management operations, container creation, advanced image management, MCP activation/installation, full per-container metric plots, or controlled comparative benchmarks against other tools. The UI preserves the terminal's transparency instead of forcing an opaque panel background. The selected-row highlight remains intentional.

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
| `1`–`8`, `Tab` / `Shift+Tab` | Switch Dashboard, Containers, Images, Networks, Volumes, MCP, Compose and Hub |
| `↑`/`↓` or `j`/`k`, `PgUp`/`PgDn`, `Home`/`End` | Navigate lists and logs |
| `Enter` | Show details; on Containers, Networks and Volumes requests an additional read-only inspection |
| `r` | Refresh Docker view or reload MCP catalog |
| `l` | Show last 100 log lines for selected container (Containers tab) |
| `s` | Start or stop selected container, with explicit confirmation |
| `R` | Restart selected container, with explicit confirmation |
| `y` / `n` | Confirm or cancel a state change |
| `/` | Filter MCP catalog locally; on Docker Hub, enter a search term and press Enter to search online. `Ctrl+U` clears inputs |
| `p` | MCP tab: toggle profiles (read-only). Docker Hub: request selected image pull (requires confirmation) |
| `?` | Show help; `Esc` closes prompts or details |
| `q`, `Ctrl+C` | Exit |

Search on the MCP page is local and does not re-fetch the full catalog on every keystroke. To access MCP browsing, prepare the catalog separately:

```bash
docker mcp --help
docker mcp catalog pull mcp/docker-mcp-catalog:latest
```

## Design and security

- Docker CLI arguments are passed directly without a shell. The active Docker context and Docker permissions are honored.
- Read-only discovery is the default. Container state changes use validated IDs and ask for a `y` confirmation. Image pulls use validated Docker Hub image names, pin the Docker context captured during review and require the same `y` confirmation.
- No telemetry, credential storage, background Docktop daemon, socket forwarding, or MCP execution.
- External text is sanitized; CLI commands are timeout-limited, logs are bounded, and metric history is capped to 60 samples.
- **Docker daemon access is highly privileged.** Do not run Docktop as root or mount the Docker socket into untrusted workloads. Confirmation prompts are guardrails, not an OS security boundary.
- Docker resource metrics are sourced from `docker stats` and do not represent whole-host CPU or RAM usage.

## Measure Docktop process overhead

After building (`go build -o docktop ./cmd/docktop`), launch `./docktop` in a terminal. In a second terminal, identify its PID with `pgrep -a -x docktop` and run:

```bash
./scripts/measure-process.sh <PID>
```

The script takes three samples, two seconds apart, using `/proc` and `ps`. RSS reflects Docktop's process memory, **not** the container workloads. CPU shown by `ps` is the process-lifetime average, not a point-in-time idle benchmark. Compare against other tools only with equivalent terminal sizes, refresh settings, loaded resources, and measurement methods.

## Development

```bash
go test ./...
go vet ./...
go build ./cmd/docktop
```

The unit tests use fake Docker responses; they never interact with your local Docker daemon or running containers.
