package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// apiBase is overridable in tests.
var apiBase = "https://api.github.com"

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
	url := apiBase + "/repos/" + repoOwner + "/" + repoName + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sandwarden")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("github returned %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return release{}, err
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return release{}, errors.New("latest release has no tag")
	}
	return release{tag: payload.TagName, url: payload.HTMLURL}, nil
}
