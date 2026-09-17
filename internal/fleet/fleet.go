// Package fleet is the filesystem-backed source of truth for sandwarden:
// sandboxes, profiles, caches, git store registrations and the global
// configuration live as files under one directory. The SQLite store only
// keeps derived catalogs and the applied-rules ledger.
package fleet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FileError is a per-file load error. A broken file is reported but never
// takes the rest of the fleet down.
type FileError struct {
	Path string
	Err  error
}

// Error implements error.
func (e FileError) Error() string {
	return e.Path + ": " + e.Err.Error()
}

// Unwrap exposes the underlying error.
func (e FileError) Unwrap() error { return e.Err }

// Sandbox is one sandbox directory: a valid kit spec plus its sidecar.
type Sandbox struct {
	Slug string
	Dir  string
	Spec Spec
	App  SandboxApp
}

// Name is the canonical sandboxd name.
func (s *Sandbox) Name() string {
	if strings.TrimSpace(s.App.Sandbox) != "" {
		return s.App.Sandbox
	}
	return s.Spec.DisplayName
}

// Label is the display name shown in the UI.
func (s *Sandbox) Label() string {
	if strings.TrimSpace(s.Spec.DisplayName) != "" {
		return s.Spec.DisplayName
	}
	return s.Name()
}

// Clone returns a deep copy safe to hand to other goroutines.
func (s *Sandbox) Clone() *Sandbox {
	if s == nil {
		return nil
	}
	c := *s
	c.Spec = s.Spec.Clone()
	c.App = s.App.Clone()
	return &c
}

// Profile is one profile directory: a valid kit spec carrying the network
// rules plus its sidecar.
type Profile struct {
	Slug string
	Dir  string
	Spec Spec
	App  ProfileApp
}

// Label is the display name shown in the UI.
func (p *Profile) Label() string {
	if strings.TrimSpace(p.Spec.DisplayName) != "" {
		return p.Spec.DisplayName
	}
	return p.Slug
}

// Clone returns a deep copy safe to hand to other goroutines.
func (p *Profile) Clone() *Profile {
	if p == nil {
		return nil
	}
	c := *p
	c.Spec = p.Spec.Clone()
	c.App = p.App.Clone()
	return &c
}

// Cache is one shared cache directory (sidecar only: a cache is a host bind
// mount, which no kit can express).
type Cache struct {
	Slug string
	Dir  string
	App  CacheApp
}

// Label is the display name shown in the UI.
func (c *Cache) Label() string { return c.App.Name }

// Clone returns a deep copy safe to hand to other goroutines.
func (c *Cache) Clone() *Cache {
	if c == nil {
		return nil
	}
	copy := *c
	copy.App = c.App.Clone()
	return &copy
}

// Fleet indexes the configuration directory. Its mutators write one file at a
// time and take no advisory lock: callers wrap a whole read-modify-write cycle
// in AcquireLock (see internal/app), because locking each save separately would
// still let two processes overwrite each other's changes.
type Fleet struct {
	dir         string
	mu          sync.RWMutex
	sandboxes   map[string]*Sandbox
	profiles    map[string]*Profile
	caches      map[string]*Cache
	skillStores map[string]*StoreReg
	kitStores   map[string]*StoreReg
	cfg         AppConfig
	errs        []FileError
	// loaded fingerprints every file read by the last load, so
	// CheckStaleness can detect outside edits without re-reading content.
	loaded []loadedFile
}

// DefaultDir resolves the configuration directory: $SANDWARDEN_CONFIG_DIR,
// then $XDG_CONFIG_HOME/sandwarden, then ~/.config/sandwarden.
func DefaultDir() string {
	if v := strings.TrimSpace(os.Getenv("SANDWARDEN_CONFIG_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return filepath.Join(v, "sandwarden")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "sandwarden")
	}
	return filepath.Join(home, ".config", "sandwarden")
}

// Open indexes dir, creating the standard layout when missing.
func Open(dir string) (*Fleet, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = DefaultDir()
	}
	for _, sub := range []string{sandboxesDir, profilesDir, cachesDir,
		filepath.Join(storesDir, StoreSkills.dir()), filepath.Join(storesDir, StoreKits.dir())} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("create config dir: %w", err)
		}
	}
	f := &Fleet{dir: dir}
	if err := f.Reload(); err != nil {
		return nil, err
	}
	return f, nil
}

