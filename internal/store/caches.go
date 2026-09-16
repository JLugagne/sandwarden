package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const cacheCols = `id, name, description, host_path, target_path, read_only, auto_attach, enabled, created_at, updated_at`

// CacheMount is a shared host directory mounted into sandboxes at a
// toolchain-specific target path (Go module cache, npm cache, …).
type CacheMount struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	HostPath    string `json:"host_path"`
	TargetPath  string `json:"target_path"`
	ReadOnly    bool   `json:"read_only"`
	AutoAttach  bool   `json:"auto_attach"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func scanCacheMount(row interface{ Scan(...any) error }) (CacheMount, error) {
	var c CacheMount
	var readOnly, autoAttach, enabled int
	if err := row.Scan(&c.ID, &c.Name, &c.Description, &c.HostPath, &c.TargetPath, &readOnly, &autoAttach, &enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CacheMount{}, ErrNotFound
		}
		return CacheMount{}, err
	}
	c.ReadOnly = readOnly != 0
	c.AutoAttach = autoAttach != 0
	c.Enabled = enabled != 0
	return c, nil
}

// ListCacheMounts returns every configured cache ordered by name.
func (s *Store) ListCacheMounts(ctx context.Context) ([]CacheMount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cacheCols+` FROM cache_mounts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CacheMount
	for rows.Next() {
		c, err := scanCacheMount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCacheMount returns one cache by id.
func (s *Store) GetCacheMount(ctx context.Context, id int64) (CacheMount, error) {
	return scanCacheMount(s.db.QueryRowContext(ctx, `SELECT `+cacheCols+` FROM cache_mounts WHERE id = ?`, id))
}

// CreateCacheMount inserts a cache definition.
func (s *Store) CreateCacheMount(ctx context.Context, c CacheMount) (CacheMount, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return CacheMount{}, errors.New("cache name is required")
	}
	ts := now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO cache_mounts (name, description, host_path, target_path, read_only, auto_attach, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Description, c.HostPath, c.TargetPath, boolInt(c.ReadOnly), boolInt(c.AutoAttach), boolInt(c.Enabled), ts, ts)
	if err != nil {
		return CacheMount{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CacheMount{}, err
	}
	return s.GetCacheMount(ctx, id)
}

// UpdateCacheMount rewrites a cache definition.
func (s *Store) UpdateCacheMount(ctx context.Context, c CacheMount) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("cache name is required")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE cache_mounts SET name = ?, description = ?, host_path = ?, target_path = ?, read_only = ?, auto_attach = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Description, c.HostPath, c.TargetPath, boolInt(c.ReadOnly), boolInt(c.AutoAttach), boolInt(c.Enabled), now(), c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCacheMount removes a cache definition and its per-sandbox assignments.
func (s *Store) DeleteCacheMount(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM cache_mounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignCache marks a cache as desired for a sandbox. Idempotent.
func (s *Store) AssignCache(ctx context.Context, sandbox string, cacheID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sandbox_caches (sandbox_name, cache_id, attached_at) VALUES (?, ?, ?) ON CONFLICT (sandbox_name, cache_id) DO NOTHING`,
		sandbox, cacheID, now())
	return err
}

// UnassignCache drops the desired-state row for a sandbox/cache pair.
func (s *Store) UnassignCache(ctx context.Context, sandbox string, cacheID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_caches WHERE sandbox_name = ? AND cache_id = ?`, sandbox, cacheID)
	return err
}

// ListCachesForSandbox returns the caches assigned to one sandbox.
func (s *Store) ListCachesForSandbox(ctx context.Context, sandbox string) ([]CacheMount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.name, c.description, c.host_path, c.target_path, c.read_only, c.auto_attach, c.enabled, c.created_at, c.updated_at
		 FROM cache_mounts c JOIN sandbox_caches sc ON sc.cache_id = c.id
		 WHERE sc.sandbox_name = ? ORDER BY c.name`, sandbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CacheMount
	for rows.Next() {
		c, err := scanCacheMount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AutoAttachCaches returns the enabled caches flagged for automatic attachment.
func (s *Store) AutoAttachCaches(ctx context.Context) ([]CacheMount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cacheCols+` FROM cache_mounts WHERE enabled = 1 AND auto_attach = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CacheMount
	for rows.Next() {
		c, err := scanCacheMount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DropSandboxCaches removes every cache assignment for a sandbox.
func (s *Store) DropSandboxCaches(ctx context.Context, sandbox string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_caches WHERE sandbox_name = ?`, sandbox)
	return err
}

// AllCacheAssignments returns cache id → sandbox names.
func (s *Store) AllCacheAssignments(ctx context.Context) (map[int64][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT cache_id, sandbox_name FROM sandbox_caches ORDER BY sandbox_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]string)
	for rows.Next() {
		var cacheID int64
		var sandbox string
		if err := rows.Scan(&cacheID, &sandbox); err != nil {
			return nil, err
		}
		out[cacheID] = append(out[cacheID], sandbox)
	}
	return out, rows.Err()
}
