package desktop

import "github.com/JLugagne/sandwarden/internal/sbx"

// SbxCompatibility reports the installed sbx client and daemon versions and,
// for every feature sandwarden version-gates, whether it is supported. A
// version that cannot be read yields Known=false and no restriction: the
// backend fails open for dev builds and the UI mirrors that.
func (d *Desktop) SbxCompatibility() (SbxCompatibilityDTO, error) {
	// Always re-read: the compatibility panel must reflect an sbx upgraded
	// while the app keeps running, not a version cached at startup.
	versions, _ := d.app.Sbx.RefreshVersions(d.root)
	version := versions.Effective()
	out := SbxCompatibilityDTO{
		Version:               version.String(),
		ClientVersion:         versions.Client.String(),
		DaemonVersion:         versions.Server.String(),
		Known:                 version.Known,
		DaemonRestartRequired: versions.DaemonRestartRequired(),
		Features:              []SbxFeatureCompatDTO{},
	}
	for _, feature := range sbx.Features() {
		out.Features = append(out.Features, SbxFeatureCompatDTO{
			ID:         feature.ID,
			Label:      feature.Label,
			MinVersion: feature.MinVersion,
			Supported:  version.AtLeast(feature.MinVersion),
		})
	}
	return out, nil
}

// RestartDaemon restarts sandboxd non-interactively. The sbx client drops its
// version cache on success, so the next SbxCompatibility read sees the fresh
// daemon. Callers should invalidate the compatibility query afterwards.
func (d *Desktop) RestartDaemon() error {
	return d.app.Sbx.RestartDaemon(d.root)
}