// Dir is the configuration directory.
func (f *Fleet) Dir() string { return f.dir }

// Errors returns the per-file load errors of the last reload.
func (f *Fleet) Errors() []FileError {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]FileError(nil), f.errs...)
}

// Reload re-reads every file from disk, replacing the in-memory index.
// Reload re-reads every file from disk, replacing the in-memory index.
func (f *Fleet) Reload() error {
	loaded := &loadedSet{}
	sandboxes, errs := f.loadSandboxes(loaded)
	profiles, perrs := f.loadProfiles(loaded)
	caches, cerrs := f.loadCaches(loaded)
	skillStores, serrs := f.loadStores(StoreSkills, loaded)
	kitStores, kerrs := f.loadStores(StoreKits, loaded)
	errs = append(errs, perrs...)
	errs = append(errs, cerrs...)
	errs = append(errs, serrs...)
	errs = append(errs, kerrs...)

	var cfg AppConfig
	cfgPath := filepath.Join(f.dir, configFile)
	if fileExists(cfgPath) {
		loaded.note(EntityConfig, "", cfgPath)
		if err := readYAML(cfgPath, &cfg); err != nil {
			errs = append(errs, FileError{Path: cfgPath, Err: err})
			cfg = AppConfig{}
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.sandboxes = sandboxes
	f.profiles = profiles
	f.caches = caches
	f.skillStores = skillStores
	f.kitStores = kitStores
	f.cfg = cfg
	f.errs = errs
	f.loaded = loaded.files
	return nil
}

func (f *Fleet) loadSandboxes(loaded *loadedSet) (map[string]*Sandbox, []FileError) {
	root := filepath.Join(f.dir, sandboxesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Sandbox{}, nil
		}
		return map[string]*Sandbox{}, []FileError{{Path: root, Err: err}}
	}
	out := make(map[string]*Sandbox, len(entries))
	var errs []FileError
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		specPath := filepath.Join(dir, specFile)
		sidecarPath := filepath.Join(dir, sidecarFile)
		loaded.note(EntitySandbox, e.Name(), specPath)
		loaded.note(EntitySandbox, e.Name(), sidecarPath)
		var spec Spec
		if err := readYAML(specPath, &spec); err != nil {
			errs = append(errs, FileError{Path: specPath, Err: err})
			continue
		}
		var app SandboxApp
		if err := readYAML(sidecarPath, &app); err != nil {
			errs = append(errs, FileError{Path: sidecarPath, Err: err})
			continue
		}
		if strings.TrimSpace(app.Sandbox) == "" {
			app.Sandbox = spec.DisplayName
		}
		sb := &Sandbox{Slug: e.Name(), Dir: dir, Spec: spec, App: app}
		sb.normalize()
		out[e.Name()] = sb
	}
	return out, errs
}

func (f *Fleet) loadProfiles(loaded *loadedSet) (map[string]*Profile, []FileError) {
	root := filepath.Join(f.dir, profilesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Profile{}, nil
		}
		return map[string]*Profile{}, []FileError{{Path: root, Err: err}}
	}
	out := make(map[string]*Profile, len(entries))
	var errs []FileError
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		specPath := filepath.Join(dir, specFile)
		sidecarPath := filepath.Join(dir, sidecarFile)
		loaded.note(EntityProfile, e.Name(), specPath)
		loaded.note(EntityProfile, e.Name(), sidecarPath)
		var spec Spec
		if err := readYAML(specPath, &spec); err != nil {
			errs = append(errs, FileError{Path: specPath, Err: err})
			continue
		}
		var app ProfileApp
		if err := readYAML(sidecarPath, &app); err != nil {
			errs = append(errs, FileError{Path: sidecarPath, Err: err})
			continue
		}
		p := &Profile{Slug: e.Name(), Dir: dir, Spec: spec, App: app}
		p.normalize()
		out[e.Name()] = p
	}
	return out, errs
}

