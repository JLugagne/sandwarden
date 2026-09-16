package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const skillStoreCols = `id, name, description, url, ref, auth, path, synced_at, error, created_at, updated_at`

const skillItemCols = `i.id, i.store_id, s.name, i.kind, i.name, i.description, i.plugin, i.rel_path`

// SkillStore is a git checkout of an Anthropic-format plugin repository.
type SkillStore struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Ref         string `json:"ref"`
	Auth        string `json:"auth"`
	Path        string `json:"path"`
	SyncedAt    string `json:"synced_at"`
	Error       string `json:"error"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// SkillItem is one skill or command discovered in a store checkout.
type SkillItem struct {
	ID          int64  `json:"id"`
	StoreID     int64  `json:"store_id"`
	StoreName   string `json:"store_name"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Plugin      string `json:"plugin"`
	RelPath     string `json:"rel_path"`
}

func scanSkillStore(row interface{ Scan(...any) error }) (SkillStore, error) {
	var s SkillStore
	if err := row.Scan(&s.ID, &s.Name, &s.Description, &s.URL, &s.Ref, &s.Auth, &s.Path, &s.SyncedAt, &s.Error, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SkillStore{}, ErrNotFound
		}
		return SkillStore{}, err
	}
	return s, nil
}

func scanSkillItem(row interface{ Scan(...any) error }) (SkillItem, error) {
	var item SkillItem
	if err := row.Scan(&item.ID, &item.StoreID, &item.StoreName, &item.Kind, &item.Name, &item.Description, &item.Plugin, &item.RelPath); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SkillItem{}, ErrNotFound
		}
		return SkillItem{}, err
	}
	return item, nil
}

