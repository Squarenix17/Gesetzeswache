package discovery

import (
	"reflect"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func discoveredLI(slug string) domain.LinkedInstrument {
	return domain.LinkedInstrument{
		ParentLawID: "sgb6",
		GIISlug:     slug,
		Kind:        "verordnung",
		Source:      SourceDiscovered,
		Confidence:  ConfidenceHigh,
	}
}

func TestFilterDiscoveredByFamilyPrefixes(t *testing.T) {
	keepBSV2018 := map[string]struct{}{"bsv_2018": {}}

	tests := []struct {
		name       string
		discovered []domain.LinkedInstrument
		prefixes   []string
		keepSlugs  map[string]struct{}
		wantSlugs  []string
	}{
		{
			name: "drops historical bsv unless in keepSlugs",
			discovered: []domain.LinkedInstrument{
				discoveredLI("bsv_2015"),
				discoveredLI("bsv_2018"),
			},
			prefixes:  []string{"bsv_"},
			keepSlugs: keepBSV2018,
			wantSlugs: []string{"bsv_2018"},
		},
		{
			name: "keeps non-matching prefix discovered",
			discovered: []domain.LinkedInstrument{
				discoveredLI("svbezgrv_2025"),
			},
			prefixes:  []string{"bsv_"},
			keepSlugs: keepBSV2018,
			wantSlugs: []string{"svbezgrv_2025"},
		},
		{
			name: "mixed bsv family and unrelated slugs",
			discovered: []domain.LinkedInstrument{
				discoveredLI("bsv_2015"),
				discoveredLI("bsv_2018"),
				discoveredLI("svbezgrv_2025"),
			},
			prefixes:  []string{"bsv_"},
			keepSlugs: keepBSV2018,
			wantSlugs: []string{"bsv_2018", "svbezgrv_2025"},
		},
		{
			name: "empty prefixes returns all discovered",
			discovered: []domain.LinkedInstrument{
				discoveredLI("bsv_2015"),
				discoveredLI("bsv_2018"),
			},
			prefixes:  nil,
			keepSlugs: keepBSV2018,
			wantSlugs: []string{"bsv_2015", "bsv_2018"},
		},
		{
			name: "empty keepSlugs drops all prefix matches",
			discovered: []domain.LinkedInstrument{
				discoveredLI("bsv_2015"),
				discoveredLI("bsv_2018"),
				discoveredLI("svbezgrv_2025"),
			},
			prefixes:  []string{"bsv_"},
			keepSlugs: map[string]struct{}{},
			wantSlugs: []string{"svbezgrv_2025"},
		},
		{
			name: "nil keepSlugs drops all prefix matches",
			discovered: []domain.LinkedInstrument{
				discoveredLI("bsv_2018"),
				discoveredLI("svbezgrv_2025"),
			},
			prefixes:  []string{"bsv_"},
			keepSlugs: nil,
			wantSlugs: []string{"svbezgrv_2025"},
		},
		{
			name:       "nil discovered returns empty",
			discovered: nil,
			prefixes:   []string{"bsv_"},
			keepSlugs:  keepBSV2018,
			wantSlugs:  nil,
		},
		{
			name:       "empty discovered returns empty",
			discovered: []domain.LinkedInstrument{},
			prefixes:   []string{"bsv_"},
			keepSlugs:  keepBSV2018,
			wantSlugs:  nil,
		},
		{
			name: "multi-prefix bsv and rvbeitrsbek keeps latest of each",
			discovered: []domain.LinkedInstrument{
				discoveredLI("bsv_2015"),
				discoveredLI("bsv_2018"),
				discoveredLI("rvbeitrsbek_2024"),
				discoveredLI("rvbeitrsbek_2026"),
				discoveredLI("svbezgrv_2025"),
			},
			prefixes: []string{"bsv_", "rvbeitrsbek_"},
			keepSlugs: map[string]struct{}{
				"bsv_2018":         {},
				"rvbeitrsbek_2026": {},
			},
			wantSlugs: []string{"bsv_2018", "rvbeitrsbek_2026", "svbezgrv_2025"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterDiscoveredByFamilyPrefixes(tt.discovered, tt.prefixes, tt.keepSlugs)
			gotSlugs := slugsFromLinked(got)
			if !reflect.DeepEqual(gotSlugs, tt.wantSlugs) {
				t.Fatalf("slugs=%v want %v (full=%+v)", gotSlugs, tt.wantSlugs, got)
			}
		})
	}
}

func TestFilterDiscoveredByFamilyPrefixes_doesNotMutateInput(t *testing.T) {
	original := []domain.LinkedInstrument{
		discoveredLI("bsv_2015"),
		discoveredLI("bsv_2018"),
	}
	snapshot := append([]domain.LinkedInstrument(nil), original...)

	_ = FilterDiscoveredByFamilyPrefixes(original, []string{"bsv_"}, map[string]struct{}{"bsv_2018": {}})

	if !reflect.DeepEqual(original, snapshot) {
		t.Fatalf("input mutated: before=%+v after=%+v", snapshot, original)
	}
}

func slugsFromLinked(rows []domain.LinkedInstrument) []string {
	if len(rows) == 0 {
		return nil
	}
	out := make([]string, len(rows))
	for i, li := range rows {
		out[i] = li.GIISlug
	}
	return out
}
