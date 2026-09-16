package store

import (
	"context"
	"errors"
	"testing"
)

func TestProfileMountLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p, err := st.CreateProfile(ctx, "dev", "", false, false)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	m, err := st.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", false)
	if err != nil {
		t.Fatalf("add mount: %v", err)
	}
	if m.ID == 0 || m.HostPath != "/host/src" || m.TargetPath != "/work/src" || m.ReadOnly {
		t.Fatalf("unexpected mount: %+v", m)
	}

	if _, err := st.AddProfileMount(ctx, p.ID, "  ", "/work/src", false); err == nil {
		t.Fatal("expected error for empty host path")
	}

	// Re-adding the same host/target updates the mode instead of duplicating.
	updated, err := st.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", true)
	if err != nil {
		t.Fatalf("re-add mount: %v", err)
	}
	if updated.ID != m.ID || !updated.ReadOnly {
		t.Fatalf("expected the same row with read_only, got %+v", updated)
	}

	mounts, err := st.ListProfileMounts(ctx, p.ID)
	if err != nil {
		t.Fatalf("list mounts: %v", err)
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}

	byProfile, err := st.AllProfileMounts(ctx)
	if err != nil {
		t.Fatalf("all mounts: %v", err)
	}
	if len(byProfile[p.ID]) != 1 {
		t.Fatalf("expected 1 mount for profile, got %d", len(byProfile[p.ID]))
	}

	removed, err := st.RemoveProfileMount(ctx, p.ID, m.ID)
	if err != nil {
		t.Fatalf("remove mount: %v", err)
	}
	if removed.ID != m.ID {
		t.Fatalf("expected removed row %d, got %d", m.ID, removed.ID)
	}
	if _, err := st.GetProfileMount(ctx, p.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after removal, got %v", err)
	}
}

func TestProfileMountsForSandbox(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p1, _ := st.CreateProfile(ctx, "p1", "", false, false)
	p2, _ := st.CreateProfile(ctx, "p2", "", false, false)
	other, _ := st.CreateProfile(ctx, "other", "", false, false)
	if _, err := st.AddProfileMount(ctx, p1.ID, "/host/src", "/work/src", false); err != nil {
		t.Fatalf("mount p1: %v", err)
	}
	if _, err := st.AddProfileMount(ctx, p2.ID, "/host/src", "/work/src", false); err != nil {
		t.Fatalf("mount p2: %v", err)
	}
	if _, err := st.AddProfileMount(ctx, other.ID, "/host/other", "/work/other", false); err != nil {
		t.Fatalf("mount other: %v", err)
	}
	if err := st.AssignProfile(ctx, "box", p1.ID); err != nil {
		t.Fatalf("assign p1: %v", err)
	}
	if err := st.AssignProfile(ctx, "box", p2.ID); err != nil {
		t.Fatalf("assign p2: %v", err)
	}

	rows, err := st.ProfileMountsForSandbox(ctx, "box")
	if err != nil {
		t.Fatalf("mounts for sandbox: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected both profiles to declare the mount, got %d", len(rows))
	}
	names := map[string]bool{}
	for _, row := range rows {
		names[row.ProfileName] = true
	}
	if !names["p1"] || !names["p2"] {
		t.Fatalf("unexpected profile names: %+v", names)
	}

	if rows, err := st.ProfileMountsForSandbox(ctx, "ghost"); err != nil || len(rows) != 0 {
		t.Fatalf("expected no mounts for unassigned sandbox, got %d (%v)", len(rows), err)
	}
}

