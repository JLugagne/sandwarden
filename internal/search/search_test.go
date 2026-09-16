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
		{Kind: KindSkill, ID: 1, Name: "release notes", Description: "draft release notes", Body: "kubernetes cluster upgrades"},
		{Kind: KindSkill, ID: 2, Name: "kubernetes deploy", Description: "roll out workloads", Body: "restart pods"},
		{Kind: KindSkill, ID: 3, Name: "python lint", Description: "ruff and friends"},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("kubernetes", 10)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(results), results)
	}
	if results[0].ID != 2 {
		t.Fatalf("name match must rank first: %+v", results)
	}
}

func TestSearchExpandsPrefixes(t *testing.T) {
	index, err := New([]Document{
		{Kind: KindSkill, ID: 3, Name: "kubernetes deploy"},
		{Kind: KindCommand, ID: 4, Name: "review", Description: "code review helper"},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("kub", 10)
	if len(results) != 1 || results[0].ID != 3 {
		t.Fatalf("prefix query must find kubernetes: %+v", results)
	}
}

func TestSearchDropsShortQueries(t *testing.T) {
	index, err := New([]Document{{Kind: KindSkill, ID: 5, Name: "kubernetes deploy"}})
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
		{Kind: KindKit, ID: 6, Name: "db-tools", Body: body},
		{Kind: KindKit, ID: 7, Name: "other-tools", Body: "unrelated text"},
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
	for i := range 5 {
		docs = append(docs, Document{Kind: KindSkill, ID: int64(i + 1), Name: "golang tooling"})
	}
	for i := range 5 {
		docs = append(docs, Document{Kind: KindSkill, ID: int64(i + 6), Name: "python tooling"})
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
	index, err := New([]Document{{Kind: KindSkill, ID: 8, Name: "only skill", Description: "the only one"}})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	results := index.Search("only", 10)
	if len(results) != 1 || results[0].ID != 8 {
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
	skillStore, err := st.CreateSkillStore(ctx, store.SkillStore{Name: "acme", URL: "https://example.com/acme.git", Path: skillCheckout})
	if err != nil {
		t.Fatalf("create skill store: %v", err)
	}
	if err := st.ReplaceSkillItems(ctx, skillStore.ID, []store.SkillItem{{Kind: "skill", Name: "deploy", Description: "ship things", RelPath: "skills/deploy"}}); err != nil {
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
	kitStore, err := st.CreateKitStore(ctx, store.KitStore{Name: "contrib", URL: "https://example.com/kits.git", Path: kitCheckout})
	if err != nil {
		t.Fatalf("create kit store: %v", err)
	}
	if err := st.ReplaceKitItems(ctx, kitStore.ID, []store.KitItem{{Kind: "mixin", Name: "code-server", Description: "web IDE", RelPath: "code-server"}}); err != nil {
		t.Fatalf("replace kit items: %v", err)
	}

	docs, err := Collect(ctx, st)
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

	index, err := Build(ctx, st)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := index.Search("kubernetes", 10); len(got) != 1 || got[0].Name != "deploy" {
		t.Fatalf("body search missed the skill: %+v", got)
	}
}
