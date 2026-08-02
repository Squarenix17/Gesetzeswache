package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// MaxOperativeBundleMembers caps linked Verordnungen exported in one bundle.
const MaxOperativeBundleMembers = 8

// BundleOpts controls operative-bundle membership and optional display compose.
type BundleOpts struct {
	Past       bool // include status=past instruments
	Compose    bool // emit display-only composed hierarchical (not for vector ingest)
	AllowStale bool // skip RefuseExportStale gate; does not change safe_to_serve
	ParentOnly bool // export parent only; skip operative members and size cap
}

// BundleMemberFreshness is one row in bundle_freshness.members.
type BundleMemberFreshness struct {
	Role        string                `json:"role"` // parent | operative
	LawID       string                `json:"law_id"`
	State       domain.FreshnessState `json:"state"`
	SectionHint string                `json:"section_hint,omitempty"`
}

// BundleFreshnessMeta is the fail-closed aggregate over parent + operative members.
type BundleFreshnessMeta struct {
	State       domain.FreshnessState   `json:"state"`
	SafeToServe bool                    `json:"safe_to_serve"`
	Rationale   string                  `json:"rationale,omitempty"`
	Members     []BundleMemberFreshness `json:"members"`
}

// BundleMemberExport is the parent side of a bundle (unmixed formats for indexing).
type BundleMemberExport struct {
	Law       *domain.Law    `json:"law,omitempty"`
	Freshness *FreshnessMeta `json:"freshness,omitempty"`
	Formats   map[string]any `json:"formats,omitempty"`
}

// OperativeMemberExport is one linked Verordnung with link metadata and unmixed formats.
type OperativeMemberExport struct {
	Link      domain.LinkedInstrument `json:"link"`
	Law       *domain.Law             `json:"law,omitempty"`
	Freshness *FreshnessMeta          `json:"freshness,omitempty"`
	Formats   map[string]any          `json:"formats,omitempty"`
	Placement string                  `json:"placement"` // after_parent_section | document_end
}

// OperativeBundleResult is the authoritative index contract: parent + operative[] unmixed.
type OperativeBundleResult struct {
	Matched         bool                    `json:"matched"`
	Query           string                  `json:"query,omitempty"`
	Parent          *BundleMemberExport     `json:"parent,omitempty"`
	Operative       []OperativeMemberExport `json:"operative,omitempty"`
	Formats         map[string]any          `json:"formats,omitempty"` // optional composed hierarchical only
	BundleFreshness *BundleFreshnessMeta    `json:"bundle_freshness,omitempty"`
	Suggestions     []Suggestion            `json:"suggestions,omitempty"`
}

// ExportOperativeBundle resolves a parent Gesetz and exports current (optionally past) linked
// Verordnungen as separate members. Parent formats never include VO body text.
func (s *Service) ExportOperativeBundle(ctx context.Context, query string, formats []string, opts BundleOpts) (OperativeBundleResult, error) {
	query = strings.TrimSpace(query)
	if err := validateQueryLength(query); err != nil {
		return OperativeBundleResult{}, err
	}
	if !s.CFG.EnableExport {
		return OperativeBundleResult{}, fmt.Errorf("export disabled")
	}
	if formats == nil {
		formats = []string{export.FormatHierarchical}
	}
	if len(formats) == 0 {
		return OperativeBundleResult{}, fmt.Errorf("empty format list")
	}
	for _, f := range formats {
		switch f {
		case export.FormatHierarchical, export.FormatChunked, export.FormatFlat, export.FormatNormtext:
		default:
			return OperativeBundleResult{}, fmt.Errorf("unknown format %q", f)
		}
	}

	include := IncludeOpts{Past: opts.Past, Linked: true}
	res, err := s.Resolve(ctx, query, include)
	if err != nil {
		key := normalize.Key(query)
		if law, ok, _ := s.Store.GetLaw(key); ok {
			return s.exportOperativeBundleLaw(ctx, query, law, formats, opts)
		}
		return OperativeBundleResult{}, err
	}
	if !res.Matched || res.Law == nil {
		return OperativeBundleResult{Matched: false, Query: query, Suggestions: res.Suggestions}, nil
	}
	return s.exportOperativeBundleLaw(ctx, query, *res.Law, formats, opts)
}

