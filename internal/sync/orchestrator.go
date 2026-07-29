// Package sync implements background catalog and BGBl observation jobs.
package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/discovery"
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
	"github.com/Squarenix17/gesetzeswache/internal/xmlsafe"
)

// Orchestrator runs independent sync jobs.
type Orchestrator struct {
	CFG         config.Config
	Store       *store.Store
	HTTP        *httpx.Client
	Search      *search.Engine
	Log         *slog.Logger
	Metrics     *metrics.Registry
	Instruments *instruments.Catalog
	Families    *instruments.FamilyCatalog

	wg sync.WaitGroup
}

type tocItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
	Abbr  string `xml:"jurabk"` // may be absent in some TOC shapes
}

// GII TOC root is <items><item>...</item></items>
type tocRoot struct {
	XMLName xml.Name  `xml:"items"`
	Items   []tocItem `xml:"item"`
}

// RunTOC refreshes law catalog from GII TOC and rebuilds search snapshot.
func (o *Orchestrator) RunTOC(ctx context.Context) error {
	start := time.Now().UTC()
	body, _, status, err := o.HTTP.Get(ctx, o.CFG.GIITOCURL, "", "")
	a := domain.SyncAttempt{Source: "toc", StartedAt: start}
	defer func() {
		a.EndedAt = time.Now().UTC()
		_ = o.Store.AppendSyncAttempt(a)
		o.recordSyncJob(a)
	}()
	if err != nil {
		a.Error = err.Error()
		return err
	}
	if status >= 400 {
		a.Error = fmt.Sprintf("status %d", status)
		return fmt.Errorf("toc status %d", status)
	}
	if err := xmlsafe.RejectUnsafeXML(body); err != nil {
		a.Error = err.Error()
		return err
	}
	var root tocRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		a.Error = err.Error()
		return err
	}
	laws := make([]domain.Law, 0, len(root.Items))
	seen := map[string]struct{}{}
	for _, it := range root.Items {
		title := strings.TrimSpace(it.Title)
		link := strings.TrimSpace(it.Link)
		if title == "" || link == "" {
			continue
		}
		abbr := strings.TrimSpace(it.Abbr)
		slug := giiurl.SlugFromTOCLink(link)
		if slug == "" {
			continue
		}
		if abbr == "" {
			abbr = guessAbbr(title, slug)
		}
		id := normalize.Key(abbr)
		if id == "" {
			id = normalize.Key(slug)
		}
		if _, dup := seen[id]; dup {
			id = normalize.Key(slug)
		}
		seen[id] = struct{}{}
		indexURL, err := giiurl.IndexURL(o.CFG.GIIBase, slug)
		if err != nil {
			continue
		}
		laws = append(laws, domain.Law{
			ID:           id,
			Abbreviation: abbr,
			Title:        title,
			GIIPath:      slug,
			GIIURL:       indexURL,
		})
	}
	if err := o.Store.UpsertLaws(laws); err != nil {
		a.Error = err.Error()
		return err
	}
	variants, _ := o.Store.ListVariants()
	o.Search.Swap(laws, variants)
	if err := o.stampSuccessMeta("last_toc_success", time.Now().UTC()); err != nil {
		a.Error = err.Error()
		return err
	}
	a.Success = true
	a.Detail = fmt.Sprintf("%d laws", len(laws))
	o.Log.Info("toc sync ok", "laws", len(laws))
	return nil
}

func guessAbbr(title, slug string) string {
	// title kept for signature compatibility with TOC ingest (future jurabk/title hints).
	s := strings.TrimLeft(slug, "_")
	if s == "" {
		return slug
	}
	// Prefer short slug uppercase (GII paths without TOC jurabk).
	if len(s) <= 12 {
		return strings.ToUpper(s)
	}
	return s
}

// RSS generic
type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

var (
	reBGBlInText = regexp.MustCompile(`(?i)BGBl\.?\s*(?:([I12]|II)\s*)?(?:(\d{4})\s*)?(?:Nr\.?\s*)?([0-9]+[a-zA-Z]?)`)
	reLawAbbr    = regexp.MustCompile(`\(([A-Za-zÄÖÜäöüß0-9\-]{1,20})\)`)
)