func (f *Fleet) loadCaches(loaded *loadedSet) (map[string]*Cache, []FileError) {
	root := filepath.Join(f.dir, cachesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Cache{}, nil
		}
		return map[string]*Cache{}, []FileError{{Path: root, Err: err}}
	}
	out := make(map[string]*Cache, len(entries))
	var errs []FileError
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		sidecarPath := filepath.Join(dir, sidecarFile)
		loaded.note(EntityCache, e.Name(), sidecarPath)
		var app CacheApp
		if err := readYAML(sidecarPath, &app); err != nil {
			errs = append(errs, FileError{Path: sidecarPath, Err: err})
			continue
		}
		c := &Cache{Slug: e.Name(), Dir: dir, App: app}
		c.normalize()
		out[e.Name()] = c
	}
	return out, errs
}

func (f *Fleet) loadStores(kind StoreKind, loaded *loadedSet) (map[string]*StoreReg, []FileError) {
	root := filepath.Join(f.dir, storesDir, kind.dir())
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*StoreReg{}, nil
		}
		return map[string]*StoreReg{}, []FileError{{Path: root, Err: err}}
	}
	out := make(map[string]*StoreReg, len(entries))
	var errs []FileError
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(root, e.Name())
		loaded.note(storeEntityKind(kind), strings.TrimSuffix(e.Name(), ".yaml"), path)
		var reg StoreReg
		if err := readYAML(path, &reg); err != nil {
			errs = append(errs, FileError{Path: path, Err: err})
			continue
		}
		reg.Kind = kind
		reg.Slug = strings.TrimSuffix(e.Name(), ".yaml")
		out[reg.Slug] = &reg
	}
	return out, errs
}

