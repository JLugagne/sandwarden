package desktop

import "github.com/JLugagne/sandwarden/internal/search"

// Search ranks the skill, command and kit catalogs against a free-text query.
// Results are best-first and capped server side.
func (d *Desktop) Search(query string) ([]search.Result, error) {
	return d.app.Search(d.root, query)
}
