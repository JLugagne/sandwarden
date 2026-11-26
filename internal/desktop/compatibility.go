package desktop

import "github.com/JLugagne/sandwarden/internal/sbx"

// SbxCompatibility reports the installed sbx version and, for every feature
// sandwarden version-gates, whether it is supported. A version that cannot be
// read yields Known=false and no restriction: the backend fails open for dev
// builds and the UI mirrors that.
func (d *Desktop) SbxCompatibility() (SbxCompatibilityDTO, error) {
	version, _ := d.app.Sbx.Version(d.root)
	out := SbxCompatibilityDTO{
		Version:  version.String(),
		Known:    version.Known,
		Features: []SbxFeatureCompatDTO{},
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
