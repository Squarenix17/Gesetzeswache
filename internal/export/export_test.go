package export

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestBuildAndFormatsSameUnits(t *testing.T) {
	law := domain.Law{ID: "demo", Abbreviation: "DEMO", Title: "Demo"}
	xml := []byte(`<?xml version="1.0"?><norm><titel>Demo</titel><enbez id="s1"><P nr="1">Hello world.</P><P nr="2">Second para.</P></enbez></norm>`)
	ir, err := BuildIR(law, "cid", xml)
	if err != nil {
		t.Fatal(err)
	}
	if len(ir.Units) < 1 {
		t.Fatalf("expected units, got %d amb=%v", len(ir.Units), ir.StructuralAmbiguity)
	}
	ids := UnitIDs(ir)
	vectorIDs := vectorUnitIDs(ir)
	h := EmitHierarchical(ir)
	f := EmitFlat(ir)
	chunks := EmitChunked(ir, domain.StandCitation{Raw: "x"}, domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent})
	if len(chunks) != len(vectorIDs) {
		t.Fatalf("chunk count %d != vector units %d (ir units %d)", len(chunks), len(vectorIDs), len(ids))
	}
	for i, c := range chunks {
		if c.UnitID != vectorIDs[i] {
			t.Fatalf("boundary mismatch at %d: %s vs %s", i, c.UnitID, vectorIDs[i])
		}
		if c.Kind != KindNormtext {
			t.Fatalf("chunk %d kind=%s want normtext", i, c.Kind)
		}
		if c.Text == "" {
			t.Fatal("empty text not allowed")
		}
	}
	if h == "" || f == "" {
		t.Fatal("empty format output")
	}
	// flat must contain each unit id marker
	for _, id := range ids {
		if !contains(f, id) {
			t.Fatalf("flat missing id %s", id)
		}
	}
}

func vectorUnitIDs(ir IR) []string {
	var ids []string
	for _, u := range ir.Units {
		if isVectorUnit(u) {
			ids = append(ids, u.ID)
		}
	}
	return ids
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
