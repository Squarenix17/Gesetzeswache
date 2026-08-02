package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestFreshnessMeta_safeToServe(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	_ = svc.Store.UpsertStand(stand)

	current, err := svc.Freshness(context.Background(), "bgb", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !current.SafeToServe || current.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("confirmed_current must be safe_to_serve=true; got %+v", current)
	}
	raw, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if !jsonContainsField(raw, "safe_to_serve") {
		t.Fatalf("safe_to_serve must always be emitted; json=%s", raw)
	}

	issueID := citation.IssueID(1, 2026, "999")
	_ = svc.Store.UpsertIssue(domain.GazetteIssue{ID: issueID, Teil: 1, Year: 2026, Number: "999"})
	_ = svc.Store.UpsertLink(domain.IssueLawLink{IssueID: issueID, LawID: "bgb", Class: domain.LinkConfirmed})
	stale, err := svc.Freshness(context.Background(), "bgb", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if stale.SafeToServe || stale.State == domain.FreshnessConfirmedCurrent {
		t.Fatalf("stale must be safe_to_serve=false; got %+v", stale)
	}
}

func TestResolve_safeToServeOnFreshness(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	_ = svc.Store.UpsertStand(stand)

	res, err := svc.Resolve(context.Background(), "BGB", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Freshness == nil || !res.Freshness.SafeToServe {
		t.Fatalf("resolve freshness must include safe_to_serve=true; %+v", res.Freshness)
	}
}

func TestResolve_SGBBookAliases(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	laws := []domain.Law{
		{
			ID: "sgb3", Abbreviation: "SGB III", Title: "Sozialgesetzbuch III",
			GIIPath: "sgb_3", GIIURL: "https://www.gesetze-im-internet.de/sgb_3/",
		},
	}
	_ = svc.Store.UpsertLaws(laws)
	svc.Search.Swap(laws, nil)

	for _, q := range []string{"SGB III", "sgb_3"} {
		res, err := svc.Resolve(context.Background(), q, IncludeOpts{})
		if err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		if !res.Matched || res.Law == nil || res.Law.ID != "sgb3" {
			t.Fatalf("query %q: res=%+v want sgb3", q, res)
		}
	}
}

func TestResolve_SGBV_notAMNutzenV(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	laws := []domain.Law{
		{
			ID: "sgb5", Abbreviation: "SGB_5", Title: "Sozialgesetzbuch (SGB) Fünftes Buch (V)",
			GIIPath: "sgb_5", GIIURL: "https://www.gesetze-im-internet.de/sgb_5/",
		},
		{
			ID: "amnutzenv", Abbreviation: "AMNutzenV", Title: "Anmeldung zur Nutzung von Verkehrswegen",
			GIIPath: "amnutzenv", GIIURL: "https://www.gesetze-im-internet.de/amnutzenv/",
		},
	}
	_ = svc.Store.UpsertLaws(laws)
	svc.Search.Swap(laws, nil)

	res, err := svc.Resolve(context.Background(), "SGB V", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched && res.Law != nil && res.Law.ID == "amnutzenv" {
		t.Fatalf("SGB V must not resolve to amnutzenv; res=%+v", res)
	}
	if !res.Matched || res.Law == nil || res.Law.ID != "sgb5" {
		t.Fatalf("SGB V must resolve to sgb5; res=%+v", res)
	}
}

func jsonContainsField(raw []byte, field string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[field]
	return ok
}
