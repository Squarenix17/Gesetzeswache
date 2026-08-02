package service

import (
	"context"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestForceRecheck_staleHonestyPayload(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	now := time.Now().UTC()
	seedFreshSync(t, svc, now)

	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 10")
	_ = svc.Store.UpsertStand(stand)
	iss := domain.GazetteIssue{
		ID: "BGBl-1/2025/5", Teil: 1, Year: 2025, Number: "5",
		Title: "Gesetz zur Änderung des BGB", ExistenceConfidence: "high", Matched: true,
		FirstSeenAt: now,
	}
	_ = svc.Store.UpsertIssue(iss)
	_ = svc.Store.UpsertLink(domain.IssueLawLink{
		IssueID: iss.ID, LawID: "bgb", Class: domain.LinkConfirmed, CreatedAt: now,
	})

	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", fixtures.MustRead("gii_feed.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", fixtures.MustRead("bgbl1_ok.xml"))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", fixtures.MustRead("bgbl2_ok.xml"))
	mt.SetBytes("www.gesetze-im-internet.de", "/bgb/index.html", []byte(`<html><body>Stand: Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 10</body></html>`))

	before, err := svc.Freshness(context.Background(), "bgb", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if before.State != domain.FreshnessConfirmedStale {
		t.Fatalf("precondition state=%s want confirmed_stale", before.State)
	}

	res, err := svc.ForceRecheck(context.Background(), "bgb")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "recheck_completed" {
		t.Fatalf("status=%q", res.Status)
	}
	if res.LawID != "bgb" {
		t.Fatalf("law_id=%q", res.LawID)
	}
	if res.StateBefore != domain.FreshnessConfirmedStale {
		t.Fatalf("state_before=%s", res.StateBefore)
	}
	if res.StateAfter != domain.FreshnessConfirmedStale {
		t.Fatalf("state_after=%s want still stale", res.StateAfter)
	}
	if res.Changed {
		t.Fatal("changed should be false when still stale")
	}
	if res.Reason == "" {
		t.Fatal("reason required when still stale")
	}
	if len(res.NewerIssueIDs) == 0 {
		t.Fatal("newer_issue_ids required when still stale")
	}
	if res.Stand == nil {
		t.Fatal("stand required when still stale")
	}
}
