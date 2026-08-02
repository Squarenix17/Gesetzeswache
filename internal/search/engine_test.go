package search

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestResolveExactAbbr(t *testing.T) {
	e := NewEngine()
	e.Swap([]domain.Law{{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch", GIIPath: "bgb"}}, nil)
	best, _, _ := e.Current().Resolve("BGB", 0.75)
	if best == nil || best.Law.ID != "bgb" {
		t.Fatalf("expected bgb, got %+v", best)
	}
}

func TestResolveTypo(t *testing.T) {
	e := NewEngine()
	e.Swap([]domain.Law{{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch"}}, nil)
	best, sug, _ := e.Current().Resolve("BGBX", 0.75)
	if best == nil {
		// may fall to suggestions depending on score
		if len(sug) == 0 {
			t.Fatal("expected suggestion for near typo")
		}
	}
}

func TestResolveBelowThreshold(t *testing.T) {
	e := NewEngine()
	e.Swap([]domain.Law{{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch"}}, nil)
	best, sug, _ := e.Current().Resolve("zzzznotalaw", 0.75)
	if best != nil {
		t.Fatalf("expected no best, got %+v", best)
	}
	_ = sug
}

func TestVariant(t *testing.T) {
	e := NewEngine()
	e.Swap(
		[]domain.Law{{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch"}},
		[]domain.LawVariant{{Variant: "Zivilgesetzbuch", LawID: "bgb"}},
	)
	best, _, _ := e.Current().Resolve("Zivilgesetzbuch", 0.75)
	if best == nil || best.Law.ID != "bgb" {
		t.Fatalf("variant failed: %+v", best)
	}
}

func TestResolveSGB9_variantBeatsSubstring(t *testing.T) {
	e := NewEngine()
	e.Swap(
		[]domain.Law{
			{ID: "sgb92018", Abbreviation: "SGB IX", Title: "Sozialgesetzbuch IX"},
			{ID: "s_g", Abbreviation: "S_G", Title: "Some other law"},
		},
		[]domain.LawVariant{
			{Variant: "sgb9", LawID: "sgb92018"},
			{Variant: "SGB 9", LawID: "sgb92018"},
		},
	)
	best, _, _ := e.Current().Resolve("sgb9", 0.75)
	if best == nil || best.Law.ID != "sgb92018" {
		t.Fatalf("expected sgb92018 via variant, got %+v", best)
	}
}

func TestResolveDSGVO_noFalseSubstringMatch(t *testing.T) {
	e := NewEngine()
	e.Swap(
		[]domain.Law{
			{ID: "s_g", Abbreviation: "S_G", Title: "Some other law"},
		},
		nil,
	)
	best, sug, _ := e.Current().Resolve("dsgvo", 0.75)
	if best != nil {
		t.Fatalf("dsgvo must not match s_g via sg substring; got %+v", best)
	}
	_ = sug
}

func TestResolveVariantSGBXI(t *testing.T) {
	e := NewEngine()
	e.Swap(
		[]domain.Law{
			{ID: "sgb11", Abbreviation: "SGB XI", Title: "Sozialgesetzbuch XI"},
			{ID: "sgb1", Abbreviation: "SGB I", Title: "Sozialgesetzbuch I"},
		},
		[]domain.LawVariant{{Variant: "SGB XI", LawID: "sgb11"}},
	)
	best, _, _ := e.Current().Resolve("SGB XI", 0.75)
	if best == nil || best.Law.ID != "sgb11" {
		t.Fatalf("expected sgb11, got %+v", best)
	}
}
