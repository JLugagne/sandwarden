package store

import (
	"context"
	"database/sql"
)

// AppliedRule records an sbx policy rule that this app created on a profile's
// behalf, so it can be removed precisely without touching hand-made rules.
// SandboxName is "" for globally applied rules.
type AppliedRule struct {
	ID          int64
	ProfileID   int64
	SandboxName string
	RuleID      string
	Pattern     string
	Decision    string
	CreatedAt   string
}

// RecordAppliedRule stores the sbx rule id returned by a policy mutation.
func (s *Store) RecordAppliedRule(ctx context.Context, profileID int64, sandbox, ruleID, pattern, decision string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO applied_rules (profile_id, sandbox_name, rule_id, pattern, decision, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		profileID, sandbox, ruleID, pattern, decision, now())
	return err
}

// ListAppliedRules returns the ledger rows for a profile and target ("" = global).
func (s *Store) ListAppliedRules(ctx context.Context, profileID int64, sandbox string) ([]AppliedRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, profile_id, sandbox_name, rule_id, pattern, decision, created_at
		 FROM applied_rules WHERE profile_id = ? AND sandbox_name = ? ORDER BY pattern`,
		profileID, sandbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApplied(rows)
}

// ListAllAppliedRules returns the whole ledger.
func (s *Store) ListAllAppliedRules(ctx context.Context) ([]AppliedRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, profile_id, sandbox_name, rule_id, pattern, decision, created_at
		 FROM applied_rules ORDER BY profile_id, sandbox_name, pattern`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApplied(rows)
}

func scanApplied(rows *sql.Rows) ([]AppliedRule, error) {
	var out []AppliedRule
	for rows.Next() {
		var a AppliedRule
		if err := rows.Scan(&a.ID, &a.ProfileID, &a.SandboxName, &a.RuleID, &a.Pattern, &a.Decision, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAppliedRule removes one ledger row by id.
func (s *Store) DeleteAppliedRule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM applied_rules WHERE id = ?`, id)
	return err
}

// ClearAppliedForTarget drops every ledger row for a profile/target pair.
func (s *Store) ClearAppliedForTarget(ctx context.Context, profileID int64, sandbox string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM applied_rules WHERE profile_id = ? AND sandbox_name = ?`, profileID, sandbox)
	return err
}

// OwnedRuleIDs returns the set of sbx rule ids managed by this app.
func (s *Store) OwnedRuleIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rule_id FROM applied_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
