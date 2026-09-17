package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/skills"
	"github.com/JLugagne/sandwarden/internal/store"
)

// Container locations sandwarden manages for skill and command mounts.
const (
	sandboxAgentsDir   = "/home/agent/.agents"
	sandboxSkillsDir   = sandboxAgentsDir + "/skills"
	sandboxCommandsDir = sandboxAgentsDir + "/commands"
)

// StoreInput is the editable registration of a git store.
type StoreInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Ref         string `json:"ref"`
	Auth        string `json:"auth"`
}

// SkillStoreView is a skill store registration plus its checkout and sync
// state.
type SkillStoreView struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Ref         string `json:"ref"`
	Auth        string `json:"auth"`
	Path        string `json:"path"`
	SyncedAt    string `json:"synced_at"`
	Error       string `json:"error"`
}

// SandboxSkill is a skill or command a sandbox exposes, with its mount
// target, provenance and live mount state for the Skills tab.
type SandboxSkill struct {
	store.SkillItem
	StoreName string   `json:"store_name"`
	Target    string   `json:"target"`
	Sources   []string `json:"sources"`
	Mounted   bool     `json:"mounted"`
	Conflict  string   `json:"conflict,omitempty"`
	Missing   bool     `json:"missing,omitempty"`
	Orphan    bool     `json:"orphan,omitempty"`
}

// desiredSkill is one item a sandbox should have mounted, resolved against
// its store checkout.
type desiredSkill struct {
	item      store.SkillItem
	storeName string
	sources   []string
	host      string
	target    string
	missing   bool
	conflict  string
}

// ListSkillStores returns every registered skill store.
func (a *App) ListSkillStores(ctx context.Context) ([]SkillStoreView, error) {
	return a.skillStoreViews(ctx)
}

// CreateSkillStore registers a store, checks it out and discovers its items.
func (a *App) CreateSkillStore(ctx context.Context, input StoreInput) (SkillStoreView, error) {
	if err := validateStoreInput(input); err != nil {
		return SkillStoreView{}, err
	}
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{
		Kind:        fleet.StoreSkills,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		URL:         strings.TrimSpace(input.URL),
		Ref:         strings.TrimSpace(input.Ref),
		Auth:        strings.TrimSpace(input.Auth),
	})
	if err != nil {
		return SkillStoreView{}, err
	}
	if _, err := a.syncSkillStore(ctx, reg); err != nil {
		a.Notify(TopicSkills)
		return a.skillStoreView(ctx, *reg), nil
	}
	a.Notify(TopicSkills)
	return a.skillStoreView(ctx, *reg), nil
}

// UpdateSkillStore rewrites a registration and re-syncs when the source
// changed.
func (a *App) UpdateSkillStore(ctx context.Context, slug string, input StoreInput) (SkillStoreView, error) {
	if err := validateStoreInput(input); err != nil {
		return SkillStoreView{}, err
	}
	reg, ok := a.Fleet.Store(fleet.StoreSkills, slug)
	if !ok {
		return SkillStoreView{}, store.ErrNotFound
	}
	changed := reg.URL != strings.TrimSpace(input.URL) || reg.Ref != strings.TrimSpace(input.Ref) || reg.Auth != strings.TrimSpace(input.Auth)
	reg.Name = strings.TrimSpace(input.Name)
	reg.Description = strings.TrimSpace(input.Description)
	reg.URL = strings.TrimSpace(input.URL)
	reg.Ref = strings.TrimSpace(input.Ref)
	reg.Auth = strings.TrimSpace(input.Auth)
	if err := a.Fleet.SaveStore(reg); err != nil {
		return SkillStoreView{}, err
	}
	if changed {
		_, _ = a.syncSkillStore(ctx, reg)
	}
	a.Notify(TopicSkills)
	return a.skillStoreView(ctx, *reg), nil
}

