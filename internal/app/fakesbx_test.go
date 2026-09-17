package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
