package export

import (
	"strings"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestIsTOCRef(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"Inhaltsübersicht", true},
		{"  Inhaltsübersicht  ", true},
		{"INHALTSÜBERSICHT", true},
		{" inhaltsverzeichnis ", true},
		{"Inhaltsverzeichnis", true},
		{"Eingangsformel", false},
		{"§ 1", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			if got := isTOCRef(tt.ref); got != tt.want {
				t.Fatalf("isTOCRef(%q) = %v want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestClassifyUnit_TOCSectionRef(t *testing.T) {
	if got := classifyUnit("Abschnitt 1", "DEMO", "Inhaltsübersicht", false); got != KindPreamble {
		t.Fatalf("TOC child kind=%s want preamble", got)
	}
	if got := classifyUnit("§ 1 Mindestlohn § 2 Fälligkeit", "DEMO", "Inhaltsübersicht", false); got != KindPreamble {
		t.Fatalf("TOC listing kind=%s want preamble", got)
	}
	if got := classifyUnit("Aufgaben", "DEMO", "Inhaltsverzeichnis", false); got != KindPreamble {
		t.Fatalf("Inhaltsverzeichnis kind=%s", got)
	}
	if got := classifyUnit("Jeder hat Anspruch.", "DEMO", "§ 1", false); got != KindNormtext {
		t.Fatalf("§ body kind=%s", got)
	}
	if got := classifyUnit("Auf Grund des § 55 …", "VO", "Eingangsformel", false); got != KindNormtext {
		t.Fatalf("Eingangsformel kind=%s want normtext", got)
	}
	if got := classifyUnit("Auf Grund des § 55 (BGBl. 2025 I Nr. 1) verordnet:", "VO", "Eingangsformel", false); got != KindNormtext {
		t.Fatalf("Eingangsformel with BGBl kind=%s want normtext", got)
	}
	if got := classifyUnit("BGBl. I 1994 S. 1170", "ArbZG", "", false); got != KindPreamble {
		t.Fatalf("BGBl orphan kind=%s want preamble", got)
	}
}

func TestTOCExcludedFromVectorAndHierarchical(t *testing.T) {
	law := domain.Law{ID: "demo", Abbreviation: "DEMO", Title: "Demo Gesetz"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "toc_enbez_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	stand := domain.StandCitation{}
	fresh := domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent}

	for _, u := range ir.Units {
		if isTOCRef(u.SectionRef) && u.Kind != KindPreamble {
			t.Fatalf("TOC unit kind=%s text=%q", u.Kind, u.Text)
		}
	}

	norm := EmitNormtext(ir, stand, fresh)
	chunked := EmitChunked(ir, stand, fresh)
	if len(norm) == 0 || len(chunked) == 0 {
		t.Fatal("expected operative vector units")
	}
	for _, c := range append(norm, chunked...) {
		if isTOCRef(c.SectionRef) {
			t.Fatalf("TOC leaked into vector: %+v", c)
		}
		if strings.Contains(c.Text, "(+++") {
			t.Fatalf("editorial +++ leaked: %q", c.Text)
		}
		if c.Kind != KindNormtext {
			t.Fatalf("vector kind=%s", c.Kind)
		}
	}
	var sawOp bool
	for _, c := range norm {
		if strings.Contains(c.Text, "Anspruch") || strings.Contains(c.Text, "12 Euro") {
			sawOp = true
		}
	}
	if !sawOp {
		t.Fatalf("missing operative § text in normtext: %+v", norm)
	}

	hier := EmitHierarchical(ir)
	if strings.Contains(hier, "Inhaltsübersicht") || strings.Contains(hier, "Abschnitt 1") {
		t.Fatalf("TOC chrome in hierarchical:\n%s", hier)
	}
	if strings.Contains(hier, "(+++") {
		t.Fatalf("+++ in hierarchical:\n%s", hier)
	}
	if !strings.Contains(hier, "## § 1") || !strings.Contains(hier, "Anspruch") {
		t.Fatalf("missing § body in hierarchical:\n%s", hier)
	}
}

func TestTOCTableStyleExcluded(t *testing.T) {
	law := domain.Law{ID: "demo2", Abbreviation: "DEMO2", Title: "Demo Gesetz Zwei"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "toc_table_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	stand := domain.StandCitation{}
	fresh := domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent}
	for _, c := range EmitNormtext(ir, stand, fresh) {
		if isTOCRef(c.SectionRef) {
			t.Fatalf("table TOC leaked: %+v", c)
		}
	}
	for _, c := range EmitChunked(ir, stand, fresh) {
		if isTOCRef(c.SectionRef) {
			t.Fatalf("table TOC leaked chunked: %+v", c)
		}
	}
	hier := EmitHierarchical(ir)
	if strings.Contains(hier, "Inhaltsübersicht") || strings.Contains(hier, "Erster Abschnitt") {
		t.Fatalf("table TOC in hierarchical:\n%s", hier)
	}
	if !strings.Contains(hier, "soziale Rechte") {
		t.Fatalf("missing § 1 body:\n%s", hier)
	}
}

func TestEingangsformelPreserved(t *testing.T) {
	law := domain.Law{ID: "vodemo", Abbreviation: "VODEMO", Title: "Demo Verordnung"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "vo_eingangsformel_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if isTOCRef("Eingangsformel") {
		t.Fatal("Eingangsformel must not be treated as TOC")
	}

	flat := EmitFlat(ir)
	if !strings.Contains(flat, "verordnet die Bundesregierung") {
		t.Fatal("flat should retain Eingangsformel body")
	}

	norm := EmitNormtext(ir, domain.StandCitation{}, domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent})
	var sawFormel, sawRate bool
	for _, c := range norm {
		if strings.Contains(c.Text, "(+++") {
			t.Fatalf("+++ in vector: %q", c.Text)
		}
		if c.SectionRef == "Eingangsformel" && strings.Contains(c.Text, "verordnet die Bundesregierung") {
			sawFormel = true
			if c.Kind != KindNormtext {
				t.Fatalf("Eingangsformel in vector should be normtext, got %s", c.Kind)
			}
			if !strings.Contains(c.Text, "BGBl") {
				t.Fatal("fixture should exercise BGBl inside Eingangsformel")
			}
		}
		if c.SectionRef == "§ 1" && strings.Contains(c.Text, "3,6") {
			sawRate = true
		}
	}
	if !sawFormel {
		t.Fatalf("Eingangsformel missing from normtext: %+v", norm)
	}
	if !sawRate {
		t.Fatalf("§ 1 missing from normtext: %+v", norm)
	}
}

func TestVectorUnitHelpers(t *testing.T) {
	if !isVectorUnit(Unit{Kind: KindNormtext}) {
		t.Fatal("normtext should be vector")
	}
	if isVectorUnit(Unit{Kind: KindPreamble}) || isVectorUnit(Unit{Kind: KindNoise}) {
		t.Fatal("preamble/noise must not be vector")
	}
	if !skipHierarchicalChrome(Unit{Kind: KindPreamble}) {
		t.Fatal("preamble is chrome")
	}
	if !skipHierarchicalChrome(Unit{SectionRef: "Inhaltsübersicht", Kind: KindNormtext}) {
		t.Fatal("TOC ref is chrome even if mis-kinded")
	}
	if skipHierarchicalChrome(Unit{Kind: KindNormtext, SectionRef: "§ 1"}) {
		t.Fatal("§ normtext is not chrome")
	}
}
