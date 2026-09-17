# Exporting a sandbox as sbxenv.yaml

`sbx env` is sbx's EXPERIMENTAL declarative environment command: it reads an `sbxenv.yaml`
document describing an agent, mixin kits, a workspace, environment variables, secrets,
MCP servers, published ports and host lifecycle commands, then plans, creates or removes
that environment. sandwarden keeps its own files as the source of truth, so exporting is a
read-only translation of a sandbox's `spec.yaml` plus `sandwarden.yaml` into an
`sbxenv.yaml`. The exported document is built from the files alone: exporting never calls
the daemon and never runs `sbx env`, so it works offline and is not a runtime dependency.

## Usage

```
sandwarden export --sbxenv SANDBOX              # YAML on stdout, report on stderr
sandwarden export --sbxenv -o FILE SANDBOX      # write the document to FILE
sandwarden export --sbxenv -o DIR               # one <slug>.sbxenv.yaml per sandbox in DIR
```

- `--sbxenv` selects the format and is required; it is the only format today.
- `SANDBOX` is a canonical sandboxd name or a directory slug.
- Stdout carries only the YAML document. Everything sbxenv cannot express is printed to
  stderr as `export: <slug>: <field> cannot be expressed in sbxenv.yaml (…)`.
- Without a sandbox name, every config sandbox is written to `-o DIR` as
  `<slug>.sbxenv.yaml`; a sandbox that cannot be exported (for example one without
  `requires.agent`) is reported and skipped, the others are still written, and the command
  exits non-zero.
- Output is deterministic: the document keys are always emitted in the order below, `env`
  keys are sorted, and `kits`, workspaces and ports keep their file order, so the output can
  be pinned by a test.

## What is exported

| `sbxenv.yaml` | sandwarden source | Notes |
| --- | --- | --- |
| `schemaVersion: 1` | — | `sbx env` accepts no other version. |
| `name` | sidecar `sandbox:`, falling back to `spec.displayName` | Emitted only when the name matches `^[a-zA-Z0-9][a-zA-Z0-9.-]+$`, is at least two characters and is not `default`. Otherwise the key is omitted (sbx derives `<agent>-<workspace basename>`) and the name is reported. |
| `agent` | `spec.requires.agent` | Required: an export of a sandbox without an agent fails. |
| `kits[]` | `create.kits` | Bare references, verbatim (the same values sandwarden passes to `sbx create --kit`). A relative source (`./…`, `../…`, `.`, `..`, `….zip`) is resolved by sbxenv against the directory of the `sbxenv.yaml`, which is reported. |
| `workspace.path` | the first writable entry of `create.workspaces` | sbxenv mounts a single read-write workspace. Read-only (`:ro`) entries and additional workspaces are reported and dropped. A relative path is reported because sbxenv resolves it against the `sbxenv.yaml` file. |
| `workspace.clone` | `create.clone` | Mapped when a workspace path exists, since `sbx env` exposes `workspace.clone` (the equivalent of `sbx create --clone`). Without a workspace it is reported. |
| `env` | `spec.environment.variables` | A mapping with sorted keys. A bare key (no value) takes its value from the host environment at create time, which sbxenv cannot express, so it is reported and omitted. |
| `ports[].sandbox` | `create.publish` entries | Parsed as `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`, the `sbx create -p` syntax. |
| `ports[].host` | same | Set for `HOST:SANDBOX` entries; omitted for `SANDBOX` entries so sbxenv picks an ephemeral host port, exactly as `sbx create -p SANDBOX` does. |
| `ports[].protocol` | same | Set from `PROTOCOL` (`tcp`, `tcp4`, `tcp6`, `udp`, `udp4`, `udp6`); omitted when the entry names none. |

Everything else in the sandbox's files is left out and reported per field.

## What cannot be expressed

`sbx env` has no field for these, so exporting reports them on stderr instead of dropping
them silently:

