package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ProfileMountTarget is one merged mount target of a sandbox, with the
// profiles that declare it.
type ProfileMountTarget struct {
	HostPath     string   `json:"host_path"`
	TargetPath   string   `json:"target_path"`
	ReadOnly     bool     `json:"read_only"`
	ProfileNames []string `json:"profile_names"`
}

// SandboxProfileMount is a profile mount as seen from one sandbox: the merged
// target plus its opt-out and live attachment state.
type SandboxProfileMount struct {
	ProfileMountTarget
	OptedOut bool `json:"opted_out"`
	Attached bool `json:"attached"`
}

// AddProfileMount declares a mount on a profile and converges its sandboxes.
func (a *App) AddProfileMount(ctx context.Context, profileSlug, hostPath, targetPath string, readOnly bool) (fleet.MountRef, error) {
	hostPath = fleet.ExpandHome(hostPath)
	if hostPath == "" {
		return fleet.MountRef{}, errors.New("host path is required")
	}
	if !filepath.IsAbs(hostPath) {
		return fleet.MountRef{}, errors.New("host path must be absolute")
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath != "" && !filepath.IsAbs(targetPath) {
		return fleet.MountRef{}, errors.New("target path must be absolute")
	}
	lock, err := a.lockFleet()
	if err != nil {
		return fleet.MountRef{}, err
	}
	defer func() { _ = lock.Release() }()
	p, ok := a.Fleet.Profile(profileSlug)
	if !ok {
		return fleet.MountRef{}, fmt.Errorf("profile %q not found", profileSlug)
	}
	mount := fleet.MountRef{HostPath: hostPath, TargetPath: targetPath, ReadOnly: readOnly}
	replaced := false
	for i, m := range p.App.Mounts {
		if m.Key() == mount.Key() {
			p.App.Mounts[i] = mount
			replaced = true
			break
		}
	}
	if !replaced {
		p.App.Mounts = append(p.App.Mounts, mount)
	}
	if err := a.Fleet.SaveProfile(p); err != nil {
		return fleet.MountRef{}, err
	}
	a.reapplyProfileToSandboxes(ctx, p.Slug)
	a.Notify(TopicProfiles)
	a.Notify(TopicSandboxes)
	return mount, nil
}

// RemoveProfileMount drops a mount from a profile, detaches it from the
// sandboxes that had it attached, and forgets its opt-outs.
func (a *App) RemoveProfileMount(ctx context.Context, profileSlug, hostPath, targetPath string) error {
	return a.withFleetLock(func() error {
		p, ok := a.Fleet.Profile(profileSlug)
		if !ok {
			return fmt.Errorf("profile %q not found", profileSlug)
		}
		removed := fleet.MountRef{HostPath: hostPath, TargetPath: targetPath}
		kept := p.App.Mounts[:0]
		for _, m := range p.App.Mounts {
			if m.Key() == removed.Key() {
				continue
			}
			kept = append(kept, m)
		}
		p.App.Mounts = kept
		if err := a.Fleet.SaveProfile(p); err != nil {
			return err
		}
		for _, s := range a.sandboxesReferencingProfile(profileSlug) {
			stale := []fleet.MountRef{removed}
			if _, errs := a.syncProfileMounts(ctx, s.Name(), stale); len(errs) > 0 {
				a.Notify(TopicSandbox(s.Name()))
			}
		}
		a.Notify(TopicProfiles)
		return nil
	})
}

// AddProfileCache adds a shared cache to a profile and converges its
// sandboxes.
func (a *App) AddProfileCache(ctx context.Context, profileSlug, cacheSlug string) error {
	return a.withFleetLock(func() error {
		p, ok := a.Fleet.Profile(profileSlug)
		if !ok {
			return fmt.Errorf("profile %q not found", profileSlug)
		}
		if _, ok := a.Fleet.Cache(cacheSlug); !ok {
			return fmt.Errorf("cache %q not found", cacheSlug)
		}
		if slices.Contains(p.App.Caches, cacheSlug) {
			return nil
		}
		p.App.Caches = append(p.App.Caches, cacheSlug)
		if err := a.Fleet.SaveProfile(p); err != nil {
			return err
		}
		a.reapplyProfileToSandboxes(ctx, p.Slug)
		a.Notify(TopicProfiles)
		return nil
	})
}

// RemoveProfileCache drops a cache from a profile and detaches it from the
// sandboxes that had it attached.
func (a *App) RemoveProfileCache(ctx context.Context, profileSlug, cacheSlug string) error {
	return a.withFleetLock(func() error {
		p, ok := a.Fleet.Profile(profileSlug)
		if !ok {
			return fmt.Errorf("profile %q not found", profileSlug)
		}
		kept := p.App.Caches[:0]
		for _, slug := range p.App.Caches {
			if slug == cacheSlug {
				continue
			}
			kept = append(kept, slug)
		}
		p.App.Caches = kept
		if err := a.Fleet.SaveProfile(p); err != nil {
			return err
		}
		a.reapplyProfileToSandboxes(ctx, p.Slug)
		a.Notify(TopicProfiles)
		return nil
	})
}

// DetachProfileMount opts a sandbox out of one profile mount and unmounts it.
func (a *App) DetachProfileMount(ctx context.Context, name, hostPath, targetPath string) error {
	return a.withFleetLock(func() error {
		if err := a.updateSandboxOptOut(ctx, name, func(o *fleet.SandboxOptOut) {
			key := fleet.MountRef{HostPath: hostPath, TargetPath: targetPath}.Key()
			if !slices.Contains(o.Mounts, key) {
				o.Mounts = append(o.Mounts, key)
			}
		}); err != nil {
			return err
		}
		if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
			if err := a.Sbx.UnmountFolderAt(ctx, name, hostPath, targetPath); err != nil {
				return err
			}
		}
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// ApplyProfileMount clears a sandbox opt-out and attaches the mount at once.
func (a *App) ApplyProfileMount(ctx context.Context, name, hostPath, targetPath string) error {
	return a.withFleetLock(func() error {
		if err := a.updateSandboxOptOut(ctx, name, func(o *fleet.SandboxOptOut) {
			key := fleet.MountRef{HostPath: hostPath, TargetPath: targetPath}.Key()
			kept := o.Mounts[:0]
			for _, k := range o.Mounts {
				if k != key {
					kept = append(kept, k)
				}
			}
			o.Mounts = kept
		}); err != nil {
			return err
		}
		if _, errs := a.syncProfileMounts(ctx, name, nil); len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// DetachProfileCache opts a sandbox out of one profile cache and unmounts it.
func (a *App) DetachProfileCache(ctx context.Context, name, cacheSlug string) error {
	return a.withFleetLock(func() error {
		if err := a.updateSandboxOptOut(ctx, name, func(o *fleet.SandboxOptOut) {
			if !slices.Contains(o.Caches, cacheSlug) {
				o.Caches = append(o.Caches, cacheSlug)
			}
		}); err != nil {
			return err
		}
		if c, ok := a.Fleet.Cache(cacheSlug); ok {
			if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
				if mounts, err := a.Sbx.Mounts(ctx, name); err == nil && mountPresent(mounts, c.App.HostPath, c.App.TargetPath) {
					if err := a.Sbx.UnmountFolderAt(ctx, name, c.App.HostPath, c.App.TargetPath); err != nil {
						return err
					}
				}
			}
		}
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// ApplyProfileCache clears a sandbox opt-out and attaches the cache at once.
func (a *App) ApplyProfileCache(ctx context.Context, name, cacheSlug string) error {
	return a.withFleetLock(func() error {
		if err := a.updateSandboxOptOut(ctx, name, func(o *fleet.SandboxOptOut) {
			kept := o.Caches[:0]
			for _, slug := range o.Caches {
				if slug != cacheSlug {
					kept = append(kept, slug)
				}
			}
			o.Caches = kept
		}); err != nil {
			return err
		}
		if c, ok := a.Fleet.Cache(cacheSlug); ok {
			if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
				if err := a.mountCache(ctx, name, *c); err != nil {
					return err
				}
			}
		}
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// profileMountsForSandbox merges the mounts every referenced profile
// declares, applying the sandbox's opt-outs.
func (a *App) profileMountsForSandbox(ctx context.Context, name string, live []sbx.MountInfo) ([]SandboxProfileMount, error) {
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return nil, nil
	}
	optedOut := map[string]bool{}
	if s.App.OptOuts != nil {
		for _, key := range s.App.OptOuts.Mounts {
			optedOut[key] = true
		}
	}
	type key struct{ host, target string }
	grouped := map[key]*SandboxProfileMount{}
	var order []key
	for _, p := range a.profilesForSandbox(s) {
		for _, m := range p.App.Mounts {
			k := key{m.HostPath, m.TargetPath}
			target, ok := grouped[k]
			if !ok {
				target = &SandboxProfileMount{ProfileMountTarget: ProfileMountTarget{
					HostPath:   m.HostPath,
					TargetPath: m.TargetPath,
					ReadOnly:   m.ReadOnly,
				}}
				grouped[k] = target
				order = append(order, k)
			}
			target.ReadOnly = target.ReadOnly || m.ReadOnly
			if label := p.Label(); !slices.Contains(target.ProfileNames, label) {
				target.ProfileNames = append(target.ProfileNames, label)
			}
		}
	}
	out := make([]SandboxProfileMount, 0, len(order))
	for _, k := range order {
		target := grouped[k]
		target.OptedOut = optedOut[fleet.MountRef{HostPath: target.HostPath, TargetPath: target.TargetPath}.Key()]
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

// syncProfileMounts attaches every desired profile mount that is missing and
// detaches the stale candidates the caller knows are gone. It reads the fleet
// and the daemon only; callers hold the fleet lock when it is part of a
// mutation.
func (a *App) syncProfileMounts(ctx context.Context, name string, stale []fleet.MountRef) (int, []string) {
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil || !info.Running() {
		return 0, nil
	}
	targets, err := a.profileMountsForSandbox(ctx, name, nil)
	if err != nil {
		return 0, []string{err.Error()}
	}
	live, err := a.Sbx.Mounts(ctx, name)
	if err != nil {
		return 0, []string{err.Error()}
	}
	applied := 0
	var errs []string
	for i := range targets {
		targets[i].Attached = mountPresent(live, targets[i].HostPath, targets[i].TargetPath)
		if targets[i].OptedOut || targets[i].Attached {
			continue
		}
		if err := a.mountRef(ctx, name, fleet.MountRef{HostPath: targets[i].HostPath, TargetPath: targets[i].TargetPath, ReadOnly: targets[i].ReadOnly}); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", targets[i].HostPath, err))
			continue
		}
		applied++
	}
	for _, m := range stale {
		if targetStillDesired(targets, m.HostPath, m.TargetPath) || !mountPresent(live, m.HostPath, m.TargetPath) {
			continue
		}
		if err := a.Sbx.UnmountFolderAt(ctx, name, m.HostPath, m.TargetPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.HostPath, err))
		}
	}
	if applied > 0 {
		a.Notify(TopicSandbox(name))
	}
	return applied, errs
}

// syncDirectMounts attaches the sandbox's own declared mounts. The caller
// holds the fleet lock.
func (a *App) syncDirectMounts(ctx context.Context, name string) (int, []string) {
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return 0, nil
	}
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil || !info.Running() {
		return 0, nil
	}
	live, err := a.Sbx.Mounts(ctx, name)
	if err != nil {
		return 0, []string{err.Error()}
	}
	applied := 0
	var errs []string
	for _, m := range s.App.Mounts {
		if mountPresent(live, m.HostPath, m.TargetPath) {
			continue
		}
		if err := a.mountRef(ctx, name, m); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.HostPath, err))
			continue
		}
		applied++
	}
	if applied > 0 {
		a.Notify(TopicSandbox(name))
	}
	return applied, errs
}

// mountRef binds a host path into a running sandbox, creating the target.
func (a *App) mountRef(ctx context.Context, name string, m fleet.MountRef) error {
	if m.EffectiveTarget() != "" {
		_ = a.Sbx.MkdirAll(ctx, name, m.EffectiveTarget())
	}
	return a.Sbx.MountFolderAt(ctx, name, m.HostPath, m.EffectiveTarget(), m.ReadOnly)
}

// reapplyProfileToSandboxes converges every sandbox referencing a profile.
// The caller holds the fleet lock.
func (a *App) reapplyProfileToSandboxes(ctx context.Context, profileSlug string) {
	for _, s := range a.sandboxesReferencingProfile(profileSlug) {
		if _, errs := a.syncProfileMounts(ctx, s.Name(), nil); len(errs) > 0 {
			a.Notify(TopicSandbox(s.Name()))
		}
		a.ReapplyCaches(ctx, s.Name())
	}
}

func (a *App) sandboxesReferencingProfile(profileSlug string) []*fleet.Sandbox {
	out := []*fleet.Sandbox{}
	for _, s := range a.Fleet.Sandboxes() {
		if slices.Contains(s.App.Profiles, profileSlug) {
			out = append(out, s)
		}
	}
	return out
}

// updateSandboxOptOut mutates a sandbox's opt-out block and saves the file.
// The caller holds the fleet lock.
func (a *App) updateSandboxOptOut(ctx context.Context, name string, mutate func(*fleet.SandboxOptOut)) error {
	s, err := a.ensureSandboxConfig(ctx, name)
	if err != nil {
		return err
	}
	if s.App.OptOuts == nil {
		s.App.OptOuts = &fleet.SandboxOptOut{}
	}
	mutate(s.App.OptOuts)
	if len(s.App.OptOuts.Mounts) == 0 && len(s.App.OptOuts.Caches) == 0 {
		s.App.OptOuts = nil
	}
	return a.Fleet.SaveSandbox(s)
}

// mountPresent reports whether a host path is attached at its target.
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

// targetStillDesired reports whether merged targets still want a mount.
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

// mountTarget normalizes an empty target to the host path.
func mountTarget(hostPath, targetPath string) string {
	if strings.TrimSpace(targetPath) == "" {
		return hostPath
	}
	return targetPath
}
