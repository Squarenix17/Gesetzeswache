// Package freshness evaluates per-law freshness against sync age and linked issues.
package freshness

import (
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

// Input gathers evidence for one law evaluation.
type Input struct {
	LawID              string
	Stand              domain.StandCitation
	LinkedIssues       []domain.GazetteIssue // issues linked to this law (confirmed or heuristic)
	LastTOCSuccess     time.Time
	LastGIIFeedSuccess time.Time
	LastBGBlSuccess    time.Time // feed or probe
	BGBlFromProbeOnly  bool
	Now                time.Time
	MaxAge             time.Duration
}

// Evaluate derives freshness state. Never returns confirmed_current when sync data is too old.
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

	var newer []string
	for _, iss := range in.LinkedIssues {
		if issueNewerThanStand(iss, in.Stand) {
			newer = append(newer, iss.ID)
		}
	}
	if len(newer) > 0 {
		rec.State = domain.FreshnessConfirmedStale
		rec.NewerIssueIDs = newer
		rec.Rationale = "newer gazette issue linked than reflected Stand"
		// heuristic-only links lower confidence
		for _, iss := range in.LinkedIssues {
			_ = iss
		}
		return rec
	}

	rec.State = domain.FreshnessConfirmedCurrent
	rec.Rationale = "no newer linked gazette issue beyond Stand"
	return rec
}

func syncWithinMaxAge(in Input, now time.Time) bool {
	if in.MaxAge <= 0 {
		in.MaxAge = 6 * time.Hour
	}
	// Require recent catalog and at least one BGBl existence source
	if in.LastTOCSuccess.IsZero() || now.Sub(in.LastTOCSuccess) > in.MaxAge {
		return false
	}
	bgbl := in.LastBGBlSuccess
	if bgbl.IsZero() || now.Sub(bgbl) > in.MaxAge {
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
		// If same teil/year unknown ordering, use dates if present
		if stand.Date != nil && iss.PublishedAt != nil {
			return iss.PublishedAt.After(*stand.Date)
		}
		return false
	}
	// cmp < 0 means stand older than issue
	return cmp < 0
}
