package instruments

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
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

func TestMarkSupersededPastKindV_321SupersededWhenMilov5Confirmed(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	ref268 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1"}
	ref321 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2023, Number: "321", SectionHint: "§ 1"}
	resolutions := []domain.VRefResolution{
		{
			Ref:            ref268,
			MatchedGIISlug: "milov5",
			MatchMethod:    "notes_identity",
			Resolved:       true,
			ChildConfirmed: true,
		},
		{Ref: ref321},
	}
	got := MarkSupersededPastKindV(resolutions, annotated, nil)
	if len(got) != 2 {
		t.Fatalf("got %d resolutions want 2", len(got))
	}
	if !got[1].Historical {
		t.Fatalf("321 Kind V should be Historical when milov5 confirmed; got %+v", got[1])
	}
	if got[1].SupersededBy != "milov5" {
		t.Fatalf("SupersededBy=%q want milov5", got[1].SupersededBy)
	}
	if got[1].MatchMethod != "superseded_past_v" {
		t.Fatalf("MatchMethod=%q want superseded_past_v", got[1].MatchMethod)
	}
	if got[0].Historical {
		t.Fatalf("268 resolution must stay non-historical; got %+v", got[0])
	}
}

func TestMarkSupersededPastKindV_321NotSupersededWhenMilov5Unconfirmed(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	ref268 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1"}
	ref321 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2023, Number: "321", SectionHint: "§ 1"}
	resolutions := []domain.VRefResolution{
		{
			Ref:            ref268,
			MatchedGIISlug: "milov5",
			MatchMethod:    "notes_identity",
		},
		{Ref: ref321},
	}
	got := MarkSupersededPastKindV(resolutions, annotated, nil)
	if got[1].Historical {
		t.Fatalf("321 must not be Historical without confirmed sibling; got %+v", got[1])
	}
}

func TestMarkSupersededPastKindV_emptyStatusCurrentSiblingSupersedes(t *testing.T) {
	// Past seeded child + discovered empty-status current sibling (no EffectiveFrom).
	annotated := []domain.LinkedInstrument{
		{
			ParentLawID: "milog", Kind: "verordnung", GIISlug: "milov4",
			Notes: "BGBl 2023 I Nr. 321", SectionHint: "§ 1", Status: StatusPast,
		},
		{
			ParentLawID: "milog", Kind: "verordnung", GIISlug: "milov5",
			Notes: "BGBl 2025 I Nr. 268", SectionHint: "§ 1", Status: "",
		},
	}
	ref268 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1"}
	ref321 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2023, Number: "321", SectionHint: "§ 1"}
	resolutions := []domain.VRefResolution{
		{
			Ref:            ref268,
			MatchedGIISlug: "milov5",
			MatchMethod:    "notes_identity",
			Resolved:       true,
			ChildConfirmed: true,
		},
		{Ref: ref321},
	}
	got := MarkSupersededPastKindV(resolutions, annotated, nil)
	if !got[1].Historical || got[1].SupersededBy != "milov5" {
		t.Fatalf("empty-status confirmed sibling should supersede past Kind V; got %+v", got[1])
	}
	if got[1].MatchMethod != "superseded_past_v" {
		t.Fatalf("MatchMethod=%q want superseded_past_v", got[1].MatchMethod)
	}
}

func TestMarkSupersededPastKindV_sectionlessRefAmbiguousPastSectionsFailClosed(t *testing.T) {
	annotated := []domain.LinkedInstrument{
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "past_a",
			Notes: "BGBl 2023 I Nr. 321", SectionHint: "§ 1", Status: StatusPast,
		},
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "past_b",
			Notes: "BGBl 2023 I Nr. 321", SectionHint: "§ 2", Status: StatusPast,
		},
		{
			ParentLawID: "parent", Kind: "verordnung", GIISlug: "cur",
			Notes: "BGBl 2025 I Nr. 268", SectionHint: "§ 1", Status: StatusCurrent,
		},
	}
	ref268 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1"}
	// Sectionless Kind V — must not inherit an arbitrary past section.
	ref321 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2023, Number: "321"}
	resolutions := []domain.VRefResolution{
		{
			Ref:            ref268,
			MatchedGIISlug: "cur",
			MatchMethod:    "notes_identity",
			Resolved:       true,
			ChildConfirmed: true,
		},
		{Ref: ref321},
	}
	got := MarkSupersededPastKindV(resolutions, annotated, nil)
	if got[1].Historical {
		t.Fatalf("sectionless Kind V with multi-section past matches must stay unresolved; got %+v", got[1])
	}
}

