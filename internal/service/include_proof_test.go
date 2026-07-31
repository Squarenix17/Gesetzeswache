package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestFreshness_includeProof_returnsResolutions(t *testing.T) {
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
	stand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	_ = svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	})

	defaultMeta, err := svc.Freshness(context.Background(), "milog", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultMeta.Proof) != 0 {
		t.Fatalf("default must omit proof; got %+v", defaultMeta.Proof)
	}

	withProof, err := svc.Freshness(context.Background(), "milog", IncludeOpts{Proof: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withProof.Proof) == 0 {
		t.Fatal("include=proof must attach vref resolutions")
	}
	found := false
	for _, p := range withProof.Proof {
		if p.Ref.Year == 2025 && p.Ref.Number == "268" {
			found = true
			if p.MatchedGIISlug == "" {
				t.Fatalf("expected matched_gii_slug on operative V ref; got %+v", p)
			}
			if p.MatchMethod == "" {
				t.Fatalf("expected match_method; got %+v", p)
			}
		}
	}
	if !found {
		t.Fatalf("expected §1 V 2025/268 resolution in proof; got %+v", withProof.Proof)
	}

	withBoth, err := svc.Freshness(context.Background(), "milog", IncludeOpts{Linked: true, Proof: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withBoth.Proof) == 0 {
		t.Fatal("include=linked,proof must still attach proof")
	}
	if len(withBoth.LinkedInstruments) != 1 || !withBoth.LinkedInstruments[0].ResolveOK {
		t.Fatalf("include=linked must still attach pointers; got %+v", withBoth.LinkedInstruments)
	}
}
