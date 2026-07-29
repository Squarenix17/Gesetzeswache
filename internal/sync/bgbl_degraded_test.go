package sync

import (
	"context"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/freshness"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func seedFreshEvidence(t *testing.T, o *Orchestrator, at time.Time) {
	t.Helper()
	for _, key := range []string{"last_toc_success", "last_gii_feed_success", "last_bgbl_feed_success"} {
		if err := o.Store.SetMetaTime(key, at); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunBGBlFeeds_partialFailureSetsDegradedMarker(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", httpmock.Response{
		Status: 500,
		Body:   []byte("error"),
	})
	o := newTestOrchestrator(t, mt)
	prior := time.Now().UTC().Add(-2 * time.Hour)
	if err := o.Store.SetMetaTime("last_bgbl_feed_success", prior); err != nil {
		t.Fatal(err)
	}

	if err := o.RunBGBlFeeds(context.Background()); err == nil {
		t.Fatal("expected partial feed failure")
	}
	degraded, ok, err := o.Store.GetMetaTime(metaKeyBGBlFeedDegraded)
	if err != nil || !ok || degraded.IsZero() {
		t.Fatalf("degraded marker missing: t=%v ok=%v err=%v", degraded, ok, err)
	}
	if !degraded.After(prior) {
		t.Fatalf("degraded=%v should be newer than prior success=%v", degraded, prior)
	}
	stored, ok, _ := o.Store.GetMetaTime("last_bgbl_feed_success")
	if !ok || !stored.Equal(prior) {
		t.Fatalf("prior success stamp should remain: got=%v ok=%v", stored, ok)
	}
}

func TestRunBGBlFeeds_fullSuccessClearsDegradedMarker(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	o := newTestOrchestrator(t, mt)
	if err := o.Store.SetMetaTime(metaKeyBGBlFeedDegraded, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if err := o.RunBGBlFeeds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := o.Store.GetMeta(metaKeyBGBlFeedDegraded); ok && v != "" {
		t.Fatalf("degraded marker should be cleared, got %q", v)
	}
	if _, ok, _ := o.Store.GetMetaTime("last_bgbl_feed_success"); !ok {
		t.Fatal("expected fresh success stamp")
	}
}

func TestReconcile_uncertainWhenBGBlDegradedMarkerNewer(t *testing.T) {
	mt := httpmock.New()
	o := newTestOrchestrator(t, mt)
	now := time.Now().UTC()
	at := now.Add(-2 * time.Hour)
	seedFreshEvidence(t, o, at)
	if err := o.Store.SetMetaTime(metaKeyBGBlFeedDegraded, now); err != nil {
		t.Fatal(err)
	}
	if err := o.Store.UpsertLaws([]domain.Law{{
		ID: "bgb", Abbreviation: "BGB", Title: "BGB", GIIPath: "bgb",
	}}); err != nil {
		t.Fatal(err)
	}
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	if err := o.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}

	if err := o.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, ok, err := o.Store.GetFreshness("bgb")
	if err != nil || !ok {
		t.Fatalf("freshness missing: ok=%v err=%v", ok, err)
	}
	if rec.State != domain.FreshnessUncertain {
		t.Fatalf("state=%s want uncertain when BGBl degraded marker newer than success", rec.State)
	}
}

func TestReconcile_confirmedCurrentAfterDegradedSelfHeal(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	o := newTestOrchestrator(t, mt)
	now := time.Now().UTC()
	seedFreshEvidence(t, o, now.Add(-2*time.Hour))
	if err := o.Store.SetMetaTime(metaKeyBGBlFeedDegraded, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := o.Store.UpsertLaws([]domain.Law{{
		ID: "bgb", Abbreviation: "BGB", Title: "BGB", GIIPath: "bgb",
	}}); err != nil {
		t.Fatal(err)
	}
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	if err := o.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}

	if err := o.RunBGBlFeeds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := o.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, ok, err := o.Store.GetFreshness("bgb")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("state=%s want confirmed_current after degraded self-heal", rec.State)
	}
}

func TestEffectiveBGBlFeedTime_ignoredWhenDegradedOlderThanSuccess(t *testing.T) {
	success := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	degraded := success.Add(-time.Hour)
	effective := freshness.EffectiveBGBlFeedTime(success, degraded)
	if !effective.Equal(success) {
		t.Fatalf("effective=%v want success=%v when degraded older", effective, success)
	}
	now := success.Add(time.Hour)
	_, probeOnly := freshness.BGBLEvidence(effective, time.Time{}, now, 6*time.Hour)
	if probeOnly {
		t.Fatal("feed evidence should not be probe-only")
	}
	rec := freshness.Evaluate(freshness.Input{
		LawID:              "bgb",
		Stand:              domain.StandCitation{Year: 2023, Teil: 1, Number: "198", ParseOK: true},
		LastTOCSuccess:     now.Add(-time.Hour),
		LastGIIFeedSuccess: now.Add(-time.Hour),
		LastBGBlSuccess:    effective,
		Now:                now,
		MaxAge:             6 * time.Hour,
	})
	if rec.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("state=%s want confirmed_current when stale degraded marker ignored", rec.State)
	}
}
