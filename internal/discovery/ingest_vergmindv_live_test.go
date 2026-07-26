package discovery

import (
	"os"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/store"
)

func TestIngestLawXML_VergMindV_realXML_liveDB(t *testing.T) {
	if os.Getenv("GEW_LIVE_INGEST") != "1" {
		t.Skip("set GEW_LIVE_INGEST=1 to run live DB mutation test")
	}
	zipPath := "/tmp/vergmindv_2023.xml.zip"
	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("no /tmp/vergmindv_2023.xml.zip")
	}
	xmlData, err := readFirstZipEntry(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.StorePath = liveTestStorePath(t)
	st, err := store.Open(cfg.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	law := domain.Law{
		ID:           "vergmindv2023",
		Abbreviation: "VergMindV 2023",
		Title:        "Verordnung zur Festsetzung eines vergabespezifischen Mindestentgelts für Aus- und Weiterbildungsdienstleistungen nach dem Zweiten oder Dritten Buch Sozialgesetzbuch für die Kalenderjahre 2023 bis 2026",
		GIIPath:      "vergmindv_2023",
	}
	if _, err := st.UpsertLawIfAbsent(law); err != nil {
		t.Fatal(err)
	}

	laws, err := st.ListLaws()
	if err != nil {
		t.Fatal(err)
	}
	variants, err := st.ListVariants()
	if err != nil {
		t.Fatal(err)
	}
	lookup := CatalogLookup{Laws: laws, Variants: variants}

	n, err := IngestLawXML(st, lookup, law, xmlData)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1", n)
	}
	edges, err := st.DiscoveredForParent("sgb3")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range edges {
		if e.GIISlug == "vergmindv_2023" {
			found = true
			t.Logf("discovered edge: parent=%s slug=%s section=%q notes=%q confidence=%s",
				e.ParentLawID, e.GIISlug, e.SectionHint, e.Notes, e.Confidence)
			if e.SectionHint != "§ 185" {
				t.Fatalf("section hint %q want § 185", e.SectionHint)
			}
		}
	}
	if !found {
		t.Fatalf("no vergmindv_2023 edge under sgb3; edges=%+v", edges)
	}
	edges2, err := st.DiscoveredForParent("sgb2")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edges2 {
		if e.GIISlug == "vergmindv_2023" {
			t.Fatalf("unexpected vergmindv_2023 edge under sgb2: %+v", e)
		}
	}
}
