package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// ProfileMount is a bind mount a profile applies by default to every sandbox
// it is assigned to. It is applied at runtime (like caches) and re-applied by
// the reconcile pass whenever the sandbox is running.
type ProfileMount struct {
	ID         int64  `json:"id"`
	ProfileID  int64  `json:"profile_id"`
	HostPath   string `json:"host_path"`
	TargetPath string `json:"target_path"`
	ReadOnly   bool   `json:"read_only"`
	CreatedAt  string `json:"created_at"`
}

// SandboxProfileMount pairs a profile mount with the profile that declares it.
type SandboxProfileMount struct {
	ProfileMount
	ProfileName string
}

// SandboxProfileCache pairs a shared cache with the profile that defaults it.
type SandboxProfileCache struct {
	CacheMount
	ProfileID   int64
	ProfileName string
}

// Opt-out kinds recorded in sandbox_profile_optouts.
const (
	OptOutMount = "mount"
	OptOutCache = "cache"
)

const profileMountCols = `id, profile_id, host_path, target_path, read_only, created_at`

const profileCacheCols = `c.id, c.name, c.description, c.host_path, c.target_path, c.read_only, c.auto_attach, c.enabled, c.created_at, c.updated_at`

func scanProfileMount(row interface{ Scan(...any) error }) (ProfileMount, error) {
	var m ProfileMount
	err := row.Scan(&m.ID, &m.ProfileID, &m.HostPath, &m.TargetPath, &m.ReadOnly, &m.CreatedAt)
	return m, err
}

func scanProfileMountWithProfile(row interface{ Scan(...any) error }) (SandboxProfileMount, error) {
	var m SandboxProfileMount
	err := row.Scan(&m.ID, &m.ProfileID, &m.HostPath, &m.TargetPath, &m.ReadOnly, &m.CreatedAt, &m.ProfileName)
	return m, err
}

func scanProfileCache(row interface{ Scan(...any) error }) (CacheMount, int64, error) {
	var c CacheMount
	var readOnly, autoAttach, enabled int
	var profileID int64
	if err := row.Scan(&c.ID, &c.Name, &c.Description, &c.HostPath, &c.TargetPath, &readOnly, &autoAttach, &enabled, &c.CreatedAt, &c.UpdatedAt, &profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CacheMount{}, 0, ErrNotFound
		}
		return CacheMount{}, 0, err
	}
	c.ReadOnly = readOnly != 0
	c.AutoAttach = autoAttach != 0
	c.Enabled = enabled != 0
	return c, profileID, nil
}

func scanSandboxProfileCache(row interface{ Scan(...any) error }) (SandboxProfileCache, error) {
	var c SandboxProfileCache
	var readOnly, autoAttach, enabled int
	if err := row.Scan(&c.ID, &c.Name, &c.Description, &c.HostPath, &c.TargetPath, &readOnly, &autoAttach, &enabled, &c.CreatedAt, &c.UpdatedAt, &c.ProfileID, &c.ProfileName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SandboxProfileCache{}, ErrNotFound
		}
		return SandboxProfileCache{}, err
	}
	c.ReadOnly = readOnly != 0
	c.AutoAttach = autoAttach != 0
	c.Enabled = enabled != 0
	return c, nil
}

