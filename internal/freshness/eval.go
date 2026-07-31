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
	VRefResolutions            []domain.VRefResolution     // nil = legacy blunt path; non-nil = proof-aware
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
	if in.VRefResolutions == nil {
		return unresolvedInstrumentRefsBlunt(in)
	}
	return unresolvedInstrumentRefsProof(in)
}

// unresolvedInstrumentRefsBlunt is the legacy fail-closed path when VRefResolutions is nil.
func unresolvedInstrumentRefsBlunt(in Input) bool {
	for _, ref := range in.InstrumentRefs {
		if ref.Year == 0 || (ref.Number == "" && ref.Teil == 0) {
			continue
		}
		if standReflectsRef(in.Stand, ref) {
			continue
		}
		if isOperativeInstrumentRef(ref, in.HasSeededLinkedInstruments) {
			return true
		}
	}
	return false
}

// unresolvedInstrumentRefsProof uses computed V-ref resolutions (Proof Model C).
func unresolvedInstrumentRefsProof(in Input) bool {
	for _, ref := range in.InstrumentRefs {
		if ref.Year == 0 || (ref.Number == "" && ref.Teil == 0) {
			continue
		}
		if standReflectsRef(in.Stand, ref) {
			continue
		}
		if !isOperativeInstrumentRef(ref, in.HasSeededLinkedInstruments) {
			continue
		}
		res, ok := findVRefResolution(in.VRefResolutions, ref)
		if !ok {
			return true
		}
		if res.Historical {
			continue
		}
		// Require both flags so Resolved alone cannot upgrade without child proof.
		if res.Resolved && res.ChildConfirmed {
			continue
		}
		return true
	}
	return false
}

func standReflectsRef(stand domain.StandCitation, ref domain.InstrumentRef) bool {
	return stand.ParseOK &&
		stand.Year == ref.Year &&
		stand.Teil == ref.Teil &&
		stand.Number == ref.Number
}

func isOperativeInstrumentRef(ref domain.InstrumentRef, hasSeeded bool) bool {
	kind := strings.ToUpper(strings.TrimSpace(ref.Kind))
	switch kind {
	case "G":
		return false
	case "V":
		return true
	case "BEK":
		if strings.TrimSpace(ref.SectionHint) != "" {
			return true
		}
		return hasSeeded
	default:
		return hasSeeded && kind == ""
	}
}

func findVRefResolution(resolutions []domain.VRefResolution, ref domain.InstrumentRef) (domain.VRefResolution, bool) {
	for _, res := range resolutions {
		if refIdentityEqual(res.Ref, ref) {
			return res, true
		}
	}
	return domain.VRefResolution{}, false
}

func refIdentityEqual(a, b domain.InstrumentRef) bool {
	if a.Year != b.Year || a.Number != b.Number {
		return false
	}
	if a.Teil != 0 && b.Teil != 0 && a.Teil != b.Teil {
		return false
	}
	ak := strings.ToUpper(strings.TrimSpace(a.Kind))
	bk := strings.ToUpper(strings.TrimSpace(b.Kind))
	if ak != bk {
		return false
	}
	if strings.TrimSpace(a.SectionHint) != strings.TrimSpace(b.SectionHint) {
		return false
	}
	return true
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
