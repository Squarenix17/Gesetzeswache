package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/discovery"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
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

// seedSyncFreshMeta stamps all feed success timestamps for happy-path freshness (P1: includes GII).
func seedSyncFreshMeta(t *testing.T, svc *Service, now time.Time) {
	t.Helper()
	if err := svc.Store.SetMetaTime("last_toc_success", now); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SetMetaTime("last_gii_feed_success", now); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SetMetaTime("last_bgbl_feed_success", now); err != nil {
		t.Fatal(err)
	}
}

// seedProbeOnlyBGBlMeta keeps TOC/GII fresh but supplies BGBl evidence only via ELI probe.
func seedProbeOnlyBGBlMeta(t *testing.T, svc *Service, now time.Time) {
	t.Helper()
	if err := svc.Store.SetMetaTime("last_toc_success", now); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SetMetaTime("last_gii_feed_success", now); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SetMetaTime("last_eli_probe_success", now); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_CatalogNotReady(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	_, err := svc.Resolve(context.Background(), "ArbZG", IncludeOpts{})
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
	seedSyncFreshMeta(t, svc, now)
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
	if err := svc.Store.SetMetaTime("last_gii_feed_success", old); err != nil {
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

	res, err := svc.ExportText(context.Background(), "ArbZG", []string{export.FormatNormtext}, IncludeOpts{}, ExportGateOpts{})
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

	_, err := svc.ExportText(context.Background(), "ArbZG", []string{export.FormatNormtext}, IncludeOpts{}, ExportGateOpts{})
	if err == nil {
		t.Fatal("expected malformed XML error")
	}
}

func TestIntegration_MiLoG_seedNotes_withoutExport_notConfirmedCurrent(t *testing.T) {
	// Seed TSV notes cite Nr. 268; no export/editorial blob — child Stand missing → Proof C not satisfied.
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	law := domain.Law{
		ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
		GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	stand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if !stand.ParseOK {
		t.Fatalf("stand parse failed: %+v", stand)
	}
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("seed notes must prevent confirmed_current; got %s (%s) refs=%+v",
			meta.State, meta.Rationale, meta.InstrumentRefs)
	}
}

func TestIntegration_MiLoG_plusPlusVerordnung_notConfirmedCurrent(t *testing.T) {
	// Live-equivalent: MiLoG Stand is a different G (Nr. 137); +++ cites Verordnung I Nr. 268
	// whose BGBl title omits "MiLoG" so title heuristics do not link — child Stand missing → Proof C not satisfied.
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat
	svc.Sync.Instruments = cat

	law := domain.Law{
		ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
		GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	// BGBl issue for the Verordnung — title has no "MiLoG", and no IssueLawLink.
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	}); err != nil {
		t.Fatal(err)
	}

	xmlBody := fixtures.MustRead("milog_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/milog/xml.zip", fixtures.MustZipXML("milog.xml", xmlBody))

	res, err := svc.ExportText(context.Background(), "milog", []string{export.FormatNormtext}, IncludeOpts{}, ExportGateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Freshness == nil {
		t.Fatal("expected freshness")
	}
	if res.Freshness.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("MiLoG must not be confirmed_current when +++ cites Nr. 268; got %s (%s) stand=%+v refs=%+v",
			res.Freshness.State, res.Freshness.Rationale, res.Freshness.Stand, res.Freshness.InstrumentRefs)
	}
	if res.Freshness.State != domain.FreshnessUncertain && res.Freshness.State != domain.FreshnessConfirmedStale {
		t.Fatalf("got state %s", res.Freshness.State)
	}
	if res.Freshness.Stand == nil || !res.Freshness.Stand.ParseOK {
		t.Fatalf("expected parsed Stand from XML, got %+v", res.Freshness.Stand)
	}
	found268 := false
	for _, r := range res.Freshness.InstrumentRefs {
		if r.Year == 2025 && r.Number == "268" {
			found268 = true
			break
		}
	}
	if !found268 {
		t.Fatalf("expected instrument ref Nr. 268 from +++ / seed; got %+v", res.Freshness.InstrumentRefs)
	}
	// Body still shows statutory 12 Euro (no paraphrase merge).
	chunks, ok := res.Formats[export.FormatNormtext].([]export.Chunk)
	if !ok || len(chunks) == 0 {
		t.Fatalf("normtext missing: %T", res.Formats[export.FormatNormtext])
	}
	body := ""
	for _, c := range chunks {
		body += c.Text
	}
	if !strings.Contains(body, "12 Euro") {
		t.Fatalf("expected unchanged MiLoG body with 12 Euro, got %q", body)
	}
}

func TestIntegration_MiLoG_linkedChain_currentOnlyByDefault(t *testing.T) {
	// Mid-2026 clock (real Now in this repo's test date) → milov5 current; milov4 past omitted unless include=past.
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	law := domain.Law{
		ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
		GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)
	stand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	_ = svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	})

	meta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.LinkedInstruments) != 1 || meta.LinkedInstruments[0].GIISlug != "milov5" {
		t.Fatalf("default want [milov5], got %+v", meta.LinkedInstruments)
	}
	if meta.LinkedInstruments[0].Status != instruments.StatusCurrent {
		t.Fatalf("status=%s want current", meta.LinkedInstruments[0].Status)
	}
	if meta.LinkedInstruments[0].SectionHint != "§ 1" {
		t.Fatalf("section_hint=%q", meta.LinkedInstruments[0].SectionHint)
	}
	if meta.LinkedInstruments[0].Coverage != instruments.CoverageSection {
		t.Fatalf("coverage=%q", meta.LinkedInstruments[0].Coverage)
	}
	// Fail-safe still sees seed citations for both ordinances.
	if meta.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("must not be confirmed_current; state=%s", meta.State)
	}

	withPast, err := svc.Freshness(context.Background(), "milog", IncludeOpts{Past: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withPast.LinkedInstruments) != 2 {
		t.Fatalf("include=past want 2, got %+v", withPast.LinkedInstruments)
	}
	slugs := map[string]string{}
	for _, li := range withPast.LinkedInstruments {
		slugs[li.GIISlug] = li.Status
	}
	if slugs["milov4"] != instruments.StatusPast || slugs["milov5"] != instruments.StatusCurrent {
		t.Fatalf("statuses=%v", slugs)
	}

	linked, err := svc.Freshness(context.Background(), "milog", IncludeOpts{Linked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(linked.LinkedInstruments) != 1 {
		t.Fatalf("linked default len=%d", len(linked.LinkedInstruments))
	}
	li := linked.LinkedInstruments[0]
	if !li.ResolveOK || li.LawID != "milov5" || li.GIIURL == "" {
		t.Fatalf("pointers %+v", li)
	}
	if _, ok, _ := svc.Store.GetLaw("milov5"); !ok {
		t.Fatal("expected milov5 stub in catalog")
	}
	if _, ok, _ := svc.Store.GetLaw("milov4"); !ok {
		t.Fatal("ensure should create past child stubs too")
	}
}

func TestIntegration_MiLoG_provenVRef_confirmedCurrent(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	laws := []domain.Law{
		{
			ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
			GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
		},
		{
			ID: "milov5", Abbreviation: "MiLoV5", Title: "Fünfte Mindestlohnanpassungsverordnung",
			GIIPath: "milov5", GIIURL: "https://www.gesetze-im-internet.de/milov5/",
		},
	}
	if err := svc.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}
	catalogLaws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(catalogLaws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	parentStand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if !parentStand.ParseOK {
		t.Fatalf("parent stand parse failed: %+v", parentStand)
	}
	if err := svc.Store.UpsertStand(parentStand); err != nil {
		t.Fatal(err)
	}

	childStand := citation.Parse("milov5", "BGBl. 2025 I Nr. 268")
	if !childStand.ParseOK {
		t.Fatalf("child stand parse failed: %+v", childStand)
	}
	if err := svc.Store.UpsertStand(childStand); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("want confirmed_current; got %s (%s) refs=%+v", meta.State, meta.Rationale, meta.InstrumentRefs)
	}
	if meta.Rationale == "unresolved_linked_instrument_refs" {
		t.Fatalf("rationale must not be unresolved_linked_instrument_refs; refs=%+v", meta.InstrumentRefs)
	}
	if len(meta.LinkedInstruments) != 1 || meta.LinkedInstruments[0].GIISlug != "milov5" {
		t.Fatalf("default want [milov5] current; got %+v", meta.LinkedInstruments)
	}
	if meta.LinkedInstruments[0].Status != instruments.StatusCurrent {
		t.Fatalf("status=%s want current", meta.LinkedInstruments[0].Status)
	}
}

func TestIntegration_MiLoG_provenVRef_plusPastKindV321_confirmedCurrent(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	laws := []domain.Law{
		{
			ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
			GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
		},
		{
			ID: "milov4", Abbreviation: "MiLoV4", Title: "Vierte Mindestlohnanpassungsverordnung",
			GIIPath: "milov4", GIIURL: "https://www.gesetze-im-internet.de/milov4/",
		},
		{
			ID: "milov5", Abbreviation: "MiLoV5", Title: "Fünfte Mindestlohnanpassungsverordnung",
			GIIPath: "milov5", GIIURL: "https://www.gesetze-im-internet.de/milov5/",
		},
	}
	if err := svc.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}
	catalogLaws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(catalogLaws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	parentStand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if !parentStand.ParseOK {
		t.Fatalf("parent stand parse failed: %+v", parentStand)
	}
	if err := svc.Store.UpsertStand(parentStand); err != nil {
		t.Fatal(err)
	}

	childStandV5 := citation.Parse("milov5", "BGBl. 2025 I Nr. 268")
	if !childStandV5.ParseOK {
		t.Fatalf("child stand parse failed: %+v", childStandV5)
	}
	if err := svc.Store.UpsertStand(childStandV5); err != nil {
		t.Fatal(err)
	}
	childStandV4 := citation.Parse("milov4", "BGBl. 2023 I Nr. 321")
	if !childStandV4.ParseOK {
		t.Fatalf("milov4 stand parse failed: %+v", childStandV4)
	}
	if err := svc.Store.UpsertStand(childStandV4); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	}); err != nil {
		t.Fatal(err)
	}

	editorial := "(+++ § 1 V v. 5.11.2025 I Nr. 268 +++)\n(+++ § 1 V v. 1.1.2023 I Nr. 321 +++)"
	if err := svc.Store.SetMeta("editorial:milog", editorial); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("want confirmed_current; got %s (%s) refs=%+v", meta.State, meta.Rationale, meta.InstrumentRefs)
	}
	if meta.Rationale == "unresolved_linked_instrument_refs" {
		t.Fatalf("rationale must not be unresolved_linked_instrument_refs; refs=%+v", meta.InstrumentRefs)
	}
	found268, found321 := false, false
	for _, r := range meta.InstrumentRefs {
		if r.Kind == "V" && r.Year == 2025 && r.Number == "268" {
			found268 = true
		}
		if r.Kind == "V" && r.Year == 2023 && r.Number == "321" {
			found321 = true
		}
	}
	if !found268 || !found321 {
		t.Fatalf("expected Kind V refs 268 and 321; got %+v", meta.InstrumentRefs)
	}
}