// RunGIIFeed ingests Aktualitätendienst for issue↔law links.
func (o *Orchestrator) RunGIIFeed(ctx context.Context) error {
	start := time.Now().UTC()
	a := domain.SyncAttempt{Source: "gii_feed", StartedAt: start}
	defer func() {
		a.EndedAt = time.Now().UTC()
		_ = o.Store.AppendSyncAttempt(a)
		o.recordSyncJob(a)
	}()
	body, _, status, err := o.HTTP.Get(ctx, o.CFG.GIIFeedURL, "", "")
	if err != nil {
		a.Error = err.Error()
		return err
	}
	if status >= 400 {
		a.Error = fmt.Sprintf("status %d", status)
		return fmt.Errorf("gii feed status %d", status)
	}
	if err := xmlsafe.RejectUnsafeXML(body); err != nil {
		a.Error = err.Error()
		return err
	}
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		a.Error = err.Error()
		return err
	}
	n := 0
	var storeErr error
	for _, it := range feed.Channel.Items {
		teil, year, num := parseBGBlRef(it.Title + " " + it.Description)
		if year == 0 || num == "" {
			continue
		}
		if teil == 0 {
			teil = 1
		}
		id := citation.IssueID(teil, year, num)
		iss := domain.GazetteIssue{
			ID:                  id,
			Teil:                teil,
			Year:                year,
			Number:              num,
			Title:               it.Title,
			ELIURL:              eliURL(o.CFG.ELIBase, teil, year, num),
			DiscoverySources:    []string{"gii_feed"},
			ExistenceConfidence: "high",
			FirstSeenAt:         time.Now().UTC(),
		}
		if pub := parseRSSDate(it.PubDate); pub != nil {
			iss.PublishedAt = pub
		}
		existing, ok, _ := o.Store.GetIssue(id)
		if ok {
			iss.FirstSeenAt = existing.FirstSeenAt
			iss.DiscoverySources = mergeSources(existing.DiscoverySources, "gii_feed")
		}
		storeErr = firstStoreErr(storeErr, o.Store.UpsertIssue(iss))

		lawID := matchLawFromItem(it, o.Search.Current())
		if lawID != "" {
			storeErr = firstStoreErr(storeErr, o.Store.UpsertLink(domain.IssueLawLink{
				IssueID:   id,
				LawID:     lawID,
				Class:     domain.LinkConfirmed,
				CreatedAt: time.Now().UTC(),
			}))
			iss.Matched = true
			storeErr = firstStoreErr(storeErr, o.Store.UpsertIssue(iss))
			if law, ok, _ := o.Store.GetLaw(lawID); ok && discovery.LooksLikeVerordnung(law) {
				storeErr = firstStoreErr(storeErr, o.Store.SetMeta("discovery_queue:"+law.ID, "1"))
			}
			n++
		}
	}
	if storeErr != nil {
		a.Error = storeErr.Error()
		o.recordStoreWriteFailure("gii_feed")
		return storeErr
	}
	if err := o.stampSuccessMeta("last_gii_feed_success", time.Now().UTC()); err != nil {
		a.Error = err.Error()
		return err
	}
	a.Success = true
	a.Detail = fmt.Sprintf("linked %d", n)
	return nil
}

