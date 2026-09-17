package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// seedLockTest creates one running sandbox referencing one profile carrying
// an allow and a deny rule: the minimal fleet Apply converges.
func seedLockTest(t *testing.T) (*App, *fakeDaemon) {
	t.Helper()
	a, fake := newTestApp(t, "box")
	newFakeSbx(t)

	profile := fleet.NewMixin("")
	profile.DisplayName = "Web dev"
	profile.Permissions = &fleet.SpecPermission{Network: &fleet.SpecNetwork{
		Allow: []string{"api.github.com"},
		Deny:  []string{"telemetry.example.com"},
	}}
	if _, err := a.Fleet.CreateProfile("Web dev", profile, fleet.ProfileApp{}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	sandbox := fleet.NewMixin("")
	sandbox.DisplayName = "box"
	if _, err := a.Fleet.CreateSandbox("box", sandbox, fleet.SandboxApp{Sandbox: "box", Profiles: []string{"web-dev"}}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return a, fake
}

// TestConcurrentAppliesProduceOneSetOfRules reproduces the double-apply bug:
// without a lock, concurrent applies all read an empty ledger before any of
// them records, so every worker installs its own copy of the rules.
func TestConcurrentAppliesProduceOneSetOfRules(t *testing.T) {
	ctx := context.Background()
	a, fake := seedLockTest(t)
	restore := lockWait
	lockWait = time.Minute
	defer func() { lockWait = restore }()
	const workers = 8

	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = a.Apply(ctx, "box")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	rows, err := a.Store.ListAppliedRules(ctx, "web-dev", "box")
	if err != nil {
		t.Fatalf("list ledger: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ledger holds %d rows, want exactly one set of 2: %+v", len(rows), rows)
	}
	created := 0
	for _, action := range fake.appliedActions() {
		if action.Action == "remove-id" {
			continue
		}
		created += len(action.Resources)
	}
	if created != 2 {
		t.Fatalf("daemon rules created %d times, want exactly 2", created)
	}
}

// TestConcurrentProfileEditsBothSurvive reproduces the lost-update bug on
// sidecar files: each edit clones the profile, appends its rule and writes the
// whole file back, so unserialized edits overwrite each other.
func TestConcurrentProfileEditsBothSurvive(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newFakeSbx(t)
	if _, err := a.Fleet.CreateProfile("Web dev", fleet.NewMixin(""), fleet.ProfileApp{}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	const edits = 16

	start := make(chan struct{})
	errs := make([]error, edits)
	var wg sync.WaitGroup
	for i := 0; i < edits; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			pattern := "host-" + string(rune('a'+i)) + ".example.com"
			errs[i] = a.AddRuleToProfile(ctx, "web-dev", "allow", pattern)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("edit %d: %v", i, err)
		}
	}
	fresh, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	p, ok := fresh.Profile("web-dev")
	if !ok {
		t.Fatal("profile disappeared")
	}
	if got := len(p.Spec.NetworkAllow()); got != edits {
		t.Fatalf("profile kept %d of %d concurrent edits: %v", got, edits, p.Spec.NetworkAllow())
	}
}

// TestHeldLockRejectsMutation pins the contention contract: while another
// holder owns the fleet lock, a mutation fails fast with the retry error
// instead of touching the sidecar.
func TestHeldLockRejectsMutation(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newFakeSbx(t)
	if _, err := a.Fleet.CreateProfile("Web dev", fleet.NewMixin(""), fleet.ProfileApp{}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	holder, err := fleet.AcquireLock(a.Fleet.Dir(), 0)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	restore := lockWait
	lockWait = 50 * time.Millisecond
	defer func() {
		lockWait = restore
		_ = holder.Release()
	}()

	if err := a.AddRuleToProfile(ctx, "web-dev", "allow", "api.github.com"); !errors.Is(err, fleet.ErrLocked) {
		t.Fatalf("mutation error = %v, want fleet.ErrLocked", err)
	}
	fresh, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	p, ok := fresh.Profile("web-dev")
	if !ok {
		t.Fatal("profile disappeared")
	}
	if got := p.Spec.NetworkAllow(); len(got) != 0 {
		t.Fatalf("contended mutation wrote the profile: %v", got)
	}
}

// TestReadPathsIgnoreLock pins that list and detail reads run while a mutation
// holds the fleet lock: only writers contend.
func TestReadPathsIgnoreLock(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newFakeSbx(t)
	if _, err := a.Fleet.CreateProfile("Web dev", fleet.NewMixin(""), fleet.ProfileApp{}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	holder, err := fleet.AcquireLock(a.Fleet.Dir(), 0)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer func() { _ = holder.Release() }()

	if _, err := a.ListProfiles(ctx); err != nil {
		t.Fatalf("ListProfiles under a held lock: %v", err)
	}
	if _, err := a.GetProfileView(ctx, "web-dev"); err != nil {
		t.Fatalf("GetProfileView under a held lock: %v", err)
	}
	if _, err := a.ListCaches(ctx); err != nil {
		t.Fatalf("ListCaches under a held lock: %v", err)
	}
	if _, err := a.GetConfig(ctx); err != nil {
		t.Fatalf("GetConfig under a held lock: %v", err)
	}
}

// TestLockedEntryPointsDoNotNest pins the locking discipline: every locked
// entry point calls lock-free helpers, so a short lockWait never trips on its
// own lock and the pass cannot self-deadlock.
func TestLockedEntryPointsDoNotNest(t *testing.T) {
	ctx := context.Background()
	a, _ := seedLockTest(t)
	restore := lockWait
	lockWait = 100 * time.Millisecond
	defer func() { lockWait = restore }()

	if _, err := a.Apply(ctx, "box"); err != nil {
		t.Fatalf("Apply nested on its own lock: %v", err)
	}
	if err := a.ApplyProfile(ctx, "box", "web-dev"); err != nil {
		t.Fatalf("ApplyProfile nested on its own lock: %v", err)
	}
	if err := a.UpdateProfile(ctx, "web-dev", "Web dev", "", false, false); err != nil {
		t.Fatalf("UpdateProfile nested on its own lock: %v", err)
	}
	if err := a.AddRuleToProfile(ctx, "web-dev", "deny", "tracker.example.com"); err != nil {
		t.Fatalf("AddRuleToProfile nested on its own lock: %v", err)
	}
	if err := a.RemoveRuleFromProfile(ctx, "web-dev", "deny", "tracker.example.com"); err != nil {
		t.Fatalf("RemoveRuleFromProfile nested on its own lock: %v", err)
	}
	if _, err := a.CreateProfile(ctx, "Second", "", false, false); err != nil {
		t.Fatalf("CreateProfile nested on its own lock: %v", err)
	}
	if err := a.DeleteProfile(ctx, "second"); err != nil {
		t.Fatalf("DeleteProfile nested on its own lock: %v", err)
	}
	if err := a.ApplyProfileMount(ctx, "box", "/host/src", "/src"); err != nil {
		t.Fatalf("ApplyProfileMount nested on its own lock: %v", err)
	}
	if _, err := a.AddProfileMount(ctx, "web-dev", "/host/src", "/src", true); err != nil {
		t.Fatalf("AddProfileMount nested on its own lock: %v", err)
	}
	if _, err := a.CreateCache(ctx, CacheInput{Name: "go-mod", HostPath: "/host/go-mod", Enabled: true, AutoAttach: true}); err != nil {
		t.Fatalf("CreateCache nested on its own lock: %v", err)
	}
	cache, err := a.UpdateCache(ctx, "go-mod", CacheInput{Name: "go-mod", HostPath: "/host/go-mod-2", Enabled: true, AutoAttach: true})
	if err != nil {
		t.Fatalf("UpdateCache nested on its own lock: %v", err)
	}
	if cache.Slug != "go-mod" {
		t.Fatalf("UpdateCache slug = %q", cache.Slug)
	}
	if err := a.DeleteCache(ctx, "go-mod"); err != nil {
		t.Fatalf("DeleteCache nested on its own lock: %v", err)
	}
	if err := a.SetConfig(ctx, fleet.AppConfig{Notifications: true}); err != nil {
		t.Fatalf("SetConfig nested on its own lock: %v", err)
	}
}

// TestNestedAcquisitionReportsContention documents that the lock is not
// reentrant: a nested acquisition inside one process gives up with ErrLocked
// instead of waiting on itself forever.
func TestNestedAcquisitionReportsContention(t *testing.T) {
	a, _ := newTestApp(t, "box")
	restore := lockWait
	lockWait = 30 * time.Millisecond
	defer func() { lockWait = restore }()

	err := a.withFleetLock(func() error {
		return a.withFleetLock(func() error { return nil })
	})
	if !errors.Is(err, fleet.ErrLocked) {
		t.Fatalf("nested acquisition error = %v, want fleet.ErrLocked", err)
	}
}
