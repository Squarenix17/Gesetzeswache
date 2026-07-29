package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func openTempStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestStore_lawsVariantsStandIssuesLinks(t *testing.T) {
	st := openTempStore(t)
	now := time.Now().UTC()

	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz", GIIPath: "arbzg"}
	if err := st.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	created, err := st.UpsertLawIfAbsent(domain.Law{ID: "arbzg", Abbreviation: "ArbZG"})
	if err != nil || created {
		t.Fatalf("UpsertLawIfAbsent existing: created=%v err=%v", created, err)
	}
	created, err = st.UpsertLawIfAbsent(domain.Law{ID: "bgb", Abbreviation: "BGB", Title: "BGB"})
	if err != nil || !created {
		t.Fatalf("UpsertLawIfAbsent new: created=%v err=%v", created, err)
	}

	laws, err := st.ListLaws()
	if err != nil || len(laws) != 2 {
		t.Fatalf("ListLaws: %d err=%v", len(laws), err)
	}
	got, ok, err := st.GetLaw("bgb")
	if err != nil || !ok || got.Abbreviation != "BGB" {
		t.Fatalf("GetLaw: %+v ok=%v err=%v", got, ok, err)
	}

	variants := []domain.LawVariant{{Variant: "ArbZG", LawID: "arbzg"}}
	if err := st.ReplaceVariants(variants); err != nil {
		t.Fatal(err)
	}
	vlist, err := st.ListVariants()
	if err != nil || len(vlist) != 1 {
		t.Fatalf("ListVariants: %v err=%v", vlist, err)
	}

	stand := domain.StandCitation{LawID: "arbzg", Raw: "BGBl. I", ParseOK: true}
	if err := st.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	gotStand, ok, err := st.GetStand("arbzg")
	if err != nil || !ok || gotStand.LawID != "arbzg" {
		t.Fatalf("GetStand: %+v ok=%v err=%v", gotStand, ok, err)
	}

	iss := domain.GazetteIssue{ID: "bgbl-1", Title: "Test", Teil: 1, Year: 2024, Number: "10", FirstSeenAt: now}
	if err := st.UpsertIssue(iss); err != nil {
		t.Fatal(err)
	}
	gotIss, ok, err := st.GetIssue("bgbl-1")
	if err != nil || !ok || gotIss.Number != "10" {
		t.Fatalf("GetIssue: %+v ok=%v err=%v", gotIss, ok, err)
	}
	issues, err := st.ListIssues()
	if err != nil || len(issues) != 1 {
		t.Fatalf("ListIssues: %v err=%v", issues, err)
	}

	link := domain.IssueLawLink{IssueID: "bgbl-1", LawID: "arbzg", Class: domain.LinkConfirmed, CreatedAt: now}
	if err := st.UpsertLink(link); err != nil {
		t.Fatal(err)
	}
	links, err := st.LinksForLaw("arbzg")
	if err != nil || len(links) != 1 {
		t.Fatalf("LinksForLaw: %v err=%v", links, err)
	}
	allLinks, err := st.ListLinks()
	if err != nil || len(allLinks) != 1 {
		t.Fatalf("ListLinks: %v err=%v", allLinks, err)
	}
}

func TestStore_freshnessMetaSyncLog(t *testing.T) {
	st := openTempStore(t)
	now := time.Now().UTC()

	rec := domain.FreshnessRecord{LawID: "arbzg", State: domain.FreshnessConfirmedStale, EvaluatedAt: now}
	if err := st.PutFreshness(rec); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.GetFreshness("arbzg")
	if err != nil || !ok || got.State != domain.FreshnessConfirmedStale {
		t.Fatalf("GetFreshness: %+v ok=%v err=%v", got, ok, err)
	}
	byState, err := st.ListFreshnessByState(domain.FreshnessConfirmedStale)
	if err != nil || len(byState) != 1 {
		t.Fatalf("ListFreshnessByState: %v err=%v", byState, err)
	}

	if err := st.SetMetaTime("last_toc_success", now); err != nil {
		t.Fatal(err)
	}
	ts, ok, err := st.GetMetaTime("last_toc_success")
	if err != nil || !ok || ts.IsZero() {
		t.Fatalf("GetMetaTime: %v ok=%v err=%v", ts, ok, err)
	}
	if err := st.SetMeta("fingerprint", "abc"); err != nil {
		t.Fatal(err)
	}
	val, ok, err := st.GetMeta("fingerprint")
	if err != nil || !ok || val != "abc" {
		t.Fatalf("GetMeta: %q ok=%v err=%v", val, ok, err)
	}

	attempt := domain.SyncAttempt{Source: "toc", StartedAt: now, EndedAt: now, Success: true}
	if err := st.AppendSyncAttempt(attempt); err != nil {
		t.Fatal(err)
	}
}

func TestStore_CountBGBlIndex(t *testing.T) {
	st := openTempStore(t)
	n, err := st.CountBGBlIndex()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
	if err := st.UpsertBGBlIndex(BGBlIndexEntry{Teil: 1, Year: 2024, Number: "10", GIISlug: "arbzg", LawID: "arbzg"}); err != nil {
		t.Fatal(err)
	}
	n, err = st.CountBGBlIndex()
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}

func TestOpen_invalidPath(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "missing", "nested", "db"))
	if err == nil {
		t.Fatal("expected error opening nested path without parent")
	}
}
