package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/kits"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/skills"
	"github.com/JLugagne/sandwarden/internal/store"
)

// KitStoreView is a kit repository registration plus its checkout and sync
// state.
type KitStoreView struct {
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

// KitItemView is one catalog kit with the reference usable at create time and
// its parsed spec for the detail view.
type KitItemView struct {
	store.KitItem
	StoreName string      `json:"store_name"`
	Ref       string      `json:"ref"`
	Spec      sbx.KitSpec `json:"spec"`
}

// KitValidation is the verdict of `sbx kit validate`.
type KitValidation struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
}

// ListKitStores returns every registered kit repository.
func (a *App) ListKitStores(ctx context.Context) ([]KitStoreView, error) {
	states, err := a.Store.StoreStates(ctx, string(fleet.StoreKits))
	if err != nil {
		return nil, err
	}
	stores := a.Fleet.Stores(fleet.StoreKits)
	out := make([]KitStoreView, 0, len(stores))
	for _, reg := range stores {
		out = append(out, kitStoreView(*reg, states[reg.Slug]))
	}
	return out, nil
}

// CreateKitStore registers a kit repository, checks it out and discovers its
// kits.
func (a *App) CreateKitStore(ctx context.Context, input StoreInput) (KitStoreView, error) {
	if err := validateStoreInput(input); err != nil {
		return KitStoreView{}, err
	}
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{
		Kind:        fleet.StoreKits,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		URL:         strings.TrimSpace(input.URL),
		Ref:         strings.TrimSpace(input.Ref),
		Auth:        strings.TrimSpace(input.Auth),
	})
	if err != nil {
		return KitStoreView{}, err
	}
	_, _ = a.syncKitStore(ctx, reg)
	a.Notify(TopicKits)
	return a.kitStoreView(ctx, *reg), nil
}

// UpdateKitStore rewrites a registration and re-syncs when the source
// changed.
func (a *App) UpdateKitStore(ctx context.Context, slug string, input StoreInput) (KitStoreView, error) {
	if err := validateStoreInput(input); err != nil {
		return KitStoreView{}, err
	}
	reg, ok := a.Fleet.Store(fleet.StoreKits, slug)
	if !ok {
		return KitStoreView{}, store.ErrNotFound
	}
	changed := reg.URL != strings.TrimSpace(input.URL) || reg.Ref != strings.TrimSpace(input.Ref) || reg.Auth != strings.TrimSpace(input.Auth)
	reg.Name = strings.TrimSpace(input.Name)
	reg.Description = strings.TrimSpace(input.Description)
	reg.URL = strings.TrimSpace(input.URL)
	reg.Ref = strings.TrimSpace(input.Ref)
	reg.Auth = strings.TrimSpace(input.Auth)
	if err := a.Fleet.SaveStore(reg); err != nil {
		return KitStoreView{}, err
	}
	if changed {
		_, _ = a.syncKitStore(ctx, reg)
	}
	a.Notify(TopicKits)
	return a.kitStoreView(ctx, *reg), nil
}

// DeleteKitStore forgets a repository, its checkout and its catalog.
func (a *App) DeleteKitStore(ctx context.Context, slug string) error {
	reg, ok := a.Fleet.Store(fleet.StoreKits, slug)
	if !ok {
		return store.ErrNotFound
	}
	if err := a.Fleet.DeleteStore(fleet.StoreKits, slug); err != nil {
		return err
	}
	if err := a.Store.DeleteStoreCatalog(ctx, string(fleet.StoreKits), slug); err != nil {
		return err
	}
	if err := a.Store.DeleteStoreState(ctx, string(fleet.StoreKits), slug); err != nil {
		return err
	}
	_ = os.RemoveAll(storeCheckoutPath(fleet.StoreKits, reg.Slug))
	a.Notify(TopicKits)
	return nil
}

// RefreshKitStore re-runs the checkout and discovery.
func (a *App) RefreshKitStore(ctx context.Context, slug string) (KitStoreView, error) {
	reg, ok := a.Fleet.Store(fleet.StoreKits, slug)
	if !ok {
		return KitStoreView{}, store.ErrNotFound
	}
	_, err := a.syncKitStore(ctx, reg)
	a.Notify(TopicKits)
	return a.kitStoreView(ctx, *reg), err
}

