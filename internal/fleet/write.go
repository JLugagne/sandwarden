package fleet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// File and header conventions for the on-disk fleet.
const (
	specFile    = "spec.yaml"
	sidecarFile = "sandwarden.yaml"

	sandboxesDir = "sandboxes"
	profilesDir  = "profiles"
	cachesDir    = "caches"
	storesDir    = "stores"
	configFile   = "config.yaml"

	sandboxHeader = "# Sandbox kit managed by sandwarden (github.com/JLugagne/sandwarden).\n" +
		"# spec.yaml is a valid sbx kit; sandwarden.yaml holds the rest.\n"
	profileHeader = "# Profile kit managed by sandwarden: network rules live here.\n" +
		"# sandwarden.yaml holds mounts, caches and skills selections.\n"
	cacheHeader  = "# Shared cache definition managed by sandwarden.\n"
	storeHeader  = "# Git store registration managed by sandwarden.\n"
	configHeader = "# sandwarden global configuration.\n"
)

// writeYAML marshals v and writes it atomically, prefixed with header.
func writeYAML(path, header string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if header != "" {
		data = append([]byte(header), data...)
	}
	return atomicWrite(path, data)
}

// readYAML decodes a YAML file strictly: unknown fields are errors, so typos
// and drift from the sbx kit schema are caught instead of silently dropped.
func readYAML(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := yaml.NewDecoder(f, yaml.DisallowUnknownField())
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// atomicWrite writes data to path via a temporary file and rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
