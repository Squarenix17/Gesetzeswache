package discovery

import (
	"strings"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
)

func TestCatalogLookup_ByTitlePhrase_prefersPhraseContainsTitle_overCitingVerordnung(t *testing.T) {
	catalog := CatalogLookup{
		Laws: []domain.Law{
			{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch", GIIPath: "bgb"},
			{
				ID: "minuhv", Abbreviation: "MinUhV", GIIPath: "minuhv",
				Title: "Verordnung zur Festlegung des Mindestunterhalts minderjähriger Kinder nach § 1612a Absatz 1 des Bürgerlichen Gesetzbuchs",
			},
			{ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz", GIIPath: "milog"},
			{
				ID: "milov5", Abbreviation: "MiLoV5", GIIPath: "milov5",
				Title: "Fünfte Verordnung zur Anpassung der Höhe des Mindestlohns auf Grundlage des Mindestlohngesetzes",
			},
		},
	}

	tests := []struct {
		name   string
		phrase string
		want   []string
	}{
		{
			name:   "BGB genitive phrase excludes citing MinUhV",
			phrase: "Bürgerlichen Gesetzbuchs",
			want:   []string{"bgb"},
		},
		{
			name:   "MiLoG genitive phrase excludes citing MiLoV5",
			phrase: "Mindestlohngesetzes",
			want:   []string{"milog"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := catalog.ByTitlePhrase(tt.phrase)
			if len(got) != len(tt.want) {
				t.Fatalf("ByTitlePhrase(%q)=%v want %v", tt.phrase, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("ByTitlePhrase(%q)=%v want %v", tt.phrase, got, tt.want)
				}
			}
		})
	}
}

func TestLooksLikeVerordnung(t *testing.T) {
	tests := []struct {
		name string
		law  domain.Law
		want bool
	}{
		{
			name: "title contains Verordnung",
			law:  domain.Law{Title: "Pflegeberufe-Ausbildungs- und Prüfungsverordnung"},
			want: true,
		},
		{
			name: "abbr ends with V",
			law:  domain.Law{Abbreviation: "PBAV"},
			want: true,
		},
		{
			name: "slug year pattern pbav_2025",
			law:  domain.Law{GIIPath: "pbav_2025"},
			want: true,
		},
		{
			name: "slug milov5",
			law:  domain.Law{GIIPath: "milov5"},
			want: true,
		},
		{
			name: "ordinary statute",
			law:  domain.Law{Title: "Bürgerliches Gesetzbuch", Abbreviation: "BGB", GIIPath: "bgb"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikeVerordnung(tt.law); got != tt.want {
				t.Fatalf("LooksLikeVerordnung()=%v want %v for %+v", got, tt.want, tt.law)
			}
		})
	}
}

func TestIngestLawXML_MinUhV_discoversBGB(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch", GIIPath: "bgb"},
			{
				ID: "minuhv", Abbreviation: "MinUhV", GIIPath: "minuhv",
				Title: "Verordnung zur Festlegung des Mindestunterhalts minderjähriger Kinder nach § 1612a Absatz 1 des Bürgerlichen Gesetzbuchs",
			},
		},
	}
	law := domain.Law{
		ID:           "minuhv",
		Abbreviation: "MinUhV",
		Title:        "Verordnung zur Festlegung des Mindestunterhalts minderjähriger Kinder nach § 1612a Absatz 1 des Bürgerlichen Gesetzbuchs",
		GIIPath:      "minuhv",
	}
	xmlData := fixtures.MustRead("minuhv_snippet.xml")

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1; discovered=%+v", n, st.discovered)
	}

	edges := st.discovered["bgb|minuhv"]
	if len(edges) != 1 {
		t.Fatalf("discovered edges=%d want 1; all=%+v", len(edges), st.discovered)
	}
	e := edges[0]
	if e.ParentLawID != "bgb" {
		t.Fatalf("ParentLawID=%q want bgb", e.ParentLawID)
	}
	if e.SectionHint != "§ 1612a" {
		t.Fatalf("SectionHint=%q want § 1612a", e.SectionHint)
	}
	if e.Confidence != ConfidenceHigh {
		t.Fatalf("Confidence=%q want high", e.Confidence)
	}
	if e.EffectiveFrom != "" {
		t.Fatalf("EffectiveFrom=%q want empty (minuhv fixture lacks ausfertigung-datum)", e.EffectiveFrom)
	}
}

