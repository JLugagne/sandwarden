package app

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/JLugagne/sandwarden/internal/search"
)

// minQueryRunes is the shortest query the catalog search answers.
const minQueryRunes = 2

// defaultSearchLimit caps the number of ranked results returned to the UI.
const defaultSearchLimit = 25

// Search ranks the skill, command and kit catalogs against a free-text query.
// The index is built lazily on first use and kept until the underlying
// catalog changes.
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

// searchIndex returns the cached index, rebuilding it when the catalog
// fingerprint moved since the last build.
func (a *App) searchIndex(ctx context.Context) (*search.Index, error) {
	fingerprint, err := a.Store.CatalogFingerprint(ctx)
	if err != nil {
		return nil, err
	}
	a.searchMu.Lock()
	defer a.searchMu.Unlock()
	if a.searchIdx != nil && a.searchFingerprint == fingerprint {
		return a.searchIdx, nil
	}
	index, err := search.Build(ctx, a.Store)
	if err != nil {
		return nil, err
	}
	a.searchIdx = index
	a.searchFingerprint = fingerprint
	return index, nil
}
