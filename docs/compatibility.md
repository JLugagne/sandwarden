# sbx compatibility

sandwarden drives the `sbx` CLI. A few create options and flags were added over several
sandbox releases, so a config that works on a recent sbx can fail on an older one with an
obscure "unknown flag" error. sandwarden reads the installed version once (`sbx version`)
and reports which features that version cannot run, instead of letting `sbx` reject the
command.

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
- **Settings.** The desktop app shows the installed sbx version and the feature table in
  **Settings → General → sbx compatibility**. The create form disables the options the
  installed sbx cannot run and shows the required minimum version.

## Upgrading

Upgrade Docker Sandboxes to get the newer `sbx`. After the upgrade, restart sandwarden
(the desktop process caches the version for its lifetime) or run any CLI command again;
the next check reads the new version.