func TestIntegration_MiLoG_provenVRef_plusPastKindV321_plusBareBek313_confirmedCurrent(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	laws := []domain.Law{
		{
			ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
			GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
		},
		{
			ID: "milov4", Abbreviation: "MiLoV4", Title: "Vierte Mindestlohnanpassungsverordnung",
			GIIPath: "milov4", GIIURL: "https://www.gesetze-im-internet.de/milov4/",
		},
		{
			ID: "milov5", Abbreviation: "MiLoV5", Title: "Fünfte Mindestlohnanpassungsverordnung",
			GIIPath: "milov5", GIIURL: "https://www.gesetze-im-internet.de/milov5/",
		},
	}
	if err := svc.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}
	catalogLaws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(catalogLaws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	parentStand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if !parentStand.ParseOK {
		t.Fatalf("parent stand parse failed: %+v", parentStand)
	}
	if err := svc.Store.UpsertStand(parentStand); err != nil {
		t.Fatal(err)
	}

	childStandV5 := citation.Parse("milov5", "BGBl. 2025 I Nr. 268")
	if !childStandV5.ParseOK {
		t.Fatalf("child stand parse failed: %+v", childStandV5)
	}
	if err := svc.Store.UpsertStand(childStandV5); err != nil {
		t.Fatal(err)
	}
	childStandV4 := citation.Parse("milov4", "BGBl. 2023 I Nr. 321")
	if !childStandV4.ParseOK {
		t.Fatalf("milov4 stand parse failed: %+v", childStandV4)
	}
	if err := svc.Store.UpsertStand(childStandV4); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	}); err != nil {
		t.Fatal(err)
	}

	editorial := strings.Join([]string{
		"(+++ Bek. v. 17.10.2024 I Nr. 313 +++)",
		"(+++ § 1 V v. 5.11.2025 I Nr. 268 +++)",
		"(+++ § 1 V v. 1.1.2023 I Nr. 321 +++)",
	}, "\n")
	if err := svc.Store.SetMeta("editorial:milog", editorial); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("want confirmed_current; got %s (%s) refs=%+v", meta.State, meta.Rationale, meta.InstrumentRefs)
	}
	if meta.Rationale == "unresolved_linked_instrument_refs" {
		t.Fatalf("bare Bek 313 must not block when V refs proven; refs=%+v", meta.InstrumentRefs)
	}
	foundBek313, found268, found321 := false, false, false
	for _, r := range meta.InstrumentRefs {
		if r.Kind == "BEK" && r.Year == 2024 && r.Number == "313" {
			foundBek313 = true
		}
		if r.Kind == "V" && r.Year == 2025 && r.Number == "268" {
			found268 = true
		}
		if r.Kind == "V" && r.Year == 2023 && r.Number == "321" {
			found321 = true
		}
	}
	if !foundBek313 || !found268 || !found321 {
		t.Fatalf("expected Bek 313 + Kind V refs 268 and 321; got %+v", meta.InstrumentRefs)
	}
}

