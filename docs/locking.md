# Cross-process locking

The desktop app and the `sandwarden` CLI can run at the same time against the
same configuration directory. Mutations are read-modify-write cycles: a process
reads a sidecar (or the applied-rules ledger), computes the new state, and
writes it back. Without coordination two processes can lose a sidecar edit
(last writer wins) or double-apply a profile's rules, because both decide the
same rule is missing before either records it.

## Lock file

- Path: `<config dir>/.lock`, where `<config dir>` is `--config`,
  `$SANDWARDEN_CONFIG_DIR`, or `$XDG_CONFIG_HOME/sandwarden` (see
  `fleet.DefaultDir`).
- One lock covers every file in the directory: sandboxes, profiles, caches,
  store registrations, `config.yaml`, and the SQLite ledger those mutations
  touch.
- The file is advisory and kernel-managed (`flock(2)`, `LOCK_EX`). It is
  released when the holding process exits or crashes; a leftover empty
  `.lock` file is harmless.

## Mechanism

`fleet.AcquireLock(dir, wait)`:

1. takes an in-process gate keyed by the absolute lock path, so goroutines of
   one process queue instead of fighting over the file;
2. takes a non-blocking `flock` on `<dir>/.lock`, retrying every 10 ms until
   the deadline.

`wait` is bounded: `app.lockFleet` uses 5 seconds. On contention
`AcquireLock` returns `fleet.ErrLocked` and callers surface the message
unchanged.

`Release` is safe to call more than once. Every locked entry point releases
through `defer`, so errors and panics do not leak the lock.

## Contention contract

```go
var ErrLocked = errors.New("another sandwarden instance is applying changes, retry")
```

- Match it with `errors.Is(err, fleet.ErrLocked)`.
- The message is user-facing: the GUI shows it as returned, and the CLI prints
  `sandbox: another sandwarden instance is applying changes, retry`.
- It means "retry shortly", not "the change was rejected".

## What locks

Each entry point below is one complete mutation: it takes the lock, reloads the
in-memory fleet from disk (so it starts from what the other process just
wrote), performs the read-modify-write, and releases.

| Area | Locked entry points |
| --- | --- |
| Convergence | `App.Apply` (rules, mounts, caches, skills), `App.Reconcile` per target and its ledger prune |
| Profiles | `CreateProfile`, `UpdateProfile`, `DeleteProfile`, `AddRuleToProfile`, `RemoveRuleFromProfile`, `ApplyProfile`, `UnapplyProfile` |
| Profile links | `AddProfileMount`, `RemoveProfileMount`, `AddProfileCache`, `RemoveProfileCache`, `DetachProfileMount`, `ApplyProfileMount`, `DetachProfileCache`, `ApplyProfileCache` |
| Caches | `CreateCache`, `UpdateCache`, `DeleteCache`, `AssignCache`, `UnassignCache` |
| Global config | `SetConfig` |
| Sandbox create | the config write of `CreateSandbox` (the convergence pass locks separately) |

## What does not lock

- **Reads are lock-free, by design.** `ListProfiles`, `GetProfileView`,
  `ListCaches`, `GetCache`, `GetConfig`, fleet listings, `App.Fleet` reads and
  `App.ReloadFleet` never take the lock. A read-only viewer may run
  concurrently with a mutation: sidecar writes are atomic (temp file +
  rename), so a reader sees either the old or the new file, never a torn one.
  The same holds for the SQLite ledger (WAL + busy timeout).
- **Daemon-only calls** such as `ReapplyCaches`, `StartSandbox`, `StopSandbox`,
  `ApplyPolicy` or `Sbx.*` do not lock. They change daemon state, not files.
- **Known gaps** (outside this change): `App.DeleteSandbox`,
  `App.SetSandboxRunArgs`, `App.AddMountAt`, `App.RemoveMountAt` (in
  `internal/app/app.go`) and the skill and kit mutators in
  `internal/app/skills.go` / `internal/app/kits.go` call fleet mutators
  without taking the lock. They still benefit from the atomic per-file writes,
  but must be wrapped in `lockFleet` in a follow-up.

## Nesting

`AcquireLock` is **not reentrant**. A nested acquisition waits for its own
holder and gives up with `ErrLocked` after the bounded wait. The design keeps
this from happening:

- only exported entry points in `internal/app` take the lock;
- helpers they call (`applyProfileToTarget`, `unapplyProfileFromTarget`,
  `convergeProfileRules`, `syncProfileMounts`, `syncDirectMounts`,
  `ReapplyCaches`, `updateSandboxOptOut`, `reapplyProfileToSandboxes`,
  `ensureSandboxConfig`) are lock-free and document "the caller holds the
  fleet lock";
- `App.Apply` is split into a locking shell and an unlocked `applyLocked`
  core, so internal callers such as `Reconcile` lock once per pass rather than
  stacking;
- `internal/fleet` mutators never take the lock themselves: locking each save
  would neither serialize a caller's read-modify-write cycle nor compose with
  the app-level lock.

`TestLockedEntryPointsDoNotNest` and `TestNestedAcquisitionReportsContention`
pin both halves of this contract.

## Non-Unix platforms

`internal/fleet/lock_unix.go` implements `flock` behind `//go:build unix`.
`internal/fleet/lock_other.go` provides no-op fallbacks so the package still
compiles elsewhere; the in-process gate keeps goroutines serialized. Linux and
macOS are the supported platforms.
