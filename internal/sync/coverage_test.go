package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestMergeSources_dedupes(t *testing.T) {
	got := mergeSources([]string{"a", "b"}, "a")
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	added := mergeSources([]string{"a"}, "b")
	if len(added) != 2 || added[1] != "b" {
		t.Fatalf("got %v", added)
	}
}

func TestMatchLawFromItem_byAbbreviation(t *testing.T) {
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz", GIIPath: "arbzg"}
	eng := search.NewEngine()
	eng.Swap([]domain.Law{law}, nil)
	snap := eng.Current()
	id := matchLawFromItem(rssItem{Title: "Änderung (ArbZG)", Description: ""}, snap)
	if id != "arbzg" {
		t.Fatalf("id=%q", id)
	}
}

func TestMatchLawFromItem_nilSnapshot(t *testing.T) {
	if id := matchLawFromItem(rssItem{Title: "ArbZG"}, nil); id != "" {
		t.Fatalf("id=%q", id)
	}
}

func TestHeuristicLink_createsLinkAfterGrace(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz", GIIPath: "arbzg"}
	_ = o.Store.UpsertLaws([]domain.Law{law})
	o.Search.Swap([]domain.Law{law}, nil)
	old := time.Now().UTC().Add(-100 * time.Hour)
	iss := domain.GazetteIssue{
		ID: "iss-1", Title: "Arbeitszeitgesetz", Teil: 1, Year: 2024, Number: "1",
		FirstSeenAt: old, Matched: false,
	}
	_ = o.Store.UpsertIssue(iss)
	o.heuristicLink(time.Now().UTC())
	links, err := o.Store.LinksForLaw("arbzg")
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%v err=%v", links, err)
	}
	if links[0].Class != domain.LinkHeuristic {
		t.Fatalf("class=%s", links[0].Class)
	}
}

func TestInitialSync_runsFeeds(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	o := newTestOrchestrator(t, mt)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o.InitialSync(ctx)
	toc, ok, _ := o.Store.GetMetaTime("last_toc_success")
	if !ok || toc.IsZero() {
		t.Fatal("expected toc success meta")
	}
}

func TestStartBackground_waitOnCancel(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	o.CFG.TOCInterval = time.Hour
	o.CFG.GIIFeedInterval = time.Hour
	o.CFG.BGBlFeedInterval = time.Hour
	o.CFG.ELIProbeInterval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	o.StartBackground(ctx)
	cancel()
	done := make(chan struct{})
	go func() {
		o.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("background loops did not exit")
	}
}

func TestParseRSSDate_formats(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Mon, 02 Jan 2006 15:04:05 MST", true},
		{"not-a-date", false},
	}
	for _, tc := range cases {
		got := parseRSSDate(tc.in)
		if tc.want && got == nil {
			t.Fatalf("want parse for %q", tc.in)
		}
		if !tc.want && got != nil {
			t.Fatalf("want nil for %q", tc.in)
		}
	}
}

func TestTeilFromToken(t *testing.T) {
	if teilFromToken("I") != 1 {
		t.Fatal("I")
	}
	if teilFromToken("II") != 2 {
		t.Fatal("II")
	}
	if teilFromToken("unknown") != 0 {
		t.Fatal("unknown")
	}
}

func TestMatchLawFromItem_byTitle(t *testing.T) {
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz", GIIPath: "arbzg"}
	eng := search.NewEngine()
	eng.Swap([]domain.Law{law}, nil)
	id := matchLawFromItem(rssItem{Title: "Arbeitszeitgesetz"}, eng.Current())
	if id != "arbzg" {
		t.Fatalf("id=%q", id)
	}
}

func TestReconcile_updatesFreshness(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	_ = o.RunTOC(context.Background())
	now := time.Now().UTC()
	_ = o.Store.SetMetaTime("last_toc_success", now)
	_ = o.Store.SetMetaTime("last_gii_feed_success", now)
	_ = o.Store.SetMetaTime("last_bgbl_feed_success", now)
	if err := o.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, ok, _ := o.Store.GetMetaTime("last_reconcile_at")
	if !ok || rec.IsZero() {
		t.Fatal("expected reconcile timestamp")
	}
}

func TestRunGIIFeed_ingestsItems(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	o := newTestOrchestrator(t, mt)
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	_ = o.RunTOC(context.Background())
	if err := o.RunGIIFeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	issues, err := o.Store.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issues from gii feed")
	}
}

func TestRunBGBlFeeds_success(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	o := newTestOrchestrator(t, mt)
	if err := o.RunBGBlFeeds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := o.Store.GetMetaTime("last_bgbl_feed_success"); !ok {
		t.Fatal("expected bgbl meta")
	}
}

