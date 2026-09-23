package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
)

// ListProfiles returns every profile with its rules and sandbox assignments.
func (d *Desktop) ListProfiles() ([]app.ProfileView, error) {
	return d.app.ListProfiles(d.root)
}

// CreateProfile writes a new profile and returns its view.
func (d *Desktop) CreateProfile(req ProfileRequest) (app.ProfileView, error) {
	if strings.TrimSpace(req.Name) == "" {
		return app.ProfileView{}, errNameRequired
	}
	return d.app.CreateProfile(d.root, req.Name, req.Description, req.IsDefault, req.IsGlobal)
}

// UpdateProfile rewrites a profile and returns its view.
func (d *Desktop) UpdateProfile(slug string, req ProfileRequest) (app.ProfileView, error) {
	if strings.TrimSpace(req.Name) == "" {
		return app.ProfileView{}, errNameRequired
	}
	if err := d.app.UpdateProfile(d.root, slug, req.Name, req.Description, req.IsDefault, req.IsGlobal); err != nil {
		return app.ProfileView{}, err
	}
	return d.app.GetProfileView(d.root, slug)
}

// DeleteProfile removes a profile file and everything it applied.
func (d *Desktop) DeleteProfile(slug string) error {
	return d.app.DeleteProfile(d.root, slug)
}

// AddRule appends an allow/deny pattern to a profile.
func (d *Desktop) AddRule(slug string, req RuleRequest) error {
	return d.app.AddRuleToProfile(d.root, slug, req.Decision, req.Pattern)
}

// AddRules appends several allow/deny patterns to a profile in one pass, for
// the Traffic page's bulk actions.
func (d *Desktop) AddRules(slug string, req RulesRequest) error {
	return d.app.AddRulesToProfile(d.root, slug, req.Decision, req.Patterns)
}

// RemoveRule drops an allow/deny pattern from a profile.
func (d *Desktop) RemoveRule(slug string, req RuleRequest) error {
	return d.app.RemoveRuleFromProfile(d.root, slug, req.Decision, req.Pattern)
}

var errNameRequired = errors.New("name is required")