// DeleteSkillStore forgets a store, its checkout and its catalog.
func (a *App) DeleteSkillStore(ctx context.Context, slug string) error {
	reg, ok := a.Fleet.Store(fleet.StoreSkills, slug)
	if !ok {
		return store.ErrNotFound
	}
	if err := a.Fleet.DeleteStore(fleet.StoreSkills, slug); err != nil {
		return err
	}
	if err := a.Store.DeleteStoreCatalog(ctx, string(fleet.StoreSkills), slug); err != nil {
		return err
	}
	if err := a.Store.DeleteStoreState(ctx, string(fleet.StoreSkills), slug); err != nil {
		return err
	}
	_ = os.RemoveAll(storeCheckoutPath(fleet.StoreSkills, reg.Slug))
	a.Notify(TopicSkills)
	return nil
}

// RefreshSkillStore re-runs the checkout and discovery.
func (a *App) RefreshSkillStore(ctx context.Context, slug string) (SkillStoreView, error) {
	reg, ok := a.Fleet.Store(fleet.StoreSkills, slug)
	if !ok {
		return SkillStoreView{}, store.ErrNotFound
	}
	_, err := a.syncSkillStore(ctx, reg)
	a.Notify(TopicSkills)
	return a.skillStoreView(ctx, *reg), err
}

// ListSkillItems returns the discovered items of one store, or of every store
// when storeSlug is empty.
func (a *App) ListSkillItems(ctx context.Context, storeSlug string) ([]store.SkillItem, error) {
	return a.Store.ListSkillItems(ctx, storeSlug)
}

// AddSkillItemToProfile selects a catalog item on a profile.
func (a *App) AddSkillItemToProfile(ctx context.Context, profileSlug string, ref fleet.SkillRef) error {
	p, ok := a.Fleet.Profile(profileSlug)
	if !ok {
		return store.ErrNotFound
	}
	if !slices.Contains(p.App.Skills, ref) {
		p.App.Skills = append(p.App.Skills, ref)
	}
	if err := a.Fleet.SaveProfile(p); err != nil {
		return err
	}
	a.reapplyProfileToSandboxes(ctx, profileSlug)
	a.Notify(TopicProfiles)
	return nil
}

// RemoveSkillItemFromProfile drops a selection from a profile.
func (a *App) RemoveSkillItemFromProfile(ctx context.Context, profileSlug string, ref fleet.SkillRef) error {
	p, ok := a.Fleet.Profile(profileSlug)
	if !ok {
		return store.ErrNotFound
	}
	kept := p.App.Skills[:0]
	for _, s := range p.App.Skills {
		if s != ref {
			kept = append(kept, s)
		}
	}
	p.App.Skills = kept
	if err := a.Fleet.SaveProfile(p); err != nil {
		return err
	}
	a.reapplyProfileToSandboxes(ctx, profileSlug)
	a.Notify(TopicProfiles)
	return nil
}

// AttachSkillItem selects a catalog item directly on a sandbox.
func (a *App) AttachSkillItem(ctx context.Context, name string, ref fleet.SkillRef) error {
	s, err := a.ensureSandboxConfig(ctx, name)
	if err != nil {
		return err
	}
	if !slices.Contains(s.App.Skills, ref) {
		s.App.Skills = append(s.App.Skills, ref)
	}
	if err := a.Fleet.SaveSandbox(s); err != nil {
		return err
	}
	a.reconcileSandboxSkills(ctx, name)
	a.Notify(TopicSandbox(name))
	return nil
}

// DetachSkillItem drops a direct selection from a sandbox.
func (a *App) DetachSkillItem(ctx context.Context, name string, ref fleet.SkillRef) error {
	s, err := a.ensureSandboxConfig(ctx, name)
	if err != nil {
		return err
	}
	kept := s.App.Skills[:0]
	for _, item := range s.App.Skills {
		if item != ref {
			kept = append(kept, item)
		}
	}
	s.App.Skills = kept
	if err := a.Fleet.SaveSandbox(s); err != nil {
		return err
	}
	a.reconcileSandboxSkills(ctx, name)
	a.Notify(TopicSandbox(name))
	return nil
}

