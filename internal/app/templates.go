package app

import (
	"context"
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
