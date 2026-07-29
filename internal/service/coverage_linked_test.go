package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/discovery"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestLinkedInstruments_familiesAndDiscovered(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat
	fam, err := instruments.LoadFamiliesTSV(filepath.Join("..", "..", "variants", "fortschreibung_families.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Families = fam

	law := domain.Law{ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz", GIIPath: "milog"}
	_ = svc.Store.UpsertLaws([]domain.Law{law})
	laws, _ := svc.Store.ListLaws()
	svc.Search.Swap(laws, nil)
	_ = svc.Store.UpsertDiscoveredLink(domain.DiscoveredEdge{
		ParentLawID: "milog", GIISlug: "milov5", Confidence: discovery.ConfidenceHigh,
	})
	seedFreshSync(t, svc, time.Now().UTC())
	stand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	_ = svc.Store.UpsertStand(stand)

	meta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{Linked: true, Past: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.LinkedInstruments) == 0 {
		t.Fatal("expected linked instruments")
	}
}

func TestForceRecheck_linkedChildRefresh(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat
	law := domain.Law{ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz", GIIPath: "milog"}
	_ = svc.Store.UpsertLaws([]domain.Law{law})
	laws, _ := svc.Store.ListLaws()
	svc.Search.Swap(laws, nil)

	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	mt.SetBytes("www.gesetze-im-internet.de", "/milog/", fixtures.MustRead("arbzg_index_no_stand.html"))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/", fixtures.MustRead("arbzg_index_no_stand.html"))
	xmlBody := fixtures.MustRead("milov5_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip", fixtures.MustZipXML("milov5.xml", xmlBody))

	if err := svc.ForceRecheck(context.Background(), "milog"); err != nil {
		t.Fatal(err)
	}
}

func TestExportText_directLawIDWhenResolveFails(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	law := domain.Law{ID: "onlyid", Abbreviation: "OID", Title: "Only ID Law", GIIPath: "onlyid"}
	_ = svc.Store.UpsertLaws([]domain.Law{law})
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/onlyid/xml.zip", fixtures.MustZipXML("onlyid.xml", xmlBody))

	res, err := svc.ExportText(context.Background(), "onlyid", []string{export.FormatHierarchical}, IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Law == nil || res.Law.ID != "onlyid" {
		t.Fatalf("res=%+v", res)
	}
}

func TestExportText_discoveryOnVerordnung(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	svc.CFG.DiscoveryEnabled = true
	seedFreshSync(t, svc, time.Now().UTC())
	law := domain.Law{
		ID: "milov5", Abbreviation: "MiLoV5", Title: "Fünfte Mindestlohnanpassungsverordnung",
		GIIPath: "milov5",
	}
	_ = svc.Store.UpsertLaws([]domain.Law{law})
	laws, _ := svc.Store.ListLaws()
	svc.Search.Swap(laws, nil)
	xmlBody := fixtures.MustRead("milov5_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip", fixtures.MustZipXML("milov5.xml", xmlBody))

	res, err := svc.ExportText(context.Background(), "milov5", []string{export.FormatNormtext}, IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched {
		t.Fatalf("res=%+v", res)
	}
}

func TestForceRecheck_expiredContext(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	err := svc.ForceRecheck(ctx, "")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestFreshness_bgblPointers(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	_ = svc.Store.UpsertStand(stand)
	issueID := citation.IssueID(1, 2023, "198")
	_ = svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: issueID, Teil: 1, Year: 2023, Number: "198", ELIURL: "https://www.recht.bund.de/eli/bund/bgbl-1/2023/198",
	})
	_ = svc.Store.UpsertLink(domain.IssueLawLink{IssueID: issueID, LawID: "bgb", Class: domain.LinkConfirmed})
	meta, err := svc.Freshness(context.Background(), "bgb", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.BGBlPointers) == 0 {
		t.Fatalf("pointers=%v", meta.BGBlPointers)
	}
}

func TestExportText_unmatchedQuery(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	res, err := svc.ExportText(context.Background(), "zzzznotalawname", []string{export.FormatHierarchical}, IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched {
		t.Fatal("expected no match")
	}
}

func TestResolve_queryRequired(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	_, err := svc.Resolve(context.Background(), "  ", IncludeOpts{})
	if err == nil || err.Error() != "query required" {
		t.Fatalf("err=%v", err)
	}
}

func TestExportText_defaultFormats(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))
	res, err := svc.ExportText(context.Background(), "ArbZG", nil, IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Formats[export.FormatHierarchical] == nil {
		t.Fatalf("res=%+v", res)
	}
}
