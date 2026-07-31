package service

import (
	"context"
	"fmt"
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

func TestExportOperativeBundle_MiLoG_currentOnly(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
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

	mt.SetBytes("www.gesetze-im-internet.de", "/milog/xml.zip",
		fixtures.MustZipXML("milog.xml", fixtures.MustRead("milog_snippet.xml")))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip",
		fixtures.MustZipXML("milov5.xml", fixtures.MustRead("milov5_snippet.xml")))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov4/xml.zip",
		fixtures.MustZipXML("milov4.xml", fixtures.MustRead("milov5_snippet.xml")))

	res, err := svc.ExportOperativeBundle(context.Background(), "MiLoG",
		[]string{export.FormatNormtext, export.FormatHierarchical}, BundleOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Parent == nil || res.Parent.Law == nil || res.Parent.Law.ID != "milog" {
		t.Fatalf("parent: %+v", res.Parent)
	}
	if len(res.Operative) != 1 {
		t.Fatalf("operative len=%d want 1: %+v", len(res.Operative), res.Operative)
	}
	op := res.Operative[0]
	if op.Link.GIISlug != "milov5" || op.Link.Status != instruments.StatusCurrent {
		t.Fatalf("link %+v", op.Link)
	}
	if op.Link.SectionHint != "§ 1" {
		t.Fatalf("section_hint=%q", op.Link.SectionHint)
	}
	if op.Law == nil || op.Law.ID != "milov5" {
		t.Fatalf("operative law %+v", op.Law)
	}
	// Parent normtext must not contain the VO rate (no section pollution).
	parentNT, _ := res.Parent.Formats[export.FormatNormtext].([]export.Chunk)
	parentText := ""
	for _, c := range parentNT {
		parentText += c.Text
	}
	if strings.Contains(parentText, "13,90") {
		t.Fatalf("parent chunks polluted with VO rate: %q", parentText)
	}
	if !strings.Contains(parentText, "12 Euro") {
		t.Fatalf("expected parent rate text, got %q", parentText)
	}
	opNT, _ := op.Formats[export.FormatNormtext].([]export.Chunk)
	opText := ""
	for _, c := range opNT {
		opText += c.Text
	}
	if !strings.Contains(opText, "13,90") {
		t.Fatalf("expected VO rate in operative: %q", opText)
	}
	if res.BundleFreshness == nil || res.BundleFreshness.SafeToServe {
		t.Fatalf("safe_to_serve should be false for uncertain parent: %+v", res.BundleFreshness)
	}
	if res.BundleFreshness.State != domain.FreshnessUncertain {
		t.Fatalf("bundle state=%s", res.BundleFreshness.State)
	}
	if res.Formats != nil {
		t.Fatalf("compose formats should be absent without Compose: %+v", res.Formats)
	}

	withPast, err := svc.ExportOperativeBundle(context.Background(), "MiLoG",
		[]string{export.FormatNormtext}, BundleOpts{Past: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withPast.Operative) != 2 {
		t.Fatalf("include=past want 2, got %d", len(withPast.Operative))
	}
	slugs := map[string]string{}
	for _, o := range withPast.Operative {
		slugs[o.Link.GIISlug] = o.Link.Status
	}
	if slugs["milov4"] != instruments.StatusPast || slugs["milov5"] != instruments.StatusCurrent {
		t.Fatalf("statuses=%v", slugs)
	}
}

func TestExportOperativeBundle_MiLoG_supersededPastKindV_safeToServe(t *testing.T) {
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

	mt.SetBytes("www.gesetze-im-internet.de", "/milog/xml.zip",
		fixtures.MustZipXML("milog.xml", fixtures.MustRead("milog_snippet.xml")))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip",
		fixtures.MustZipXML("milov5.xml", fixtures.MustRead("milov5_snippet.xml")))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov4/xml.zip",
		fixtures.MustZipXML("milov4.xml", fixtures.MustRead("milov5_snippet.xml")))

	res, err := svc.ExportOperativeBundle(context.Background(), "MiLoG",
		[]string{export.FormatHierarchical}, BundleOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.BundleFreshness == nil {
		t.Fatal("expected bundle freshness")
	}
	if !res.BundleFreshness.SafeToServe {
		t.Fatalf("safe_to_serve should be true for superseded scenario: %+v", res.BundleFreshness)
	}
	if res.BundleFreshness.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("bundle state=%s want confirmed_current; freshness=%+v", res.BundleFreshness.State, res.BundleFreshness)
	}
}

func TestExportOperativeBundle_composeDisplayOnly(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
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
	mt.SetBytes("www.gesetze-im-internet.de", "/milog/xml.zip",
		fixtures.MustZipXML("milog.xml", fixtures.MustRead("milog_snippet.xml")))
	mt.SetBytes("www.gesetze-im-internet.de", "/milov5/xml.zip",
		fixtures.MustZipXML("milov5.xml", fixtures.MustRead("milov5_snippet.xml")))

	res, err := svc.ExportOperativeBundle(context.Background(), "MiLoG",
		[]string{export.FormatHierarchical}, BundleOpts{Compose: true})
	if err != nil {
		t.Fatal(err)
	}
	composed, _ := res.Formats[export.FormatHierarchical].(string)
	if !strings.Contains(composed, "Verordnung (nicht Teil des MiLoG)") {
		t.Fatalf("missing display banner:\n%s", composed)
	}
	if !strings.Contains(composed, "13,90") {
		t.Fatalf("missing VO in compose:\n%s", composed)
	}
	// Index path still clean
	parentH, _ := res.Parent.Formats[export.FormatHierarchical].(string)
	if strings.Contains(parentH, "13,90") {
		t.Fatalf("parent hierarchical polluted")
	}
	if res.Operative[0].Placement != export.PlacementAfterParentSection {
		t.Fatalf("placement=%s", res.Operative[0].Placement)
	}
}

