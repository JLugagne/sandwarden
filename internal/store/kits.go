package store

import (
	"context"
	"database/sql"
)

// KitItem is one discovered kit artifact, keyed by its store slug and name.
// Spec holds the raw `sbx kit inspect --json` payload.
type KitItem struct {
	Store         string `json:"store"`
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

const kitItemCols = `store, kind, name, display_name, description, version, image, requires_agent, rel_path, spec`

// ReplaceKitItems converges one kit store's catalog in a single transaction.
func (s *Store) ReplaceKitItems(ctx context.Context, storeSlug string, items []KitItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM kit_items WHERE store = ?`, storeSlug); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO kit_items (`+kitItemCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (store, name) DO UPDATE SET
			kind = excluded.kind,
			display_name = excluded.display_name,
			description = excluded.description,
			version = excluded.version,
			image = excluded.image,
			requires_agent = excluded.requires_agent,
			rel_path = excluded.rel_path,
			spec = excluded.spec`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if _, err := stmt.ExecContext(ctx, storeSlug, item.Kind, item.Name, item.DisplayName, item.Description,
			item.Version, item.Image, item.RequiresAgent, item.RelPath, item.Spec); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListKitItems returns one store's kits when storeSlug is set, all of them
// otherwise.
func (s *Store) ListKitItems(ctx context.Context, storeSlug string) ([]KitItem, error) {
	query := `SELECT ` + kitItemCols + ` FROM kit_items`
	args := []any{}
	if storeSlug != "" {
		query += ` WHERE store = ?`
		args = append(args, storeSlug)
	}
	query += ` ORDER BY store, name`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []KitItem{}
	for rows.Next() {
		var item KitItem
		if err := rows.Scan(&item.Store, &item.Kind, &item.Name, &item.DisplayName, &item.Description,
			&item.Version, &item.Image, &item.RequiresAgent, &item.RelPath, &item.Spec); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListAllKitItems returns every catalog kit.
func (s *Store) ListAllKitItems(ctx context.Context) ([]KitItem, error) {
	return s.ListKitItems(ctx, "")
}

// GetKitItem returns one kit by store slug and name.
func (s *Store) GetKitItem(ctx context.Context, storeSlug, name string) (KitItem, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+kitItemCols+` FROM kit_items WHERE store = ? AND name = ?`, storeSlug, name)
	var item KitItem
	err := row.Scan(&item.Store, &item.Kind, &item.Name, &item.DisplayName, &item.Description,
		&item.Version, &item.Image, &item.RequiresAgent, &item.RelPath, &item.Spec)
	if err == sql.ErrNoRows {
		return KitItem{}, ErrNotFound
	}
	if err != nil {
		return KitItem{}, err
	}
	return item, nil
}
