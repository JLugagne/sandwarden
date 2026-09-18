package app

import (
	"context"
	"io"
	"strings"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ListTemplates returns the template images of the local sandbox runtime.
func (a *App) ListTemplates(ctx context.Context) ([]sbx.Template, error) {
	return a.Sbx.ListTemplates(ctx)
}

// RemoveTemplate deletes a template image by tag or ID.
func (a *App) RemoveTemplate(ctx context.Context, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	return a.Sbx.RemoveTemplate(ctx, ref)
}

// SaveTemplate snapshots a sandbox into a template image tagged tag, so it
// can later be picked as the base of a new sandbox instead of re-baking its
// kits. Combined sbx output is streamed to w when non-nil.
func (a *App) SaveTemplate(ctx context.Context, sandbox, tag string, w io.Writer) (string, error) {
	return a.Sbx.SaveTemplate(ctx, sandbox, tag, w)
}
