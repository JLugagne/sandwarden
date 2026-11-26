package sbx

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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

// Version runs `sbx version` once per client and caches the parsed result. A
// failure to run the CLI returns an unknown Version together with the error;
// callers treat that as "allow every feature".
func (c *Client) Version(ctx context.Context) (Version, error) {
	c.versionMu.Lock()
	if c.versionLoaded {
		cached := c.version
		c.versionMu.Unlock()
		return cached, nil
	}
	c.versionMu.Unlock()

	raw, err := c.runCLI(ctx, nil, nil, "version")
	if err != nil {
		return Version{Raw: strings.TrimSpace(raw)}, err
	}
	parsed := ParseVersion(raw)
	c.versionMu.Lock()
	c.version = parsed
	c.versionLoaded = true
	c.versionMu.Unlock()
	return parsed, nil
}

// ResetVersionCache forgets the cached `sbx version` result, so the next call
// re-reads it. Use it after upgrading sbx while the process stays alive.
func (c *Client) ResetVersionCache() {
	c.versionMu.Lock()
	c.version = Version{}
	c.versionLoaded = false
	c.versionMu.Unlock()
}
