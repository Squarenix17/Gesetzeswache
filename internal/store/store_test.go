package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestCountFreshnessByState(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	now := time.Now().UTC()
	records := []domain.FreshnessRecord{
		{LawID: "a", State: domain.FreshnessConfirmedCurrent, EvaluatedAt: now},
		{LawID: "b", State: domain.FreshnessConfirmedCurrent, EvaluatedAt: now},
		{LawID: "c", State: domain.FreshnessConfirmedStale, EvaluatedAt: now},
		{LawID: "d", State: domain.FreshnessUncertain, EvaluatedAt: now},
	}
	for _, r := range records {
		if err := st.PutFreshness(r); err != nil {
			t.Fatal(err)
		}
	}

	counts, err := st.CountFreshnessByState()
	if err != nil {
		t.Fatal(err)
	}
	if counts[domain.FreshnessConfirmedCurrent] != 2 {
		t.Fatalf("current=%d want 2", counts[domain.FreshnessConfirmedCurrent])
	}
	if counts[domain.FreshnessConfirmedStale] != 1 {
		t.Fatalf("stale=%d want 1", counts[domain.FreshnessConfirmedStale])
	}
	if counts[domain.FreshnessUncertain] != 1 {
		t.Fatalf("uncertain=%d want 1", counts[domain.FreshnessUncertain])
	}
}

func TestCountFreshnessByState_empty(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	counts, err := st.CountFreshnessByState()
	if err != nil {
		t.Fatal(err)
	}
	if counts[domain.FreshnessConfirmedCurrent] != 0 ||
		counts[domain.FreshnessConfirmedStale] != 0 ||
		counts[domain.FreshnessUncertain] != 0 {
		t.Fatalf("want zeros, got %#v", counts)
	}
}
