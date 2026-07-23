package freshness

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestConfirmedCurrent(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "bgb",
		Stand: domain.StandCitation{LawID: "bgb", Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:   now.Add(-time.Hour),
		LastBGBlSuccess:  now.Add(-time.Hour),
		Now:              now,
		MaxAge:           6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("got %s (%s)", rec.State, rec.Rationale)
	}
}

func TestForcedUncertainOnOldSync(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID: "bgb",
		Stand: domain.StandCitation{ParseOK: true, Year: 2024, Teil: 1, Number: "1"},
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
		LastTOCSuccess:  now.Add(-time.Hour),
		LastBGBlSuccess: now.Add(-time.Hour),
		Now:             now,
		MaxAge:          6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedStale {
		t.Fatalf("got %s", rec.State)
	}
}
