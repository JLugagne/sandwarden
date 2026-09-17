package app

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/JLugagne/sandwarden/internal/search"
)

// minQueryRunes is the shortest query the global search answers.
const minQueryRunes = 2

// defaultSearchLimit caps the number of ranked results returned to the UI.
const defaultSearchLimit = 25

// Search ranks the fleet config entities (sandboxes, profiles and caches) and
// the skill, command and kit catalogs against a free-text query. The index is
// built lazily on first use and kept until the store catalogs or the in-memory
// fleet change.
func (a *App) Search(ctx context.Context, query string) ([]search.Result, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < minQueryRunes {
		return nil, nil
	}
	index, err := a.searchIndex(ctx)
	if err != nil {
		return nil, err
	}
	return index.Search(query, defaultSearchLimit), nil
}

// searchIndex returns the cached index, rebuilding it when the store catalog
// fingerprint or the fleet-derived blueprint moved since the last build. The
// fleet has no revision of its own, so its fingerprint is computed from the
// documents the index would hold; a reload or apply is enough to refresh it.
func (a *App) searchIndex(ctx context.Context) (*search.Index, error) {
	storeFingerprint, err := a.Store.CatalogFingerprint(ctx)
	if err != nil {
		return nil, err
	}
	config := a.configDocuments()
	fingerprint := storeFingerprint + ":" + search.Fingerprint(config)
	a.searchMu.Lock()
	defer a.searchMu.Unlock()
	if a.searchIdx != nil && a.searchFingerprint == fingerprint {
		return a.searchIdx, nil
	}
	index, err := search.Build(ctx, a.Store, a.storePaths(), config)
	if err != nil {
		return nil, err
	}
	a.searchIdx = index
	a.searchFingerprint = fingerprint
	return index, nil
}

// configDocuments projects the in-memory fleet onto indexable documents: one
// per sandbox, profile and cache. Only names, labels, descriptions, the agent
// and non-secret references are indexed; no secret value ever reaches the
// index.
func (a *App) configDocuments() []search.Document {
	var docs []search.Document
	for _, s := range a.Fleet.Sandboxes() {
		name := s.Name()
		if strings.TrimSpace(name) == "" {
			name = s.Slug
		}
		refs := a.profileRefLabels(s.App.Profiles)
		refs = append(refs, a.cacheRefLabels(s.App.Caches)...)
		docs = append(docs, search.Document{
			Kind:        search.KindSandbox,
			Name:        name,
			DisplayName: s.Label(),
			Slug:        s.Slug,
			Description: s.Spec.Description,
			Agent:       s.Spec.Agent(),
			Refs:        refs,
		})
	}
	for _, p := range a.Fleet.Profiles() {
		allow, deny := p.Spec.NetworkAllow(), p.Spec.NetworkDeny()
		refs := make([]string, 0, len(allow)+len(deny))
		refs = append(refs, allow...)
		refs = append(refs, deny...)
		docs = append(docs, search.Document{
			Kind:        search.KindProfile,
			Name:        p.Label(),
			Slug:        p.Slug,
			Description: p.Spec.Description,
			Refs:        refs,
		})
	}
	for _, c := range a.Fleet.Caches() {
		label := c.Label()
		if strings.TrimSpace(label) == "" {
			label = c.Slug
		}
		docs = append(docs, search.Document{
			Kind:        search.KindCache,
			Name:        label,
			Slug:        c.Slug,
			Description: c.App.Description,
			Refs:        []string{c.App.HostPath, c.App.EffectiveTarget()},
		})
	}
	return docs
}

// profileRefLabels resolves the profile slugs referenced by a sandbox to their
// labels, adding the slug when it differs and falling back to it when the
// profile no longer exists.
func (a *App) profileRefLabels(slugs []string) []string {
	out := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		p, ok := a.Fleet.Profile(slug)
		if !ok {
			out = append(out, slug)
			continue
		}
		label := p.Label()
		out = append(out, label)
		if label != slug {
			out = append(out, slug)
		}
	}
	return out
}

// cacheRefLabels resolves the cache slugs referenced by a sandbox to their
// labels, adding the slug when it differs and falling back to it when the
// cache no longer exists.
func (a *App) cacheRefLabels(slugs []string) []string {
	out := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		c, ok := a.Fleet.Cache(slug)
		if !ok {
			out = append(out, slug)
			continue
		}
		label := c.Label()
		if strings.TrimSpace(label) == "" {
			label = slug
		}
		out = append(out, label)
		if label != slug {
			out = append(out, slug)
		}
	}
	return out
}
