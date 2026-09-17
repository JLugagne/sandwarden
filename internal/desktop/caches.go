package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
)

// ListCaches returns every shared cache definition.
func (d *Desktop) ListCaches() ([]fleet.Cache, error) {
	return d.app.ListCaches(d.root)
}

// CreateCache writes a shared cache definition.
func (d *Desktop) CreateCache(input CacheInput) (fleet.Cache, error) {
	return d.app.CreateCache(d.root, cacheInput(input))
}

// UpdateCache rewrites a shared cache definition.
func (d *Desktop) UpdateCache(slug string, input CacheInput) (fleet.Cache, error) {
	if strings.TrimSpace(slug) == "" {
		return fleet.Cache{}, errors.New("cache slug is required")
	}
	return d.app.UpdateCache(d.root, slug, cacheInput(input))
}

// DeleteCache removes a shared cache definition and detaches it everywhere.
func (d *Desktop) DeleteCache(slug string) error {
	if strings.TrimSpace(slug) == "" {
		return errors.New("cache slug is required")
	}
	return d.app.DeleteCache(d.root, slug)
}

// AttachCache declares a direct cache attachment on a sandbox.
func (d *Desktop) AttachCache(name, cacheSlug string) error {
	if strings.TrimSpace(cacheSlug) == "" {
		return errors.New("cache slug is required")
	}
	return d.app.AssignCache(d.root, name, cacheSlug)
}

// DetachCache revokes a direct cache attachment from a sandbox.
func (d *Desktop) DetachCache(name, cacheSlug string) error {
	if strings.TrimSpace(cacheSlug) == "" {
		return errors.New("cache slug is required")
	}
	return d.app.UnassignCache(d.root, name, cacheSlug)
}

// ReapplyCaches re-attaches every desired cache; failures are reported per
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

func cacheInput(input CacheInput) app.CacheInput {
	return app.CacheInput{
		Name:        input.Name,
		Description: input.Description,
		HostPath:    input.HostPath,
		TargetPath:  input.TargetPath,
		ReadOnly:    input.ReadOnly,
		AutoAttach:  input.AutoAttach,
		Enabled:     input.Enabled,
	}
}
