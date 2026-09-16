package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

// fakeSbxScript is a stand-in for the sbx CLI: it keeps a mount table per
// sandbox in a state file and answers `inspect ... --json` with it, so the
// reconcile path can be exercised without a daemon.
const fakeSbxScript = `#!/usr/bin/env python3
import json, os, sys

state = os.environ.get("FAKE_SBX_MOUNTS", "")

def load():
    if not state or not os.path.exists(state):
        return []
    with open(state) as fh:
        return [line.rstrip("\n") for line in fh if line.strip()]

def save(lines):
    with open(state, "w") as fh:
        fh.write("\n".join(lines) + ("\n" if lines else ""))

args = sys.argv[1:]
cmd = args[0] if args else ""
name = args[1] if len(args) > 1 else ""

if cmd == "inspect":
    rows = []
    for line in load():
        parts = line.split("\x1f")
        if parts[0] == name:
            rows.append({"host_path": parts[1], "container_target": parts[2], "read_only": parts[3] == "1"})
    sys.stdout.write(json.dumps({"runtime_mounts": rows}))
    sys.exit(0)

if cmd == "mount":
    spec = args[2] if len(args) > 2 else ""
    fields = spec.split(":")
    host = fields[0]
    target = ""
    ro = False
    if len(fields) == 2:
        if fields[1] == "ro":
            ro = True
        else:
            target = fields[1]
    elif len(fields) >= 3:
        target = fields[1]
        ro = fields[2] == "ro"
    lines = [l for l in load() if not (l.split("\x1f")[0] == name and l.split("\x1f")[1] == host and l.split("\x1f")[2] == target)]
    lines.append("\x1f".join([name, host, target, "1" if ro else "0"]))
    save(lines)
    sys.exit(0)

if cmd == "exec":
    stats = os.environ.get("FAKE_SBX_STATS", "")
    if stats and os.path.exists(stats):
        with open(stats) as fh:
            sys.stdout.write(fh.read())
    sys.exit(0)

if cmd == "umount":
    spec = args[2] if len(args) > 2 else ""
    fields = spec.split(":")
    host = fields[0]
    target = fields[1] if len(fields) > 1 and fields[1] != "ro" else ""
    lines = [l for l in load() if not (l.split("\x1f")[0] == name and l.split("\x1f")[1] == host and l.split("\x1f")[2] == target)]
    save(lines)
    sys.exit(0)

sys.exit(0)
`

type fakeMount struct {
	host   string
	target string
	readOn bool
}

type fakeSbx struct {
	state string
	stats string
}

func newFakeSbx(t *testing.T) *fakeSbx {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "sbx")
	if err := os.WriteFile(script, []byte(fakeSbxScript), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	state := filepath.Join(dir, "mounts")
	stats := filepath.Join(dir, "stats")
	t.Setenv("SBX_BINARY", script)
	t.Setenv("FAKE_SBX_MOUNTS", state)
	t.Setenv("FAKE_SBX_STATS", stats)
	return &fakeSbx{state: state, stats: stats}
}

func (f *fakeSbx) mounts(t *testing.T, sandbox string) []fakeMount {
	t.Helper()
	raw, err := os.ReadFile(f.state)
	if err != nil {
		return nil
	}
	var out []fakeMount
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		parts := strings.Split(line, "\x1f")
		if len(parts) != 4 || parts[0] != sandbox {
			continue
		}
		out = append(out, fakeMount{host: parts[1], target: parts[2], readOn: parts[3] == "1"})
	}
	return out
}

func hasMount(mounts []fakeMount, host, target string, readOn bool) bool {
	for _, m := range mounts {
		if m.host == host && m.target == target && m.readOn == readOn {
			return true
		}
	}
	return false
}

func TestApplyProfileAppliesAndReleasesDefaultMounts(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	p, err := a.CreateProfile(ctx, "dev", "", false, false)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := a.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", true); err != nil {
		t.Fatalf("add profile mount: %v", err)
	}
	if _, err := a.AddProfileMount(ctx, p.ID, "relative/path", "", false); err == nil {
		t.Fatal("expected error for relative host path")
	}

	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if got := fake.mounts(t, "box"); !hasMount(got, "/host/src", "/work/src", true) {
		t.Fatalf("expected profile mount to be applied, got %+v", got)
	}

	if err := a.UnapplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("unapply profile: %v", err)
	}
	if got := fake.mounts(t, "box"); len(got) != 0 {
		t.Fatalf("expected profile mount to be released, got %+v", got)
	}
}