func (s *Service) exportOperativeBundleLaw(ctx context.Context, query string, parent domain.Law, formats []string, opts BundleOpts) (OperativeBundleResult, error) {
	include := IncludeOpts{Past: opts.Past, Linked: true}
	now := time.Now().UTC()
	gate := ExportGateOpts{AllowStale: opts.AllowStale}

	var members []domain.LinkedInstrument
	if !opts.ParentOnly {
		annotated, _ := s.operativeAnnotatedLinked(parent.ID, now)
		members = instruments.FilterBundleMembers(annotated, opts.Past)
		maxMembers := s.maxOperativeBundleMembers()
		if len(members) > maxMembers {
			refs := make([]OperativeBundleMemberRef, 0, len(members))
			for _, m := range members {
				refs = append(refs, OperativeBundleMemberRef{
					LawID:       m.LawID,
					GIISlug:     m.GIISlug,
					SectionHint: m.SectionHint,
				})
			}
			return OperativeBundleResult{}, &OperativeBundleTooLargeError{
				Max:     maxMembers,
				Actual:  len(members),
				Members: refs,
			}
		}
	}

	parentMeta, err := s.freshnessFor(parent.ID, include)
	if err != nil {
		return OperativeBundleResult{}, err
	}
	if s.CFG.RefuseExportStale && !opts.AllowStale && parentMeta.State == domain.FreshnessConfirmedStale {
		return OperativeBundleResult{}, fmt.Errorf("export refused: bundle member confirmed_stale")
	}

	parentExport, err := s.exportLaw(ctx, parent, parentMeta, formats, include, gate)
	if err != nil {
		if strings.Contains(err.Error(), "confirmed_stale") {
			return OperativeBundleResult{}, fmt.Errorf("export refused: bundle member confirmed_stale")
		}
		return OperativeBundleResult{}, err
	}

	parentHier, _ := parentExport.Formats[export.FormatHierarchical].(string)
	needHierForPlacement := parentHier == "" && len(members) > 0
	if needHierForPlacement || opts.Compose {
		// Ensure hierarchical exists for placement / compose without polluting index formats.
		if parentHier == "" {
			extra, err := s.exportLaw(ctx, parent, parentMeta, []string{export.FormatHierarchical}, include, gate)
			if err != nil {
				return OperativeBundleResult{}, err
			}
			parentHier, _ = extra.Formats[export.FormatHierarchical].(string)
		}
	}

	out := OperativeBundleResult{
		Matched: true,
		Query:   query,
		Parent: &BundleMemberExport{
			Law:       parentExport.Law,
			Freshness: parentExport.Freshness,
			Formats:   parentExport.Formats,
		},
		Operative: make([]OperativeMemberExport, 0, len(members)),
	}

	composeMembers := make([]export.OperativeComposeMember, 0, len(members))
	for _, link := range members {
		child, neu, err := instruments.EnsureLawFromSlug(s.Store, s.CFG.GIIBase, link.GIISlug)
		if err != nil {
			return OperativeBundleResult{}, err
		}
		if neu {
			s.refreshSearchIndex()
		}
		childMeta, err := s.freshnessFor(child.ID, IncludeOpts{})
		if err != nil {
			return OperativeBundleResult{}, err
		}
		if s.CFG.RefuseExportStale && !opts.AllowStale && childMeta.State == domain.FreshnessConfirmedStale {
			return OperativeBundleResult{}, fmt.Errorf("export refused: bundle member confirmed_stale")
		}
		childExport, err := s.exportLaw(ctx, child, childMeta, formats, IncludeOpts{}, gate)
		if err != nil {
			if strings.Contains(err.Error(), "confirmed_stale") {
				return OperativeBundleResult{}, fmt.Errorf("export refused: bundle member confirmed_stale")
			}
			return OperativeBundleResult{}, err
		}
		// Prefer pointer fields on the link for consumers.
		linkOut := link
		linkOut.LawID = child.ID
		linkOut.GIIURL = child.GIIURL
		linkOut.ResolveOK = true

		childHier, _ := childExport.Formats[export.FormatHierarchical].(string)
		if childHier == "" && opts.Compose {
			extra, err := s.exportLaw(ctx, child, childMeta, []string{export.FormatHierarchical}, IncludeOpts{}, gate)
			if err != nil {
				return OperativeBundleResult{}, err
			}
			childHier, _ = extra.Formats[export.FormatHierarchical].(string)
		}

		placement := export.PlacementDocumentEnd
		if parentHier != "" {
			placement = export.PlacementForHint(parentHier, link.SectionHint)
		}
		op := OperativeMemberExport{
			Link:      linkOut,
			Law:       childExport.Law,
			Freshness: childExport.Freshness,
			Formats:   childExport.Formats,
			Placement: placement,
		}
		out.Operative = append(out.Operative, op)
		if opts.Compose {
			abbr := ""
			if childExport.Law != nil {
				abbr = childExport.Law.Abbreviation
			}
			composeMembers = append(composeMembers, export.OperativeComposeMember{
				LawID:         child.ID,
				Abbreviation:  abbr,
				SectionHint:   link.SectionHint,
				Status:        link.Status,
				EffectiveFrom: link.EffectiveFrom,
				Hierarchical:  childHier,
			})
		}
	}

	bf := aggregateBundleFreshness(out.Parent, out.Operative)
	out.BundleFreshness = &bf

	if opts.Compose {
		parentAbbr := parent.Abbreviation
		composed := export.ComposeOperativeHierarchical(parentHier, parentAbbr, composeMembers)
		out.Formats = map[string]any{export.FormatHierarchical: composed}
	}

	return out, nil
}

