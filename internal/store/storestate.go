package store

import "context"

// StoreState is the per-store sync outcome: when the checkout last refreshed
// and the last error, if any.
type StoreState struct {
	Kind     string `json:"kind"`
	Store    string `json:"store"`
	SyncedAt string `json:"synced_at"`
	Error    string `json:"error"`
}

// SetStoreSync records a sync outcome for one store.
func (s *Store) SetStoreSync(ctx context.Context, kind, storeSlug, syncedAt, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO store_state (kind, store, synced_at, error) VALUES (?, ?, ?, ?)
		ON CONFLICT (kind, store) DO UPDATE SET synced_at = excluded.synced_at, error = excluded.error`,
		kind, storeSlug, syncedAt, errMsg)
	return err
}

// StoreStates returns every sync state of one kind, keyed by store slug.
func (s *Store) StoreStates(ctx context.Context, kind string) (map[string]StoreState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT kind, store, synced_at, error FROM store_state WHERE kind = ?`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]StoreState{}
	for rows.Next() {
		var st StoreState
		if err := rows.Scan(&st.Kind, &st.Store, &st.SyncedAt, &st.Error); err != nil {
			return nil, err
		}
		out[st.Store] = st
	}
	return out, rows.Err()
}

// DeleteStoreState forgets one store's sync state.
func (s *Store) DeleteStoreState(ctx context.Context, kind, storeSlug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM store_state WHERE kind = ? AND store = ?`, kind, storeSlug)
	return err
}