func TestInitialSync_withDiscovery(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	o := newTestOrchestrator(t, mt)
	o.CFG.DiscoveryEnabled = true
	o.CFG.StandRefreshMax = 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	o.InitialSync(ctx)
}

func TestRunELIProbe_success(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	year := time.Now().UTC().Year()
	_ = o.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, year, "1"), Teil: 1, Year: year, Number: "1",
	})
	path := fmt.Sprintf("/eli/bund/BGBl-1/%d/1", year)
	mt.SetBytes("www.recht.bund.de", path, []byte("ok"))
	if err := o.RunELIProbe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := o.Store.GetMetaTime("last_eli_probe_success"); !ok {
		t.Fatal("expected eli probe meta")
	}
}

func TestFetchLawXML_rawXML(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	law := domain.Law{ID: "arbzg", GIIPath: "arbzg"}
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", xmlBody)
	data, err := o.fetchLawXML(context.Background(), law)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected xml data")
	}
}

func TestRefreshMissingStands_fillsStand(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	o := newTestOrchestrator(t, mt)
	_ = o.RunTOC(context.Background())
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/", fixtures.MustRead("arbzg_index_with_stand.html"))
	n, err := o.RefreshMissingStands(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("filled=%d want >=1", n)
	}
}

func TestRecordStoreWriteFailure_incrementsMetric(t *testing.T) {
	mt := httpmock.New()
	reg := metrics.NewRegistry()
	o := newTestOrchestrator(t, mt)
	o.Metrics = reg
	o.recordStoreWriteFailure("toc")
	if reg.CounterValue(metrics.MetricSyncStoreWriteFailuresTotal, map[string]string{"source": "toc"}) != 1 {
		t.Fatal("expected metric increment")
	}
}

func TestReconcile_repairsStandAndWritesFreshness(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	o.Instruments = cat
	fam, err := instruments.LoadFamiliesTSV(filepath.Join("..", "..", "variants", "fortschreibung_families.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	o.Families = fam
	now := time.Now().UTC()
	_ = o.Store.SetMetaTime("last_toc_success", now)
	_ = o.Store.SetMetaTime("last_gii_feed_success", now)
	_ = o.Store.SetMetaTime("last_bgbl_feed_success", now)
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz", GIIPath: "arbzg"}
	_ = o.Store.UpsertLaws([]domain.Law{law})
	stand := domain.StandCitation{
		LawID: "arbzg", Raw: "Zuletzt geändert durch Art. 1 G v. 20.7.2022 BGBl. I S. 1170", ParseOK: false,
	}
	_ = o.Store.UpsertStand(stand)
	issueID := citation.IssueID(1, 2022, "1170")
	_ = o.Store.UpsertIssue(domain.GazetteIssue{ID: issueID, Teil: 1, Year: 2022, Number: "1170"})
	_ = o.Store.UpsertLink(domain.IssueLawLink{IssueID: issueID, LawID: "arbzg", Class: domain.LinkConfirmed})
	if err := o.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, ok, _ := o.Store.GetFreshness("arbzg")
	if !ok {
		t.Fatal("expected freshness record")
	}
	if rec.State == "" {
		t.Fatalf("state empty: %+v", rec)
	}
}

func TestRefreshStandForLaw(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", fixtures.MustRead("gii_toc.xml"))
	_ = o.RunTOC(context.Background())
	law, ok, _ := o.Store.GetLaw("arbzg")
	if !ok {
		t.Fatal("arbzg missing")
	}
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/", fixtures.MustRead("arbzg_index_with_stand.html"))
	if err := o.RefreshStandForLaw(context.Background(), law); err != nil {
		t.Fatal(err)
	}
	stand, ok, _ := o.Store.GetStand("arbzg")
	if !ok || stand.Raw == "" {
		t.Fatalf("stand=%+v ok=%v", stand, ok)
	}
}

func TestLatestNumber_parsesDigits(t *testing.T) {
	st := openTestStore(t)
	now := time.Now().UTC()
	issues := []domain.GazetteIssue{
		{ID: "a", Teil: 1, Year: 2024, Number: "12a", FirstSeenAt: now},
		{ID: "b", Teil: 1, Year: 2024, Number: "9", FirstSeenAt: now},
	}
	for _, iss := range issues {
		_ = st.UpsertIssue(iss)
	}
	n := latestNumber(st, 1, 2024)
	if n != 12 {
		t.Fatalf("n=%d want 12", n)
	}
}
