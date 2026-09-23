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
- **Secrets** — manage service, registry and custom secrets through the `sbx` CLI, including
  import. sandwarden never writes secret values into a spec: the create form and
  `sandwarden create --env` reject values that look like credentials (known prefixes such as
  `sk-`, `ghp_`, `xoxb-`, AWS keys, PEM blocks, long high-entropy tokens); a bare `KEY` reads
  the value from the host environment, and `sandwarden doctor` scans existing specs.
- **Traffic** — live egress proxy allow/deny log per sandbox, native desktop notifications for newly
  blocked hosts, and ad-hoc policy actions.
- **Search** — Ctrl/Cmd+K opens a BM25 search over fleet configuration and catalogs: sandboxes
  (including stopped ones), profiles, shared caches, and the discovered skill, command and kit
  items, with deep links that open or highlight the hit.
- **Raw config viewer** — inspect a sandbox or profile's `spec.yaml` and `sandwarden.yaml`
  read-only (syntax coloring, line numbers, copy), with per-file parse errors shown.
- **Settings** — connection health, shared caches and notification preferences.

## Architecture

```
main.go              Wails bootstrap: window, services, embedded assets
internal/app         domain services and background reconcile/event loops
internal/cli         headless sandwarden commands (cobra) wrapping sbx
internal/desktop     Wails binding surface (each exported method becomes a typed TS binding)
internal/fleet       filesystem-backed configuration (the source of truth)
internal/sbx         sandboxd client (unix-socket JSON API + sbx CLI)
internal/store       SQLite index: discovered catalogs and the applied-rules ledger
internal/skills      git checkout and Anthropic plugin-format discovery
web/                 React 19 + Vite + TanStack Query UI (web/src/bindings is generated)
```