func TestProfileCacheOptOuts(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p1, _ := st.CreateProfile(ctx, "p1", "", false, false)
	p2, _ := st.CreateProfile(ctx, "p2", "", false, false)
	cache, err := st.CreateCacheMount(ctx, CacheMount{Name: "go-mod", HostPath: "/host/go", TargetPath: "/go", Enabled: true})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	if err := st.AddProfileCache(ctx, p1.ID, cache.ID); err != nil {
		t.Fatalf("add cache p1: %v", err)
	}
	if err := st.AddProfileCache(ctx, p2.ID, cache.ID); err != nil {
		t.Fatalf("add cache p2: %v", err)
	}
	if err := st.AssignProfile(ctx, "box", p1.ID); err != nil {
		t.Fatalf("assign: %v", err)
	}

	byProfile, err := st.AllProfileCaches(ctx)
	if err != nil {
		t.Fatalf("all profile caches: %v", err)
	}
	if len(byProfile[p1.ID]) != 1 || len(byProfile[p2.ID]) != 1 {
		t.Fatalf("expected CacheMount on both profiles, got %+v", byProfile)
	}

	rows, err := st.ProfileCachesForSandbox(ctx, "box")
	if err != nil {
		t.Fatalf("caches for sandbox: %v", err)
	}
	if len(rows) != 1 || rows[0].ProfileName != "p1" || rows[0].Name != "go-mod" {
		t.Fatalf("unexpected cache rows: %+v", rows)
	}

	if err := st.OptOutProfileItem(ctx, "box", OptOutCache, cache.ID); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	optOuts, err := st.ProfileOptOuts(ctx, "box")
	if err != nil {
		t.Fatalf("opt-outs: %v", err)
	}
	if !optOuts[ProfileOptOutKey(OptOutCache, cache.ID)] {
		t.Fatalf("expected cache opt-out, got %+v", optOuts)
	}

	if err := st.ClearProfileItemOptOut(ctx, "box", OptOutCache, cache.ID); err != nil {
		t.Fatalf("clear opt out: %v", err)
	}
	if optOuts, _ := st.ProfileOptOuts(ctx, "box"); len(optOuts) != 0 {
		t.Fatalf("expected no opt-outs, got %+v", optOuts)
	}

	// Removing the cache from one profile keeps the opt-out for the other.
	if err := st.OptOutProfileItem(ctx, "box", OptOutCache, cache.ID); err != nil {
		t.Fatalf("opt out again: %v", err)
	}
	if err := st.RemoveProfileCache(ctx, p1.ID, cache.ID); err != nil {
		t.Fatalf("remove cache p1: %v", err)
	}
	if optOuts, _ := st.ProfileOptOuts(ctx, "box"); len(optOuts) != 1 {
		t.Fatalf("opt-out should survive while another profile defaults the cache, got %+v", optOuts)
	}
	// Removing the last reference drops the stale opt-out.
	if err := st.RemoveProfileCache(ctx, p2.ID, cache.ID); err != nil {
		t.Fatalf("remove cache p2: %v", err)
	}
	if optOuts, _ := st.ProfileOptOuts(ctx, "box"); len(optOuts) != 0 {
		t.Fatalf("expected stale opt-out to be dropped, got %+v", optOuts)
	}
}

func TestPurgeProfileOptOuts(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p, _ := st.CreateProfile(ctx, "p", "", false, false)
	m, err := st.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", false)
	if err != nil {
		t.Fatalf("add mount: %v", err)
	}
	other, _ := st.CreateProfile(ctx, "other", "", false, false)
	cache, _ := st.CreateCacheMount(ctx, CacheMount{Name: "npm", HostPath: "/host/npm", TargetPath: "/npm", Enabled: true})
	if err := st.AddProfileCache(ctx, p.ID, cache.ID); err != nil {
		t.Fatalf("add profile cache: %v", err)
	}

	if err := st.OptOutProfileItem(ctx, "box", OptOutMount, m.ID); err != nil {
		t.Fatalf("opt out mount: %v", err)
	}
	if err := st.OptOutProfileItem(ctx, "box", OptOutCache, cache.ID); err != nil {
		t.Fatalf("opt out cache: %v", err)
	}
	if err := st.PurgeProfileOptOuts(ctx, p.ID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if optOuts, _ := st.ProfileOptOuts(ctx, "box"); len(optOuts) != 0 {
		t.Fatalf("expected opt-outs purged, got %+v", optOuts)
	}

	// The cache opt-out survives a purge while another profile defaults it.
	if err := st.AddProfileCache(ctx, other.ID, cache.ID); err != nil {
		t.Fatalf("add cache to other: %v", err)
	}
	if err := st.OptOutProfileItem(ctx, "box", OptOutCache, cache.ID); err != nil {
		t.Fatalf("opt out cache: %v", err)
	}
	if err := st.PurgeProfileOptOuts(ctx, p.ID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if optOuts, _ := st.ProfileOptOuts(ctx, "box"); len(optOuts) != 1 {
		t.Fatalf("expected the shared cache opt-out to survive, got %+v", optOuts)
	}
}

func TestProfileDefaultsCascade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p, _ := st.CreateProfile(ctx, "p", "", false, false)
	if _, err := st.AddProfileMount(ctx, p.ID, "/host/src", "/work/src", false); err != nil {
		t.Fatalf("add mount: %v", err)
	}
	cache, _ := st.CreateCacheMount(ctx, CacheMount{Name: "go", HostPath: "/host/go", TargetPath: "/go", Enabled: true})
	if err := st.AddProfileCache(ctx, p.ID, cache.ID); err != nil {
		t.Fatalf("add cache: %v", err)
	}

	if err := st.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if mounts, _ := st.AllProfileMounts(ctx); len(mounts) != 0 {
		t.Fatalf("expected mounts to cascade, got %+v", mounts)
	}
	if caches, _ := st.AllProfileCaches(ctx); len(caches) != 0 {
		t.Fatalf("expected caches to cascade, got %+v", caches)
	}
}

func TestDropSandboxOptOuts(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.OptOutProfileItem(ctx, "box", OptOutMount, 7); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	if err := st.DropSandboxOptOuts(ctx, "box"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if optOuts, _ := st.ProfileOptOuts(ctx, "box"); len(optOuts) != 0 {
		t.Fatalf("expected opt-outs dropped, got %+v", optOuts)
	}
}
