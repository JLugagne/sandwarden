# Configuration files

sandwarden keeps every sandbox, profile, cache and git store registration as a file under one
configuration directory. Files are the source of truth: this document is the field-by-field
reference for them, and hand-editing them is a supported workflow.

SQLite is only an index — discovered skill and kit catalogs, per-store sync state, and the ledger
of policy rules sandwarden installed. Deleting it forces a re-discovery and nothing else. Git
store checkouts live in a per-user data directory, not in the configuration directory.

## Table of contents

- [Where the files live](#where-the-files-live)
- [Directory layout](#directory-layout)
- [config.yaml](#configyaml)
- [How files are read and written](#how-files-are-read-and-written)
- [spec.yaml: the kit half](#specyaml-the-kit-half)
- [sandwarden.yaml: sandboxes](#sandwardenyaml-sandboxes)
- [create: the recorded create parameters](#create-the-recorded-create-parameters)
- [sandwarden.yaml: profiles](#sandwardenyaml-profiles)
- [sandwarden.yaml: caches](#sandwardenyaml-caches)
- [Skills, commands and store registrations](#skills-commands-and-store-registrations)
- [Mounts and opt-outs](#mounts-and-opt-outs)
- [Slugs and canonical names](#slugs-and-canonical-names)
- [~ expansion for host paths](#-expansion-for-host-paths)
- [incomplete: true](#incomplete-true)
- [What sandwarden applies vs what is kit-only](#what-sandwarden-applies-vs-what-is-kit-only)
- [Running, applying and recreating](#running-applying-and-recreating)
- [Worked examples](#worked-examples)
- [Writing a sandbox by hand: checklist](#writing-a-sandbox-by-hand-checklist)
- [See also](#see-also)

## Where the files live

| What | Path |
| --- | --- |
| Configuration directory | `--config`, else `$SANDWARDEN_CONFIG_DIR`, else `$XDG_CONFIG_HOME/sandwarden`, else `~/.config/sandwarden` |
| Sandbox | `<config>/sandboxes/<slug>/` |
| Profile | `<config>/profiles/<slug>/` |
| Cache | `<config>/caches/<slug>/` |
| Skill / kit store registration | `<config>/stores/skills/<slug>.yaml`, `<config>/stores/kits/<slug>.yaml` |
| Global preferences | `<config>/config.yaml` |
| Cross-process lock | `<config>/.lock` (see [Cross-process locking](locking.md)) |
| Store checkouts (data, not truth) | `$XDG_DATA_HOME/sandwarden/skill-stores/<slug>/`, `$XDG_DATA_HOME/sandwarden/kit-stores/<slug>/` |
| Index and ledger | `$XDG_STATE_HOME/sandwarden/sandwarden.db` |

## Directory layout

```
<config>/
  config.yaml                        # global preferences (terminal launcher, notifications)
  sandboxes/
    my-vm/
      spec.yaml                      # sbx kit: requires, environment, ports, permissions
      sandwarden.yaml                # canonical name, create params, profiles, caches, skills, mounts, run args
  profiles/
    web-dev/
      spec.yaml                      # permissions.network.allow/deny = the profile rules
      sandwarden.yaml                # default/global flags, mounts, caches, skills
  caches/
    go-mod-cache/
      sandwarden.yaml                # hostPath, targetPath, readOnly, autoAttach, enabled
  stores/
    skills/
      anthropics-skills.yaml         # git store registration (url, ref, auth)
    kits/
      acme-kits.yaml
  .lock
```

The split is the same everywhere: `spec.yaml` is the part sbx can express as a kit, and
`sandwarden.yaml` holds what kits cannot express (canonical names, create parameters, profile
mounts, caches, skills, opt-outs, run arguments). A cache directory contains only
`sandwarden.yaml`: a cache is a host bind mount, which no kit can express.

Sandwarden creates the standard subdirectories when it opens the configuration directory. Any
other file in an entity directory is ignored.

### config.yaml

`config.yaml` at the root of the configuration directory holds global preferences:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `terminals.enabled` | list of strings | absent (all terminals) | Terminal emulator ids the launcher may use. Absent means every detected terminal. |
| `terminals.default` | string | empty | Preferred terminal id; empty falls back to the first enabled terminal. |
| `notifications` | bool | `false` | Native desktop notifications for newly blocked hosts. |

## How files are read and written

- A sandbox or profile directory **must contain both** `spec.yaml` and `sandwarden.yaml`; if either
  is missing or invalid, the entity is skipped for that reload and the error is reported (by
  `sandwarden status`, and in the GUI). A cache needs only `sandwarden.yaml`.
- Decoding is **strict**: an unknown YAML key is an error, not a silently ignored field. A typo
  such as `optouts:` instead of `optOuts:` makes the sandbox disappear from the fleet until it is
  fixed.
- Writes are atomic (temporary file + rename), so a reader never sees a torn file.
- When sandwarden saves a file it re-marshals the decoded structs. Comments and formatting are
  **not preserved** on the next save, and `~` in host paths is written back expanded.
- `spec.yaml` is a valid sbx kit: `sbx kit validate <entity dir>` accepts the directory, which is
  what `sandwarden validate` runs. The sidecar is sandwarden's own schema and is not seen by sbx.

## spec.yaml: the kit half

`sandbox` and `profile` spec files are the subset of the sbx kit schema v2 that sandwarden reads
and writes. They are decoded strictly, and credentials and other opaque blocks are carried through
untouched. `sbx kit validate` remains the reference validator for the full schema — use it for
anything beyond the fields below.

| Key | Type | Notes |
| --- | --- | --- |
| `schemaVersion` | string | `"2"`. Sandwarden forces it to `"2"` when it writes. |
| `kind` | string | `mixin`. Sandwarden forces it to `mixin` when it writes. |
| `name` | string | Sandwarden forces it to the directory slug when it writes. |
| `displayName` | string | UI label; also the canonical-name fallback when the sidecar has no `sandbox` key. |
| `description` | string | Metadata. Shown for profiles in the GUI. |
| `sourceURL` | string | Metadata, round-tripped only. |
| `requires.agent` | string | Required to create a sandbox: a built-in agent name (for example `claude`) or a kit reference. It becomes the agent argument of `sbx create`. |
| `sandbox.image` | string | Round-tripped only. |
| `sandbox.entrypoint` | list of strings | Round-tripped only. |
| `sandbox.command.default` / `sandbox.command.interactive` | string or argv list | Round-tripped only. An argv list is joined with spaces, as sbx does. |
| `sandbox.resources.cpu` | number | Round-tripped only. |
| `sandbox.resources.memory` | string | Round-tripped only. |
| `environment.variables` | mapping `KEY: value` | Injected at create as `-e KEY=VALUE`. |
| `ports[].container` | int | Round-tripped only; publishing uses `create.publish`. |
| `ports[].protocol`, `ports[].name` | string | Round-tripped only. |
| `permissions.network.allow` | list of strings | Profile specs: compiled into sandbox-scoped daemon rules. Sandbox specs: round-tripped only. |
| `permissions.network.deny` | list of strings | Profile specs: compiled into daemon rules. Sandbox specs: passed to `sbx create --deny-network`. |
| `setup.install[]`, `setup.startup[]` | list of `{command, user, description}` | Round-tripped only. `command` is a string or argv list. |
| `setup.files[]` | list of `{path, mode, description}` | Round-tripped only. |
| `credentials` | list | Opaque, round-tripped only. |
| `arguments` | mapping | Opaque, round-tripped only. |

"Round-tripped only" means sandwarden preserves the value when it saves the file but never applies
it to the daemon. To have sbx itself apply those fields, pass the directory (or another kit) as a
kit — see
[What sandwarden applies vs what is kit-only](#what-sandwarden-applies-vs-what-is-kit-only).

Values in `environment.variables` are written in clear. Sandwarden refuses to write a value that
looks like a credential (known prefixes such as `sk-`, `ghp_`, `xoxb-`, AWS keys, PEM blocks, or
long high-entropy tokens) from the create form or `sandwarden create --env`: keep only the bare
`KEY` form for secret values — the value is then read from the host environment — and manage the
value with `sbx secret`. `sandwarden doctor` scans existing spec files for likely secrets and
never prints the values; see [secrets.md](secrets.md).

Note that **recording a create rebuilds `spec.yaml`**. When sandwarden creates a sandbox — the GUI
create form, `sandwarden create`, or `sandwarden run` on a sandbox that is absent from the daemon —
it writes a spec containing only `displayName`, `requires.agent`, `environment.variables` and
`permissions.network.deny`; any other field you hand-wrote in the spec is dropped. Sidecar-only
edits (mounts, caches, skills, profiles, opt-outs, run arguments) and `recreate` keep the spec as
it is.

## sandwarden.yaml: sandboxes

```yaml
sandbox: Web dev            # canonical sandboxd name
create:                     # recorded create parameters (see below)
  cpus: 4
  memory: 8g
profiles: [web-dev]         # profile slugs
caches: [go-mod-cache]      # cache slugs attached directly to this sandbox
skills:                     # catalog items attached directly to this sandbox
  - store: anthropics-skills
    kind: command
    name: review
mounts:                     # direct host bind mounts
  - hostPath: /home/you/data
    targetPath: /data
    readOnly: true
runArgs: --model opus       # appended after -- to the connect command
optOuts:                    # inherited items this sandbox detached
  mounts:
    - /home/you/src/shared:/shared
  caches:
    - npm-cache
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `sandbox` | string | `spec.displayName` | Canonical sandboxd name. The directory slug may differ from it. |
| `create` | mapping | absent | Parameters the sandbox was created with, so it can be recreated. See the next section. |
| `profiles` | list of slugs | empty | Profiles whose rules, mounts, caches and skills this sandbox inherits. Missing slugs produce an apply warning. |
| `caches` | list of slugs | empty | Caches attached directly, in addition to the ones profiles provide. A direct entry wins over an opt-out. |
| `skills` | list of `SkillRef` | empty | Catalog items attached directly. See [Skills, commands and store registrations](#skills-commands-and-store-registrations). |
| `mounts` | list of `MountRef` | empty | Direct bind mounts. See [Mounts and opt-outs](#mounts-and-opt-outs). |
| `runArgs` | string | empty | Arguments appended after `--` to the connect command (`sbx run --name <name> -- <runArgs>`). |
| `optOuts.mounts` | list of strings | empty | Keys of profile mounts to leave detached. |
| `optOuts.caches` | list of slugs | empty | Profile caches to leave detached. |

## create: the recorded create parameters

Everything `create` records is replayed verbatim by `sandwarden recreate` and by
`sandwarden run` when the sandbox no longer exists in the daemon. The keys map to `sbx create`
flags:

| Key | Type | Default | `sbx create` flag |
| --- | --- | --- | --- |
| `cpus` | int | `0` (not passed) | `--cpus` |
| `memory` | string, e.g. `8g` | empty (not passed) | `--memory` |
| `workspaces` | list of host paths | empty | positional arguments; a `:ro` suffix marks a read-only workspace |
| `clone` | bool | `false` | `--clone` |
| `template` | string | empty | `--template` (container image template) |
| `daemonProfile` | string | empty | `--profile` (sandboxd governance profile, not a sandwarden profile) |
| `publish` | list of strings, e.g. `3000:3000` | empty | `-p` |
| `kits` | list of kit references | empty | `--kit` (external mixin kits; any reference sbx accepts, for example a `git+…#dir=…&ref=…` URL) |
| `incomplete` | bool | `false` | Marker, not a flag. See [incomplete: true](#incomplete-true). |

`create` may be absent; `recreate` then replays only what the spec provides (`requires.agent`,
environment variables and network deny patterns).

## sandwarden.yaml: profiles

A profile is a named allow/deny bundle plus inherited items. Its `spec.yaml` carries the network
rules; its sidecar carries everything else.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `default` | bool | `false` | New sandboxes created through sandwarden get this profile appended to their `profiles:` list. At most one default is kept by the GUI/CLI; if hand-written files set several, the alphabetically first slug wins. |
| `global` | bool | `false` | The profile applies to every sandbox in the daemon, referenced or not, including sandboxes with no config directory. Its mounts, caches and skills are inherited too. |
| `mounts` | list of `MountRef` | empty | Bind mounts every covered sandbox inherits. |
| `caches` | list of slugs | empty | Caches every covered sandbox inherits. |
| `skills` | list of `SkillRef` | empty | Catalog items every covered sandbox inherits. |

A sandbox's effective profiles are its `profiles:` slugs plus every `global` profile,
deduplicated. The profile's allow/deny patterns are compiled into sandbox-scoped `sandboxd` policy
rules (see
[What sandwarden applies vs what is kit-only](#what-sandwarden-applies-vs-what-is-kit-only)).

## sandwarden.yaml: caches

A cache is a named host directory that can be bind-mounted into any number of sandboxes. Its slug
is the directory name under `caches/`; `caches:` lists in sandboxes and profiles reference that
slug.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | required | Display name. |
| `description` | string | empty | Free text. |
| `hostPath` | string | required | Host directory to mount. Absolute, or `~`-prefixed. Validated as absolute when created through sandwarden. |
| `targetPath` | string | `hostPath` | Container path to mount at. Must be absolute. Never `~`-expanded. |
| `readOnly` | bool | `false` | Mount the cache read-only. |
| `autoAttach` | bool | `true` | When the create request asks for auto-attach (the default for `sandwarden create` and `sandwarden run`), append this cache to the new sandbox's `caches:` list. Absent means true. |
| `enabled` | bool | `true` | When false, the cache is never mounted, even if referenced. Absent means true. |

Caches declared by a profile are inherited by the sandboxes that profile covers; caches declared in
a sandbox's `caches:` are attached directly. The merged set is deduplicated by slug, minus the
sandbox's `optOuts.caches`, and only entries whose definition is `enabled` are mounted.

## Skills, commands and store registrations

Skills and commands come from a git **store**. Registering a store creates
`stores/skills/<slug>.yaml`:

```yaml
name: Anthropic skills
description: Official skills and commands
url: https://github.com/anthropics/skills
ref: main
auth: ""
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | required | Display name of the store. |
| `description` | string | empty | Free text. |
| `url` | string | required | Git URL. An HTTPS URL may embed credentials. |
| `ref` | string | empty (default branch) | Branch or tag to check out. |
| `auth` | string | empty | `""` (public), or `ssh` to authenticate with the ssh-agent or the private keys of `~/.ssh`. |

The store `slug` is the file name and the `kind` (`skill` or `kit`) is the registry
(`stores/skills/`, `stores/kits/`); neither is written in the file. The catalog of items is
discovered from the checkout and cached in SQLite — it is an index, not truth. A selected item is
referenced with `SkillRef`:

| Key | Type | Meaning |
| --- | --- | --- |
| `store` | string | Slug of the skill store registration (the `.yaml` file name). |
| `kind` | string | `skill` or `command`. |
| `name` | string | Item name as discovered from the checkout. |

In the sidecar the three are separate keys; on the CLI a direct attachment is written
`--skill store:kind:name`, for example `--skill anthropics-skills:command:review`.

A skill is mounted read-only at `/home/agent/.agents/skills/<name>`; a command is mounted
read-only at `/home/agent/.agents/commands/<name>.md`. Two selected items that resolve to the same
target are a conflict and are reported by `apply`; a reference whose store or item is missing is
reported as an error and skipped. Mounts under those two directories that sandwarden no longer
wants are unmounted during apply. Kit stores are a catalog for the create form only: nothing in a
sidecar references a kit store.

## Mounts and opt-outs

A `MountRef` is a host bind mount:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `hostPath` | string | required | Host path. Absolute or `~`-prefixed. |
| `targetPath` | string | empty | Container path. When empty, the host path is used as the target. |
| `readOnly` | bool | `false` | Mount read-only. |

Sandbox `mounts:` are direct mounts. Profile `mounts:` are inherited by every sandbox the profile
covers; when several profiles declare the same `hostPath` and `targetPath`, they merge into one
mount and `readOnly` is true if any of them says so.

**Detach** (opt out) records a key in the sandbox sidecar and unmounts it if the sandbox is running:

```yaml
optOuts:
  mounts:
    - /home/you/src/shared:/shared   # hostPath:targetPath, from the profile
  caches:
    - npm-cache                      # cache slug, from the profile
```

The mount key is the literal `hostPath + ":" + targetPath`, using the expanded host path and the
target path exactly as the profile declares them. A mount with no `targetPath` therefore keys with
a trailing colon: `/home/you/src/shared:`. A key that does not match exactly has no effect.

**Apply** clears the entry from `optOuts` and attaches the item immediately. Detaching only affects
profile-provided items: a direct cache reference in the sandbox's `caches:` wins over
`optOuts.caches`, and direct mounts are never opted out. When `optOuts` becomes empty sandwarden
drops the block.

## Slugs and canonical names

Every entity lives in a directory whose name is its **slug**, and the slug identifies it in every
reference (`profiles:`, `caches:`, `store:`). When sandwarden creates an entity from a display
name it derives the slug with these rules:

- lowercased ASCII letters and digits are kept; every other run of characters becomes a single
  `-`; leading and trailing `-` are trimmed. `My VM` → `my-vm`, `v1.2.3` → `v1-2-3`, `a/b\c` →
  `a-b-c`, `ÄÖÜ` → no ASCII characters at all.
- the result is truncated to 64 characters (collision suffixes are appended after truncation).
- an empty result becomes `item`.
- if the directory already exists, sandwarden appends `-2`, `-3`, … until the slug is free
  (`my-vm`, `my-vm-2`, …). This applies to sandboxes, profiles and caches. Store registrations are
  files rather than directories and are keyed by `<slug>.yaml`, so two registrations that slugify
  to the same name share the same file.

When you write files by hand, **the directory name is the slug** and is used as-is — no slugifying
happens on load. Keep `spec.name` equal to the directory name; sandwarden forces it on the next
save. The canonical sandboxd name is separate:

- it is the sidecar `sandbox:` value, falling back to `spec.displayName` when `sandbox:` is empty;
- commands like `sandwarden run`, `start`, `apply`, `recreate` and `rm` address a sandbox by its
  **canonical name** (`sandwarden run "Web dev"`), not by its slug;
- `sandwarden validate` is the exception: it accepts a canonical name or a slug;
- `CreateSandbox` derives the slug from the display name and stores the display name as the
  canonical name, so `My VM` typically has slug `my-vm` and canonical name `My VM`.

If you want commands to use the slug, set `sandbox:` to the slug and keep `spec.displayName` for
the UI label.

## ~ expansion for host paths

A leading `~` in a **host** path is resolved to the host user's home directory at load time:

| Field | Expanded |
| --- | --- |
| sandbox `mounts[].hostPath` | yes |
| sandbox `create.workspaces[]` | yes |
| profile `mounts[].hostPath` | yes |
| cache `hostPath` | yes |
| `targetPath` fields, `runArgs`, store URLs | no |

`~` alone resolves to the home directory; `~user` is not expanded. Because sandwarden expands on
load and writes the expanded value back, a `~`-prefixed path in a hand-written file still works,
but reads back as an absolute path after the next save.

## incomplete: true

When a sandwarden entry point needs the config of a sandbox that has none (for example attaching
a profile or a cache from the GUI), sandwarden imports it on demand with
`create.incomplete: true`. `sandwarden import` does the same for every daemon sandbox at once (or
for the named ones). Only what the daemon exposes is recovered:

- `requires.agent`,
- the main and additional workspaces,
- applied kits,
- `template` (from the image), only when there was no agent to record.

Not recoverable from daemon state, and therefore absent from the new sidecar: CPUs, memory,
environment variables, published ports, the sandboxd governance profile, the clone flag, and
network deny patterns.

The marker is surfaced wherever the sandbox is shown:

- `sandwarden status` prints `config=incomplete` and a one-line explanation of what is missing;
- the `--json` payloads carry `incomplete` (the shared sandbox shape, and the detail projection
  under `detail.incomplete`);
- the GUI badges the sandbox card and the detail header, and the detail page explains what a
  recreate will not restore.

A `recreate` of an incomplete config warns before proceeding and then replays only the recovered
fields: the missing CPU, memory, environment, published ports, daemon profile and clone flag are
not restored.

To complete the config, record the missing create parameters, which clears the marker. In the GUI,
"Complete this config" on the sandbox detail page writes `create.cpus`, `create.memory` and
`spec.environment.variables` from the form and clears `create.incomplete`. By hand, edit
`sandwarden.yaml` and `spec.yaml` and either fill the fields or drop `incomplete: true`. The marker
also clears when the sandbox is created again through sandwarden (`sandwarden run` on a sandbox
absent from the daemon, `sandwarden create`, or the GUI create form), because that rebuilds the
sidecar from the create request.

## What sandwarden applies vs what is kit-only

Sandwarden compiles what it understands and applies it directly:

| Source | Mechanism |
| --- | --- |
| Profile `permissions.network.allow` / `deny` | Sandbox-scoped `sandboxd` policy rules, tracked in the ledger so only sandwarden's own rules are removed. A `global` profile is applied to every sandbox. |
| `spec.requires.agent` | Agent argument of `sbx create`. |
| `spec.environment.variables` | `-e KEY=VALUE` at create. |
| `spec.permissions.network.deny` | `--deny-network` at create. |
| `create.*` | `sbx create` flags (see [create](#create-the-recorded-create-parameters)). |
| Profile and direct `mounts` | Bind mounts on the running sandbox. |
| Caches (profile + direct, minus opt-outs, enabled only) | Bind mounts on the running sandbox. |
| `skills` | Read-only mounts under `/home/agent/.agents/`. |
| `runArgs` | Appended after `--` to the connect command. |

Everything else in `spec.yaml` is carried for kit tooling but not applied: `sandbox.*`, `ports`,
`setup`, `credentials`, `arguments`, the sandbox spec's `permissions.network.allow`, `sourceURL`.

`sandwarden validate SANDBOX` runs `sbx kit validate` over the sandbox's directory; the GUI can
also validate profiles and catalog kit items. Validation checks `spec.yaml` against the kit schema
— it does not check the sidecar, which is already decoded strictly on load.

The sidecar's `create.kits` list is the only thing sandwarden passes to `sbx create --kit`: external
mixin kits, by any reference sbx accepts (for example a local directory or a
`git+…#dir=…&ref=…` URL). The sandbox directory itself is a valid kit and can be passed to
`sbx create --kit` by hand, which is how sbx's own kit machinery would apply the fields sandwarden
ignores; sandwarden does not add it to `create.kits` itself.

## Running, applying and recreating

- `sandwarden run NAME [-- ARGS...]` — if the sandbox does not exist in the daemon, create it from
  its files (this rewrites `spec.yaml` from the create request, see the note above), start it,
  apply it and attach. If it exists, start it and converge it.
- `sandwarden start NAME`, `sandwarden apply [NAME...]` — converge a running sandbox onto its
  files. `apply` reports an error if the sandbox is not running. Both re-apply profile rules,
  profile and direct mounts, caches and skills, because bind mounts do not survive a restart.
  Convergence is idempotent and per-item: a failing item is reported and the pass continues.
- `sandwarden watch` keeps converging in the background.
- `sandwarden rm NAME` removes the sandbox from the daemon and **keeps its config directory**
  (`--purge` deletes it too), so `run` can recreate it identically.
- `sandwarden import [SANDBOX...]` adopts daemon sandboxes into the config directory — every one
  of them when none is named — and reports created / already configured / failed per sandbox. A
  sandbox that cannot be imported does not abort the batch. Adopted configs are marked
  `incomplete: true`; see [incomplete: true](#incomplete-true).
- `sandwarden recreate NAME` deletes the sandbox from the daemon (keeping the files), creates it
  again from `create:` plus the spec's agent, environment and deny patterns, then applies. It
  warns first when the config is incomplete.

Recreate consequences:

- Changes to `create:` fields and to the spec's `requires.agent`, `environment.variables` and
  `permissions.network.deny` only take effect on recreate. Changing the `agent`, a workspace or a
  kit is flagged by `apply` as "recreate required"; CPU, memory and environment are not observable
  through `sbx inspect`, so they are not flagged.
- Anything inside the sandbox filesystem that is not on a mount is lost. Workspaces, mounts,
  caches and skills are restored from the files afterwards.
- A sandbox marked `incomplete: true` is recreated from the recovered fields only, so its CPU,
  memory, environment, published ports, daemon profile and clone flag are not restored. `recreate`
  warns before proceeding when the config is incomplete.

## Worked examples

The configuration directory below is `~/.config/sandwarden`. The store registration is shared by
the two sandboxes.

### Minimal sandbox (agent only)

`sandboxes/my-vm/spec.yaml`:

```yaml
schemaVersion: "2"
kind: mixin
name: my-vm
displayName: My VM
requires:
  agent: claude
```

`sandboxes/my-vm/sandwarden.yaml`:

```yaml
sandbox: My VM
```

Create it and attach:

```sh
sandwarden run "My VM"
```

The canonical name is `My VM`; the slug is `my-vm`; `sandwarden validate my-vm` accepts the slug.

### Fuller sandbox

`sandboxes/web-dev/spec.yaml`:

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
    LOG_LEVEL: debug
permissions:
  network:
    deny:
      - telemetry.example.com
```

`sandboxes/web-dev/sandwarden.yaml`:

```yaml
sandbox: web-dev
create:
  cpus: 4
  memory: 8g
  workspaces:
    - /home/you/src/web
    - /home/you/src/api:ro
  clone: false
  template: ubuntu-24.04
  publish:
    - "3000:3000"
  kits:
    - "git+https://github.com/acme/sbx-kits#dir=node&ref=v1"
profiles:
  - web-dev
caches:
  - go-mod-cache
skills:
  - store: anthropics-skills
    kind: command
    name: review
mounts:
  - hostPath: /home/you/data
    targetPath: /data
    readOnly: true
runArgs: --model opus
optOuts:
  mounts:
    - /home/you/src/shared:/shared
  caches:
    - npm-cache
```

This sandbox has 4 CPUs, 8g of memory, two workspaces (the second read-only), publishes port 3000,
applies one external kit, inherits the `web-dev` profile's rules, attaches the `go-mod-cache` cache
and a skill command directly, adds one direct mount, connects with `--model opus`, and opts out of
the profile's `/home/you/src/shared:/shared` mount and its `npm-cache` cache.

### Profile example

`profiles/web-dev/spec.yaml`:

```yaml
schemaVersion: "2"
kind: mixin
name: web-dev
displayName: Web dev
description: Package registries and the shared scratch directory
permissions:
  network:
    allow:
      - api.github.com
      - registry.npmjs.org
    deny:
      - telemetry.example.com
```

`profiles/web-dev/sandwarden.yaml`:

```yaml
default: true
global: false
mounts:
  - hostPath: /home/you/src/shared
    targetPath: /shared
    readOnly: true
caches:
  - npm-cache
skills:
  - store: anthropics-skills
    kind: skill
    name: pdf
```

### Cache example

`caches/go-mod-cache/sandwarden.yaml`:

```yaml
name: Go module cache
description: Shared Go module and build cache
hostPath: ~/go/pkg/mod
targetPath: /go/pkg/mod
readOnly: false
autoAttach: true
enabled: true
```

`hostPath` is expanded to `/home/you/go/pkg/mod`. Because the cache is `autoAttach`, new sandboxes
created through sandwarden get `go-mod-cache` appended to their `caches:` list.

### Store registration example

`stores/skills/anthropics-skills.yaml`:

```yaml
name: Anthropic skills
description: Official skills and commands
url: https://github.com/anthropics/skills
ref: main
```

## Writing a sandbox by hand: checklist

1. Pick a slug and create `sandboxes/<slug>/`.
2. Write `spec.yaml` with `schemaVersion: "2"`, `kind: mixin`, `name: <slug>`,
   `requires.agent: <agent>` and, optionally, environment variables and network deny patterns.
3. Write `sandwarden.yaml` with `sandbox: <canonical name>`. Add `create:`, `profiles:`, `caches:`,
   `skills:`, `mounts:`, `runArgs:` and `optOuts:` as needed. Every key is camelCase; an unknown
   key is an error.
4. Make sure any profile and cache slugs you reference exist under `profiles/` and `caches/`, and
   any skill store slug exists under `stores/skills/`.
5. Check the files load: `sandwarden status` prints per-file config errors.
6. Validate the kit half: `sandwarden validate <slug>` (or `sbx kit validate sandboxes/<slug>`).
7. Run it: `sandwarden run "<canonical name>"`. `run` creates the sandbox if needed, starts it and
   applies mounts, caches, skills and rules.

## See also

- [Cross-process locking](locking.md) — how concurrent GUI and CLI mutations are serialized.
- `README.md` § Configuration — environment variables and command-line flags.
