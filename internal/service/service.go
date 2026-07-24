// Package service is the application service layer shared by REST, MCP, and CLI.
package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/freshness"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/sync"
)

// Service exposes core operations.
type Service struct {
	CFG         config.Config
	Store       *store.Store
	Search      *search.Engine
	Sync        *sync.Orchestrator
	HTTP        *httpx.Client
	Export      *export.Cache
	Log         *slog.Logger
	Metrics     *metrics.Registry
	Instruments *instruments.Catalog
}

// Envelope is the API response shape.
type Envelope struct {
	Success bool        `json:"success"`
	Data    any         `json:"data"`
	Error   *string     `json:"error"`
}

type Suggestion struct {
	ID           string  `json:"id"`
	Abbreviation string  `json:"abbreviation"`
	Title        string  `json:"title"`
	Score        float64 `json:"score"`
}

type FreshnessMeta struct {
	State              domain.FreshnessState     `json:"state"`
	Confidence         string                    `json:"confidence"`
	Method             domain.VerificationMethod `json:"method"`
	EvaluatedAt        time.Time                 `json:"evaluated_at"`
	LastTOCSuccess     *time.Time                `json:"last_toc_success,omitempty"`
	LastBGBlSuccess    *time.Time                `json:"last_bgbl_success,omitempty"`
	NewerIssueIDs      []string                  `json:"newer_issue_ids,omitempty"`
	Rationale          string                    `json:"rationale,omitempty"`
	Stand              *domain.StandCitation     `json:"stand,omitempty"`
	GIIURL             string                    `json:"gii_url,omitempty"`
	BGBlPointers       []string                  `json:"bgbl_pointers,omitempty"`
	LinkedInstruments  []domain.LinkedInstrument `json:"linked_instruments,omitempty"`
	InstrumentRefs     []domain.InstrumentRef    `json:"instrument_refs,omitempty"`
}

type ResolveResult struct {
	Matched     bool           `json:"matched"`
	Law         *domain.Law    `json:"law,omitempty"`
	Score       float64        `json:"score,omitempty"`
	Freshness   *FreshnessMeta `json:"freshness,omitempty"`
	Suggestions []Suggestion   `json:"suggestions,omitempty"`
	Threshold   float64        `json:"threshold"`
}

func (s *Service) Resolve(ctx context.Context, query string) (ResolveResult, error) {
	_ = ctx
	query = strings.TrimSpace(query)
	if query == "" {
		return ResolveResult{}, fmt.Errorf("query required")
	}
	snap := s.Search.Current()
	if snap == nil || len(mustLaws(s)) == 0 {
		return ResolveResult{Threshold: s.CFG.MatchThreshold}, fmt.Errorf("catalog not ready")
	}
	best, sug := snap.Resolve(query, s.CFG.MatchThreshold)
	out := ResolveResult{Threshold: s.CFG.MatchThreshold}
	for _, c := range sug {
		out.Suggestions = append(out.Suggestions, Suggestion{
			ID: c.Law.ID, Abbreviation: c.Law.Abbreviation, Title: c.Law.Title, Score: c.Score,
		})
	}
	if best == nil {
		out.Matched = false
		return out, nil
	}
	meta, err := s.freshnessFor(best.Law.ID)
	if err != nil {
		return out, err
	}
	law := best.Law
	out.Matched = true
	out.Law = &law
	out.Score = best.Score
	out.Freshness = &meta
	return out, nil
}

func mustLaws(s *Service) []domain.Law {
	laws, _ := s.Store.ListLaws()
	return laws
}

func (s *Service) Freshness(ctx context.Context, lawID string) (FreshnessMeta, error) {
	_ = ctx
	lawID = strings.TrimSpace(lawID)
	if lawID == "" {
		return FreshnessMeta{}, fmt.Errorf("law id required")
	}
	// allow abbreviation resolve
	if _, ok, _ := s.Store.GetLaw(lawID); !ok {
		if best, _ := s.Search.Current().Resolve(lawID, s.CFG.MatchThreshold); best != nil {
			lawID = best.Law.ID
		}
	}
	return s.freshnessFor(lawID)
}

