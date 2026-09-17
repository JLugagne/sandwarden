package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/store"
)

func TestSearchRanksMetadataAboveBody(t *testing.T) {
	index, err := New([]Document{
		{Kind: KindSkill, Store: "s1", Name: "release notes", Description: "draft release notes", Body: "kubernetes cluster upgrades"},
		{Kind: KindSkill, Store: "s2", Name: "kubernetes deploy", Description: "roll out workloads", Body: "restart pods"},
		{Kind: KindSkill, Store: "s3", Name: "python lint", Description: "ruff and friends"},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("kubernetes", 10)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(results), results)
	}
	if results[0].Store != "s2" {
		t.Fatalf("name match must rank first: %+v", results)
	}
}

func TestSearchExpandsPrefixes(t *testing.T) {
	index, err := New([]Document{
		{Kind: KindSkill, Store: "s3", Name: "kubernetes deploy"},
		{Kind: KindCommand, Store: "s4", Name: "review", Description: "code review helper"},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("kub", 10)
	if len(results) != 1 || results[0].Store != "s3" {
		t.Fatalf("prefix query must find kubernetes: %+v", results)
	}
}

func TestSearchDropsShortQueries(t *testing.T) {
	index, err := New([]Document{{Kind: KindSkill, Store: "s5", Name: "kubernetes deploy"}})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	if got := index.Search("k", 10); got != nil {
		t.Fatalf("single-character query must return nothing: %+v", got)
	}
	if got := index.Search("  ", 10); got != nil {
		t.Fatalf("blank query must return nothing: %+v", got)
	}
}

func TestSearchSnippetShowsBodyMatch(t *testing.T) {
	body := strings.Repeat("filler ", 60) + "migrate postgres schemas safely" + strings.Repeat(" filler", 60)
	index, err := New([]Document{
		{Kind: KindKit, Store: "s6", Name: "db-tools", Body: body},
		{Kind: KindKit, Store: "s7", Name: "other-tools", Body: "unrelated text"},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("postgres", 10)
	if len(results) != 1 {
		t.Fatalf("want one result, got %+v", results)
	}
	if !strings.Contains(results[0].Snippet, "postgres") {
		t.Fatalf("snippet must show the body match: %q", results[0].Snippet)
	}
}

func TestSearchLimit(t *testing.T) {
	docs := make([]Document, 0, 10)
	for range 5 {
		docs = append(docs, Document{Kind: KindSkill, Store: "golang", Name: "golang tooling"})
	}
	for range 5 {
		docs = append(docs, Document{Kind: KindSkill, Store: "python", Name: "python tooling"})
	}
	index, err := New(docs)
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	if got := index.Search("golang", 3); len(got) != 3 {
		t.Fatalf("limit ignored: got %d results", len(got))
	}
}

func TestSearchFallsBackOnUbiquitousTerms(t *testing.T) {
	index, err := New([]Document{{Kind: KindSkill, Store: "s8", Name: "only skill", Description: "the only one"}})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("only", 10)
	if len(results) != 1 || results[0].Store != "s8" {
		t.Fatalf("fallback must find the single document: %+v", results)
	}
	if results[0].Score <= 0 {
		t.Fatalf("fallback score must be positive: %+v", results[0])
	}
}

func TestCollectReadsCheckoutBodies(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	skillCheckout := t.TempDir()
	skillDir := filepath.Join(skillCheckout, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: ship things\n---\nRoll out kubernetes workloads."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := st.ReplaceSkillItems(ctx, "acme", []store.SkillItem{{Store: "acme", Kind: "skill", Name: "deploy", Description: "ship things", RelPath: "skills/deploy"}}); err != nil {
		t.Fatalf("replace skill items: %v", err)
	}

	kitCheckout := t.TempDir()
	kitDir := filepath.Join(kitCheckout, "code-server")
	if err := os.MkdirAll(kitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kitDir, "spec.yaml"), []byte("name: code-server\ndescription: web IDE in the sandbox\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := st.ReplaceKitItems(ctx, "contrib", []store.KitItem{{Store: "contrib", Kind: "mixin", Name: "code-server", Description: "web IDE", RelPath: "code-server"}}); err != nil {
		t.Fatalf("replace kit items: %v", err)
	}

	paths := map[string]string{"skill:acme": skillCheckout, "kit:contrib": kitCheckout}
	docs, err := Collect(ctx, st, paths)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("want 2 documents, got %d: %+v", len(docs), docs)
	}
	bodies := map[Kind]string{}
	for _, doc := range docs {
		bodies[doc.Kind] = doc.Body
	}
	if !strings.Contains(bodies[KindSkill], "kubernetes workloads") {
		t.Fatalf("skill body not read: %q", bodies[KindSkill])
	}
	if !strings.Contains(bodies[KindKit], "web IDE in the sandbox") {
		t.Fatalf("kit body not read: %q", bodies[KindKit])
	}

	index, err := Build(ctx, st, paths, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := index.Search("kubernetes", 10); len(got) != 1 || got[0].Name != "deploy" {
		t.Fatalf("body search missed the skill: %+v", got)
	}
}

func TestSearchFindsConfigEntities(t *testing.T) {
	index, err := New([]Document{
		{Kind: KindSandbox, Name: "frontend-box", DisplayName: "Frontend Box", Slug: "frontend-box", Description: "UI work sandbox", Agent: "claude", Refs: []string{"Net Allow", "net-allow", "Go module cache", "go-mod"}},
		{Kind: KindProfile, Name: "Net Allow", Slug: "net-allow", Description: "corporate egress allowlist", Refs: []string{"api.github.com", "telemetry.example.com"}},
		{Kind: KindCache, Name: "Go module cache", Slug: "go-mod", Description: "shared downloads", Refs: []string{"/home/agent/go/pkg/mod"}},
		{Kind: KindSkill, Store: "acme", Name: "deploy", Description: "ship things"},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	cases := []struct {
		query string
		kind  Kind
		name  string
		slug  string
	}{
		{"frontend", KindSandbox, "frontend-box", "frontend-box"},
		{"ui work", KindSandbox, "frontend-box", "frontend-box"},
		{"claude", KindSandbox, "frontend-box", "frontend-box"},
		{"corporate", KindProfile, "Net Allow", "net-allow"},
		{"api.github.com", KindProfile, "Net Allow", "net-allow"},
		{"module", KindCache, "Go module cache", "go-mod"},
		{"pkg", KindCache, "Go module cache", "go-mod"},
	}
	for _, tc := range cases {
		results := index.Search(tc.query, 10)
		if len(results) == 0 || results[0].Kind != tc.kind || results[0].Name != tc.name || results[0].Slug != tc.slug {
			t.Fatalf("query %q must rank the %s first: %+v", tc.query, tc.kind, results)
		}
	}
}

func TestSearchConfigSnippetShowsReferences(t *testing.T) {
	index, err := New([]Document{
		{Kind: KindCache, Name: "Go module cache", Slug: "go-mod", Refs: []string{"/home/agent/go/pkg/mod"}},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("pkg", 10)
	if len(results) != 1 {
		t.Fatalf("want one result, got %+v", results)
	}
	if !strings.Contains(results[0].Snippet, "pkg") {
		t.Fatalf("snippet must show the reference match: %q", results[0].Snippet)
	}
}

func TestFingerprintTracksDocumentChanges(t *testing.T) {
	docs := []Document{
		{Kind: KindProfile, Name: "Net Allow", Slug: "net-allow", Description: "corp allowlist", Refs: []string{"api.github.com"}},
	}
	before := Fingerprint(docs)
	if again := Fingerprint(docs); again != before {
		t.Fatalf("fingerprint must be stable: %q != %q", again, before)
	}
	renamed := []Document{
		{Kind: KindProfile, Name: "Egress Allowlist", Slug: "net-allow", Description: "corp allowlist", Refs: []string{"api.github.com"}},
	}
	if Fingerprint(renamed) == before {
		t.Fatal("fingerprint must change when a display name changes")
	}
	withRule := []Document{
		{Kind: KindProfile, Name: "Net Allow", Slug: "net-allow", Description: "corp allowlist", Refs: []string{"api.github.com", "telemetry.example.com"}},
	}
	if Fingerprint(withRule) == before {
		t.Fatal("fingerprint must change when a reference is added")
	}
	moved := []Document{
		{Kind: KindProfile, Name: "Net Allow", Slug: "egress", Description: "corp allowlist", Refs: []string{"api.github.com"}},
	}
	if Fingerprint(moved) == before {
		t.Fatal("fingerprint must change when a slug moves")
	}
}

func TestBuildMergesConfigDocuments(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	index, err := Build(ctx, st, nil, []Document{
		{Kind: KindProfile, Name: "Net Allow", Slug: "net-allow", Refs: []string{"api.github.com"}},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	results := index.Search("allow", 10)
	if len(results) != 1 || results[0].Kind != KindProfile || results[0].Name != "Net Allow" || results[0].Slug != "net-allow" {
		t.Fatalf("config document was not indexed: %+v", results)
	}
}
