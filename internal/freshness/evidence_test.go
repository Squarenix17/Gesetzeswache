package freshness

import (
	"testing"
	"time"
)

func TestBGBLEvidence_freshBGBl(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	bgbl := now.Add(-time.Hour)
	eli := now.Add(-30 * time.Minute)
	got, probe := BGBLEvidence(bgbl, eli, now, 6*time.Hour)
	if probe || !got.Equal(bgbl) {
		t.Fatalf("got=%v probe=%v want bgbl fresh", got, probe)
	}
}

func TestBGBLEvidence_staleBGBlFreshNewerELI(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	bgbl := now.Add(-8 * time.Hour)
	eli := now.Add(-time.Hour)
	got, probe := BGBLEvidence(bgbl, eli, now, 6*time.Hour)
	if !probe || !got.Equal(eli) {
		t.Fatalf("got=%v probe=%v want eli probe-only", got, probe)
	}
}

func TestBGBLEvidence_staleBGBlStaleELI(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	bgbl := now.Add(-8 * time.Hour)
	eli := now.Add(-7 * time.Hour)
	got, probe := BGBLEvidence(bgbl, eli, now, 6*time.Hour)
	if !got.IsZero() || probe {
		t.Fatalf("got=%v probe=%v want zero", got, probe)
	}
}

func TestBGBLEvidence_eliOlderThanBGBl(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	bgbl := now.Add(-8 * time.Hour)
	eli := now.Add(-9 * time.Hour)
	got, probe := BGBLEvidence(bgbl, eli, now, 6*time.Hour)
	if !got.IsZero() || probe {
		t.Fatalf("got=%v probe=%v want zero when ELI older than BGBl", got, probe)
	}
}

func TestBGBLEvidence_zeros(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	got, probe := BGBLEvidence(time.Time{}, time.Time{}, now, 6*time.Hour)
	if !got.IsZero() || probe {
		t.Fatalf("got=%v probe=%v want zero", got, probe)
	}
}

func TestTimestampFresh_rejectsFutureBeyondTolerance(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	future := now.Add(10 * time.Minute)
	if TimestampFresh(future, now, 6*time.Hour) {
		t.Fatal("future timestamp beyond tolerance must be invalid")
	}
	if !TimestampFresh(now.Add(2*time.Minute), now, 6*time.Hour) {
		t.Fatal("timestamp within tolerance should be valid")
	}
}
