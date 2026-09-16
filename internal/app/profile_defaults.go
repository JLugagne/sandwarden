package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ProfileMountTarget is one bind mount a sandbox should have because of its
// profiles, merged across every profile that declares it.
type ProfileMountTarget struct {
	HostPath     string   `json:"host_path"`
	TargetPath   string   `json:"target_path"`
	ReadOnly     bool     `json:"read_only"`
	ProfileNames []string `json:"profile_names"`
	// ProfileMountIDs lists the declaring profile_mounts rows so the UI can
	// opt out of (or re-enable) every declaration at once.
	ProfileMountIDs []int64 `json:"profile_mount_ids"`
}

// SandboxProfileMount is a profile mount as seen from one sandbox: the merged
// target plus its opt-out and live attachment state.
type SandboxProfileMount struct {
	ProfileMountTarget
	OptedOut bool `json:"opted_out"`
	Attached bool `json:"attached"`
}

// AddProfileMount declares a default bind mount on a profile and applies it to
// the sandboxes that already have the profile.
func (a *App) AddProfileMount(ctx context.Context, profileID int64, hostPath, targetPath string, readOnly bool) (store.ProfileMount, error) {
	hostPath = strings.TrimSpace(hostPath)
	targetPath = strings.TrimSpace(targetPath)
	if hostPath == "" {
		return store.ProfileMount{}, errors.New("host path is required")
	}
	if !filepath.IsAbs(hostPath) {
		return store.ProfileMount{}, errors.New("host path must be absolute")
	}
	if targetPath != "" && !filepath.IsAbs(targetPath) {
		return store.ProfileMount{}, errors.New("target path must be absolute")
	}
	if _, err := a.Store.GetProfile(ctx, profileID); err != nil {
		return store.ProfileMount{}, err
	}
	m, err := a.Store.AddProfileMount(ctx, profileID, hostPath, targetPath, readOnly)
	if err != nil {
		return store.ProfileMount{}, err
	}
	a.syncProfileSandboxes(ctx, profileID, nil, nil)
	a.Notify(TopicProfiles)
	a.Notify(TopicSandboxes)
	return m, nil
}

// RemoveProfileMount deletes a default mount and releases its bind from the
// running sandboxes that no longer want it.
func (a *App) RemoveProfileMount(ctx context.Context, profileID, mountID int64) error {
	m, err := a.Store.RemoveProfileMount(ctx, profileID, mountID)
	if err != nil {
		return err
	}
	a.syncProfileSandboxes(ctx, profileID, []store.ProfileMount{m}, nil)
	a.Notify(TopicProfiles)
	a.Notify(TopicSandboxes)
	return nil
}

// AddProfileCache defaults a shared cache on a profile and attaches it to the
// sandboxes that already have the profile.
func (a *App) AddProfileCache(ctx context.Context, profileID, cacheID int64) error {
	if _, err := a.Store.GetProfile(ctx, profileID); err != nil {
		return err
	}
	if _, err := a.Store.GetCacheMount(ctx, cacheID); err != nil {
		return err
	}
	if err := a.Store.AddProfileCache(ctx, profileID, cacheID); err != nil {
		return err
	}
	a.syncProfileSandboxes(ctx, profileID, nil, nil)
	a.Notify(TopicProfiles)
	a.Notify(TopicCaches)
	return nil
}

// RemoveProfileCache stops defaulting a cache and releases it from the running
// sandboxes that no longer want it.
func (a *App) RemoveProfileCache(ctx context.Context, profileID, cacheID int64) error {
	c, err := a.Store.GetCacheMount(ctx, cacheID)
	if err != nil {
		return err
	}
	if err := a.Store.RemoveProfileCache(ctx, profileID, cacheID); err != nil {
		return err
	}
	a.syncProfileSandboxes(ctx, profileID, nil, []store.CacheMount{c})
	a.Notify(TopicProfiles)
	a.Notify(TopicCaches)
	return nil
}

