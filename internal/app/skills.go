package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/skills"
	"github.com/JLugagne/sandwarden/internal/store"
)

const (
	sandboxAgentsDir   = "/home/agent/.agents"
	sandboxSkillsDir   = sandboxAgentsDir + "/skills"
	sandboxCommandsDir = sandboxAgentsDir + "/commands"
)

// SandboxSkill is a skill or command a sandbox exposes, with its mount target,
// provenance and live mount state for the Skills tab.
type SandboxSkill struct {
	store.SkillItem
	Target   string   `json:"target"`
	Sources  []string `json:"sources"`
	Mounted  bool     `json:"mounted"`
	Conflict string   `json:"conflict,omitempty"`
	Missing  bool     `json:"missing,omitempty"`
	Orphan   bool     `json:"orphan,omitempty"`
}

// SkillReconcileResult reports what one reconcile pass changed.
type SkillReconcileResult struct {
	Applied int      `json:"applied"`
	Removed int      `json:"removed"`
	Errors  []string `json:"errors"`
}

// desiredSkill is one item a sandbox should have mounted, resolved against its
// store checkout.
type desiredSkill struct {
	item     store.SkillItem
	sources  []string
	host     string
	target   string
	missing  bool
	conflict string
}

// ListSkillStores returns every registered skill store.
func (a *App) ListSkillStores(ctx context.Context) ([]store.SkillStore, error) {
	return a.Store.ListSkillStores(ctx)
}

// ListSkillItems returns the discovered catalog of one store, or of every store
// when storeID is zero.
func (a *App) ListSkillItems(ctx context.Context, storeID int64) ([]store.SkillItem, error) {
	if storeID == 0 {
		return a.Store.ListAllSkillItems(ctx)
	}
	return a.Store.ListSkillItems(ctx, storeID)
}

// CreateSkillStore registers a store, performs its first checkout and applies
// it to already running sandboxes.
func (a *App) CreateSkillStore(ctx context.Context, name, description, url, ref string) (store.SkillStore, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	url = strings.TrimSpace(url)
	ref = strings.TrimSpace(ref)
	if name == "" {
		return store.SkillStore{}, errors.New("skill store name is required")
	}
	if url == "" {
		return store.SkillStore{}, errors.New("git url is required")
	}
	path, err := skillStoreCheckoutPath(name)
	if err != nil {
		return store.SkillStore{}, err
	}
	created, err := a.Store.CreateSkillStore(ctx, store.SkillStore{
		Name:        name,
		Description: description,
		URL:         url,
		Ref:         ref,
		Path:        path,
	})
	if err != nil {
		return store.SkillStore{}, err
	}
	updated, err := a.syncSkillStore(ctx, created)
	if err != nil {
		return store.SkillStore{}, err
	}
	a.reconcileAllSkills(ctx)
	return updated, nil
}