func TestIntegration_MiLoG_matchedButChildProbeOnly_uncertain(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	laws := []domain.Law{
		{
			ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
			GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
		},
		{
			ID: "milov5", Abbreviation: "MiLoV5", Title: "Fünfte Mindestlohnanpassungsverordnung",
			GIIPath: "milov5", GIIURL: "https://www.gesetze-im-internet.de/milov5/",
		},
	}
	if err := svc.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}
	catalogLaws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(catalogLaws, variants)

	now := time.Now().UTC()
	seedProbeOnlyBGBlMeta(t, svc, now)

	parentStand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if !parentStand.ParseOK {
		t.Fatalf("parent stand parse failed: %+v", parentStand)
	}
	if err := svc.Store.UpsertStand(parentStand); err != nil {
		t.Fatal(err)
	}
	childStand := citation.Parse("milov5", "BGBl. 2025 I Nr. 268")
	if !childStand.ParseOK {
		t.Fatalf("child stand parse failed: %+v", childStand)
	}
	if err := svc.Store.UpsertStand(childStand); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessUncertain {
		t.Fatalf("want uncertain when BGBl evidence is probe-only; got %s (%s)", meta.State, meta.Rationale)
	}
	if meta.Rationale != "bgbl_evidence_probe_only" {
		t.Fatalf("rationale=%q want bgbl_evidence_probe_only", meta.Rationale)
	}
}

