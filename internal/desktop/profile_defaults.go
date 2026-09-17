package desktop

import "github.com/JLugagne/sandwarden/internal/fleet"

// AddProfileMount declares a mount on a profile.
func (d *Desktop) AddProfileMount(profileSlug string, req ProfileMountRequest) (fleet.MountRef, error) {
	return d.app.AddProfileMount(d.root, profileSlug, req.HostPath, req.TargetPath, req.ReadOnly)
}

// RemoveProfileMount drops a mount from a profile and detaches it.
func (d *Desktop) RemoveProfileMount(profileSlug, hostPath, targetPath string) error {
	return d.app.RemoveProfileMount(d.root, profileSlug, hostPath, targetPath)
}

// AddProfileCache adds a shared cache to a profile.
func (d *Desktop) AddProfileCache(profileSlug, cacheSlug string) error {
	return d.app.AddProfileCache(d.root, profileSlug, cacheSlug)
}

// RemoveProfileCache drops a shared cache from a profile.
func (d *Desktop) RemoveProfileCache(profileSlug, cacheSlug string) error {
	return d.app.RemoveProfileCache(d.root, profileSlug, cacheSlug)
}

// DetachProfileMount opts a sandbox out of one profile mount.
func (d *Desktop) DetachProfileMount(name, hostPath, targetPath string) error {
	return d.app.DetachProfileMount(d.root, name, hostPath, targetPath)
}

// ApplyProfileMount clears the opt-out and attaches the mount.
func (d *Desktop) ApplyProfileMount(name, hostPath, targetPath string) error {
	return d.app.ApplyProfileMount(d.root, name, hostPath, targetPath)
}

// DetachProfileCache opts a sandbox out of one profile cache.
func (d *Desktop) DetachProfileCache(name, cacheSlug string) error {
	return d.app.DetachProfileCache(d.root, name, cacheSlug)
}

// ApplyProfileCache clears the opt-out and attaches the cache.
func (d *Desktop) ApplyProfileCache(name, cacheSlug string) error {
	return d.app.ApplyProfileCache(d.root, name, cacheSlug)
}