// UpdateSkillStore rewrites a store registration and re-checks out its source
// when the url or ref changed.
func (a *App) UpdateSkillStore(ctx context.Context, id int64, name, description, url, ref string) (store.SkillStore, error) {
	current, err := a.Store.GetSkillStore(ctx, id)
	if err != nil {
		return store.SkillStore{}, err
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	url = strings.TrimSpace(url)
	ref = strings.TrimSpace(ref)
	if name == "" {
		return store.SkillStore{}, errors.New("skill store name is required")
	}
	if url == "" {
		return store.SkillStore{}, errors.New("git url is required")
	}
	updated := store.SkillStore{ID: id, Name: name, Description: description, URL: url, Ref: ref}
	if err := a.Store.UpdateSkillStore(ctx, updated); err != nil {
		return store.SkillStore{}, err
	}
	saved, err := a.Store.GetSkillStore(ctx, id)
	if err != nil {
		return store.SkillStore{}, err
	}
	a.Notify(TopicSkills)
	a.Notify(TopicProfiles)
	if url != current.URL || ref != current.Ref {
		synced, err := a.syncSkillStore(ctx, saved)
		if err != nil {
			return store.SkillStore{}, err
		}
		a.reconcileAllSkills(ctx)
		return synced, nil
	}
	return saved, nil
}

// DeleteSkillStore removes a store, its checkout and every selection that
// pointed at it, then unmounts the orphaned items from running sandboxes.
func (a *App) DeleteSkillStore(ctx context.Context, id int64) error {
	current, err := a.Store.GetSkillStore(ctx, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteSkillStore(ctx, id); err != nil {
		return err
	}
	if strings.TrimSpace(current.Path) != "" {
		_ = os.RemoveAll(current.Path)
	}
	a.Notify(TopicSkills)
	a.Notify(TopicProfiles)
	a.reconcileAllSkills(ctx)
	return nil
}

// RefreshSkillStore re-checks out a store and rebuilds its catalog.
func (a *App) RefreshSkillStore(ctx context.Context, id int64) (store.SkillStore, error) {
	current, err := a.Store.GetSkillStore(ctx, id)
	if err != nil {
		return store.SkillStore{}, err
	}
	updated, err := a.syncSkillStore(ctx, current)
	if err != nil {
		return store.SkillStore{}, err
	}
	a.reconcileAllSkills(ctx)
	a.Notify(TopicProfiles)
	return updated, nil
}

// syncSkillStore re-checks out a store and replaces its catalog. A checkout
// failure is recorded on the store instead of returned, so the registration
// stays editable and visible with its error.
func (a *App) syncSkillStore(ctx context.Context, current store.SkillStore) (store.SkillStore, error) {
	if err := skills.Checkout(ctx, current.Path, current.URL, current.Ref); err != nil {
		if markErr := a.Store.MarkSkillStoreSynced(ctx, current.ID, current.SyncedAt, err.Error()); markErr != nil {
			return store.SkillStore{}, markErr
		}
		a.Notify(TopicSkills)
		return a.Store.GetSkillStore(ctx, current.ID)
	}
	discovered, err := skills.Discover(current.Path)
	if err != nil {
		if markErr := a.Store.MarkSkillStoreSynced(ctx, current.ID, current.SyncedAt, err.Error()); markErr != nil {
			return store.SkillStore{}, markErr
		}
		a.Notify(TopicSkills)
		return a.Store.GetSkillStore(ctx, current.ID)
	}
	items := make([]store.SkillItem, 0, len(discovered))
	for _, item := range discovered {
		items = append(items, store.SkillItem{
			Kind:        string(item.Kind),
			Name:        item.Name,
			Description: item.Description,
			Plugin:      item.Plugin,
			RelPath:     item.RelPath,
		})
	}
	if err := a.Store.ReplaceSkillItems(ctx, current.ID, items); err != nil {
		return store.SkillStore{}, err
	}
	if err := a.Store.MarkSkillStoreSynced(ctx, current.ID, time.Now().UTC().Format(time.RFC3339), ""); err != nil {
		return store.SkillStore{}, err
	}
	a.Notify(TopicSkills)
	return a.Store.GetSkillStore(ctx, current.ID)
}

// AddSkillItemToProfile selects a catalog item for a profile and reconciles the
// sandboxes it is assigned to.
func (a *App) AddSkillItemToProfile(ctx context.Context, profileID, itemID int64) error {
	if _, err := a.Store.GetProfile(ctx, profileID); err != nil {
		return err
	}
	if err := a.Store.AddProfileSkillItem(ctx, profileID, itemID); err != nil {
		return err
	}
	a.Notify(TopicSkills)
	a.Notify(TopicProfiles)
	a.reconcileSkillsForProfile(ctx, profileID)
	return nil
}

// RemoveSkillItemFromProfile deselects a catalog item and reconciles the
// sandboxes the profile is assigned to.
func (a *App) RemoveSkillItemFromProfile(ctx context.Context, profileID, itemID int64) error {
	if _, err := a.Store.GetProfile(ctx, profileID); err != nil {
		return err
	}
	if err := a.Store.RemoveProfileSkillItem(ctx, profileID, itemID); err != nil {
		return err
	}
	a.Notify(TopicSkills)
	a.Notify(TopicProfiles)
	a.reconcileSkillsForProfile(ctx, profileID)
	return nil
}

// AttachSkillItem selects a catalog item directly for one sandbox, on top of
// whatever its profiles provide.
func (a *App) AttachSkillItem(ctx context.Context, sandbox string, itemID int64) error {
	if _, err := a.Sbx.InspectSandbox(ctx, sandbox); err != nil {
		return err
	}
	if err := a.Store.AddSandboxSkillItem(ctx, sandbox, itemID); err != nil {
		return err
	}
	a.Notify(TopicSkills)
	a.Notify(TopicSandbox(sandbox))
	a.reconcileSandboxSkills(ctx, sandbox)
	return nil
}

// DetachSkillItem removes a directly attached item and unmounts it when the
// sandbox is running.
func (a *App) DetachSkillItem(ctx context.Context, sandbox string, itemID int64) error {
	if err := a.Store.RemoveSandboxSkillItem(ctx, sandbox, itemID); err != nil {
		return err
	}
	a.Notify(TopicSkills)
	a.Notify(TopicSandbox(sandbox))
	a.reconcileSandboxSkills(ctx, sandbox)
	return nil
}

// ReconcileSkills converges the sandbox's .agents mounts onto the items
// selected by its profiles and direct attachments: missing items are mounted
// read-only at their target, and managed mounts that are no longer wanted are
// unmounted. Errors are per-item and never abort the whole pass.
func (a *App) ReconcileSkills(ctx context.Context, sandbox string) (SkillReconcileResult, error) {
	result := SkillReconcileResult{Errors: []string{}}
	info, err := a.Sbx.InspectSandbox(ctx, sandbox)
	if err != nil {
		return result, err
	}
	if !info.Running() {
		return result, fmt.Errorf("sandbox %q is not running; start it to mount skills", sandbox)
	}
	desired, errs, err := a.desiredSkills(ctx, sandbox)
	if err != nil {
		return result, err
	}
	result.Errors = append(result.Errors, errs...)
	mounts, err := a.Sbx.Mounts(ctx, sandbox)
	if err != nil {
		return result, err
	}

	want := make(map[string]desiredSkill)
	for _, d := range desired {
		if d.conflict == "" && !d.missing {
			want[d.target] = d
		}
	}
	for _, mount := range mounts {
		target := mount.Target
		if target == "" || !managedSkillTarget(target) {
			continue
		}
		if d, ok := want[target]; ok && d.host == mount.HostPath {
			delete(want, target)
			continue
		}
		if strings.TrimSpace(mount.HostPath) == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("unmount %s: mount has no host path", target))
			delete(want, target)
			continue
		}
		if err := a.Sbx.UnmountFolderAt(ctx, sandbox, mount.HostPath, target); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("unmount %s: %v", target, err))
			delete(want, target)
			continue
		}
		result.Removed++
	}

	for _, d := range sortedDesired(want) {
		parent := d.target
		if d.item.Kind == string(skills.KindCommand) {
			parent = filepath.Dir(d.target)
		}
		_ = a.Sbx.MkdirAll(ctx, sandbox, parent)
		if err := a.Sbx.MountFolderAt(ctx, sandbox, d.host, d.target, true); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("mount %s: %v", d.target, err))
			continue
		}
		result.Applied++
	}
	if result.Applied > 0 || result.Removed > 0 {
		a.Notify(TopicSandbox(sandbox))
	}
	return result, nil
}

