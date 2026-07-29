package freshness

import (
	"testing"
	"time"
)

func TestEffectiveBGBlFeedTime_degradedNewerThanSuccess(t *testing.T) {
	success := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	degraded := success.Add(time.Hour)
	got := EffectiveBGBlFeedTime(success, degraded)
	if !got.IsZero() {
		t.Fatalf("got=%v want zero when degraded newer than success", got)
	}
}

func TestEffectiveBGBlFeedTime_degradedOlderThanSuccessIgnored(t *testing.T) {
	success := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	degraded := success.Add(-time.Hour)
	got := EffectiveBGBlFeedTime(success, degraded)
	if !got.Equal(success) {
		t.Fatalf("got=%v want success preserved when degraded older", got)
	}
}

func TestEffectiveBGBlFeedTime_noDegraded(t *testing.T) {
	success := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	got := EffectiveBGBlFeedTime(success, time.Time{})
	if !got.Equal(success) {
		t.Fatalf("got=%v want success", got)
	}
}
