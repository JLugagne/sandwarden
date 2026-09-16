package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Rule is one allow/deny pattern belonging to a profile.
type Rule struct {
	ID        int64  `json:"id"`
	ProfileID int64  `json:"profile_id"`
	Decision  string `json:"decision"`
	Pattern   string `json:"pattern"`
	CreatedAt string `json:"created_at"`
}

// NormalizePattern trims surrounding space from a user-supplied URL/domain pattern.
func NormalizePattern(pattern string) string {
	return strings.TrimSpace(pattern)
}

// ListRules returns a profile's rules ordered by decision then pattern.
func (s *Store) ListRules(ctx context.Context, profileID int64) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, profile_id, decision, pattern, created_at FROM profile_rules WHERE profile_id = ? ORDER BY decision, pattern`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

// ListAllRules returns every profile's rules grouped by profile id.
func (s *Store) ListAllRules(ctx context.Context) (map[int64][]Rule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, profile_id, decision, pattern, created_at FROM profile_rules ORDER BY profile_id, decision, pattern`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanRules(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]Rule)
	for _, r := range all {
		out[r.ProfileID] = append(out[r.ProfileID], r)
	}
	return out, nil
}

func scanRules(rows *sql.Rows) ([]Rule, error) {
	var out []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.ProfileID, &r.Decision, &r.Pattern, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddRule adds a pattern to a profile. It is idempotent: re-adding the same
// decision/pattern returns the existing rule unchanged.
func (s *Store) AddRule(ctx context.Context, profileID int64, decision, pattern string) (Rule, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != "allow" && decision != "deny" {
		return Rule{}, fmt.Errorf("invalid decision %q", decision)
	}
	pattern = NormalizePattern(pattern)
	if pattern == "" {
		return Rule{}, errors.New("pattern is required")
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO profile_rules (profile_id, decision, pattern, created_at) VALUES (?, ?, ?, ?)`,
		profileID, decision, pattern, now()); err != nil {
		return Rule{}, err
	}
	var r Rule
	err := s.db.QueryRowContext(ctx,
		`SELECT id, profile_id, decision, pattern, created_at FROM profile_rules WHERE profile_id = ? AND decision = ? AND pattern = ?`,
		profileID, decision, pattern).Scan(&r.ID, &r.ProfileID, &r.Decision, &r.Pattern, &r.CreatedAt)
	return r, err
}

// RemoveRule deletes a rule by id.
func (s *Store) RemoveRule(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM profile_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveRuleByPattern deletes a profile's matching rule.
func (s *Store) RemoveRuleByPattern(ctx context.Context, profileID int64, decision, pattern string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM profile_rules WHERE profile_id = ? AND decision = ? AND pattern = ?`,
		profileID, strings.ToLower(strings.TrimSpace(decision)), NormalizePattern(pattern))
	return err
}
