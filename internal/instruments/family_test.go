package instruments

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/discovery"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestLatestSlugByPrefix_prefersNewerYear(t *testing.T) {
	laws := []domain.Law{
		{ID: "rbsfv2025", GIIPath: "rbsfv_2025"},
		{ID: "rbsfv2026", GIIPath: "rbsfv_2026"},
	}
	slug, ok := LatestSlugByPrefix(laws, "rbsfv_")
	if !ok {
		t.Fatal("expected match")
	}
	if slug != "rbsfv_2026" {
		t.Fatalf("slug=%q want rbsfv_2026", slug)
	}
}

func TestLatestSlugByPrefix_empty(t *testing.T) {
	_, ok := LatestSlugByPrefix(nil, "rbsfv_")
	if ok {
		t.Fatal("expected ok=false for empty laws")
	}
	_, ok = LatestSlugByPrefix([]domain.Law{{GIIPath: "bgb"}}, "rbsfv_")
	if ok {
		t.Fatal("expected ok=false when no prefix match")
	}
}

func TestLoadFamiliesTSV_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "families.tsv")
	content := `# comment
sgb2	rbsfv_	§ 20	Regelbedarfsstufen Fortschreibung (BGBl. 2025 I Nr. 243)
asylblg	rbsfv_	§ 3	Regelbedarfsstufen Fortschreibung (BGBl. 2025 I Nr. 243)
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFamiliesTSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 2 {
		t.Fatalf("Len()=%d want 2", c.Len())
	}
	got := c.ForParent("sgb2")
	if len(got) != 1 || got[0].SlugPrefix != "rbsfv_" || got[0].SectionHint != "§ 20" {
		t.Fatalf("sgb2: %+v", got)
	}
}

func TestLoadFamiliesTSV_missing(t *testing.T) {
	_, err := LoadFamiliesTSV(filepath.Join(t.TempDir(), "nope.tsv"))
	if err == nil {
		t.Fatal("expected error for missing configured path")
	}
}

func TestLoadFamiliesTSV_emptyPath(t *testing.T) {
	c, err := LoadFamiliesTSV("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Fatal("expected empty catalog")
	}
}

func TestExpandForParent_sgb2(t *testing.T) {
	c := &FamilyCatalog{
		byParent: map[string][]FamilyRow{
			"sgb2": {{
				ConsumerParentID: "sgb2",
				SlugPrefix:       "rbsfv_",
				SectionHint:      "§ 20",
				Notes:            "Regelbedarfsstufen Fortschreibung (BGBl. 2025 I Nr. 243)",
			}},
		},
	}
	laws := []domain.Law{
		{ID: "rbsfv2025", GIIPath: "rbsfv_2025"},
		{ID: "rbsfv2026", GIIPath: "rbsfv_2026"},
	}
	got := c.ExpandForParent("sgb2", laws)
	if len(got) != 1 {
		t.Fatalf("got %d want 1: %+v", len(got), got)
	}
	li := got[0]
	if li.GIISlug != "rbsfv_2026" {
		t.Fatalf("GIISlug=%q want rbsfv_2026", li.GIISlug)
	}
	if li.ParentLawID != "sgb2" || li.Kind != "verordnung" || li.SectionHint != "§ 20" {
		t.Fatalf("fields: %+v", li)
	}
	if li.Coverage != CoverageSection {
		t.Fatalf("coverage=%q", li.Coverage)
	}
	if li.Source != "" {
		t.Fatalf("Source should be unset before merge, got %q", li.Source)
	}
}

func TestMerge_familyExpandedSourceSeeded(t *testing.T) {
	expanded := []domain.LinkedInstrument{
		{
			ParentLawID: "sgb2",
			Kind:        "verordnung",
			GIISlug:     "rbsfv_2026",
			Notes:       "Regelbedarfsstufen Fortschreibung (BGBl. 2025 I Nr. 243)",
			SectionHint: "§ 20",
			Coverage:    CoverageSection,
		},
	}
	got := discovery.Merge(expanded, nil)
	if len(got) != 1 {
		t.Fatalf("got %d want 1", len(got))
	}
	if got[0].Source != discovery.SourceSeeded {
		t.Fatalf("source=%q want %q", got[0].Source, discovery.SourceSeeded)
	}
}
