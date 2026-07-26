package discovery

import (
	"os"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestIngestLawXML_WoGV_realXML(t *testing.T) {
	data, err := os.ReadFile("/tmp/wogv.xml")
	if err != nil {
		t.Skip("no /tmp/wogv.xml")
	}
	st := newMemIngestStore()
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "wogg", Abbreviation: "WoGG", Title: "Wohngeldgesetz", GIIPath: "wogg"},
			{ID: "wogv", Abbreviation: "WoGV", Title: "Wohngeldverordnung", GIIPath: "wogv"},
		},
	}
	law := domain.Law{
		ID: "wogv", Abbreviation: "WoGV", Title: "Wohngeldverordnung", GIIPath: "wogv",
	}
	n, err := IngestLawXML(st, lookup, law, data)
	if err != nil {
		t.Fatalf("IngestLawXML: %v", err)
	}
	if n != 1 {
		t.Fatalf("nLinks=%d want 1; discovered=%+v", n, st.discovered)
	}
	edges := st.discovered["wogg|wogv"]
	if len(edges) != 1 || edges[0].SectionHint != "§ 36" {
		t.Fatalf("unexpected edges: %+v", st.discovered)
	}
}
