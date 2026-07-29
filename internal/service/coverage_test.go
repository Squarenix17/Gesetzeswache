package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestListStale_returnsStaleRecords(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	now := time.Now().UTC()
	rec := domain.FreshnessRecord{LawID: "arbzg", State: domain.FreshnessConfirmedStale, EvaluatedAt: now}
	if err := svc.Store.PutFreshness(rec); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListStale(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].LawID != "arbzg" {
		t.Fatalf("list=%+v", list)
	}
}

func TestForceRecheck_unknownLaw(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))

	err := svc.ForceRecheck(context.Background(), "no-such-law")
	if err != ErrLawNotFound {
		t.Fatalf("err=%v want ErrLawNotFound", err)
	}
}

func TestForceRecheck_allLaws(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))

	if err := svc.ForceRecheck(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestExportText_disabled(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	svc.CFG.EnableExport = false
	_, err := svc.ExportText(context.Background(), "ArbZG", nil, IncludeOpts{})
	if err == nil || err.Error() != "export disabled" {
		t.Fatalf("err=%v", err)
	}
}

func TestExportText_emptyFormats(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	_, err := svc.ExportText(context.Background(), "ArbZG", []string{}, IncludeOpts{})
	if err == nil || err.Error() != "empty format list" {
		t.Fatalf("err=%v", err)
	}
}

func TestExportText_unknownFormat(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	_, err := svc.ExportText(context.Background(), "ArbZG", []string{"bogus"}, IncludeOpts{})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("err=%v", err)
	}
}

func TestFreshness_lawRequired(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	_, err := svc.Freshness(context.Background(), "", IncludeOpts{})
	if err == nil || err.Error() != "law id required" {
		t.Fatalf("err=%v", err)
	}
}
