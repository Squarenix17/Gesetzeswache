package instruments

import (
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/freshness"
)

func TestCollectEvidence_softGesetzLinkedOmittedFromOperativeSlice(t *testing.T) {
	st := &evidenceMemStore{
		laws: map[string]domain.Law{
			"altzertg": {ID: "altzertg", Title: "Gesetz über die Zertifizierung von Altersvorsorgeverträgen"},
		},
		meta: map[string]string{},
	}
	linked := []domain.LinkedInstrument{{
		ParentLawID: "estg",
		Kind:        "verordnung",
		GIISlug:     "altzertg",
		Notes:       "BGBl 2020 I Nr. 123",
	}}
	operative := FilterOperativeLinked(st, linked)
	if len(operative) != 0 {
		t.Fatalf("soft Gesetz must be demoted; operative=%+v", operative)
	}

	stand := domain.StandCitation{
		LawID: "estg", Year: 2024, Teil: 1, Number: "100", ParseOK: true,
		Raw: "Zuletzt geändert durch Art. 1 G v. 1.1.2024 I Nr. 100",
	}
	refs, _ := CollectEvidence(st, operative, "estg", stand)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	rec := freshness.Evaluate(freshness.Input{
		LawID:                      "estg",
		Stand:                      stand,
		InstrumentRefs:             refs,
		HasSeededLinkedInstruments: len(operative) > 0,
		LastTOCSuccess:             now.Add(-time.Hour),
		LastGIIFeedSuccess:         now.Add(-time.Hour),
		LastBGBlSuccess:            now.Add(-time.Hour),
		Now:                        now,
		MaxAge:                     6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("soft Gesetz demoted: got %s (%s) refs=%+v", rec.State, rec.Rationale, refs)
	}
}

type evidenceMemStore struct {
	laws map[string]domain.Law
	meta map[string]string
}

func (m *evidenceMemStore) GetMeta(key string) (string, bool, error) {
	v, ok := m.meta[key]
	return v, ok, nil
}

func (m *evidenceMemStore) GetStand(lawID string) (domain.StandCitation, bool, error) {
	return domain.StandCitation{}, false, nil
}

func (m *evidenceMemStore) GetIssue(id string) (domain.GazetteIssue, bool, error) {
	return domain.GazetteIssue{}, false, nil
}

func (m *evidenceMemStore) GetLaw(id string) (domain.Law, bool, error) {
	law, ok := m.laws[id]
	return law, ok, nil
}

func TestCollectEvidence_operativeVerordnungStillFailCloses(t *testing.T) {
	st := &evidenceMemStore{
		laws: map[string]domain.Law{
			"milov5": {ID: "milov5", Title: "Fünfte Mindestlohnanpassungsverordnung"},
		},
		meta: map[string]string{},
	}
	linked := []domain.LinkedInstrument{{
		ParentLawID: "milog",
		Kind:        "verordnung",
		GIISlug:     "milov5",
		Notes:       "§ 1 V v. 5.11.2025 I Nr. 268",
	}}
	operative := FilterOperativeLinked(st, linked)
	if len(operative) != 1 {
		t.Fatal("real Verordnung must stay operative")
	}
	stand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
		Raw: "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137",
	}
	refs, _ := CollectEvidence(st, operative, "milog", stand)
	_ = citation.Parse("milog", stand.Raw)
	if len(refs) == 0 {
		t.Fatal("expected refs from linked Verordnung notes")
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	rec := freshness.Evaluate(freshness.Input{
		LawID:                      "milog",
		Stand:                      stand,
		InstrumentRefs:             refs,
		HasSeededLinkedInstruments: len(operative) > 0,
		LastTOCSuccess:             now.Add(-time.Hour),
		LastGIIFeedSuccess:         now.Add(-time.Hour),
		LastBGBlSuccess:            now.Add(-time.Hour),
		Now:                        now,
		MaxAge:                     6 * time.Hour,
	})
	if rec.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("operative Verordnung must fail-close; got %s (%s)", rec.State, rec.Rationale)
	}
}