// ListKitItems returns the discovered kits of one store, or of every store
// when storeSlug is empty.
func (a *App) ListKitItems(ctx context.Context, storeSlug string) ([]KitItemView, error) {
	items, err := a.Store.ListKitItems(ctx, storeSlug)
	if err != nil {
		return nil, err
	}
	out := make([]KitItemView, 0, len(items))
	for _, item := range items {
		view := KitItemView{KitItem: item}
		if reg, ok := a.Fleet.Store(fleet.StoreKits, item.Store); ok {
			view.StoreName = reg.Name
			view.Ref = kitReference(*reg, item.RelPath)
		}
		if strings.TrimSpace(item.Spec) != "" {
			_ = json.Unmarshal([]byte(item.Spec), &view.Spec)
		}
		out = append(out, view)
	}
	return out, nil
}

// KitValidate runs `sbx kit validate` over one catalog kit.
func (a *App) KitValidate(ctx context.Context, storeSlug, name string) (KitValidation, error) {
	item, err := a.Store.GetKitItem(ctx, storeSlug, name)
	if err != nil {
		return KitValidation{}, err
	}
	reg, ok := a.Fleet.Store(fleet.StoreKits, storeSlug)
	if !ok {
		return KitValidation{}, fmt.Errorf("kit store %q not found", storeSlug)
	}
	dir := filepath.Join(storeCheckoutPath(fleet.StoreKits, reg.Slug), filepath.FromSlash(item.RelPath))
	output, err := a.Sbx.KitValidate(ctx, dir)
	if err != nil {
		return KitValidation{OK: false, Output: err.Error()}, nil
	}
	return KitValidation{OK: true, Output: output}, nil
}

func (a *App) syncKitStore(ctx context.Context, reg *fleet.StoreReg) (*fleet.StoreReg, error) {
	path := storeCheckoutPath(fleet.StoreKits, reg.Slug)
	if err := skills.Checkout(ctx, path, reg.URL, reg.Ref, skills.Auth(reg.Auth)); err != nil {
		_ = a.Store.SetStoreSync(ctx, string(fleet.StoreKits), reg.Slug, "", err.Error())
		return reg, err
	}
	candidates, err := kits.Discover(path)
	if err != nil {
		_ = a.Store.SetStoreSync(ctx, string(fleet.StoreKits), reg.Slug, "", err.Error())
		return reg, err
	}
	items := make([]store.KitItem, 0, len(candidates))
	skipped := 0
	var firstSkip error
	for _, candidate := range candidates {
		spec, err := a.Sbx.KitInspect(ctx, filepath.Join(path, filepath.FromSlash(candidate.RelPath)))
		if err != nil {
			skipped++
			if firstSkip == nil {
				firstSkip = fmt.Errorf("%s: %w", candidate.RelPath, err)
			}
			continue
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			_ = a.Store.SetStoreSync(ctx, string(fleet.StoreKits), reg.Slug, "", err.Error())
			return reg, err
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = candidate.Name
		}
		items = append(items, store.KitItem{
			Store:         reg.Slug,
			Kind:          strings.TrimSpace(spec.Kind),
			Name:          name,
			DisplayName:   spec.DisplayName,
			Description:   spec.Description,
			Version:       spec.Version,
			Image:         spec.Sandbox.Image,
			RequiresAgent: spec.Requires.Agent,
			RelPath:       candidate.RelPath,
			Spec:          string(raw),
		})
	}
	if err := a.Store.ReplaceKitItems(ctx, reg.Slug, items); err != nil {
		_ = a.Store.SetStoreSync(ctx, string(fleet.StoreKits), reg.Slug, "", err.Error())
		return reg, err
	}
	var syncErr error
	if skipped > 0 {
		syncErr = fmt.Errorf("%d kit(s) skipped: %s", skipped, firstSkip)
	}
	if err := a.allowKitSource(ctx, reg.URL); err != nil {
		if syncErr == nil {
			syncErr = err
		} else {
			syncErr = fmt.Errorf("%w; %v", syncErr, err)
		}
	}
	message := ""
	if syncErr != nil {
		message = syncErr.Error()
	}
	if err := a.Store.SetStoreSync(ctx, string(fleet.StoreKits), reg.Slug, time.Now().UTC().Format(time.RFC3339), message); err != nil {
		return reg, err
	}
	return reg, nil
}

