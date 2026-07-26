package discovery

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestPreciseParentCandidates_builtinGenitiveContainment(t *testing.T) {
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz", GIIPath: "milog"},
			{ID: "lag", Abbreviation: "LAG", Title: "Gesetz über den Lastenausgleich", GIIPath: "lag"},
			{ID: "aentg2009", Abbreviation: "AEntG", Title: "Gesetz über zwingende Arbeitsbedingungen für grenzüberschreitende Entsendungen", GIIPath: "aentg2009"},
		},
	}

	tests := []struct {
		phrase string
		wantID string
	}{
		{phrase: "Mindestlohngesetzes", wantID: "milog"},
		{phrase: "Lastenausgleichsgesetzes", wantID: "lag"},
		{phrase: "Arbeitnehmer-Entsendegesetzes", wantID: "aentg2009"},
	}

	for _, tt := range tests {
		t.Run(tt.phrase, func(t *testing.T) {
			e := Ermaechtigung{LawTitlePhrase: tt.phrase}
			ids := preciseParentCandidates(e, lookup)
			id, ok := uniqueFrom(ids)
			if !ok || id != tt.wantID {
				t.Fatalf("uniqueFrom(preciseParentCandidates)=%q ok=%v want %q", id, ok, tt.wantID)
			}
		})
	}
}
