// Package sync implements background catalog and BGBl observation jobs.
package sync

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/freshness"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/store"
)

// Orchestrator runs independent sync jobs.
type Orchestrator struct {
	CFG    config.Config
	Store  *store.Store
	HTTP   *httpx.Client
	Search *search.Engine
	Log    *slog.Logger
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
	}()
	if err != nil {
		a.Error = err.Error()
		return err
	}
	if status >= 400 {
		a.Error = fmt.Sprintf("status %d", status)
		return fmt.Errorf("toc status %d", status)
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
	_ = o.Store.SetMetaTime("last_toc_success", time.Now().UTC())
	a.Success = true
	a.Detail = fmt.Sprintf("%d laws", len(laws))
	o.Log.Info("toc sync ok", "laws", len(laws))
	return nil
}

func guessAbbr(title, slug string) string {
	// Prefer slug uppercase if short
	if slug != "" && len(slug) <= 12 {
		return strings.ToUpper(slug)
	}
	return slug
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
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		a.Error = err.Error()
		return err
	}
	n := 0
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
		_ = o.Store.UpsertIssue(iss)

		lawID := matchLawFromItem(it, o.Search.Current())
		if lawID != "" {
			_ = o.Store.UpsertLink(domain.IssueLawLink{
				IssueID:   id,
				LawID:     lawID,
				Class:     domain.LinkConfirmed,
				CreatedAt: time.Now().UTC(),
			})
			iss.Matched = true
			_ = o.Store.UpsertIssue(iss)
			n++
		}
	}
	_ = o.Store.SetMetaTime("last_gii_feed_success", time.Now().UTC())
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
	}()
	count := 0
	for _, url := range []string{o.CFG.BGBlFeed1URL, o.CFG.BGBlFeed2URL} {
		body, _, status, err := o.HTTP.Get(ctx, url, "", "")
		if err != nil {
			o.Log.Warn("bgbl feed fetch failed", "url", url, "err", err)
			continue
		}
		if status >= 400 {
			o.Log.Warn("bgbl feed status", "url", url, "status", status)
			continue
		}
		var feed rssFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			o.Log.Warn("bgbl feed parse", "err", err)
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
			_ = o.Store.UpsertIssue(iss)
			count++
		}
	}
	if count == 0 {
		a.Error = "no items parsed from bgbl feeds"
		return fmt.Errorf("%s", a.Error)
	}
	_ = o.Store.SetMetaTime("last_bgbl_feed_success", time.Now().UTC())
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
	}()
	year := time.Now().UTC().Year()
	found := 0
	for _, teil := range []int{1, 2} {
		maxN := latestNumber(o.Store, teil, year)
		for n := maxN; n <= maxN+3; n++ {
			num := strconv.Itoa(n)
			url := eliURL(o.CFG.ELIBase, teil, year, num)
			ok, _, err := o.HTTP.Exists(ctx, url)
			if err != nil {
				continue
			}
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
			_ = o.Store.UpsertIssue(iss)
			found++
		}
	}
	_ = o.Store.SetMetaTime("last_eli_probe_success", time.Now().UTC())
	a.Success = true
	a.Detail = fmt.Sprintf("probed hits %d", found)
	return nil
}

func latestNumber(st *store.Store, teil, year int) int {
	issues, err := st.ListIssues()
	if err != nil {
		return 1
	}
	max := 1
	for _, iss := range issues {
		if iss.Teil == teil && iss.Year == year {
			n, _ := strconv.Atoi(strings.TrimRightFunc(iss.Number, func(r rune) bool {
				return r < '0' || r > '9'
			}))
			// parse leading digits
			n = 0
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
	_ = ctx
	now := time.Now().UTC()
	tocT, _, _ := o.Store.GetMetaTime("last_toc_success")
	giiT, _, _ := o.Store.GetMetaTime("last_gii_feed_success")
	bgblT, bgblOK, _ := o.Store.GetMetaTime("last_bgbl_feed_success")
	eliT, eliOK, _ := o.Store.GetMetaTime("last_eli_probe_success")
	bgblSuccess := bgblT
	probeOnly := false
	if !bgblOK || (eliOK && (bgblT.IsZero() || eliT.After(bgblT.Add(o.CFG.FreshnessMaxAge)))) {
		if eliOK {
			if bgblT.IsZero() || now.Sub(bgblT) > o.CFG.FreshnessMaxAge {
				bgblSuccess = eliT
				probeOnly = true
			}
		}
	}
	_ = giiT

	if o.CFG.EnableHeuristic {
		o.heuristicLink(now)
	}

	laws, err := o.Store.ListLaws()
	if err != nil {
		return err
	}
	for _, law := range laws {
		stand, _, _ := o.Store.GetStand(law.ID)
		links, _ := o.Store.LinksForLaw(law.ID)
		var issues []domain.GazetteIssue
		for _, l := range links {
			if iss, ok, _ := o.Store.GetIssue(l.IssueID); ok {
				issues = append(issues, iss)
			}
		}
		rec := freshness.Evaluate(freshness.Input{
			LawID:             law.ID,
			Stand:             stand,
			LinkedIssues:      issues,
			LastTOCSuccess:    tocT,
			LastGIIFeedSuccess: giiT,
			LastBGBlSuccess:   bgblSuccess,
			BGBlFromProbeOnly: probeOnly,
			Now:               now,
			MaxAge:            o.CFG.FreshnessMaxAge,
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

// RefreshStandForLaw fetches a law HTML page for Stand — URLs are always rebuilt from config base + validated slug.
func (o *Orchestrator) RefreshStandForLaw(ctx context.Context, law domain.Law) error {
	if law.GIIPath == "" {
		return nil
	}
	url, err := giiurl.IndexURL(o.CFG.GIIBase, law.GIIPath)
	if err != nil {
		return err
	}
	body, _, status, err := o.HTTP.Get(ctx, url, "", "")
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("stand fetch status %d", status)
	}
	raw := extractStand(string(body))
	if raw == "" {
		return nil
	}
	c := citation.Parse(law.ID, raw)
	return o.Store.UpsertStand(c)
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
	go o.loop(ctx, o.CFG.TOCInterval, o.RunTOC)
	go o.loop(ctx, o.CFG.GIIFeedInterval, o.RunGIIFeed)
	go o.loop(ctx, o.CFG.BGBlFeedInterval, o.RunBGBlFeeds)
	go o.loop(ctx, o.CFG.ELIProbeInterval, o.RunELIProbe)
	go o.loop(ctx, 5*time.Minute, func(c context.Context) error { return o.Reconcile(c) })
}

func (o *Orchestrator) loop(ctx context.Context, every time.Duration, fn func(context.Context) error) {
	// Delay first tick — InitialSync already ran at startup.
	t := time.NewTimer(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cctx, cancel := context.WithTimeout(ctx, o.CFG.HTTPTimeout*4)
			if err := fn(cctx); err != nil {
				o.Log.Error("sync job failed", "err", err)
			}
			cancel()
			t.Reset(every)
		}
	}
}

// InitialSync runs catalog + feeds once at startup (best effort).
func (o *Orchestrator) InitialSync(ctx context.Context) {
	_ = o.RunTOC(ctx)
	// light stand refresh for first N laws is expensive; skip bulk in v1 — Stand filled on demand / partial
	_ = o.RunGIIFeed(ctx)
	if err := o.RunBGBlFeeds(ctx); err != nil {
		o.Log.Warn("initial bgbl feed failed, will rely on eli probe", "err", err)
		_ = o.RunELIProbe(ctx)
	}
	_ = o.Reconcile(ctx)
}
