// Package store is the derived SQLite index: the discovered skill and kit
// catalogs, per-store sync state, and the ledger of policy rules sandwarden
// applied. Sandbox, profile and cache definitions live in files, not here
// (see internal/fleet).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Store owns the SQLite index.
type Store struct {
	db *sql.DB
}

// schema is the fresh, file-driven schema. Nothing here is a source of
// truth: catalogs are rediscovered from git checkouts and the ledger only
// tracks rules sandwarden itself installed.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS skill_items (
		store       TEXT NOT NULL,
		kind        TEXT NOT NULL CHECK (kind IN ('skill','command')),
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		plugin      TEXT NOT NULL DEFAULT '',
		rel_path    TEXT NOT NULL,
		PRIMARY KEY (store, kind, name)
	)`,
	`CREATE TABLE IF NOT EXISTS kit_items (
		store          TEXT NOT NULL,
		kind           TEXT NOT NULL,
		name           TEXT NOT NULL,
		display_name   TEXT NOT NULL DEFAULT '',
		description    TEXT NOT NULL DEFAULT '',
		version        TEXT NOT NULL DEFAULT '',
		image          TEXT NOT NULL DEFAULT '',
		requires_agent TEXT NOT NULL DEFAULT '',
		rel_path       TEXT NOT NULL,
		spec           TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (store, name)
	)`,
	`CREATE TABLE IF NOT EXISTS store_state (
		kind      TEXT NOT NULL,
		store     TEXT NOT NULL,
		synced_at TEXT NOT NULL DEFAULT '',
		error     TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (kind, store)
	)`,
	`CREATE TABLE IF NOT EXISTS applied_rules (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		profile    TEXT NOT NULL,
		sandbox    TEXT NOT NULL DEFAULT '',
		rule_id    TEXT NOT NULL,
		pattern    TEXT NOT NULL,
		decision   TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_applied_rules_target ON applied_rules (profile, sandbox)`,
	`CREATE INDEX IF NOT EXISTS idx_applied_rules_rule ON applied_rules (rule_id)`,
}

// legacyTables belonged to the pre-fleet schema; they are dropped on open
// because files are now the source of truth (no data migration by design).
var legacyTables = []string{
	"profiles",
	"profile_rules",
	"sandbox_profiles",
	"applied_rules_v2",
	"sandbox_default_optout",
	"cache_mounts",
	"sandbox_caches",
	"skill_stores",
	"skill_items",
	"profile_skill_items",
	"sandbox_skill_items",
	"kit_stores",
	"kit_items",
	"profile_mounts",
	"profile_caches",
	"sandbox_profile_optouts",
	"sandbox_run_args",
}

// Open opens (or creates) the SQLite index at path.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	st := &Store{db: db}
	if err := st.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) init(ctx context.Context) error {
	// Legacy tables carry foreign keys between them; enforcement must be off
	// for the drops to succeed regardless of order.
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	for _, table := range legacyTables {
		if _, err := s.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			return fmt.Errorf("drop legacy table %s: %w", table, err)
		}
	}
	if err := s.dropLegacyAppliedRules(ctx); err != nil {
		return fmt.Errorf("reset legacy ledger: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	for _, stmt := range schema {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// now is the RFC3339 UTC timestamp stored in every table.
func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// dropLegacyAppliedRules removes an applied_rules table left by the pre-fleet
// schema, whose profile_id/sandbox_name columns no longer match the code.
// Unlike the rediscoverable catalog tables it is not dropped unconditionally:
// a ledger already in the current shape cannot be rebuilt from files and must
// survive every reopen. The new table is created by the schema pass.
func (s *Store) dropLegacyAppliedRules(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(applied_rules)`)
	if err != nil {
		return err
	}
	var columns []string
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notNull int
			dflt    any
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		columns = append(columns, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(columns) == 0 {
		return nil
	}
	for _, name := range columns {
		if name == "sandbox" {
			return nil
		}
	}
	_, err = s.db.ExecContext(ctx, "DROP TABLE applied_rules")
	return err
}
