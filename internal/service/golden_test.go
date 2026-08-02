package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

// seedFreshSync marks TOC, GII, and BGBl feeds as recently successful (happy-path baseline).
func seedFreshSync(t *testing.T, svc *Service, now time.Time) {
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

func TestGolden_ResolveBGB_confirmedCurrent(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())

	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	if !stand.ParseOK {
		t.Fatalf("stand parse failed: %+v", stand)
	}
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Resolve(context.Background(), "bgb", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Law == nil {
		t.Fatalf("matched=%v law=%v", res.Matched, res.Law)
	}
	if res.Law.ID != "bgb" || res.Law.Abbreviation != "BGB" {
		t.Fatalf("law=%+v", res.Law)
	}
	if res.Freshness == nil {
		t.Fatal("expected freshness envelope")
	}
	f := res.Freshness
	if f.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("state=%s want confirmed_current", f.State)
	}
	if f.Confidence != "high" {
		t.Fatalf("confidence=%q want high", f.Confidence)
	}
	if f.Method != domain.MethodFeeds {
		t.Fatalf("method=%q want feeds", f.Method)
	}
	if f.Rationale != "no newer linked gazette issue beyond Stand" {
		t.Fatalf("rationale=%q", f.Rationale)
	}
	if f.Stand == nil || !f.Stand.ParseOK {
		t.Fatalf("stand=%+v want parse_ok", f.Stand)
	}
	if f.Stand.Year != 2023 || f.Stand.Teil != 1 || f.Stand.Number != "198" {
		t.Fatalf("stand fields=%+v", f.Stand)
	}
}

func TestGolden_MiLoG_includeLinked_linkedInstruments(t *testing.T) {
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

	seedFreshSync(t, svc, time.Now().UTC())
	stand := citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
		Title: "Fünfte Mindestlohnanpassungsverordnung",
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Resolve(context.Background(), "milog", IncludeOpts{Linked: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Freshness == nil {
		t.Fatalf("matched=%v freshness=%v", res.Matched, res.Freshness)
	}
	meta := res.Freshness
	if len(meta.LinkedInstruments) != 1 {
		t.Fatalf("linked len=%d want 1: %+v", len(meta.LinkedInstruments), meta.LinkedInstruments)
	}
	li := meta.LinkedInstruments[0]
	if li.GIISlug != "milov5" {
		t.Fatalf("slug=%q want milov5", li.GIISlug)
	}
	if li.EffectiveFrom != "2026-01-01" {
		t.Fatalf("effective_from=%q want 2026-01-01", li.EffectiveFrom)
	}
	if li.SectionHint != "§ 1" {
		t.Fatalf("section_hint=%q want § 1", li.SectionHint)
	}
	if li.Status != instruments.StatusCurrent {
		t.Fatalf("status=%q want current", li.Status)
	}
	if !li.ResolveOK || li.LawID != "milov5" {
		t.Fatalf("linked pointers: resolve_ok=%v law_id=%q", li.ResolveOK, li.LawID)
	}
}

func TestGolden_BGB_confirmedStale_newerLinkedIssue(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())

	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}
	issueID := citation.IssueID(1, 2025, "42")
	if err := svc.Store.UpsertIssue(domain.GazetteIssue{
		ID: issueID, Teil: 1, Year: 2025, Number: "42", Title: "Neueres Gesetz",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.UpsertLink(domain.IssueLawLink{
		IssueID: issueID, LawID: "bgb", Class: domain.LinkConfirmed,
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := svc.Freshness(context.Background(), "bgb", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessConfirmedStale {
		t.Fatalf("state=%s want confirmed_stale", meta.State)
	}
	if meta.Rationale != "newer gazette issue linked than reflected Stand" {
		t.Fatalf("rationale=%q", meta.Rationale)
	}
	if len(meta.NewerIssueIDs) != 1 || meta.NewerIssueIDs[0] != issueID {
		t.Fatalf("newer_issue_ids=%v want [%s]", meta.NewerIssueIDs, issueID)
	}
}

func TestGolden_ArbZG_uncertain_missingStand(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())

	meta, err := svc.Freshness(context.Background(), "arbzg", IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != domain.FreshnessUncertain {
		t.Fatalf("state=%s want uncertain", meta.State)
	}
	if meta.Rationale != "stand_unparsed_or_missing" {
		t.Fatalf("rationale=%q want stand_unparsed_or_missing", meta.Rationale)
	}
	if meta.Confidence != "low" {
		t.Fatalf("confidence=%q want low", meta.Confidence)
	}
}

func TestGolden_ExportBGB_hierarchical(t *testing.T) {
	mt := httpmock.New()
	svc := newTestService(t, mt)
	seedCatalog(t, svc, mt)
	seedFreshSync(t, svc, time.Now().UTC())

	stand := citation.Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198")
	if err := svc.Store.UpsertStand(stand); err != nil {
		t.Fatal(err)
	}

	xmlBody := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE dokument SYSTEM "https://www.gesetze-im-internet.de/dtd/1.01/gii-norm.dtd">
<dokument>
  <norm builddate="20240115120000" doknr="BJNR000000000BJNE000100000">
    <metadaten>
      <jurabk>BGB</jurabk>
      <langue>Bürgerliches Gesetzbuch</langue>
      <standangabe>
        <standtyp>Stand</standtyp>
        <standkommentar>Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. I Nr. 198</standkommentar>
      </standangabe>
    </metadaten>
    <textdaten><text><Content><P>BGB</P></Content></text></textdaten>
  </norm>
  <norm builddate="20240115120001" doknr="BJNR000000000BJNE000101000">
    <metadaten><jurabk>BGB</jurabk><enbez>§ 1</enbez><titel format="parat">Geltungsbereich</titel></metadaten>
    <textdaten><text><Content><P nr="1">Dieses Gesetz gilt im gesamten Bundesgebiet.</P></Content></text></textdaten>
  </norm>
</dokument>`)
	mt.SetBytes("www.gesetze-im-internet.de", "/bgb/xml.zip", fixtures.MustZipXML("bgb.xml", xmlBody))

	res, err := svc.ExportText(context.Background(), "bgb", []string{export.FormatHierarchical}, IncludeOpts{}, ExportGateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.Law == nil {
		t.Fatalf("matched=%v law=%v", res.Matched, res.Law)
	}
	if res.Law.ID != "bgb" {
		t.Fatalf("law id=%q", res.Law.ID)
	}
	if res.Freshness == nil {
		t.Fatal("expected freshness on export")
	}
	if res.Freshness.State != domain.FreshnessConfirmedCurrent {
		t.Fatalf("freshness state=%s want confirmed_current", res.Freshness.State)
	}
	h, ok := res.Formats[export.FormatHierarchical].(string)
	if !ok {
		t.Fatalf("hierarchical type %T", res.Formats[export.FormatHierarchical])
	}
	if h == "" {
		t.Fatal("expected non-empty hierarchical export")
	}
	if len(res.UnitIDs) == 0 {
		t.Fatal("expected non-empty unit_ids")
	}
}