// DetachProfileMount opts a sandbox out of one profile mount and releases the
// bind when no other source keeps it.
func (a *App) DetachProfileMount(ctx context.Context, sandbox string, mountID int64) error {
	if _, err := a.Sbx.InspectSandbox(ctx, sandbox); err != nil {
		return err
	}
	m, ok := a.profileMountByID(ctx, sandbox, mountID)
	if !ok {
		return fmt.Errorf("profile mount %d is not declared by the sandbox profiles", mountID)
	}
	if err := a.Store.OptOutProfileItem(ctx, sandbox, store.OptOutMount, mountID); err != nil {
		return err
	}
	a.syncProfileMounts(ctx, sandbox, []store.ProfileMount{m})
	a.Notify(TopicProfiles)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// ApplyProfileMount re-enables a profile mount the sandbox had detached.
func (a *App) ApplyProfileMount(ctx context.Context, sandbox string, mountID int64) error {
	if _, err := a.Sbx.InspectSandbox(ctx, sandbox); err != nil {
		return err
	}
	if err := a.Store.ClearProfileItemOptOut(ctx, sandbox, store.OptOutMount, mountID); err != nil {
		return err
	}
	a.syncProfileMounts(ctx, sandbox, nil)
	a.Notify(TopicProfiles)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// DetachProfileCache opts a sandbox out of one profile default cache and
// releases the bind when nothing else wants it.
func (a *App) DetachProfileCache(ctx context.Context, sandbox string, cacheID int64) error {
	c, err := a.Store.GetCacheMount(ctx, cacheID)
	if err != nil {
		return err
	}
	if _, err := a.Sbx.InspectSandbox(ctx, sandbox); err != nil {
		return err
	}
	if err := a.Store.OptOutProfileItem(ctx, sandbox, store.OptOutCache, cacheID); err != nil {
		return err
	}
	a.releaseStaleCaches(ctx, sandbox, []store.CacheMount{c})
	a.Notify(TopicProfiles)
	a.Notify(TopicCaches)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// ApplyProfileCache re-enables a profile default cache the sandbox had
// detached and mounts it when the sandbox is running.
func (a *App) ApplyProfileCache(ctx context.Context, sandbox string, cacheID int64) error {
	if _, err := a.Sbx.InspectSandbox(ctx, sandbox); err != nil {
		return err
	}
	if err := a.Store.ClearProfileItemOptOut(ctx, sandbox, store.OptOutCache, cacheID); err != nil {
		return err
	}
	a.ReapplyCaches(ctx, sandbox)
	a.Notify(TopicProfiles)
	a.Notify(TopicCaches)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// desiredCaches returns every cache a sandbox should have: direct assignments
// plus profile defaults, minus the ones the sandbox opted out of. A direct
// assignment always wins over an opt-out.
func (a *App) desiredCaches(ctx context.Context, sandbox string) ([]store.CacheMount, error) {
	direct, err := a.Store.ListCachesForSandbox(ctx, sandbox)
	if err != nil {
		return nil, err
	}
	profiled, err := a.Store.ProfileCachesForSandbox(ctx, sandbox)
	if err != nil {
		return nil, err
	}
	optOuts, err := a.Store.ProfileOptOuts(ctx, sandbox)
	if err != nil {
		return nil, err
	}
	merged := map[int64]store.CacheMount{}
	for _, c := range profiled {
		if optOuts[store.ProfileOptOutKey(store.OptOutCache, c.ID)] {
			continue
		}
		merged[c.ID] = c.CacheMount
	}
	for _, c := range direct {
		merged[c.ID] = c
	}
	out := make([]store.CacheMount, 0, len(merged))
	for _, c := range merged {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// profileMountsForSandbox merges every mount declared by the sandbox profiles
// by host/target and annotates it with opt-out and attachment state.
func (a *App) profileMountsForSandbox(ctx context.Context, sandbox string, live []sbx.MountInfo) ([]SandboxProfileMount, error) {
	declared, err := a.Store.ProfileMountsForSandbox(ctx, sandbox)
	if err != nil {
		return nil, err
	}
	if len(declared) == 0 {
		return nil, nil
	}
	optOuts, err := a.Store.ProfileOptOuts(ctx, sandbox)
	if err != nil {
		return nil, err
	}
	type key struct{ host, target string }
	grouped := map[key]*SandboxProfileMount{}
	var order []key
	for _, row := range declared {
		k := key{row.HostPath, row.TargetPath}
		target, ok := grouped[k]
		if !ok {
			target = &SandboxProfileMount{ProfileMountTarget: ProfileMountTarget{
				HostPath:   row.HostPath,
				TargetPath: row.TargetPath,
				ReadOnly:   row.ReadOnly,
			}}
			grouped[k] = target
			order = append(order, k)
		}
		target.ReadOnly = target.ReadOnly || row.ReadOnly
		target.ProfileMountIDs = append(target.ProfileMountIDs, row.ID)
		if !slices.Contains(target.ProfileNames, row.ProfileName) {
			target.ProfileNames = append(target.ProfileNames, row.ProfileName)
		}
	}
	out := make([]SandboxProfileMount, 0, len(order))
	for _, k := range order {
		target := grouped[k]
		optedOut := true
		for _, id := range target.ProfileMountIDs {
			if !optOuts[store.ProfileOptOutKey(store.OptOutMount, id)] {
				optedOut = false
				break
			}
		}
		target.OptedOut = optedOut
		target.Attached = mountPresent(live, target.HostPath, target.TargetPath)
		sort.Strings(target.ProfileNames)
		out = append(out, *target)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HostPath != out[j].HostPath {
			return out[i].HostPath < out[j].HostPath
		}
		return out[i].TargetPath < out[j].TargetPath
	})
	return out, nil
}

// syncProfileMounts applies the desired profile mounts of a sandbox and
// releases the stale candidates nothing wants anymore. Stopped sandboxes are
// left alone; the next start converges them.
func (a *App) syncProfileMounts(ctx context.Context, sandbox string, stale []store.ProfileMount) (int, []string) {
	info, err := a.Sbx.InspectSandbox(ctx, sandbox)
	if err != nil || !info.Running() {
		return 0, nil
	}
	targets, err := a.profileMountsForSandbox(ctx, sandbox, nil)
	if err != nil {
		return 0, []string{err.Error()}
	}
	live, err := a.Sbx.Mounts(ctx, sandbox)
	if err != nil {
		return 0, []string{err.Error()}
	}
	for i := range targets {
		targets[i].Attached = mountPresent(live, targets[i].HostPath, targets[i].TargetPath)
	}
	applied := 0
	var errs []string
	for _, target := range targets {
		if target.OptedOut || target.Attached {
			continue
		}
		if err := a.mountProfileTarget(ctx, sandbox, target); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.HostPath, err))
			continue
		}
		applied++
	}
	for _, m := range stale {
		if targetStillDesired(targets, m.HostPath, m.TargetPath) || !mountPresent(live, m.HostPath, m.TargetPath) {
			continue
		}
		if err := a.Sbx.UnmountFolderAt(ctx, sandbox, m.HostPath, m.TargetPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.HostPath, err))
		}
	}
	if applied > 0 {
		a.Notify(TopicSandbox(sandbox))
	}
	return applied, errs
}

