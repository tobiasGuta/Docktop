# Docktop

**A lightweight, btop-inspired terminal dashboard for Docker Engine and the Docker MCP Catalog.**

Docktop runs on your existing Docker CLI and active Docker context. Unlike Docker Desktop for Linux, it does not start a separate VM or background Docktop service. Designed for Fedora; compatible with other Linux and macOS terminals.

## What works (v0.14 reviewed MCP client connection)

- btop-inspired multi-panel dashboard with container CPU and memory history, per-second container network/block I/O rates, a **separate local Linux host CPU/RAM/GPU summary**, and a dense Docker container list. Two-panel inspection pages remain available for detailed browsing.
- Transparent panels that inherit the terminal theme and an adaptive compact dashboard on narrow terminals. Graph y-axes scale to observed peaks and show their true maxima.
- Docker containers, images, networks, and volumes, with full identifiers and metadata in the details panel.
- Recent Docker container CPU and memory history in bounded plots (60 samples; refresh ~5 seconds). CPU is the sum of running-container percentages: 100% equals one CPU core, **not** total host CPU. Memory is the sum of Docker-reported running-container usage.
- Container CPU, memory, network I/O and block I/O readings where Docker supplies them.
- Last 100 container log lines (bounded and sanitized), start/stop/restart **only after explicit confirmation**.
- Read-only MCP catalog browser with client-side search of server names, titles, and descriptions. Press Enter to inspect sanitized catalog metadata and any advertised tool names and descriptions; the catalog may not include every live tool. MCP, Compose and Hub now have compact, transparent status ribbons with navigation hints.
- Responsive single-panel mode for narrow terminals. No Docker Desktop or web interface required.

Also supported since v0.3:

- **Reviewed image-tag removal** (`3`, select a tagged image, `d`): shows exact repository, tag, image ID and Docker context; requires `y` confirmation; rereads the active context and target image reference/ID before invoking ordinary `docker image rm` without force. Readback checks that the selected tag is absent, reporting blocked or unverified results when appropriate. Dangling/untagged images are intentionally unsupported; removing one tag need not reclaim the underlying image layers.
- **Compose discovery** (`7`): on-demand, read-only `docker compose ls --all` project list, statuses and config paths; requires Docker Compose CLI plugin.
- **Docker Hub search** (`8`): press `/` to search the public catalog on demand and browse descriptions, stars and official status. Press `p` to request an image pull, which displays the target and requires explicit `y` confirmation. Pulls may take several minutes and download sizable data.
- **Deep container inspection** (`2`, then `Enter`): health, mount attachments and networks. Environment *keys* are displayed with **all values redacted**, not only those with obvious secret names. `Enter` also discovers selected network attachments and volume usage on demand. These queries do not modify Docker resources.
- **MCP profile visibility** (`6`, then `p`): on-demand, read-only list of profiles. Press `a` to list configured MCP servers across profiles.
- **MCP catalog-to-profile add** (`6`, select server, `Shift+A`): choose an existing profile, review the exact `catalog://mcp/docker-mcp-catalog/<server>` reference, and confirm with `y`. Docktop rechecks profile existence, current membership and catalog metadata immediately before the add. No automatic `--enable-all`, gateway launch, or tool execution; Docker MCP itself may expose assigned tools, so treat the change as security-relevant.
- **Full tool descriptions:** Bounded 4-KiB sanitized descriptions, dedicated right-panel focus (`Tab`) and line/page scrolling (`↑↓`, `PgUp/PgDn`), without fetching live MCP data while navigating.
- **Harder concurrent-change checks:** Before each staged permission write, Docktop rereads and compares the whole profile tool policy (not just the selected tool) and rechecks membership. It blocks stale plans or unexpected changes; Docker MCP has no compare-and-swap guarantee, so external races remain possible.
- **Explicit failure accounting:** Plan results distinguish blocked/no write attempted, previously verified partial execution, and an attempted write whose outcome cannot be verified. Never claim an atomic batch or automatic rollback.
- **Read-only MCP secret inventory** (`6`, then `s`): on-demand `docker mcp secret ls --json`, showing only validated secret identifiers and storage provider names, never values or raw CLI errors. A reachable store does not prove GitHub authentication, credential validity, or scope. Close with `Esc`.
- **Configured MCP clients** (`6`, then `i`): local client integrations from `docker mcp client ls --global --json`, with search (`/`), installation/OS-support status and configuration state, excluding raw client commands, credentials and config. **Not live connection telemetry**. For a confirmed installed and supported unconfigured client, press `c`, choose an existing MCP profile, review the exact `docker mcp client connect --global --profile <id> --quiet <client>` command, and press `y` to approve. The client is reread after connection; only the exact matching client/profile is reported verified. Errors after write dispatch are **unverified**, not assumed failures or successes. The workflow does not silently overwrite existing Docker MCP client connections and never runs a gateway or tools.
- **Staged MCP permission plans:** Use `Space` on a tool to stage its opposite permission, up to 12 named tools. `v` previews exact changes before `y` authorizes sequential Docker MCP operations. Each tool is rechecked and read back before the next write. There is **no transaction/rollback**; a partial failure reports verified earlier steps and warns about uncertain attempted steps. `c` clears the plan; read-only browsing never writes.
- **Searchable MCP Tool Explorer:** Search by tool name or description with `/`, filter by annotation group using `g`, and view enabled/disabled counts. A responsive two-panel layout separates permission state, capability hints, and tool descriptions. Classifications use provider-supplied annotations only; unannotated tools stay **unspecified**, never implicitly safe.
- **Post-change verification:** After approved individual-tool edits or server removal, Docktop reads Docker MCP state again. It reports **verified** only after confirming the target state; if Docker reports success but state cannot be confirmed, Docktop displays an **unverified** warning and requires a fresh inspection. Changes to unrelated tool states are flagged.
- **MCP per-tool inspection** (`6 → m → Enter profile → t`): reads `docker mcp profile show <profile> --format json` and exposes only validated tool names, descriptions and allowlist state (`all`, `none`, `allowlist`, `unknown`). Individual tool `e`/`d` actions require a separate confirmation and an immediate membership/status recheck; ambiguous snapshots fail closed.
- **Profile server removal** (`6 → m → Enter profile → x`): reviews exact server/profile names, requires `y`, and invokes `docker mcp profile server remove <profile> --name <server>`. It does not delete the profile or a Docker Engine container; Docker MCP may clean up orphaned OAuth registrations.
- **Empty-profile onboarding:** The profile manager explains when profiles may be disabled, shows the manual enable command, and supports `c` to create an empty profile after name review and explicit `y` confirmation. No servers or tools are added or enabled automatically.
- **Profile tool permission manager** (`6`, then `m`): navigate a structured profile list, select an assigned server, press `e` to request enabling **all** its tools or `d` to request disabling **all** its tools. An explicit `y` confirmation is required. Profile IDs and server membership are rechecked before modifying tool permissions. This neither adds/removes a server nor starts it. Changes may affect connected clients immediately.
- **Performance sampler**: `scripts/benchmark-process.sh PID [seconds] [interval]` reports interval CPU and resident memory min/mean/peak for Docktop without changing Docker state. The original `scripts/measure-process.sh` remains available for quick legacy snapshots.

The Docker I/O panel computes approximate **bytes per second** from successive `docker stats` cumulative counters, matched by container ID in the same Docker context. No rate appears until two valid samples are available. New containers and counter resets do not produce artificial spikes. The container CPU figure sums running-container usage (100% = one CPU core), not whole-host utilization. **The new local host CPU percentage** is calculated separately from `/proc/stat` as percent of total logical CPU time. A dashboard graph with a low but nonzero reading is autoscaled and labelled; its apparent height is not the percent of total host CPU.

Not yet implemented: Compose management operations, container creation, advanced image management beyond reviewed tag removal, live MCP session telemetry or automatic MCP client installation, full per-container metric plots, or controlled comparative benchmarks against other tools. The panel background inherits the terminal theme; only selected rows have a highlight background.

### Local host metrics (Linux only)

The upper-right card distinguishes **LOCAL LINUX HOST** (`/proc/stat` CPU, `/proc/meminfo` RAM using `MemAvailable`) from **DOCKER ENGINE** (active Docker context and container counts). GPU readings are optional: a bounded `nvidia-smi` query is attempted approximately every 15 seconds; AMD/compatible GPU drivers may use read-only `/sys/class/drm/.../gpu_busy_percent`. Unsupported drivers show **GPU not reported**, not a fabricated zero. Missing GPU tools do not prevent Docker monitoring. Host metrics describe the local machine running Docktop, even if the Docker context points to a remote daemon.