func TestMarkSupersededPastKindV_sectionMismatchStaysUnresolved(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	chain := milogChain()
	chain[0].SectionHint = "§ 2"
	annotated := AnnotateChain(chain, asOf)
	ref268 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1"}
	ref321 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2023, Number: "321", SectionHint: "§ 2"}
	resolutions := []domain.VRefResolution{
		{
			Ref:            ref268,
			MatchedGIISlug: "milov5",
			MatchMethod:    "notes_identity",
			Resolved:       true,
			ChildConfirmed: true,
		},
		{Ref: ref321},
	}
	childStands := map[string]domain.StandCitation{
		"milov4": {LawID: "milov4", Year: 2023, Teil: 1, Number: "321", ParseOK: true},
		"milov5": {LawID: "milov5", Year: 2025, Teil: 1, Number: "268", ParseOK: true},
	}
	got := MarkSupersededPastKindV(resolutions, annotated, childStands)
	if len(got) != 2 {
		t.Fatalf("got %d resolutions want 2", len(got))
	}
	if got[1].Historical {
		t.Fatalf("321 at §2 must not be Historical when confirmed sibling is only at §1; got %+v", got[1])
	}
	if got[0].Historical {
		t.Fatalf("268 resolution must stay non-historical; got %+v", got[0])
	}
}

func TestProveVRefResolutions_MiLoG321SupersededWhenMilov5Confirmed(t *testing.T) {
	asOf := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	parentStand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
	}
	st := &memStore{
		stand: map[string]domain.StandCitation{
			"milov5": {
				LawID: "milov5", Year: 2025, Teil: 1, Number: "268", ParseOK: true,
				Raw: "BGBl. 2025 I Nr. 268",
			},
			"milov4": {
				LawID: "milov4", Year: 2023, Teil: 1, Number: "321", ParseOK: true,
				Raw: "BGBl. 2023 I Nr. 321",
			},
		},
		issue: map[string]domain.GazetteIssue{
			citation.IssueID(1, 2025, "268"): {
				ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
			},
		},
	}
	ctx := VRefProofContext{
		Now:                asOf,
		MaxAge:             6 * time.Hour,
		LastTOCSuccess:     asOf.Add(-time.Hour),
		LastGIIFeedSuccess: asOf.Add(-time.Hour),
		LastBGBlSuccess:    asOf.Add(-time.Hour),
		BGBlFromProbeOnly:  false,
	}
	refs := []domain.InstrumentRef{
		{Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1"},
		{Kind: "V", Teil: 1, Year: 2023, Number: "321", SectionHint: "§ 1"},
	}
	got := ProveVRefResolutions(refs, annotated, parentStand, st, nil, nil, ctx)
	if len(got) != 2 {
		t.Fatalf("got %d resolutions want 2: %+v", len(got), got)
	}
	if !got[0].Resolved || !got[0].ChildConfirmed {
		t.Fatalf("268 must be Resolved+ChildConfirmed; got %+v", got[0])
	}
	if got[0].MatchedGIISlug != "milov5" {
		t.Fatalf("268 MatchedGIISlug=%q want milov5", got[0].MatchedGIISlug)
	}
	if !got[1].Historical {
		t.Fatalf("321 Kind V should be Historical when milov5 confirmed; got %+v", got[1])
	}
	if got[1].SupersededBy != "milov5" {
		t.Fatalf("321 SupersededBy=%q want milov5", got[1].SupersededBy)
	}
	if got[1].MatchMethod != "superseded_past_v" {
		t.Fatalf("321 MatchMethod=%q want superseded_past_v", got[1].MatchMethod)
	}
}

func TestProveVRefResolutions_emptyStatusUnconfirmedChild(t *testing.T) {
	asOf := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	annotated := []domain.LinkedInstrument{{
		ParentLawID: "bgb", Kind: "verordnung", GIISlug: "minuhv",
		Notes: "BGBl 2015 I Nr. 359", SectionHint: "§ 1612a", Status: "",
	}}
	parentStand := domain.StandCitation{
		LawID: "bgb", Year: 2024, Teil: 1, Number: "100", ParseOK: true,
	}
	st := &memStore{
		stand: map[string]domain.StandCitation{
			"minuhv": {
				LawID: "minuhv", Year: 2015, Teil: 1, Number: "359", ParseOK: true,
				Raw: "BGBl. 2015 I Nr. 359",
			},
		},
	}
	ctx := VRefProofContext{
		Now:                asOf,
		MaxAge:             6 * time.Hour,
		LastTOCSuccess:     asOf.Add(-time.Hour),
		LastGIIFeedSuccess: asOf.Add(-time.Hour),
		LastBGBlSuccess:    asOf.Add(-time.Hour),
		BGBlFromProbeOnly:  true, // child freshness stays uncertain
	}
	refs := []domain.InstrumentRef{
		{Kind: "V", Teil: 1, Year: 2015, Number: "359", SectionHint: "§ 1612a"},
	}
	got := ProveVRefResolutions(refs, annotated, parentStand, st, nil, nil, ctx)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1: %+v", len(got), got)
	}
	if got[0].MatchedGIISlug != "minuhv" {
		t.Fatalf("MatchedGIISlug=%q want minuhv", got[0].MatchedGIISlug)
	}
	if got[0].Resolved || got[0].ChildConfirmed {
		t.Fatalf("unconfirmed child must not resolve parent; got %+v", got[0])
	}
}

