package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
)

// ListKitStores returns every registered kit repository.
func (d *Desktop) ListKitStores() ([]app.KitStoreView, error) {
	return d.app.ListKitStores(d.root)
}

// ListKitItems returns the discovered kits of one store, or all of them when
// storeSlug is empty.
func (d *Desktop) ListKitItems(storeSlug string) ([]app.KitItemView, error) {
	return d.app.ListKitItems(d.root, storeSlug)
}

// CreateKitStore registers, checks out and discovers a kit repository.
func (d *Desktop) CreateKitStore(req KitStoreRequest) (app.KitStoreView, error) {
	return d.app.CreateKitStore(d.root, storeInput(SkillStoreRequest(req)))
}

// UpdateKitStore rewrites a kit repository registration.
func (d *Desktop) UpdateKitStore(slug string, req KitStoreRequest) (app.KitStoreView, error) {
	return d.app.UpdateKitStore(d.root, slug, storeInput(SkillStoreRequest(req)))
}

// DeleteKitStore forgets a kit repository, its checkout and its catalog.
func (d *Desktop) DeleteKitStore(slug string) error {
	return d.app.DeleteKitStore(d.root, slug)
}

// RefreshKitStore re-runs the checkout and discovery.
func (d *Desktop) RefreshKitStore(slug string) (app.KitStoreView, error) {
	return d.app.RefreshKitStore(d.root, slug)
}

// KitValidate runs `sbx kit validate` over one catalog kit.
func (d *Desktop) KitValidate(storeSlug, name string) (app.KitValidation, error) {
	return d.app.KitValidate(d.root, storeSlug, name)
}

// AttachKit attaches a mixin kit to an existing sandbox: `sbx kit add`
// recreates the container while preserving kit-owned volumes and --clone
// workspaces, then the sidecar is re-applied and its report returned.
func (d *Desktop) AttachKit(name, ref string) (app.KitAddResult, error) {
	if strings.TrimSpace(name) == "" {
		return app.KitAddResult{}, errors.New("name is required")
	}
	if strings.TrimSpace(ref) == "" {
		return app.KitAddResult{}, errors.New("kit reference is required")
	}
	return d.app.AttachKit(d.root, name, ref, nil)
}
