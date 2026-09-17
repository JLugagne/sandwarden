package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/kits"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/skills"
	"github.com/JLugagne/sandwarden/internal/store"
)

// KitItemView is one catalog kit with the reference usable at create time and
// its parsed spec for the detail view.
type KitItemView struct {
	store.KitItem
	Ref  string      `json:"ref"`
	Spec sbx.KitSpec `json:"spec"`
}

// KitValidation is the verdict of `sbx kit validate`.
type KitValidation struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
}

// ListKitStores returns every registered kit repository.
func (a *App) ListKitStores(ctx context.Context) ([]store.KitStore, error) {
	return a.Store.ListKitStores(ctx)
}

// ListKitItems returns the discovered kits of one repository, or of every
// repository when storeID is zero.
func (a *App) ListKitItems(ctx context.Context, storeID int64) ([]KitItemView, error) {
	var (
		items []store.KitItem
		err   error
	)
	if storeID == 0 {
		items, err = a.Store.ListAllKitItems(ctx)
	} else {
		items, err = a.Store.ListKitItems(ctx, storeID)
	}
	if err != nil {
		return nil, err
	}
	stores, err := a.Store.ListKitStores(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]store.KitStore, len(stores))
	for _, s := range stores {
		byID[s.ID] = s
	}
	out := make([]KitItemView, 0, len(items))
	for _, item := range items {
		view := KitItemView{KitItem: item}
		if s, ok := byID[item.StoreID]; ok {
			view.Ref = kitReference(s, item.RelPath)
		}
		if strings.TrimSpace(item.Spec) != "" {
			_ = json.Unmarshal([]byte(item.Spec), &view.Spec)
		}
		out = append(out, view)
	}
	return out, nil
}

// CreateKitStore registers a repository and performs its first checkout.
func (a *App) CreateKitStore(ctx context.Context, input store.KitStore) (store.KitStore, error) {
	normalized, err := normalizeKitStore(input)
	if err != nil {
		return store.KitStore{}, err
	}
	path, err := kitStoreCheckoutPath(normalized.Name)
	if err != nil {
		return store.KitStore{}, err
	}
	normalized.Path = path
	created, err := a.Store.CreateKitStore(ctx, normalized)
	if err != nil {
		return store.KitStore{}, err
	}
	return a.syncKitStore(ctx, created)
}

// UpdateKitStore rewrites a registration and re-checks out its source when the
// url or ref changed.
func (a *App) UpdateKitStore(ctx context.Context, id int64, input store.KitStore) (store.KitStore, error) {
	current, err := a.Store.GetKitStore(ctx, id)
	if err != nil {
		return store.KitStore{}, err
	}
	normalized, err := normalizeKitStore(input)
	if err != nil {
		return store.KitStore{}, err
	}
	normalized.ID = id
	if err := a.Store.UpdateKitStore(ctx, normalized); err != nil {
		return store.KitStore{}, err
	}
	saved, err := a.Store.GetKitStore(ctx, id)
	if err != nil {
		return store.KitStore{}, err
	}
	if saved.URL != current.URL || saved.Ref != current.Ref || saved.Auth != current.Auth {
		return a.syncKitStore(ctx, saved)
	}
	return saved, nil
}

