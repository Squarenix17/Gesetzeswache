package search

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestResolveExactAbbr(t *testing.T) {
	e := NewEngine()
	e.Swap([]domain.Law{{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch", GIIPath: "bgb"}}, nil)
	best, _ := e.Current().Resolve("BGB", 0.75)
	if best == nil || best.Law.ID != "bgb" {
		t.Fatalf("expected bgb, got %+v", best)
	}
}

func TestResolveTypo(t *testing.T) {
	e := NewEngine()
	e.Swap([]domain.Law{{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch"}}, nil)
	best, sug := e.Current().Resolve("BGBX", 0.75)
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
	best, sug := e.Current().Resolve("zzzznotalaw", 0.75)
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
	best, _ := e.Current().Resolve("Zivilgesetzbuch", 0.75)
	if best == nil || best.Law.ID != "bgb" {
		t.Fatalf("variant failed: %+v", best)
	}
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
	best, _ := e.Current().Resolve("SGB XI", 0.75)
	if best == nil || best.Law.ID != "sgb11" {
		t.Fatalf("expected sgb11, got %+v", best)
	}
}
