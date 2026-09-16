package desktop

import (
	"os"
	"path/filepath"

	"github.com/JLugagne/sandwarden/internal/update"
)

// VersionInfo reports the running build and the latest release state.
type VersionInfo struct {
	Version         string `json:"version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
}

// VersionInfo reports the running build version.
func (d *Desktop) VersionInfo() VersionInfo {
	return VersionInfo{Version: d.version}
}

// CheckUpdates queries the latest stable release. A cached result is returned
// when the last check is fresh unless force is set.
func (d *Desktop) CheckUpdates(force bool) (VersionInfo, error) {
	result, err := update.Check(d.root, updateCacheDir(), d.version, force)
	if err != nil {
		return VersionInfo{Version: d.version}, err
	}
	return VersionInfo{
		Version:         d.version,
		LatestVersion:   result.LatestVersion,
		UpdateAvailable: result.UpdateAvailable,
		ReleaseURL:      result.ReleaseURL,
	}, nil
}

// updateCacheDir is where the last update check is remembered.
func updateCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "sandwarden")
}
