package sync

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestRunGIIFeed_storeClosedNoSuccessStamp(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	reg := metrics.NewRegistry()
	o := newTestOrchestrator(t, mt)
	o.Metrics = reg
	if err := o.RunTOC(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = o.Store.Close()

	if err := o.RunGIIFeed(context.Background()); err == nil {
		t.Fatal("expected store write error")
	}
	if _, ok, _ := o.Store.GetMetaTime("last_gii_feed_success"); ok {
		t.Fatal("must not stamp success on store failure")
	}
	if reg.CounterValue(metrics.MetricSyncStoreWriteFailuresTotal, map[string]string{"source": "gii_feed"}) != 1 {
		t.Fatal("expected store write failure metric")
	}
}

func TestRunBGBlFeeds_oneFeedFailsNoStamp(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", httpmock.Response{
		Status: 500,
		Body:   []byte("error"),
	})
	o := newTestOrchestrator(t, mt)

	if err := o.RunBGBlFeeds(context.Background()); err == nil {
		t.Fatal("expected error when one feed fails")
	}
	if _, ok, _ := o.Store.GetMetaTime("last_bgbl_feed_success"); ok {
		t.Fatal("must not stamp success when a feed fails")
	}
	if _, ok, _ := o.Store.GetMetaTime(metaKeyBGBlFeedDegraded); !ok {
		t.Fatal("expected degraded marker on partial feed failure")
	}
}

func TestRunELIProbe_allHTTPFailNoStamp(t *testing.T) {
	mt := httpmock.New()
	year := time.Now().UTC().Year()
	for _, n := range []int{1, 2, 3, 4} {
		for _, teil := range []int{1, 2} {
			path := fmt.Sprintf("/eli/bund/BGBl-%d/%d/%d", teil, year, n)
			mt.Set("www.recht.bund.de", path, httpmock.Response{Err: fmt.Errorf("timeout")})
		}
	}
	o := newTestOrchestrator(t, mt)
	if err := o.Store.UpsertIssue(domain.GazetteIssue{
		ID: fmt.Sprintf("BGBl-1/%d/1", year), Teil: 1, Year: year, Number: "1",
	}); err != nil {
		t.Fatal(err)
	}

	if err := o.RunELIProbe(context.Background()); err == nil {
		t.Fatal("expected error when all probe HTTP requests fail")
	}
	if _, ok, _ := o.Store.GetMetaTime("last_eli_probe_success"); ok {
		t.Fatal("must not stamp success when all probes fail")
	}
}

func TestRunELIProbe_zeroHitsStillStampsOnHTTPSuccess(t *testing.T) {
	mt := httpmock.New()
	year := time.Now().UTC().Year()
	o := newTestOrchestrator(t, mt)
	if err := o.Store.UpsertIssue(domain.GazetteIssue{
		ID: fmt.Sprintf("BGBl-1/%d/99", year), Teil: 1, Year: year, Number: "99",
	}); err != nil {
		t.Fatal(err)
	}
	// All probe URLs return 404 (HTTP ok, not found) — legitimate zero hits.
	for n := 100; n <= 103; n++ {
		for _, teil := range []int{1, 2} {
			path := fmt.Sprintf("/eli/bund/BGBl-%d/%d/%d", teil, year, n)
			mt.Set("www.recht.bund.de", path, httpmock.Response{Status: 404, Body: []byte("not found")})
		}
	}

	if err := o.RunELIProbe(context.Background()); err != nil {
		t.Fatalf("probe with HTTP success but zero hits should succeed: %v", err)
	}
	if _, ok, _ := o.Store.GetMetaTime("last_eli_probe_success"); !ok {
		t.Fatal("expected last_eli_probe_success when HTTP succeeded")
	}
}

func TestRefreshStandForLaw_htmlAndXMLFail(t *testing.T) {
	mt := httpmock.New()
	mt.Set("www.gesetze-im-internet.de", "/arbzg/", httpmock.Response{Status: 500, Body: []byte("error")})
	mt.Set("www.gesetze-im-internet.de", "/arbzg/xml.zip", httpmock.Response{Status: 404, Body: []byte("missing")})
	reg := metrics.NewRegistry()
	o := newTestOrchestrator(t, mt)
	o.Metrics = reg
	law := domain.Law{ID: "arbzg", GIIPath: "arbzg"}

	if err := o.RefreshStandForLaw(context.Background(), law); err == nil {
		t.Fatal("expected error when html and xml both fail")
	}
	if reg.CounterValue(metrics.MetricStandRefreshFailuresTotal, nil) != 0 {
		t.Fatal("metric incremented by caller, not RefreshStandForLaw itself")
	}
}

