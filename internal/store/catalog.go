package store

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
)

// CatalogFingerprint digests the discovered catalogs so the search index is
// rebuilt only when something actually changed.
func (s *Store) CatalogFingerprint(ctx context.Context) (string, error) {
	h := fnv.New64a()
	skillRows, err := s.db.QueryContext(ctx, `SELECT `+skillItemCols+` FROM skill_items ORDER BY store, kind, name`)
	if err != nil {
		return "", err
	}
	defer skillRows.Close()
	for skillRows.Next() {
		var item SkillItem
		if err := skillRows.Scan(&item.Store, &item.Kind, &item.Name, &item.Description, &item.Plugin, &item.RelPath); err != nil {
			return "", err
		}
		fmt.Fprintf(h, "s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00", item.Store, item.Kind, item.Name, item.Description, item.Plugin, item.RelPath)
	}
	if err := skillRows.Err(); err != nil {
		return "", err
	}
	kitRows, err := s.db.QueryContext(ctx, `SELECT store, kind, name, display_name, description, version, image, requires_agent, rel_path FROM kit_items ORDER BY store, name`)
	if err != nil {
		return "", err
	}
	defer kitRows.Close()
	for kitRows.Next() {
		var item KitItem
		if err := kitRows.Scan(&item.Store, &item.Kind, &item.Name, &item.DisplayName, &item.Description,
			&item.Version, &item.Image, &item.RequiresAgent, &item.RelPath); err != nil {
			return "", err
		}
		fmt.Fprintf(h, "k\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00",
			item.Store, item.Kind, item.Name, item.DisplayName, item.Description, item.Version, item.Image, item.RequiresAgent, item.RelPath)
	}
	if err := kitRows.Err(); err != nil {
		return "", err
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}

// CatalogSizes returns the number of catalog rows per store slug for one kind
// ("skill" or "kit").
func (s *Store) CatalogSizes(ctx context.Context, kind string) (map[string]int, error) {
	table := "skill_items"
	if kind == "kit" {
		table = "kit_items"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT store, COUNT(*) FROM `+table+` GROUP BY store`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sizes := map[string]int{}
	for rows.Next() {
		var storeSlug string
		var count int
		if err := rows.Scan(&storeSlug, &count); err != nil {
			return nil, err
		}
		sizes[storeSlug] = count
	}
	return sizes, rows.Err()
}
