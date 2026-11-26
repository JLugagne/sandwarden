package sbx

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// defaultVersionTTL bounds how long the cached `sbx version` result is served
// before it is re-read. It caps the window in which an sbx upgrade goes
// unnoticed by a long-running process such as the desktop app.
const defaultVersionTTL = 5 * time.Minute

// Version is the parsed `sbx version` output. Known is false when the output
// carries no recognizable semver, as dev builds that print a bare commit hash.
// An unknown version never blocks a feature: AtLeast treats it as new enough.
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string
	Raw   string
	Known bool
}

// Versions is the full `sbx version` answer: the CLI (client) version and, when
// `sbx version --json` reports one, the daemon (server) version. ServerPresent
// records whether the server block existed at all, so a stopped daemon with no
// version is distinguishable from an sbx that ignores --json.
type Versions struct {
	Client        Version
	Server        Version
	ServerPresent bool
	ServerState   string
}

// ServerRunning reports whether the daemon reported the running state.
func (vs Versions) ServerRunning() bool {
	return strings.EqualFold(vs.ServerState, "running")
}

// Effective is the version feature checks must use: the lower of the client and
// server versions when both are known. An unknown part never blocks a feature,
// so an unknown client stays unknown (and thus allows everything), matching the
// previous fail-open behaviour.
func (vs Versions) Effective() Version {
	if vs.Client.Known && vs.Server.Known && vs.Server.olderThan(vs.Client) {
		return vs.Server
	}
	return vs.Client
}

// DaemonRestartRequired reports whether the daemon is running and its version
// differs from the client's. A stopped or version-less daemon never asks for a
// restart.
func (vs Versions) DaemonRestartRequired() bool {
	if !vs.ServerRunning() {
		return false
	}
	if !vs.Client.Known || !vs.Server.Known {
		return false
	}
	return !vs.Client.sameVersion(vs.Server)
}

// olderThan reports whether v precedes other. Pre-release tags are ignored, as
// in AtLeast. Unknown versions never compare as older.
func (v Version) olderThan(other Version) bool {
	if !v.Known || !other.Known {
		return false
	}
	switch {
	case v.Major != other.Major:
		return v.Major < other.Major
	case v.Minor != other.Minor:
		return v.Minor < other.Minor
	default:
		return v.Patch < other.Patch
	}
}

// sameVersion reports whether two known versions are identical, including the
// pre-release tag.
func (v Version) sameVersion(other Version) bool {
	return v.Major == other.Major && v.Minor == other.Minor && v.Patch == other.Patch && v.Pre == other.Pre
}