func TestRefreshStandForLaw_htmlOKNoStandNoError(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/", fixtures.MustRead("arbzg_index_no_stand.html"))
	o := newTestOrchestrator(t, mt)
	law := domain.Law{ID: "arbzg", GIIPath: "arbzg"}

	if err := o.RefreshStandForLaw(context.Background(), law); err != nil {
		t.Fatalf("html ok without stand marker is not an error: %v", err)
	}
}

func TestRefreshMissingStands_incrementsStandFailureMetric(t *testing.T) {
	mt := httpmock.New()
	mt.Set("www.gesetze-im-internet.de", "/arbzg/", httpmock.Response{Status: 500, Body: []byte("error")})
	mt.Set("www.gesetze-im-internet.de", "/arbzg/xml.zip", httpmock.Response{Status: 404, Body: []byte("missing")})
	reg := metrics.NewRegistry()
	o := newTestOrchestrator(t, mt)
	o.Metrics = reg
	if err := o.Store.UpsertLaws([]domain.Law{{ID: "arbzg", GIIPath: "arbzg"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := o.RefreshMissingStands(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if reg.CounterValue(metrics.MetricStandRefreshFailuresTotal, nil) != 1 {
		t.Fatal("expected stand refresh failure metric")
	}
}

func TestDiscoverOrdinances_ingestFailureNoIngestedFlag(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/", fixtures.MustRead("arbzg_index_no_stand.html"))
	mt.Set("www.gesetze-im-internet.de", "/milov5/xml.zip", httpmock.Response{Status: 404, Body: []byte("missing")})
	reg := metrics.NewRegistry()
	o := newTestOrchestrator(t, mt)
	o.CFG.DiscoveryEnabled = true
	o.Metrics = reg
	if err := o.Store.UpsertLaws([]domain.Law{{
		ID: "milov5", Abbreviation: "MiLoV5", Title: "Test", GIIPath: "milov5",
	}}); err != nil {
		t.Fatal(err)
	}
	_ = o.Store.SetMeta("discovery_queue:milov5", "1")

	if n, err := o.DiscoverOrdinances(context.Background(), 1); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatalf("ingested=%d want 0 on failure", n)
	}
	if v, ok, _ := o.Store.GetMeta("discovery_ingested:milov5"); ok && v == "1" {
		t.Fatal("discovery_ingested must not be set on ingest failure")
	}
	if reg.CounterValue(metrics.MetricDiscoveryIngestTotal, map[string]string{"result": "error"}) != 1 {
		t.Fatal("expected discovery ingest error metric")
	}
}

func TestStampSuccessMeta_doesNotMoveBackwards(t *testing.T) {
	o := newTestOrchestrator(t, httpmock.New())
	o.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	newer := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Hour)
	if err := o.Store.SetMetaTime("last_toc_success", newer); err != nil {
		t.Fatal(err)
	}
	if err := o.stampSuccessMeta("last_toc_success", older); err != nil {
		t.Fatal(err)
	}
	got, ok, err := o.Store.GetMetaTime("last_toc_success")
	if err != nil || !ok || !got.Equal(newer) {
		t.Fatalf("stored=%v ok=%v want newer preserved", got, ok)
	}
}

func TestDiscoverOrdinances_successSetsIngestedFlag(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/pbav_2025/", fixtures.MustRead("arbzg_index_no_stand.html"))
	xmlBody := fixtures.MustRead("pbav_2025_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/pbav_2025/xml.zip", fixtures.MustZipXML("pbav_2025.xml", xmlBody))
	reg := metrics.NewRegistry()
	o := newTestOrchestrator(t, mt)
	o.CFG.DiscoveryEnabled = true
	o.Metrics = reg
	indexURL, _ := giiurl.IndexURL(o.CFG.GIIBase, "pbav_2025")
	laws := []domain.Law{
		{ID: "sgb11", Abbreviation: "SGB XI", Title: "SGB XI", GIIPath: "sgb11"},
		{ID: "pbav2025", Abbreviation: "PBAV", Title: "PBAV", GIIPath: "pbav_2025", GIIURL: indexURL},
	}
	if err := o.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}
	_ = o.Store.SetMeta("discovery_queue:pbav2025", "1")

	n, err := o.DiscoverOrdinances(context.Background(), 1)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if v, ok, _ := o.Store.GetMeta("discovery_ingested:pbav2025"); !ok || v != "1" {
		t.Fatalf("discovery_ingested=%q ok=%v", v, ok)
	}
	if reg.CounterValue(metrics.MetricDiscoveryIngestTotal, map[string]string{"result": "success"}) != 1 {
		t.Fatal("expected discovery ingest success metric")
	}
}