// desiredSkills resolves the union of profile items and direct attachments of a
// sandbox against their store checkouts. Colliding targets keep a deterministic
// winner (store name, then item id) and mark the others as conflicts.
func (a *App) desiredSkills(ctx context.Context, sandbox string) ([]desiredSkill, []string, error) {
	profiles, err := a.Store.ListProfilesForSandbox(ctx, sandbox)
	if err != nil {
		return nil, nil, err
	}
	byProfile, err := a.Store.AllProfileSkillItems(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make(map[int64]*desiredSkill)
	var order []int64
	add := func(item store.SkillItem, source string) {
		d, ok := items[item.ID]
		if !ok {
			d = &desiredSkill{item: item}
			items[item.ID] = d
			order = append(order, item.ID)
		}
		for _, existing := range d.sources {
			if existing == source {
				return
			}
		}
		d.sources = append(d.sources, source)
	}
	for _, p := range profiles {
		for _, item := range byProfile[p.ID] {
			add(item, p.Name)
		}
	}
	extras, err := a.Store.ListSandboxSkillItems(ctx, sandbox)
	if err != nil {
		return nil, nil, err
	}
	for _, item := range extras {
		add(item, "sandbox")
	}

	stores, err := a.Store.ListSkillStores(ctx)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[int64]store.SkillStore, len(stores))
	for _, st := range stores {
		byID[st.ID] = st
	}

	var errs []string
	out := make([]desiredSkill, 0, len(order))
	for _, id := range order {
		d := items[id]
		d.target = skillTarget(d.item)
		st, ok := byID[d.item.StoreID]
		if !ok {
			d.missing = true
			errs = append(errs, fmt.Sprintf("%s: store registration is missing", d.item.Name))
		} else {
			d.host = filepath.Join(st.Path, filepath.FromSlash(d.item.RelPath))
			if _, err := os.Stat(d.host); err != nil {
				d.missing = true
				errs = append(errs, fmt.Sprintf("%s: %v", d.item.Name, err))
			}
		}
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].target != out[j].target {
			return out[i].target < out[j].target
		}
		if out[i].item.StoreName != out[j].item.StoreName {
			return out[i].item.StoreName < out[j].item.StoreName
		}
		return out[i].item.ID < out[j].item.ID
	})
	for i := range out {
		if i > 0 && out[i].target == out[i-1].target && out[i].conflict == "" {
			out[i].conflict = out[i-1].item.StoreName
			errs = append(errs, fmt.Sprintf("%s: target %s is already provided by store %s", out[i].item.Name, out[i].target, out[i].conflict))
		}
	}
	return out, errs, nil
}

