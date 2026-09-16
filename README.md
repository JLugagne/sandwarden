# sandwarden

A desktop control panel for **Docker Sandboxes**.

sandwarden is a [Wails](https://v3.wails.io/) application (Go backend, React frontend) that drives the
`sandboxd` daemon and the `sbx` CLI: it lists and manages sandboxes, compiles reusable network policy
profiles, mounts shared caches and git-hosted agent skills, manages secrets, and surfaces the egress
proxy log in real time.

There is no HTTP or websocket server: the UI runs inside the Wails webview and talks to the Go
backend over in-process bindings and events. The app is never served to a browser.

## Features

- **Sandboxes** — create (agent, workspaces, CPUs/memory, published ports, environment, `--deny-network`,
  clone), list, inspect, start/stop, delete, run an interactive terminal, and manage bind mounts.
- **Profiles** — named allow/deny bundles compiled into sandbox-scoped `sandboxd` policy rules. A profile
  can be the *default* for new sandboxes and *global* to every sandbox. A profile is a whitelist **plus**
  a selection of skills and commands.
- **Skill stores** — register git repositories in the Anthropic plugin format (marketplace manifests, a
  single plugin, or plain `skills/` and `commands/` directories). sandwarden clones them locally,
  discovers every skill and command, and mounts the selected items read-only inside sandboxes at
  `/home/agent/.agents/skills/<name>` and `/home/agent/.agents/commands/<name>.md`. Public
  repositories clone anonymously, HTTPS URLs may embed credentials, and private marketplaces can
  authenticate with your `~/.ssh` keys or ssh-agent through a per-store option. Mounts are reconciled
  both ways (mounted and unmounted) on profile changes, sandbox start and periodic reconcile, with
  per-item conflict and missing-source detection.
- **Shared caches** — bind-mount host directories (Go module and build caches, npm, pnpm, yarn presets)
  into sandboxes, shareable across them, auto-attached on creation and re-applied after a restart.
- **Secrets** — manage service, registry and custom secrets through the `sbx` CLI, including import.
- **Traffic** — live egress proxy allow/deny log per sandbox, native desktop notifications for newly
  blocked hosts, and ad-hoc policy actions.
- **Settings** — connection health, shared caches and notification preferences.

## Architecture

```
main.go              Wails bootstrap: window, services, embedded assets
internal/app         domain services and background reconcile/event loops
internal/desktop     Wails binding surface (each exported method becomes a typed TS binding)
internal/sbx         sandboxd client (unix-socket JSON API + sbx CLI)
internal/store       SQLite store and migrations
internal/skills      git checkout and Anthropic plugin-format discovery
web/                 React 19 + Vite + TanStack Query UI (web/src/bindings is generated)
```

- Realtime updates flow through one Wails event channel (`hub:event`).
- Application state lives in SQLite, skill checkouts in a per-user data directory (see
  [Configuration](#configuration)).

## Requirements

**Runtime**

- A Linux desktop with GTK3 / WebKitGTK (Debian/Ubuntu: `libgtk-3-0 libwebkit2gtk-4.1-0`).
- Docker Sandboxes installed on the host: the `sandboxd` daemon (unix socket) and the `sbx` CLI on
  `PATH` (or `SBX_BINARY` pointing at it).

**Build**

- Go 1.26+
- Node.js 24+ and npm
- GTK3 / WebKitGTK development headers, `pkg-config` and a C toolchain:

  ```sh
  sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev
  ```

- The Wails v3 CLI, used to regenerate the TypeScript bindings:

  ```sh
  go install github.com/wailsapp/wails/v3/cmd/wails3@latest
  ```

- Optional: Docker with buildx for the container builds.

## Install

Prebuilt binaries are published for Linux (amd64) and macOS (Apple silicon). Two one-liners:

```sh
# latest stable release
curl -fsSL https://raw.githubusercontent.com/JLugagne/sandwarden/main/install.sh | bash

# rolling unstable pre-release (main branch)
curl -fsSL https://raw.githubusercontent.com/JLugagne/sandwarden/main/install-unstable.sh | bash
```

Before the first stable release exists, the stable command falls back to the unstable pre-release.
The scripts verify the release checksum and install to `~/.local/bin/sandwarden`
(`SANDWARDEN_INSTALL_DIR` overrides the location). Pin a version with `SANDWARDEN_VERSION=v0.1.0`,
or pass a channel directly: `... | bash -s unstable`.

Stable releases are cut from `v*` tags. The Settings page checks the latest stable release and reports
when an update is available, with the install command to run.

## Build and run with make

```sh
git clone https://github.com/JLugagne/sandwarden.git
cd sandwarden
make build     # builds the frontend, then the Go binary into bin/sandwarden
make run       # builds and launches the app
```

`make build` regenerates the Wails bindings, installs the frontend dependencies, builds the frontend
into `web/dist` and embeds it into the binary.

For development with Vite HMR inside the webview and Go hot-restart:

```sh
make dev       # wails3 dev
```

Other targets:

| Target | Purpose |
| --- | --- |
| `make check` | `fmt` + `vet` + Go tests + frontend tests |
| `make test` | Go and frontend tests |
| `make test-go` | Go tests only (uses the committed `web/dist` placeholder) |
| `make test-race` | Go tests with the race detector |
| `make typecheck` | TypeScript typecheck |
| `make bindings` | Regenerate `web/src/bindings` from the Go services |
| `make vet` / `make fmt` / `make tidy` | Go vet / formatting / `go mod tidy` |
| `make web` / `make web-test` | Build the frontend / run its unit tests |
| `make dev-web` | Vite dev server only (frontend-only; Wails bindings are unavailable in a browser) |
| `make install` | `go install` with the `gtk3,production` tags (run `make web` first so the real frontend is embedded) |
| `make clean` | Remove `bin/` and `web/dist` |

## Docker

Build the runtime image:

```sh
make image                  # docker build -t sandwarden:local .
```

sandwarden is a GUI: the container needs your display, the `sandboxd` socket and the `sbx` binary.
The following mounts the sandboxd socket, persists the app state and data, and reuses the host `sbx`:

```sh
docker run --rm \
  -e DISPLAY \
  -v /tmp/.X11-unix:/tmp/.X11-unix \
  -e DOCKER_SANDBOXES_API=/run/sandboxd.sock \
  -v "$HOME/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock:/run/sandboxd.sock" \
  -e XDG_STATE_HOME=/state -e XDG_DATA_HOME=/data \
  -v "$HOME/.local/state/sandwarden:/state/sandwarden" \
  -v "$HOME/.local/share/sandwarden:/data/sandwarden" \
  -v "$(command -v sbx):/usr/local/bin/sbx:ro" \
  sandwarden:local
```

If the window fails to open, share your X authority as well
(`-e XAUTHORITY=/root/.Xauthority -v "$XAUTHORITY:/root/.Xauthority:ro"`) or allow local clients with
`xhost +local:`.

To extract only the binary instead of running the container:

```sh
make image-binary           # docker buildx build --target binary --output type=local,dest=bin .
```

The Docker build does not need the Wails CLI: the TypeScript bindings are committed and only
regenerated with `make bindings`.

## Configuration

| Flag | Environment variable | Default |
| --- | --- | --- |
| `-socket` | `DOCKER_SANDBOXES_API` | `~/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock` |
| `-db` | `XDG_STATE_HOME` | `~/.local/state/sandwarden/sandwarden.db` |
| | `SBX_BINARY` | `sbx` found on `PATH` |
| | `XDG_DATA_HOME` | skill store checkouts under `~/.local/share/sandwarden/skill-stores/` |

## Data

- The SQLite database holds profiles, rules, cache definitions, skill stores and their catalog, and
  the profile/sandbox assignments.
- Each registered skill store is cloned into its own directory and refreshed from the Skills page.
- Deleting the database resets the app to a clean state; nothing else on the host is modified.

## License

No license has been chosen yet.
