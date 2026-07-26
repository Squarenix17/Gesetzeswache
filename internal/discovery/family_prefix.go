package discovery

import (
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

// FilterDiscoveredByFamilyPrefixes drops discovered links whose GIISlug shares a family
// prefix unless that exact slug is in keepSlugs (seeded latest expansions).
// Does not mutate the input slice; returns a new slice.
func FilterDiscoveredByFamilyPrefixes(discovered []domain.LinkedInstrument, prefixes []string, keepSlugs map[string]struct{}) []domain.LinkedInstrument {
	if len(discovered) == 0 {
		return nil
	}
	if len(prefixes) == 0 {
		out := make([]domain.LinkedInstrument, len(discovered))
		copy(out, discovered)
		return out
	}
	out := make([]domain.LinkedInstrument, 0, len(discovered))
	for _, li := range discovered {
		if shouldDropDiscoveredByFamilyPrefix(li.GIISlug, prefixes, keepSlugs) {
			continue
		}
		out = append(out, li)
	}
	return out
}

func shouldDropDiscoveredByFamilyPrefix(slug string, prefixes []string, keepSlugs map[string]struct{}) bool {
	for _, prefix := range prefixes {
		if prefix == "" || !strings.HasPrefix(slug, prefix) {
			continue
		}
		if _, keep := keepSlugs[slug]; keep {
			return false
		}
		return true
	}
	return false
}
