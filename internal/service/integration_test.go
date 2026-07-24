package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/sync"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func testCFG(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		HTTPAddr:         ":0",
		StorePath:        "test.db",
		MatchThreshold:   0.75,
		FreshnessMaxAge:  6 * time.Hour,
		TOCInterval:      6 * time.Hour,
		GIIFeedInterval:  15 * time.Minute,
		BGBlFeedInterval: 15 * time.Minute,
		ELIProbeInterval: 30 * time.Minute,
		UnmatchedGrace:   72 * time.Hour,
		EnableHeuristic:  true,
		EnableExport:     true,
		ExportCacheMax:   8,
		HTTPTimeout:      5 * time.Second,
		RequestMinGap:    time.Millisecond,
		GIIBase:          "https://www.gesetze-im-internet.de",
		GIITOCURL:        "https://www.gesetze-im-internet.de/gii-toc.xml",
		GIIFeedURL:       "https://www.gesetze-im-internet.de/aktuDienst-rss-feed.xml",
		BGBlFeed1URL:     "https://www.recht.bund.de/rss/feeds/rss_bgbl-1.xml",
		BGBlFeed2URL:     "https://www.recht.bund.de/rss/feeds/rss_bgbl-2.xml",
		ELIBase:          "https://www.recht.bund.de/eli/bund",
		VariantsPath:     "variants/variants.tsv",
		StandRefreshMax:  10,
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestService(t *testing.T, mt *httpmock.Transport) *Service {
	t.Helper()
	cfg := testCFG(t)
	st := openTestStore(t)
	eng := search.NewEngine()
	httpClient := httpx.NewWithTransport(cfg.HTTPTimeout, cfg.RequestMinGap, 1<<20, mt)
	orch := &sync.Orchestrator{CFG: cfg, Store: st, HTTP: httpClient, Search: eng, Log: discardLog()}
	return &Service{
		CFG:    cfg,
		Store:  st,
		Search: eng,
		Sync:   orch,
		HTTP:   httpClient,
		Export: export.NewCache(8),
		Log:    discardLog(),
	}
}

func seedCatalog(t *testing.T, svc *Service, mt *httpmock.Transport) {
	t.Helper()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	if err := svc.Sync.RunTOC(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_CatalogNotReady(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	_, err := svc.Resolve(context.Background(), "ArbZG")
	if err == nil || err.Error() != "catalog not ready" {
		t.Fatalf("want catalog not ready, got %v", err)
	}
	st, err := svc.SyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.CatalogReady || st.DataFresh {
		t.Fatalf("catalogReady=%v dataFresh=%v", st.CatalogReady, st.DataFresh)
	}
}

func TestIntegration_DataFresh_trueAndFalse(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	st, err := svc.SyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.CatalogReady {
		t.Fatal("expected catalog ready after TOC")
	}
	if st.DataFresh {
		t.Fatal("data_fresh should be false without recent BGBl/ELI")
	}

	now := time.Now().UTC()
	if err := svc.Store.SetMetaTime("last_bgbl_feed_success", now); err != nil {
		t.Fatal(err)
	}
	st, err = svc.SyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.DataFresh {
		t.Fatal("expected data_fresh true when TOC+BGBl within max age")
	}

	old := now.Add(-7 * time.Hour)
	if err := svc.Store.SetMetaTime("last_toc_success", old); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SetMetaTime("last_bgbl_feed_success", old); err != nil {
		t.Fatal(err)
	}
	st, err = svc.SyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.DataFresh {
		t.Fatal("expected data_fresh false when timestamps older than max age")
	}
}

func TestIntegration_ExportNormtext(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	res, err := svc.ExportText(context.Background(), "ArbZG", []string{export.FormatNormtext})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Law == nil || res.Freshness == nil {
		t.Fatalf("matched=%v law=%v freshness=%v", res.Matched, res.Law, res.Freshness)
	}
	chunks, ok := res.Formats[export.FormatNormtext].([]export.Chunk)
	if !ok {
		t.Fatalf("normtext type %T", res.Formats[export.FormatNormtext])
	}
	if len(chunks) == 0 {
		t.Fatal("expected normtext chunks")
	}
	for _, c := range chunks {
		if c.Kind != export.KindNormtext {
			t.Fatalf("unexpected kind %q", c.Kind)
		}
		if c.StandRaw == "" {
			t.Fatal("expected stand_raw populated from XML standangabe")
		}
	}
}

func TestIntegration_ExportMalformedXML(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	bad := fixtures.MustRead("malformed.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("bad.xml", bad))

	_, err := svc.ExportText(context.Background(), "ArbZG", []string{export.FormatNormtext})
	if err == nil {
		t.Fatal("expected malformed XML error")
	}
}

func TestIntegration_BGBlFail_ELIFallback_setsProbeMeta(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", httpmock.Response{Err: context.DeadlineExceeded})
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", httpmock.Response{Err: context.DeadlineExceeded})

	err := svc.Sync.RunBGBlFeeds(context.Background())
	if err == nil {
		t.Fatal("expected bgbl feed failure")
	}

	year := time.Now().UTC().Year()
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID:     citation.IssueID(1, year, "1"),
		Teil:   1,
		Year:   year,
		Number: "1",
	}); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/eli/bund/BGBl-1/%d/1", year)
	mt.SetBytes("www.recht.bund.de", path, []byte("ok"))

	if err := svc.Sync.RunELIProbe(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, ok, err := svc.Store.GetMetaTime("last_eli_probe_success")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected last_eli_probe_success")
	}
	wantHit := "www.recht.bund.de|" + path
	foundHit := false
	for _, h := range mt.Hits() {
		if h == wantHit {
			foundHit = true
			break
		}
	}
	if !foundHit {
		t.Fatalf("expected ELI probe hit %s, hits=%v", wantHit, mt.Hits())
	}

	st, err := svc.SyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.DataFresh {
		t.Fatal("data_fresh should be true via ELI probe within max age")
	}
}
