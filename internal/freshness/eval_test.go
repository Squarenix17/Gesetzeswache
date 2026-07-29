package freshness

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestConfirmedCurrent(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{LawID: "bgb", Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("got %s (%s)", rec.State, rec.Rationale)
	}
}

func TestForcedUncertainOnOldSync(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:           "bgb",
		Stand:           domain.StandCitation{ParseOK: true, Year: 2024, Teil: 1, Number: "1"},
		LastTOCSuccess:  now.Add(-48 * time.Hour),
		LastBGBlSuccess: now.Add(-time.Hour),
		Now:             now,
		MaxAge:          6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s", rec.State)
	}
}

func TestStaleWhenNewerIssue(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "bgb",
		Stand: domain.StandCitation{Year: 2024, Teil: 1, Number: "10", ParseOK: true},
		LinkedIssues: []domain.GazetteIssue{{
			ID: "BGBl-1/2025/5", Teil: 1, Year: 2025, Number: "5",
		}},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedStale {
		t.Fatalf("got %s", rec.State)
	}
}

func TestUncertainWhenStandMissing(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "milog",
		Stand:              domain.StandCitation{LawID: "milog", Raw: "", ParseOK: false},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s want uncertain (%s)", rec.State, rec.Rationale)
	}
	if rec.Rationale != "stand_unparsed_or_missing" {
		t.Fatalf("rationale=%q", rec.Rationale)
	}
}

func TestUncertainWhenStandUnparsedNoLinks(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "milog",
		Stand: domain.StandCitation{
			LawID: "milog", Raw: "opaque stand text", Year: 2026, ParseOK: false,
			ParseNotes: "insufficient structured fields",
		},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s want uncertain", rec.State)
	}
}

func TestStaleWhenUnparsedStandWithLinkedIssue(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "milog",
		Stand: domain.StandCitation{Raw: "opaque", ParseOK: false},
		LinkedIssues: []domain.GazetteIssue{{
			ID: "BGBl-1/2025/268", Teil: 1, Year: 2025, Number: "268",
		}},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedStale {
		t.Fatalf("got %s want confirmed_stale", rec.State)
	}
}

func TestConfirmedCurrentDespiteEditorialGRefs(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "bgb",
		Stand: domain.StandCitation{
			LawID: "bgb", Year: 2024, Teil: 1, Number: "198", ParseOK: true,
			Raw: "Zuletzt geändert durch Art. 1 G v. 20.7.2024 I Nr. 198",
		},
		InstrumentRefs: []domain.InstrumentRef{
			{Kind: "G", Teil: 1, Year: 2023, Number: "217", Raw: "G v. 16.8.2023 I Nr. 217"},
			{Kind: "", Teil: 1, Year: 2022, Number: "361", Raw: "BGBl. I 2022 Nr. 361"},
			{Kind: "G", Teil: 1, Year: 2021, Number: "49", Raw: "G v. 10.2.2021 I Nr. 49"},
		},
		HasSeededLinkedInstruments: false,
		LastTOCSuccess:             now.Add(-time.Hour),
		LastGIIFeedSuccess:         now.Add(-time.Hour),
		LastBGBlSuccess:            now.Add(-time.Hour),
		Now:                        now,
		MaxAge:                     6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("got %s (%s) want confirmed_current", rec.State, rec.Rationale)
	}
}

func TestConfirmedCurrentDespiteEditorialBekRefs(t *testing.T) {
	// Live BGB pattern: Stand OK + bare Bek. republication notes + distinct G — not fail-closed.
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "bgb",
		Stand: domain.StandCitation{
			LawID: "bgb", Year: 2026, Teil: 1, Number: "198", ParseOK: true,
			Raw: "zuletzt geändert durch Art. 2 G v. 2.7.2026 I Nr. 198",
		},
		InstrumentRefs: []domain.InstrumentRef{
			{Kind: "G", Teil: 1, Year: 2026, Number: "198", Raw: "G v. 2.7.2026 I Nr. 198"},
			{Kind: "BEK", Teil: 1, Year: 2023, Number: "296", Raw: "Bek. v. 1.11.2023 I Nr. 296"},
			{Kind: "BEK", Teil: 1, Year: 2024, Number: "69", Raw: "Bek. v. 27.2.2024 I Nr. 69"},
			{Kind: "BEK", Teil: 1, Year: 2024, Number: "313", Raw: "Bek. v. 17.10.2024 I Nr. 313"},
			{Kind: "G", Teil: 1, Year: 2026, Number: "139", Raw: "G v. 12.5.2026 I Nr. 139"},
		},
		HasSeededLinkedInstruments: false,
		LastTOCSuccess:             now.Add(-time.Hour),
		LastGIIFeedSuccess:         now.Add(-time.Hour),
		LastBGBlSuccess:            now.Add(-time.Hour),
		Now:                        now,
		MaxAge:                     6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("got %s (%s) want confirmed_current for bare editorial BEK", rec.State, rec.Rationale)
	}
}

