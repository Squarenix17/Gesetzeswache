package instruments

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestEvaluateLeafChildFreshness_probeOnly_uncertain(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	st := &memStore{
		stand: map[string]domain.StandCitation{
			"milov5": {
				LawID: "milov5", Year: 2025, Teil: 1, Number: "268", ParseOK: true,
				Raw: "BGBl. 2025 I Nr. 268",
			},
		},
	}
	ctx := VRefProofContext{
		Now:                now,
		MaxAge:             6 * time.Hour,
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		BGBlFromProbeOnly:  true,
	}
	got := EvaluateLeafChildFreshness(st, nil, "milov5", ctx)
	if got != domain.FreshnessUncertain {
		t.Fatalf("got %s want uncertain for probe-only child eval", got)
	}
}

func TestApplyChildFreshnessProof_probeOnlyChild(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	ref := domain.InstrumentRef{
		Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1",
	}
	raw := []domain.VRefResolution{{
		Ref:            ref,
		MatchedGIISlug: "milov5",
		MatchMethod:    "notes_identity",
	}}
	st := &memStore{
		stand: map[string]domain.StandCitation{
			"milov5": {
				LawID: "milov5", Year: 2025, Teil: 1, Number: "268", ParseOK: true,
				Raw: "BGBl. 2025 I Nr. 268",
			},
		},
	}
	ctx := VRefProofContext{
		Now:                now,
		MaxAge:             6 * time.Hour,
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		BGBlFromProbeOnly:  true,
	}
	got := ApplyChildFreshnessProof(raw, func(slug string) domain.FreshnessState {
		return EvaluateLeafChildFreshness(st, nil, slug, ctx)
	})
	if len(got) != 1 {
		t.Fatalf("got %d resolutions", len(got))
	}
	if got[0].Resolved || got[0].ChildConfirmed {
		t.Fatalf("probe-only child must not confirm; got %+v", got[0])
	}
}