func TestIngestLawXML_EffectiveFrom_notFromAusfertigungDatum(t *testing.T) {
	// Ausfertigung ≠ Inkrafttreten; do not write EffectiveFrom from ausfertigung-datum alone.
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch", GIIPath: "bgb"},
			{
				ID: "minuhv", Abbreviation: "MinUhV", GIIPath: "minuhv",
				Title: "Verordnung zur Festlegung des Mindestunterhalts minderjähriger Kinder nach § 1612a Absatz 1 des Bürgerlichen Gesetzbuchs",
			},
		},
	}
	law := domain.Law{
		ID:           "minuhv",
		Abbreviation: "MinUhV",
		Title:        "Verordnung zur Festlegung des Mindestunterhalts minderjähriger Kinder nach § 1612a Absatz 1 des Bürgerlichen Gesetzbuchs",
		GIIPath:      "minuhv",
	}
	xmlData := []byte(`<?xml version="1.0"?><dokument><norm><metadaten>
      <fundstelle><periodikum>BGBl. I</periodikum><zit>2015 Nr. 2188</zit></fundstelle>
      <ausfertigung-datum man="ja">2015-12-01</ausfertigung-datum>
    </metadaten><textdaten><text><Content><P>Auf Grund des § 1612a Absatz 4 des Bürgerlichen Gesetzbuchs, der durch Artikel 1 Nummer 3 des Gesetzes vom 20. November 2015 (BGBl. I S. 2018) eingefügt worden ist, verordnet das Bundesministerium der Justiz und für Verbraucherschutz:</P></Content></text></textdaten></norm></dokument>`)

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1; discovered=%+v", n, st.discovered)
	}
	edges := st.discovered["bgb|minuhv"]
	if len(edges) != 1 {
		t.Fatalf("discovered edges=%d want 1; all=%+v", len(edges), st.discovered)
	}
	if edges[0].EffectiveFrom != "" {
		t.Fatalf("EffectiveFrom=%q want empty (ausfertigung must not become Inkrafttreten)", edges[0].EffectiveFrom)
	}
}

func TestFundstelleFromXML_PBAV(t *testing.T) {
	xmlData := fixtures.MustRead("pbav_2025_snippet.xml")
	teil, year, number, ok := FundstelleFromXML(xmlData)
	if !ok {
		t.Fatal("expected ok fundstelle")
	}
	if teil != 1 {
		t.Fatalf("teil=%d want 1", teil)
	}
	if year != 2024 {
		t.Fatalf("year=%d want 2024", year)
	}
	if number != "446" {
		t.Fatalf("number=%q want 446", number)
	}
}

func TestExtractPreambleText_PBAV(t *testing.T) {
	xmlData := fixtures.MustRead("pbav_2025_snippet.xml")
	got := ExtractPreambleText(xmlData)
	if got == "" {
		t.Fatal("expected non-empty preamble")
	}
	if !containsFold(got, "Auf Grund") {
		t.Fatalf("preamble missing Auf Grund: %q", got)
	}
	if !containsFold(got, "§ 55") {
		t.Fatalf("preamble missing § 55: %q", got)
	}
}

func TestIngestLawXML_PBAV(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb11", Abbreviation: "SGB XI", Title: "Elftes Buch Sozialgesetzbuch"},
		},
		Variants: []domain.LawVariant{
			{Variant: "SGB XI", LawID: "sgb11"},
		},
	}
	law := domain.Law{
		ID:           "pbav2025",
		Abbreviation: "PBAV 2025",
		Title:        "Pflegeberufe-Ausbildungs- und Prüfungsverordnung",
		GIIPath:      "pbav_2025",
	}
	xmlData := fixtures.MustRead("pbav_2025_snippet.xml")

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1", n)
	}

	idx, ok := st.bgblIndex["BGBl-1/2024/446"]
	if !ok {
		t.Fatal("expected BGBl index entry")
	}
	if idx.GIISlug != "pbav_2025" || idx.LawID != "pbav2025" {
		t.Fatalf("index=%+v unexpected", idx)
	}

	edges := st.discovered["sgb11|pbav_2025"]
	if len(edges) != 1 {
		t.Fatalf("discovered edges=%d want 1; all=%+v", len(edges), st.discovered)
	}
	e := edges[0]
	if e.ParentLawID != "sgb11" {
		t.Fatalf("ParentLawID=%q want sgb11", e.ParentLawID)
	}
	if e.SectionHint != "§ 55" {
		t.Fatalf("SectionHint=%q want § 55", e.SectionHint)
	}
	if e.Confidence != ConfidenceHigh {
		t.Fatalf("Confidence=%q want high", e.Confidence)
	}
	if e.EdgeType != EdgeErmaechtigung {
		t.Fatalf("EdgeType=%q want %q", e.EdgeType, EdgeErmaechtigung)
	}
	if e.ChildLawID != "pbav2025" {
		t.Fatalf("ChildLawID=%q want pbav2025", e.ChildLawID)
	}
	if e.GIISlug != "pbav_2025" {
		t.Fatalf("GIISlug=%q want pbav_2025", e.GIISlug)
	}
	if e.Notes != "BGBl. 2024 I Nr. 446" {
		t.Fatalf("Notes=%q want BGBl. 2024 I Nr. 446", e.Notes)
	}
}