func TestUncertainWhenSeededAndEmptyKindRefs(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "milog",
		Stand: domain.StandCitation{
			LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
			Raw: "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137",
		},
		InstrumentRefs: []domain.InstrumentRef{{
			Kind: "", Teil: 1, Year: 2025, Number: "268",
			Raw: "BGBl 2025 I Nr. 268",
		}},
		HasSeededLinkedInstruments: true,
		LastTOCSuccess:             now.Add(-time.Hour),
		LastGIIFeedSuccess:         now.Add(-time.Hour),
		LastBGBlSuccess:            now.Add(-time.Hour),
		Now:                        now,
		MaxAge:                     6 * time.Hour,
	})
	if rec.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("must not be confirmed_current when seeded empty-kind refs remain unresolved; got %s (%s)", rec.State, rec.Rationale)
	}
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s want uncertain", rec.State)
	}
	if rec.Rationale != "unresolved_linked_instrument_refs" {
		t.Fatalf("rationale=%q want unresolved_linked_instrument_refs", rec.Rationale)
	}
}

func TestUncertainWhenPlusPlusCitesVerordnung(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "milog",
		Stand: domain.StandCitation{
			LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
			Raw: "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137",
		},
		InstrumentRefs: []domain.InstrumentRef{{
			Kind: "V", Teil: 1, Year: 2025, Number: "268", SectionHint: "§ 1",
			Raw: "§ 1 V v. 5.11.2025 I Nr. 268",
		}},
		InstrumentIssues: []domain.GazetteIssue{{
			ID: "BGBl-1/2025/268", Teil: 1, Year: 2025, Number: "268",
			Title: "Fünfte Mindestlohnanpassungsverordnung", // no MiLoG in title
		}},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("must not be confirmed_current when +++ cites Verordnung; got %s (%s)", rec.State, rec.Rationale)
	}
	if rec.State != domain.FreshnessUncertain && rec.State != domain.FreshnessConfirmedStale {
		t.Fatalf("got %s", rec.State)
	}
}

func TestConfirmedCurrentWhenBareBekOnNonSeeded(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "somelaw",
		Stand: domain.StandCitation{
			LawID: "somelaw", Year: 2024, Teil: 1, Number: "10", ParseOK: true,
		},
		InstrumentRefs: []domain.InstrumentRef{{
			Kind: "BEK", Teil: 1, Year: 2025, Number: "50",
			Raw: "Bek. v. 1.2.2025 I Nr. 50",
		}},
		HasSeededLinkedInstruments: false,
		LastTOCSuccess:             now.Add(-time.Hour),
		LastGIIFeedSuccess:         now.Add(-time.Hour),
		LastBGBlSuccess:            now.Add(-time.Hour),
		Now:                        now,
		MaxAge:                     6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("got %s (%s) want confirmed_current for bare BEK on non-seeded", rec.State, rec.Rationale)
	}
}

func TestUncertainWhenSectionScopedBek(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "somelaw",
		Stand: domain.StandCitation{
			LawID: "somelaw", Year: 2024, Teil: 1, Number: "10", ParseOK: true,
		},
		InstrumentRefs: []domain.InstrumentRef{{
			Kind: "BEK", Teil: 1, Year: 2025, Number: "50", SectionHint: "§ 1",
			Raw: "§ 1 Bek. v. 1.2.2025 I Nr. 50",
		}},
		HasSeededLinkedInstruments: false,
		LastTOCSuccess:             now.Add(-time.Hour),
		LastGIIFeedSuccess:         now.Add(-time.Hour),
		LastBGBlSuccess:            now.Add(-time.Hour),
		Now:                        now,
		MaxAge:                     6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s (%s) want uncertain for section-scoped BEK", rec.State, rec.Rationale)
	}
	if rec.Rationale != "unresolved_linked_instrument_refs" {
		t.Fatalf("rationale=%q", rec.Rationale)
	}
}

func TestHeuristicLinksLowerConfidence(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	iss := domain.GazetteIssue{ID: "BGBl-1/2025/5", Teil: 1, Year: 2025, Number: "5"}
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2024, Teil: 1, Number: "10", ParseOK: true},
		LinkedIssues:       []domain.GazetteIssue{iss},
		LinkClasses:        map[string]domain.LinkClass{iss.ID: domain.LinkHeuristic},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedStale {
		t.Fatalf("got %s", rec.State)
	}
	if rec.Confidence != "medium" && rec.Confidence != "low" {
		t.Fatalf("confidence=%q want lowered", rec.Confidence)
	}
}
