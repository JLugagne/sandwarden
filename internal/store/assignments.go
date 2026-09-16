package store

import (
	"context"
	"errors"
	"strings"
)

// AssignProfile links a profile to a sandbox (many-to-many). Idempotent.
func (s *Store) AssignProfile(ctx context.Context, sandbox string, profileID int64) error {
	sandbox = strings.TrimSpace(sandbox)
	if sandbox == "" {
		return errors.New("sandbox name is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO sandbox_profiles (sandbox_name, profile_id, applied_at) VALUES (?, ?, ?)`,
		sandbox, profileID, now())
	return err
}

// UnassignProfile removes a profile from a sandbox.
func (s *Store) UnassignProfile(ctx context.Context, sandbox string, profileID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sandbox_profiles WHERE sandbox_name = ? AND profile_id = ?`, sandbox, profileID)
	return err
}

// ListProfilesForSandbox returns the profiles assigned to a sandbox.
func (s *Store) ListProfilesForSandbox(ctx context.Context, sandbox string) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+prefixedCols+` FROM profiles p
		 JOIN sandbox_profiles sp ON sp.profile_id = p.id
		 WHERE sp.sandbox_name = ? ORDER BY p.name`, sandbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SandboxesForProfile returns the sandboxes a profile is assigned to.
func (s *Store) SandboxesForProfile(ctx context.Context, profileID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sandbox_name FROM sandbox_profiles WHERE profile_id = ? ORDER BY sandbox_name`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// AllAssignments returns sandbox -> profile ids for every assignment.
func (s *Store) AllAssignments(ctx context.Context) (map[string][]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sandbox_name, profile_id FROM sandbox_profiles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]int64)
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return nil, err
		}
		out[name] = append(out[name], id)
	}
	return out, rows.Err()
}

// DropSandboxAssignments removes every assignment and default-optout row for a
// sandbox (used when the sandbox is deleted).
func (s *Store) DropSandboxAssignments(ctx context.Context, sandbox string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_profiles WHERE sandbox_name = ?`, sandbox); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_default_optout WHERE sandbox_name = ?`, sandbox)
	return err
}

// OptOutDefault records that a sandbox should not receive the default profile.
func (s *Store) OptOutDefault(ctx context.Context, sandbox string) error {
	sandbox = strings.TrimSpace(sandbox)
	if sandbox == "" {
		return errors.New("sandbox name is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO sandbox_default_optout (sandbox_name, opted_out_at) VALUES (?, ?)`,
		sandbox, now())
	return err
}

// ClearOptOut removes a sandbox's default-profile opt-out.
func (s *Store) ClearOptOut(ctx context.Context, sandbox string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_default_optout WHERE sandbox_name = ?`, sandbox)
	return err
}

// OptedOutSandboxes returns the set of sandboxes that explicitly opted out of
// the default profile, so auto-apply will not re-add it.
func (s *Store) OptedOutSandboxes(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sandbox_name FROM sandbox_default_optout`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}