func TestProveVRefResolutions_emptyStatusConfirmedChild(t *testing.T) {
	asOf := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	annotated := []domain.LinkedInstrument{{
		ParentLawID: "arbzg", Kind: "verordnung", GIISlug: "binscharbzv",
		Notes: "BGBl 2020 I Nr. 42", SectionHint: "§ 3", Status: "",
	}}
	parentStand := domain.StandCitation{
		LawID: "arbzg", Year: 2024, Teil: 1, Number: "50", ParseOK: true,
	}
	st := &memStore{
		stand: map[string]domain.StandCitation{
			"binscharbzv": {
				LawID: "binscharbzv", Year: 2020, Teil: 1, Number: "42", ParseOK: true,
				Raw: "BGBl. 2020 I Nr. 42",
			},
		},
		issue: map[string]domain.GazetteIssue{
			citation.IssueID(1, 2020, "42"): {
				ID: citation.IssueID(1, 2020, "42"), Teil: 1, Year: 2020, Number: "42",
			},
		},
	}
	ctx := VRefProofContext{
		Now:                asOf,
		MaxAge:             6 * time.Hour,
		LastTOCSuccess:     asOf.Add(-time.Hour),
		LastGIIFeedSuccess: asOf.Add(-time.Hour),
		LastBGBlSuccess:    asOf.Add(-time.Hour),
		BGBlFromProbeOnly:  false,
	}
	refs := []domain.InstrumentRef{
		{Kind: "V", Teil: 1, Year: 2020, Number: "42", SectionHint: "§ 3"},
	}
	got := ProveVRefResolutions(refs, annotated, parentStand, st, nil, nil, ctx)
	if len(got) != 1 {
		t.Fatalf("got %d resolutions want 1: %+v", len(got), got)
	}
	if got[0].MatchedGIISlug != "binscharbzv" {
		t.Fatalf("MatchedGIISlug=%q want binscharbzv", got[0].MatchedGIISlug)
	}
	if !got[0].Resolved || !got[0].ChildConfirmed {
		t.Fatalf("confirmed empty-status child must resolve; got %+v", got[0])
	}
}

func TestMarkSupersededPastKindV_unmatched999StaysUnresolved(t *testing.T) {
	asOf := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	annotated := AnnotateChain(milogChain(), asOf)
	ref268 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1"}
	ref999 := domain.InstrumentRef{Kind: "V", Teil: 1, Year: 2025, Number: "999"}
	resolutions := []domain.VRefResolution{
		{
			Ref:            ref268,
			MatchedGIISlug: "milov5",
			MatchMethod:    "notes_identity",
			Resolved:       true,
			ChildConfirmed: true,
		},
		{Ref: ref999},
	}
	got := MarkSupersededPastKindV(resolutions, annotated, nil)
	if got[1].Historical {
		t.Fatalf("unmatched 999 must not become Historical; got %+v", got[1])
	}
}

func TestResolveOperativeVRefs_SGBIII_InsoGeldFestV2024_Nr379(t *testing.T) {
	// Consumer Error B: V v. 15.12.2023 I Nr. 379 must match seeded InsoGeldFestV 2024.
	annotated := []domain.LinkedInstrument{{
		ParentLawID: "sgb3", Kind: "verordnung", GIISlug: "insogeldfestv_2024",
		Notes: "Insolvenzgeldumlagesatzverordnung 2024 (BGBl 2023 I Nr. 379)",
		EffectiveFrom: "2024-01-01", SectionHint: "§ 358", Status: StatusCurrent,
	}}
	ref := domain.InstrumentRef{
		Kind: "V", Teil: 1, Year: 2023, Number: "379", SectionHint: "§ 358",
		Raw: "§ 358 V v. 15.12.2023 I Nr. 379",
	}
	childStands := map[string]domain.StandCitation{
		"insogeldfestv_2024": {
			LawID: "insogeldfestv_2024", Year: 2023, Teil: 1, Number: "379", ParseOK: true,
			Raw: "BGBl. 2023 I Nr. 379",
		},
	}
	got := ResolveOperativeVRefs([]domain.InstrumentRef{ref}, annotated, childStands, nil, domain.StandCitation{
		LawID: "sgb3", Year: 2026, Teil: 1, Number: "100", ParseOK: true,
	})
	if len(got) != 1 {
		t.Fatalf("got %d resolutions", len(got))
	}
	if got[0].MatchedGIISlug != "insogeldfestv_2024" {
		t.Fatalf("MatchedGIISlug=%q want insogeldfestv_2024", got[0].MatchedGIISlug)
	}
}

