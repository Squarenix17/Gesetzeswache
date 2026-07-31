package instruments

import (
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/freshness"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// LinksReader loads issue links for a law.
type LinksReader interface {
	LinksForLaw(lawID string) ([]domain.IssueLawLink, error)
}

// VRefProofContext holds sync/evidence fields shared between parent and child freshness eval.
type VRefProofContext struct {
	Now                time.Time
	MaxAge             time.Duration
	LastTOCSuccess     time.Time
	LastGIIFeedSuccess time.Time
	LastBGBlSuccess    time.Time
	BGBlFromProbeOnly  bool
	LinksReadFailed    bool
}

// ApplyChildFreshnessProof runs Proof C child gate and sets Resolved/ChildConfirmed.
func ApplyChildFreshnessProof(
	resolutions []domain.VRefResolution,
	evalChild func(slug string) domain.FreshnessState,
) []domain.VRefResolution {
	if len(resolutions) == 0 {
		return []domain.VRefResolution{}
	}
	out := make([]domain.VRefResolution, len(resolutions))
	copy(out, resolutions)
	for i := range out {
		if out[i].Historical || out[i].MatchedGIISlug == "" {
			continue
		}
		state := evalChild(out[i].MatchedGIISlug)
		if state == domain.FreshnessConfirmedCurrent {
			out[i].ChildConfirmed = true
			out[i].Resolved = true
		}
	}
	return out
}

// EvaluateLeafChildFreshness evaluates a matched child as a leaf (no V-ref recursion).
func EvaluateLeafChildFreshness(
	st MetaStandIssueStore,
	links LinksReader,
	childSlug string,
	ctx VRefProofContext,
) domain.FreshnessState {
	childID := normalize.Key(childSlug)
	if childID == "" {
		return domain.FreshnessUncertain
	}
	stand, _, _ := st.GetStand(childID)
	var issues []domain.GazetteIssue
	classes := map[string]domain.LinkClass{}
	linksErr := ctx.LinksReadFailed
	if links != nil {
		linkRows, err := links.LinksForLaw(childID)
		if err != nil {
			linksErr = true
		} else {
			for _, l := range linkRows {
				classes[l.IssueID] = l.Class
				if iss, ok, _ := st.GetIssue(l.IssueID); ok {
					issues = append(issues, iss)
				}
			}
		}
	}
	instrRefs, instrIssues := CollectEvidence(st, nil, childID, stand)
	rec := freshness.Evaluate(freshness.Input{
		LawID:                      childID,
		Stand:                      stand,
		LinkedIssues:               issues,
		LinkClasses:                classes,
		InstrumentRefs:             instrRefs,
		InstrumentIssues:           instrIssues,
		HasSeededLinkedInstruments: false,
		VRefResolutions:            nil,
		LinksReadFailed:            linksErr,
		LastTOCSuccess:             ctx.LastTOCSuccess,
		LastGIIFeedSuccess:         ctx.LastGIIFeedSuccess,
		LastBGBlSuccess:            ctx.LastBGBlSuccess,
		BGBlFromProbeOnly:          ctx.BGBlFromProbeOnly,
		Now:                        ctx.Now,
		MaxAge:                     ctx.MaxAge,
	})
	return rec.State
}

// ProveVRefResolutions resolves operative refs and applies Proof C child gate.
func ProveVRefResolutions(
	refs []domain.InstrumentRef,
	annotated []domain.LinkedInstrument,
	parentStand domain.StandCitation,
	st MetaStandIssueStore,
	index BGBlIndexLookup,
	links LinksReader,
	ctx VRefProofContext,
) []domain.VRefResolution {
	childStands := BuildChildStands(annotated, st)
	raw := ResolveOperativeVRefs(refs, annotated, childStands, index, parentStand)
	return ApplyChildFreshnessProof(raw, func(slug string) domain.FreshnessState {
		return EvaluateLeafChildFreshness(st, links, slug, ctx)
	})
}