func aggregateBundleFreshness(parent *BundleMemberExport, ops []OperativeMemberExport) BundleFreshnessMeta {
	meta := BundleFreshnessMeta{Members: make([]BundleMemberFreshness, 0, 1+len(ops))}
	worst := domain.FreshnessConfirmedCurrent
	parentUncertain := false
	anyStale := false
	anyUncertain := false

	add := func(role, lawID, hint string, state domain.FreshnessState) {
		meta.Members = append(meta.Members, BundleMemberFreshness{
			Role: role, LawID: lawID, State: state, SectionHint: hint,
		})
		switch state {
		case domain.FreshnessConfirmedStale:
			anyStale = true
			worst = domain.FreshnessConfirmedStale
		case domain.FreshnessUncertain:
			anyUncertain = true
			if role == "parent" {
				parentUncertain = true
			}
			if worst != domain.FreshnessConfirmedStale {
				worst = domain.FreshnessUncertain
			}
		}
	}

	if parent != nil {
		st := domain.FreshnessUncertain
		id := ""
		if parent.Law != nil {
			id = parent.Law.ID
		}
		if parent.Freshness != nil {
			st = parent.Freshness.State
		}
		add("parent", id, "", st)
	}
	for _, op := range ops {
		st := domain.FreshnessUncertain
		if op.Freshness != nil {
			st = op.Freshness.State
		}
		id := op.Link.LawID
		if id == "" && op.Law != nil {
			id = op.Law.ID
		}
		add("operative", id, op.Link.SectionHint, st)
	}

	meta.State = worst
	meta.SafeToServe = worst == domain.FreshnessConfirmedCurrent
	switch {
	case anyStale:
		meta.Rationale = "member_confirmed_stale"
	case parentUncertain:
		meta.Rationale = "parent_uncertain"
	case anyUncertain:
		meta.Rationale = "member_uncertain"
	}
	return meta
}

func (s *Service) maxOperativeBundleMembers() int {
	n := s.CFG.MaxOperativeBundleMembers
	if n <= 0 {
		return MaxOperativeBundleMembers
	}
	return n
}