// ListProfileMounts returns the mounts declared by one profile.
func (s *Store) ListProfileMounts(ctx context.Context, profileID int64) ([]ProfileMount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+profileMountCols+` FROM profile_mounts WHERE profile_id = ? ORDER BY host_path, target_path`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProfileMount
	for rows.Next() {
		m, err := scanProfileMount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AllProfileMounts returns every profile mount keyed by profile id.
func (s *Store) AllProfileMounts(ctx context.Context) (map[int64][]ProfileMount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+profileMountCols+` FROM profile_mounts ORDER BY profile_id, host_path, target_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]ProfileMount{}
	for rows.Next() {
		m, err := scanProfileMount(rows)
		if err != nil {
			return nil, err
		}
		out[m.ProfileID] = append(out[m.ProfileID], m)
	}
	return out, rows.Err()
}

// GetProfileMount loads one mount of a profile.
func (s *Store) GetProfileMount(ctx context.Context, profileID, mountID int64) (ProfileMount, error) {
	m, err := scanProfileMount(s.db.QueryRowContext(ctx,
		`SELECT `+profileMountCols+` FROM profile_mounts WHERE profile_id = ? AND id = ?`, profileID, mountID))
	if errors.Is(err, sql.ErrNoRows) {
		return ProfileMount{}, ErrNotFound
	}
	if err != nil {
		return ProfileMount{}, err
	}
	return m, nil
}

// AddProfileMount declares a default bind mount on a profile. Idempotent for
// the same profile/host/target triple.
func (s *Store) AddProfileMount(ctx context.Context, profileID int64, hostPath, targetPath string, readOnly bool) (ProfileMount, error) {
	hostPath = strings.TrimSpace(hostPath)
	targetPath = strings.TrimSpace(targetPath)
	if hostPath == "" {
		return ProfileMount{}, errors.New("host path is required")
	}
	createdAt := now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profile_mounts (profile_id, host_path, target_path, read_only, created_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (profile_id, host_path, target_path) DO UPDATE SET read_only = excluded.read_only`, profileID, hostPath, targetPath, readOnly, createdAt)
	if err != nil {
		return ProfileMount{}, err
	}
	return scanProfileMount(s.db.QueryRowContext(ctx,
		`SELECT `+profileMountCols+` FROM profile_mounts WHERE profile_id = ? AND host_path = ? AND target_path = ?`,
		profileID, hostPath, targetPath))
}

// RemoveProfileMount deletes one mount and returns the removed row so the
// caller can release the bind from running sandboxes. Opt-outs recorded for
// the mount are dropped too.
func (s *Store) RemoveProfileMount(ctx context.Context, profileID, mountID int64) (ProfileMount, error) {
	m, err := s.GetProfileMount(ctx, profileID, mountID)
	if err != nil {
		return ProfileMount{}, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM profile_mounts WHERE profile_id = ? AND id = ?`, profileID, mountID); err != nil {
		return ProfileMount{}, err
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM sandbox_profile_optouts WHERE kind = ? AND ref_id = ?`, OptOutMount, mountID); err != nil {
		return ProfileMount{}, err
	}
	return m, nil
}

// ProfileMountsForSandbox returns every mount declared by a profile assigned
// to the sandbox, with the declaring profile name.
func (s *Store) ProfileMountsForSandbox(ctx context.Context, sandbox string) ([]SandboxProfileMount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT pm.id, pm.profile_id, pm.host_path, pm.target_path, pm.read_only, pm.created_at, p.name
		 FROM profile_mounts pm
		 JOIN sandbox_profiles sp ON sp.profile_id = pm.profile_id
		 JOIN profiles p ON p.id = pm.profile_id
		 WHERE sp.sandbox_name = ?
		 ORDER BY pm.host_path, pm.target_path, p.name`, sandbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SandboxProfileMount
	for rows.Next() {
		m, err := scanProfileMountWithProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListProfileCaches returns the caches defaulted by one profile.
func (s *Store) ListProfileCaches(ctx context.Context, profileID int64) ([]CacheMount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+profileCacheCols+` FROM cache_mounts c JOIN profile_caches pc ON pc.cache_id = c.id
		 WHERE pc.profile_id = ? ORDER BY c.name`, profileID)
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

// AllProfileCaches returns every profile default cache keyed by profile id.
func (s *Store) AllProfileCaches(ctx context.Context) (map[int64][]CacheMount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+profileCacheCols+`, pc.profile_id FROM cache_mounts c JOIN profile_caches pc ON pc.cache_id = c.id
		 ORDER BY pc.profile_id, c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]CacheMount{}
	for rows.Next() {
		c, profileID, err := scanProfileCache(rows)
		if err != nil {
			return nil, err
		}
		out[profileID] = append(out[profileID], c)
	}
	return out, rows.Err()
}

