package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func seedMiLoGIndexFixture(t *testing.T, svc *Service, mt *httpmock.Transport) {
	t.Helper()
	seedCatalog(t, svc, mt)
	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat
	milog := domain.Law{
		ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
		GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
	}
	if err := svc.Store.UpsertLaws([]domain.Law{milog}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)
	seedSyncFreshMeta(t, svc, time.Now().UTC())
	_ = svc.Store.UpsertStand(citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137"))
	_ = svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	})
	mt.SetBytes("www.gesetze-im-internet.de", "/milog/xml.zip",
		fixtures.MustZipXML("milog.xml", fixtures.MustRead("milog_snippet.xml")))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip",
		fixtures.MustZipXML("milov5.xml", fixtures.MustRead("milov5_snippet.xml")))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov4/xml.zip",
		fixtures.MustZipXML("milov4.xml", fixtures.MustRead("milov5_snippet.xml")))
}

func TestExportIndexChunks_MiLoG(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedMiLoGIndexFixture(t, svc, mt)

	res, err := svc.ExportIndexChunks(context.Background(), "MiLoG", IndexOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || len(res.Chunks) == 0 {
		t.Fatalf("result: %+v", res)
	}
	if res.BundleFreshness == nil {
		t.Fatal("expected bundle_freshness")
	}

	var parentN, opN int
	for _, c := range res.Chunks {
		raw, _ := json.Marshal(c)
		s := string(raw)
		for _, bad := range []string{"abbreviation", "paragraph_num", "stand_raw", "freshness_state", "safe_to_serve"} {
			if strings.Contains(s, `"`+bad+`"`) {
				t.Fatalf("forbidden %q in %s", bad, s)
			}
		}
		switch c.LawID {
		case "milog":
			parentN++
			if c.LawName != "Mindestlohngesetz" {
				t.Fatalf("law_name=%q", c.LawName)
			}
			if c.InstrumentKind != "gesetz" {
				t.Fatalf("kind=%q", c.InstrumentKind)
			}
			if c.ParentLawID != "" || c.ParentSectionHint != "" {
				t.Fatalf("parent fields on gesetz: %+v", c)
			}
		case "milov5":
			opN++
			if c.InstrumentKind != "verordnung" {
				t.Fatalf("kind=%q", c.InstrumentKind)
			}
			if c.ParentLawID != "milog" || c.ParentSectionHint != "§ 1" {
				t.Fatalf("parent fields: %+v", c)
			}
		default:
			t.Fatalf("unexpected law_id %q", c.LawID)
		}
	}
	if parentN == 0 || opN == 0 {
		t.Fatalf("parent=%d operative=%d", parentN, opN)
	}
	for _, c := range res.Chunks {
		if strings.EqualFold(c.SectionRef, "Eingangsformel") {
			t.Fatalf("Eingangsformel must not appear in index chunks: %+v", c)
		}
	}

	filtered, err := svc.ExportIndexChunks(context.Background(), "MiLoG", IndexOpts{
		Sections: export.ParseSectionRefs("§ 1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Chunks) == 0 {
		t.Fatal("section filter emptied unexpectedly")
	}
	for _, c := range filtered.Chunks {
		refOK := export.MatchSectionFilter(c, []string{"§ 1"})
		if !refOK {
			t.Fatalf("chunk escaped filter: %+v", c)
		}
	}
	hasParent := false
	hasVO := false
	for _, c := range filtered.Chunks {
		if c.LawID == "milog" {
			hasParent = true
		}
		if c.LawID == "milov5" {
			hasVO = true
		}
	}
	if !hasParent || !hasVO {
		t.Fatalf("§1 filter should keep parent+VO via hint; parent=%v vo=%v", hasParent, hasVO)
	}

	withPast, err := svc.ExportIndexChunks(context.Background(), "MiLoG", IndexOpts{Past: true})
	if err != nil {
		t.Fatal(err)
	}
	laws := map[string]bool{}
	for _, c := range withPast.Chunks {
		laws[c.LawID] = true
	}
	if !laws["milov4"] || !laws["milov5"] {
		t.Fatalf("past want milov4+milov5, got %v", laws)
	}
}

func TestExportIndexChunks_dropsEingangsformel(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedMiLoGIndexFixture(t, svc, mt)
	// Override milov5 with a fixture that includes Eingangsformel + § 1.
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip",
		fixtures.MustZipXML("milov5.xml", mustReadExportTestdata(t, "vo_eingangsformel_snippet.xml")))

	res, err := svc.ExportIndexChunks(context.Background(), "MiLoG", IndexOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var voN int
	for _, c := range res.Chunks {
		if strings.EqualFold(c.SectionRef, "Eingangsformel") {
			t.Fatalf("Eingangsformel leaked into index: %+v", c)
		}
		if c.LawID == "milov5" {
			voN++
			if c.SectionRef != "§ 1" {
				t.Fatalf("expected milov5 § 1 only, got %+v", c)
			}
		}
	}
	if voN == 0 {
		t.Fatal("expected milov5 § chunks after dropping Eingangsformel")
	}
}

func mustReadExportTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "export", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExportIndexChunks_unmatched(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	res, err := svc.ExportIndexChunks(context.Background(), "zzzz-not-a-law", IndexOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched || len(res.Chunks) != 0 {
		t.Fatalf("%+v", res)
	}
}
