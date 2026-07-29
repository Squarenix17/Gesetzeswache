package freshness

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestUncertainWhenGIIStale(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-8 * time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s (%s) want uncertain when GII stale", rec.State, rec.Rationale)
	}
}

func TestUncertainWhenGIIMissing(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:           "bgb",
		Stand:           domain.StandCitation{Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:  now.Add(-time.Hour),
		LastBGBlSuccess: now.Add(-time.Hour),
		Now:             now,
		MaxAge:          6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s want uncertain when GII missing", rec.State)
	}
}

func TestConfirmedCurrentWhenAllFeedsFresh(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("got %s (%s) want confirmed_current", rec.State, rec.Rationale)
	}
}

func TestUncertainWhenProbeOnlyEvidence(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		BGBlFromProbeOnly:  true,
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s want uncertain", rec.State)
	}
	if rec.Rationale != "bgbl_evidence_probe_only" {
		t.Fatalf("rationale=%q", rec.Rationale)
	}
}

func TestUncertainWhenLinksReadFailed(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		LinksReadFailed:    true,
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain || rec.Rationale != "links_read_failed" {
		t.Fatalf("got %s (%s)", rec.State, rec.Rationale)
	}
}

func TestUncertainWhenDiscoveredLinksReadFailed(t *testing.T) {
	// DiscoveredForParent failures use the same LinksReadFailed fail-closed path.
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		LinksReadFailed:    true,
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain || rec.Rationale != "links_read_failed" {
		t.Fatalf("got %s (%s)", rec.State, rec.Rationale)
	}
}

func TestUncertainWhenFutureSyncTimestamp(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	rec := Evaluate(Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2024, Teil: 1, Number: "100", ParseOK: true},
		LastTOCSuccess:     now.Add(10 * time.Minute),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    now.Add(-time.Hour),
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("got %s want uncertain for future TOC timestamp", rec.State)
	}
}
