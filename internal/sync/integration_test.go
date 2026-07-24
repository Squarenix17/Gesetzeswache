package sync

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func testCFG() config.Config {
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
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newTestOrchestrator(t *testing.T, mt *httpmock.Transport) *Orchestrator {
	t.Helper()
	cfg := testCFG()
	st := openTestStore(t)
	httpClient := httpx.NewWithTransport(cfg.HTTPTimeout, cfg.RequestMinGap, 1<<20, mt)
	eng := search.NewEngine()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Orchestrator{
		CFG:    cfg,
		Store:  st,
		HTTP:   httpClient,
		Search: eng,
		Log:    log,
	}
}

func TestIntegration_RunTOC_catalogReady(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))

	o := newTestOrchestrator(t, mt)
	ctx := context.Background()

	if err := o.RunTOC(ctx); err != nil {
		t.Fatalf("RunTOC: %v", err)
	}

	for _, id := range []string{"arbzg", "bgb"} {
		if _, ok, err := o.Store.GetLaw(id); err != nil {
			t.Fatalf("GetLaw(%s): %v", id, err)
		} else if !ok {
			t.Fatalf("law %s missing after TOC sync", id)
		}
	}

	if _, ok, err := o.Store.GetMetaTime("last_toc_success"); err != nil {
		t.Fatalf("GetMetaTime: %v", err)
	} else if !ok {
		t.Fatal("last_toc_success not set")
	}
}

func TestIntegration_RunBGBlFeeds_success(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))

	o := newTestOrchestrator(t, mt)
	ctx := context.Background()

	if err := o.RunBGBlFeeds(ctx); err != nil {
		t.Fatalf("RunBGBlFeeds: %v", err)
	}

	if _, ok, err := o.Store.GetMetaTime("last_bgbl_feed_success"); err != nil {
		t.Fatalf("GetMetaTime: %v", err)
	} else if !ok {
		t.Fatal("last_bgbl_feed_success not set")
	}

	issues, err := o.Store.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issues from BGBl feeds")
	}
}

func TestIntegration_RunBGBlFeeds_allFail_thenELIProbe(t *testing.T) {
	tlsErr := fmt.Errorf("tls: handshake timeout")

	mt := httpmock.New()
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", httpmock.Response{Err: tlsErr})
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", httpmock.Response{Err: tlsErr})

	year := time.Now().UTC().Year()
	eliHit := fmt.Sprintf("/eli/bund/BGBl-1/%d/1", year)
	mt.SetBytes("www.recht.bund.de", eliHit, []byte("ok"))

	o := newTestOrchestrator(t, mt)
	ctx := context.Background()

	if err := o.RunBGBlFeeds(ctx); err == nil {
		t.Fatal("expected RunBGBlFeeds error when feeds fail")
	}

	if err := o.RunELIProbe(ctx); err != nil {
		t.Fatalf("RunELIProbe: %v", err)
	}

	if _, ok, err := o.Store.GetMetaTime("last_eli_probe_success"); err != nil {
		t.Fatalf("GetMetaTime: %v", err)
	} else if !ok {
		t.Fatal("last_eli_probe_success not set")
	}

	issues, err := o.Store.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	found := false
	wantID := fmt.Sprintf("BGBl-1/%d/1", year)
	for _, iss := range issues {
		if iss.ID == wantID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ELI-probed issue %s, got %d issues", wantID, len(issues))
	}
}

func TestIntegration_RefreshStandForLaw_html(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/", fixtures.MustRead("arbzg_index_with_stand.html"))

	o := newTestOrchestrator(t, mt)
	indexURL, err := giiurl.IndexURL(o.CFG.GIIBase, "arbzg")
	if err != nil {
		t.Fatal(err)
	}
	law := domain.Law{
		ID:           "arbzg",
		Abbreviation: "ArbZG",
		Title:        "Arbeitszeitgesetz",
		GIIPath:      "arbzg",
		GIIURL:       indexURL,
	}
	if err := o.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := o.RefreshStandForLaw(ctx, law); err != nil {
		t.Fatalf("RefreshStandForLaw: %v", err)
	}

	stand, ok, err := o.Store.GetStand("arbzg")
	if err != nil {
		t.Fatalf("GetStand: %v", err)
	}
	if !ok || stand.Raw == "" {
		t.Fatalf("expected non-empty stand, ok=%v raw=%q", ok, stand.Raw)
	}
}

func TestIntegration_RefreshStandForLaw_noStandHTML(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/", fixtures.MustRead("arbzg_index_no_stand.html"))
	// No xml.zip mock — XML fallback fails quietly; Stand stays empty.

	o := newTestOrchestrator(t, mt)
	indexURL, err := giiurl.IndexURL(o.CFG.GIIBase, "arbzg")
	if err != nil {
		t.Fatal(err)
	}
	law := domain.Law{
		ID:           "arbzg",
		Abbreviation: "ArbZG",
		Title:        "Arbeitszeitgesetz",
		GIIPath:      "arbzg",
		GIIURL:       indexURL,
	}
	if err := o.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := o.RefreshStandForLaw(ctx, law); err != nil {
		t.Fatalf("RefreshStandForLaw: %v", err)
	}

	stand, ok, err := o.Store.GetStand("arbzg")
	if err != nil {
		t.Fatalf("GetStand: %v", err)
	}
	if ok && stand.Raw != "" {
		t.Fatalf("expected missing or empty stand, ok=%v raw=%q", ok, stand.Raw)
	}
}

func TestIntegration_RefreshStandForLaw_xmlFallback(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/", fixtures.MustRead("arbzg_index_no_stand.html"))
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	o := newTestOrchestrator(t, mt)
	indexURL, err := giiurl.IndexURL(o.CFG.GIIBase, "arbzg")
	if err != nil {
		t.Fatal(err)
	}
	law := domain.Law{
		ID:           "arbzg",
		Abbreviation: "ArbZG",
		Title:        "Arbeitszeitgesetz",
		GIIPath:      "arbzg",
		GIIURL:       indexURL,
	}
	if err := o.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}

	if err := o.RefreshStandForLaw(context.Background(), law); err != nil {
		t.Fatal(err)
	}
	stand, ok, err := o.Store.GetStand("arbzg")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || stand.Raw == "" {
		t.Fatalf("expected stand from XML, ok=%v raw=%q", ok, stand.Raw)
	}
	if !strings.Contains(stand.Raw, "BGBl") {
		t.Fatalf("stand raw unexpected: %q", stand.Raw)
	}
}

func TestIntegration_RefreshMissingStands_bounded(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/", fixtures.MustRead("arbzg_index_no_stand.html"))
	mt.SetBytes("www.gesetze-im-internet.de", "/bgb/", fixtures.MustRead("arbzg_index_no_stand.html"))
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))
	mt.SetBytes("www.gesetze-im-internet.de", "/bgb/xml.zip", fixtures.MustZipXML("bgb.xml", xmlBody))

	o := newTestOrchestrator(t, mt)
	laws := []domain.Law{
		{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz", GIIPath: "arbzg"},
		{ID: "bgb", Abbreviation: "BGB", Title: "Bürgerliches Gesetzbuch", GIIPath: "bgb"},
	}
	if err := o.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}

	n, err := o.RefreshMissingStands(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("filled=%d want 1", n)
	}
	filled := 0
	for _, id := range []string{"arbzg", "bgb"} {
		if st, ok, _ := o.Store.GetStand(id); ok && st.Raw != "" {
			filled++
		}
	}
	if filled != 1 {
		t.Fatalf("stored stands=%d want 1", filled)
	}
}
