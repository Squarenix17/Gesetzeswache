package discovery

import (
	"os"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/store"
)

func TestIngestLawXML_PflegeArbbV_realXML_liveDB(t *testing.T) {
	if os.Getenv("GEW_LIVE_INGEST") != "1" {
		t.Skip("set GEW_LIVE_INGEST=1 to run live DB mutation test")
	}
	zipPath := "/tmp/pflegearbbv_7.xml.zip"
	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("no /tmp/pflegearbbv_7.xml.zip")
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
		ID:           "pflegearbbv7",
		Abbreviation: "PflegeArbbV 7",
		Title:        "Siebte Verordnung über zwingende Arbeitsbedingungen für die Pflegebranche",
		GIIPath:      "pflegearbbv_7",
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
	edges, err := st.DiscoveredForParent("aentg2009")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range edges {
		if e.GIISlug == "pflegearbbv_7" {
			found = true
			t.Logf("discovered edge: parent=%s slug=%s section=%q notes=%q confidence=%s",
				e.ParentLawID, e.GIISlug, e.SectionHint, e.Notes, e.Confidence)
		}
	}
	if !found {
		t.Fatalf("no pflegearbbv_7 edge under aentg2009; edges=%+v", edges)
	}
}
