package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/store"
)

// ListCaches returns every configured shared cache.
func (a *App) ListCaches(ctx context.Context) ([]store.CacheMount, error) {
	return a.Store.ListCacheMounts(ctx)
}

// GetCache returns one configured cache.
func (a *App) GetCache(ctx context.Context, id int64) (store.CacheMount, error) {
	return a.Store.GetCacheMount(ctx, id)
}

// CreateCache validates and stores a cache definition.
func (a *App) CreateCache(ctx context.Context, c store.CacheMount) (store.CacheMount, error) {
	normalized, err := validateCache(c)
	if err != nil {
		return store.CacheMount{}, err
	}
	out, err := a.Store.CreateCacheMount(ctx, normalized)
	if err != nil {
		return store.CacheMount{}, err
	}
	a.Notify(TopicCaches)
	return out, nil
}

// UpdateCache rewrites a cache definition and refreshes viewers of every
// sandbox it is assigned to.
func (a *App) UpdateCache(ctx context.Context, c store.CacheMount) (store.CacheMount, error) {
	normalized, err := validateCache(c)
	if err != nil {
		return store.CacheMount{}, err
	}
	if err := a.Store.UpdateCacheMount(ctx, normalized); err != nil {
		return store.CacheMount{}, err
	}
	a.Notify(TopicCaches)
	a.notifyCacheSandboxes(ctx, normalized.ID)
	return a.Store.GetCacheMount(ctx, normalized.ID)
}

// DeleteCache removes a cache definition. Live bind mounts are left in place;
// they are dropped on the next sandbox restart or from the Caches tab.
func (a *App) DeleteCache(ctx context.Context, id int64) error {
	assignments, err := a.Store.AllCacheAssignments(ctx)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteCacheMount(ctx, id); err != nil {
		return err
	}
	a.Notify(TopicCaches)
	for _, name := range assignments[id] {
		a.Notify(TopicSandbox(name))
	}
	return nil
}

// AssignCache marks a cache as desired for a sandbox and mounts it when the
// sandbox is running.
func (a *App) AssignCache(ctx context.Context, sandbox string, cacheID int64) error {
	c, err := a.Store.GetCacheMount(ctx, cacheID)
	if err != nil {
		return err
	}
	if !c.Enabled {
		return errors.New("this cache is disabled")
	}
	if err := a.mountCache(ctx, sandbox, c); err != nil {
		return err
	}
	if err := a.Store.AssignCache(ctx, sandbox, cacheID); err != nil {
		return err
	}
	a.Notify(TopicCaches)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// UnassignCache drops the desired state and unmounts the bind when possible.
func (a *App) UnassignCache(ctx context.Context, sandbox string, cacheID int64) error {
	c, err := a.Store.GetCacheMount(ctx, cacheID)
	if err != nil {
		return err
	}
	if err := a.Store.UnassignCache(ctx, sandbox, cacheID); err != nil {
		return err
	}
	if info, err := a.Sbx.InspectSandbox(ctx, sandbox); err == nil && info.Running() {
		_ = a.Sbx.UnmountFolderAt(ctx, sandbox, c.HostPath, c.TargetPath)
	}
	a.Notify(TopicCaches)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// ReapplyCaches mounts every desired cache that is missing from a running
// sandbox. Errors are per-cache and never abort the whole pass.
func (a *App) ReapplyCaches(ctx context.Context, sandbox string) (int, []string) {
	caches, err := a.desiredCaches(ctx, sandbox)
	if err != nil {
		return 0, []string{err.Error()}
	}
	if len(caches) == 0 {
		return 0, nil
	}
	mounts, err := a.Sbx.Mounts(ctx, sandbox)
	if err != nil {
		return 0, []string{err.Error()}
	}
	applied := 0
	var errs []string
	for _, c := range caches {
		if !c.Enabled || cacheMounted(mounts, c) {
			continue
		}
		if err := a.mountCache(ctx, sandbox, c); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.Name, err))
			continue
		}
		applied++
	}
	if applied > 0 {
		a.Notify(TopicSandbox(sandbox))
	}
	return applied, errs
}