// RunBGBlFeeds records official issue existence from recht.bund.de feeds.
func (o *Orchestrator) RunBGBlFeeds(ctx context.Context) error {
	start := time.Now().UTC()
	a := domain.SyncAttempt{Source: "bgbl_feed", StartedAt: start}
	defer func() {
		a.EndedAt = time.Now().UTC()
		_ = o.Store.AppendSyncAttempt(a)
		o.recordSyncJob(a)
	}()
	count := 0
	var storeErr error
	var feedErr error
	for feedIdx, url := range []string{o.CFG.BGBlFeed1URL, o.CFG.BGBlFeed2URL} {
		body, _, status, err := o.HTTP.Get(ctx, url, "", "")
		if err != nil {
			o.Log.Warn("bgbl feed fetch failed", "url", url, "err", err)
			feedErr = firstStoreErr(feedErr, fmt.Errorf("feed %d fetch: %w", feedIdx+1, err))
			continue
		}
		if status >= 400 {
			o.Log.Warn("bgbl feed status", "url", url, "status", status)
			feedErr = firstStoreErr(feedErr, fmt.Errorf("feed %d status %d", feedIdx+1, status))
			continue
		}
		if err := xmlsafe.RejectUnsafeXML(body); err != nil {
			o.Log.Warn("bgbl feed unsafe xml", "err", err)
			feedErr = firstStoreErr(feedErr, fmt.Errorf("feed %d unsafe xml: %w", feedIdx+1, err))
			continue
		}
		var feed rssFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			o.Log.Warn("bgbl feed parse", "err", err)
			feedErr = firstStoreErr(feedErr, fmt.Errorf("feed %d parse: %w", feedIdx+1, err))
			continue
		}
		for _, it := range feed.Channel.Items {
			teil, year, num := parseBGBlRef(it.Title + " " + it.Link)
			if year == 0 || num == "" {
				teil, year, num = parseELI(it.Link)
			}
			if year == 0 || num == "" {
				continue
			}
			if teil == 0 {
				teil = 1
			}
			id := citation.IssueID(teil, year, num)
			iss := domain.GazetteIssue{
				ID:                  id,
				Teil:                teil,
				Year:                year,
				Number:              num,
				Title:               it.Title,
				ELIURL:              eliURL(o.CFG.ELIBase, teil, year, num),
				DiscoverySources:    []string{"bgbl_feed"},
				ExistenceConfidence: "high",
				FirstSeenAt:         time.Now().UTC(),
			}
			if pub := parseRSSDate(it.PubDate); pub != nil {
				iss.PublishedAt = pub
			}
			existing, ok, _ := o.Store.GetIssue(id)
			if ok {
				iss.FirstSeenAt = existing.FirstSeenAt
				iss.Matched = existing.Matched
				iss.DiscoverySources = mergeSources(existing.DiscoverySources, "bgbl_feed")
			}
			storeErr = firstStoreErr(storeErr, o.Store.UpsertIssue(iss))
			count++
		}
	}
	if feedErr != nil {
		a.Error = feedErr.Error()
		o.markBGBlFeedDegraded(time.Now().UTC())
		return feedErr
	}
	if count == 0 {
		a.Error = "no items parsed from bgbl feeds"
		return fmt.Errorf("%s", a.Error)
	}
	if storeErr != nil {
		a.Error = storeErr.Error()
		o.recordStoreWriteFailure("bgbl_feed")
		return storeErr
	}
	o.clearBGBlFeedDegraded()
	if err := o.stampSuccessMeta("last_bgbl_feed_success", time.Now().UTC()); err != nil {
		a.Error = err.Error()
		return err
	}
	a.Success = true
	a.Detail = fmt.Sprintf("%d issues touched", count)
	return nil
}

// RunELIProbe checks the next expected ELI permalinks (existence-only, low confidence).
func (o *Orchestrator) RunELIProbe(ctx context.Context) error {
	start := time.Now().UTC()
	a := domain.SyncAttempt{Source: "eli_probe", StartedAt: start}
	defer func() {
		a.EndedAt = time.Now().UTC()
		_ = o.Store.AppendSyncAttempt(a)
		o.recordSyncJob(a)
	}()
	year := time.Now().UTC().Year()
	found := 0
	anyHTTPSuccess := false
	var storeErr error
	for _, teil := range []int{1, 2} {
		maxN := latestNumber(o.Store, teil, year)
		for n := maxN; n <= maxN+3; n++ {
			num := strconv.Itoa(n)
			url := eliURL(o.CFG.ELIBase, teil, year, num)
			ok, _, err := o.HTTP.Exists(ctx, url)
			if err != nil {
				continue
			}
			anyHTTPSuccess = true
			if !ok {
				continue
			}
			id := citation.IssueID(teil, year, num)
			iss := domain.GazetteIssue{
				ID:                  id,
				Teil:                teil,
				Year:                year,
				Number:              num,
				ELIURL:              url,
				DiscoverySources:    []string{"eli_probe"},
				ExistenceConfidence: "low",
				FirstSeenAt:         time.Now().UTC(),
			}
			existing, exists, _ := o.Store.GetIssue(id)
			if exists {
				iss.FirstSeenAt = existing.FirstSeenAt
				iss.Matched = existing.Matched
				iss.DiscoverySources = mergeSources(existing.DiscoverySources, "eli_probe")
				if existing.ExistenceConfidence == "high" {
					iss.ExistenceConfidence = "high"
				}
			}
			storeErr = firstStoreErr(storeErr, o.Store.UpsertIssue(iss))
			found++
		}
	}
	if !anyHTTPSuccess {
		a.Error = "all eli probe HTTP requests failed"
		return fmt.Errorf("%s", a.Error)
	}
	if storeErr != nil {
		a.Error = storeErr.Error()
		o.recordStoreWriteFailure("eli_probe")
		return storeErr
	}
	if err := o.stampSuccessMeta("last_eli_probe_success", time.Now().UTC()); err != nil {
		a.Error = err.Error()
		return err
	}
	a.Success = true
	a.Detail = fmt.Sprintf("probed hits %d", found)
	return nil
}

