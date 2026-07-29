// Package freshness evaluates per-law freshness against sync age and linked issues.
package freshness

import (
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

// Input gathers evidence for one law evaluation.
type Input struct {
	LawID                      string
	Stand                      domain.StandCitation
	LinkedIssues               []domain.GazetteIssue       // issues linked to this law (confirmed or heuristic)
	LinkClasses                map[string]domain.LinkClass // issueID → class; optional
	InstrumentRefs             []domain.InstrumentRef      // from Stand / +++ notes
	InstrumentIssues           []domain.GazetteIssue       // store issues matching InstrumentRefs
	HasSeededLinkedInstruments bool                        // seeded TSV and/or high-confidence discovered linked instruments
	LinksReadFailed            bool                        // LinksForLaw or DiscoveredForParent store read failed → fail closed
	LastTOCSuccess             time.Time
	LastGIIFeedSuccess         time.Time
	LastBGBlSuccess            time.Time // feed or probe
	BGBlFromProbeOnly          bool
	Now                        time.Time
	MaxAge                     time.Duration
}

// Evaluate derives freshness state. Never returns confirmed_current when sync data is too old,
// Stand is missing/unparsed without compensating links, or unresolved instrument refs remain.
func Evaluate(in Input) domain.FreshnessRecord {
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rec := domain.FreshnessRecord{
		LawID:       in.LawID,
		EvaluatedAt: now,
		Confidence:  "high",
		Method:      domain.MethodFeeds,
	}
	if in.BGBlFromProbeOnly {
		rec.Method = domain.MethodProbeOnly
		rec.Confidence = "low"
	}

	syncFresh := syncWithinMaxAge(in, now)
	if !syncFresh {
		rec.State = domain.FreshnessUncertain
		rec.Confidence = "low"
		rec.Rationale = "sync data older than max age"
		return rec
	}

	if in.BGBlFromProbeOnly {
		rec.State = domain.FreshnessUncertain
		rec.Confidence = "low"
		rec.Rationale = "bgbl_evidence_probe_only"
		return rec
	}

	if in.LinksReadFailed {
		rec.State = domain.FreshnessUncertain
		rec.Confidence = "low"
		rec.Rationale = "links_read_failed"
		return rec
	}

	standMissing := !in.Stand.ParseOK && in.Stand.Raw == "" && in.Stand.Year == 0 && in.Stand.Number == "" && in.Stand.Page == ""
	standUnparsed := !in.Stand.ParseOK
	if standMissing {
		standUnparsed = true
	}

	var newer []string
	for _, iss := range in.LinkedIssues {
		if issueNewerThanStand(iss, in.Stand) {
			newer = append(newer, iss.ID)
		}
	}
	// Instrument issues cited in +++ / Stand that are newer than reflected Stand
	for _, iss := range in.InstrumentIssues {
		if issueNewerThanStand(iss, in.Stand) {
			newer = append(newer, iss.ID)
		}
	}
	newer = uniqueStrings(newer)

	if len(newer) > 0 {
		rec.State = domain.FreshnessConfirmedStale
		rec.NewerIssueIDs = newer
		rec.Rationale = "newer gazette issue linked than reflected Stand"
		if onlyHeuristicLinks(in) {
			rec.Confidence = lowerConfidence(rec.Confidence)
		}
		return rec
	}

	// Architecture: parse failure / missing Stand → uncertain when we cannot prove current.
	if standMissing || standUnparsed {
		// Unparseable Stand + linked issues with year/num already handled as stale above when applicable.
		// Empty or inconclusive links → uncertain (never false confirmed_current).
		rec.State = domain.FreshnessUncertain
		rec.Confidence = "low"
		rec.Rationale = "stand_unparsed_or_missing"
		return rec
	}

	// Parsed Stand: unresolved instrument refs that cite known BGBl identity but weren't
	// reflected / compared into a conclusive current claim → uncertain.
	if unresolvedInstrumentRefs(in) {
		rec.State = domain.FreshnessUncertain
		rec.Confidence = lowerConfidence(rec.Confidence)
		rec.Rationale = "unresolved_linked_instrument_refs"
		return rec
	}

	rec.State = domain.FreshnessConfirmedCurrent
	rec.Rationale = "no newer linked gazette issue beyond Stand"
	if onlyHeuristicLinks(in) {
		rec.Confidence = lowerConfidence(rec.Confidence)
	}
	return rec
}

func syncWithinMaxAge(in Input, now time.Time) bool {
	if in.MaxAge <= 0 {
		in.MaxAge = 6 * time.Hour
	}
	if !timestampFresh(in.LastTOCSuccess, now, in.MaxAge) {
		return false
	}
	if !timestampFresh(in.LastGIIFeedSuccess, now, in.MaxAge) {
		return false
	}
	if !timestampFresh(in.LastBGBlSuccess, now, in.MaxAge) {
		return false
	}
	return true
}

func issueNewerThanStand(iss domain.GazetteIssue, stand domain.StandCitation) bool {
	if !stand.ParseOK {
		// Cannot prove reflection; if linked issue exists with year/num, treat as potentially newer → stale signal
		return iss.Year > 0 && iss.Number != ""
	}
	pseudo := domain.StandCitation{
		Teil:    iss.Teil,
		Year:    iss.Year,
		Number:  iss.Number,
		ParseOK: true,
	}
	cmp, ok := citation.Compare(stand, pseudo)
	if !ok {
		if stand.Date != nil && iss.PublishedAt != nil {
			return iss.PublishedAt.After(*stand.Date)
		}
		return false
	}
	return cmp < 0
}

func onlyHeuristicLinks(in Input) bool {
	if len(in.LinkedIssues) == 0 {
		return false
	}
	if len(in.LinkClasses) == 0 {
		return false
	}
	hasConfirmed := false
	hasHeuristic := false
	for _, iss := range in.LinkedIssues {
		c, ok := in.LinkClasses[iss.ID]
		if !ok {
			continue
		}
		switch c {
		case domain.LinkConfirmed:
			hasConfirmed = true
		case domain.LinkHeuristic:
			hasHeuristic = true
		}
	}
	return hasHeuristic && !hasConfirmed
}

func unresolvedInstrumentRefs(in Input) bool {
	if len(in.InstrumentRefs) == 0 {
		return false
	}
	for _, ref := range in.InstrumentRefs {
		if ref.Year == 0 || (ref.Number == "" && ref.Teil == 0) {
			continue
		}
		// Same citation as Stand → reflected
		if in.Stand.ParseOK &&
			in.Stand.Year == ref.Year &&
			in.Stand.Teil == ref.Teil &&
			in.Stand.Number == ref.Number {
			continue
		}
		// Operative amendment-by-reference: Verordnung always fail-closes.
		// Bare Bekanntmachung on mass codes is editorial history (like G) — ignore.
		// Section-scoped BEK still fail-closes (operative pointer).
		kind := strings.ToUpper(ref.Kind)
		if kind == "V" {
			return true
		}
		if kind == "BEK" && strings.TrimSpace(ref.SectionHint) != "" {
			return true
		}
		// Seeded parent→instrument laws: empty-Kind / bare BEK notes from TSV still fail closed.
		if in.HasSeededLinkedInstruments {
			return true
		}
		// Kind G, empty, or bare BEK: mass-code editorial / in-law cross-refs — ignore.
	}
	return false
}

func lowerConfidence(c string) string {
	switch c {
	case "high":
		return "medium"
	case "medium":
		return "low"
	default:
		return "low"
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
