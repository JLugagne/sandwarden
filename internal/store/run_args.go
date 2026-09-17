package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// SetRunArgs stores the extra arguments a sandbox's connect command appends
// after `--`. An empty (or whitespace-only) value clears the setting.
func (s *Store) SetRunArgs(ctx context.Context, sandbox, args string) error {
	sandbox = strings.TrimSpace(sandbox)
	if sandbox == "" {
		return errors.New("sandbox name is required")
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return s.DropSandboxRunArgs(ctx, sandbox)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sandbox_run_args (sandbox_name, args, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT (sandbox_name) DO UPDATE SET args = excluded.args, updated_at = excluded.updated_at`,
		sandbox, args, now())
	return err
}

// RunArgsForSandbox returns the stored run arguments for one sandbox, or ""
// when none are set.
func (s *Store) RunArgsForSandbox(ctx context.Context, sandbox string) (string, error) {
	var args string
	err := s.db.QueryRowContext(ctx, `SELECT args FROM sandbox_run_args WHERE sandbox_name = ?`, sandbox).Scan(&args)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return args, nil
}

// AllRunArgs returns sandbox name -> run args for every sandbox with a
// non-empty setting.
func (s *Store) AllRunArgs(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sandbox_name, args FROM sandbox_run_args`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var name, args string
		if err := rows.Scan(&name, &args); err != nil {
			return nil, err
		}
		out[name] = args
	}
	return out, rows.Err()
}

// DropSandboxRunArgs removes the stored run args for a sandbox, if any.
func (s *Store) DropSandboxRunArgs(ctx context.Context, sandbox string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_run_args WHERE sandbox_name = ?`, sandbox)
	return err
}
