package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ListTemplates returns the template images of the local sandbox runtime.
func (d *Desktop) ListTemplates() ([]sbx.Template, error) {
	return d.app.ListTemplates(d.root)
}

// RemoveTemplate deletes a template image by tag or ID.
func (d *Desktop) RemoveTemplate(ref string) error {
	return d.app.RemoveTemplate(d.root, ref)
}

// SaveTemplate snapshots a sandbox into a template image tagged tag, so it
// can be picked as the --template base of a future create instead of
// re-baking its kits.
func (d *Desktop) SaveTemplate(sandbox, tag string) (string, error) {
	if strings.TrimSpace(sandbox) == "" {
		return "", errors.New("sandbox name is required")
	}
	if strings.TrimSpace(tag) == "" {
		return "", errors.New("template tag is required")
	}
	return d.app.SaveTemplate(d.root, sandbox, tag, nil)
}