// sandboxSkills projects the desired items of a sandbox with their live mount
// state, including manual mounts under .agents as orphans.
func (a *App) sandboxSkills(ctx context.Context, sandbox string, mounts []sbx.MountInfo) []SandboxSkill {
	desired, _, err := a.desiredSkills(ctx, sandbox)
	if err != nil {
		return nil
	}
	out := make([]SandboxSkill, 0, len(desired))
	matched := make(map[string]bool)
	for _, d := range desired {
		entry := SandboxSkill{
			SkillItem: d.item,
			Target:    d.target,
			Sources:   d.sources,
			Missing:   d.missing,
			Conflict:  d.conflict,
		}
		if d.conflict == "" && !d.missing {
			entry.Mounted = mountedAt(mounts, d.host, d.target)
		}
		if entry.Mounted {
			matched[d.target] = true
		}
		out = append(out, entry)
	}
	for _, mount := range mounts {
		target := mount.Target
		if target == "" || !managedSkillTarget(target) || matched[target] {
			continue
		}
		out = append(out, SandboxSkill{
			Target:  target,
			Sources: []string{"manual"},
			Mounted: true,
			Orphan:  true,
		})
	}
	return out
}

func managedSkillTarget(target string) bool {
	return strings.HasPrefix(target, sandboxSkillsDir+"/") || strings.HasPrefix(target, sandboxCommandsDir+"/")
}

func skillTarget(item store.SkillItem) string {
	if item.Kind == string(skills.KindCommand) {
		return sandboxCommandsDir + "/" + item.Name + ".md"
	}
	return sandboxSkillsDir + "/" + item.Name
}

func mountedAt(mounts []sbx.MountInfo, host, target string) bool {
	for _, mount := range mounts {
		if mount.HostPath != host {
			continue
		}
		actual := mount.Target
		if actual == "" {
			actual = mount.HostPath
		}
		if actual == target {
			return true
		}
	}
	return false
}

func sortedDesired(want map[string]desiredSkill) []desiredSkill {
	out := make([]desiredSkill, 0, len(want))
	for _, d := range want {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].target < out[j].target })
	return out
}

// reconcileSkillsForProfile reconciles every sandbox assigned to a profile.
func (a *App) reconcileSkillsForProfile(ctx context.Context, profileID int64) {
	sandboxes, err := a.Store.SandboxesForProfile(ctx, profileID)
	if err != nil {
		return
	}
	for _, sandbox := range sandboxes {
		a.reconcileSandboxSkills(ctx, sandbox)
	}
}

// reconcileSandboxSkills reconciles one sandbox, leaving per-item failures to
// surface as drift in the Skills tab.
func (a *App) reconcileSandboxSkills(ctx context.Context, sandbox string) {
	_, _ = a.ReconcileSkills(ctx, sandbox)
}

// reconcileAllSkills reconciles every running sandbox.
func (a *App) reconcileAllSkills(ctx context.Context) {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return
	}
	for _, sandbox := range sandboxes {
		if sandbox.Running() {
			a.reconcileSandboxSkills(ctx, sandbox.Name)
		}
	}
}

// reapplySkillsWhenReady waits for a freshly started sandbox and mounts its
// desired items, retrying briefly while the VM comes up.
func (a *App) reapplySkillsWhenReady(ctx context.Context, name string) {
	desired, _, err := a.desiredSkills(ctx, name)
	if err != nil || len(desired) == 0 {
		return
	}
	for attempt := 0; attempt < 8; attempt++ {
		if ctx.Err() != nil {
			return
		}
		info, err := a.Sbx.InspectSandbox(ctx, name)
		if err == nil && info.Running() {
			if result, err := a.ReconcileSkills(ctx, name); err == nil && len(result.Errors) == 0 {
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

// skillStoreCheckoutPath allocates a stable, collision-free directory for a
// store checkout under the XDG data directory.
func skillStoreCheckoutPath(name string) (string, error) {
	slug := skillSlug(name)
	if slug == "" {
		return "", errors.New("skill store name must contain letters or digits")
	}
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	root := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(root, "sandwarden", "skill-stores", fmt.Sprintf("%s-%x", slug, suffix)), nil
}

// skillSlug reduces a store name to a filesystem-safe slug.
func skillSlug(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
