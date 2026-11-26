# sbx compatibility

sandwarden drives the `sbx` CLI. A few create options and flags were added over several
sandbox releases, so a config that works on a recent sbx can fail on an older one with an
obscure "unknown flag" error. sandwarden reads the installed versions (`sbx version --json`)
and reports which features that version cannot run, instead of letting `sbx` reject the
command.

## Client and daemon versions

`sandboxd` is the daemon behind `sbx`, and it is upgraded separately. sandwarden reads
both versions:

- the **client** version is the `sbx` binary on `PATH`;
- the **daemon** version is reported by `sbx version --json` in its `server` block.

Feature checks use the **effective version**, which is the *lower* of the client and the
daemon when both are known. An unknown part (a dev build or a stopped daemon without a
version) never blocks a feature. When `sbx` predates `version --json`, sandwarden falls
back to the plain `sbx version` text and treats it as the client version alone.

A running daemon whose version differs from the CLI requires a restart. sandwarden reports
`daemon_restart_required` in the desktop compatibility payload, prints both versions in
`sandwarden doctor`, and exits non-zero from `doctor` when a restart is pending. The
desktop app shows a global banner with a confirmation dialog listing the running sandboxes.

```
sbx was updated (CLI v0.54.1, daemon v0.43.0) and sandboxd must restart; run `sbx daemon restart` (this stops running sandboxes)
```

`sbx` refuses daemon-touching commands non-interactively when an update is pending
(`ensure daemon: cannot prompt for restart: stdin is not a terminal`); sandwarden turns
that stderr into the typed, actionable error above instead of surfacing it raw. The daemon
restart is performed with `sbx daemon restart --no-input`: `sbx` exposes no
`--yes`/`--force` flag, and `--no-input` is its documented non-interactive switch (it fails
with the command to run in a terminal if an answer is still required).

## Feature minimum versions

| Feature | Trigger | Minimum sbx |
| --- | --- | --- |
| Skills mode (`create.skills`) | `create.skills` set, or `create --skills off\|readonly\|readwrite` | v0.43.0 |
| Kit arguments (`create.kitArgs`) | `create.kitArgs` set, or `create --kit-arg` | v0.43.0 |
| Kit argument files (`create.kitArgsFiles`) | `create.kitArgsFiles` set, or `create --kit-args-file` | v0.43.0 |
| Environment files (`create.envFiles`) | `create.envFiles` set, or `create --env-file` | v0.43.0 |
| Static MCP servers (`create.staticMcp`) | `create.staticMcp` set, or `create --static-mcp` | v0.43.0 |
| `kit add --kit-arg` (`kit.add.kitArg`) | `sandwarden kit add --kit-arg` | v0.43.0 |
| Static MCP `none` (`create.staticMcp.none`) | the literal value `none` in `create.staticMcp` | v0.51.0 |
| Workspace mount target (`create.workspaces`) | `PATH:TARGET[:ro]` in `create.workspaces` (a two-part `PATH:ro` has no target) | v0.51.0 |
| sbxenv additional workspace target (`sbxenv.additionalWorkspaces.target`) | `additionalWorkspaces[].target` in an sbxenv export | v0.51.0 |
| `sbx port ls/publish/unpublish` (`sbx.port`) | informational only; sandwarden does not call it | v0.54.0 |

## How the checks behave

- **Unknown version means allowed.** Dev builds may print `dev-<sha>` or something
  unparseable. sandwarden then treats every feature as supported, so a dev build is never
  blocked. A pre-release such as `v0.43.0-rc1` counts as `v0.43.0`.
- **Fail fast.** `sandwarden create`, `sandwarden recreate` and the GUI's create/recreate
  check the required features before any `sbx` call, and before `recreate` deletes
  anything. `sandwarden kit add --kit-arg` checks before invoking `sbx kit add`. The error
  names the feature and both versions, for example:

  ```
  workspace mount target (create.workspaces) requires sbx v0.51.0 or later; installed sbx is v0.43.0
  ```

- **Reporting.** The sandbox detail payload carries an `incompatible` list; the sandbox
  page shows it as a warning callout. `sandwarden validate <sandbox>` prints each message
  and exits non-zero. `sandwarden doctor` prints the installed version and every affected
  sandbox. `sandwarden status --json` exposes the list per sandbox.
- **Export.** When the installed sbx predates v0.51.0, `sandwarden export --sbxenv` reports
  each `additionalWorkspaces[].target` as incompatible. The target stays in the document:
  the export is informational and is not silently downgraded.
- **Settings.** The desktop app shows the effective, client and daemon versions plus the
  feature table in **Settings → General → sbx compatibility**. The create form disables
  the options the installed sbx cannot run and shows the required minimum version.

## Upgrading

Upgrade Docker Sandboxes to get the newer `sbx`, then restart the daemon so `sandboxd`
matches the CLI: `sbx daemon restart` (this stops running sandboxes). Until it is
restarted, the effective version is the older daemon version and feature checks use it.
sandwarden re-reads the versions on every compatibility check (`doctor` and the Settings
panel) and after a restart, so an upgrade picked up by a long-running process is not
ignored until a cache TTL expires.
