package store

import (
	"context"
	"testing"
)

func TestCatalogSizesCountsPerStore(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	if err := st.ReplaceSkillItems(ctx, "alpha", []SkillItem{
		{Store: "alpha", Kind: "skill", Name: "pdf", RelPath: "skills/pdf"},
		{Store: "alpha", Kind: "command", Name: "review", RelPath: "commands/review.md"},
	}); err != nil {
		t.Fatalf("ReplaceSkillItems: %v", err)
	}
	if err := st.ReplaceSkillItems(ctx, "beta", []SkillItem{
		{Store: "beta", Kind: "skill", Name: "deploy", RelPath: "skills/deploy"},
	}); err != nil {
		t.Fatalf("ReplaceSkillItems: %v", err)
	}
	if err := st.ReplaceKitItems(ctx, "kits", []KitItem{
		{Store: "kits", Kind: "mixin", Name: "go-lint", RelPath: "go-lint"},
	}); err != nil {
		t.Fatalf("ReplaceKitItems: %v", err)
	}

	skills, err := st.CatalogSizes(ctx, "skill")
	if err != nil {
		t.Fatalf("CatalogSizes skill: %v", err)
	}
	if skills["alpha"] != 2 || skills["beta"] != 1 || len(skills) != 2 {
		t.Fatalf("skill sizes = %+v", skills)
	}
	kits, err := st.CatalogSizes(ctx, "kit")
	if err != nil {
		t.Fatalf("CatalogSizes kit: %v", err)
	}
	if kits["kits"] != 1 || len(kits) != 1 {
		t.Fatalf("kit sizes = %+v", kits)
	}
}