func (a *App) kitStoreView(ctx context.Context, reg fleet.StoreReg) KitStoreView {
	states, _ := a.Store.StoreStates(ctx, string(fleet.StoreKits))
	return kitStoreView(reg, states[reg.Slug])
}

func kitStoreView(reg fleet.StoreReg, state store.StoreState) KitStoreView {
	return KitStoreView{
		Slug:        reg.Slug,
		Name:        reg.Name,
		Description: reg.Description,
		URL:         reg.URL,
		Ref:         reg.Ref,
		Auth:        reg.Auth,
		Path:        storeCheckoutPath(fleet.StoreKits, reg.Slug),
		SyncedAt:    state.SyncedAt,
		Error:       state.Error,
	}
}

const kitAllowedSourcesKey = "kit.allowedSources"

// allowKitSource merges a repository's host and path prefix into the sbx
// kit.allowedSources setting.
func (a *App) allowKitSource(ctx context.Context, rawURL string) error {
	source := kitSourcePrefix(rawURL)
	if source == "" {
		return nil
	}
	current, err := a.Sbx.SettingsGet(ctx, kitAllowedSourcesKey)
	if err != nil {
		return fmt.Errorf("read %s: %w", kitAllowedSourcesKey, err)
	}
	var allowed []string
	if strings.TrimSpace(current) != "" {
		if err := json.Unmarshal([]byte(current), &allowed); err != nil {
			return fmt.Errorf("parse %s: %w", kitAllowedSourcesKey, err)
		}
	}
	for _, entry := range allowed {
		if kitSourceAllowed(entry, source) {
			return nil
		}
	}
	allowed = append(allowed, source)
	merged, err := json.Marshal(allowed)
	if err != nil {
		return fmt.Errorf("encode %s: %w", kitAllowedSourcesKey, err)
	}
	if err := a.Sbx.SettingsSet(ctx, kitAllowedSourcesKey, string(merged)); err != nil {
		return fmt.Errorf("allow kit source %s: %w", source, err)
	}
	return nil
}

// kitReference builds the `git+…#dir=…&ref=…` reference of a catalog kit.
func kitReference(reg fleet.StoreReg, relPath string) string {
	url := strings.TrimSpace(reg.URL)
	if url == "" {
		return ""
	}
	ref := url
	if !strings.HasPrefix(ref, "git+") {
		ref = "git+" + ref
	}
	var params []string
	if rel := strings.Trim(strings.TrimSpace(filepath.ToSlash(relPath)), "/"); rel != "" && rel != "." {
		params = append(params, "dir="+rel)
	}
	if pinned := strings.TrimSpace(reg.Ref); pinned != "" {
		params = append(params, "ref="+pinned)
	}
	if len(params) > 0 {
		ref += "#" + strings.Join(params, "&")
	}
	return ref
}

// kitSourcePrefix reduces a repository URL to the host and path prefix
// allowed in the sandboxd setting.
func kitSourcePrefix(rawURL string) string {
	raw := strings.TrimSpace(rawURL)
	raw = strings.TrimPrefix(raw, "git+")
	if raw == "" {
		return ""
	}
	var host, path string
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		host, path = parsed.Hostname(), parsed.Path
	} else {
		rest := raw
		if at := strings.Index(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return ""
		}
		host, path = rest[:colon], rest[colon+1:]
	}
	if host == "" {
		return ""
	}
	host = strings.ToLower(host)
	path = strings.Trim(strings.TrimSuffix(path, ".git"), "/")
	if path == "" {
		return host
	}
	return host + "/" + path
}

func kitSourceAllowed(entry, source string) bool {
	entry = strings.ToLower(strings.Trim(strings.TrimSpace(entry), "/"))
	if entry == "" {
		return false
	}
	if entry == "*" {
		return true
	}
	source = strings.ToLower(source)
	return source == entry || strings.HasPrefix(source, entry+"/")
}
