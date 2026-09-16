package desktop

import "github.com/JLugagne/sandwarden/internal/store"

// AddProfileMount declares a default bind mount on a profile.
func (d *Desktop) AddProfileMount(profileID int64, req ProfileMountRequest) (store.ProfileMount, error) {
	return d.app.AddProfileMount(d.root, profileID, req.HostPath, req.TargetPath, req.ReadOnly)
}

// RemoveProfileMount deletes a default bind mount from a profile.
func (d *Desktop) RemoveProfileMount(profileID, mountID int64) error {
	return d.app.RemoveProfileMount(d.root, profileID, mountID)
}

// AddProfileCache defaults a shared cache on a profile.
func (d *Desktop) AddProfileCache(profileID, cacheID int64) error {
	return d.app.AddProfileCache(d.root, profileID, cacheID)
}

// RemoveProfileCache stops defaulting a shared cache from a profile.
func (d *Desktop) RemoveProfileCache(profileID, cacheID int64) error {
	return d.app.RemoveProfileCache(d.root, profileID, cacheID)
}

// DetachProfileMount opts a sandbox out of one profile default mount.
func (d *Desktop) DetachProfileMount(name string, mountID int64) error {
	return d.app.DetachProfileMount(d.root, name, mountID)
}

// ApplyProfileMount re-enables a profile default mount on a sandbox.
func (d *Desktop) ApplyProfileMount(name string, mountID int64) error {
	return d.app.ApplyProfileMount(d.root, name, mountID)
}

// DetachProfileCache opts a sandbox out of one profile default cache.
func (d *Desktop) DetachProfileCache(name string, cacheID int64) error {
	return d.app.DetachProfileCache(d.root, name, cacheID)
}

// ApplyProfileCache re-enables a profile default cache on a sandbox.
func (d *Desktop) ApplyProfileCache(name string, cacheID int64) error {
	return d.app.ApplyProfileCache(d.root, name, cacheID)
}