func TestIntegration_SGB11_discovered_withoutTSV(t *testing.T) {
	// No TSV seed — discovered edge from PBAV XML ingest must drive linked_instruments + fail-safe.
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	// svc.Instruments left nil (no TSV catalog)

	law := domain.Law{
		ID: "sgb11", Abbreviation: "SGB XI", Title: "Sozialgesetzbuch XI",
		GIIPath: "sgb_11", GIIURL: "https://www.gesetze-im-internet.de/sgb_11/",
	}
	pbav := domain.Law{
		ID: "pbav2025", Abbreviation: "PBAV 2025",
		Title:   "Pflegeberufe-Ausbildungs- und Prüfungsverordnung",
		GIIPath: "pbav_2025", GIIURL: "https://www.gesetze-im-internet.de/pbav_2025/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{law, pbav}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)

	lookup := discovery.CatalogLookup{Laws: laws, Variants: variants}
	xmlBody := fixtures.MustRead("pbav_2025_snippet.xml")
	n, err := discovery.IngestLawXML(svc.Store, lookup, pbav, xmlBody)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("IngestLawXML n=%d want 1", n)
	}

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)
	stand := citation.Parse("sgb11", "Zuletzt geändert durch Art. 1 G v. 20.12.2024 BGBl. 2024 I Nr. 400")
	if !stand.ParseOK {
		t.Fatalf("stand parse failed: %+v", stand)
	}
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "sgb11", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessUncertain {
		t.Fatalf("state=%s want uncertain", meta.State)
	}
	if meta.Rationale != "linked_child_not_confirmed" {
		t.Fatalf("rationale=%q", meta.Rationale)
	}
	if len(meta.LinkedInstruments) != 1 || meta.LinkedInstruments[0].GIISlug != "pbav_2025" {
		t.Fatalf("want [pbav_2025], got %+v", meta.LinkedInstruments)
	}
	li := meta.LinkedInstruments[0]
	if li.SectionHint != "§ 55" {
		t.Fatalf("section_hint=%q", li.SectionHint)
	}
	if li.Source != discovery.SourceDiscovered {
		t.Fatalf("source=%q want %q", li.Source, discovery.SourceDiscovered)
	}
}

