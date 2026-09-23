package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

// CacheInput is the editable definition of a shared cache.
type CacheInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HostPath    string `json:"host_path"`
	TargetPath  string `json:"target_path"`
	ReadOnly    bool   `json:"read_only"`
	AutoAttach  bool   `json:"auto_attach"`
	Enabled     bool   `json:"enabled"`
}

// ListCaches returns every shared cache definition.
func (a *App) ListCaches(ctx context.Context) ([]fleet.Cache, error) {
	caches := a.Fleet.Caches()
	out := make([]fleet.Cache, 0, len(caches))
	for _, c := range caches {
		out = append(out, *c)
	}
	return out, nil
}

// GetCache returns one cache by slug.
func (a *App) GetCache(ctx context.Context, slug string) (fleet.Cache, error) {
	c, ok := a.Fleet.Cache(slug)
	if !ok {
		return fleet.Cache{}, store.ErrNotFound
	}
	return *c, nil
}

// CreateCache writes a new cache definition.
func (a *App) CreateCache(ctx context.Context, input CacheInput) (fleet.Cache, error) {
	if err := validateCacheInput(input); err != nil {
		return fleet.Cache{}, err
	}
	var created fleet.Cache
	err := a.withFleetLock(func() error {
		auto, enabled := input.AutoAttach, input.Enabled
		c, err := a.Fleet.CreateCache(input.Name, fleet.CacheApp{
			Name:        input.Name,
			Description: input.Description,
			HostPath:    input.HostPath,
			TargetPath:  input.TargetPath,
			ReadOnly:    input.ReadOnly,
			AutoAttach:  &auto,
			Enabled:     &enabled,
		})
		if err != nil {
			return err
		}
		created = *c
		return nil
	})
	if err != nil {
		return fleet.Cache{}, err
	}
	a.Notify(TopicCaches)
	return created, nil
}

// UpdateCache rewrites a cache definition and re-applies it where attached.
func (a *App) UpdateCache(ctx context.Context, slug string, input CacheInput) (fleet.Cache, error) {
	if err := validateCacheInput(input); err != nil {
		return fleet.Cache{}, err
	}
	var updated fleet.Cache
	err := a.withFleetLock(func() error {
		c, ok := a.Fleet.Cache(slug)
		if !ok {
			return store.ErrNotFound
		}
		auto, enabled := input.AutoAttach, input.Enabled
		previous := c.App
		c.App = fleet.CacheApp{
			Name:        input.Name,
			Description: input.Description,
			HostPath:    input.HostPath,
			TargetPath:  input.TargetPath,
			ReadOnly:    input.ReadOnly,
			AutoAttach:  &auto,
			Enabled:     &enabled,
		}
		if err := a.Fleet.SaveCache(c); err != nil {
			return err
		}
		for _, s := range a.sandboxesWithCache(slug) {
			if pathChanged(previous, c.App) {
				if info, err := a.Sbx.InspectSandbox(ctx, s.Name()); err == nil && info.Running() {
					_ = a.Sbx.UnmountFolderAt(ctx, s.Name(), previous.HostPath, previous.EffectiveTarget())
				}
			}
			a.ReapplyCaches(ctx, s.Name())
		}
		updated = *c
		return nil
	})
	if err != nil {
		return fleet.Cache{}, err
	}
	a.Notify(TopicCaches)
	return updated, nil
}

// DeleteCache removes a cache definition and detaches it everywhere.
func (a *App) DeleteCache(ctx context.Context, slug string) error {
	return a.withFleetLock(func() error {
		c, ok := a.Fleet.Cache(slug)
		if !ok {
			return store.ErrNotFound
		}
		for _, s := range a.sandboxesWithCache(slug) {
			if info, err := a.Sbx.InspectSandbox(ctx, s.Name()); err == nil && info.Running() {
				if mounts, err := a.Sbx.Mounts(ctx, s.Name()); err == nil && mountPresent(mounts, c.App.HostPath, c.App.TargetPath) {
					_ = a.Sbx.UnmountFolderAt(ctx, s.Name(), c.App.HostPath, c.App.TargetPath)
				}
			}
			s.App.Caches = removeString(s.App.Caches, slug)
			if s.App.OptOuts != nil {
				s.App.OptOuts.Caches = removeString(s.App.OptOuts.Caches, slug)
			}
			_ = a.Fleet.SaveSandbox(s)
		}
		if err := a.Fleet.DeleteCache(slug); err != nil {
			return err
		}
		a.Notify(TopicCaches)
		return nil
	})
}

