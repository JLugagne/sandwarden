package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"strconv"
)

// catalogFingerprintQueries lists every table that makes up the skill and kit
// catalogs, with nullable columns coalesced so the digest is stable.
var catalogFingerprintQueries = []string{
	`SELECT id, name, description, url, ref, auth, path, COALESCE(synced_at, ''), COALESCE(error, '') FROM skill_stores ORDER BY id`,
	`SELECT id, store_id, kind, name, description, plugin, rel_path FROM skill_items ORDER BY id`,
	`SELECT id, name, description, url, ref, auth, path, COALESCE(synced_at, ''), COALESCE(error, '') FROM kit_stores ORDER BY id`,
	`SELECT id, store_id, kind, name, display_name, description, version, image, requires_agent, rel_path, spec FROM kit_items ORDER BY id`,
}

// CatalogFingerprint returns a digest of the skill and kit catalogs: every
// store registration and every discovered item, document bodies excluded. It
// changes whenever the catalog moves, so callers can cache derived data (such
// as a search index) and rebuild it only when needed.
func (s *Store) CatalogFingerprint(ctx context.Context) (string, error) {
	digest := fnv.New64a()
	for _, query := range catalogFingerprintQueries {
		if err := s.digestCatalogQuery(ctx, digest, query); err != nil {
			return "", err
		}
	}
	return strconv.FormatUint(digest.Sum64(), 16), nil
}

// digestCatalogQuery folds every row of one catalog query into the digest.
func (s *Store) digestCatalogQuery(ctx context.Context, digest hash.Hash64, query string) error {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return err
		}
		for _, value := range values {
			switch typed := value.(type) {
			case nil:
				digest.Write([]byte{0})
			case []byte:
				digest.Write(typed)
			case string:
				digest.Write([]byte(typed))
			case int64:
				var encoded [8]byte
				binary.LittleEndian.PutUint64(encoded[:], uint64(typed))
				digest.Write(encoded[:])
			default:
				fmt.Fprintf(digest, "%v", typed)
			}
			digest.Write([]byte{0x1f})
		}
		digest.Write([]byte{0x1e})
	}
	return rows.Err()
}