func (o *Orchestrator) recordSyncJob(a domain.SyncAttempt) {
	result := "error"
	if a.Success {
		result = "success"
	}
	_ = o.Metrics.IncCounter(metrics.MetricSyncJobsTotal, map[string]string{
		"source": a.Source,
		"result": result,
	}, 1)
}

func latestNumber(st *store.Store, teil, year int) int {
	issues, err := st.ListIssues()
	if err != nil {
		return 1
	}
	max := 1
	for _, iss := range issues {
		if iss.Teil == teil && iss.Year == year {
			n := 0
			for _, r := range iss.Number {
				if r < '0' || r > '9' {
					break
				}
				n = n*10 + int(r-'0')
			}
			if n > max {
				max = n
			}
		}
	}
	return max
}

// Reconcile updates freshness for all laws and optionally heuristic-links unmatched issues.
func (o *Orchestrator) Reconcile(ctx context.Context) error {
	now := time.Now().UTC()
	tocT, _, _ := o.Store.GetMetaTime("last_toc_success")
	giiT, _, _ := o.Store.GetMetaTime("last_gii_feed_success")
	bgblT, _, _ := o.Store.GetMetaTime("last_bgbl_feed_success")
	bgblDegraded, _, _ := o.Store.GetMetaTime(metaKeyBGBlFeedDegraded)
	bgblT = freshness.EffectiveBGBlFeedTime(bgblT, bgblDegraded)
	eliT, _, _ := o.Store.GetMetaTime("last_eli_probe_success")
	bgblSuccess, probeOnly := freshness.BGBLEvidence(bgblT, eliT, now, o.CFG.FreshnessMaxAge)

	if o.CFG.EnableHeuristic {
		o.heuristicLink(now)
	}

	laws, err := o.Store.ListLaws()
	if err != nil {
		return err
	}
	for _, law := range laws {
		if err := ctx.Err(); err != nil {
			return err
		}
		stand, _, _ := o.Store.GetStand(law.ID)
		if !stand.ParseOK && strings.TrimSpace(stand.Raw) != "" {
			if parsed := citation.Parse(law.ID, stand.Raw); parsed.ParseOK {
				_ = o.Store.UpsertStand(parsed)
				stand = parsed
			}
		}
		links, linksErr := o.Store.LinksForLaw(law.ID)
		if linksErr != nil && o.Log != nil {
			o.Log.Warn("links read failed", "law", law.ID, "err", linksErr)
		}
		var issues []domain.GazetteIssue
		classes := map[string]domain.LinkClass{}
		for _, l := range links {
			classes[l.IssueID] = l.Class
			if iss, ok, _ := o.Store.GetIssue(l.IssueID); ok {
				issues = append(issues, iss)
			}
		}
		seeded := instruments.ForParentSafe(o.Instruments, law.ID)
		var familyExpanded []domain.LinkedInstrument
		if o.Families != nil {
			familyExpanded = instruments.ExpandForParentSafe(o.Families, law.ID, laws)
			seeded = append(seeded, familyExpanded...)
		}
		edges, discErr := o.Store.DiscoveredForParent(law.ID)
		if discErr != nil && o.Log != nil {
			o.Log.Warn("discovered links read failed", "law", law.ID, "err", discErr)
		}
		var disc []domain.LinkedInstrument
		for _, e := range edges {
			disc = append(disc, discovery.EdgeToLinked(e))
		}
		if o.Families != nil {
			rows := o.Families.ForParent(law.ID)
			if len(rows) > 0 {
				prefixes := make([]string, 0, len(rows))
				for _, row := range rows {
					prefixes = append(prefixes, row.SlugPrefix)
				}
				keepSlugs := make(map[string]struct{}, len(familyExpanded))
				for _, li := range familyExpanded {
					keepSlugs[li.GIISlug] = struct{}{}
				}
				disc = discovery.FilterDiscoveredByFamilyPrefixes(disc, prefixes, keepSlugs)
			}
		}
		linked := discovery.Merge(seeded, disc)
		operativeLinked := instruments.FilterOperativeLinked(o.Store, linked)
		instrRefs, instrIssues := instruments.CollectEvidence(o.Store, operativeLinked, law.ID, stand)
		hasLinked := len(operativeLinked) > 0 || discErr != nil
		rec := freshness.Evaluate(freshness.Input{
			LawID:                      law.ID,
			Stand:                      stand,
			LinkedIssues:               issues,
			LinkClasses:                classes,
			InstrumentRefs:             instrRefs,
			InstrumentIssues:           instrIssues,
			HasSeededLinkedInstruments: hasLinked,
			LinksReadFailed:            linksErr != nil || discErr != nil,
			LastTOCSuccess:             tocT,
			LastGIIFeedSuccess:         giiT,
			LastBGBlSuccess:            bgblSuccess,
			BGBlFromProbeOnly:          probeOnly,
			Now:                        now,
			MaxAge:                     o.CFG.FreshnessMaxAge,
		})
		_ = o.Store.PutFreshness(rec)
	}
	_ = o.Store.SetMetaTime("last_reconcile_at", now)
	return nil
}