// AddProfileCache defaults a shared cache on a profile. Idempotent.
func (s *Store) AddProfileCache(ctx context.Context, profileID, cacheID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profile_caches (profile_id, cache_id, added_at) VALUES (?, ?, ?)
		 ON CONFLICT (profile_id, cache_id) DO NOTHING`, profileID, cacheID, now())
	return err
}

// RemoveProfileCache stops defaulting a cache from a profile and drops the
// opt-outs recorded for it.
func (s *Store) RemoveProfileCache(ctx context.Context, profileID, cacheID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM profile_caches WHERE profile_id = ? AND cache_id = ?`, profileID, cacheID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sandbox_profile_optouts WHERE kind = ? AND ref_id = ? AND ref_id NOT IN (SELECT cache_id FROM profile_caches)`,
		OptOutCache, cacheID)
	return err
}

// ProfileCachesForSandbox returns every cache defaulted by a profile assigned
// to the sandbox, with the declaring profile name.
func (s *Store) ProfileCachesForSandbox(ctx context.Context, sandbox string) ([]SandboxProfileCache, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+profileCacheCols+`, pc.profile_id, p.name
		 FROM cache_mounts c
		 JOIN profile_caches pc ON pc.cache_id = c.id
		 JOIN sandbox_profiles sp ON sp.profile_id = pc.profile_id
		 JOIN profiles p ON p.id = pc.profile_id
		 WHERE sp.sandbox_name = ?
		 ORDER BY c.name, p.name`, sandbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SandboxProfileCache
	for rows.Next() {
		c, err := scanSandboxProfileCache(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// OptOutProfileItem remembers that a sandbox detached one profile-provided
// mount or cache, so reconcile does not re-add it.
func (s *Store) OptOutProfileItem(ctx context.Context, sandbox, kind string, refID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sandbox_profile_optouts (sandbox_name, kind, ref_id, opted_out_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (sandbox_name, kind, ref_id) DO NOTHING`, sandbox, kind, refID, now())
	return err
}

// ClearProfileItemOptOut re-enables a profile-provided item on a sandbox.
func (s *Store) ClearProfileItemOptOut(ctx context.Context, sandbox, kind string, refID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sandbox_profile_optouts WHERE sandbox_name = ? AND kind = ? AND ref_id = ?`, sandbox, kind, refID)
	return err
}

// ProfileOptOuts returns the sandbox's profile-item opt-outs keyed by
// "kind:ref_id".
func (s *Store) ProfileOptOuts(ctx context.Context, sandbox string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, ref_id FROM sandbox_profile_optouts WHERE sandbox_name = ?`, sandbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var kind string
		var refID int64
		if err := rows.Scan(&kind, &refID); err != nil {
			return nil, err
		}
		out[optOutKey(kind, refID)] = true
	}
	return out, rows.Err()
}

// ClearOptOutsForProfile forgets every opt-out recorded against a profile's
// mounts and caches, so a fresh assignment re-applies them everywhere.
func (s *Store) ClearOptOutsForProfile(ctx context.Context, profileID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sandbox_profile_optouts
		 WHERE (kind = ? AND ref_id IN (SELECT id FROM profile_mounts WHERE profile_id = ?))
		    OR (kind = ? AND ref_id IN (SELECT cache_id FROM profile_caches WHERE profile_id = ?))`,
		OptOutMount, profileID, OptOutCache, profileID)
	return err
}

// PurgeProfileOptOuts forgets the opt-outs pointing at a profile's mounts and
// caches, unless another profile still defaults the same cache. Call it before
// the profile itself is deleted.
func (s *Store) PurgeProfileOptOuts(ctx context.Context, profileID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sandbox_profile_optouts
		 WHERE (kind = ? AND ref_id IN (SELECT id FROM profile_mounts WHERE profile_id = ?))
		    OR (kind = ? AND ref_id IN (SELECT cache_id FROM profile_caches WHERE profile_id = ?)
		        AND ref_id NOT IN (SELECT cache_id FROM profile_caches WHERE profile_id <> ?))`,
		OptOutMount, profileID, OptOutCache, profileID, profileID)
	return err
}

// DropSandboxOptOuts removes every opt-out of a sandbox that no longer exists.
func (s *Store) DropSandboxOptOuts(ctx context.Context, sandbox string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_profile_optouts WHERE sandbox_name = ?`, sandbox)
	return err
}

// optOutKey builds the lookup key used by ProfileOptOuts.
func optOutKey(kind string, refID int64) string {
	return kind + ":" + strconv.FormatInt(refID, 10)
}

// ProfileOptOutKey exposes the opt-out lookup key to callers.
func ProfileOptOutKey(kind string, refID int64) string {
	return optOutKey(kind, refID)
}
