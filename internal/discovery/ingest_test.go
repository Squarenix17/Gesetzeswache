package discovery

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
)

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
