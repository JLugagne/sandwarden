package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const kitStoreCols = `id, name, description, url, ref, auth, path, synced_at, error, created_at, updated_at`

const kitItemCols = `i.id, i.store_id, s.name, i.kind, i.name, i.display_name, i.description, i.version, i.image, i.requires_agent, i.rel_path, i.spec`

// KitStore is a git checkout of a kit repository.
type KitStore struct {
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

// KitItem is one kit artifact discovered in a store checkout. Spec holds the
// raw `sbx kit inspect --json` payload parsed by the app layer.
type KitItem struct {
	ID            int64  `json:"id"`
	StoreID       int64  `json:"store_id"`
	StoreName     string `json:"store_name"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description"`
	Version       string `json:"version"`
	Image         string `json:"image"`
	RequiresAgent string `json:"requires_agent"`
	RelPath       string `json:"rel_path"`
	Spec          string `json:"-"`
}

func scanKitStore(row interface{ Scan(...any) error }) (KitStore, error) {
	var s KitStore
	if err := row.Scan(&s.ID, &s.Name, &s.Description, &s.URL, &s.Ref, &s.Auth, &s.Path, &s.SyncedAt, &s.Error, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return KitStore{}, ErrNotFound
		}
		return KitStore{}, err
	}
	return s, nil
}

func scanKitItem(row interface{ Scan(...any) error }) (KitItem, error) {
	var item KitItem
	if err := row.Scan(&item.ID, &item.StoreID, &item.StoreName, &item.Kind, &item.Name, &item.DisplayName, &item.Description, &item.Version, &item.Image, &item.RequiresAgent, &item.RelPath, &item.Spec); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return KitItem{}, ErrNotFound
		}
		return KitItem{}, err
	}
	return item, nil
}

func scanKitItems(rows *sql.Rows) ([]KitItem, error) {
	defer rows.Close()
	var out []KitItem
	for rows.Next() {
		item, err := scanKitItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListKitStores returns every registered kit repository ordered by name.
func (s *Store) ListKitStores(ctx context.Context) ([]KitStore, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+kitStoreCols+` FROM kit_stores ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KitStore
	for rows.Next() {
		item, err := scanKitStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetKitStore returns one repository by id.
func (s *Store) GetKitStore(ctx context.Context, id int64) (KitStore, error) {
	return scanKitStore(s.db.QueryRowContext(ctx, `SELECT `+kitStoreCols+` FROM kit_stores WHERE id = ?`, id))
}

// CreateKitStore inserts a repository registration.
func (s *Store) CreateKitStore(ctx context.Context, input KitStore) (KitStore, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.URL = strings.TrimSpace(input.URL)
	input.Path = strings.TrimSpace(input.Path)
	if input.Name == "" {
		return KitStore{}, errors.New("kit repository name is required")
	}
	if input.URL == "" {
		return KitStore{}, errors.New("git url is required")
	}
	if input.Path == "" {
		return KitStore{}, errors.New("checkout path is required")
	}
	ts := now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO kit_stores (name, description, url, ref, auth, path, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Name, input.Description, input.URL, input.Ref, input.Auth, input.Path, ts, ts)
	if err != nil {
		return KitStore{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return KitStore{}, err
	}
	return s.GetKitStore(ctx, id)
}

// UpdateKitStore rewrites a repository registration. The checkout path stays
// stable across renames.
func (s *Store) UpdateKitStore(ctx context.Context, input KitStore) error {
	input.Name = strings.TrimSpace(input.Name)
	input.URL = strings.TrimSpace(input.URL)
	if input.Name == "" {
		return errors.New("kit repository name is required")
	}
	if input.URL == "" {
		return errors.New("git url is required")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE kit_stores SET name = ?, description = ?, url = ?, ref = ?, auth = ?, updated_at = ? WHERE id = ?`,
		input.Name, input.Description, input.URL, input.Ref, input.Auth, now(), input.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteKitStore removes a repository and, by cascade, its catalog.
func (s *Store) DeleteKitStore(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM kit_stores WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkKitStoreSynced records the outcome of a checkout refresh.
func (s *Store) MarkKitStoreSynced(ctx context.Context, id int64, syncedAt, syncErr string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE kit_stores SET synced_at = ?, error = ?, updated_at = ? WHERE id = ?`,
		syncedAt, syncErr, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceKitItems converges the catalog of one repository onto the discovered
// kits: new kits are inserted, existing ones keep their id, and kits that
// disappeared are removed.
func (s *Store) ReplaceKitItems(ctx context.Context, storeID int64, items []KitItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM kit_items WHERE store_id = ?`, storeID)
	if err != nil {
		return err
	}
	existing := make(map[string]int64)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		existing[strings.ToLower(strings.TrimSpace(name))] = id
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	keep := make(map[string]bool, len(items))
	ts := now()
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		keep[key] = true
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO kit_items (store_id, kind, name, display_name, description, version, image, requires_agent, rel_path, spec, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (store_id, name) DO UPDATE SET
			   kind = excluded.kind,
			   display_name = excluded.display_name,
			   description = excluded.description,
			   version = excluded.version,
			   image = excluded.image,
			   requires_agent = excluded.requires_agent,
			   rel_path = excluded.rel_path,
			   spec = excluded.spec`,
			storeID, item.Kind, item.Name, item.DisplayName, item.Description, item.Version, item.Image, item.RequiresAgent, item.RelPath, item.Spec, ts); err != nil {
			return err
		}
	}
	for key, id := range existing {
		if keep[key] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM kit_items WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListKitItems returns one repository's catalog.
func (s *Store) ListKitItems(ctx context.Context, storeID int64) ([]KitItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+kitItemCols+` FROM kit_items i JOIN kit_stores s ON s.id = i.store_id
		 WHERE i.store_id = ? ORDER BY i.name`, storeID)
	if err != nil {
		return nil, err
	}
	return scanKitItems(rows)
}

// ListAllKitItems returns every discovered kit across repositories.
func (s *Store) ListAllKitItems(ctx context.Context) ([]KitItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+kitItemCols+` FROM kit_items i JOIN kit_stores s ON s.id = i.store_id
		 ORDER BY s.name, i.name`)
	if err != nil {
		return nil, err
	}
	return scanKitItems(rows)
}

// GetKitItem returns one catalog kit by id.
func (s *Store) GetKitItem(ctx context.Context, id int64) (KitItem, error) {
	return scanKitItem(s.db.QueryRowContext(ctx,
		`SELECT `+kitItemCols+` FROM kit_items i JOIN kit_stores s ON s.id = i.store_id WHERE i.id = ?`, id))
}
