# sandwarden CLI

`ls`, `status`, `apply`, `start` and `import` accept `--json`. With it, stdout carries exactly one
compact JSON document followed by a newline, and every diagnostic goes to stderr, so
`sandwarden ls --json | jq` is always safe to pipe. Without `--json`, the human output is
unchanged.

The contract below is stable. Any incompatible change to a documented key bumps
`schemaVersion`.

## Shared shapes

### Config errors

Per-file load errors, from any entity under the configuration directory. A broken file never
fails the command; it is reported here instead.

| Field | Type | Description |
| --- | --- | --- |
| `path` | string | Absolute path of the file that failed to load |
| `error` | string | Parse or read error |

### Sandbox

| Field | Type | Description |
| --- | --- | --- |
| `name` | string | Canonical sandboxd name |
| `id` | string | sandboxd id, empty when the daemon is unreachable or the sandbox is unknown |
| `status` | string | sandboxd status (`running`, `stopped`, ...), empty when not in the daemon |
| `running` | bool | Whether the daemon reports the sandbox as running |
| `configSlug` | string | Directory slug under `<config>/sandboxes/`, empty when there is no config directory |
| `configPath` | string | Absolute path of that config directory, empty when there is none |
| `profiles` | array of strings | Profile slugs recorded in `sandwarden.yaml` (file side, not daemon-resolved names) |
| `caches` | number | Number of cache slugs recorded in `sandwarden.yaml` |
| `skills` | number | Number of skill references recorded in `sandwarden.yaml` |
| `mounts` | number | Number of direct mount references recorded in `sandwarden.yaml` |
| `incomplete` | bool | `create.incomplete: true` marker from `sandwarden.yaml`: the config was adopted from the daemon without its original CPU, memory and environment |

The counts and `profiles` describe the files; they do not claim the daemon has them attached.

## ls --json

```json
{
  "schemaVersion": 1,
  "sandboxes": [
    {
      "name": "web",
      "id": "web",
      "status": "running",
      "running": true,
      "configSlug": "web",
      "configPath": "/home/me/.config/sandwarden/sandboxes/web",
      "profiles": ["dev"],
      "caches": 1,
      "skills": 2,
      "mounts": 0,
      "incomplete": false
    }
  ],
  "configErrors": []
}
```

`sandboxes` is the daemon's sandbox list, each entry enriched with its file-side state.
`configErrors` is always present, `[]` when clean.

When sandboxd is unreachable, `sandboxes` is `[]`, a `warning:` line is written to stderr and
the exit code is non-zero.

## status --json

```json
{
  "schemaVersion": 1,
  "configDir": "/home/me/.config/sandwarden",
  "configErrors": [],
  "sandboxes": [
    {
      "name": "web",
      "id": "web",
      "status": "running",
      "running": true,
      "configSlug": "web",
      "configPath": "/home/me/.config/sandwarden/sandboxes/web",
      "profiles": ["dev"],
      "caches": 1,
      "skills": 2,
      "mounts": 0,
      "incomplete": false,
      "errors": [],
      "detail": { "...": "full sandbox detail projection" }
    }
  ]
}
```

Each entry adds two keys to the shared sandbox shape:

| Field | Type | Description |
| --- | --- | --- |
| `errors` | array of strings | Per-name problems; `["not in daemon: ..."]` when sandboxd does not know the sandbox |
| `detail` | object, omitted when absent | The full daemon-side projection (same keys as the desktop event payload: `sandbox`, `profiles`, `mounts`, `mounts_error`, `incomplete`, `image`, `image_digest`, `kits`, `secrets`, `custom_secrets`, `policy_rules`, `caches`, `profile_mounts`, `direct_mounts`, `additional_workspaces`, `skills`). `detail.incomplete` mirrors the entry's top-level `incomplete`. |

With no arguments, every configured sandbox is listed. When any name fails, the document is
still printed and the exit code is non-zero.

## import --json

```json
{
  "schemaVersion": 1,
  "created": 1,
  "alreadyConfigured": 1,
  "failed": 1,
  "results": [
    { "name": "web", "status": "created", "incomplete": true },
    { "name": "api", "status": "already configured", "incomplete": false },
    { "name": "ghost", "status": "failed", "incomplete": false, "error": "not in daemon" }
  ]
}
```

`sandwarden import [SANDBOX...]` writes a config directory for every daemon sandbox that has none,
or only for the named ones. `status` is one of `created`, `already configured` or `failed`; a
failed sandbox is reported and the batch continues. `incomplete` is `true` when the adopted config
is missing create parameters the daemon cannot report back (CPU, memory, environment). Any
`failed` entry makes the exit code non-zero; the document is still printed.

## apply --json

```json
{
  "schemaVersion": 1,
  "reports": [
    {
      "name": "web",
      "report": {
        "rules_applied": 1,
        "mounts_applied": 0,
        "caches_applied": 1,
        "skills_applied": 0,
        "skills_removed": 0,
        "warnings": [],
        "errors": []
      }
    },
    { "name": "ghost", "error": "..." }
  ]
}
```

Exactly one of `report` / `error` is present per entry. Any entry with an `error`, or with a
non-empty `report.errors`, makes the exit code non-zero.

## start --json

```json
{
  "schemaVersion": 1,
  "sandboxes": [
    {
      "name": "web",
      "started": true,
      "report": { "...": "same shape as apply's report" }
    },
    { "name": "ghost", "started": false, "error": "..." }
  ]
}
```

## doctor

`sandwarden doctor` scans every sandbox `spec.yaml` for environment values that look like
credentials written in clear (the same heuristic as the create guard: known prefixes such as
`sk-`, `ghp_`, `xoxb-`, AWS keys, PEM blocks, and long high-entropy tokens). It prints one line
per finding with the file and line, never the value, and exits non-zero when anything is found:

```sh
$ sandwarden doctor
sandbox leaky: spec.yaml:6: ANTHROPIC_API_KEY looks like a secret (API key prefix "sk-")
doctor: 1 environment value(s) look like secrets; rotate them, store them with `sbx secret set` (or from the Secrets page), then keep only the bare KEY form in spec.yaml
```

A clean fleet exits 0. See [secrets.md](secrets.md) for the rules and the bare `KEY` form.

## Exit codes and stderr

| Situation | stdout | stderr | exit |
| --- | --- | --- | --- |
| Success | one JSON document | empty | 0 |
| Daemon unreachable (`ls`) | envelope with `"sandboxes":[]` | `warning: ...` | 1 |
| Sandbox not in daemon (`status`) | envelope with per-name `errors` | error summary | 1 |
| Sandbox apply/start failed | envelope with per-entry `error` or `report.errors` | error summary | 1 |
| Sandbox import failed | envelope with per-entry `error` | error summary | 1 |
| Broken config file | envelope with `configErrors` | empty | 0 when nothing else failed |
| Index or config directory unreadable | empty envelope | error | 1 |

## Examples

```sh
sandwarden ls --json | jq -r '.sandboxes[] | select(.incomplete) | .name'
sandwarden status --json | jq -r '.sandboxes[] | select(.errors | length > 0) | .name'
sandwarden ls --json | jq '.configErrors'
sandwarden import --json | jq -r '.results[] | select(.incomplete) | .name'
```
