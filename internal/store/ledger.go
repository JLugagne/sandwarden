package store

import (
	"context"
	"database/sql"
)

// The ledger is read-modify-write state: a convergence pass lists a target's
// rows, installs the missing rules on the daemon and records them. Callers run
// the whole cycle under the fleet lock (internal/fleet/lock.go) so a concurrent
// GUI or CLI process sees the first pass's rows and does not duplicate them;
// the helpers here stay lock-free.
//
// AppliedRule is one policy rule the app installed on a profile's behalf. The
// ledger is what lets sandwarden remove exactly its own rules and never
// hand-made ones; it is the only runtime state that cannot be derived from
// the files.
type AppliedRule struct {
	ID        int64  `json:"id"`
	Profile   string `json:"profile"`
	Sandbox   string `json:"sandbox"`
	RuleID    string `json:"rule_id"`
	Pattern   string `json:"pattern"`
	Decision  string `json:"decision"`
	CreatedAt string `json:"created_at"`
}

const appliedRuleCols = `id, profile, sandbox, rule_id, pattern, decision, created_at`

// RecordAppliedRule appends one applied rule to the ledger.
func (s *Store) RecordAppliedRule(ctx context.Context, profile, sandbox, ruleID, pattern, decision string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO applied_rules (profile, sandbox, rule_id, pattern, decision, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		profile, sandbox, ruleID, pattern, decision, now())
	return err
}

// ListAppliedRules returns the ledger rows for one profile and sandbox.
func (s *Store) ListAppliedRules(ctx context.Context, profile, sandbox string) ([]AppliedRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+appliedRuleCols+` FROM applied_rules WHERE profile = ? AND sandbox = ? ORDER BY id`, profile, sandbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAppliedRules(rows)
}

// DeleteAppliedRule removes one ledger row.
func (s *Store) DeleteAppliedRule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM applied_rules WHERE id = ?`, id)
	return err
}

// ClearAppliedForTarget drops every ledger row of one profile and sandbox.
func (s *Store) ClearAppliedForTarget(ctx context.Context, profile, sandbox string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM applied_rules WHERE profile = ? AND sandbox = ?`, profile, sandbox)
	return err
}

// ClearAppliedForProfile drops every ledger row of one profile.
func (s *Store) ClearAppliedForProfile(ctx context.Context, profile string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM applied_rules WHERE profile = ?`, profile)
	return err
}

func scanAppliedRules(rows *sql.Rows) ([]AppliedRule, error) {
	rules := []AppliedRule{}
	for rows.Next() {
		var r AppliedRule
		if err := rows.Scan(&r.ID, &r.Profile, &r.Sandbox, &r.RuleID, &r.Pattern, &r.Decision, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// ListAllAppliedRules returns every ledger row.
func (s *Store) ListAllAppliedRules(ctx context.Context) ([]AppliedRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+appliedRuleCols+` FROM applied_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAppliedRules(rows)
}

// ClearAppliedForSandbox drops every ledger row of one sandbox.
func (s *Store) ClearAppliedForSandbox(ctx context.Context, sandbox string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM applied_rules WHERE sandbox = ?`, sandbox)
	return err
}