func TestIntegration_SGB11_pbav2025_linkedInstrument(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	law := domain.Law{
		ID: "sgb11", Abbreviation: "SGB XI", Title: "Sozialgesetzbuch XI",
		GIIPath: "sgb_11", GIIURL: "https://www.gesetze-im-internet.de/sgb_11/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)
	stand := citation.Parse("sgb11", "Zuletzt geändert durch Art. 1 G v. 20.12.2024 BGBl. 2024 I Nr. 400")
	if !stand.ParseOK {
		t.Fatalf("stand parse failed: %+v", stand)
	}
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "sgb11", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.LinkedInstruments) != 1 || meta.LinkedInstruments[0].GIISlug != "pbav_2025" {
		t.Fatalf("want [pbav_2025], got %+v", meta.LinkedInstruments)
	}
	if meta.LinkedInstruments[0].SectionHint != "§ 55" {
		t.Fatalf("section_hint=%q", meta.LinkedInstruments[0].SectionHint)
	}
	if meta.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("parent must not be confirmed_current; state=%s rationale=%s", meta.State, meta.Rationale)
	}
	if meta.Rationale != "linked_child_not_confirmed" {
		t.Fatalf("rationale=%q", meta.Rationale)
	}

	linked, err := svc.Freshness(context.Background(), "sgb11", IncludeOpts{Linked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(linked.LinkedInstruments) != 1 {
		t.Fatalf("linked len=%d", len(linked.LinkedInstruments))
	}
	li := linked.LinkedInstruments[0]
	if !li.ResolveOK || li.LawID != "pbav2025" {
		t.Fatalf("pointers %+v", li)
	}
	if _, ok, _ := svc.Store.GetLaw("pbav2025"); !ok {
		t.Fatal("expected pbav2025 stub in catalog")
	}
}

func TestIntegration_SGB11_provenPBAV_confirmedCurrent(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	laws := []domain.Law{
		{
			ID: "sgb11", Abbreviation: "SGB XI", Title: "Sozialgesetzbuch XI",
			GIIPath: "sgb_11", GIIURL: "https://www.gesetze-im-internet.de/sgb_11/",
		},
		{
			ID: "pbav2025", Abbreviation: "PBAV 2025",
			Title:   "Pflegeberufe-Ausbildungs- und Prüfungsverordnung",
			GIIPath: "pbav_2025", GIIURL: "https://www.gesetze-im-internet.de/pbav_2025/",
		},
	}
	if err := svc.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}
	catalogLaws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(catalogLaws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	parentStand := citation.Parse("sgb11", "Zuletzt geändert durch Art. 1 G v. 20.12.2024 BGBl. 2024 I Nr. 400")
	if !parentStand.ParseOK {
		t.Fatalf("parent stand parse failed: %+v", parentStand)
	}
	if err := svc.Store.UpsertStand(parentStand); err != nil {
		t.Fatal(err)
	}
	childStand := citation.Parse("pbav2025", "BGBl. 2024 I Nr. 446")
	if !childStand.ParseOK {
		t.Fatalf("child stand parse failed: %+v", childStand)
	}
	if err := svc.Store.UpsertStand(childStand); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "sgb11", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("want confirmed_current; got %s (%s) refs=%+v", meta.State, meta.Rationale, meta.InstrumentRefs)
	}
	if meta.Rationale == "unresolved_linked_instrument_refs" {
		t.Fatalf("rationale must not be unresolved_linked_instrument_refs; refs=%+v", meta.InstrumentRefs)
	}
	if len(meta.LinkedInstruments) != 1 || meta.LinkedInstruments[0].GIISlug != "pbav_2025" {
		t.Fatalf("want [pbav_2025], got %+v", meta.LinkedInstruments)
	}
}

