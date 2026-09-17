package fleet

import (
	"os"
	"path/filepath"
	"sort"
)

// EntityKind names the fleet object a loaded configuration file belongs to.
type EntityKind string

// Entity kinds used to key staleness by (kind, slug, file).
const (
	EntitySandbox    EntityKind = "sandbox"
	EntityProfile    EntityKind = "profile"
	EntityCache      EntityKind = "cache"
	EntitySkillStore EntityKind = "skill-store"
	EntityKitStore   EntityKind = "kit-store"
	EntityConfig     EntityKind = "config"
)

// StaleFile is one loaded file whose on-disk fingerprint changed.
type StaleFile struct {
	Kind EntityKind `json:"kind"`
	Slug string     `json:"slug"`
	Name string     `json:"name"`
	File string     `json:"file"`
	Path string     `json:"path"`
}

// Staleness reports whether any loaded file changed on disk since the last
// load, and which ones.
type Staleness struct {
	Stale   bool        `json:"stale"`
	Changed []StaleFile `json:"changed"`
}

// fileFingerprint is the cheap identity of a file: size and modification
// time. Detection stats files and never re-reads their content.
type fileFingerprint struct {
	size    int64
	modTime int64
}

// loadedFile records one file read (or attempted) by the last load.
type loadedFile struct {
	kind        EntityKind
	slug        string
	file        string
	path        string
	fingerprint fileFingerprint
}

// fingerprintOf stats path and returns its size and modification time.
func fingerprintOf(path string) (fileFingerprint, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fileFingerprint{}, false
	}
	return fileFingerprint{size: info.Size(), modTime: info.ModTime().UnixNano()}, true
}

// CheckStaleness stats every file recorded by the last load and reports the
// ones whose fingerprint changed, appeared or disappeared since. The check is
// cheap: it never re-reads file content, so the GUI can poll it.
func (f *Fleet) CheckStaleness() Staleness {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := Staleness{Changed: []StaleFile{}}
	for _, lf := range f.loaded {
		current, ok := fingerprintOf(lf.path)
		if ok && current == lf.fingerprint {
			continue
		}
		out.Changed = append(out.Changed, StaleFile{
			Kind: lf.kind,
			Slug: lf.slug,
			Name: f.entityLabel(lf.kind, lf.slug),
			File: lf.file,
			Path: lf.path,
		})
	}
	sort.Slice(out.Changed, func(i, j int) bool {
		a, b := out.Changed[i], out.Changed[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Slug != b.Slug {
			return a.Slug < b.Slug
		}
		return a.File < b.File
	})
	out.Stale = len(out.Changed) > 0
	return out
}

// entityLabel is the display name of one loaded entity, falling back to its
// slug when the file failed to load.
func (f *Fleet) entityLabel(kind EntityKind, slug string) string {
	switch kind {
	case EntitySandbox:
		if s, ok := f.sandboxes[slug]; ok {
			return s.Name()
		}
	case EntityProfile:
		if p, ok := f.profiles[slug]; ok {
			return p.Label()
		}
	case EntityCache:
		if c, ok := f.caches[slug]; ok {
			return c.Label()
		}
	case EntitySkillStore:
		if r, ok := f.skillStores[slug]; ok {
			return r.Name
		}
	case EntityKitStore:
		if r, ok := f.kitStores[slug]; ok {
			return r.Name
		}
	case EntityConfig:
		return "global configuration"
	}
	return slug
}

// loadedSet accumulates the files one load pass read.
type loadedSet struct {
	files []loadedFile
}

// note records the current fingerprint of one file, read or not.
func (l *loadedSet) note(kind EntityKind, slug, path string) {
	fp, ok := fingerprintOf(path)
	if !ok {
		return
	}
	l.files = append(l.files, loadedFile{
		kind: kind, slug: slug, file: filepath.Base(path), path: path, fingerprint: fp,
	})
}

// markWritten refreshes the fingerprints of the files one entity just wrote.
// The caller holds f.mu. A file that vanished is dropped instead.
func (f *Fleet) markWritten(kind EntityKind, slug, dir string) {
	for _, file := range entityFiles(kind) {
		f.markFile(kind, slug, file, filepath.Join(dir, file))
	}
}

// markFile refreshes one recorded file. The caller holds f.mu.
func (f *Fleet) markFile(kind EntityKind, slug, file, path string) {
	fp, ok := fingerprintOf(path)
	if !ok {
		f.forgetFile(kind, slug, file)
		return
	}
	for i := range f.loaded {
		if f.loaded[i].path == path {
			f.loaded[i] = loadedFile{kind: kind, slug: slug, file: file, path: path, fingerprint: fp}
			return
		}
	}
	f.loaded = append(f.loaded, loadedFile{kind: kind, slug: slug, file: file, path: path, fingerprint: fp})
}

// forgetFile drops one recorded file. The caller holds f.mu.
func (f *Fleet) forgetFile(kind EntityKind, slug, file string) {
	kept := f.loaded[:0]
	for _, lf := range f.loaded {
		if lf.kind == kind && lf.slug == slug && lf.file == file {
			continue
		}
		kept = append(kept, lf)
	}
	f.loaded = kept
}

// forget drops every recorded file of one entity. The caller holds f.mu.
func (f *Fleet) forget(kind EntityKind, slug string) {
	kept := f.loaded[:0]
	for _, lf := range f.loaded {
		if lf.kind == kind && lf.slug == slug {
			continue
		}
		kept = append(kept, lf)
	}
	f.loaded = kept
}

// entityFiles lists the files an entity owns under its directory.
func entityFiles(kind EntityKind) []string {
	switch kind {
	case EntitySandbox, EntityProfile:
		return []string{specFile, sidecarFile}
	case EntityCache:
		return []string{sidecarFile}
	default:
		return nil
	}
}

// storeEntityKind maps a store kind onto its staleness kind.
func storeEntityKind(kind StoreKind) EntityKind {
	if kind == StoreKits {
		return EntityKitStore
	}
	return EntitySkillStore
}
