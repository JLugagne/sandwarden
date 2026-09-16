package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ListProfiles returns every profile with its rules and sandbox assignments.
func (d *Desktop) ListProfiles() ([]app.ProfileView, error) {
	return d.app.ListProfiles(d.root)
}

// CreateProfile stores a new profile and returns its view.
func (d *Desktop) CreateProfile(req ProfileRequest) (app.ProfileView, error) {
	if strings.TrimSpace(req.Name) == "" {
		return app.ProfileView{}, errNameRequired
	}
	profile, err := d.app.CreateProfile(d.root, req.Name, req.Description, req.IsDefault, req.IsGlobal)
	if err != nil {
		return app.ProfileView{}, err
	}
	return d.app.GetProfileView(d.root, profile.ID)
}

// UpdateProfile rewrites a profile and returns its view.
func (d *Desktop) UpdateProfile(id int64, req ProfileRequest) (app.ProfileView, error) {
	if strings.TrimSpace(req.Name) == "" {
		return app.ProfileView{}, errNameRequired
	}
	if err := d.app.UpdateProfile(d.root, id, req.Name, req.Description, req.IsDefault, req.IsGlobal); err != nil {
		return app.ProfileView{}, err
	}
	return d.app.GetProfileView(d.root, id)
}

// DeleteProfile removes a profile and its rules.
func (d *Desktop) DeleteProfile(id int64) error {
	return d.app.DeleteProfile(d.root, id)
}

// AddRule appends an allow/deny pattern to a profile.
func (d *Desktop) AddRule(profileID int64, req RuleRequest) (store.Rule, error) {
	return d.app.AddRuleToProfile(d.root, profileID, req.Decision, req.Pattern)
}

// RemoveRule deletes one rule from a profile.
func (d *Desktop) RemoveRule(profileID, ruleID int64) error {
	return d.app.RemoveRuleFromProfile(d.root, profileID, ruleID)
}

var errNameRequired = errors.New("name is required")
