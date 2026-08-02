package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// RecheckResult is the honest delta payload for POST /v1/recheck.
type RecheckResult struct {
	Status        string                `json:"status"`
	LawID         string                `json:"law_id,omitempty"`
	StateBefore   domain.FreshnessState `json:"state_before,omitempty"`
	StateAfter    domain.FreshnessState `json:"state_after,omitempty"`
	Changed       bool                  `json:"changed"`
	Reason        string                `json:"reason,omitempty"`
	NewerIssueIDs []string              `json:"newer_issue_ids,omitempty"`
	Stand         *domain.StandCitation `json:"stand,omitempty"`
	Rationale     string                `json:"rationale,omitempty"`
}

// ForceRecheck re-runs feeds/reconcile and returns whether targeted law freshness changed.
// It never claims export is unblocked; callers must still use allow_stale / profile=ingest.
func (s *Service) ForceRecheck(ctx context.Context, lawID string) (RecheckResult, error) {
	out := RecheckResult{Status: "recheck_completed"}
	if err := ctx.Err(); err != nil {
		return out, fmt.Errorf("%w: %w", ErrRecheckTimeout, err)
	}
	lawID = strings.TrimSpace(lawID)
	if lawID != "" {
		if err := validateQueryLength(lawID); err != nil {
			return out, err
		}
		if _, ok, err := s.Store.GetLaw(lawID); err != nil {
			return out, err
		} else if !ok {
			var resolved bool
			if snap := s.Search.Current(); snap != nil {
				if best, _, _ := snap.Resolve(lawID, s.CFG.MatchThreshold); best != nil {
					lawID = best.Law.ID
					resolved = true
				}
			}
			if !resolved {
				return out, ErrLawNotFound
			}
		}
		out.LawID = lawID
		if before, err := s.freshnessFor(lawID, IncludeOpts{}); err == nil {
			out.StateBefore = before.State
		}
	}
	if err := s.Sync.RunGIIFeed(ctx); err != nil {
		if err := recheckCtxExpired(ctx); err != nil {
			return out, err
		}
		if s.Log != nil {
			s.Log.Warn("force recheck gii feed", "err", err)
		}
	}
	if err := s.Sync.RunBGBlFeeds(ctx); err != nil {
		if err := recheckCtxExpired(ctx); err != nil {
			return out, err
		}
		_ = s.Sync.RunELIProbe(ctx)
	}
	if lawID != "" {
		if law, ok, _ := s.Store.GetLaw(lawID); ok {
			if err := s.Sync.RefreshStandForLaw(ctx, law); err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					return out, fmt.Errorf("%w: %w", ErrRecheckTimeout, err)
				}
			}
			s.Export.InvalidateLaw(law.ID)
			rows, err := s.linkedInstrumentsFor(lawID)
			if err != nil && s.Log != nil {
				s.Log.Warn("linked instruments on recheck", "law", lawID, "err", err)
			}
			ensured := map[string]struct{}{}
			refreshedIndex := false
			for _, li := range rows {
				slug := strings.TrimSpace(li.GIISlug)
				if slug == "" {
					continue
				}
				if _, done := ensured[slug]; done {
					continue
				}
				ensured[slug] = struct{}{}
				if _, neu, err := instruments.EnsureLawFromSlug(s.Store, s.CFG.GIIBase, slug); err != nil {
					if s.Log != nil {
						s.Log.Warn("ensure linked child on recheck", "law", lawID, "slug", slug, "err", err)
					}
					continue
				} else if neu {
					refreshedIndex = true
				}
				childID := normalize.Key(slug)
				if li.LawID != "" {
					childID = normalize.Key(li.LawID)
				}
				if child, ok, _ := s.Store.GetLaw(childID); ok {
					if err := s.Sync.RefreshStandForLaw(ctx, child); err != nil {
						if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
							return out, fmt.Errorf("%w: %w", ErrRecheckTimeout, err)
						}
					}
					s.Export.InvalidateLaw(child.ID)
				}
			}
			if refreshedIndex {
				s.refreshSearchIndex()
			}
		}
	}
	if err := s.Sync.Reconcile(ctx); err != nil {
		if err := recheckCtxExpired(ctx); err != nil {
			return out, err
		}
		return out, err
	}
	if err := recheckCtxExpired(ctx); err != nil {
		return out, err
	}
	if lawID == "" {
		return out, nil
	}
	after, err := s.freshnessFor(lawID, IncludeOpts{Linked: true, Proof: true})
	if err != nil {
		return out, err
	}
	out.StateAfter = after.State
	out.Changed = out.StateBefore != "" && out.StateBefore != after.State
	out.Rationale = after.Rationale
	out.Stand = after.Stand
	out.NewerIssueIDs = after.NewerIssueIDs
	switch after.State {
	case domain.FreshnessConfirmedStale:
		out.Reason = recheckStaleReason(after)
	case domain.FreshnessUncertain:
		out.Reason = after.Rationale
		if out.Reason == "" {
			out.Reason = "uncertain"
		}
	}
	return out, nil
}

func recheckStaleReason(meta FreshnessMeta) string {
	if meta.Rationale == "newer gazette issue linked than reflected Stand" {
		return "newer_gazette_than_stand"
	}
	if meta.Rationale != "" {
		return meta.Rationale
	}
	return "gii_stand_lag"
}
