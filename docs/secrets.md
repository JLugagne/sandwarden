# Secrets and environment values

Sandbox environment variables live in `sandboxes/<slug>/spec.yaml` as
`environment.variables`. That file is plain YAML in the configuration
directory, and a configuration directory is typically kept in git. Anything
written there is written in clear, forever.

Secrets have their own feature: `sbx secret set` (or the **Secrets** page of
the desktop app) stores them with the sandbox runtime, outside the config
directory. The environment block should only ever carry variable *names* for
those, never values.

## The bare `KEY` form

An environment entry without a value tells the runtime to read the value from
the host environment of the sandbox process:

```yaml
environment:
  variables:
    ANTHROPIC_API_KEY: ""   # value comes from the host, not from this file
    NODE_ENV: development   # plain values stay as they are
```

Both forms are valid in the create dialog (one entry per line: `KEY=VALUE` or
bare `KEY`) and with `sandwarden create --env KEY` / `--env KEY=VALUE`.

## Create-time guard

`app.CreateSandbox` — the write path shared by the create dialog, `sandwarden
create` and `sandwarden run` when it has to create a missing sandbox — refuses
environment values that look like secret material. The check is conservative:
false negatives are accepted, false positives are not. It runs before `sbx
create`, so nothing is created when it rejects.

A rejected value produces:

```
Error: environment variable ANTHROPIC_API_KEY looks like a secret value (API key prefix "sk-"): secrets must never be written in clear into spec.yaml: store it with `sbx secret set` (or from the Secrets page) and enter just ANTHROPIC_API_KEY= so the value is read from the host environment
```

### Detection rules

| Shape | Reason reported |
| --- | --- |
| Value starting with `sk-` | `API key prefix "sk-"` |
| Value starting with `github_pat_`, `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_` | `GitHub token prefix "<prefix>"` |
| Value starting with `xoxa-`, `xoxb-`, `xoxp-`, `xoxr-`, `xoxs-`, `xapp-` | `Slack token prefix "<prefix>"` |
| Value starting with `AKIA` or `ASIA` | `AWS access key id prefix "<prefix>"` |
| Value starting with `AIza` | `Google API key prefix "AIza"` |
| Value starting with `glpat-`, `gldt-`, `glrt-` | `GitLab token prefix "<prefix>"` |
| Value starting with `npm_` | `npm token prefix "npm_"` |
| Value starting with `pypi-` | `PyPI token prefix "pypi-"` |
| Value starting with `dckr_pat_` | `Docker token prefix "dckr_pat_"` |
| PEM private key block | `PEM private key block` |
| Single token of 32 characters or more, ASCII letters and digits plus `-` and `_` only, containing both a letter and a digit, Shannon entropy >= 3.5 bits per character | `long high-entropy token` |

Notes:

- An empty value (bare `KEY`) is always accepted.
- Prefix matching happens after trimming surrounding whitespace and a pair of
  matching quotes.
- The generic token rule ignores anything containing a space, a path separator
  or URL punctuation (`/`, `\`, `:`, `@`, `.` and friends). Values such as
  `true`, `debug`, `8g`, `/usr/local/go`, `https://proxy:8080`,
  `some.long.domain.example.com` or a UUID are never flagged.
- A credential already present verbatim in the sandbox's own `spec.yaml` is
  tolerated, so recreating an existing hand-written config keeps working. The
  doctor reports those instead.

## `sandwarden doctor`

`sandwarden doctor` scans every sandbox `spec.yaml` with the same heuristic and
reports what it finds. It never prints a value — only the variable name, file
and line — and exits non-zero when there is at least one finding.

```
$ sandwarden doctor
sandbox leaky: spec.yaml:6: ANTHROPIC_API_KEY looks like a secret (API key prefix "sk-")
doctor: 1 environment value(s) look like secrets; rotate them, store them with `sbx secret set` (or from the Secrets page), then keep only the bare KEY form in spec.yaml
```

```
$ sandwarden doctor
doctor: 4 sandbox spec(s) scanned, no likely secrets found
```

Remediation: rotate the exposed value, store the new one with `sbx secret set`
(or from the Secrets page), then replace the value in `spec.yaml` with the bare
`KEY` form. Rotating matters — the old value is already in the file, its
backups and possibly git history.

## What is deliberately not guarded

`sbx secret` reports a masked value and sandwarden never writes secret values
of its own. The guard only covers values entered through a create request; it
does not rewrite existing files. `apply`, `recreate` and a `run` that starts an
already-configured sandbox read `spec.yaml` as-is: hand-written configurations
keep working so the doctor can report them instead of blocking the sandbox.

The check runs at write time only; a value that is not secret today can become
one tomorrow (a short token that later looks like a password). `doctor` is the
backstop for everything already on disk.

## See also

- [Configuration files](configuration.md) — `spec.yaml` layout and the
  `environment.variables` field.
- [CLI reference](cli.md) — `create`, `run` and the other verbs.
