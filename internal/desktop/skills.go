package desktop

import (
	"errors"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ListSkillStores returns every registered skill store.
func (d *Desktop) ListSkillStores() ([]store.SkillStore, error) {
	return d.app.ListSkillStores(d.root)
}

// ListSkillItems returns the discovered catalog of one store, or every store
// when storeID is zero.
func (d *Desktop) ListSkillItems(storeID int64) ([]store.SkillItem, error) {
	return d.app.ListSkillItems(d.root, storeID)
}

// CreateSkillStore registers a skill store and performs its first checkout.
func (d *Desktop) CreateSkillStore(req SkillStoreRequest) (store.SkillStore, error) {
	return d.app.CreateSkillStore(d.root, req.Name, req.Description, req.URL, req.Ref)
}

// UpdateSkillStore rewrites a skill store and refreshes it when its source
// changed.
func (d *Desktop) UpdateSkillStore(id int64, req SkillStoreRequest) (store.SkillStore, error) {
	if id == 0 {
		return store.SkillStore{}, errors.New("store id is required")
	}
	return d.app.UpdateSkillStore(d.root, id, req.Name, req.Description, req.URL, req.Ref)
}

// DeleteSkillStore removes a skill store and its selections.
func (d *Desktop) DeleteSkillStore(id int64) error {
	if id == 0 {
		return errors.New("store id is required")
	}
	return d.app.DeleteSkillStore(d.root, id)
}

// RefreshSkillStore re-checks out a store and rebuilds its catalog.
func (d *Desktop) RefreshSkillStore(id int64) (store.SkillStore, error) {
	if id == 0 {
		return store.SkillStore{}, errors.New("store id is required")
	}
	return d.app.RefreshSkillStore(d.root, id)
}

// AddProfileSkillItem selects a catalog item for a profile.
func (d *Desktop) AddProfileSkillItem(profileID, itemID int64) error {
	if profileID == 0 || itemID == 0 {
		return errors.New("profile_id and item_id are required")
	}
	return d.app.AddSkillItemToProfile(d.root, profileID, itemID)
}

// RemoveProfileSkillItem deselects a catalog item from a profile.
func (d *Desktop) RemoveProfileSkillItem(profileID, itemID int64) error {
	if profileID == 0 || itemID == 0 {
		return errors.New("profile_id and item_id are required")
	}
	return d.app.RemoveSkillItemFromProfile(d.root, profileID, itemID)
}

// AttachSkillItem adds a catalog item directly to one sandbox.
func (d *Desktop) AttachSkillItem(name string, itemID int64) error {
	if itemID == 0 {
		return errors.New("item_id is required")
	}
	return d.app.AttachSkillItem(d.root, name, itemID)
}

// DetachSkillItem removes a directly attached catalog item from one sandbox.
func (d *Desktop) DetachSkillItem(name string, itemID int64) error {
	if itemID == 0 {
		return errors.New("item_id is required")
	}
	return d.app.DetachSkillItem(d.root, name, itemID)
}

// ReconcileSkills mounts and unmounts the .agents items of one sandbox.
func (d *Desktop) ReconcileSkills(name string) (app.SkillReconcileResult, error) {
	return d.app.ReconcileSkills(d.root, name)
}