func TestIngestLawXML_Idempotent(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb11", Abbreviation: "SGB XI", Title: "Elftes Buch Sozialgesetzbuch"},
		},
	}
	law := domain.Law{ID: "pbav2025", GIIPath: "pbav_2025"}
	xmlData := fixtures.MustRead("pbav_2025_snippet.xml")

	n1, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 1 || n2 != 1 {
		t.Fatalf("n1=%d n2=%d want 1 each", n1, n2)
	}
	if len(st.discovered) != 1 {
		t.Fatalf("discovered keys=%d want 1", len(st.discovered))
	}
}

func TestExtractPreambleText_WoGV_Fussnoten(t *testing.T) {
	xmlData := fixtures.MustRead("wogv_fussnote_snippet.xml")
	got := ExtractPreambleText(xmlData)
	if got == "" {
		t.Fatal("expected non-empty preamble from fussnoten")
	}
	if !containsFold(got, "aufgrund") {
		t.Fatalf("preamble missing aufgrund: %q", got)
	}
	if !strings.Contains(got, "§ 36") {
		t.Fatalf("preamble missing § 36: %q", got)
	}
	if containsFold(got, "Vertrages") {
		t.Fatalf("body false positive must not win over fussnoten: %q", got)
	}
}

func TestIngestLawXML_WoGV_discoversWoGG(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "wogg", Abbreviation: "WoGG", Title: "Wohngeldgesetz", GIIPath: "wogg"},
			{ID: "wogv", Abbreviation: "WoGV", Title: "Wohngeldverordnung", GIIPath: "wogv"},
		},
	}
	law := domain.Law{
		ID:           "wogv",
		Abbreviation: "WoGV",
		Title:        "Wohngeldverordnung",
		GIIPath:      "wogv",
	}
	xmlData := fixtures.MustRead("wogv_fussnote_snippet.xml")

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1; discovered=%+v", n, st.discovered)
	}

	edges := st.discovered["wogg|wogv"]
	if len(edges) != 1 {
		t.Fatalf("discovered edges=%d want 1; all=%+v", len(edges), st.discovered)
	}
	e := edges[0]
	if e.ParentLawID != "wogg" {
		t.Fatalf("ParentLawID=%q want wogg", e.ParentLawID)
	}
	if e.SectionHint != "§ 36" {
		t.Fatalf("SectionHint=%q want § 36", e.SectionHint)
	}
	if e.Confidence != ConfidenceHigh {
		t.Fatalf("Confidence=%q want high", e.Confidence)
	}
	if e.GIISlug != "wogv" {
		t.Fatalf("GIISlug=%q want wogv", e.GIISlug)
	}
}

func TestIngestLawXML_AmbiguousPhraseNoTitleFallback(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "wogg", Abbreviation: "WoGG", Title: "Wohngeldgesetz", GIIPath: "wogg"},
			{ID: "sgb5", Abbreviation: "SGB_5", Title: "Sozialgesetzbuch (SGB) Fünftes Buch (V)"},
			{ID: "sgb11", Abbreviation: "SGB_11", Title: "Sozialgesetzbuch (SGB) Elftes Buch (XI)"},
		},
	}
	law := domain.Law{
		ID:           "wogv",
		Abbreviation: "WoGV",
		Title:        "Wohngeldverordnung",
		GIIPath:      "wogv",
	}
	// Explicit "Sozialgesetzbuch" phrase is ambiguous; title fallback to WoGG must not run.
	xmlData := []byte(`<?xml version="1.0"?><dokument><norm><metadaten>
      <fundstelle><periodikum>BGBl. I</periodikum><zit>2025 Nr. 1</zit></fundstelle>
    </metadaten><textdaten><text><Content><P>Auf Grund des § 1 des Sozialgesetzbuches vom 1.1.2020 (BGBl. I S. 1):</P></Content></text></textdaten></norm></dokument>`)

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 0 {
		t.Fatalf("nLinks=%d want 0 (ambiguous phrase must not fall back to WoGG); discovered=%+v", n, st.discovered)
	}
	if len(st.discovered) != 0 {
		t.Fatalf("unexpected edges: %+v", st.discovered)
	}
}