func TestIntegration_unmatchedOperativeV_uncertain(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	// No TSV / seeded linked instruments.

	law := domain.Law{
		ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz",
		GIIPath: "arbzg", GIIURL: "https://www.gesetze-im-internet.de/arbzg/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	stand := citation.Parse("arbzg", "Zuletzt geändert durch Art. 1 G v. 20.7.2022 BGBl. 2022 I Nr. 1170")
	if !stand.ParseOK {
		t.Fatalf("stand parse failed: %+v", stand)
	}
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SetMeta("editorial:arbzg", "§ 1 V v. 1.1.2025 I Nr. 999"); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "arbzg", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessUncertain {
		t.Fatalf("want uncertain; got %s (%s)", meta.State, meta.Rationale)
	}
	if meta.Rationale != "unresolved_linked_instrument_refs" {
		t.Fatalf("rationale=%q want unresolved_linked_instrument_refs", meta.Rationale)
	}
}

func TestIntegration_ArbZG_confirmedCurrentWithoutSeededInstruments(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	stand := citation.Parse("arbzg", "Zuletzt geändert durch Art. 1 G v. 20.7.2022 BGBl. 2022 I Nr. 1170")
	if !stand.ParseOK {
		t.Fatalf("stand parse failed: %+v", stand)
	}
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	// Textnachweis-style editorial without operative V/Bek BGBl Nr — must not force uncertain.
	if err := svc.Store.SetMeta("editorial:arbzg", "(+++ Textnachweis der Geltung des § 16 Abs. 2: 1.1.2024 +++)"); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "arbzg", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("ArbZG want confirmed_current; got %s (%s) refs=%+v", meta.State, meta.Rationale, meta.InstrumentRefs)
	}
}

func TestIntegration_BGB_editorialGRefs_notForcedUncertain(t *testing.T) {
	// Mass-code noise: editorial +++ cites G / empty-Kind / bare BEK ≠ Stand → must not permanent uncertain.
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	if !stand.ParseOK {
		t.Fatalf("stand parse failed: %+v", stand)
	}
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	blob := strings.Join([]string{
		"(+++ Art. 2 G v. 15.1.2024 I Nr. 12 +++)",
		"(+++ geändert durch BGBl. 2022 I Nr. 99 +++)",
		"(+++ Hinweis: Art. 5 G v. 1.6.2021 I Nr. 45 +++)",
		"(+++ Bek. v. 1.11.2023 I Nr. 296 +++)",
		"(+++ Bek. v. 27.2.2024 I Nr. 69 +++)",
	}, "\n")
	if err := svc.Store.SetMeta("editorial:bgb", blob); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "bgb", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("BGB editorial G/BEK noise must not force uncertain; got %s (%s) refs=%+v",
			meta.State, meta.Rationale, meta.InstrumentRefs)
	}
	if meta.Rationale == "unresolved_linked_instrument_refs" {
		t.Fatal("rationale must not be unresolved_linked_instrument_refs for mass-code editorial refs")
	}
}

func TestIntegration_ArbZG_repairsStaleUnparsedStand(t *testing.T) {
	// Live failure mode: Raw present, ParseOK false (stale row) — freshness must re-parse and clear.
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	stale := domain.StandCitation{
		LawID:      "arbzg",
		Raw:        "Zuletzt geändert durch Art. 52 G v. 23.10.2024 I Nr. 323",
		Year:       2024,
		ParseOK:    false,
		ParseNotes: "insufficient structured fields",
	}
	d := time.Date(2024, 10, 23, 0, 0, 0, 0, time.UTC)
	stale.Date = &d
	if err := svc.Store.UpsertStand(stale); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "arbzg", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Stand == nil || !meta.Stand.ParseOK {
		t.Fatalf("expected repaired Stand parse_ok; stand=%+v", meta.Stand)
	}
	if meta.Stand.Number != "323" || meta.Stand.Teil != 1 {
		t.Fatalf("repaired stand=%+v want teil=1 number=323", meta.Stand)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("ArbZG after repair want confirmed_current; got %s (%s)", meta.State, meta.Rationale)
	}
}