| sandwarden | Reported field |
| --- | --- |
| `profiles` | `profiles` — networks, mounts, caches and skills a profile inherits are not merged into the document |
| `caches` | `caches` |
| `skills` | `skills` (as `store:kind:name`) |
| `mounts` | `mounts` (as `hostPath:targetPath`) |
| `optOuts.mounts`, `optOuts.caches` | `optOuts.mounts`, `optOuts.caches` |
| `runArgs` | `runArgs` |
| `create.cpus`, `create.memory` | `create.cpus`, `create.memory` |
| `create.template`, `create.daemonProfile` | `create.template`, `create.daemonProfile` |
| `create.incomplete: true` | a warning that the export may be missing create parameters |
| additional / read-only `create.workspaces` entries | one line per entry |
| host-IP publishes (`127.0.0.1:8080:3000`) | one line per entry; the whole binding is dropped, because sbxenv has no bind-address field and publishing it anyway would widen the exposure to every interface |
| `spec.permissions.network.allow`, `.deny` | `permissions.network.allow`, `permissions.network.deny`; sbxenv rejects a `network` key, so there is no equivalent to map onto |
| `spec.ports` | `spec.ports` — kit-declared container ports are informational; publishing comes from `create.publish` |
| `spec.sandbox.image`, `.entrypoint`, `.command`, `.resources.cpu`, `.resources.memory` | one line per non-empty field |
| `spec.setup.install`, `.startup`, `.files` | one line per non-empty list |
| `spec.credentials`, `spec.arguments` | one line each |
| `spec.description`, `spec.sourceURL` | one line each |
| `displayName` when it differs from the exported `name` | `displayName` |
| an invalid canonical `name` | `name` |

Secrets are not exported either: sandwarden has no mapping onto the sbxenv `secrets:` block,
and the sandbox's own secrets live in sbx's secret store, not in the config directory.

## Example

`spec.yaml`:

```yaml
schemaVersion: "2"
kind: mixin
name: web-dev
displayName: Web dev
description: Frontend work
requires:
  agent: claude
environment:
  variables:
    NODE_ENV: development
permissions:
  network:
    deny:
      - telemetry.example.com
```

`sandwarden.yaml`:

```yaml
sandbox: web-dev
create:
  workspaces:
    - /home/you/src/web
  publish:
    - "3000:3000"
  kits:
    - ./mixins/base
caches:
  - go-mod-cache
```

```console
$ sandwarden export --sbxenv web-dev > sbxenv.yaml
export: web-dev: create.kits[0] (./mixins/base): sbxenv resolves a relative kit source against the sbxenv.yaml file
export: web-dev: displayName cannot be expressed in sbxenv.yaml (Web dev)
export: web-dev: description cannot be expressed in sbxenv.yaml (Frontend work)
export: web-dev: permissions.network.deny cannot be expressed in sbxenv.yaml (telemetry.example.com)
export: web-dev: caches cannot be expressed in sbxenv.yaml (go-mod-cache)
$ cat sbxenv.yaml
schemaVersion: 1
name: web-dev
agent: claude
kits:
- ./mixins/base
workspace:
  path: /home/you/src/web
env:
  NODE_ENV: development
ports:
- sandbox: 3000
  host: 3000
```

Validate the result with sbx itself:

```console
$ sbx env plan sbxenv.yaml
── LOAD ENVIRONMENT
   reading environment file…
     /home/you/sbxenv.yaml
   ✓ environment loaded
── ENVIRONMENT PLAN
   web-dev
   …
   Plan: + 4 to add, ~ 0 to change, - 0 to destroy.
$ sbx env create sbxenv.yaml     # applies the plan
```

## See also

- [Configuration files](configuration.md) — `spec.yaml` and `sandwarden.yaml`, the source of
  truth this export reads.
- `sbx env --help`, `sbx env plan --help` and `sbx env create --help` — the reference for the
  sbxenv schema; it is EXPERIMENTAL and may change.