func TestExportOperativeBundle_parentOnlyNoLinks(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedSyncFreshMeta(t, svc, time.Now().UTC())
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip",
		fixtures.MustZipXML("arbzg.xml", fixtures.MustRead("arbzg_snippet.xml")))
	res, err := svc.ExportOperativeBundle(context.Background(), "ArbZG",
		[]string{export.FormatHierarchical}, BundleOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Parent == nil {
		t.Fatal("expected parent")
	}
	if len(res.Operative) != 0 {
		t.Fatalf("operative=%+v", res.Operative)
	}
}

func TestExportOperativeBundle_capExceeded(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)

	tsvPath := filepath.Join(t.TempDir(), "links.tsv")
	var b strings.Builder
	b.WriteString("# parent\tkind\tslug\tnotes\teffective_from\tsection_hint\n")
	for i := 0; i < MaxOperativeBundleMembers+1; i++ {
		slug := fmt.Sprintf("capv_%d", i)
		hint := fmt.Sprintf("§ %d", i+1)
		fmt.Fprintf(&b, "p\tverordnung\t%s\tBGBl 2024 I Nr. %d\t2024-01-01\t%s\n", slug, i+1, hint)
	}
	if err := os.WriteFile(tsvPath, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := instruments.LoadTSV(tsvPath)
	if err != nil {
		t.Fatal(err)
	}
	svc.Instruments = cat

	parent := domain.Law{ID: "p", Abbreviation: "P", Title: "Parent", GIIPath: "p", GIIURL: "https://www.gesetze-im-internet.de/p/"}
	if err := svc.Store.UpsertLaws([]domain.Law{parent}); err != nil {
		t.Fatal(err)
	}
	laws, _ := svc.Store.ListLaws()
	variants, _ := svc.Store.ListVariants()
	svc.Search.Swap(laws, variants)
	seedSyncFreshMeta(t, svc, time.Now().UTC())

	_, err = svc.ExportOperativeBundle(context.Background(), "p", []string{export.FormatHierarchical}, BundleOpts{})
	if err == nil || !strings.Contains(err.Error(), "operative bundle too large") {
		t.Fatalf("err=%v want too large", err)
	}
}

func TestAggregateBundleFreshness(t *testing.T) {
	bf := aggregateBundleFreshness(
		&BundleMemberExport{
			Law:       &domain.Law{ID: "milog"},
			Freshness: &FreshnessMeta{State: domain.FreshnessUncertain},
		},
		[]OperativeMemberExport{{
			Link:      domain.LinkedInstrument{LawID: "milov5", SectionHint: "§ 1"},
			Freshness: &FreshnessMeta{State: domain.FreshnessConfirmedCurrent},
		}},
	)
	if bf.SafeToServe || bf.State != domain.FreshnessUncertain || bf.Rationale != "parent_uncertain" {
		t.Fatalf("%+v", bf)
	}
}

func TestExportOperativeBundle_validation(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	_, err := svc.ExportOperativeBundle(context.Background(), "x", []string{"nope"}, BundleOpts{})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("err=%v", err)
	}
	_, err = svc.ExportOperativeBundle(context.Background(), "x", []string{}, BundleOpts{})
	if err == nil || !strings.Contains(err.Error(), "empty format list") {
		t.Fatalf("err=%v", err)
	}
	svc.CFG.EnableExport = false
	_, err = svc.ExportOperativeBundle(context.Background(), "x", nil, BundleOpts{})
	if err == nil || !strings.Contains(err.Error(), "export disabled") {
		t.Fatalf("err=%v", err)
	}
}

func TestExportOperativeBundle_unmatched(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	res, err := svc.ExportOperativeBundle(context.Background(), "zzzznotalaw", nil, BundleOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched {
		t.Fatal("expected unmatched")
	}
}

func TestExportOperativeBundle_refuseStaleMember(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	svc.CFG.RefuseExportStale = true
	seedCatalog(t, svc, mt)
	seedSyncFreshMeta(t, svc, time.Now().UTC())
	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	_ = svc.Store.UpsertStand(stand)
	issueID := citation.IssueID(1, 2026, "999")
	_ = svc.Store.UpsertIssue(domain.GazetteIssue{ID: issueID, Teil: 1, Year: 2026, Number: "999"})
	_ = svc.Store.UpsertLink(domain.IssueLawLink{IssueID: issueID, LawID: "bgb", Class: domain.LinkConfirmed})

	_, err := svc.ExportOperativeBundle(context.Background(), "bgb", []string{export.FormatHierarchical}, BundleOpts{})
	if err == nil || err.Error() != "export refused: bundle member confirmed_stale" {
		t.Fatalf("err=%v want export refused: bundle member confirmed_stale", err)
	}
}
