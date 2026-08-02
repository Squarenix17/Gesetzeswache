package search

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestResolveSGBV_prefersSGB5OverAMNutzenV(t *testing.T) {
	e := NewEngine()
	e.Swap([]domain.Law{
		{
			ID: "sgb5", Abbreviation: "SGB_5", Title: "Sozialgesetzbuch (SGB) Fünftes Buch (V)",
			GIIPath: "sgb_5",
		},
		{
			ID: "amnutzenv", Abbreviation: "AMNutzenV", Title: "Anmeldung zur Nutzung von Verkehrswegen",
			GIIPath: "amnutzenv",
		},
	}, nil)
	best, _, ambiguous := e.Current().Resolve("SGB V", 0.75)
	if ambiguous {
		t.Fatal("expected unambiguous match to sgb5")
	}
	if best == nil || best.Law.ID != "sgb5" {
		t.Fatalf("expected sgb5, got best=%+v ambiguous=%v", best, ambiguous)
	}
}

func TestResolveSGBIII_matchesSGB3ByAlias(t *testing.T) {
	e := NewEngine()
	e.Swap([]domain.Law{
		{ID: "sgb3", Abbreviation: "SGB III", Title: "Sozialgesetzbuch III", GIIPath: "sgb_3"},
	}, nil)
	for _, q := range []string{"SGB III", "sgb_3", "sgb3"} {
		best, _, _ := e.Current().Resolve(q, 0.75)
		if best == nil || best.Law.ID != "sgb3" {
			t.Fatalf("query %q: best=%+v want sgb3", q, best)
		}
	}
}
