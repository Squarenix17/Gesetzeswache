package instruments

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func milogChain() []domain.LinkedInstrument {
	return []domain.LinkedInstrument{
		{
			ParentLawID:   "milog",
			Kind:          "verordnung",
			GIISlug:       "milov4",
			Notes:         "Vierte Mindestlohnanpassungsverordnung (BGBl 2023 I Nr. 321)",
			EffectiveFrom: "2024-01-01",
			SectionHint:   "§ 1",
			Coverage:      CoverageSection,
		},
		{
			ParentLawID:   "milog",
			Kind:          "verordnung",
			GIISlug:       "milov5",
			Notes:         "Fünfte Mindestlohnanpassungsverordnung (BGBl 2025 I Nr. 268)",
			EffectiveFrom: "2026-01-01",
			SectionHint:   "§ 1",
			Coverage:      CoverageSection,
		},
	}
}

func TestAnnotateChain_MiLoG_asOf2025(t *testing.T) {
	asOf := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	got := AnnotateChain(milogChain(), asOf)
	if len(got) != 2 {
		t.Fatalf("got %d want 2", len(got))
	}
	if got[0].GIISlug != "milov4" || got[0].Status != StatusCurrent {
		t.Fatalf("milov4: %+v want status=%s", got[0], StatusCurrent)
	}
	if got[1].GIISlug != "milov5" || got[1].Status != StatusFuture {
		t.Fatalf("milov5: %+v want status=%s", got[1], StatusFuture)
	}
	filtered := FilterLinkedForResponse(got, false)
	if len(filtered) != 2 {
		t.Fatalf("filter default: got %d want 2", len(filtered))
	}
}

func TestAnnotateChain_MiLoG_asOf2026(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	got := AnnotateChain(milogChain(), asOf)
	if len(got) != 2 {
		t.Fatalf("got %d want 2", len(got))
	}
	if got[0].GIISlug != "milov4" || got[0].Status != StatusPast {
		t.Fatalf("milov4: %+v want status=%s", got[0], StatusPast)
	}
	if got[1].GIISlug != "milov5" || got[1].Status != StatusCurrent {
		t.Fatalf("milov5: %+v want status=%s", got[1], StatusCurrent)
	}
	withoutPast := FilterLinkedForResponse(got, false)
	if len(withoutPast) != 1 || withoutPast[0].GIISlug != "milov5" {
		t.Fatalf("filter without past: %+v", withoutPast)
	}
	withPast := FilterLinkedForResponse(got, true)
	if len(withPast) != 2 {
		t.Fatalf("filter with past: got %d want 2", len(withPast))
	}
}

func TestAnnotateChain_differentSectionHintsBothCurrent(t *testing.T) {
	rows := []domain.LinkedInstrument{
		{
			ParentLawID: "arbzg", Kind: "verordnung", GIISlug: "arbzgv1",
			EffectiveFrom: "2024-01-01", SectionHint: "§ 3", Coverage: CoverageSection,
		},
		{
			ParentLawID: "arbzg", Kind: "verordnung", GIISlug: "arbzgv2",
			EffectiveFrom: "2024-06-01", SectionHint: "§ 9", Coverage: CoverageSection,
		},
	}
	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got := AnnotateChain(rows, asOf)
	if len(got) != 2 {
		t.Fatalf("got %d want 2", len(got))
	}
	for _, li := range got {
		if li.Status != StatusCurrent {
			t.Fatalf("%s: status=%q want %s", li.GIISlug, li.Status, StatusCurrent)
		}
	}
}

func TestAnnotateChain_noEffectiveFromLeavesStatusEmpty(t *testing.T) {
	rows := []domain.LinkedInstrument{
		{ParentLawID: "milog", Kind: "verordnung", GIISlug: "milov4", SectionHint: "§ 1"},
	}
	got := AnnotateChain(rows, time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	if got[0].Status != "" {
		t.Fatalf("status=%q want empty", got[0].Status)
	}
	if got[0].Coverage != CoverageSection {
		t.Fatalf("coverage=%q want %s", got[0].Coverage, CoverageSection)
	}
}