func (s *Service) freshnessFor(lawID string) (FreshnessMeta, error) {
	law, ok, err := s.Store.GetLaw(lawID)
	if err != nil {
		return FreshnessMeta{}, err
	}
	if !ok {
		return FreshnessMeta{}, fmt.Errorf("law not found")
	}
	stand, _, _ := s.Store.GetStand(lawID)
	links, _ := s.Store.LinksForLaw(lawID)
	var issues []domain.GazetteIssue
	var pointers []string
	classes := map[string]domain.LinkClass{}
	for _, l := range links {
		classes[l.IssueID] = l.Class
		if iss, ok, _ := s.Store.GetIssue(l.IssueID); ok {
			issues = append(issues, iss)
			if iss.ELIURL != "" {
				pointers = append(pointers, iss.ELIURL)
			}
		}
	}
	tocT, _, _ := s.Store.GetMetaTime("last_toc_success")
	giiT, _, _ := s.Store.GetMetaTime("last_gii_feed_success")
	bgblT, bgblOK, _ := s.Store.GetMetaTime("last_bgbl_feed_success")
	eliT, eliOK, _ := s.Store.GetMetaTime("last_eli_probe_success")
	bgbl := bgblT
	probeOnly := false
	if (!bgblOK || time.Since(bgblT) > s.CFG.FreshnessMaxAge) && eliOK {
		bgbl = eliT
		probeOnly = true
	}
	instrRefs, instrIssues := instruments.CollectEvidence(s.Store, s.Instruments, lawID, stand)
	seeded := s.seededInstruments(lawID)
	rec := freshness.Evaluate(freshness.Input{
		LawID:              lawID,
		Stand:              stand,
		LinkedIssues:       issues,
		LinkClasses:        classes,
		InstrumentRefs:     instrRefs,
		InstrumentIssues:   instrIssues,
		LastTOCSuccess:     tocT,
		LastGIIFeedSuccess: giiT,
		LastBGBlSuccess:    bgbl,
		BGBlFromProbeOnly:  probeOnly,
		Now:                time.Now().UTC(),
		MaxAge:             s.CFG.FreshnessMaxAge,
	})
	_ = s.Store.PutFreshness(rec)
	meta := FreshnessMeta{
		State:             rec.State,
		Confidence:        rec.Confidence,
		Method:            rec.Method,
		EvaluatedAt:       rec.EvaluatedAt,
		NewerIssueIDs:     rec.NewerIssueIDs,
		Rationale:         rec.Rationale,
		Stand:             &stand,
		GIIURL:            law.GIIURL,
		BGBlPointers:      pointers,
		LinkedInstruments: seeded,
		InstrumentRefs:    instrRefs,
	}
	if !tocT.IsZero() {
		meta.LastTOCSuccess = &tocT
	}
	if !bgbl.IsZero() {
		meta.LastBGBlSuccess = &bgbl
	}
	return meta, nil
}

func (s *Service) seededInstruments(lawID string) []domain.LinkedInstrument {
	return instruments.ForParentSafe(s.Instruments, lawID)
}

// PersistEditorialInstruments stores +++ citation blob + fingerprint after export IR build.
func (s *Service) PersistEditorialInstruments(lawID string, ir export.IR) {
	texts := export.EditorialCitationTexts(ir)
	if len(texts) == 0 {
		return
	}
	blob := strings.Join(texts, "\n")
	if err := s.Store.SetMeta("editorial:"+lawID, blob); err != nil && s.Log != nil {
		s.Log.Warn("persist editorial instruments", "law", lawID, "err", err)
	}
	refs := export.InstrumentRefsFromIR(ir)
	if err := s.Store.SetMeta("editorial_fp:"+lawID, citation.FingerprintInstruments(refs)); err != nil && s.Log != nil {
		s.Log.Warn("persist editorial fingerprint", "law", lawID, "err", err)
	}
}

func (s *Service) ListStale(ctx context.Context) ([]domain.FreshnessRecord, error) {
	_ = ctx
	// re-evaluate quickly via stored; prefer live list after reconcile
	return s.Store.ListFreshnessByState(domain.FreshnessConfirmedStale)
}

func (s *Service) ForceRecheck(ctx context.Context, lawID string) error {
	if err := s.Sync.RunGIIFeed(ctx); err != nil {
		s.Log.Warn("force recheck gii feed", "err", err)
	}
	if err := s.Sync.RunBGBlFeeds(ctx); err != nil {
		_ = s.Sync.RunELIProbe(ctx)
	}
	if lawID != "" {
		if law, ok, _ := s.Store.GetLaw(lawID); ok {
			_ = s.Sync.RefreshStandForLaw(ctx, law)
			s.Export.InvalidateLaw(law.ID)
			// Also refresh seeded linked ordinance Stands (MiLoV4/MiLoV5, …).
			for _, li := range s.seededInstruments(lawID) {
				childID := normalize.Key(li.GIISlug)
				if child, ok, _ := s.Store.GetLaw(childID); ok {
					_ = s.Sync.RefreshStandForLaw(ctx, child)
				}
			}
		}
	}
	return s.Sync.Reconcile(ctx)
}

