package app

import (
	"fmt"
	"math"
	"strings"
)

// SecretValueError reports an environment value that looks like secret
// material. It names the variable and why the value is suspicious, and points
// at the secrets feature and the bare KEY form.
type SecretValueError struct {
	Key    string
	Reason string
}

// Error implements error.
func (e *SecretValueError) Error() string {
	return fmt.Sprintf(
		"environment variable %s looks like a secret value (%s); secrets must never be written in clear into spec.yaml: store it with `sbx secret set` (or from the Secrets page) and enter just %s= so the value is read from the host environment",
		e.Key, e.Reason, e.Key)
}

// secretPrefix is one known credential prefix and the reason reported for it.
type secretPrefix struct {
	prefix string
	reason string
}

// secretPrefixes lists the credential shapes the guard recognizes. It only
// holds prefixes that are unambiguous in the wild, to keep false positives out.
var secretPrefixes = []secretPrefix{
	{"sk-", `API key prefix "sk-"`},
	{"github_pat_", `GitHub token prefix "github_pat_"`},
	{"ghp_", `GitHub token prefix "ghp_"`},
	{"gho_", `GitHub token prefix "gho_"`},
	{"ghu_", `GitHub token prefix "ghu_"`},
	{"ghs_", `GitHub token prefix "ghs_"`},
	{"ghr_", `GitHub token prefix "ghr_"`},
	{"xoxa-", `Slack token prefix "xoxa-"`},
	{"xoxb-", `Slack token prefix "xoxb-"`},
	{"xoxp-", `Slack token prefix "xoxp-"`},
	{"xoxr-", `Slack token prefix "xoxr-"`},
	{"xoxs-", `Slack token prefix "xoxs-"`},
	{"xapp-", `Slack token prefix "xapp-"`},
	{"AKIA", `AWS access key id prefix "AKIA"`},
	{"ASIA", `AWS access key id prefix "ASIA"`},
	{"AIza", `Google API key prefix "AIza"`},
	{"glpat-", `GitLab token prefix "glpat-"`},
	{"gldt-", `GitLab token prefix "gldt-"`},
	{"glrt-", `GitLab token prefix "glrt-"`},
	{"npm_", `npm token prefix "npm_"`},
	{"pypi-", `PyPI token prefix "pypi-"`},
	{"dckr_pat_", `Docker token prefix "dckr_pat_"`},
}

// Entropy tuning for the generic long-token rule. The thresholds are
// deliberately conservative: a false negative costs nothing, a false positive
// blocks a legitimate value.
const (
	secretMinTokenLen = 32
	secretMinEntropy  = 3.5
)

// SecretReason explains why value looks like secret material, or returns ""
// when it does not. The rule is: known credential prefixes, PEM private key
// blocks, or a single long high-entropy token (>= 32 characters, letters and
// digits only plus - and _, Shannon entropy >= 3.5 bits per character). Values
// carrying spaces, paths, URLs or other punctuation are never rejected this way.
func SecretReason(value string) string {
	v := strings.TrimSpace(value)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "-----begin ") && strings.Contains(lower, "private key-----") {
		return "PEM private key block"
	}
	for _, rule := range secretPrefixes {
		if strings.HasPrefix(v, rule.prefix) {
			return rule.reason
		}
	}
	if len(v) < secretMinTokenLen || !isTokenString(v) || !hasLetter(v) || !hasDigit(v) {
		return ""
	}
	if shannonEntropy(v) < secretMinEntropy {
		return ""
	}
	return "long high-entropy token"
}

// checkCreateEnvSecrets refuses to persist environment values that look like
// secret material. A value already present verbatim in the target sandbox's
// on-disk spec is tolerated, so recreating an existing hand-written config
// keeps working; `sandwarden doctor` reports those instead.
func (a *App) checkCreateEnvSecrets(req CreateRequest) error {
	if len(req.Opts.Env) == 0 {
		return nil
	}
	existing := map[string]string{}
	if name := strings.TrimSpace(req.Opts.Name); name != "" {
		if cfg, ok := a.Fleet.SandboxByName(name); ok {
			existing = cfg.Spec.Env()
		}
	}
	for _, kv := range req.Opts.Env {
		key, value, found := strings.Cut(kv, "=")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !found {
			value = ""
		}
		if prev, ok := existing[key]; ok && prev == value {
			continue
		}
		if reason := SecretReason(value); reason != "" {
			return &SecretValueError{Key: key, Reason: reason}
		}
	}
	return nil
}

// isTokenString reports whether v is a single bare token: letters, digits and
// the URL-safe punctuation base64url credentials use, but no separator that
// would suggest a path, URL or sentence.
func isTokenString(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// hasLetter reports whether v contains an ASCII letter.
func hasLetter(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

// hasDigit reports whether v contains an ASCII digit.
func hasDigit(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] >= '0' && v[i] <= '9' {
			return true
		}
	}
	return false
}

// shannonEntropy returns the Shannon entropy of v in bits per character.
func shannonEntropy(v string) float64 {
	var counts [256]int
	for i := 0; i < len(v); i++ {
		counts[v[i]]++
	}
	total := float64(len(v))
	entropy := 0.0
	for _, count := range counts {
		if count == 0 {
			continue
		}
		p := float64(count) / total
		entropy -= p * math.Log2(p)
	}
	return entropy
}