func (o *Orchestrator) heuristicLink(now time.Time) {
	issues, _ := o.Store.ListIssues()
	snap := o.Search.Current()
	if snap == nil {
		return
	}
	for _, iss := range issues {
		if iss.Matched {
			continue
		}
		if now.Sub(iss.FirstSeenAt) < o.CFG.UnmatchedGrace {
			continue // still in grace — do not heuristic yet
		}
		best, _ := snap.Resolve(iss.Title, o.CFG.MatchThreshold)
		if best == nil {
			continue
		}
		_ = o.Store.UpsertLink(domain.IssueLawLink{
			IssueID:   iss.ID,
			LawID:     best.Law.ID,
			Class:     domain.LinkHeuristic,
			CreatedAt: now,
		})
		iss.Matched = true
		_ = o.Store.UpsertIssue(iss)
		o.Log.Info("heuristic link", "issue", iss.ID, "law", best.Law.ID, "score", best.Score)
	}
}

// RefreshStandForLaw fetches Stand from the law HTML index page; if absent, falls back to
// standangabe in the GII export XML. URLs are always rebuilt from config base + validated slug.
func (o *Orchestrator) RefreshStandForLaw(ctx context.Context, law domain.Law) error {
	lookup, err := o.catalogLookup()
	if err != nil {
		return err
	}
	return o.refreshStandForLaw(ctx, law, lookup)
}

func (o *Orchestrator) refreshStandForLaw(ctx context.Context, law domain.Law, lookup discovery.CatalogLookup) error {
	if law.GIIPath == "" {
		return nil
	}
	url, err := giiurl.IndexURL(o.CFG.GIIBase, law.GIIPath)
	if err != nil {
		return err
	}
	body, _, status, err := o.HTTP.Get(ctx, url, "", "")
	htmlFailed := err != nil || status >= 400
	if err != nil {
		// fall through to XML attempt
	} else if status >= 400 {
		err = fmt.Errorf("stand fetch status %d", status)
	}
	var raw string
	if !htmlFailed {
		raw = extractStand(string(body))
	}
	var xmlData []byte
	if raw == "" {
		var xerr error
		xmlData, xerr = o.fetchLawXML(ctx, law)
		if xerr != nil {
			if htmlFailed {
				if o.Log != nil {
					o.Log.Debug("stand refresh failed", "law", law.ID, "html_err", err, "xml_err", xerr)
				}
				return fmt.Errorf("stand refresh: html and xml both failed: html=%v xml=%v", err, xerr)
			}
			if o.Log != nil {
				o.Log.Debug("stand xml fallback failed", "law", law.ID, "err", xerr)
			}
			return nil
		}
		raw = export.ExtractStandRaw(xmlData)
	}
	if raw == "" && xmlData == nil {
		return nil
	}
	if raw != "" {
		c := citation.Parse(law.ID, raw)
		if err := o.Store.UpsertStand(c); err != nil {
			return err
		}
	}
	// Persist +++ instrument citations from XML when available.
	if xmlData == nil {
		if xd, xerr := o.fetchLawXML(ctx, law); xerr == nil {
			xmlData = xd
		}
	}
	if len(xmlData) > 0 {
		ir, err := export.BuildIR(law, "instruments", xmlData)
		if err == nil {
			texts := export.EditorialCitationTexts(ir)
			if len(texts) > 0 {
				if err := o.Store.SetMeta("editorial:"+law.ID, strings.Join(texts, "\n")); err != nil && o.Log != nil {
					o.Log.Warn("persist editorial", "law", law.ID, "err", err)
				}
			}
		}
		if o.CFG.DiscoveryEnabled {
			if _, err := discovery.IngestLawXML(o.Store, lookup, law, xmlData); err != nil {
				if o.Log != nil {
					o.Log.Warn("discovery ingest", "law", law.ID, "err", err)
				}
			}
		}
	}
	return nil
}