func (s *Service) SyncStatus(ctx context.Context) (domain.SyncStatus, error) {
	_ = ctx
	toc, tocOK, _ := s.Store.GetMetaTime("last_toc_success")
	gii, _, _ := s.Store.GetMetaTime("last_gii_feed_success")
	bgbl, _, _ := s.Store.GetMetaTime("last_bgbl_feed_success")
	eli, _, _ := s.Store.GetMetaTime("last_eli_probe_success")
	rec, _, _ := s.Store.GetMetaTime("last_reconcile_at")
	now := time.Now().UTC()
	st := domain.SyncStatus{
		CatalogReady: tocOK,
		MaxAge:       domain.DurationJSON(s.CFG.FreshnessMaxAge),
	}
	if tocOK {
		st.LastTOCSuccess = &toc
	}
	if !gii.IsZero() {
		st.LastGIIFeedSuccess = &gii
	}
	if !bgbl.IsZero() {
		st.LastBGBlFeedSuccess = &bgbl
	}
	if !eli.IsZero() {
		st.LastELIProbeSuccess = &eli
	}
	if !rec.IsZero() {
		st.LastReconcileAt = &rec
	}
	bgblOK := (!bgbl.IsZero() && now.Sub(bgbl) <= s.CFG.FreshnessMaxAge) ||
		(!eli.IsZero() && now.Sub(eli) <= s.CFG.FreshnessMaxAge)
	st.DataFresh = tocOK && now.Sub(toc) <= s.CFG.FreshnessMaxAge && bgblOK
	return st, nil
}

// CollectMetrics refreshes scrape-time gauges into reg (typically the shared Registry).
func (s *Service) CollectMetrics(reg *metrics.Registry) {
	if s == nil || reg == nil || s.Store == nil {
		return
	}
	st, err := s.SyncStatus(context.Background())
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("metrics collect SyncStatus failed", "err", err)
		}
		return
	}
	ready, fresh := 0.0, 0.0
	if st.CatalogReady {
		ready = 1
	}
	if st.DataFresh {
		fresh = 1
	}
	_ = reg.SetGauge(metrics.MetricCatalogReady, nil, ready)
	_ = reg.SetGauge(metrics.MetricDataFresh, nil, fresh)

	setTS := func(source string, t *time.Time) {
		v := 0.0
		if t != nil && !t.IsZero() {
			v = float64(t.Unix())
		}
		_ = reg.SetGauge(metrics.MetricSyncLastSuccess, map[string]string{"source": source}, v)
	}
	setTS("toc", st.LastTOCSuccess)
	setTS("gii_feed", st.LastGIIFeedSuccess)
	setTS("bgbl_feed", st.LastBGBlFeedSuccess)
	setTS("eli_probe", st.LastELIProbeSuccess)
	setTS("reconcile", st.LastReconcileAt)

	counts, err := s.Store.CountFreshnessByState()
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("metrics collect CountFreshnessByState failed", "err", err)
		}
		return
	}
	for state, n := range counts {
		_ = reg.SetGauge(metrics.MetricFreshnessLaws, map[string]string{"state": string(state)}, float64(n))
	}
}

type ExportResult struct {
	Matched             bool              `json:"matched"`
	Law                 *domain.Law       `json:"law,omitempty"`
	Freshness           *FreshnessMeta    `json:"freshness,omitempty"`
	Formats             map[string]any    `json:"formats,omitempty"`
	StructuralAmbiguity bool              `json:"structural_ambiguity"`
	Suggestions         []Suggestion      `json:"suggestions,omitempty"`
	UnitIDs             []string          `json:"unit_ids,omitempty"`
}

