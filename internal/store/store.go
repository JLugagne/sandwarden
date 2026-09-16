package store

import (
	"database/sql"

	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// migrations are applied in order; user_version tracks the count applied.
var migrations = []string{
	`CREATE TABLE profiles (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '',
		is_default  INTEGER NOT NULL DEFAULT 0,
		is_global   INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);`,
	`CREATE TABLE profile_rules (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		decision   TEXT NOT NULL CHECK (decision IN ('allow','deny')),
		pattern    TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE (profile_id, decision, pattern)
	);`,
	`CREATE TABLE sandbox_profiles (
		sandbox_name TEXT NOT NULL,
		profile_id   INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		applied_at   TEXT NOT NULL,
		PRIMARY KEY (sandbox_name, profile_id)
	);`,
	`CREATE TABLE applied_rules (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		profile_id   INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		sandbox_name TEXT NOT NULL DEFAULT '',
		rule_id      TEXT NOT NULL,
		pattern      TEXT NOT NULL,
		decision     TEXT NOT NULL,
		created_at   TEXT NOT NULL
	);`,
	`CREATE INDEX idx_applied_rules_target ON applied_rules (profile_id, sandbox_name);`,
	`CREATE INDEX idx_applied_rules_rule ON applied_rules (rule_id);`,
	`CREATE TABLE sandbox_default_optout (
		sandbox_name TEXT PRIMARY KEY,
		opted_out_at TEXT NOT NULL
	);`,
	`CREATE TABLE cache_mounts (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '',
		host_path   TEXT NOT NULL,
		target_path TEXT NOT NULL,
		read_only   INTEGER NOT NULL DEFAULT 0,
		auto_attach INTEGER NOT NULL DEFAULT 1,
		enabled     INTEGER NOT NULL DEFAULT 1,
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);`,
	`CREATE TABLE sandbox_caches (
		sandbox_name TEXT NOT NULL,
		cache_id     INTEGER NOT NULL REFERENCES cache_mounts(id) ON DELETE CASCADE,
		attached_at  TEXT NOT NULL,
		PRIMARY KEY (sandbox_name, cache_id)
	);`,
	`CREATE TABLE skill_stores (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '',
		url         TEXT NOT NULL,
		ref         TEXT NOT NULL DEFAULT '',
		path        TEXT NOT NULL,
		synced_at   TEXT NOT NULL DEFAULT '',
		error       TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);`,
	`CREATE TABLE skill_items (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		store_id    INTEGER NOT NULL REFERENCES skill_stores(id) ON DELETE CASCADE,
		kind        TEXT NOT NULL CHECK (kind IN ('skill','command')),
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		plugin      TEXT NOT NULL DEFAULT '',
		rel_path    TEXT NOT NULL,
		created_at  TEXT NOT NULL,
		UNIQUE (store_id, kind, name)
	);`,
	`CREATE INDEX idx_skill_items_store ON skill_items (store_id);`,
	`CREATE TABLE profile_skill_items (
		profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		item_id    INTEGER NOT NULL REFERENCES skill_items(id) ON DELETE CASCADE,
		added_at   TEXT NOT NULL,
		PRIMARY KEY (profile_id, item_id)
	);`,
	`CREATE INDEX idx_profile_skill_items_item ON profile_skill_items (item_id);`,
	`CREATE TABLE sandbox_skill_items (
		sandbox_name TEXT NOT NULL,
		item_id      INTEGER NOT NULL REFERENCES skill_items(id) ON DELETE CASCADE,
		added_at     TEXT NOT NULL,
		PRIMARY KEY (sandbox_name, item_id)
	);`,
	`CREATE INDEX idx_sandbox_skill_items_item ON sandbox_skill_items (item_id);`,
	`ALTER TABLE skill_stores ADD COLUMN auth TEXT NOT NULL DEFAULT '';`,
	`CREATE TABLE kit_stores (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '',
		url         TEXT NOT NULL,
		ref         TEXT NOT NULL DEFAULT '',
		auth        TEXT NOT NULL DEFAULT '',
		path        TEXT NOT NULL,
		synced_at   TEXT NOT NULL DEFAULT '',
		error       TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);`,
	`CREATE TABLE kit_items (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		store_id       INTEGER NOT NULL REFERENCES kit_stores(id) ON DELETE CASCADE,
		kind           TEXT NOT NULL,
		name           TEXT NOT NULL,
		display_name   TEXT NOT NULL DEFAULT '',
		description    TEXT NOT NULL DEFAULT '',
		version        TEXT NOT NULL DEFAULT '',
		image          TEXT NOT NULL DEFAULT '',
		requires_agent TEXT NOT NULL DEFAULT '',
		rel_path       TEXT NOT NULL,
		spec           TEXT NOT NULL DEFAULT '',
		created_at     TEXT NOT NULL,
		UNIQUE (store_id, name)
	);`,
	`CREATE INDEX idx_kit_items_store ON kit_items (store_id);`,
	`CREATE TABLE profile_mounts (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		profile_id  INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		host_path   TEXT NOT NULL,
		target_path TEXT NOT NULL DEFAULT '',
		read_only   INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL,
		UNIQUE (profile_id, host_path, target_path)
	);`,
	`CREATE INDEX idx_profile_mounts_profile ON profile_mounts (profile_id);`,
	`CREATE TABLE profile_caches (
		profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		cache_id   INTEGER NOT NULL REFERENCES cache_mounts(id) ON DELETE CASCADE,
		added_at   TEXT NOT NULL,
		PRIMARY KEY (profile_id, cache_id)
	);`,
	`CREATE TABLE sandbox_profile_optouts (
		sandbox_name TEXT NOT NULL,
		kind         TEXT NOT NULL CHECK (kind IN ('mount','cache')),
		ref_id       INTEGER NOT NULL,
		opted_out_at TEXT NOT NULL,
		PRIMARY KEY (sandbox_name, kind, ref_id)
	);`,
}

// Store is the SQLite-backed profile store.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies
// migrations.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		if _, err := s.db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }
