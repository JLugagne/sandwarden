package desktop

import (
	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ListKitStores returns every registered kit repository.
func (d *Desktop) ListKitStores() ([]store.KitStore, error) {
	return d.app.ListKitStores(d.root)
}

// ListKitItems returns the discovered kits of one repository, or of every
// repository when storeID is zero.
func (d *Desktop) ListKitItems(storeID int64) ([]app.KitItemView, error) {
	return d.app.ListKitItems(d.root, storeID)
}

// CreateKitStore registers a kit repository and performs its first checkout.
func (d *Desktop) CreateKitStore(req KitStoreRequest) (store.KitStore, error) {
	return d.app.CreateKitStore(d.root, store.KitStore{
		Name:        req.Name,
		Description: req.Description,
		URL:         req.URL,
		Ref:         req.Ref,
		Auth:        req.Auth,
	})
}

// UpdateKitStore rewrites a registration and re-checks out its source when the
// url or ref changed.
func (d *Desktop) UpdateKitStore(id int64, req KitStoreRequest) (store.KitStore, error) {
	return d.app.UpdateKitStore(d.root, id, store.KitStore{
		Name:        req.Name,
		Description: req.Description,
		URL:         req.URL,
		Ref:         req.Ref,
		Auth:        req.Auth,
	})
}

// DeleteKitStore removes a repository, its checkout and its catalog.
func (d *Desktop) DeleteKitStore(id int64) error {
	return d.app.DeleteKitStore(d.root, id)
}

// RefreshKitStore re-checks out a repository and rebuilds its catalog.
func (d *Desktop) RefreshKitStore(id int64) (store.KitStore, error) {
	return d.app.RefreshKitStore(d.root, id)
}

// KitValidate runs `sbx kit validate` on one catalog kit.
func (d *Desktop) KitValidate(itemID int64) (app.KitValidation, error) {
	return d.app.KitValidate(d.root, itemID)
}
