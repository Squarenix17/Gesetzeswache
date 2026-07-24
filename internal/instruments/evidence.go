package instruments

import (
	"strconv"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// MetaStandIssueStore is the store surface needed to gather instrument evidence.
type MetaStandIssueStore interface {
	GetMeta(key string) (string, bool, error)
	GetStand(lawID string) (domain.StandCitation, bool, error)
	GetIssue(id string) (domain.GazetteIssue, bool, error)
}

// CollectEvidence gathers Stand / editorial / linked-instrument citation refs and matching store issues.
// Issue lookup uses canonical citation.IssueID only (no full-catalog scan).
func CollectEvidence(st MetaStandIssueStore, linked []domain.LinkedInstrument, lawID string, stand domain.StandCitation) ([]domain.InstrumentRef, []domain.GazetteIssue) {
	var refs []domain.InstrumentRef
	refs = append(refs, citation.ParseLinkedInstruments(stand.Raw)...)
	if blob, ok, _ := st.GetMeta("editorial:" + lawID); ok && blob != "" {
		refs = append(refs, citation.ParseLinkedInstruments(blob)...)
	}
	for _, li := range linked {
		refs = append(refs, citation.ParseLinkedInstruments(li.Notes)...)
		childID := normalize.Key(li.LawID)
		if childID == "" {
			childID = normalize.Key(li.GIISlug)
		}
		if childStand, ok, _ := st.GetStand(childID); ok {
			refs = append(refs, citation.ParseLinkedInstruments(childStand.Raw)...)
		}
	}
	refs = dedupeRefs(refs)

	var issues []domain.GazetteIssue
	seen := map[string]struct{}{}
	for _, ref := range refs {
		if ref.Year == 0 || ref.Number == "" {
			continue
		}
		id := citation.IssueID(ref.Teil, ref.Year, ref.Number)
		iss, ok, _ := st.GetIssue(id)
		if !ok {
			continue
		}
		if _, dup := seen[iss.ID]; dup {
			continue
		}
		seen[iss.ID] = struct{}{}
		issues = append(issues, iss)
	}
	return refs, issues
}

// ForParentSafe returns seeded instruments; nil catalog → nil.
func ForParentSafe(cat *Catalog, lawID string) []domain.LinkedInstrument {
	if cat == nil {
		return nil
	}
	return cat.ForParent(lawID)
}

func dedupeRefs(in []domain.InstrumentRef) []domain.InstrumentRef {
	seen := map[string]struct{}{}
	var out []domain.InstrumentRef
	for _, r := range in {
		k := r.Kind + "|" + strconv.Itoa(r.Teil) + "|" + strconv.Itoa(r.Year) + "|" + r.Number
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, r)
	}
	return out
}