// mountProfileTarget prepares the target directory and bind-mounts a profile
// target into a running sandbox.
func (a *App) mountProfileTarget(ctx context.Context, sandbox string, target SandboxProfileMount) error {
	if target.TargetPath != "" {
		_ = a.Sbx.MkdirAll(ctx, sandbox, target.TargetPath)
	}
	return a.Sbx.MountFolderAt(ctx, sandbox, target.HostPath, target.TargetPath, target.ReadOnly)
}

// releaseStaleCaches unmounts the candidates that are no longer desired by the
// sandbox, either directly or through another profile.
func (a *App) releaseStaleCaches(ctx context.Context, sandbox string, candidates []store.CacheMount) {
	if len(candidates) == 0 {
		return
	}
	info, err := a.Sbx.InspectSandbox(ctx, sandbox)
	if err != nil || !info.Running() {
		return
	}
	desired, err := a.desiredCaches(ctx, sandbox)
	if err != nil {
		return
	}
	live, err := a.Sbx.Mounts(ctx, sandbox)
	if err != nil {
		return
	}
	for _, c := range candidates {
		if cacheIn(desired, c.ID) || !cacheMounted(live, c) {
			continue
		}
		_ = a.Sbx.UnmountFolderAt(ctx, sandbox, c.HostPath, c.TargetPath)
	}
}

// syncProfileSandboxes propagates a profile-default change to the sandboxes
// that have the profile: desired mounts and caches are applied, stale
// candidates are released when nothing wants them anymore.
func (a *App) syncProfileSandboxes(ctx context.Context, profileID int64, staleMounts []store.ProfileMount, staleCaches []store.CacheMount) {
	sandboxes, err := a.Store.SandboxesForProfile(ctx, profileID)
	if err != nil {
		return
	}
	for _, sandbox := range sandboxes {
		a.ReapplyCaches(ctx, sandbox)
		a.syncProfileMounts(ctx, sandbox, staleMounts)
		a.releaseStaleCaches(ctx, sandbox, staleCaches)
	}
}

// profileMountByID finds one declared mount of a sandbox across its profiles.
func (a *App) profileMountByID(ctx context.Context, sandbox string, mountID int64) (store.ProfileMount, bool) {
	declared, err := a.Store.ProfileMountsForSandbox(ctx, sandbox)
	if err != nil {
		return store.ProfileMount{}, false
	}
	for _, row := range declared {
		if row.ID == mountID {
			return row.ProfileMount, true
		}
	}
	return store.ProfileMount{}, false
}

// mountTarget returns the effective container path of a mount target.
func mountTarget(hostPath, targetPath string) string {
	if targetPath != "" {
		return targetPath
	}
	return hostPath
}

// mountPresent reports whether a bind mount is live in the sandbox.
func mountPresent(mounts []sbx.MountInfo, hostPath, targetPath string) bool {
	want := mountTarget(hostPath, targetPath)
	for _, m := range mounts {
		if m.HostPath != hostPath {
			continue
		}
		if mountTarget(m.HostPath, m.Target) == want {
			return true
		}
	}
	return false
}

// targetStillDesired reports whether a non-opted-out target still covers the
// host/target pair.
func targetStillDesired(targets []SandboxProfileMount, hostPath, targetPath string) bool {
	for _, t := range targets {
		if t.OptedOut {
			continue
		}
		if t.HostPath == hostPath && mountTarget(t.HostPath, t.TargetPath) == mountTarget(hostPath, targetPath) {
			return true
		}
	}
	return false
}

// cacheIn reports whether a cache id is part of a desired set.
func cacheIn(caches []store.CacheMount, id int64) bool {
	for _, c := range caches {
		if c.ID == id {
			return true
		}
	}
	return false
}
