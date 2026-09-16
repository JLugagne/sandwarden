package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShouldOffer(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"newer tag", "v0.1.0", "v0.2.0", true},
		{"same tag", "v0.2.0", "v0.2.0", false},
		{"dev build", "dev", "v0.2.0", false},
		{"unstable build", "unstable", "v0.2.0", false},
		{"unstable localversion", "v0.2.0-unstable", "v0.3.0", false},
		{"empty latest", "v0.1.0", "", false},
		{"empty current", "", "v0.1.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldOffer(tc.current, tc.latest); got != tc.want {
				t.Fatalf("ShouldOffer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestCheckFetchesAndCaches(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/JLugagne/sandwarden/releases/latest" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/JLugagne/sandwarden/releases/tag/v0.2.0", http.StatusFound)
	}))
	defer server.Close()

	previous := webBase
	webBase = server.URL
	t.Cleanup(func() { webBase = previous })

	cacheDir := t.TempDir()
	got, err := Check(context.Background(), cacheDir, "v0.1.0", false)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	wantURL := server.URL + "/JLugagne/sandwarden/releases/tag/v0.2.0"
	if !got.UpdateAvailable || got.LatestVersion != "v0.2.0" || got.ReleaseURL != wantURL {
		t.Fatalf("unexpected result: %+v", got)
	}

	cached, err := Check(context.Background(), cacheDir, "v0.1.0", false)
	if err != nil {
		t.Fatalf("cached check: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected the second check to use the cache, got %d requests", hits)
	}
	if !cached.UpdateAvailable || cached.LatestVersion != "v0.2.0" {
		t.Fatalf("unexpected cached result: %+v", cached)
	}

	if _, err := Check(context.Background(), cacheDir, "v0.1.0", true); err != nil {
		t.Fatalf("forced check: %v", err)
	}
	if hits != 2 {
		t.Fatalf("expected the forced check to hit the web host, got %d requests", hits)
	}
}

func TestCheckSurfacesWebError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	previous := webBase
	webBase = server.URL
	t.Cleanup(func() { webBase = previous })

	if _, err := Check(context.Background(), t.TempDir(), "v0.1.0", true); err == nil {
		t.Fatal("expected an error for a failing web host")
	}
}

func TestCheckWithoutStableRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/JLugagne/sandwarden/releases", http.StatusFound)
	}))
	defer server.Close()

	previous := webBase
	webBase = server.URL
	t.Cleanup(func() { webBase = previous })

	got, err := Check(context.Background(), t.TempDir(), "v0.1.0", true)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if got.LatestVersion != "" || got.UpdateAvailable {
		t.Fatalf("expected no stable release, got %+v", got)
	}
}