The graphs use a 60-sample rolling window at roughly five-second intervals. Network and disk rates are sampling estimates, not exact real-time telemetry, and they are **Docker container workloads**, not whole-host I/O.

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
| `a` | MCP tab: toggle configured servers across profiles (read-only); close an inspection with `Esc` first |
| `i` | MCP tab: show configured client integrations. Press `/` to search and `c` on an eligible client to choose a profile, review, then `y` to connect; `n/Esc` cancels without editing configuration. |
| `Shift+A` | MCP Catalog: request adding selected catalog server to an existing profile. Select profile, review exact reference, then `y` to approve or `n/Esc` to cancel. |
| `m` | MCP tab: open profile tool-permission manager. Inside a selected profile, use `t` for individual tool inspection/permission controls or `x` to request removing one server; both changes require confirmation. Use `c` to create a new empty profile (review + `y` confirmation), or select an existing profile with Enter, choose server, `e` enable all tools / `d` disable all tools, `y` confirm or `n` cancel. `Esc` backs out. |
| `Enter` on MCP server | Inspect catalog metadata and advertised tool names (no gateway execution); `Esc` returns to the catalog |
| `?` | Show help; `Esc` closes prompts or details |
| `q`, `Ctrl+C` | Exit |

On standalone Docker Engine, profiles may initially be disabled. First run `docker mcp feature enable profiles` yourself; Docktop never changes this experimental setting automatically. Profile creation is gated on a fresh, parseable profile list and explicit confirmation. MCP permission management uses `docker mcp profile tools --enable-all/--disable-all`, not server membership or gateway execution. Tool state is read only from structured profile-show JSON (never from the server list). The raw document may contain confidential configuration; Docktop discards those fields and never logs the raw JSON. Unknown/missing state blocks individual tool modifications. In the tool inspector use `Tab` to focus and scroll the description, `Space` to stage a tool permission change, `v` to review staged changes, `c` to clear them, `/` to search locally, `g` to cycle provider-annotation groups (all/read-only/destructive/other/unspecified), and `Esc` to leave; type-search never invokes Docker. Tool annotations are advisory metadata, not security enforcement. Existing bulk enable-all/disable-all operations remain available with confirmation. Unsupported JSON formats fail closed; if your Docker MCP plugin returns a different schema, share the error rather than attempting a blind write. Enabling all tools can expand an agent's capabilities. MCP inspection is a modal view: press `Esc` to return to the catalog before using `p` or `a`. The footer changes with the active view so unavailable actions aren't presented as active. Search on the MCP page is local and does not re-fetch the full catalog on every keystroke. To access MCP browsing, prepare the catalog separately:

```bash
docker mcp --help
docker mcp catalog pull mcp/docker-mcp-catalog:latest
```

## Design and security

- Docker CLI arguments are passed directly without a shell. The active Docker context and Docker permissions are honored.
- Read-only discovery is the default. Container state changes use validated IDs and ask for a `y` confirmation. Image pulls use validated Docker Hub image names, pin the Docker context captured during review and require the same `y` confirmation.
- No telemetry, credential storage, background Docktop daemon, socket forwarding, or MCP execution. MCP inspection whitelists metadata fields and never prints configuration or secret payloads.
- External text is sanitized; CLI commands are timeout-limited, logs are bounded, and metric history is capped to 60 samples.
- **Docker daemon access is highly privileged.** Do not run Docktop as root or mount the Docker socket into untrusted workloads. Confirmation prompts are guardrails, not an OS security boundary.
- Docker resource metrics are sourced from `docker stats` and do not represent whole-host CPU or RAM usage.

## Measure Docktop process overhead

Build a standalone binary first. Running through `go run` spawns a compiler process and complicates PID identification.

```bash
go build -o docktop ./cmd/docktop
./docktop
```

In a second terminal, identify the process and benchmark it while the dashboard is visible:

```bash
pgrep -a -x docktop
./scripts/benchmark-process.sh <PID> 60 2
```

This read-only sampler reports interval CPU usage (100% = one logical core) plus resident RAM min, average and peak. It rejects non-Docktop process names, notices PID reuse, and never invokes Docker commands. It measures the Docktop process **only**, excluding child processes, containers, and other Fedora applications. For comparison, run the sampler again while browsing MCP. Results are real process measurements, **not** evidence that Docktop outperforms Docker Desktop without a controlled test on the same hardware.

Legacy quick snapshot: `./scripts/measure-process.sh <PID>`.

## Development

```bash
go test ./...
go vet ./...
go build ./cmd/docktop
```

The unit tests use fake Docker responses and synthetic Linux host counters; they never interact with your local Docker daemon or running containers.