// ReconcileSkills converges the sandbox's .agents mounts onto the items
// selected by its profiles and direct attachments: missing items are mounted
// read-only at their target, and managed mounts that are no longer wanted are
// unmounted. Errors are per-item and never abort the whole pass.
func (a *App) ReconcileSkills(ctx context.Context, name string) (SkillReconcileResult, error) {
	result := SkillReconcileResult{Errors: []string{}}
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil {
		return result, err
	}
	if !info.Running() {
		return result, fmt.Errorf("sandbox %q is not running; start it to mount skills", name)
	}
	desired, errs, err := a.desiredSkills(ctx, name)
	if err != nil {
		return result, err
	}
	result.Errors = append(result.Errors, errs...)
	mounts, err := a.Sbx.Mounts(ctx, name)
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
		if err := a.Sbx.UnmountFolderAt(ctx, name, mount.HostPath, target); err != nil {
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
		_ = a.Sbx.MkdirAll(ctx, name, parent)
		if err := a.Sbx.MountFolderAt(ctx, name, d.host, d.target, true); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("mount %s: %v", d.target, err))
			continue
		}
		result.Applied++
	}
	if result.Applied > 0 || result.Removed > 0 {
		a.Notify(TopicSandbox(name))
	}
	return result, nil
}

// desiredSkills resolves the union of profile selections and direct
// attachments into host checkout paths and mount targets.
func (a *App) desiredSkills(ctx context.Context, name string) ([]desiredSkill, []string, error) {
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return nil, nil, fmt.Errorf("no configuration found for sandbox %q", name)
	}
	type ref struct {
		item   fleet.SkillRef
		source string
	}
	var refs []ref
	for _, p := range a.profilesForSandbox(s) {
		for _, item := range p.App.Skills {
			refs = append(refs, ref{item: item, source: p.Label()})
		}
	}
	for _, item := range s.App.Skills {
		refs = append(refs, ref{item: item, source: "sandbox"})
	}
	grouped := map[fleet.SkillRef]*desiredSkill{}
	var order []fleet.SkillRef
	var errs []string
	for _, r := range refs {
		d, ok := grouped[r.item]
		if !ok {
			item, err := a.Store.GetSkillItem(ctx, r.item.Store, r.item.Kind, r.item.Name)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", r.item.Store, r.item.Name, err))
				continue
			}
			reg, ok := a.Fleet.Store(fleet.StoreSkills, r.item.Store)
			d = &desiredSkill{item: item}
			if ok {
				d.storeName = reg.Name
				d.host = filepath.Join(storeCheckoutPath(fleet.StoreSkills, reg.Slug), filepath.FromSlash(item.RelPath))
				if _, err := os.Stat(d.host); err != nil {
					d.missing = true
					errs = append(errs, fmt.Sprintf("%s: %v", item.Name, err))
				}
			} else {
				d.missing = true
				errs = append(errs, fmt.Sprintf("%s: store registration is missing", item.Name))
			}
			d.target = skillTarget(item)
			grouped[r.item] = d
			order = append(order, r.item)
		}
		if !slices.Contains(d.sources, r.source) {
			d.sources = append(d.sources, r.source)
		}
	}
	out := make([]desiredSkill, 0, len(order))
	for _, key := range order {
		out = append(out, *grouped[key])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].target != out[j].target {
			return out[i].target < out[j].target
		}
		if out[i].storeName != out[j].storeName {
			return out[i].storeName < out[j].storeName
		}
		return out[i].item.Name < out[j].item.Name
	})
	for i := range out {
		if i > 0 && out[i].target == out[i-1].target && out[i].conflict == "" {
			out[i].conflict = out[i-1].storeName
			errs = append(errs, fmt.Sprintf("%s: target %s is already provided by store %s", out[i].item.Name, out[i].target, out[i].conflict))
		}
	}
	return out, errs, nil
}

