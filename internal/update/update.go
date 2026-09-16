package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	repoOwner = "JLugagne"
	repoName  = "sandwarden"
)

// webBase is overridable in tests. The public web host is used instead of the
// REST API because unauthenticated API calls share a 60-requests-per-hour quota
// per IP and surface 403s to the user.
var webBase = "https://github.com"

// checkInterval bounds how often the GitHub API is queried; the last result is
// reused within that window unless the caller forces a refresh.
var checkInterval = 6 * time.Hour

// stateFile stores the last successful check under the user cache directory.
const stateFile = "update-check.json"

// State persists the last successful check.
type State struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version,omitempty"`
	ReleaseURL    string    `json:"release_url,omitempty"`
}

// ErrNoStableRelease reports that the repository has no stable release yet,
// only pre-releases.
var ErrNoStableRelease = errors.New("no stable release published")

// Result describes the outcome of an update check.
type Result struct {
	CurrentVersion  string
	LatestVersion   string
	ReleaseURL      string
	UpdateAvailable bool
}

type release struct {
	tag string
	url string
}

// Check returns the latest stable release. A cached result is used when it is
// fresh and force is not set. An empty cacheDir disables caching.
func Check(ctx context.Context, cacheDir, current string, force bool) (Result, error) {
	if cacheDir == "" {
		force = true
	}
	if !force && !shouldCheck(cacheDir) {
		return cachedResult(cacheDir, current), nil
	}
	latest, err := fetchLatest(ctx)
	if errors.Is(err, ErrNoStableRelease) {
		saveState(cacheDir, State{LastCheck: time.Now()})
		return Result{CurrentVersion: current}, nil
	}
	if err != nil {
		return Result{CurrentVersion: current}, err
	}
	saveState(cacheDir, State{LastCheck: time.Now(), LatestVersion: latest.tag, ReleaseURL: latest.url})
	return result(current, latest), nil
}

// ShouldOffer reports whether latest is a different stable release than the
// running build. Development and unstable builds are never flagged.
func ShouldOffer(current, latest string) bool {
	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)
	if current == "" || latest == "" || current == "dev" {
		return false
	}
	if strings.Contains(strings.ToLower(current), "unstable") {
		return false
	}
	return current != latest
}

func result(current string, latest release) Result {
	return Result{
		CurrentVersion:  current,
		LatestVersion:   latest.tag,
		ReleaseURL:      latest.url,
		UpdateAvailable: ShouldOffer(current, latest.tag),
	}
}

func statePath(cacheDir string) string {
	return filepath.Join(cacheDir, stateFile)
}

func shouldCheck(cacheDir string) bool {
	state := loadState(cacheDir)
	return time.Since(state.LastCheck) >= checkInterval
}

func loadState(cacheDir string) State {
	raw, err := os.ReadFile(statePath(cacheDir))
	if err != nil {
		return State{}
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}
	}
	return state
}

func saveState(cacheDir string, state State) {
	if cacheDir == "" {
		return
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(statePath(cacheDir), raw, 0o644)
}

func cachedResult(cacheDir, current string) Result {
	state := loadState(cacheDir)
	return result(current, release{tag: state.LatestVersion, url: state.ReleaseURL})
}

func fetchLatest(ctx context.Context) (release, error) {
	base := webBase + "/" + repoOwner + "/" + repoName
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/releases/latest", nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("User-Agent", "sandwarden")
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return release{}, fmt.Errorf("github returned %s", resp.Status)
	}
	const marker = "/releases/tag/"
	loc := resp.Header.Get("Location")
	idx := strings.LastIndex(loc, marker)
	if idx < 0 {
		return release{}, ErrNoStableRelease
	}
	tag := strings.Trim(loc[idx+len(marker):], "/")
	if tag == "" {
		return release{}, errors.New("latest release has no tag")
	}
	return release{tag: tag, url: base + marker + tag}, nil
}