func scanSkillItems(rows *sql.Rows) ([]SkillItem, error) {
	defer rows.Close()
	var out []SkillItem
	for rows.Next() {
		item, err := scanSkillItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListSkillStores returns every registered store ordered by name.
func (s *Store) ListSkillStores(ctx context.Context) ([]SkillStore, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+skillStoreCols+` FROM skill_stores ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillStore
	for rows.Next() {
		store, err := scanSkillStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, store)
	}
	return out, rows.Err()
}

// GetSkillStore returns one store by id.
func (s *Store) GetSkillStore(ctx context.Context, id int64) (SkillStore, error) {
	return scanSkillStore(s.db.QueryRowContext(ctx, `SELECT `+skillStoreCols+` FROM skill_stores WHERE id = ?`, id))
}

// CreateSkillStore inserts a store registration.
func (s *Store) CreateSkillStore(ctx context.Context, store SkillStore) (SkillStore, error) {
	store.Name = strings.TrimSpace(store.Name)
	store.URL = strings.TrimSpace(store.URL)
	store.Path = strings.TrimSpace(store.Path)
	if store.Name == "" {
		return SkillStore{}, errors.New("skill store name is required")
	}
	if store.URL == "" {
		return SkillStore{}, errors.New("git url is required")
	}
	if store.Path == "" {
		return SkillStore{}, errors.New("checkout path is required")
	}
	ts := now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO skill_stores (name, description, url, ref, auth, path, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		store.Name, store.Description, store.URL, store.Ref, store.Auth, store.Path, ts, ts)
	if err != nil {
		return SkillStore{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SkillStore{}, err
	}
	return s.GetSkillStore(ctx, id)
}

// UpdateSkillStore rewrites a store registration. The checkout path is kept
// stable across renames so the mount targets do not move.
func (s *Store) UpdateSkillStore(ctx context.Context, store SkillStore) error {
	store.Name = strings.TrimSpace(store.Name)
	store.URL = strings.TrimSpace(store.URL)
	if store.Name == "" {
		return errors.New("skill store name is required")
	}
	if store.URL == "" {
		return errors.New("git url is required")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE skill_stores SET name = ?, description = ?, url = ?, ref = ?, auth = ?, updated_at = ? WHERE id = ?`,
		store.Name, store.Description, store.URL, store.Ref, store.Auth, now(), store.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSkillStore removes a store and, by cascade, its catalog and every
// profile or sandbox selection pointing at it.
func (s *Store) DeleteSkillStore(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM skill_stores WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkSkillStoreSynced records the outcome of a checkout refresh.
func (s *Store) MarkSkillStoreSynced(ctx context.Context, id int64, syncedAt, syncErr string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE skill_stores SET synced_at = ?, error = ?, updated_at = ? WHERE id = ?`,
		syncedAt, syncErr, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceSkillItems converges the catalog of one store onto the discovered
// items: new items are inserted, existing ones keep their id and selections,
// and items that disappeared are removed.
func (s *Store) ReplaceSkillItems(ctx context.Context, storeID int64, items []SkillItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, kind, name FROM skill_items WHERE store_id = ?`, storeID)
	if err != nil {
		return err
	}
	existing := make(map[string]int64)
	for rows.Next() {
		var id int64
		var kind, name string
		if err := rows.Scan(&id, &kind, &name); err != nil {
			rows.Close()
			return err
		}
		existing[skillItemKey(kind, name)] = id
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	keep := make(map[string]bool, len(items))
	ts := now()
	for _, item := range items {
		key := skillItemKey(item.Kind, item.Name)
		keep[key] = true
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO skill_items (store_id, kind, name, description, plugin, rel_path, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (store_id, kind, name) DO UPDATE SET
			   description = excluded.description,
			   plugin = excluded.plugin,
			   rel_path = excluded.rel_path`,
			storeID, item.Kind, item.Name, item.Description, item.Plugin, item.RelPath, ts); err != nil {
			return err
		}
	}
	for key, id := range existing {
		if keep[key] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM skill_items WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func skillItemKey(kind, name string) string { return kind + "\x00" + strings.ToLower(name) }

// ListSkillItems returns one store's catalog.
func (s *Store) ListSkillItems(ctx context.Context, storeID int64) ([]SkillItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+skillItemCols+` FROM skill_items i JOIN skill_stores s ON s.id = i.store_id
		 WHERE i.store_id = ? ORDER BY i.kind, i.name`, storeID)
	if err != nil {
		return nil, err
	}
	return scanSkillItems(rows)
}

// ListAllSkillItems returns every discovered item across stores.
func (s *Store) ListAllSkillItems(ctx context.Context) ([]SkillItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+skillItemCols+` FROM skill_items i JOIN skill_stores s ON s.id = i.store_id
		 ORDER BY s.name, i.kind, i.name`)
	if err != nil {
		return nil, err
	}
	return scanSkillItems(rows)
}

// AddProfileSkillItem selects a catalog item for a profile. Idempotent.
func (s *Store) AddProfileSkillItem(ctx context.Context, profileID, itemID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profile_skill_items (profile_id, item_id, added_at) VALUES (?, ?, ?)
		 ON CONFLICT (profile_id, item_id) DO NOTHING`, profileID, itemID, now())
	return err
}

// RemoveProfileSkillItem deselects a catalog item from a profile.
func (s *Store) RemoveProfileSkillItem(ctx context.Context, profileID, itemID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM profile_skill_items WHERE profile_id = ? AND item_id = ?`, profileID, itemID)
	return err
}

// ListProfileSkillItems returns the items selected by one profile.
func (s *Store) ListProfileSkillItems(ctx context.Context, profileID int64) ([]SkillItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+skillItemCols+` FROM skill_items i
		 JOIN skill_stores s ON s.id = i.store_id
		 JOIN profile_skill_items ps ON ps.item_id = i.id
		 WHERE ps.profile_id = ? ORDER BY s.name, i.kind, i.name`, profileID)
	if err != nil {
		return nil, err
	}
	return scanSkillItems(rows)
}

// AllProfileSkillItems returns profile id → selected items.
func (s *Store) AllProfileSkillItems(ctx context.Context) (map[int64][]SkillItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ps.profile_id, `+skillItemCols+` FROM profile_skill_items ps
		 JOIN skill_items i ON i.id = ps.item_id
		 JOIN skill_stores s ON s.id = i.store_id
		 ORDER BY ps.profile_id, s.name, i.kind, i.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]SkillItem)
	for rows.Next() {
		var profileID int64
		var item SkillItem
		if err := rows.Scan(&profileID, &item.ID, &item.StoreID, &item.StoreName, &item.Kind, &item.Name, &item.Description, &item.Plugin, &item.RelPath); err != nil {
			return nil, err
		}
		out[profileID] = append(out[profileID], item)
	}
	return out, rows.Err()
}

// AddSandboxSkillItem selects a catalog item for one sandbox. Idempotent.
func (s *Store) AddSandboxSkillItem(ctx context.Context, sandbox string, itemID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sandbox_skill_items (sandbox_name, item_id, added_at) VALUES (?, ?, ?)
		 ON CONFLICT (sandbox_name, item_id) DO NOTHING`, sandbox, itemID, now())
	return err
}

// RemoveSandboxSkillItem deselects a catalog item from one sandbox.
func (s *Store) RemoveSandboxSkillItem(ctx context.Context, sandbox string, itemID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sandbox_skill_items WHERE sandbox_name = ? AND item_id = ?`, sandbox, itemID)
	return err
}

// ListSandboxSkillItems returns the items attached directly to one sandbox.
func (s *Store) ListSandboxSkillItems(ctx context.Context, sandbox string) ([]SkillItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+skillItemCols+` FROM skill_items i
		 JOIN skill_stores s ON s.id = i.store_id
		 JOIN sandbox_skill_items si ON si.item_id = i.id
		 WHERE si.sandbox_name = ? ORDER BY s.name, i.kind, i.name`, sandbox)
	if err != nil {
		return nil, err
	}
	return scanSkillItems(rows)
}

// AllSandboxSkillItems returns sandbox name → directly attached items.
func (s *Store) AllSandboxSkillItems(ctx context.Context) (map[string][]SkillItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT si.sandbox_name, `+skillItemCols+` FROM sandbox_skill_items si
		 JOIN skill_items i ON i.id = si.item_id
		 JOIN skill_stores s ON s.id = i.store_id
		 ORDER BY si.sandbox_name, s.name, i.kind, i.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]SkillItem)
	for rows.Next() {
		var sandbox string
		var item SkillItem
		if err := rows.Scan(&sandbox, &item.ID, &item.StoreID, &item.StoreName, &item.Kind, &item.Name, &item.Description, &item.Plugin, &item.RelPath); err != nil {
			return nil, err
		}
		out[sandbox] = append(out[sandbox], item)
	}
	return out, rows.Err()
}

// DropSandboxSkillItems removes every direct item attachment of a sandbox.
func (s *Store) DropSandboxSkillItems(ctx context.Context, sandbox string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_skill_items WHERE sandbox_name = ?`, sandbox)
	return err
}
