package fleet

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome resolves a leading ~ with the host user's home directory, so the
// UI can offer presets like ~/.npm and files can stay portable. Anything else
// is returned unchanged.
func ExpandHome(path string) string {
	path = strings.TrimSpace(path)
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
}

// normalize expands ~ in host paths so the runtime always sees absolute
// paths while files can keep a portable spelling.
func (a *SandboxApp) normalize() {
	for i := range a.Mounts {
		a.Mounts[i].HostPath = ExpandHome(a.Mounts[i].HostPath)
	}
	if a.Create != nil {
		for i := range a.Create.Workspaces {
			a.Create.Workspaces[i] = ExpandHome(a.Create.Workspaces[i])
		}
	}
}

// normalize expands ~ in host paths.
func (a *ProfileApp) normalize() {
	for i := range a.Mounts {
		a.Mounts[i].HostPath = ExpandHome(a.Mounts[i].HostPath)
	}
}

// normalize expands ~ in the host path.
func (a *CacheApp) normalize() {
	a.HostPath = ExpandHome(a.HostPath)
}

func (s *Sandbox) normalize() { s.App.normalize() }

func (p *Profile) normalize() { p.App.normalize() }

func (c *Cache) normalize() { c.App.normalize() }
