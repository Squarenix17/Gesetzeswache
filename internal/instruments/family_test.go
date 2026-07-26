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

func TestExpandForParent_sgb6_bsv(t *testing.T) {
	c := &FamilyCatalog{
		byParent: map[string][]FamilyRow{
			"sgb6": {{
				ConsumerParentID: "sgb6",
				SlugPrefix:       "bsv_",
				SectionHint:      "§ 160",
				Notes:            "Beitragssatz in der Rentenversicherung (BGBl. 2017 I Nr. 3976)",
			}},
		},
	}
	laws := []domain.Law{
		{ID: "bsv2015", GIIPath: "bsv_2015"},
		{ID: "bsv2018", GIIPath: "bsv_2018"},
	}
	got := c.ExpandForParent("sgb6", laws)
	if len(got) != 1 {
		t.Fatalf("got %d want 1: %+v", len(got), got)
	}
	li := got[0]
	if li.GIISlug != "bsv_2018" {
		t.Fatalf("GIISlug=%q want bsv_2018", li.GIISlug)
	}
	if li.ParentLawID != "sgb6" || li.Kind != "verordnung" || li.SectionHint != "§ 160" {
		t.Fatalf("fields: %+v", li)
	}
	if li.Coverage != CoverageSection {
		t.Fatalf("coverage=%q", li.Coverage)
	}
	if li.Source != "" {
		t.Fatalf("Source should be unset before merge, got %q", li.Source)
	}
}

func TestMerge_familyExpandedBsvSourceSeeded(t *testing.T) {
	expanded := []domain.LinkedInstrument{
		{
			ParentLawID: "sgb6",
			Kind:        "verordnung",
			GIISlug:     "bsv_2018",
			Notes:       "Beitragssatz in der Rentenversicherung (BGBl. 2017 I Nr. 3976)",
			SectionHint: "§ 160",
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

func TestExpandForParent_sgb6_rvbeitrsbek(t *testing.T) {
	c := &FamilyCatalog{
		byParent: map[string][]FamilyRow{
			"sgb6": {{
				ConsumerParentID: "sgb6",
				SlugPrefix:       "rvbeitrsbek_",
				SectionHint:      "§ 158",
				Notes:            "Bekanntmachung der Beitragssätze in der Rentenversicherung (BGBl. 2025 I Nr. 291)",
			}},
		},
	}
	laws := []domain.Law{
		{ID: "rvbeitrsbek2025", GIIPath: "rvbeitrsbek_2025"},
		{ID: "rvbeitrsbek2026", GIIPath: "rvbeitrsbek_2026"},
	}
	got := c.ExpandForParent("sgb6", laws)
	if len(got) != 1 {
		t.Fatalf("got %d want 1: %+v", len(got), got)
	}
	li := got[0]
	if li.GIISlug != "rvbeitrsbek_2026" {
		t.Fatalf("GIISlug=%q want rvbeitrsbek_2026", li.GIISlug)
	}
	if li.ParentLawID != "sgb6" || li.Kind != "verordnung" || li.SectionHint != "§ 158" {
		t.Fatalf("fields: %+v", li)
	}
	if li.Coverage != CoverageSection {
		t.Fatalf("coverage=%q", li.Coverage)
	}
	if li.Source != "" {
		t.Fatalf("Source should be unset before merge, got %q", li.Source)
	}
}

func TestExpandForParent_sgb6_twoFamilies(t *testing.T) {
	c := &FamilyCatalog{
		byParent: map[string][]FamilyRow{
			"sgb6": {
				{
					ConsumerParentID: "sgb6",
					SlugPrefix:       "bsv_",
					SectionHint:      "§ 160",
					Notes:            "Beitragssatz in der Rentenversicherung (BGBl. 2017 I Nr. 3976)",
				},
				{
					ConsumerParentID: "sgb6",
					SlugPrefix:       "rvbeitrsbek_",
					SectionHint:      "§ 158",
					Notes:            "Bekanntmachung der Beitragssätze in der Rentenversicherung (BGBl. 2025 I Nr. 291)",
				},
			},
		},
	}
	laws := []domain.Law{
		{ID: "bsv2015", GIIPath: "bsv_2015"},
		{ID: "bsv2018", GIIPath: "bsv_2018"},
		{ID: "rvbeitrsbek2025", GIIPath: "rvbeitrsbek_2025"},
		{ID: "rvbeitrsbek2026", GIIPath: "rvbeitrsbek_2026"},
	}
	got := c.ExpandForParent("sgb6", laws)
	if len(got) != 2 {
		t.Fatalf("got %d want 2: %+v", len(got), got)
	}
	if got[0].GIISlug != "bsv_2018" {
		t.Fatalf("first GIISlug=%q want bsv_2018", got[0].GIISlug)
	}
	if got[1].GIISlug != "rvbeitrsbek_2026" {
		t.Fatalf("second GIISlug=%q want rvbeitrsbek_2026", got[1].GIISlug)
	}
}

func TestMerge_familyExpandedRvbeitrsbekSourceSeeded(t *testing.T) {
	expanded := []domain.LinkedInstrument{
		{
			ParentLawID: "sgb6",
			Kind:        "verordnung",
			GIISlug:     "rvbeitrsbek_2026",
			Notes:       "Bekanntmachung der Beitragssätze in der Rentenversicherung (BGBl. 2025 I Nr. 291)",
			SectionHint: "§ 158",
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

func TestLoadFamiliesTSV_repoFile_sgb6HasRvbeitrsbek(t *testing.T) {
	path, ok := repoRootVariantsPath("fortschreibung_families.tsv")
	if !ok {
		t.Skip("module root not found (run from gesetzeswache checkout)")
	}
	c, err := LoadFamiliesTSV(path)
	if err != nil {
		t.Fatal(err)
	}
	got := c.ForParent("sgb6")
	var found bool
	for _, row := range got {
		if row.SlugPrefix == "rvbeitrsbek_" && row.SectionHint == "§ 158" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("sgb6 missing rvbeitrsbek_ row with § 158: %+v", got)
	}
}

// repoRootVariantsPath returns variants/<name> under the module root (walk up for go.mod).
func repoRootVariantsPath(name string) (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "variants", name), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