// AssignCache declares a direct cache attachment on a sandbox and mounts it
// immediately when the sandbox runs.
func (a *App) AssignCache(ctx context.Context, name, cacheSlug string) error {
	return a.withFleetLock(func() error {
		s, err := a.ensureSandboxConfig(ctx, name)
		if err != nil {
			return err
		}
		c, ok := a.Fleet.Cache(cacheSlug)
		if !ok {
			return fmt.Errorf("cache %q not found", cacheSlug)
		}
		if !slices.Contains(s.App.Caches, cacheSlug) {
			s.App.Caches = append(s.App.Caches, cacheSlug)
		}
		if s.App.OptOuts != nil {
			s.App.OptOuts.Caches = removeString(s.App.OptOuts.Caches, cacheSlug)
		}
		if err := a.Fleet.SaveSandbox(s); err != nil {
			return err
		}
		if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
			if err := a.mountCache(ctx, name, *c); err != nil {
				return err
			}
		}
		a.Notify(TopicCaches)
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// UnassignCache revokes a direct cache attachment and unmounts it.
func (a *App) UnassignCache(ctx context.Context, name, cacheSlug string) error {
	return a.withFleetLock(func() error {
		s, err := a.ensureSandboxConfig(ctx, name)
		if err != nil {
			return err
		}
		c, ok := a.Fleet.Cache(cacheSlug)
		if !ok {
			return fmt.Errorf("cache %q not found", cacheSlug)
		}
		s.App.Caches = removeString(s.App.Caches, cacheSlug)
		if err := a.Fleet.SaveSandbox(s); err != nil {
			return err
		}
		if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
			if mounts, err := a.Sbx.Mounts(ctx, name); err == nil && mountPresent(mounts, c.App.HostPath, c.App.TargetPath) {
				if err := a.Sbx.UnmountFolderAt(ctx, name, c.App.HostPath, c.App.TargetPath); err != nil {
					return err
				}
			}
		}
		a.Notify(TopicCaches)
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// ReapplyCaches mounts every desired cache that is missing from a running
// sandbox. Errors are per-cache and never abort the whole pass. It reads the
// fleet and the daemon only: callers hold the fleet lock when it is part of a
// mutation, and read-only callers take no lock.
func (a *App) ReapplyCaches(ctx context.Context, name string) (int, []string) {
	caches, err := a.desiredCaches(ctx, name)
	if err != nil {
		return 0, []string{err.Error()}
	}
	if len(caches) == 0 {
		return 0, nil
	}
	mounts, err := a.Sbx.Mounts(ctx, name)
	if err != nil {
		return 0, []string{err.Error()}
	}
	applied := 0
	var errs []string
	for _, c := range caches {
		current, err := a.releaseDriftedMount(ctx, name, mounts, c.App.HostPath, c.App.EffectiveTarget(), c.App.ReadOnly)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.App.Name, err))
			continue
		}
		if current {
			continue
		}
		if err := a.mountCache(ctx, name, c); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.App.Name, err))
			continue
		}
		applied++
	}
	if applied > 0 {
		a.Notify(TopicSandbox(name))
	}
	return applied, errs
}

