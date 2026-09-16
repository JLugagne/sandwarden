package desktop

import "github.com/JLugagne/sandwarden/internal/sbx"

// ListTemplates returns the template images of the local sandbox runtime.
func (d *Desktop) ListTemplates() ([]sbx.Template, error) {
	return d.app.ListTemplates(d.root)
}

// RemoveTemplate deletes a template image by tag or ID.
func (d *Desktop) RemoveTemplate(ref string) error {
	return d.app.RemoveTemplate(d.root, ref)
}