func TestIntegration_MiLoV5_export_fundstelleStand_confirmedCurrent(t *testing.T) {
	// Child Verordnung: no standangabe; fundstelle → Stand → confirmed_current when sync fresh.
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	law := domain.Law{
		ID: "milov5", Abbreviation: "MiLoV5", Title: "Fünfte Mindestlohnanpassungsverordnung",
		GIIPath: "milov5", GIIURL: "https://www.gesetze-im-internet.de/milov5/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)

	xmlBody := fixtures.MustRead("milov5_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip", fixtures.MustZipXML("milov5.xml", xmlBody))

	res, err := svc.ExportText(context.Background(), "milov5", []string{export.FormatNormtext}, IncludeOpts{}, ExportGateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Freshness == nil {
		t.Fatal("expected freshness")
	}
	if res.Freshness.Stand == nil || !res.Freshness.Stand.ParseOK {
		t.Fatalf("expected fundstelle Stand parse_ok; stand=%+v", res.Freshness.Stand)
	}
	if res.Freshness.Stand.Year != 2025 || res.Freshness.Stand.Number != "268" {
		t.Fatalf("stand=%+v want 2025/268", res.Freshness.Stand)
	}
	if res.Freshness.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("milov5 want confirmed_current; got %s (%s)", res.Freshness.State, res.Freshness.Rationale)
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

	// P1: DataFresh also requires fresh GII feed (TOC already set by seedCatalog).
	if err := svc.Store.SetMetaTime("last_gii_feed_success", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	st, err := svc.SyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.DataFresh {
		t.Fatal("data_fresh should be true via ELI probe within max age")
	}
}

func TestIntegration_EStG_softGesetzFPDemoted(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	parent := domain.Law{
		ID: "estg", Abbreviation: "EStG", Title: "Einkommensteuergesetz",
		GIIPath: "estg", GIIURL: "https://www.gesetze-im-internet.de/estg/",
	}
	children := []domain.Law{
		{ID: "altzertg", Abbreviation: "AltZertG", Title: "Gesetz über die Zertifizierung von Altersvorsorgeverträgen", GIIPath: "altzertg"},
		{ID: "astg", Abbreviation: "AStG", Title: "Gesetz zur Regelung der Rechtsverhältnisse der in der Steuerverwaltung tätigen Personen", GIIPath: "astg"},
	}
	if err := svc.Store.UpsertLaws(append([]domain.Law{parent}, children...)); err != nil {
		t.Fatal(err)
	}
	for _, c := range children {
		if err := svc.Store.UpsertDiscoveredLink(domain.DiscoveredEdge{
			ParentLawID: "estg", GIISlug: c.GIIPath, Confidence: discovery.ConfidenceHigh,
			Notes: "BGBl 2020 I Nr. 123",
		}); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC()
	seedSyncFreshMeta(t, svc, now)
	stand := citation.Parse("estg", "Zuletzt geändert durch Art. 1 G v. 1.1.2024 I Nr. 100")
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "estg", IncludeOpts{Linked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.LinkedInstruments) != 0 {
		t.Fatalf("soft Gesetz children omitted from linked response; got %+v", meta.LinkedInstruments)
	}
	if meta.Rationale == "unresolved_linked_instrument_refs" {
		t.Fatalf("must not fail-close on soft Gesetz FP; state=%s rationale=%s refs=%+v",
			meta.State, meta.Rationale, meta.InstrumentRefs)
	}
	if meta.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("want confirmed_current when only soft Gesetz linked; got %s (%s)", meta.State, meta.Rationale)
	}

	edges, err := svc.Store.DiscoveredForParent("estg")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 {
		t.Fatalf("discovery rows must remain in store; got %d", len(edges))
	}
}
