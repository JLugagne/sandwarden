package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JLugagne/sandwarden/internal/store"
)

func TestRunArgsLifecycle(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	if args, err := st.RunArgsForSandbox(ctx, "box"); err != nil || args != "" {
		t.Fatalf("expected empty args, got %q (%v)", args, err)
	}

	if err := st.SetRunArgs(ctx, "box", "--auto"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if args, err := st.RunArgsForSandbox(ctx, "box"); err != nil || args != "--auto" {
		t.Fatalf("unexpected args: %q (%v)", args, err)
	}

	if err := st.SetRunArgs(ctx, "box", "--other"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if args, err := st.RunArgsForSandbox(ctx, "box"); err != nil || args != "--other" {
		t.Fatalf("expected overwritten args, got %q (%v)", args, err)
	}

	all, err := st.AllRunArgs(ctx)
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if all["box"] != "--other" || len(all) != 1 {
		t.Fatalf("unexpected all run args: %+v", all)
	}

	if err := st.SetRunArgs(ctx, "box", "  "); err != nil {
		t.Fatalf("clear via blank: %v", err)
	}
	if args, err := st.RunArgsForSandbox(ctx, "box"); err != nil || args != "" {
		t.Fatalf("expected cleared args, got %q (%v)", args, err)
	}

	if err := st.SetRunArgs(ctx, "box", "--auto"); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	if err := st.DropSandboxRunArgs(ctx, "box"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	all, _ = st.AllRunArgs(ctx)
	if len(all) != 0 {
		t.Fatalf("expected no run args after drop, got %+v", all)
	}

	if err := st.SetRunArgs(ctx, " ", "--auto"); err == nil {
		t.Fatal("expected error for empty sandbox name")
	}
}
