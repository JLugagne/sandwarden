package desktop

import (
	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ListSkillStores returns every registered skill store.
func (d *Desktop) ListSkillStores() ([]app.SkillStoreView, error) {
	return d.app.ListSkillStores(d.root)
}

// ListSkillItems returns the discovered items of one store, or all of them
// when storeSlug is empty.
func (d *Desktop) ListSkillItems(storeSlug string) ([]store.SkillItem, error) {
	return d.app.ListSkillItems(d.root, storeSlug)
}

// CreateSkillStore registers, checks out and discovers a skill store.
func (d *Desktop) CreateSkillStore(req SkillStoreRequest) (app.SkillStoreView, error) {
	return d.app.CreateSkillStore(d.root, storeInput(req))
}

// UpdateSkillStore rewrites a skill store registration.
func (d *Desktop) UpdateSkillStore(slug string, req SkillStoreRequest) (app.SkillStoreView, error) {
	return d.app.UpdateSkillStore(d.root, slug, storeInput(req))
}

// DeleteSkillStore forgets a skill store, its checkout and its catalog.
func (d *Desktop) DeleteSkillStore(slug string) error {
	return d.app.DeleteSkillStore(d.root, slug)
}

// RefreshSkillStore re-runs the checkout and discovery.
func (d *Desktop) RefreshSkillStore(slug string) (app.SkillStoreView, error) {
	return d.app.RefreshSkillStore(d.root, slug)
}

// AddProfileSkillItem selects a catalog item on a profile.
func (d *Desktop) AddProfileSkillItem(profileSlug string, ref SkillRefRequest) error {
	return d.app.AddSkillItemToProfile(d.root, profileSlug, ref.SkillRefToFleet())
}

// RemoveProfileSkillItem drops a selection from a profile.
func (d *Desktop) RemoveProfileSkillItem(profileSlug string, ref SkillRefRequest) error {
	return d.app.RemoveSkillItemFromProfile(d.root, profileSlug, ref.SkillRefToFleet())
}

// AttachSkillItem selects a catalog item directly on a sandbox.
func (d *Desktop) AttachSkillItem(name string, ref SkillRefRequest) error {
	return d.app.AttachSkillItem(d.root, name, ref.SkillRefToFleet())
}

// DetachSkillItem drops a direct selection from a sandbox.
func (d *Desktop) DetachSkillItem(name string, ref SkillRefRequest) error {
	return d.app.DetachSkillItem(d.root, name, ref.SkillRefToFleet())
}

// ReconcileSkills converges the sandbox's .agents mounts.
func (d *Desktop) ReconcileSkills(name string) (app.SkillReconcileResult, error) {
	return d.app.ReconcileSkills(d.root, name)
}

func storeInput(req SkillStoreRequest) app.StoreInput {
	return app.StoreInput{
		Name:        req.Name,
		Description: req.Description,
		URL:         req.URL,
		Ref:         req.Ref,
		Auth:        req.Auth,
	}
}