- Realtime updates flow through one Wails event channel (`hub:event`).
- Every sandbox, profile and cache is a **file**: a valid sbx kit `spec.yaml` plus a
  `sandwarden.yaml` sidecar. Files are the source of truth; SQLite only caches discovered
  catalogs and tracks the policy rules sandwarden applied. Skill checkouts live in a
  per-user data directory (see [Configuration](#configuration)).
- The same binary is a CLI: `sandwarden run`, `start`, `stop`, `restart`, `rm`, `apply`,
  `create`, `recreate`, `status`, `validate`, `import`, `export`, `kit add`, `doctor`,
  `setup`, `watch` and a raw `sandwarden sbx …` passthrough. Every mutating command applies
  the files, so mounts and rules survive restarts even without the GUI.

## Requirements

**Runtime**

- A Linux desktop with GTK3 / WebKitGTK (Debian/Ubuntu: `libgtk-3-0 libwebkit2gtk-4.1-0`), or macOS.
  Native notifications need an `.app` bundle, so the standalone macOS binary logs that they are
  disabled and runs without them.
- Docker Sandboxes installed on the host: the `sandboxd` daemon (unix socket) and the `sbx` CLI on
  `PATH` (or `SBX_BINARY` pointing at it).

**Build**

- Go 1.26+
- Node.js 24+ and npm
- On Linux, GTK3 / WebKitGTK development headers, `pkg-config` and a C toolchain:

  ```sh
  sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev
  ```

- On macOS, the Xcode command line tools (`xcode-select --install`).
- Optional: the Wails v3 CLI, needed only to regenerate the committed TypeScript bindings:

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
| `make test-go` | Go tests only (no frontend build needed) |
| `make test-race` | Go tests with the race detector |
| `make typecheck` | TypeScript typecheck |
| `make bindings` | Regenerate `web/src/bindings` from the Go services |
| `make vet` / `make fmt` / `make tidy` | Go vet / formatting / `go mod tidy` |
| `make web` / `make web-test` | Build the frontend / run its unit tests |
| `make dev-web` | Vite dev server only (frontend-only; Wails bindings are unavailable in a browser) |
| `make install` | `go install` with the `gtk3,production` tags (run `make web` first so the real frontend is embedded) |
| `make clean` | Remove `bin/` and `web/dist` |

The Go suite needs no daemon and no network: the `internal/app` and `internal/cli` tests run
against a fake sandboxd unix socket and fake `sbx` binaries.

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
  -e XDG_STATE_HOME=/state -e XDG_DATA_HOME=/data -e XDG_CONFIG_HOME=/config \
  -v "$HOME/.local/state/sandwarden:/state/sandwarden" \
  -v "$HOME/.local/share/sandwarden:/data/sandwarden" \
  -v "$HOME/.config/sandwarden:/config/sandwarden" \
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
| `-socket` | `DOCKER_SANDBOXES_API` | Linux: `~/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock`<br>macOS: `~/Library/Application Support/com.docker.sandboxes/sandboxes/sandboxd/sandboxd.sock` |
| `-config` | `SANDWARDEN_CONFIG_DIR` | `~/.config/sandwarden` (or `$XDG_CONFIG_HOME/sandwarden`) |
| `-db` | `XDG_STATE_HOME` | `~/.local/state/sandwarden/sandwarden.db` |
| | `SBX_BINARY` | `sbx` found on `PATH` |
| | `XDG_DATA_HOME` | store checkouts under `~/.local/share/sandwarden/{skill,kit}-stores/<slug>/` |

## Declarative files

The configuration directory is the source of truth. Every entity is a directory with a
kit-format `spec.yaml` (validated by `sbx kit validate`, passable to `sbx create --kit`)
and a `sandwarden.yaml` sidecar for what kits cannot express:

```
config.yaml                        # global preferences (terminal launcher, notifications)
sandboxes/<slug>/spec.yaml         # requires.agent, environment, ports, permissions
sandboxes/<slug>/sandwarden.yaml   # canonical name, create params, profiles, caches, skills, mounts, run args
profiles/<slug>/spec.yaml          # permissions.network allow/deny = the profile rules
profiles/<slug>/sandwarden.yaml    # default/global flags, mounts, caches, skills
caches/<slug>/sandwarden.yaml      # host path, target, read-only, auto-attach, enabled
stores/skills/<slug>.yaml          # git store registrations (url, ref, auth)
stores/kits/<slug>.yaml
```

The complete field-by-field reference for the `spec.yaml`/`sandwarden.yaml` schema, slugs and
canonical names, skill/cache references, opt-outs, `create:` parameters and the `incomplete`
marker is in [docs/configuration.md](docs/configuration.md).

Starting a sandbox — from the GUI or with `sandwarden start` — re-applies its profile
rules, profile and direct mounts, caches and skill mounts, because bind mounts do not
survive a restart. `sandwarden run <sandbox>` goes further: it creates the sandbox from
its files if it no longer exists, starts it, applies everything and attaches.
`sandwarden watch` keeps converging in the background (including sandboxes started with
a raw `sbx start`). It stays opt-in: run it in a terminal, or install it as a per-user
autostart service with `sandwarden setup` (systemd --user on Linux, launchd on macOS),
inspect it with `sandwarden setup status`, remove it with `sandwarden setup uninstall`.
See [docs/service.md](docs/service.md) for the trade-off, unit paths and logs.

Deleting a sandbox keeps its config directory by default (`sandwarden rm` or the GUI
offer a purge option), so `sandwarden run` can recreate it identically. Recreating a
sandbox needs the create parameters recorded in the sidecar; sandboxes imported from the
daemon are marked `incomplete: true` when some of them are not recoverable.

`sandwarden import [SANDBOX...]` (or **Import existing** on the Sandboxes page) adopts
daemon sandboxes that have no config directory and reports which adoptions are incomplete.
Incomplete configs are badged in the GUI and shown as `config=incomplete` by
`sandwarden status`; a `recreate` warns first, because CPU, memory and environment were not
recoverable. The sandbox page's **Complete this config** action records those values and
clears the marker.

`sandwarden kit add SANDBOX REF` (GUI: **Attach kit**) adds a mixin to an existing sandbox
without deleting it: sbx swaps the container for one created with the kit appended, so
kit-owned volumes (agent session state) and a `--clone` sandbox's working tree are
preserved. The reference is recorded in `create.kits` and survives a later recreate.

Files are also the interface for other tools. `sandwarden export --sbxenv SANDBOX` renders a
sandbox as an `sbxenv.yaml` for sbx's experimental `sbx env` command, reporting every field
sbxenv cannot express on stderr ([docs/sbxenv.md](docs/sbxenv.md)). `sandwarden ls`, `status`,
`apply`, `start` and `import` accept `--json` with a stable schema
([docs/cli.md](docs/cli.md)).

sandwarden does not watch the configuration directory — reloading stays an explicit action,
so the app never races your editor or the CLI. It fingerprints every file it loads and stats
them on window focus and every few seconds: a file changed on disk gets a "changed on disk"
badge and a one-click **Reload from disk** that only re-reads the fleet, never mutates the
daemon.

## Data

- SQLite is an index: discovered skill and kit catalogs, per-store sync state, and the
  ledger of policy rules sandwarden installed (so it only ever removes its own rules).
- On startup sandwarden checks every registered store: when its catalog is empty or its
  checkout directory is missing it re-clones and re-discovers it automatically (a few at a
  time). A failing remote is reported on the store's sync state and never blocks startup;
  a manual Refresh stays available per store.
- Git stores are cloned into deterministic directories derived from their slug.
- Deleting the index only forces a re-discovery; nothing else on the host is modified.

## License

No license has been chosen yet.
