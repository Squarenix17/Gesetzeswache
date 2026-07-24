package sync

import (
	"context"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestIntegration_RefreshStandForLaw_discoveryIngest(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/pbav_2025/", fixtures.MustRead("arbzg_index_no_stand.html"))
	xmlBody := fixtures.MustRead("pbav_2025_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/pbav_2025/xml.zip", fixtures.MustZipXML("pbav_2025.xml", xmlBody))

	o := newTestOrchestrator(t, mt)
	o.CFG.DiscoveryEnabled = true
	indexURL, err := giiurl.IndexURL(o.CFG.GIIBase, "pbav_2025")
	if err != nil {
		t.Fatal(err)
	}
	laws := []domain.Law{
		{ID: "sgb11", Abbreviation: "SGB XI", Title: "Elftes Buch Sozialgesetzbuch", GIIPath: "sgb11"},
		{
			ID:           "pbav2025",
			Abbreviation: "PBAV 2025",
			Title:        "Pflegeberufe-Ausbildungs- und Prüfungsverordnung",
			GIIPath:      "pbav_2025",
			GIIURL:       indexURL,
		},
	}
	if err := o.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}

	law := laws[1]
	if err := o.RefreshStandForLaw(context.Background(), law); err != nil {
		t.Fatal(err)
	}

	edges, err := o.Store.DiscoveredForParent("sgb11")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("discovered edges=%d want 1", len(edges))
	}
	if edges[0].GIISlug != "pbav_2025" || edges[0].Confidence != "high" {
		t.Fatalf("edge=%+v unexpected", edges[0])
	}
}

func TestIntegration_DiscoverOrdinances_bounded(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/pbav_2025/", fixtures.MustRead("arbzg_index_no_stand.html"))
	xmlBody := fixtures.MustRead("pbav_2025_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/pbav_2025/xml.zip", fixtures.MustZipXML("pbav_2025.xml", xmlBody))

	o := newTestOrchestrator(t, mt)
	o.CFG.DiscoveryEnabled = true
	o.CFG.DiscoveryMaxPerCycle = 1
	laws := []domain.Law{
		{ID: "sgb11", Abbreviation: "SGB XI", Title: "Elftes Buch Sozialgesetzbuch", GIIPath: "sgb11"},
		{ID: "pbav2025", Abbreviation: "PBAV 2025", Title: "Pflegeberufe-Ausbildungs- und Prüfungsverordnung", GIIPath: "pbav_2025"},
		{ID: "milov5", Abbreviation: "MiLoV5", Title: "Fünfte Mindestlohnanpassungsverordnung", GIIPath: "milov5"},
	}
	if err := o.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}
	_ = o.Store.SetMeta("discovery_queue:pbav2025", "1")

	n, err := o.DiscoverOrdinances(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ingested=%d want 1", n)
	}
	if v, ok, _ := o.Store.GetMeta("discovery_ingested:pbav2025"); !ok || v != "1" {
		t.Fatalf("discovery_ingested meta=%q ok=%v", v, ok)
	}
}