// versionPattern matches an optional "v" and a dotted triple, keeping an
// optional pre-release suffix. It is searched anywhere in the output because
// `sbx version` prefixes its answer with "sbx version:".
var versionPattern = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z][0-9A-Za-z.-]*))?`)

// ParseVersion parses the combined output of `sbx version`. Output without a
// dotted version (for example a dev build printing only a commit hash) yields
// an unknown Version rather than an error.
func ParseVersion(output string) Version {
	raw := strings.TrimSpace(output)
	match := versionPattern.FindStringSubmatch(raw)
	if match == nil {
		return Version{Raw: raw}
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Pre:   match[4],
		Raw:   raw,
		Known: true,
	}
}

// versionJSON is the `sbx version --json` payload. The server block is absent
// while the daemon is stopped or on older sbx builds.
type versionJSON struct {
	Client *struct {
		Version string `json:"version"`
	} `json:"client"`
	Server *struct {
		State   string `json:"state"`
		Version string `json:"version"`
	} `json:"server"`
}

// ParseVersions parses `sbx version` output. It recognises the `--json` payload
// (client plus optional server) and falls back to the plain-text output, which
// only names the client version. Noise before the JSON (warnings) is skipped.
func ParseVersions(output string) Versions {
	if vs, ok := parseVersionJSON(output); ok {
		return vs
	}
	return Versions{Client: ParseVersion(output)}
}

// parseVersionJSON decodes the structured payload when output carries one. It
// reports false for plain-text output so the caller can fall back.
func parseVersionJSON(output string) (Versions, bool) {
	var payload versionJSON
	if err := decodeCLIJSON(output, &payload); err != nil || payload.Client == nil {
		return Versions{}, false
	}
	vs := Versions{
		Client:        ParseVersion(payload.Client.Version),
		ServerPresent: payload.Server != nil,
	}
	if payload.Server != nil {
		vs.ServerState = payload.Server.State
		vs.Server = ParseVersion(payload.Server.Version)
	}
	return vs, true
}

// String renders the version as vMAJOR.MINOR.PATCH[-PRE], or the raw output
// when the version could not be parsed.
func (v Version) String() string {
	if !v.Known {
		if v.Raw != "" {
			return v.Raw
		}
		return "unknown"
	}
	out := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		out += "-" + v.Pre
	}
	return out
}

// AtLeast reports whether the version is at least min. An unknown installed
// version always satisfies it, so a dev build is never blocked. A pre-release
// such as v0.43.0-rc1 counts as v0.43.0: the pre-release tag is ignored.
func (v Version) AtLeast(min string) bool {
	if !v.Known {
		return true
	}
	want := ParseVersion(min)
	if !want.Known {
		return true
	}
	switch {
	case v.Major != want.Major:
		return v.Major > want.Major
	case v.Minor != want.Minor:
		return v.Minor > want.Minor
	default:
		return v.Patch >= want.Patch
	}
}

// clock returns the time source the version cache trusts.
func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// cacheTTL returns how long a cached version stays valid.
func (c *Client) cacheTTL() time.Duration {
	if c.versionTTL > 0 {
		return c.versionTTL
	}
	return defaultVersionTTL
}

// Version returns the effective version: the lower of client and daemon when
// both are known, otherwise the client. It runs and caches `sbx version`. A
// failure to run the CLI returns an unknown Version together with the error;
// callers treat that as "allow every feature".
func (c *Client) Version(ctx context.Context) (Version, error) {
	vs, err := c.Versions(ctx)
	return vs.Effective(), err
}

// Versions runs `sbx version --json` (falling back to plain `sbx version`) and
// caches the parsed client and server versions for cacheTTL.
func (c *Client) Versions(ctx context.Context) (Versions, error) {
	c.versionMu.Lock()
	if c.versionLoaded {
		age := c.clock().Sub(c.versionAt)
		if age < c.cacheTTL() {
			cached := c.versions
			c.versionMu.Unlock()
			return cached, nil
		}
	}
	c.versionMu.Unlock()
	return c.loadVersions(ctx)
}

// RefreshVersion re-reads `sbx version`, bypassing and replacing the cache. Use
// it when an incompatibility was just detected so an sbx upgrade picked up
// since the cache was filled is not ignored until the TTL expires.
func (c *Client) RefreshVersion(ctx context.Context) (Version, error) {
	vs, err := c.RefreshVersions(ctx)
	return vs.Effective(), err
}

// RefreshVersions re-reads both versions, bypassing and replacing the cache.
func (c *Client) RefreshVersions(ctx context.Context) (Versions, error) {
	return c.loadVersions(ctx)
}

// loadVersions runs `sbx version --json`, falling back to plain `sbx version`
// when the installed sbx predates the flag, and stores the result with its
// timestamp.
func (c *Client) loadVersions(ctx context.Context) (Versions, error) {
	raw, err := c.runCLI(ctx, nil, nil, "version", "--json")
	if err == nil {
		if vs, ok := parseVersionJSON(raw); ok {
			c.storeVersions(vs)
			return vs, nil
		}
	}
	rawText, textErr := c.runCLI(ctx, nil, nil, "version")
	if textErr != nil {
		return Versions{Client: Version{Raw: strings.TrimSpace(rawText)}}, textErr
	}
	vs := Versions{Client: ParseVersion(rawText)}
	c.storeVersions(vs)
	return vs, nil
}

// storeVersions caches the parsed result with its timestamp.
func (c *Client) storeVersions(vs Versions) {
	c.versionMu.Lock()
	c.versions = vs
	c.versionAt = c.clock()
	c.versionLoaded = true
	c.versionMu.Unlock()
}

// ResetVersionCache forgets the cached `sbx version` result, so the next call
// re-reads it. Use it after upgrading sbx while the process stays alive.
func (c *Client) ResetVersionCache() {
	c.versionMu.Lock()
	c.versions = Versions{}
	c.versionAt = time.Time{}
	c.versionLoaded = false
	c.versionMu.Unlock()
}