// desiredCaches merges the caches declared directly by the sandbox with those
// its profiles provide, minus its opt-outs.
func (a *App) desiredCaches(ctx context.Context, name string) ([]fleet.Cache, error) {
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return nil, fmt.Errorf("no configuration found for sandbox %q", name)
	}
	optedOut := map[string]bool{}
	if s.App.OptOuts != nil {
		for _, slug := range s.App.OptOuts.Caches {
			optedOut[slug] = true
		}
	}
	merged := map[string]fleet.Cache{}
	for _, p := range a.profilesForSandbox(s) {
		for _, slug := range p.App.Caches {
			if optedOut[slug] {
				continue
			}
			if c, ok := a.Fleet.Cache(slug); ok && c.App.IsEnabled() {
				merged[slug] = *c
			}
		}
	}
	for _, slug := range s.App.Caches {
		if c, ok := a.Fleet.Cache(slug); ok && c.App.IsEnabled() {
			merged[slug] = *c
		}
	}
	out := make([]fleet.Cache, 0, len(merged))
	for _, c := range merged {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].App.Name < out[j].App.Name })
	return out, nil
}

// mountCache binds a cache host path into a running sandbox.
func (a *App) mountCache(ctx context.Context, name string, c fleet.Cache) error {
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil {
		return err
	}
	if !info.Running() {
		return fmt.Errorf("sandbox %q is not running; start it before attaching caches", name)
	}
	target := c.App.TargetPath
	if strings.TrimSpace(target) == "" {
		target = c.App.HostPath
	}
	_ = a.Sbx.MkdirAll(ctx, name, target)
	return a.Sbx.MountFolderAt(ctx, name, c.App.HostPath, target, c.App.ReadOnly)
}

// attachAutoCaches adds every auto-attach cache to a fresh sandbox and mounts
// it; used after create when the config already carries them. The caller holds
// the fleet lock.
func (a *App) attachAutoCaches(ctx context.Context, name string, w io.Writer) {
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return
	}
	changed := false
	for _, c := range a.Fleet.Caches() {
		if !c.App.AutoAttaches() || !c.App.IsEnabled() || slices.Contains(s.App.Caches, c.Slug) {
			continue
		}
		s.App.Caches = append(s.App.Caches, c.Slug)
		changed = true
	}
	if changed {
		if err := a.Fleet.SaveSandbox(s); err != nil {
			fmt.Fprintf(w, "caches: %v\n", err)
			return
		}
	}
	if applied, errs := a.ReapplyCaches(ctx, name); applied > 0 || len(errs) > 0 {
		fmt.Fprintf(w, "caches: %d attached\n", applied)
		for _, err := range errs {
			fmt.Fprintf(w, "cache error: %s\n", err)
		}
	}
	a.Notify(TopicCaches)
	a.Notify(TopicSandbox(name))
}

// sandboxesWithCache lists every sandbox referencing a cache, directly or
// through a profile.
func (a *App) sandboxesWithCache(cacheSlug string) []*fleet.Sandbox {
	out := []*fleet.Sandbox{}
	for _, s := range a.Fleet.Sandboxes() {
		if slices.Contains(s.App.Caches, cacheSlug) {
			out = append(out, s)
			continue
		}
		for _, slug := range s.App.Profiles {
			if p, ok := a.Fleet.Profile(slug); ok && slices.Contains(p.App.Caches, cacheSlug) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

func cacheMounted(mounts []sbx.MountInfo, c fleet.Cache) bool {
	target := c.App.TargetPath
	if strings.TrimSpace(target) == "" {
		target = c.App.HostPath
	}
	return mountPresent(mounts, c.App.HostPath, target)
}

func validateCacheInput(input CacheInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("cache name is required")
	}
	if !filepath.IsAbs(fleet.ExpandHome(input.HostPath)) {
		return errors.New("host path must be absolute")
	}
	if target := strings.TrimSpace(input.TargetPath); target != "" && !filepath.IsAbs(target) {
		return errors.New("target path must be absolute")
	}
	return nil
}

func pathChanged(before, after fleet.CacheApp) bool {
	return before.HostPath != after.HostPath || before.EffectiveTarget() != after.EffectiveTarget() || before.ReadOnly != after.ReadOnly
}

func removeString(in []string, want string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != want {
			out = append(out, v)
		}
	}
	return out
}
