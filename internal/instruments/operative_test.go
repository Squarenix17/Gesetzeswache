package instruments

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

type titleStore map[string]domain.Law

func (m titleStore) GetLaw(id string) (domain.Law, bool, error) {
	law, ok := m[id]
	return law, ok, nil
}

func TestFilterOperativeLinked_demotesSoftGesetzFP(t *testing.T) {
	st := titleStore{
		"altzertg": {ID: "altzertg", Title: "Gesetz über die Zertifizierung von Altersvorsorgeverträgen"},
		"astg":     {ID: "astg", Title: "Gesetz zur Regelung der Rechtsverhältnisse der in der Steuerverwaltung tätigen Personen"},
		"milov5":   {ID: "milov5", Title: "Fünfte Mindestlohnanpassungsverordnung"},
	}
	linked := []domain.LinkedInstrument{
		{ParentLawID: "estg", Kind: "verordnung", GIISlug: "altzertg", Source: "discovered", Confidence: "high"},
		{ParentLawID: "estg", Kind: "verordnung", GIISlug: "astg", Source: "discovered", Confidence: "high"},
		{ParentLawID: "milog", Kind: "verordnung", GIISlug: "milov5", Notes: "§ 1 V v. 5.11.2025 I Nr. 268"},
	}

	got := FilterOperativeLinked(st, linked)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1 operative; got %+v", len(got), got)
	}
	if got[0].GIISlug != "milov5" {
		t.Fatalf("want milov5 operative, got %+v", got[0])
	}
}

func TestFilterOperativeLinked_keepsVerordnungWithoutStoreTitle(t *testing.T) {
	linked := []domain.LinkedInstrument{
		{ParentLawID: "milog", Kind: "verordnung", GIISlug: "milov5", Notes: "§ 1 V v. 5.11.2025 I Nr. 268"},
	}
	got := FilterOperativeLinked(nil, linked)
	if len(got) != 1 {
		t.Fatalf("unknown title must stay operative; got %+v", got)
	}
}