func TestIngestLawXML_Mindestlohnanpassungsverordnung_noFalseMiLoG(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz", GIIPath: "milog"},
		},
	}
	law := domain.Law{
		ID:           "milov5",
		Abbreviation: "MiLoV5",
		Title:        "Mindestlohnanpassungsverordnung",
		GIIPath:      "milov5",
	}
	// Abbreviated fussnoten-style Ermächtigung without explicit parent title phrase.
	xmlData := []byte(`<?xml version="1.0"?><dokument><norm><metadaten>
      <fundstelle><periodikum>BGBl. I</periodikum><zit>2025 Nr. 268</zit></fundstelle>
    </metadaten><textdaten><fussnoten><Content><P>aufgrund d. § 11 G v. 11.8.2014 I 1348</P></Content></fussnoten></textdaten></norm></dokument>`)

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 0 {
		t.Fatalf("nLinks=%d want 0 (no false milog via title fallback); discovered=%+v", n, st.discovered)
	}
	if len(st.discovered) != 0 {
		t.Fatalf("unexpected edges: %+v", st.discovered)
	}
}

func TestIngestLawXML_UhAnpV_discoversLAG(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "lag", Abbreviation: "LAG", Title: "Gesetz über den Lastenausgleich", GIIPath: "lag"},
		},
	}
	law := domain.Law{
		ID:           "uhanpv24",
		Abbreviation: "UhAnpV 24",
		Title:        "Vierundzwanzigste Verordnung zur Anpassung der Unterhaltshilfe nach dem Lastenausgleichsgesetz",
		GIIPath:      "uhanpv_24",
	}
	xmlData := fixtures.MustRead("uhanpv_24_snippet.xml")

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1; discovered=%+v", n, st.discovered)
	}

	edges := st.discovered["lag|uhanpv_24"]
	if len(edges) != 1 {
		t.Fatalf("discovered edges=%d want 1; all=%+v", len(edges), st.discovered)
	}
	e := edges[0]
	if e.ParentLawID != "lag" {
		t.Fatalf("ParentLawID=%q want lag", e.ParentLawID)
	}
	if e.SectionHint == "" {
		t.Fatal("SectionHint should not be empty")
	}
	if !strings.Contains(e.SectionHint, "§ 267") {
		t.Fatalf("SectionHint=%q want containing § 267", e.SectionHint)
	}
	if e.Confidence != ConfidenceHigh {
		t.Fatalf("Confidence=%q want high", e.Confidence)
	}
	if e.Notes != "BGBl. 1997 I Nr. 1806" {
		t.Fatalf("Notes=%q want BGBl. 1997 I Nr. 1806", e.Notes)
	}
}

func TestIngestLawXML_PflegeArbbV_discoversAEntG(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "aentg2009", Abbreviation: "AEntG", Title: "Gesetz über zwingende Arbeitsbedingungen für grenzüberschreitende Entsendungen", GIIPath: "aentg2009"},
		},
	}
	law := domain.Law{
		ID:           "pflegearbbv7",
		Abbreviation: "PflegeArbbV 7",
		Title:        "Siebte Verordnung über zwingende Arbeitsbedingungen für die Pflegebranche",
		GIIPath:      "pflegearbbv_7",
	}
	xmlData := fixtures.MustRead("pflegearbbv_7_snippet.xml")

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1; discovered=%+v", n, st.discovered)
	}

	edges := st.discovered["aentg2009|pflegearbbv_7"]
	if len(edges) != 1 {
		t.Fatalf("discovered edges=%d want 1; all=%+v", len(edges), st.discovered)
	}
	e := edges[0]
	if e.ParentLawID != "aentg2009" {
		t.Fatalf("ParentLawID=%q want aentg2009", e.ParentLawID)
	}
	if e.SectionHint == "" {
		t.Fatal("SectionHint should not be empty")
	}
	if !strings.Contains(e.SectionHint, "§ 11") {
		t.Fatalf("SectionHint=%q want containing § 11", e.SectionHint)
	}
}

