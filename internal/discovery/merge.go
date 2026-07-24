package discovery

import (
	"sort"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// EdgeToLinked maps a persisted discovered edge to a LinkedInstrument for merge/API use.
func EdgeToLinked(e domain.DiscoveredEdge) domain.LinkedInstrument {
	kind := "verordnung"
	return domain.LinkedInstrument{
		ParentLawID:   e.ParentLawID,
		Kind:          kind,
		GIISlug:       e.GIISlug,
		SectionHint:   e.SectionHint,
		Notes:         e.Notes,
		EffectiveFrom: e.EffectiveFrom,
		LawID:         e.ChildLawID,
		EdgeType:      e.EdgeType,
		Confidence:    e.Confidence,
		Source:        SourceDiscovered,
	}
}

// Merge combines seeded and high-confidence discovered linked instruments.
// Seeded rows win on key collision (normalize.Key(parent)|gii_slug).
func Merge(seeded []domain.LinkedInstrument, discovered []domain.LinkedInstrument) []domain.LinkedInstrument {
	byKey := make(map[string]domain.LinkedInstrument, len(seeded)+len(discovered))

	for _, li := range seeded {
		li.Source = SourceSeeded
		byKey[linkKey(li.ParentLawID, li.GIISlug)] = li
	}

	for _, li := range discovered {
		if li.Confidence != ConfidenceHigh {
			continue
		}
		k := linkKey(li.ParentLawID, li.GIISlug)
		if _, exists := byKey[k]; exists {
			continue
		}
		if li.Source == "" {
			li.Source = SourceDiscovered
		}
		byKey[k] = li
	}

	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]domain.LinkedInstrument, 0, len(keys))
	for _, k := range keys {
		out = append(out, byKey[k])
	}
	return out
}

func linkKey(parent, slug string) string {
	return normalize.Key(parent) + "|" + slug
}
