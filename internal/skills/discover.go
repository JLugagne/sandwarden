package skills

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Kind distinguishes the mountable item types of a plugin store.
type Kind string

const (
	// KindSkill is a skill directory containing a SKILL.md file.
	KindSkill Kind = "skill"
	// KindCommand is a markdown file under a plugin's commands/ directory.
	KindCommand Kind = "command"
)

// Item is one discovered skill or command of a store checkout.
type Item struct {
	Kind        Kind   `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Plugin      string `json:"plugin"`
	RelPath     string `json:"rel_path"`
}

// Discover returns every skill and command found under root, following the
// Anthropic plugin layout: a marketplace listing plugins, a single plugin, or
// loose skills/ and commands/ directories. Duplicate names keep their first
// occurrence so repeated scans of a checkout yield the same catalog.
func Discover(root string) ([]Item, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill store %q is not a directory", root)
	}
	var items []Item
	seen := make(map[string]bool)
	for _, ref := range pluginRefs(root) {
		scanPlugin(root, ref, seen, &items)
	}
	return items, nil
}

type pluginRef struct {
	name string
	dir  string
}

// pluginRefs resolves the plugin roots of a checkout: the entries of a
// marketplace manifest, the repository root when it carries items itself, and
// any directory under plugins/.
func pluginRefs(root string) []pluginRef {
	var refs []pluginRef
	seen := make(map[string]bool)
	add := func(name, dir string) {
		dir = filepath.Clean(dir)
		if seen[dir] {
			return
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return
		}
		seen[dir] = true
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(dir)
		}
		refs = append(refs, pluginRef{name: name, dir: dir})
	}

	for _, ref := range marketplacePlugins(root) {
		add(ref.name, ref.dir)
	}
	if hasItemsDir(root) || fileExists(filepath.Join(root, ".claude-plugin", "plugin.json")) {
		add(filepath.Base(root), root)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "plugins")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				add(entry.Name(), filepath.Join(root, "plugins", entry.Name()))
			}
		}
	}
	return refs
}

// marketplacePlugins reads .claude-plugin/marketplace.json and resolves each
// plugin's local source directory. Remote sources are ignored here; their
// content is expected inside the checkout.
func marketplacePlugins(root string) []pluginRef {
	raw, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return nil
	}
	var manifest struct {
		Plugins []struct {
			Name   string          `json:"name"`
			Source json.RawMessage `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	var refs []pluginRef
	for _, plugin := range manifest.Plugins {
		var source string
		if err := json.Unmarshal(plugin.Source, &source); err != nil || strings.TrimSpace(source) == "" {
			continue
		}
		if strings.Contains(source, "://") || strings.HasPrefix(source, "git@") {
			continue
		}
		dir := filepath.FromSlash(strings.TrimPrefix(source, "./"))
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		refs = append(refs, pluginRef{name: plugin.Name, dir: dir})
	}
	return refs
}

// scanPlugin collects the skills and commands of one plugin root: a SKILL.md
// at the root, skills/<name>/SKILL.md directories, and commands/*.md files.
func scanPlugin(root string, ref pluginRef, seen map[string]bool, items *[]Item) {
	if meta := filepath.Join(ref.dir, "SKILL.md"); fileExists(meta) {
		appendItem(items, seen, root, ref, KindSkill, filepath.Base(ref.dir), ref.dir, meta)
	}
	if entries, err := os.ReadDir(filepath.Join(ref.dir, "skills")); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(ref.dir, "skills", entry.Name())
			meta := filepath.Join(dir, "SKILL.md")
			if !fileExists(meta) {
				continue
			}
			appendItem(items, seen, root, ref, KindSkill, entry.Name(), dir, meta)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(ref.dir, "commands")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			path := filepath.Join(ref.dir, "commands", entry.Name())
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			appendItem(items, seen, root, ref, KindCommand, name, path, path)
		}
	}
}

func appendItem(items *[]Item, seen map[string]bool, root string, ref pluginRef, kind Kind, name, path, metaPath string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	key := string(kind) + "\x00" + strings.ToLower(name)
	if seen[key] {
		return
	}
	seen[key] = true
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	*items = append(*items, Item{
		Kind:        kind,
		Name:        name,
		Description: readFrontmatter(metaPath).description,
		Plugin:      ref.name,
		RelPath:     filepath.ToSlash(rel),
	})
}

// metadata is the subset of a SKILL.md or command frontmatter the catalog
// shows in the UI.
type metadata struct {
	description string
}

// readFrontmatter extracts the description from a leading YAML frontmatter
// block. It intentionally implements only the flat key/value subset used by
// SKILL.md and command files, including folded and literal block scalars.
func readFrontmatter(path string) metadata {
	file, err := os.Open(path)
	if err != nil {
		return metadata{}
	}
	defer file.Close()

	scanner := bufio.NewScanner(io.LimitReader(file, 64*1024))
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return metadata{}
	}
	var meta metadata
	var key, indent string
	var block []string
	flush := func() {
		if key != "" {
			setMeta(&meta, key, strings.TrimSpace(strings.Join(block, " ")))
		}
		key, indent, block = "", "", nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if key != "" {
			if trimmed == "" || leadingWhitespace(line) > len(indent) {
				block = append(block, trimmed)
				continue
			}
			flush()
		}
		k, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		value = strings.TrimSpace(value)
		if isBlockScalar(value) {
			key, indent = k, line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			continue
		}
		setMeta(&meta, k, unquote(value))
	}
	flush()
	return meta
}

func setMeta(meta *metadata, key, value string) {
	if strings.EqualFold(strings.TrimSpace(key), "description") {
		meta.description = value
	}
}

func isBlockScalar(value string) bool {
	if value == "" {
		return true
	}
	switch value[0] {
	case '>', '|':
		return !strings.ContainsAny(value[1:], " \t")
	}
	return false
}

func leadingWhitespace(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func hasItemsDir(root string) bool {
	return dirExists(filepath.Join(root, "skills")) || dirExists(filepath.Join(root, "commands"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