func TestIngestLawXML_SVBezGrV_MultiParent(t *testing.T) {
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb2", Abbreviation: "SGB_2", Title: "Sozialgesetzbuch (SGB) Zweites Buch (II)"},
			{ID: "sgb4", Abbreviation: "SGB_4", Title: "Sozialgesetzbuch (SGB) Viertes Buch (IV)"},
			{ID: "sgb5", Abbreviation: "SGB_5", Title: "Sozialgesetzbuch (SGB) Fünftes Buch (V)"},
			{ID: "sgb6", Abbreviation: "SGB_6", Title: "Sozialgesetzbuch (SGB) Sechstes Buch (VI)"},
			{ID: "sgb11", Abbreviation: "SGB_11", Title: "Sozialgesetzbuch (SGB) Elftes Buch (XI)"},
		},
		Variants: []domain.LawVariant{
			{Variant: "SGB IV", LawID: "sgb4"},
			{Variant: "SGB V", LawID: "sgb5"},
			{Variant: "SGB VI", LawID: "sgb6"},
			{Variant: "SGB 4", LawID: "sgb4"},
			{Variant: "SGB 5", LawID: "sgb5"},
			{Variant: "SGB 6", LawID: "sgb6"},
		},
	}
	law := domain.Law{
		ID:           "svbezgrv2025",
		Abbreviation: "SVBezGrV 2025",
		Title:        "Verordnung über maßgebende Rechengrößen der Sozialversicherung für 2025",
		GIIPath:      "svbezgrv_2025",
	}
	xmlData := fixtures.MustRead("svbezgrv_2025_preamble.xml")

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 3 {
		t.Fatalf("nLinks=%d want 3; discovered=%+v", n, st.discovered)
	}

	idx, ok := st.bgblIndex["BGBl-1/2024/365"]
	if !ok {
		t.Fatalf("expected BGBl index; got %+v", st.bgblIndex)
	}
	if idx.GIISlug != "svbezgrv_2025" {
		t.Fatalf("index slug=%q", idx.GIISlug)
	}

	want := map[string]string{
		"sgb4|svbezgrv_2025": "§ 17",
		"sgb5|svbezgrv_2025": "§ 6",
		"sgb6|svbezgrv_2025": "§ 69",
	}
	for key, hintPart := range want {
		edges := st.discovered[key]
		if len(edges) != 1 {
			t.Fatalf("key %s edges=%d want 1; all=%+v", key, len(edges), st.discovered)
		}
		e := edges[0]
		if e.Confidence != ConfidenceHigh {
			t.Fatalf("%s Confidence=%q want high", key, e.Confidence)
		}
		if e.ChildLawID != "svbezgrv2025" {
			t.Fatalf("%s ChildLawID=%q", key, e.ChildLawID)
		}
		if !strings.Contains(e.SectionHint, hintPart) {
			t.Fatalf("%s SectionHint=%q want containing %q", key, e.SectionHint, hintPart)
		}
	}
	if !strings.Contains(st.discovered["sgb4|svbezgrv_2025"][0].SectionHint, "§ 18") {
		t.Fatalf("sgb4 hints should include § 18: %q", st.discovered["sgb4|svbezgrv_2025"][0].SectionHint)
	}
}

type memIngestStore struct {
	bgblIndex  map[string]store.BGBlIndexEntry
	discovered map[string][]domain.DiscoveredEdge
}

func newMemIngestStore() *memIngestStore {
	return &memIngestStore{
		bgblIndex:  map[string]store.BGBlIndexEntry{},
		discovered: map[string][]domain.DiscoveredEdge{},
	}
}

func (m *memIngestStore) UpsertBGBlIndex(e store.BGBlIndexEntry) error {
	m.bgblIndex[citation.IssueID(e.Teil, e.Year, e.Number)] = e
	return nil
}

func (m *memIngestStore) UpsertDiscoveredLink(e domain.DiscoveredEdge) error {
	k := normalize.Key(e.ParentLawID) + "|" + e.GIISlug
	m.discovered[k] = []domain.DiscoveredEdge{e}
	return nil
}

func (m *memIngestStore) DeleteDiscoveredBySlug(slug string) error {
	for k, edges := range m.discovered {
		if len(edges) > 0 && edges[0].GIISlug == slug {
			delete(m.discovered, k)
		}
	}
	return nil
}
