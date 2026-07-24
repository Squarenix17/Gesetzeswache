package instruments

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

type memStore struct {
	meta  map[string]string
	stand map[string]domain.StandCitation
	issue map[string]domain.GazetteIssue
}

func (m *memStore) GetMeta(key string) (string, bool, error) {
	v, ok := m.meta[key]
	return v, ok, nil
}
func (m *memStore) GetStand(lawID string) (domain.StandCitation, bool, error) {
	v, ok := m.stand[lawID]
	return v, ok, nil
}
func (m *memStore) GetIssue(id string) (domain.GazetteIssue, bool, error) {
	v, ok := m.issue[id]
	return v, ok, nil
}

func TestCollectEvidence_editorialAndSeed(t *testing.T) {
	st := &memStore{
		meta: map[string]string{
			"editorial:milog": "(+++ § 1 V v. 5.11.2025 I Nr. 268 +++)",
		},
		stand: map[string]domain.StandCitation{},
		issue: map[string]domain.GazetteIssue{
			citation.IssueID(1, 2025, "268"): {
				ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
			},
		},
	}
	cat := &Catalog{byParent: map[string][]domain.LinkedInstrument{
		"milog": {{
			ParentLawID: "milog", Kind: "verordnung", GIISlug: "milov5",
			Notes: "Fünfte (BGBl 2025 I Nr. 268)",
		}},
	}}
	stand := domain.StandCitation{
		LawID: "milog", Year: 2026, Teil: 1, Number: "137", ParseOK: true,
		Raw: "G v. 12.5.2026 I Nr. 137",
	}
	refs, issues := CollectEvidence(st, cat, "milog", stand)
	if len(refs) == 0 {
		t.Fatal("expected refs")
	}
	if len(issues) != 1 || issues[0].Number != "268" {
		t.Fatalf("issues=%+v", issues)
	}
}
