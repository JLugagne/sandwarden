package store

import (
	"context"
	"database/sql"
)

// SkillItem is one discovered skill or command, keyed by its store slug, kind
// and name; nothing references an item by database id.
type SkillItem struct {
	Store       string `json:"store"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Plugin      string `json:"plugin"`
	RelPath     string `json:"rel_path"`
}

const skillItemCols = `store, kind, name, description, plugin, rel_path`

// ReplaceSkillItems converges one store's catalog: rows that disappeared are
// removed, the rest upserted, all in one transaction.
func (s *Store) ReplaceSkillItems(ctx context.Context, storeSlug string, items []SkillItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM skill_items WHERE store = ?`, storeSlug); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO skill_items (`+skillItemCols+`) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (store, kind, name) DO UPDATE SET
			description = excluded.description,
			plugin = excluded.plugin,
			rel_path = excluded.rel_path`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if _, err := stmt.ExecContext(ctx, storeSlug, item.Kind, item.Name, item.Description, item.Plugin, item.RelPath); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSkillItems returns one store's items when storeSlug is set, all of them
// otherwise.
func (s *Store) ListSkillItems(ctx context.Context, storeSlug string) ([]SkillItem, error) {
	query := `SELECT ` + skillItemCols + ` FROM skill_items`
	args := []any{}
	if storeSlug != "" {
		query += ` WHERE store = ?`
		args = append(args, storeSlug)
	}
	query += ` ORDER BY store, kind, name`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSkillItems(rows)
}

// ListAllSkillItems returns every catalog item.
func (s *Store) ListAllSkillItems(ctx context.Context) ([]SkillItem, error) {
	return s.ListSkillItems(ctx, "")
}

func scanSkillItems(rows *sql.Rows) ([]SkillItem, error) {
	items := []SkillItem{}
	for rows.Next() {
		var item SkillItem
		if err := rows.Scan(&item.Store, &item.Kind, &item.Name, &item.Description, &item.Plugin, &item.RelPath); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetSkillItem returns one catalog item by store slug, kind and name.
func (s *Store) GetSkillItem(ctx context.Context, storeSlug, kind, name string) (SkillItem, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+skillItemCols+` FROM skill_items WHERE store = ? AND kind = ? AND name = ?`, storeSlug, kind, name)
	var item SkillItem
	err := row.Scan(&item.Store, &item.Kind, &item.Name, &item.Description, &item.Plugin, &item.RelPath)
	if err == sql.ErrNoRows {
		return SkillItem{}, ErrNotFound
	}
	if err != nil {
		return SkillItem{}, err
	}
	return item, nil
}

// DeleteStoreCatalog removes every catalog item of one store.
func (s *Store) DeleteStoreCatalog(ctx context.Context, kind, storeSlug string) error {
	table := "skill_items"
	if kind == "kit" {
		table = "kit_items"
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE store = ?`, storeSlug)
	return err
}
