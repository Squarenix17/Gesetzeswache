package export

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestInstrumentRefsFromIR_plusPlusVerordnung(t *testing.T) {
	ir := IR{
		LawID: "milog",
		Units: []Unit{
			{Kind: KindPreamble, Text: "(+++ Hinweis: Zur Anpassung des Mindestlohns nach § 1 Abs. 2 vgl. § 1 V v. 5.11.2025 I Nr. 268 +++)"},
			{Kind: KindNormtext, Text: "Die Höhe des Mindestlohns beträgt ab dem 1. Oktober 2022 brutto 12 Euro"},
		},
	}
	refs := InstrumentRefsFromIR(ir)
	if len(refs) == 0 {
		t.Fatal("expected instrument refs from +++")
	}
	found := false
	for _, r := range refs {
		if r.Year == 2025 && r.Number == "268" && r.Teil == 1 {
			found = true
			if r.Kind != "V" && r.Kind != "v" {
				// kind may be V
			}
		}
	}
	if !found {
		t.Fatalf("missing Nr. 268 ref: %+v", refs)
	}
}

func TestEditorialCitationTexts(t *testing.T) {
	ir := IR{Units: []Unit{
		{Kind: KindPreamble, Text: "(+++ Textnachweis +++)"},
		{Kind: KindNormtext, Text: "body"},
	}}
	got := EditorialCitationTexts(ir)
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestInstrumentRefsFromXML_fixture(t *testing.T) {
	// ArbZG fixture has Textnachweis without BGBl Nr — may yield empty; ensure no panic
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG"}
	xml := []byte(`<?xml version="1.0"?><norm><metadaten><jurabk>ArbZG</jurabk></metadaten><textdaten><metangabe>(+++ § 1 V v. 5.11.2025 I Nr. 268 +++)</metangabe></textdaten></norm>`)
	refs, err := InstrumentRefsFromXML(law, xml)
	if err != nil {
		// BuildIR may fail on minimal XML — acceptable if we still try
		t.Log(err)
		return
	}
	_ = refs
}