// DeleteKitStore removes a repository, its checkout and its catalog.
func (a *App) DeleteKitStore(ctx context.Context, id int64) error {
	current, err := a.Store.GetKitStore(ctx, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteKitStore(ctx, id); err != nil {
		return err
	}
	if strings.TrimSpace(current.Path) != "" {
		_ = os.RemoveAll(current.Path)
	}
	return nil
}

// RefreshKitStore re-checks out a repository and rebuilds its catalog.
func (a *App) RefreshKitStore(ctx context.Context, id int64) (store.KitStore, error) {
	current, err := a.Store.GetKitStore(ctx, id)
	if err != nil {
		return store.KitStore{}, err
	}
	return a.syncKitStore(ctx, current)
}

// KitValidate runs `sbx kit validate` on one catalog kit.
func (a *App) KitValidate(ctx context.Context, itemID int64) (KitValidation, error) {
	item, err := a.Store.GetKitItem(ctx, itemID)
	if err != nil {
		return KitValidation{}, err
	}
	storeRow, err := a.Store.GetKitStore(ctx, item.StoreID)
	if err != nil {
		return KitValidation{}, err
	}
	path := kitItemPath(storeRow, item)
	output, runErr := a.Sbx.KitValidate(ctx, path)
	if runErr != nil {
		return KitValidation{OK: false, Output: strings.TrimSpace(output)}, nil
	}
	return KitValidation{OK: true, Output: strings.TrimSpace(output)}, nil
}

// syncKitStore re-checks out a repository, discovers its kits through
// `sbx kit inspect` and replaces the catalog. Checkout failures are recorded on
// the store instead of returned so the registration stays editable.
func (a *App) syncKitStore(ctx context.Context, current store.KitStore) (store.KitStore, error) {
	if err := skills.Checkout(ctx, current.Path, current.URL, current.Ref, skills.Auth(current.Auth)); err != nil {
		return a.markKitStoreSync(ctx, current, "", err)
	}
	candidates, err := kits.Discover(current.Path)
	if err != nil {
		return a.markKitStoreSync(ctx, current, "", err)
	}
	items := make([]store.KitItem, 0, len(candidates))
	skipped := 0
	var firstSkip error
	for _, candidate := range candidates {
		ref := filepath.Join(current.Path, filepath.FromSlash(candidate.RelPath))
		spec, err := a.Sbx.KitInspect(ctx, ref)
		if err != nil {
			skipped++
			if firstSkip == nil {
				firstSkip = fmt.Errorf("%s: %w", candidate.RelPath, err)
			}
			continue
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			return store.KitStore{}, err
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = candidate.Name
		}
		items = append(items, store.KitItem{
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
	if err := a.Store.ReplaceKitItems(ctx, current.ID, items); err != nil {
		return store.KitStore{}, err
	}
	var syncErr error
	if skipped > 0 {
		syncErr = fmt.Errorf("%d kit(s) skipped: %s", skipped, firstSkip)
	}
	if err := a.allowKitSource(ctx, current.URL); err != nil {
		if syncErr == nil {
			syncErr = err
		} else {
			syncErr = fmt.Errorf("%w; %v", syncErr, err)
		}
	}
	return a.markKitStoreSync(ctx, current, time.Now().UTC().Format(time.RFC3339), syncErr)
}

func (a *App) markKitStoreSync(ctx context.Context, current store.KitStore, syncedAt string, syncErr error) (store.KitStore, error) {
	message := ""
	if syncErr != nil {
		message = syncErr.Error()
	}
	if err := a.Store.MarkKitStoreSynced(ctx, current.ID, syncedAt, message); err != nil {
		return store.KitStore{}, err
	}
	return a.Store.GetKitStore(ctx, current.ID)
}

// kitItemPath resolves a catalog kit to its directory in the checkout.
func kitItemPath(s store.KitStore, item store.KitItem) string {
	return filepath.Join(s.Path, filepath.FromSlash(item.RelPath))
}

// kitReference builds the `git+…` reference `sbx create --kit` accepts for a
// kit discovered in a repository checkout.
func kitReference(s store.KitStore, relPath string) string {
	url := strings.TrimSpace(s.URL)
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
	if pinned := strings.TrimSpace(s.Ref); pinned != "" {
		params = append(params, "ref="+pinned)
	}
	if len(params) > 0 {
		ref += "#" + strings.Join(params, "&")
	}
	return ref
}

func normalizeKitStore(input store.KitStore) (store.KitStore, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.URL = strings.TrimSpace(input.URL)
	input.Ref = strings.TrimSpace(input.Ref)
	input.Auth = strings.TrimSpace(input.Auth)
	if input.Name == "" {
		return store.KitStore{}, errors.New("kit repository name is required")
	}
	if input.URL == "" {
		return store.KitStore{}, errors.New("git url is required")
	}
	if err := skills.ValidateAuthURL(input.URL, skills.Auth(input.Auth)); err != nil {
		return store.KitStore{}, err
	}
	return input, nil
}

// kitStoreCheckoutPath allocates a stable, collision-free directory for a kit
// repository checkout under the XDG data directory.
func kitStoreCheckoutPath(name string) (string, error) {
	slug := skillSlug(name)
	if slug == "" {
		return "", errors.New("kit repository name must contain letters or digits")
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
	return filepath.Join(root, "sandwarden", "kit-stores", fmt.Sprintf("%s-%x", slug, suffix)), nil
}

// kitAllowedSourcesKey is the sbx setting listing the kit source prefixes a
// sandbox may install kits from.
const kitAllowedSourcesKey = "kit.allowedSources"

// kitSourcePrefix derives the `kit.allowedSources` entry that admits a kit
// repository URL: host and path, without scheme, credentials, port or `.git`.
// Local sources (file:// URLs or plain paths) have no remote prefix and
// return "".
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
		// scp-like syntax: [user@]host:path
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

// allowKitSource makes sure a kit repository URL is admitted by the
// `kit.allowedSources` setting, merging the URL's host and path prefix into
// the existing list. It is a no-op for local sources or when an entry already
// admits the source.
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

// kitSourceAllowed reports whether an allowlist entry admits source, following
// sbx's path-segment prefix rule. "*" admits every remote source.
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
