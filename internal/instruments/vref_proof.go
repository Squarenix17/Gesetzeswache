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

// MarkSupersededPastKindV marks unmatched past-only Kind V refs Historical when a same-section
// current sibling is already Resolved and ChildConfirmed.
func MarkSupersededPastKindV(
	resolutions []domain.VRefResolution,
	annotated []domain.LinkedInstrument,
	childStands map[string]domain.StandCitation,
) []domain.VRefResolution {
	if len(resolutions) == 0 {
		return []domain.VRefResolution{}
	}
	out := make([]domain.VRefResolution, len(resolutions))
	copy(out, resolutions)

	confirmedSlugs := make(map[string]struct{})
	for _, res := range out {
		if res.Resolved && res.ChildConfirmed && res.MatchedGIISlug != "" {
			confirmedSlugs[res.MatchedGIISlug] = struct{}{}
		}
	}

	var past, current []domain.LinkedInstrument
	for _, li := range annotated {
		switch li.Status {
		case StatusPast:
			past = append(past, li)
		case StatusCurrent, "":
			// Empty status: same as ResolveOperativeVRefs — discovery edges without EffectiveFrom.
			current = append(current, li)
		}
	}

	for i := range out {
		res := &out[i]
		if res.Historical {
			continue
		}
		if res.Resolved && res.ChildConfirmed {
			continue
		}
		if res.MatchedGIISlug != "" {
			continue
		}
		if !isKindV(res.Ref) {
			continue
		}

		pastMatches := matchChildrenByIdentity(res.Ref, past, childStands)
		if len(pastMatches) == 0 {
			continue
		}

		sectionNorm := normalizeSectionHint(res.Ref.SectionHint)
		if sectionNorm == "*" {
			// Fail closed if past matches disagree on section — do not pick arbitrarily.
			pastSection := normalizeSectionHint(pastMatches[0].SectionHint)
			for _, pm := range pastMatches[1:] {
				if normalizeSectionHint(pm.SectionHint) != pastSection {
					pastSection = "*"
					break
				}
			}
			sectionNorm = pastSection
		}
		if sectionNorm == "*" {
			continue
		}

		for _, child := range current {
			if normalizeSectionHint(child.SectionHint) != sectionNorm {
				continue
			}
			if _, ok := confirmedSlugs[child.GIISlug]; !ok {
				continue
			}
			res.Historical = true
			res.MatchMethod = "superseded_past_v"
			res.SupersededBy = child.GIISlug
			break
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

// ProveVRefResolutions resolves operative refs, applies Proof C child gate, then
// supersedes past-only Kind V when a same-section current sibling is confirmed.
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
	proven := ApplyChildFreshnessProof(raw, func(slug string) domain.FreshnessState {
		return EvaluateLeafChildFreshness(st, links, slug, ctx)
	})
	return MarkSupersededPastKindV(proven, annotated, childStands)
}