// RefreshMissingStands refreshes Stand for up to max laws that have no stored Stand citation.
// max <= 0 skips the bulk pass (startup stays cheap).
func (o *Orchestrator) RefreshMissingStands(ctx context.Context, max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	laws, err := o.Store.ListLaws()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, law := range laws {
		if n >= max {
			break
		}
		if _, ok, _ := o.Store.GetStand(law.ID); ok {
			continue
		}
		if err := o.RefreshStandForLaw(ctx, law); err != nil {
			o.Log.Warn("stand refresh", "law", law.ID, "err", err)
			o.recordStandRefreshFailure()
			continue
		}
		if _, ok, _ := o.Store.GetStand(law.ID); ok {
			n++
		}
	}
	return n, nil
}

// DiscoverOrdinances ingests Ermächtigung edges for Verordnung candidates up to max laws.
func (o *Orchestrator) DiscoverOrdinances(ctx context.Context, max int) (int, error) {
	if max <= 0 || !o.CFG.DiscoveryEnabled {
		return 0, nil
	}
	laws, err := o.Store.ListLaws()
	if err != nil {
		return 0, err
	}
	candidates := o.discoveryCandidates(laws)
	lookup, err := o.catalogLookup()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, law := range candidates {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if n >= max {
			break
		}
		if ingested, _, _ := o.Store.GetMeta("discovery_ingested:" + law.ID); ingested == "1" {
			continue
		}
		if err := o.refreshStandForLaw(ctx, law, lookup); err != nil {
			o.Log.Warn("discovery refresh", "law", law.ID, "err", err)
			o.recordStandRefreshFailure()
			continue
		}
		ingestOK := false
		if o.CFG.DiscoveryEnabled {
			ok, ierr := o.discoveryIngestLaw(ctx, law, lookup)
			if ierr != nil {
				o.recordDiscoveryIngest("error")
				continue
			}
			o.recordDiscoveryIngest("success")
			ingestOK = ok
		}
		if !ingestOK {
			continue
		}
		_ = o.Store.SetMeta("discovery_ingested:"+law.ID, "1")
		_ = o.Store.SetMeta("discovery_queue:"+law.ID, "")
		n++
	}
	return n, nil
}

func (o *Orchestrator) discoveryCandidates(laws []domain.Law) []domain.Law {
	var queued, rest []domain.Law
	for _, law := range laws {
		if !discovery.LooksLikeVerordnung(law) {
			continue
		}
		if v, ok, _ := o.Store.GetMeta("discovery_queue:" + law.ID); ok && v == "1" {
			queued = append(queued, law)
			continue
		}
		rest = append(rest, law)
	}
	return append(queued, rest...)
}

func (o *Orchestrator) catalogLookup() (discovery.CatalogLookup, error) {
	laws, err := o.Store.ListLaws()
	if err != nil {
		return discovery.CatalogLookup{}, err
	}
	variants, err := o.Store.ListVariants()
	if err != nil {
		return discovery.CatalogLookup{}, err
	}
	return discovery.CatalogLookup{Laws: laws, Variants: variants}, nil
}

