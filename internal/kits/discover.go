package kits

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Candidate is a directory inside a kit repository that holds a spec.yaml.
type Candidate struct {
	Name    string `json:"name"`
	RelPath string `json:"rel_path"`
}

// skippedDirs are repository subtrees that never hold kit artifacts but can be
// large or numerous.
var skippedDirs = map[string]bool{
	".git":         true,
	".github":      true,
	".agents":      true,
	".claude":      true,
	"node_modules": true,
	"vendor":       true,
	"testdata":     true,
}

// Discover returns every directory under root that contains a spec.yaml. A
// directory holding a spec.yaml is not descended into, so a kit's own files are
// never mistaken for nested kits.
func Discover(root string) ([]Candidate, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("kit repository %q is not a directory", root)
	}
	var out []Candidate
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && skippedDirs[entry.Name()] {
			return fs.SkipDir
		}
		if !fileExists(filepath.Join(path, "spec.yaml")) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		out = append(out, Candidate{Name: filepath.Base(path), RelPath: filepath.ToSlash(rel)})
		return fs.SkipDir
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