func TestDetachProfileMountSurvivesReconcile(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	m, err := a.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", false)
	if err != nil {
		t.Fatalf("add profile mount: %v", err)
	}
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}

	if err := a.DetachProfileMount(ctx, "box", m.ID); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if got := fake.mounts(t, "box"); len(got) != 0 {
		t.Fatalf("expected the mount to be released on detach, got %+v", got)
	}

	// A reconcile pass must not resurrect an opted-out mount.
	a.syncProfileMounts(ctx, "box", nil)
	if got := fake.mounts(t, "box"); len(got) != 0 {
		t.Fatalf("reconcile re-applied an opted-out mount: %+v", got)
	}
	optOuts, err := a.Store.ProfileOptOuts(ctx, "box")
	if err != nil || len(optOuts) != 1 {
		t.Fatalf("expected one opt-out, got %+v (%v)", optOuts, err)
	}

	if err := a.ApplyProfileMount(ctx, "box", m.ID); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if got := fake.mounts(t, "box"); !hasMount(got, "/host/src", "/work/src", false) {
		t.Fatalf("expected the mount back after re-apply, got %+v", got)
	}
	if optOuts, _ := a.Store.ProfileOptOuts(ctx, "box"); len(optOuts) != 0 {
		t.Fatalf("expected the opt-out to be cleared, got %+v", optOuts)
	}
}

func TestProfileDefaultCacheOptOut(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	cache, err := a.CreateCache(ctx, store.CacheMount{
		Name: "go-mod", HostPath: "/host/go/pkg/mod", TargetPath: "/home/agent/go/pkg/mod", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	if err := a.AddProfileCache(ctx, p.ID, cache.ID); err != nil {
		t.Fatalf("add profile cache: %v", err)
	}
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if got := fake.mounts(t, "box"); !hasMount(got, cache.HostPath, cache.TargetPath, false) {
		t.Fatalf("expected the profile cache to be attached, got %+v", got)
	}

	if err := a.DetachProfileCache(ctx, "box", cache.ID); err != nil {
		t.Fatalf("detach cache: %v", err)
	}
	if got := fake.mounts(t, "box"); len(got) != 0 {
		t.Fatalf("expected the cache bind to be released, got %+v", got)
	}
	if _, errs := a.ReapplyCaches(ctx, "box"); len(errs) != 0 {
		t.Fatalf("reapply should be quiet, got %v", errs)
	}
	if got := fake.mounts(t, "box"); len(got) != 0 {
		t.Fatalf("reapply re-attached an opted-out cache: %+v", got)
	}

	if err := a.ApplyProfileCache(ctx, "box", cache.ID); err != nil {
		t.Fatalf("re-apply cache: %v", err)
	}
	if got := fake.mounts(t, "box"); !hasMount(got, cache.HostPath, cache.TargetPath, false) {
		t.Fatalf("expected the cache back after re-apply, got %+v", got)
	}
}

func TestRemoveProfileDefaultsReleasesBindings(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	m, err := a.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", false)
	if err != nil {
		t.Fatalf("add mount: %v", err)
	}
	cache, _ := a.CreateCache(ctx, store.CacheMount{
		Name: "npm", HostPath: "/host/npm", TargetPath: "/npm", Enabled: true,
	})
	if err := a.AddProfileCache(ctx, p.ID, cache.ID); err != nil {
		t.Fatalf("add cache: %v", err)
	}
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if got := fake.mounts(t, "box"); len(got) != 2 {
		t.Fatalf("expected mount plus cache, got %+v", got)
	}

	if err := a.RemoveProfileMount(ctx, p.ID, m.ID); err != nil {
		t.Fatalf("remove mount: %v", err)
	}
	if got := fake.mounts(t, "box"); hasMount(got, "/host/src", "/work/src", false) {
		t.Fatalf("expected the removed mount to be released, got %+v", got)
	}

	if err := a.RemoveProfileCache(ctx, p.ID, cache.ID); err != nil {
		t.Fatalf("remove cache: %v", err)
	}
	if got := fake.mounts(t, "box"); len(got) != 0 {
		t.Fatalf("expected the removed cache to be released, got %+v", got)
	}
}

func TestDeleteProfileReleasesDefaultsAndOptOuts(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	m, _ := a.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", false)
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if err := a.DetachProfileMount(ctx, "box", m.ID); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if optOuts, _ := a.Store.ProfileOptOuts(ctx, "box"); len(optOuts) != 1 {
		t.Fatalf("expected an opt-out before deletion, got %+v", optOuts)
	}

	if err := a.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if got := fake.mounts(t, "box"); len(got) != 0 {
		t.Fatalf("expected no leftover mounts, got %+v", got)
	}
	if optOuts, _ := a.Store.ProfileOptOuts(ctx, "box"); len(optOuts) != 0 {
		t.Fatalf("expected opt-outs purged with the profile, got %+v", optOuts)
	}
}

func TestProfileMountsForSandboxMergesProfiles(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")

	p1, _ := a.CreateProfile(ctx, "p1", "", false, false)
	p2, _ := a.CreateProfile(ctx, "p2", "", false, false)
	m1, _ := a.AddProfileMount(ctx, p1.ID, "/host/src", "/work/src", false)
	m2, _ := a.AddProfileMount(ctx, p2.ID, "/host/src", "/work/src", true)
	if err := a.Store.AssignProfile(ctx, "box", p1.ID); err != nil {
		t.Fatalf("assign p1: %v", err)
	}
	if err := a.Store.AssignProfile(ctx, "box", p2.ID); err != nil {
		t.Fatalf("assign p2: %v", err)
	}

	live := []sbx.MountInfo{{HostPath: "/host/src", Target: "/work/src", ReadOnly: true}}
	targets, err := a.profileMountsForSandbox(ctx, "box", live)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one merged target, got %+v", targets)
	}
	if len(targets[0].ProfileNames) != 2 || !targets[0].ReadOnly || !targets[0].Attached || targets[0].OptedOut {
		t.Fatalf("unexpected merged target: %+v", targets[0])
	}

	// Opting out of one profile keeps the mount desired by the other.
	if err := a.Store.OptOutProfileItem(ctx, "box", store.OptOutMount, m1.ID); err != nil {
		t.Fatalf("opt out m1: %v", err)
	}
	targets, _ = a.profileMountsForSandbox(ctx, "box", live)
	if len(targets) != 1 || targets[0].OptedOut {
		t.Fatalf("expected the mount to stay desired, got %+v", targets)
	}

	// Opting out of every declaring profile marks the target detached.
	if err := a.Store.OptOutProfileItem(ctx, "box", store.OptOutMount, m2.ID); err != nil {
		t.Fatalf("opt out m2: %v", err)
	}
	targets, _ = a.profileMountsForSandbox(ctx, "box", live)
	if len(targets) != 1 || !targets[0].OptedOut {
		t.Fatalf("expected the target to be opted out, got %+v", targets)
	}
}

func TestSandboxCachesProjectionMergesSources(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	profileCache, _ := a.CreateCache(ctx, store.CacheMount{
		Name: "go", HostPath: "/host/go", TargetPath: "/go", Enabled: true,
	})
	if err := a.AddProfileCache(ctx, p.ID, profileCache.ID); err != nil {
		t.Fatalf("add profile cache: %v", err)
	}
	directCache, _ := a.CreateCache(ctx, store.CacheMount{
		Name: "npm", HostPath: "/host/npm", TargetPath: "/npm", Enabled: true,
	})
	if err := a.Store.AssignCache(ctx, "box", directCache.ID); err != nil {
		t.Fatalf("assign cache: %v", err)
	}
	if err := a.Store.AssignProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("assign profile: %v", err)
	}
	if err := a.Store.OptOutProfileItem(ctx, "box", store.OptOutCache, profileCache.ID); err != nil {
		t.Fatalf("opt out: %v", err)
	}

	rows := a.sandboxCaches(ctx, "box", nil)
	if len(rows) != 2 {
		t.Fatalf("expected both caches, got %+v", rows)
	}
	var goRow, npmRow *SandboxCache
	for i := range rows {
		switch rows[i].Name {
		case "go":
			goRow = &rows[i]
		case "npm":
			npmRow = &rows[i]
		}
	}
	if goRow == nil || goRow.Direct || len(goRow.Profiles) != 1 || goRow.Profiles[0] != "dev" || !goRow.OptedOut {
		t.Fatalf("unexpected profile cache row: %+v", goRow)
	}
	if npmRow == nil || !npmRow.Direct || len(npmRow.Profiles) != 0 || npmRow.OptedOut {
		t.Fatalf("unexpected direct cache row: %+v", npmRow)
	}

	// A direct assignment wins over the opt-out.
	desired, err := a.desiredCaches(ctx, "box")
	if err != nil {
		t.Fatalf("desired caches: %v", err)
	}
	if len(desired) != 1 || desired[0].Name != "npm" {
		t.Fatalf("expected only the direct cache to be desired, got %+v", desired)
	}
}
