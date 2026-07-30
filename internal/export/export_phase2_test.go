package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func loadTestdata(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("testdata", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}

func TestArbZGFixtureWhitespaceAndMetadata(t *testing.T) {
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "arbzg_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ir.Units) == 0 {
		t.Fatal("expected units from ArbZG fixture")
	}

	joined := ""
	for _, u := range ir.Units {
		joined += u.Text
		if strings.Contains(u.Text, "ArbZGArbZG") {
			t.Fatalf("glued abbreviation in unit text: %q", u.Text)
		}
		if strings.Contains(u.Text, "§ 1Zweck") || strings.Contains(u.Text, "§1Zweck") {
			t.Fatalf("glued section+title in unit text: %q", u.Text)
		}
	}
	_ = joined

	var foundPara1, foundNoise, foundHeading bool
	for _, u := range ir.Units {
		if u.Kind == KindNoise && strings.TrimSpace(u.Text) == "ArbZG" {
			foundNoise = true
		}
		if u.Kind == KindSectionHeading {
			foundHeading = true
			if u.SectionTitle == "" {
				t.Errorf("section_heading missing section_title: %+v", u)
			}
		}
		if u.SectionRef == "§ 1" && u.Kind == KindNormtext {
			foundPara1 = true
			if u.SectionTitle == "" {
				t.Errorf("§ 1 missing section_title")
			}
			if u.ParagraphNum != "1" {
				t.Errorf("§ 1 Abs. want 1, got %q", u.ParagraphNum)
			}
			if !strings.Contains(u.Text, "Zweck des Gesetzes") {
				t.Errorf("§ 1 text missing Zweck body: %q", u.Text)
			}
			if strings.HasPrefix(u.SectionKey, "enbez-") {
				t.Errorf("section_key should not be synthetic enbez-N, got %q", u.SectionKey)
			}
		}
	}
	if !foundPara1 {
		t.Fatal("expected normtext unit for § 1")
	}
	if !foundNoise {
		t.Fatal("expected bare ArbZG classified as noise")
	}
	if !foundHeading {
		t.Fatal("expected at least one section_heading unit")
	}

	h := EmitHierarchical(ir)
	if strings.Contains(h, "## enbez-") || strings.Contains(h, "## gliederungseinheit-") {
		t.Fatalf("hierarchical leaked internal keys:\n%s", h[:min(400, len(h))])
	}
}

func TestArbZGContentLowercaseFlush(t *testing.T) {
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "arbzg_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var absCount int
	for _, u := range ir.Units {
		if u.SectionRef == "§ 2" && u.Kind == KindNormtext {
			absCount++
			if u.ParagraphNum == "" {
				t.Errorf("§ 2 unit missing paragraph_num: %q", u.Text[:min(60, len(u.Text))])
			}
		}
	}
	if absCount < 5 {
		t.Fatalf("expected ≥5 § 2 Absätze from lowercase <content>, got %d", absCount)
	}
}

func TestBGBFixtureSectionHeadingsAndNormtext(t *testing.T) {
	law := domain.Law{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "bgb_header_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var tocLike, para1 bool
	for _, u := range ir.Units {
		if strings.Contains(u.Text, "Inhaltsübersicht") {
			if u.Kind != KindPreamble && u.Kind != KindNoise {
				t.Errorf("Inhaltsübersicht should be preamble/noise, got %s", u.Kind)
			}
			tocLike = true
		}
		if u.SectionRef == "§ 1" && u.Kind == KindNormtext {
			para1 = true
			if !strings.Contains(u.Text, "Rechtsfähigkeit") {
				t.Errorf("§ 1 body unexpected: %q", u.Text)
			}
		}
		if u.Kind == KindSectionHeading && strings.Contains(u.Text, "010Erster") {
			if u.SectionTitle == "" {
				t.Error("smashed heading should still have section_title from attributes")
			}
		}
	}
	if !tocLike {
		t.Fatal("expected Inhaltsübersicht unit")
	}
	if !para1 {
		t.Fatal("expected BGB § 1 normtext")
	}
}

func TestChunkedKindAndSectionRef(t *testing.T) {
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "arbzg_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	chunks := EmitChunked(ir, domain.StandCitation{Raw: "Stand: x"}, domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent})
	vectorIDs := vectorUnitIDs(ir)
	if len(chunks) != len(vectorIDs) {
		t.Fatalf("boundary mismatch chunks=%d vector=%d ir=%d", len(chunks), len(vectorIDs), len(ir.Units))
	}
	var hasKind, hasRef bool
	for i, c := range chunks {
		if c.UnitID != vectorIDs[i] {
			t.Fatalf("unit_id mismatch at %d", i)
		}
		if c.Kind == "" {
			t.Fatalf("chunk %d missing kind", i)
		}
		hasKind = true
		if c.SectionRef != "" {
			hasRef = true
		}
	}
	if !hasKind || !hasRef {
		t.Fatal("expected kind and at least one section_ref on chunks")
	}
}

func TestNormtextFormatFilters(t *testing.T) {
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "arbzg_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	all := EmitChunked(ir, domain.StandCitation{}, domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent})
	norm := EmitNormtext(ir, domain.StandCitation{}, domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent})
	if len(norm) == 0 {
		t.Fatal("normtext empty")
	}
	if len(norm) != len(all) {
		t.Fatalf("chunked and normtext should match: norm=%d chunked=%d", len(norm), len(all))
	}
	ids := map[string]bool{}
	for _, u := range ir.Units {
		ids[u.ID] = true
	}
	for _, c := range norm {
		if c.Kind != KindNormtext {
			t.Fatalf("normtext chunk kind=%s", c.Kind)
		}
		if !ids[c.UnitID] {
			t.Fatalf("normtext unit_id %s not in full IR", c.UnitID)
		}
	}
}

