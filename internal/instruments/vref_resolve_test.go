package instruments

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

type stubBGBlIndex struct {
	slug  string
	lawID string
	ok    bool
}

func (s stubBGBlIndex) LookupBGBlIndex(teil, year int, number string) (string, string, bool) {
	return s.slug, s.lawID, s.ok
}

func TestResolveOperativeVRefs_MiLoG268MatchesCurrent(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	ref := domain.InstrumentRef{
		Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1",
		Raw: "§ 1 V v. 5.11.2025 I Nr. 268",
	}
	parentStand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1: %+v", len(got), got)
	}
	if got[0].MatchedGIISlug != "milov5" {
		t.Fatalf("MatchedGIISlug=%q want milov5", got[0].MatchedGIISlug)
	}
	if got[0].Historical {
		t.Fatal("expected current match, not historical")
	}
	if got[0].MatchMethod != "notes_identity" {
		t.Fatalf("MatchMethod=%q want notes_identity", got[0].MatchMethod)
	}
}

func TestResolveOperativeVRefs_321OnlyPastHistorical(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	ref := domain.InstrumentRef{Kind: "", Teil: 1, Year: 2023, Number: "321"}
	parentStand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1: %+v", len(got), got)
	}
	if !got[0].Historical {
		t.Fatalf("expected historical for past-only 321; got %+v", got[0])
	}
	if got[0].MatchedGIISlug != "" {
		t.Fatalf("historical must not set MatchedGIISlug; got %q", got[0].MatchedGIISlug)
	}
}

func TestResolveOperativeVRefs_kindVPastOnlyNotHistorical(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	// Kind V citing only past milov4 identity must NOT bypass Proof C via Historical.
	ref := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2023, Number: "321", SectionHint: "§ 1"}
	parentStand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1", len(got))
	}
	if got[0].Historical {
		t.Fatalf("Kind V must not be Historical via past-only match; got %+v", got[0])
	}
	if got[0].MatchedGIISlug != "" || got[0].Resolved {
		t.Fatalf("want unmatched unresolved; got %+v", got[0])
	}
}

func TestResolveOperativeVRefs_ambiguousCurrentNoHistoricalFallthrough(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	// Two current children in different section groups, same BGBl identity.
	rows := []domain.LinkedInstrument{
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "child_a",
			Notes: "BGBl 2025 I Nr. 268", EffectiveFrom: "2026-01-01", SectionHint: "§ 1",
		},
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "child_b",
			Notes: "BGBl 2025 I Nr. 268", EffectiveFrom: "2026-01-01", SectionHint: "§ 2",
		},
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "child_past",
			Notes: "BGBl 2025 I Nr. 268", EffectiveFrom: "2024-01-01", SectionHint: "§ 1",
		},
	}
	annotated := AnnotateChain(rows, asOf)
	// Empty section hint → cannot disambiguate → fail closed (no past fallthrough).
	ref := domain.InstrumentRef{Kind: "", Teil: 1, Year: 2025, Number: "268"}
	parentStand := domain.StandCitation{
		LawID: "parent", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d want 1", len(got))
	}
	if got[0].Historical || got[0].MatchedGIISlug != "" {
		t.Fatalf("ambiguous current must stay unmatched (no historical fallthrough); got %+v", got[0])
	}
}

func TestResolveOperativeVRefs_unmatched999(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	ref := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "999"}
	parentStand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1", len(got))
	}
	if got[0].Resolved || got[0].Historical || got[0].MatchedGIISlug != "" {
		t.Fatalf("unmatched ref should be unresolved; got %+v", got[0])
	}
}

func TestResolveOperativeVRefs_bgblIndexFallback(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	rows := []domain.LinkedInstrument{
		{
			ParentLawID: "milog", Kind: "verordnung", GIISlug: "milov5",
			Notes:         "Fünfte Mindestlohnanpassungsverordnung",
			EffectiveFrom: "2026-01-01", SectionHint: "§ 1", Coverage: CoverageSection,
		},
	}
	annotated := AnnotateChain(rows, asOf)
	ref := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "268"}
	parentStand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	index := stubBGBlIndex{slug: "milov5", lawID: "milov5", ok: true}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, index, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1", len(got))
	}
	if got[0].MatchedGIISlug != "milov5" {
		t.Fatalf("MatchedGIISlug=%q want milov5 via bgbl_index", got[0].MatchedGIISlug)
	}
	if got[0].MatchMethod != "bgbl_index" {
		t.Fatalf("MatchMethod=%q want bgbl_index", got[0].MatchMethod)
	}
}
