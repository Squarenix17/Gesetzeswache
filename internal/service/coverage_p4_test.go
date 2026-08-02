package service

import (
	"context"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestForceRecheck_specificLaw(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/index.html", fixtures.MustRead("arbzg_index_no_stand.html"))

	if _, err := svc.ForceRecheck(context.Background(), "arbzg"); err != nil {
		t.Fatal(err)
	}
}

func TestExportText_cacheHit(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	_ = svc.Store.UpsertStand(stand)
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/bgb/xml.zip", fixtures.MustZipXML("bgb.xml", xmlBody))

	formats := []string{export.FormatHierarchical, export.FormatFlat, export.FormatChunked}
	res1, err := svc.ExportText(context.Background(), "bgb", formats, IncludeOpts{}, ExportGateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := svc.ExportText(context.Background(), "bgb", formats, IncludeOpts{}, ExportGateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res1.Matched || !res2.Matched {
		t.Fatal("expected matched exports")
	}
	if len(res1.Formats) != 3 || len(res2.Formats) != 3 {
		t.Fatalf("formats=%d/%d", len(res1.Formats), len(res2.Formats))
	}
}

func TestResolve_directIDLookup(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	law := domain.Law{ID: "customlaw", Abbreviation: "CL", Title: "Custom", GIIPath: "customlaw"}
	_ = svc.Store.UpsertLaws([]domain.Law{law})
	res, err := svc.Resolve(context.Background(), "customlaw", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Law == nil || res.Law.ID != "customlaw" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestFreshness_resolvesAbbreviation(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	meta, err := svc.Freshness(context.Background(), "ArbZG", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State == "" {
		t.Fatal("expected freshness state")
	}
}

func TestExportText_refuseStale(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	svc.CFG.RefuseExportStale = true
	seedFreshSync(t, svc, time.Now().UTC())
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	_ = svc.Store.UpsertStand(stand)
	issueID := citation.IssueID(1, 2026, "999")
	_ = svc.Store.UpsertIssue(domain.GazetteIssue{ID: issueID, Teil: 1, Year: 2026, Number: "999"})
	_ = svc.Store.UpsertLink(domain.IssueLawLink{IssueID: issueID, LawID: "bgb", Class: domain.LinkConfirmed})
	_, err := svc.ExportText(context.Background(), "bgb", []string{export.FormatHierarchical}, IncludeOpts{}, ExportGateOpts{})
	if err == nil || err.Error() != "export refused: law confirmed_stale" {
		t.Fatalf("err=%v", err)
	}
}

func TestCollectMetrics_withData(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	reg := metrics.NewRegistry()
	metrics.RegisterDefaults(reg)
	svc.Metrics = reg
	svc.CollectMetrics(reg)
}

func TestExportText_arbzg_normtext(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	res, err := svc.ExportText(context.Background(), "arbzg", []string{export.FormatNormtext, export.FormatFlat}, IncludeOpts{}, ExportGateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Formats[export.FormatNormtext] == nil {
		t.Fatalf("res=%+v", res)
	}
}

func TestResolve_noMatch(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	res, err := svc.Resolve(context.Background(), "zzzznonexistentlawquery", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched {
		t.Fatal("expected no match")
	}
}

func TestForceRecheck_byAbbreviation(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	if _, err := svc.ForceRecheck(context.Background(), "ArbZG"); err != nil {
		t.Fatal(err)
	}
}

func TestFreshness_lawNotFound(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	_, err := svc.Freshness(context.Background(), "missing-law-id", IncludeOpts{})
	if err == nil || err.Error() != "law not found" {
		t.Fatalf("err=%v", err)
	}
}
