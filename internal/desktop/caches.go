package desktop

import (
	"errors"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ListCaches returns every shared cache mount.
func (d *Desktop) ListCaches() ([]store.CacheMount, error) {
	return d.app.ListCaches(d.root)
}

// CreateCache stores a shared cache mount.
func (d *Desktop) CreateCache(input CacheInput) (store.CacheMount, error) {
	return d.app.CreateCache(d.root, cacheFromInput(input, 0))
}

// UpdateCache rewrites a shared cache mount.
func (d *Desktop) UpdateCache(id int64, input CacheInput) (store.CacheMount, error) {
	return d.app.UpdateCache(d.root, cacheFromInput(input, id))
}

// DeleteCache removes a shared cache mount.
func (d *Desktop) DeleteCache(id int64) error {
	return d.app.DeleteCache(d.root, id)
}

// AttachCache binds a shared cache to a sandbox.
func (d *Desktop) AttachCache(name string, cacheID int64) error {
	if cacheID == 0 {
		return errors.New("cache_id is required")
	}
	return d.app.AssignCache(d.root, name, cacheID)
}

// DetachCache unbinds a shared cache from a sandbox.
func (d *Desktop) DetachCache(name string, cacheID int64) error {
	return d.app.UnassignCache(d.root, name, cacheID)
}

// ReapplyCaches re-attaches every assigned cache; failures are reported per
// cache instead of aborting the batch.
func (d *Desktop) ReapplyCaches(name string) ReapplyResult {
	applied, errs := d.app.ReapplyCaches(d.root, name)
	return ReapplyResult{Applied: applied, Errors: errs}
}

// Job returns the buffered view of a streamed job.
func (d *Desktop) Job(id string) (app.JobView, error) {
	view, ok := d.app.Jobs.Get(id)
	if !ok {
		return app.JobView{}, errors.New("job not found")
	}
	return view, nil
}

// CancelJob stops a running job.
func (d *Desktop) CancelJob(id string) {
	d.app.Jobs.Cancel(id)
}
