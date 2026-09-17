# Running watch as a user service

`sandwarden watch` is the headless convergence loop. It watches sandboxd for
lifecycle events and re-applies profile rules, profile and direct mounts,
caches and skill mounts whenever a sandbox starts — including sandboxes
started with a raw `sbx start`. The desktop app runs the same loop internally,
so sandboxes started while the GUI is open are already converged.

`watch` is **opt-in**, and installing it is a trade-off:

| | Run `watch` by hand | Install the user service |
| --- | --- | --- |
| Convergence after a raw `sbx start` | only while your terminal runs | from login, without the GUI |
| Resource use | none when it is not running | one `sandwarden` process per login |
| Control | foreground, `Ctrl-C` to stop | `sandwarden setup status` / `uninstall` |

## Running watch without installing anything

Foreground:

```sh
sandwarden watch
```

Detached, in tmux or screen:

```sh
tmux new-session -d -s sandwarden 'sandwarden watch'
```

Or as a transient user unit, which survives until logout without writing any
file:

```sh
systemd-run --user --unit sandwarden-watch --collect sandwarden watch
```

Remember to pass `--config`, `--socket` or `--db` if you use non-default
paths; `watch` accepts the same persistent flags as every other command.

## Installing the service

```console
$ sandwarden setup
```

On Linux this writes a systemd --user unit and enables it:

- `~/.config/systemd/user/sandwarden-watch.service` (respecting
  `$XDG_CONFIG_HOME`),
- `systemctl --user daemon-reload`,
- `systemctl --user enable --now sandwarden-watch.service`.

On macOS it writes a launchd agent and loads it:

- `~/Library/LaunchAgents/com.sandwarden.watch.plist`,
- `launchctl bootstrap gui/$UID <plist>`,
- `launchctl kickstart -k gui/$UID/com.sandwarden.watch`.

The service runs the absolute path of the current `sandwarden` executable,
followed by `watch` and the `--config`, `--socket` and `--db` flags you passed
to `setup`. For example:

```sh
sandwarden --config ~/.config/sandwarden setup
```

bakes `ExecStart=/path/to/sandwarden watch --config /home/me/.config/sandwarden`
into the unit. Run `sandwarden setup` again to reinstall: it reports the
previous install, rewrites the definition file and restarts the service, so
the flags and executable path stay in sync.

`setup` never asks for sudo and never writes outside your user config or home
directory. It is the only command that installs anything; everything else,
including `watch`, keeps working exactly as before if you never run it.

### Generated systemd unit

```ini
[Unit]
Description=sandwarden convergence loop (sandwarden watch)
Documentation=https://github.com/JLugagne/sandwarden/blob/main/docs/service.md

[Service]
Type=simple
ExecStart=/path/to/sandwarden watch --config /home/me/.config/sandwarden
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

### Generated launchd agent

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.sandwarden.watch</string>
	<key>ProgramArguments</key>
	<array>
		<string>/path/to/sandwarden</string>
		<string>watch</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
```

## Checking the service

```console
$ sandwarden setup status
sandwarden watch service (systemd --user)
  unit:      /home/me/.config/systemd/user/sandwarden-watch.service
  installed: yes
  enabled:   yes
  running:   yes
  detail:    enabled and running
```

`installed` means the unit or plist file exists, `enabled` means it starts at
login (`systemctl --user is-enabled`, or loaded in launchd), and `running`
means the loop is active (`systemctl --user is-active`, or
`launchctl print` reporting `state = running`). When the unit file is missing
the command reports `installed: no` and exits 0.

## Logs

Linux:

```sh
journalctl --user -u sandwarden-watch -f
```

macOS:

```sh
log show --predicate 'process == "sandwarden"' --last 1h
```

## Disabling and removing

Stop and remove the service, including its file:

```console
$ sandwarden setup uninstall
Removed the sandwarden watch service (systemd --user)
  unit: /home/me/.config/systemd/user/sandwarden-watch.service
  ran systemctl --user disable --now sandwarden-watch.service
  ran systemctl --user daemon-reload
```

Uninstalling when nothing is installed is a no-op that exits 0. To keep the
file but stop autostarting at login:

```sh
systemctl --user disable --now sandwarden-watch   # Linux
launchctl bootout gui/$UID/com.sandwarden.watch   # macOS
```

## Failure modes

- **No systemd user session** (containers, some SSH sessions): `setup` exits
  with `the watch service needs a running systemd user session ...`. On a
  machine with logind, `loginctl enable-linger $USER` keeps a user manager
  available outside a login session without changing anything system-wide.
- **`systemctl` or `launchctl` not on `PATH`**: `setup` and `setup uninstall`
  report `not found in PATH`; `setup status` still reports whether the file is
  installed and explains that the state is unknown.
- **Unsupported platform**: only Linux (systemd --user) and macOS (launchd)
  are implemented; other platforms report an explicit error instead of
  writing something half-supported.
- **Uninstall without a running service manager**: the unit/plist file is
  still removed and each failed `systemctl`/`launchctl` call is reported as a
  warning.

## See also

- [Configuration files](configuration.md) for the paths `--config` points at.
- [sandwarden CLI](cli.md) for the `--json` contract of the other commands.
