package desktop

import (
	"strings"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

// cleanStrings trims entries and drops the empty ones.
func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// normalizeSecretScope maps the UI's display scopes onto the CLI's.
func normalizeSecretScope(scope string) string {
	switch scope {
	case "global":
		return ""
	case "host-only":
		return sbx.SecretScopeHostOnly
	}
	return scope
}

// cacheFromInput builds a store record from the binding payload.
func cacheFromInput(input CacheInput, id int64) store.CacheMount {
	return store.CacheMount{
		ID:          id,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		HostPath:    strings.TrimSpace(input.HostPath),
		TargetPath:  strings.TrimSpace(input.TargetPath),
		ReadOnly:    input.ReadOnly,
		AutoAttach:  input.AutoAttach,
		Enabled:     input.Enabled,
	}
}
