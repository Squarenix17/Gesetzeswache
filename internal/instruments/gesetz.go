package instruments

import (
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// LawTitleLookup resolves child law titles for soft-FP demotion.
type LawTitleLookup interface {
	GetLaw(id string) (domain.Law, bool, error)
}

// IsGesetzChild reports whether a catalog law title denotes a Gesetz (not a Verordnung).
// Used to demote discovery false-positives labeled kind=verordnung.
func IsGesetzChild(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	if strings.HasPrefix(lower, "verordnung") {
		return false
	}
	if strings.HasPrefix(lower, "gesetz") {
		return true
	}
	if strings.HasSuffix(lower, "gesetz") {
		return true
	}
	return false
}

func childLawID(li domain.LinkedInstrument) string {
	if id := strings.TrimSpace(li.LawID); id != "" {
		return normalize.Key(id)
	}
	return normalize.Key(li.GIISlug)
}

func childTitle(st LawTitleLookup, li domain.LinkedInstrument) string {
	if st == nil {
		return ""
	}
	id := childLawID(li)
	if id == "" {
		return ""
	}
	law, ok, err := st.GetLaw(id)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(law.Title)
}

// IsSoftGesetzFP reports linked rows that are Gesetze mis-attached as operative Verordnungen.
func IsSoftGesetzFP(st LawTitleLookup, li domain.LinkedInstrument) bool {
	if strings.EqualFold(strings.TrimSpace(li.Kind), "gesetz") {
		return true
	}
	title := childTitle(st, li)
	if title == "" {
		return false
	}
	return IsGesetzChild(title)
}

// FilterOperativeLinked drops soft Gesetz false-positives from the slice used for
// HasSeededLinkedInstruments, CollectEvidence, and default include=linked presentation.
// Rows remain in discovered_links storage; only the operational boundary is filtered.
func FilterOperativeLinked(st LawTitleLookup, linked []domain.LinkedInstrument) []domain.LinkedInstrument {
	if len(linked) == 0 {
		return nil
	}
	out := make([]domain.LinkedInstrument, 0, len(linked))
	for _, li := range linked {
		if IsSoftGesetzFP(st, li) {
			continue
		}
		out = append(out, li)
	}
	return out
}
