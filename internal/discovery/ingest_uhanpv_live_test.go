package discovery

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/store"
)

func TestIngestLawXML_UhAnpV_realXML_liveDB(t *testing.T) {
	if os.Getenv("GEW_LIVE_INGEST") != "1" {
		t.Skip("set GEW_LIVE_INGEST=1 to run live DB mutation test")
	}
	zipPath := "/tmp/uhanpv_24.xml.zip"
	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("no /tmp/uhanpv_24.xml.zip")
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
		ID:           "uhanpv24",
		Abbreviation: "UhAnpV 24",
		Title:        "Vierundzwanzigste Verordnung zur Anpassung der Unterhaltshilfe nach dem Lastenausgleichsgesetz",
		GIIPath:      "uhanpv_24",
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
	edges, err := st.DiscoveredForParent("lag")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range edges {
		if e.GIISlug == "uhanpv_24" {
			found = true
			t.Logf("discovered edge: parent=%s slug=%s section=%q notes=%q confidence=%s",
				e.ParentLawID, e.GIISlug, e.SectionHint, e.Notes, e.Confidence)
		}
	}
	if !found {
		t.Fatalf("no uhanpv_24 edge under lag; edges=%+v", edges)
	}
}

// liveTestStorePath opens the repo-root gesetzeswache.db during go test.
// go test runs with cwd = this package dir, so GEW_STORE_PATH=gesetzeswache.db
// would otherwise create/use internal/discovery/gesetzeswache.db (wrong catalog).
func liveTestStorePath(t *testing.T) string {
	t.Helper()
	repoDB := filepath.Join("..", "..", "gesetzeswache.db")
	if _, err := os.Stat(repoDB); err != nil {
		t.Skip("no gesetzeswache.db at repo root")
	}
	if p := os.Getenv("GEW_STORE_PATH"); p != "" {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join("..", "..", filepath.Clean(p))
	}
	abs, err := filepath.Abs(repoDB)
	if err != nil {
		return repoDB
	}
	return abs
}

func readFirstZipEntry(path string) ([]byte, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, os.ErrNotExist
}
