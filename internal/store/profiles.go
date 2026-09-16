package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const profileCols = `id, name, description, is_default, is_global, created_at, updated_at`

// prefixedCols is profileCols qualified for JOINs that alias profiles as p.
const prefixedCols = `p.id, p.name, p.description, p.is_default, p.is_global, p.created_at, p.updated_at`

// Profile is a named, reusable set of allow/deny URL patterns.
type Profile struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	IsGlobal    bool   `json:"is_global"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	var isDefault, isGlobal int
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &isDefault, &isGlobal, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Profile{}, ErrNotFound
		}
		return Profile{}, err
	}
	p.IsDefault = isDefault != 0
	p.IsGlobal = isGlobal != 0
	return p, nil
}

// ListProfiles returns every profile ordered by name.
func (s *Store) ListProfiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileCols+` FROM profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProfile returns one profile by id.
func (s *Store) GetProfile(ctx context.Context, id int64) (Profile, error) {
	return scanProfile(s.db.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE id = ?`, id))
}

// CreateProfile inserts a profile, enforcing single-default and unique-name rules.
func (s *Store) CreateProfile(ctx context.Context, name, description string, isDefault, isGlobal bool) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, errors.New("profile name is required")
	}
	ts := now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	if isDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE profiles SET is_default = 0`); err != nil {
			return Profile{}, err
		}
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO profiles (name, description, is_default, is_global, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		name, description, boolInt(isDefault), boolInt(isGlobal), ts, ts)
	if err != nil {
		return Profile{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	return s.GetProfile(ctx, id)
}

// UpdateProfile rewrites a profile's mutable fields.
func (s *Store) UpdateProfile(ctx context.Context, id int64, name, description string, isDefault, isGlobal bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("profile name is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if isDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE profiles SET is_default = 0`); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE profiles SET name = ?, description = ?, is_default = ?, is_global = ?, updated_at = ? WHERE id = ?`,
		name, description, boolInt(isDefault), boolInt(isGlobal), now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// DeleteProfile removes a profile and its dependent rows.
func (s *Store) DeleteProfile(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DefaultProfile returns the profile marked as default, or ErrNotFound.
func (s *Store) DefaultProfile(ctx context.Context) (Profile, error) {
	return scanProfile(s.db.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE is_default = 1 LIMIT 1`))
}

// GlobalProfiles returns every profile flagged global.
func (s *Store) GlobalProfiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE is_global = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
