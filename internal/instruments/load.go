// Package instruments loads seeded parent→ordinance mappings (amendment-by-reference).
package instruments

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// Catalog holds parent_law_id → linked instruments.
type Catalog struct {
	byParent map[string][]domain.LinkedInstrument
}

// LoadTSV reads parent_law_id\tkind\tgii_slug\tnotes lines.
// Notes must contain at least one parseable BGBl citation (the fail-safe signal).
// Empty path returns an empty catalog. A configured but missing path is an error.
func LoadTSV(path string) (*Catalog, error) {
	c := &Catalog{byParent: map[string][]domain.LinkedInstrument{}}
	if path == "" {
		return c, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			return nil, fmt.Errorf("%s:%d: want parent\\tkind\\tslug[\\tnotes]", path, lineNo)
		}
		li := domain.LinkedInstrument{
			ParentLawID: normalize.Key(strings.TrimSpace(parts[0])),
			Kind:        strings.TrimSpace(parts[1]),
			GIISlug:     strings.TrimSpace(parts[2]),
		}
		if !giiurl.ValidSlug(li.GIISlug) {
			return nil, fmt.Errorf("%s:%d: invalid gii_slug %q", path, lineNo, li.GIISlug)
		}
		if len(parts) >= 4 {
			li.Notes = strings.TrimSpace(parts[3])
		}
		if len(parts) >= 5 {
			li.EffectiveFrom = strings.TrimSpace(parts[4])
			if li.EffectiveFrom != "" {
				if _, err := time.Parse("2006-01-02", li.EffectiveFrom); err != nil {
					return nil, fmt.Errorf("%s:%d: invalid effective_from %q (want YYYY-MM-DD)", path, lineNo, li.EffectiveFrom)
				}
			}
		}
		if len(parts) >= 6 {
			li.SectionHint = strings.TrimSpace(parts[5])
		}
		li.Coverage = CoverageSection
		if li.ParentLawID == "" || li.GIISlug == "" {
			continue
		}
		if len(citation.ParseLinkedInstruments(li.Notes)) == 0 {
			return nil, fmt.Errorf("%s:%d: notes must include a BGBl citation (year + I/II + Nr.) for fail-safe freshness", path, lineNo)
		}
		c.byParent[li.ParentLawID] = append(c.byParent[li.ParentLawID], li)
	}
	return c, sc.Err()
}

// ForParent returns seeded instruments for a law id (normalized key).
func (c *Catalog) ForParent(lawID string) []domain.LinkedInstrument {
	if c == nil {
		return nil
	}
	return append([]domain.LinkedInstrument(nil), c.byParent[normalize.Key(lawID)]...)
}

// Len returns the number of parent laws with seeded instruments.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.byParent)
}
