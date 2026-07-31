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

func TestResolveOperativeVRefs_emptyStatusMatchesAsCurrent(t *testing.T) {
	// Discovery edges without EffectiveFrom keep Status==""; treat as current match candidate.
	annotated := []domain.LinkedInstrument{{
		ParentLawID: "bgb", Kind: "verordnung", GIISlug: "minuhv",
		Notes: "BGBl 2015 I Nr. 359", SectionHint: "§ 1612a", Status: "",
	}}
	ref := domain.InstrumentRef{
		Kind: "V", Teil: 1, Year: 2015, Number: "359", SectionHint: "§ 1612a",
		Raw: "§ 1612a V v. 1.12.2015 I Nr. 359",
	}
	parentStand := domain.StandCitation{
		LawID: "bgb", Year: 2024, Teil: 1, Number: "100", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1: %+v", len(got), got)
	}
	if got[0].MatchedGIISlug != "minuhv" {
		t.Fatalf("MatchedGIISlug=%q want minuhv", got[0].MatchedGIISlug)
	}
	if got[0].Historical {
		t.Fatal("empty-status current candidate must not be historical")
	}
	if got[0].MatchMethod != "notes_identity" {
		t.Fatalf("MatchMethod=%q want notes_identity", got[0].MatchMethod)
	}
}

func TestResolveOperativeVRefs_twoEmptyStatusAmbiguousFailClosed(t *testing.T) {
	annotated := []domain.LinkedInstrument{
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "child_a",
			Notes: "BGBl 2015 I Nr. 359", SectionHint: "§ 1612a", Status: "",
		},
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "child_b",
			Notes: "BGBl 2015 I Nr. 359", SectionHint: "§ 1612b", Status: "",
		},
	}
	ref := domain.InstrumentRef{Kind: "", Teil: 1, Year: 2015, Number: "359"}
	parentStand := domain.StandCitation{
		LawID: "parent", Year: 2024, Teil: 1, Number: "100", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d want 1", len(got))
	}
	if got[0].Historical || got[0].MatchedGIISlug != "" {
		t.Fatalf("ambiguous empty-status must stay unmatched (no historical fallthrough); got %+v", got[0])
	}
}

func TestResolveOperativeVRefs_emptyStatusStillRequiresCurrentBucketOnlyForMatch(t *testing.T) {
	annotated := []domain.LinkedInstrument{
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "child_past",
			Notes: "BGBl 2015 I Nr. 359", SectionHint: "§ 1612a",
			EffectiveFrom: "2014-01-01", Status: StatusPast,
		},
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "minuhv",
			Notes: "BGBl 2015 I Nr. 359", SectionHint: "§ 1612a", Status: "",
		},
	}
	ref := domain.InstrumentRef{Kind: "", Teil: 1, Year: 2015, Number: "359", SectionHint: "§ 1612a"}
	parentStand := domain.StandCitation{
		LawID: "parent", Year: 2024, Teil: 1, Number: "100", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d want 1", len(got))
	}
	if got[0].MatchedGIISlug != "minuhv" {
		t.Fatalf("MatchedGIISlug=%q want minuhv (empty-status current candidate)", got[0].MatchedGIISlug)
	}
	if got[0].Historical {
		t.Fatal("empty-status match must not be historical")
	}

	refPastOnly := domain.InstrumentRef{Kind: "", Teil: 1, Year: 2014, Number: "100"}
	annotatedPastOnly := []domain.LinkedInstrument{{
		ParentLawID: "parent", Kind: "verordnung", GIISlug: "child_past",
		Notes: "BGBl 2014 I Nr. 100", SectionHint: "§ 1",
		EffectiveFrom: "2014-01-01", Status: StatusPast,
	}}
	gotPast := ResolveOperativeVRefs([]domain.InstrumentRef{refPastOnly}, annotatedPastOnly, nil, nil, parentStand)
	if len(gotPast) != 1 {
		t.Fatalf("got %d want 1", len(gotPast))
	}
	if !gotPast[0].Historical {
		t.Fatalf("past-only non-V ref should be historical; got %+v", gotPast[0])
	}
}

func TestResolveOperativeVRefs_bareBekSkipped(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	bareBek := domain.InstrumentRef{
		Kind: "BEK", Teil: 1, Year: 2024, Number: "313",
		Raw: "Bek. v. 17.10.2024 I Nr. 313",
	}
	parentStand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{bareBek}, annotated, nil, nil, parentStand)
	if len(got) != 0 {
		t.Fatalf("bare BEK must not emit resolutions; got %+v", got)
	}
}

func TestResolveOperativeVRefs_sectionScopedBekResolved(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	rows := []domain.LinkedInstrument{
		{
			ParentLawID: "parent", Kind: "bekanntmachung", GIISlug: "bek_child",
			Notes: "§ 1 Bek. v. 1.2.2025 I Nr. 50", EffectiveFrom: "2026-01-01",
			SectionHint: "§ 1", Coverage: CoverageSection,
		},
	}
	annotated := AnnotateChain(rows, asOf)
	ref := domain.InstrumentRef{
		Kind: "BEK", Teil: 1, Year: 2025, Number: "50", SectionHint: "§ 1",
		Raw: "§ 1 Bek. v. 1.2.2025 I Nr. 50",
	}
	parentStand := domain.StandCitation{
		LawID: "parent", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, nil, nil, parentStand)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1 for section-scoped BEK: %+v", len(got), got)
	}
	if got[0].MatchedGIISlug != "bek_child" {
		t.Fatalf("MatchedGIISlug=%q want bek_child", got[0].MatchedGIISlug)
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