// Sandboxes returns every sandbox, sorted by slug.
func (f *Fleet) Sandboxes() []*Sandbox {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Sandbox, 0, len(f.sandboxes))
	for _, s := range f.sandboxes {
		out = append(out, s.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// Sandbox returns a sandbox by directory slug.
func (f *Fleet) Sandbox(slug string) (*Sandbox, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	s, ok := f.sandboxes[slug]
	return s.Clone(), ok
}

// SandboxByName returns a sandbox by canonical sandboxd name.
func (f *Fleet) SandboxByName(name string) (*Sandbox, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, s := range f.sandboxes {
		if s.Name() == name {
			return s.Clone(), true
		}
	}
	return nil, false
}

// CreateSandbox writes a new sandbox directory and returns it. The kit name
// becomes the directory slug; the canonical name stays in the sidecar.
func (f *Fleet) CreateSandbox(name string, spec Spec, app SandboxApp) (*Sandbox, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("sandbox name is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	slug := f.uniqueSandboxSlug(name)
	dir := filepath.Join(f.dir, sandboxesDir, slug)
	spec.SchemaVersion = "2"
	spec.Kind = "mixin"
	spec.Name = slug
	if strings.TrimSpace(spec.DisplayName) == "" {
		spec.DisplayName = name
	}
	app.Sandbox = name
	s := &Sandbox{Slug: slug, Dir: dir, Spec: spec, App: app}
	s.normalize()
	if err := s.write(); err != nil {
		return nil, err
	}
	f.sandboxes[slug] = s
	f.markWritten(EntitySandbox, slug, dir)
	return s.Clone(), nil
}

// SaveSandbox rewrites an existing sandbox directory.
func (f *Fleet) SaveSandbox(s *Sandbox) error {
	if s == nil || strings.TrimSpace(s.Slug) == "" {
		return errors.New("sandbox slug is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s.Dir = filepath.Join(f.dir, sandboxesDir, s.Slug)
	s.Spec.SchemaVersion = "2"
	s.Spec.Kind = "mixin"
	s.Spec.Name = s.Slug
	s.normalize()
	if err := s.write(); err != nil {
		return err
	}
	f.sandboxes[s.Slug] = s.Clone()
	f.markWritten(EntitySandbox, s.Slug, s.Dir)
	return nil
}

// DeleteSandbox removes a sandbox directory from disk.
func (f *Fleet) DeleteSandbox(slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.RemoveAll(filepath.Join(f.dir, sandboxesDir, slug)); err != nil {
		return err
	}
	delete(f.sandboxes, slug)
	f.forget(EntitySandbox, slug)
	return nil
}

func (s *Sandbox) write() error {
	if err := writeYAML(filepath.Join(s.Dir, specFile), sandboxHeader, s.Spec); err != nil {
		return err
	}
	return writeYAML(filepath.Join(s.Dir, sidecarFile), "", s.App)
}

func (f *Fleet) uniqueSandboxSlug(name string) string {
	return uniqueSlug(filepath.Join(f.dir, sandboxesDir), name)
}

// Profiles returns every profile, sorted by slug.
func (f *Fleet) Profiles() []*Profile {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Profile, 0, len(f.profiles))
	for _, p := range f.profiles {
		out = append(out, p.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// Profile returns a profile by directory slug.
func (f *Fleet) Profile(slug string) (*Profile, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	p, ok := f.profiles[slug]
	return p.Clone(), ok
}

// CreateProfile writes a new profile directory.
func (f *Fleet) CreateProfile(label string, spec Spec, app ProfileApp) (*Profile, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("profile name is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	slug := uniqueSlug(filepath.Join(f.dir, profilesDir), label)
	dir := filepath.Join(f.dir, profilesDir, slug)
	spec.SchemaVersion = "2"
	spec.Kind = "mixin"
	spec.Name = slug
	if strings.TrimSpace(spec.DisplayName) == "" {
		spec.DisplayName = label
	}
	p := &Profile{Slug: slug, Dir: dir, Spec: spec, App: app}
	p.normalize()
	if err := p.write(); err != nil {
		return nil, err
	}
	f.profiles[slug] = p
	f.markWritten(EntityProfile, slug, dir)
	return p.Clone(), nil
}

// SaveProfile rewrites an existing profile directory.
func (f *Fleet) SaveProfile(p *Profile) error {
	if p == nil || strings.TrimSpace(p.Slug) == "" {
		return errors.New("profile slug is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p.Dir = filepath.Join(f.dir, profilesDir, p.Slug)
	p.Spec.SchemaVersion = "2"
	p.Spec.Kind = "mixin"
	p.Spec.Name = p.Slug
	if strings.TrimSpace(p.Spec.DisplayName) == "" {
		p.Spec.DisplayName = p.Slug
	}
	p.normalize()
	if err := p.write(); err != nil {
		return err
	}
	f.profiles[p.Slug] = p.Clone()
	f.markWritten(EntityProfile, p.Slug, p.Dir)
	return nil
}

// DeleteProfile removes a profile directory from disk.
func (f *Fleet) DeleteProfile(slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.RemoveAll(filepath.Join(f.dir, profilesDir, slug)); err != nil {
		return err
	}
	delete(f.profiles, slug)
	f.forget(EntityProfile, slug)
	return nil
}

func (p *Profile) write() error {
	if err := writeYAML(filepath.Join(p.Dir, specFile), profileHeader, p.Spec); err != nil {
		return err
	}
	return writeYAML(filepath.Join(p.Dir, sidecarFile), "", p.App)
}

// Caches returns every cache, sorted by slug.
func (f *Fleet) Caches() []*Cache {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Cache, 0, len(f.caches))
	for _, c := range f.caches {
		out = append(out, c.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// Cache returns a cache by directory slug.
func (f *Fleet) Cache(slug string) (*Cache, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	c, ok := f.caches[slug]
	return c.Clone(), ok
}

// CreateCache writes a new cache directory.
func (f *Fleet) CreateCache(label string, app CacheApp) (*Cache, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("cache name is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	slug := uniqueSlug(filepath.Join(f.dir, cachesDir), label)
	dir := filepath.Join(f.dir, cachesDir, slug)
	app.Name = label
	c := &Cache{Slug: slug, Dir: dir, App: app}
	c.normalize()
	if err := c.write(); err != nil {
		return nil, err
	}
	f.caches[slug] = c
	f.markWritten(EntityCache, slug, dir)
	return c.Clone(), nil
}

// SaveCache rewrites an existing cache directory.
func (f *Fleet) SaveCache(c *Cache) error {
	if c == nil || strings.TrimSpace(c.Slug) == "" {
		return errors.New("cache slug is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c.Dir = filepath.Join(f.dir, cachesDir, c.Slug)
	c.normalize()
	if err := c.write(); err != nil {
		return err
	}
	f.caches[c.Slug] = c.Clone()
	f.markWritten(EntityCache, c.Slug, c.Dir)
	return nil
}

// DeleteCache removes a cache directory from disk.
func (f *Fleet) DeleteCache(slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.RemoveAll(filepath.Join(f.dir, cachesDir, slug)); err != nil {
		return err
	}
	delete(f.caches, slug)
	f.forget(EntityCache, slug)
	return nil
}

func (c *Cache) write() error {
	return writeYAML(filepath.Join(c.Dir, sidecarFile), cacheHeader, c.App)
}

// Stores returns the registrations of one kind, sorted by slug.
func (f *Fleet) Stores(kind StoreKind) []*StoreReg {
	f.mu.RLock()
	defer f.mu.RUnlock()
	src := f.skillStores
	if kind == StoreKits {
		src = f.kitStores
	}
	out := make([]*StoreReg, 0, len(src))
	for _, r := range src {
		out = append(out, r.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// Store returns one registration.
func (f *Fleet) Store(kind StoreKind, slug string) (*StoreReg, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	src := f.skillStores
	if kind == StoreKits {
		src = f.kitStores
	}
	r, ok := src[slug]
	return r.Clone(), ok
}

// CreateStore writes a new store registration.
func (f *Fleet) CreateStore(reg StoreReg) (*StoreReg, error) {
	reg.Name = strings.TrimSpace(reg.Name)
	if reg.Name == "" {
		return nil, errors.New("store name is required")
	}
	if strings.TrimSpace(reg.URL) == "" {
		return nil, errors.New("store url is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	root := f.storeRoot(reg.Kind)
	reg.Slug = uniqueSlug(root, reg.Name)
	if err := reg.write(root); err != nil {
		return nil, err
	}
	f.storeMap(reg.Kind)[reg.Slug] = &reg
	f.markFile(storeEntityKind(reg.Kind), reg.Slug, reg.Slug+".yaml", reg.path(root))
	return reg.Clone(), nil
}

// SaveStore rewrites an existing store registration.
func (f *Fleet) SaveStore(reg *StoreReg) error {
	if reg == nil || strings.TrimSpace(reg.Slug) == "" {
		return errors.New("store slug is required")
	}
	if strings.TrimSpace(reg.URL) == "" {
		return errors.New("store url is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	root := f.storeRoot(reg.Kind)
	if err := reg.write(root); err != nil {
		return err
	}
	f.storeMap(reg.Kind)[reg.Slug] = reg.Clone()
	f.markFile(storeEntityKind(reg.Kind), reg.Slug, reg.Slug+".yaml", reg.path(root))
	return nil
}

// DeleteStore removes a store registration from disk.
func (f *Fleet) DeleteStore(kind StoreKind, slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.Remove(filepath.Join(f.storeRoot(kind), slug+".yaml")); err != nil && !os.IsNotExist(err) {
		return err
	}
	delete(f.storeMap(kind), slug)
	f.forget(storeEntityKind(kind), slug)
	return nil
}

func (f *Fleet) storeMap(kind StoreKind) map[string]*StoreReg {
	if kind == StoreKits {
		return f.kitStores
	}
	return f.skillStores
}

func (f *Fleet) storeRoot(kind StoreKind) string {
	return filepath.Join(f.dir, storesDir, kind.dir())
}

func (r *StoreReg) write(root string) error {
	return writeYAML(r.path(root), storeHeader, r)
}

// Config returns the global configuration.
func (f *Fleet) Config() AppConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cfg.Clone()
}

// SetConfig writes the global configuration.
func (f *Fleet) SetConfig(cfg AppConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := writeYAML(filepath.Join(f.dir, configFile), configHeader, cfg); err != nil {
		return err
	}
	f.cfg = cfg.Clone()
	f.markFile(EntityConfig, "", configFile, filepath.Join(f.dir, configFile))
	return nil
}

func uniqueSlug(root, name string) string {
	base := Slugify(name)
	for i := 1; ; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		if !dirExists(filepath.Join(root, candidate)) {
			return candidate
		}
	}
}

// path is the registration file of one store.
func (r *StoreReg) path(root string) string {
	return filepath.Join(root, r.Slug+".yaml")
}