func (s *Service) ExportText(ctx context.Context, queryOrID string, formats []string) (ExportResult, error) {
	if !s.CFG.EnableExport {
		return ExportResult{}, fmt.Errorf("export disabled")
	}
	if formats == nil {
		formats = []string{export.FormatHierarchical}
	}
	if len(formats) == 0 {
		return ExportResult{}, fmt.Errorf("empty format list")
	}
	for _, f := range formats {
		switch f {
		case export.FormatHierarchical, export.FormatChunked, export.FormatFlat, export.FormatNormtext:
		default:
			return ExportResult{}, fmt.Errorf("unknown format %q", f)
		}
	}
	res, err := s.Resolve(ctx, queryOrID)
	if err != nil {
		// try direct id
		if law, ok, _ := s.Store.GetLaw(queryOrID); ok {
			meta, err2 := s.freshnessFor(law.ID)
			if err2 != nil {
				return ExportResult{}, err2
			}
			return s.exportLaw(ctx, law, meta, formats)
		}
		return ExportResult{}, err
	}
	if !res.Matched || res.Law == nil {
		return ExportResult{Matched: false, Suggestions: res.Suggestions}, nil
	}
	return s.exportLaw(ctx, *res.Law, *res.Freshness, formats)
}

func (s *Service) exportLaw(ctx context.Context, law domain.Law, meta FreshnessMeta, formats []string) (ExportResult, error) {
	stand := domain.StandCitation{}
	if meta.Stand != nil {
		stand = *meta.Stand
	}

	var xmlData []byte
	haveXML := false
	ensureXML := func() error {
		if haveXML {
			return nil
		}
		body, err := s.fetchLawXML(ctx, law)
		if err != nil {
			return err
		}
		xmlData = body
		haveXML = true
		return nil
	}

	// Prefer standangabe from export XML when store Stand is empty, then re-evaluate freshness
	// before the RefuseExportStale gate so state and Stand stay consistent.
	if stand.Raw == "" {
		if err := ensureXML(); err == nil {
			if raw := export.ExtractStandRaw(xmlData); raw != "" {
				parsed := citation.Parse(law.ID, raw)
				_ = s.Store.UpsertStand(parsed)
				newMeta, err := s.freshnessFor(law.ID)
				if err != nil {
					return ExportResult{}, err
				}
				meta = newMeta
				if meta.Stand != nil {
					stand = *meta.Stand
				} else {
					stand = parsed
					meta.Stand = &stand
				}
			}
		}
	}

	if s.CFG.RefuseExportStale && meta.State == domain.FreshnessConfirmedStale {
		return ExportResult{}, fmt.Errorf("export refused: law confirmed_stale")
	}

	contentID := export.ContentIDFromStand(stand)
	ir, ok := s.Export.Get(law.ID, contentID)
	if ok {
		_ = s.Metrics.IncCounter(metrics.MetricExportCacheLookups, map[string]string{"result": "hit"}, 1)
	} else {
		_ = s.Metrics.IncCounter(metrics.MetricExportCacheLookups, map[string]string{"result": "miss"}, 1)
		if err := ensureXML(); err != nil {
			return ExportResult{Matched: true, Law: &law, Freshness: &meta}, err
		}
		built, err := export.BuildIR(law, contentID, xmlData)
		if err != nil {
			return ExportResult{}, err
		}
		ir = built
		s.Export.Put(ir)
	}
	s.PersistEditorialInstruments(law.ID, ir)
	if newMeta, err := s.freshnessFor(law.ID); err == nil {
		meta = newMeta
		if meta.Stand != nil {
			stand = *meta.Stand
		}
	}
	out := ExportResult{
		Matched:             true,
		Law:                 &law,
		Freshness:           &meta,
		StructuralAmbiguity: ir.StructuralAmbiguity,
		Formats:             map[string]any{},
		UnitIDs:             export.UnitIDs(ir),
	}
	for _, f := range formats {
		switch f {
		case export.FormatHierarchical:
			out.Formats[f] = export.EmitHierarchical(ir)
		case export.FormatFlat:
			out.Formats[f] = export.EmitFlat(ir)
		case export.FormatChunked:
			rec := domain.FreshnessRecord{State: meta.State}
			out.Formats[f] = export.EmitChunked(ir, stand, rec)
		case export.FormatNormtext:
			rec := domain.FreshnessRecord{State: meta.State}
			out.Formats[f] = export.EmitNormtext(ir, stand, rec)
		}
	}
	return out, nil
}

func (s *Service) fetchLawXML(ctx context.Context, law domain.Law) ([]byte, error) {
	url, err := giiurl.XMLZip(s.CFG.GIIBase, law.GIIPath)
	if err != nil {
		return nil, err
	}
	body, _, status, err := s.HTTP.Get(ctx, url, "", "")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("xml.zip status %d", status)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		// maybe raw xml
		if bytes.Contains(body, []byte("<")) {
			return body, nil
		}
		return nil, err
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 16<<20))
		rc.Close()
		if err != nil {
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("no xml in zip")
}
