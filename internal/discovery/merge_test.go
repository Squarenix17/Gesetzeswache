package discovery

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestMerge_SeedWins(t *testing.T) {
	seeded := []domain.LinkedInstrument{
		{ParentLawID: "milog", GIISlug: "milov5", Notes: "seeded note", Kind: "verordnung"},
	}
	discovered := []domain.LinkedInstrument{
		{
			ParentLawID: "milog",
			GIISlug:     "milov5",
			Notes:       "discovered note",
			Confidence:  ConfidenceHigh,
			Source:      SourceDiscovered,
			EdgeType:    EdgeErmaechtigung,
		},
		{
			ParentLawID: "milog",
			GIISlug:     "milov_new",
			Notes:       "only discovered",
			Confidence:  ConfidenceHigh,
			Source:      SourceDiscovered,
			EdgeType:    EdgeErmaechtigung,
		},
	}

	got := Merge(seeded, discovered)
	if len(got) != 2 {
		t.Fatalf("want 2 merged, got %d: %+v", len(got), got)
	}

	bySlug := map[string]domain.LinkedInstrument{}
	for _, li := range got {
		bySlug[li.GIISlug] = li
	}

	collision := bySlug["milov5"]
	if collision.Notes != "seeded note" {
		t.Fatalf("seed should win notes=%q", collision.Notes)
	}
	if collision.Source != SourceSeeded {
		t.Fatalf("collision source=%q want %q", collision.Source, SourceSeeded)
	}

	onlyDisc := bySlug["milov_new"]
	if onlyDisc.Source != SourceDiscovered {
		t.Fatalf("discovered-only source=%q", onlyDisc.Source)
	}
	if onlyDisc.Confidence != ConfidenceHigh {
		t.Fatalf("confidence=%q", onlyDisc.Confidence)
	}
}

func TestMerge_DropsMediumDiscovered(t *testing.T) {
	seeded := []domain.LinkedInstrument{
		{ParentLawID: "milog", GIISlug: "milov5", Kind: "verordnung"},
	}
	discovered := []domain.LinkedInstrument{
		{
			ParentLawID: "milog",
			GIISlug:     "milov_med",
			Confidence:  ConfidenceMedium,
			Source:      SourceDiscovered,
		},
		{
			ParentLawID: "milog",
			GIISlug:     "milov_low",
			Confidence:  ConfidenceLow,
			Source:      SourceDiscovered,
		},
		{
			ParentLawID: "milog",
			GIISlug:     "milov_high",
			Confidence:  ConfidenceHigh,
			Source:      SourceDiscovered,
		},
	}

	got := Merge(seeded, discovered)
	if len(got) != 2 {
		t.Fatalf("want seeded + high only (2), got %d: %+v", len(got), got)
	}
	slugs := map[string]bool{}
	for _, li := range got {
		slugs[li.GIISlug] = true
	}
	if !slugs["milov5"] || !slugs["milov_high"] {
		t.Fatalf("slugs=%v", slugs)
	}
	if slugs["milov_med"] || slugs["milov_low"] {
		t.Fatalf("medium/low should be dropped, slugs=%v", slugs)
	}
}

func TestEdgeToLinked(t *testing.T) {
	e := domain.DiscoveredEdge{
		ParentLawID:   "milog",
		GIISlug:       "milov5",
		SectionHint:   "§ 1",
		Notes:         "auth",
		EdgeType:      EdgeErmaechtigung,
		Confidence:    ConfidenceHigh,
		EffectiveFrom: "2025-11-05",
		ChildLawID:    "milov5",
	}
	li := EdgeToLinked(e)
	if li.ParentLawID != "milog" || li.GIISlug != "milov5" {
		t.Fatalf("ids: %+v", li)
	}
	if li.Source != SourceDiscovered || li.Confidence != ConfidenceHigh || li.EdgeType != EdgeErmaechtigung {
		t.Fatalf("metadata: %+v", li)
	}
	if li.Kind != "verordnung" {
		t.Fatalf("kind=%q want verordnung", li.Kind)
	}
	if li.SectionHint != "§ 1" || li.Notes != "auth" || li.EffectiveFrom != "2025-11-05" || li.LawID != "milov5" {
		t.Fatalf("fields: %+v", li)
	}
}
