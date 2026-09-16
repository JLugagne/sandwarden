package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDiscoverMarketplace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".claude-plugin", "marketplace.json"),
		`{"name":"market","plugins":[{"name":"alpha","source":"./plugins/alpha"},{"name":"beta","source":"./plugins/beta"}]}`)
	writeTestFile(t, filepath.Join(root, "plugins", "alpha", ".claude-plugin", "plugin.json"), `{"name":"alpha"}`)
	writeTestFile(t, filepath.Join(root, "plugins", "alpha", "skills", "foo", "SKILL.md"),
		"---\nname: foo\ndescription: >\n  Foo skill does\n  useful things\n---\n# Foo\n")
	writeTestFile(t, filepath.Join(root, "plugins", "alpha", "commands", "deploy.md"),
		"---\ndescription: Deploy the app\n---\nDeploy it.\n")
	writeTestFile(t, filepath.Join(root, "plugins", "beta", "skills", "bar", "SKILL.md"),
		"---\nname: bar\ndescription: \"Bar skill\"\n---\n# Bar\n")

	items, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %+v", items)
	}
	if items[0].Kind != KindSkill || items[0].Name != "foo" || items[0].Plugin != "alpha" {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[0].Description != "Foo skill does useful things" {
		t.Fatalf("unexpected folded description: %q", items[0].Description)
	}
	if items[0].RelPath != "plugins/alpha/skills/foo" {
		t.Fatalf("unexpected rel path: %q", items[0].RelPath)
	}
	if items[1].Kind != KindCommand || items[1].Name != "deploy" || items[1].Description != "Deploy the app" {
		t.Fatalf("unexpected command item: %+v", items[1])
	}
	if items[1].RelPath != "plugins/alpha/commands/deploy.md" {
		t.Fatalf("unexpected command path: %q", items[1].RelPath)
	}
	if items[2].Kind != KindSkill || items[2].Name != "bar" || items[2].Plugin != "beta" {
		t.Fatalf("unexpected last item: %+v", items[2])
	}
}

func TestDiscoverLooseRepo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my-skills")
	writeTestFile(t, filepath.Join(root, "skills", "foo", "SKILL.md"), "# Foo\n")
	writeTestFile(t, filepath.Join(root, "commands", "run.md"), "Run it.\n")
	writeTestFile(t, filepath.Join(root, "skills", "notes", "readme.md"), "not a skill\n")
	writeTestFile(t, filepath.Join(root, "commands", "notes.txt"), "not a command\n")

	items, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %+v", items)
	}
	if items[0].Kind != KindSkill || items[0].Name != "foo" || items[0].Plugin != "my-skills" {
		t.Fatalf("unexpected skill: %+v", items[0])
	}
	if items[1].Kind != KindCommand || items[1].Name != "run" {
		t.Fatalf("unexpected command: %+v", items[1])
	}
}

func TestDiscoverKeepsFirstDuplicate(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "skills", "dup", "SKILL.md"), "---\ndescription: first\n---\n")
	writeTestFile(t, filepath.Join(root, "plugins", "extra", "skills", "dup", "SKILL.md"), "---\ndescription: second\n---\n")

	items, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %+v", items)
	}
	if items[0].Description != "first" {
		t.Fatalf("expected the first occurrence to win, got %+v", items[0])
	}
}

func TestDiscoverPluginsDirWithoutManifest(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "plugins", "solo", "skills", "s", "SKILL.md"), "# S\n")

	items, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(items) != 1 || items[0].Name != "s" || items[0].Plugin != "solo" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestReadFrontmatterVariants(t *testing.T) {
	root := t.TempDir()
	cases := map[string]string{
		"plain.md":   "---\ndescription: plain value\n---\nbody\n",
		"quoted.md":  "---\ndescription: \"quoted value\"\n---\nbody\n",
		"literal.md": "---\ndescription: |\n  line one\n  line two\nother: x\n---\nbody\n",
		"none.md":    "no frontmatter here\n",
	}
	for name, content := range cases {
		writeTestFile(t, filepath.Join(root, name), content)
	}

	if got := readFrontmatter(filepath.Join(root, "plain.md")).description; got != "plain value" {
		t.Fatalf("plain: %q", got)
	}
	if got := readFrontmatter(filepath.Join(root, "quoted.md")).description; got != "quoted value" {
		t.Fatalf("quoted: %q", got)
	}
	if got := readFrontmatter(filepath.Join(root, "literal.md")).description; got != "line one line two" {
		t.Fatalf("literal: %q", got)
	}
	if got := readFrontmatter(filepath.Join(root, "none.md")).description; got != "" {
		t.Fatalf("none: %q", got)
	}
}
