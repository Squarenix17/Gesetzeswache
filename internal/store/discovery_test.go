package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestUpsertLookupBGBlIndex(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	entry := BGBlIndexEntry{
		Teil:    1,
		Year:    2025,
		Number:  "268",
		GIISlug: "milov5",
		LawID:   "milov5",
	}
	if err := st.UpsertBGBlIndex(entry); err != nil {
		t.Fatal(err)
	}

	got, ok, err := st.LookupBGBlIndex(1, 2025, "268")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected hit")
	}
	if got.GIISlug != "milov5" || got.LawID != "milov5" {
		t.Fatalf("got %#v", got)
	}

	_, ok, err = st.LookupBGBlIndex(1, 2025, "999")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected miss for unknown number")
	}
}

func TestUpsertDiscoveredLink_Idempotent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	edge := domain.DiscoveredEdge{
		ParentLawID: "milog",
		GIISlug:     "milov5",
		EdgeType:    "ermaechtigung",
		Confidence:  "high",
		SectionHint: "§ 1",
	}
	if err := st.UpsertDiscoveredLink(edge); err != nil {
		t.Fatal(err)
	}
	edge.Notes = "updated notes"
	if err := st.UpsertDiscoveredLink(edge); err != nil {
		t.Fatal(err)
	}

	rows, err := st.DiscoveredForParent("milog")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d: %+v", len(rows), rows)
	}
	if rows[0].Confidence != "high" {
		t.Fatalf("confidence=%q want high", rows[0].Confidence)
	}
	if rows[0].Notes != "updated notes" {
		t.Fatalf("notes=%q want updated", rows[0].Notes)
	}
}

func TestDeleteDiscoveredBySlug_RemovesStaleParents(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	_ = st.UpsertDiscoveredLink(domain.DiscoveredEdge{ParentLawID: "milog", GIISlug: "asphausbv", Confidence: "high"})
	_ = st.UpsertDiscoveredLink(domain.DiscoveredEdge{ParentLawID: "bbig", GIISlug: "asphausbv", Confidence: "high"})
	_ = st.UpsertDiscoveredLink(domain.DiscoveredEdge{ParentLawID: "sgb11", GIISlug: "pbav_2025", Confidence: "high"})

	if err := st.DeleteDiscoveredBySlug("asphausbv"); err != nil {
		t.Fatal(err)
	}
	milo, _ := st.DiscoveredForParent("milog")
	if len(milo) != 0 {
		t.Fatalf("milog edges=%d want 0", len(milo))
	}
	sgb, _ := st.DiscoveredForParent("sgb11")
	if len(sgb) != 1 {
		t.Fatalf("sgb11 edges=%d want 1", len(sgb))
	}
}

func TestClearDiscoveredLinks(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_ = st.UpsertDiscoveredLink(domain.DiscoveredEdge{ParentLawID: "milog", GIISlug: "milov5", Confidence: "high"})
	if err := st.ClearDiscoveredLinks(); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountDiscoveredLinks()
	if err != nil || n != 0 {
		t.Fatalf("count=%d err=%v want 0", n, err)
	}
}

// TestClearLiveDB clears discovered_links on GEW_CLEAR_DISC_DB when set (ops helper).
func TestClearLiveDB(t *testing.T) {
	path := os.Getenv("GEW_CLEAR_DISC_DB")
	if path == "" {
		t.Skip("set GEW_CLEAR_DISC_DB to clear a live store")
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.ClearDiscoveredLinks(); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountDiscoveredLinks()
	if err != nil || n != 0 {
		t.Fatalf("count=%d err=%v want 0", n, err)
	}
}

func TestDiscoveredForParent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	edges := []domain.DiscoveredEdge{
		{ParentLawID: "milog", GIISlug: "milov5", EdgeType: "ermaechtigung", Confidence: "high"},
		{ParentLawID: "milog", GIISlug: "milov4", EdgeType: "ermaechtigung", Confidence: "medium"},
		{ParentLawID: "sgb11", GIISlug: "pbav_2025", EdgeType: "ermaechtigung", Confidence: "high"},
	}
	for _, e := range edges {
		if err := st.UpsertDiscoveredLink(e); err != nil {
			t.Fatal(err)
		}
	}

	milog, err := st.DiscoveredForParent("milog")
	if err != nil {
		t.Fatal(err)
	}
	if len(milog) != 2 {
		t.Fatalf("milog want 2, got %d", len(milog))
	}

	sgb11, err := st.DiscoveredForParent("sgb11")
	if err != nil {
		t.Fatal(err)
	}
	if len(sgb11) != 1 || sgb11[0].GIISlug != "pbav_2025" {
		t.Fatalf("sgb11=%+v", sgb11)
	}

	empty, err := st.DiscoveredForParent("unknown")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("unknown want 0, got %d", len(empty))
	}
}