// sandboxSkills is the Skills tab projection: desired items plus orphan
// mounts under the managed directories.
func (a *App) sandboxSkills(ctx context.Context, name string, mounts []sbx.MountInfo) []SandboxSkill {
	desired, _, err := a.desiredSkills(ctx, name)
	if err != nil {
		return nil
	}
	out := make([]SandboxSkill, 0, len(desired))
	matched := make(map[string]bool)
	for _, d := range desired {
		entry := SandboxSkill{
			SkillItem: d.item,
			StoreName: d.storeName,
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

func (a *App) syncSkillStore(ctx context.Context, reg *fleet.StoreReg) (*fleet.StoreReg, error) {
	path := storeCheckoutPath(fleet.StoreSkills, reg.Slug)
	if err := skills.Checkout(ctx, path, reg.URL, reg.Ref, skills.Auth(reg.Auth)); err != nil {
		_ = a.Store.SetStoreSync(ctx, string(fleet.StoreSkills), reg.Slug, "", err.Error())
		return reg, err
	}
	found, err := skills.Discover(path)
	if err != nil {
		_ = a.Store.SetStoreSync(ctx, string(fleet.StoreSkills), reg.Slug, "", err.Error())
		return reg, err
	}
	items := make([]store.SkillItem, 0, len(found))
	for _, item := range found {
		items = append(items, store.SkillItem{
			Store:       reg.Slug,
			Kind:        string(item.Kind),
			Name:        item.Name,
			Description: item.Description,
			Plugin:      item.Plugin,
			RelPath:     item.RelPath,
		})
	}
	if err := a.Store.ReplaceSkillItems(ctx, reg.Slug, items); err != nil {
		_ = a.Store.SetStoreSync(ctx, string(fleet.StoreSkills), reg.Slug, "", err.Error())
		return reg, err
	}
	_ = a.Store.SetStoreSync(ctx, string(fleet.StoreSkills), reg.Slug, time.Now().UTC().Format(time.RFC3339), "")
	return reg, nil
}

func (a *App) skillStoreViews(ctx context.Context) ([]SkillStoreView, error) {
	states, err := a.Store.StoreStates(ctx, string(fleet.StoreSkills))
	if err != nil {
		return nil, err
	}
	stores := a.Fleet.Stores(fleet.StoreSkills)
	out := make([]SkillStoreView, 0, len(stores))
	for _, reg := range stores {
		view := skillStoreView(*reg, states[reg.Slug])
		out = append(out, view)
	}
	return out, nil
}

func (a *App) skillStoreView(ctx context.Context, reg fleet.StoreReg) SkillStoreView {
	states, _ := a.Store.StoreStates(ctx, string(fleet.StoreSkills))
	return skillStoreView(reg, states[reg.Slug])
}

func skillStoreView(reg fleet.StoreReg, state store.StoreState) SkillStoreView {
	return SkillStoreView{
		Slug:        reg.Slug,
		Name:        reg.Name,
		Description: reg.Description,
		URL:         reg.URL,
		Ref:         reg.Ref,
		Auth:        reg.Auth,
		Path:        storeCheckoutPath(fleet.StoreSkills, reg.Slug),
		SyncedAt:    state.SyncedAt,
		Error:       state.Error,
	}
}

// SkillReconcileResult reports one convergence pass.
type SkillReconcileResult struct {
	Applied int      `json:"applied"`
	Removed int      `json:"removed"`
	Errors  []string `json:"errors"`
}

// reconcileSandboxSkills runs a best-effort skills convergence.
func (a *App) reconcileSandboxSkills(ctx context.Context, name string) {
	_, _ = a.ReconcileSkills(ctx, name)
}

func sortedDesired(want map[string]desiredSkill) []desiredSkill {
	out := make([]desiredSkill, 0, len(want))
	for _, d := range want {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].target < out[j].target })
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

func validateStoreInput(input StoreInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("store name is required")
	}
	if strings.TrimSpace(input.URL) == "" {
		return errors.New("store url is required")
	}
	return nil
}