func (o *Orchestrator) fetchLawXML(ctx context.Context, law domain.Law) ([]byte, error) {
	url, err := giiurl.XMLZip(o.CFG.GIIBase, law.GIIPath)
	if err != nil {
		return nil, err
	}
	body, _, status, err := o.HTTP.Get(ctx, url, "", "")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("xml.zip status %d", status)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
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
		rc.Close() // #nosec G104 -- best-effort close after ReadAll in zip entry loop
		if err != nil {
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("no xml in zip")
}

var reStandHTML = regexp.MustCompile(`(?is)Stand[^:<]{0,40}:?\s*</[^>]+>\s*([^<]{5,200})`)
var reStandPlain = regexp.MustCompile(`(?i)Stand:\s*([^\n<]{5,200})`)

func extractStand(html string) string {
	if m := reStandPlain.FindStringSubmatch(html); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	if m := reStandHTML.FindStringSubmatch(html); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parseBGBlRef(s string) (teil, year int, number string) {
	m := reBGBlInText.FindStringSubmatch(s)
	if len(m) < 4 {
		return 0, 0, ""
	}
	teil = teilFromToken(m[1])
	year, _ = strconv.Atoi(m[2])
	number = m[3]
	return
}

func parseELI(link string) (teil, year int, number string) {
	// .../eli/bund/BGBl-1/2026/209 or bgbl-1 or BGBl_1
	re := regexp.MustCompile(`(?i)bgbl[-_]([12])/(\d{4})/([0-9]+[a-zA-Z]?)`)
	m := re.FindStringSubmatch(link)
	if len(m) != 4 {
		return 0, 0, ""
	}
	teil, _ = strconv.Atoi(m[1])
	year, _ = strconv.Atoi(m[2])
	number = m[3]
	return
}

func teilFromToken(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "I", "1":
		return 1
	case "II", "2":
		return 2
	default:
		return 0
	}
}

func eliURL(base string, teil, year int, number string) string {
	return fmt.Sprintf("%s/BGBl-%d/%d/%s", strings.TrimRight(base, "/"), teil, year, number)
}

func parseRSSDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	formats := []string{time.RFC1123Z, time.RFC1123, time.RFC3339}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}

func mergeSources(existing []string, add string) []string {
	for _, s := range existing {
		if s == add {
			return existing
		}
	}
	return append(append([]string{}, existing...), add)
}

func matchLawFromItem(it rssItem, snap *search.Snapshot) string {
	if snap == nil {
		return ""
	}
	text := it.Title + " " + it.Description
	if m := reLawAbbr.FindStringSubmatch(text); len(m) == 2 {
		if best, _ := snap.Resolve(m[1], 0.9); best != nil {
			return best.Law.ID
		}
	}
	if best, _ := snap.Resolve(it.Title, 0.85); best != nil {
		return best.Law.ID
	}
	return ""
}

// StartBackground launches independent timers.
func (o *Orchestrator) StartBackground(ctx context.Context) {
	o.loop(ctx, o.CFG.TOCInterval, o.RunTOC)
	o.loop(ctx, o.CFG.GIIFeedInterval, o.RunGIIFeed)
	o.loop(ctx, o.CFG.BGBlFeedInterval, o.RunBGBlFeeds)
	o.loop(ctx, o.CFG.ELIProbeInterval, o.RunELIProbe)
	o.loop(ctx, 5*time.Minute, func(c context.Context) error { return o.Reconcile(c) })
}

// Wait blocks until all background sync loops started via StartBackground exit.
func (o *Orchestrator) Wait() {
	o.wg.Wait()
}

func (o *Orchestrator) sourceTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 2*o.CFG.HTTPTimeout)
}

func (o *Orchestrator) loop(ctx context.Context, every time.Duration, fn func(context.Context) error) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		// Delay first tick — InitialSync already ran at startup.
		t := time.NewTimer(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cctx, cancel := o.sourceTimeout(ctx)
				if err := fn(cctx); err != nil {
					o.Log.Error("sync job failed", "err", err)
				}
				cancel()
				t.Reset(every)
			}
		}
	}()
}

// InitialSync runs catalog + feeds once at startup (best effort).
func (o *Orchestrator) InitialSync(ctx context.Context) {
	_ = o.RunTOC(ctx)
	if n, err := o.RefreshMissingStands(ctx, o.CFG.StandRefreshMax); err != nil {
		o.Log.Warn("initial stand refresh", "err", err)
	} else if n > 0 {
		o.Log.Info("initial stand refresh", "filled", n, "max", o.CFG.StandRefreshMax)
	}
	if o.CFG.DiscoveryEnabled {
		if n, err := o.DiscoverOrdinances(ctx, o.CFG.DiscoveryMaxPerCycle); err != nil {
			o.Log.Warn("initial discovery", "err", err)
		} else if n > 0 {
			o.Log.Info("initial discovery", "ingested", n, "max", o.CFG.DiscoveryMaxPerCycle)
		}
	}
	_ = o.RunGIIFeed(ctx)
	if err := o.RunBGBlFeeds(ctx); err != nil {
		o.Log.Warn("initial bgbl feed failed, will rely on eli probe", "err", err)
		_ = o.RunELIProbe(ctx)
	}
	_ = o.Reconcile(ctx)
}