// mountCache prepares the target directory and bind-mounts the cache into a
// running sandbox.
func (a *App) mountCache(ctx context.Context, sandbox string, c store.CacheMount) error {
	info, err := a.Sbx.InspectSandbox(ctx, sandbox)
	if err != nil {
		return err
	}
	if !info.Running() {
		return fmt.Errorf("sandbox %q is not running; start it before attaching caches", sandbox)
	}
	_ = a.Sbx.MkdirAll(ctx, sandbox, c.TargetPath)
	return a.Sbx.MountFolderAt(ctx, sandbox, c.HostPath, c.TargetPath, c.ReadOnly)
}

// reapplyCachesWhenReady waits for a freshly started sandbox and re-mounts its
// desired caches, retrying briefly while the VM comes up.
func (a *App) reapplyCachesWhenReady(ctx context.Context, name string) {
	caches, err := a.desiredCaches(ctx, name)
	if err != nil {
		return
	}
	mounts, err := a.Store.ProfileMountsForSandbox(ctx, name)
	if err != nil {
		return
	}
	if len(caches) == 0 && len(mounts) == 0 {
		return
	}
	for attempt := 0; attempt < 8; attempt++ {
		if ctx.Err() != nil {
			return
		}
		info, err := a.Sbx.InspectSandbox(ctx, name)
		if err == nil && info.Running() {
			_, errs := a.ReapplyCaches(ctx, name)
			a.syncProfileMounts(ctx, name, nil)
			if len(errs) == 0 {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

// attachAutoCaches mounts every auto-attach cache into a newly created sandbox
// and records the desired state. Failures are reported to the job output.
func (a *App) attachAutoCaches(ctx context.Context, sandbox string, w io.Writer) {
	caches, err := a.Store.AutoAttachCaches(ctx)
	if err != nil {
		return
	}
	for _, c := range caches {
		if err := a.mountCache(ctx, sandbox, c); err != nil {
			fmt.Fprintf(w, "cache %s: %v\n", c.Name, err)
			continue
		}
		if err := a.Store.AssignCache(ctx, sandbox, c.ID); err != nil {
			fmt.Fprintf(w, "cache %s: %v\n", c.Name, err)
			continue
		}
		fmt.Fprintf(w, "cache %s: mounted %s at %s\n", c.Name, c.HostPath, c.TargetPath)
	}
	a.Notify(TopicCaches)
	a.Notify(TopicSandbox(sandbox))
}

func (a *App) sandboxNameSet(ctx context.Context) map[string]bool {
	set := make(map[string]bool)
	if sandboxes, err := a.Sbx.ListSandboxes(ctx); err == nil {
		for _, s := range sandboxes {
			set[s.Name] = true
		}
	}
	return set
}

// findNewSandbox returns the name of the sandbox that appeared after creation.
func (a *App) findNewSandbox(ctx context.Context, before map[string]bool) string {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return ""
	}
	newest := ""
	var newestAt time.Time
	for _, s := range sandboxes {
		if before[s.Name] {
			continue
		}
		if s.CreatedAt == nil {
			if newest == "" {
				newest = s.Name
			}
			continue
		}
		if newest == "" || s.CreatedAt.After(newestAt) {
			newestAt = *s.CreatedAt
			newest = s.Name
		}
	}
	return newest
}

func (a *App) notifyCacheSandboxes(ctx context.Context, cacheID int64) {
	assignments, err := a.Store.AllCacheAssignments(ctx)
	if err != nil {
		return
	}
	for _, name := range assignments[cacheID] {
		a.Notify(TopicSandbox(name))
	}
}

func validateCache(c store.CacheMount) (store.CacheMount, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.HostPath = expandHome(strings.TrimSpace(c.HostPath))
	c.TargetPath = strings.TrimSpace(c.TargetPath)
	if c.Name == "" {
		return store.CacheMount{}, errors.New("cache name is required")
	}
	if !filepath.IsAbs(c.HostPath) {
		return store.CacheMount{}, errors.New("host path must be absolute")
	}
	if !filepath.IsAbs(c.TargetPath) {
		return store.CacheMount{}, errors.New("sandbox path must be absolute")
	}
	return c, nil
}

// expandHome resolves a leading ~ with the host user's home directory, so the
// UI can offer presets like ~/.npm.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
}