func TestCrossFormatBoundariesFullFormats(t *testing.T) {
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz"}
	ir, err := BuildIR(law, "cid", loadTestdata(t, "arbzg_snippet.xml"))
	if err != nil {
		t.Fatal(err)
	}
	ids := UnitIDs(ir)
	vectorIDs := vectorUnitIDs(ir)
	chunks := EmitChunked(ir, domain.StandCitation{}, domain.FreshnessRecord{State: domain.FreshnessConfirmedCurrent})
	flat := EmitFlat(ir)
	if len(chunks) != len(vectorIDs) {
		t.Fatalf("chunked/vector boundary fail: chunks=%d vector=%d ir=%d", len(chunks), len(vectorIDs), len(ids))
	}
	for _, id := range ids {
		if !strings.Contains(flat, id) {
			t.Fatalf("flat missing unit id %s", id)
		}
	}
	if EmitHierarchical(ir) == "" {
		t.Fatal("empty hierarchical")
	}
}

func TestContentIDFromStandUnknown(t *testing.T) {
	if got := ContentIDFromStand(domain.StandCitation{}); got != "unknown" {
		t.Fatalf("empty stand → unknown, got %q", got)
	}
	got := ContentIDFromStand(domain.StandCitation{Teil: 1, Year: 2022, Number: "1170", Page: "1171"})
	if got == "unknown" || got == "0/0//" {
		t.Fatalf("parsed stand unexpected: %q", got)
	}
}

func TestBuildIRMalformedXML(t *testing.T) {
	law := domain.Law{ID: "x", Abbreviation: "X", Title: "X"}
	_, err := BuildIR(law, "cid", []byte(`<norm><enbez>§ 1</enbez><P>unclosed`))
	if err == nil {
		t.Fatal("expected parse error for malformed XML")
	}
}

func TestClassifyUnitHelpers(t *testing.T) {
	tests := []struct {
		name string
		text string
		abbr string
		ref  string
		want UnitKind
	}{
		{"bare abbrev", "ArbZG", "ArbZG", "", KindNoise},
		{"textnachweis", "(+++ Textnachweis der Geltung +++)", "ArbZG", "", KindPreamble},
		{"toc", "Inhaltsübersicht", "BGB", "", KindPreamble},
		{"toc child under enbez", "Abschnitt 1 Allgemeine Vorschriften", "DEMO", "Inhaltsübersicht", KindPreamble},
		{"toc verzeichnis ref", "§ 1 Zweck", "DEMO", "Inhaltsverzeichnis", KindPreamble},
		{"norm", "Die werktägliche Arbeitszeit…", "ArbZG", "§ 2", KindNormtext},
		{"real para 1", "Dieses Gesetz regelt den Zweck der Norm.", "DEMO", "§ 1", KindNormtext},
		{"heading smash", "010Erster AbschnittAllgemeine Vorschriften", "ArbZG", "", KindSectionHeading},
		{"footnote", "Fußnote 1: siehe Abs. 2", "ArbZG", "", KindFootnote},
		{"bgbl under paragraph", "geändert durch BGBl. I S. 1", "ArbZG", "§ 1", KindNormtext},
		{"bgbl orphan", "BGBl. I 1994 S. 1170", "ArbZG", "", KindPreamble},
		{"eingangsformel", "Auf Grund des § 55 …", "VO", "Eingangsformel", KindNormtext},
		{"eingangsformel bgbl", "Auf Grund des § 55 (BGBl. 2025 I Nr. 1) verordnet:", "VO", "Eingangsformel", KindNormtext},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyUnit(tt.text, tt.abbr, tt.ref, false)
			if got != tt.want {
				t.Fatalf("classifyUnit(%q) = %s want %s", tt.text, got, tt.want)
			}
		})
	}
}
