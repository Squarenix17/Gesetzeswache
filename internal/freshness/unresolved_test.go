package freshness

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestCollectUnresolvedRefs_classifications(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	vRef := domain.InstrumentRef{
		Kind: "V", Teil: 1, Year: 2025, Number: "999",
		Raw: "§ 1 V v. 5.11.2025 I Nr. 999",
	}
	bareBek := domain.InstrumentRef{
		Kind: "BEK", Teil: 1, Year: 2024, Number: "313",
		Raw: "Bek. v. 17.10.2024 I Nr. 313",
	}
	childNotCurrent := domain.InstrumentRef{
		Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1",
		Raw: "§ 1 V v. 5.11.2025 I Nr. 268",
	}
	historicalRef := domain.InstrumentRef{
		Kind: "", Teil: 1, Year: 2020, Number: "100",
		Raw: "BGBl. 2020 I Nr. 100",
	}
	in := Input{
		LawID: "milog",
		Stand: domain.StandCitation{
			LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
		},
		InstrumentRefs:             []domain.InstrumentRef{bareBek, vRef, childNotCurrent, historicalRef},
		HasSeededLinkedInstruments: true,
		VRefResolutions: []domain.VRefResolution{
			{
				Ref:            childNotCurrent,
				MatchedGIISlug: "milov5",
				MatchMethod:    "notes_identity",
				Resolved:       true,
				ChildConfirmed: false,
			},
			{
				Ref:            historicalRef,
				Historical:     true,
				MatchMethod:    "notes_identity",
			},
		},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	}
	got := CollectUnresolvedRefs(in)
	byClass := map[string]int{}
	for _, u := range got {
		byClass[u.Classification]++
	}
	if byClass["ignored_bare_bek"] != 1 {
		t.Fatalf("ignored_bare_bek=%d want 1; got %+v", byClass["ignored_bare_bek"], got)
	}
	if byClass["unmatched"] != 1 {
		t.Fatalf("unmatched=%d want 1; got %+v", byClass["unmatched"], got)
	}
	if byClass["child_not_current"] != 1 {
		t.Fatalf("child_not_current=%d want 1; got %+v", byClass["child_not_current"], got)
	}
	if byClass["historical"] != 1 {
		t.Fatalf("historical=%d want 1; got %+v", byClass["historical"], got)
	}
}

func TestCollectUnresolvedRefs_missingSeed(t *testing.T) {
	ref := domain.InstrumentRef{Kind: "", Teil: 1, Year: 2025, Number: "1"}
	got := CollectUnresolvedRefs(Input{
		Stand:                      domain.StandCitation{ParseOK: true, Year: 2024, Teil: 1, Number: "1"},
		InstrumentRefs:             []domain.InstrumentRef{ref},
		HasSeededLinkedInstruments: false,
		VRefResolutions:            []domain.VRefResolution{},
	})
	if len(got) != 1 || got[0].Classification != "missing_seed" {
		t.Fatalf("got %+v want missing_seed", got)
	}
}
