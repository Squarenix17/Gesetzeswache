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

func TestRejectUnsafeXML_EntityBlocked(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe "xxe">]><dokumente></dokumente>`)
	if err := rejectUnsafeXML(xmlData); err == nil {
		t.Fatal("expected error for internal ENTITY")
	}
	safe := []byte(`<?xml version="1.0"?><!DOCTYPE dokumente SYSTEM "http://www.gesetze-im-internet.de/dtd/1.01/gii-norm.dtd"><dokumente></dokumente>`)
	if err := rejectUnsafeXML(safe); err != nil {
		t.Fatalf("SYSTEM doctype should be allowed: %v", err)
	}
}

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
