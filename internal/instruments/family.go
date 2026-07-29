package instruments

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

var (
	reSafePrefix = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	reYearSuffix = regexp.MustCompile(`_(\d{4})$`)
)

// FamilyRow is one Fortschreibung family mapping (consumer → slug prefix).
type FamilyRow struct {
	ConsumerParentID string
	SlugPrefix       string
	SectionHint      string
	Notes            string
}

// FamilyCatalog holds consumer_parent_id → Fortschreibung family rows.
type FamilyCatalog struct {
	byParent map[string][]FamilyRow
}

// LoadFamiliesTSV reads consumer_parent_id\tslug_prefix\tsection_hint\tnotes lines.
// Notes must contain at least one parseable BGBl citation.
// Empty path returns an empty catalog. A configured but missing path is an error.
func LoadFamiliesTSV(path string) (*FamilyCatalog, error) {
	c := &FamilyCatalog{byParent: map[string][]FamilyRow{}}
	if path == "" {
		return c, nil
	}
	f, err := os.Open(path) // #nosec G304 -- path is operator-controlled config (families TSV)
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
		if len(parts) < 4 {
			return nil, fmt.Errorf("%s:%d: want consumer_parent_id\\tslug_prefix\\tsection_hint\\tnotes", path, lineNo)
		}
		row := FamilyRow{
			ConsumerParentID: normalize.Key(strings.TrimSpace(parts[0])),
			SlugPrefix:       strings.TrimSpace(parts[1]),
			SectionHint:      strings.TrimSpace(parts[2]),
			Notes:            strings.TrimSpace(parts[3]),
		}
		if row.ConsumerParentID == "" {
			return nil, fmt.Errorf("%s:%d: empty consumer_parent_id", path, lineNo)
		}
		if row.SlugPrefix == "" {
			return nil, fmt.Errorf("%s:%d: empty slug_prefix", path, lineNo)
		}
		if !reSafePrefix.MatchString(row.SlugPrefix) {
			return nil, fmt.Errorf("%s:%d: unsafe slug_prefix %q", path, lineNo, row.SlugPrefix)
		}
		if len(citation.ParseLinkedInstruments(row.Notes)) == 0 {
			return nil, fmt.Errorf("%s:%d: notes must include a BGBl citation (year + I/II + Nr.) for fail-safe freshness", path, lineNo)
		}
		c.byParent[row.ConsumerParentID] = append(c.byParent[row.ConsumerParentID], row)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// ForParent returns family rows for a consumer law id (normalized key).
func (c *FamilyCatalog) ForParent(lawID string) []FamilyRow {
	if c == nil {
		return nil
	}
	rows := c.byParent[normalize.Key(lawID)]
	out := make([]FamilyRow, len(rows))
	copy(out, rows)
	return out
}

// Len returns the number of consumer parents with family rows.
func (c *FamilyCatalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.byParent)
}

// LatestSlugByPrefix picks the law GIIPath with the highest year matching prefix.
// Matches paths equal to prefix+YYYY or starting with prefix and ending in _YYYY.
func LatestSlugByPrefix(laws []domain.Law, prefix string) (slug string, ok bool) {
	if prefix == "" || len(laws) == 0 {
		return "", false
	}
	bestYear := 0
	for _, law := range laws {
		path := strings.TrimSpace(law.GIIPath)
		if path == "" {
			continue
		}
		year, match := yearFromPrefixPath(path, prefix)
		if !match || year <= bestYear {
			continue
		}
		bestYear = year
		slug = path
		ok = true
	}
	return slug, ok
}

func yearFromPrefixPath(path, prefix string) (year int, ok bool) {
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	suffix := strings.TrimPrefix(path, prefix)
	if len(suffix) == 4 {
		if y, err := strconv.Atoi(suffix); err == nil && y >= 1000 && y <= 9999 {
			return y, true
		}
	}
	if m := reYearSuffix.FindStringSubmatch(path); len(m) == 2 {
		if y, err := strconv.Atoi(m[1]); err == nil && y >= 1000 && y <= 9999 {
			return y, true
		}
	}
	return 0, false
}

// ExpandForParent resolves latest catalog slugs for family rows of a consumer parent.
func (c *FamilyCatalog) ExpandForParent(parentLawID string, laws []domain.Law) []domain.LinkedInstrument {
	if c == nil {
		return nil
	}
	rows := c.ForParent(parentLawID)
	if len(rows) == 0 {
		return nil
	}
	out := make([]domain.LinkedInstrument, 0, len(rows))
	for _, row := range rows {
		slug, ok := LatestSlugByPrefix(laws, row.SlugPrefix)
		if !ok || !giiurl.ValidSlug(slug) {
			continue
		}
		out = append(out, domain.LinkedInstrument{
			ParentLawID: normalize.Key(parentLawID),
			Kind:        "verordnung",
			GIISlug:     slug,
			Notes:       row.Notes,
			SectionHint: row.SectionHint,
			Coverage:    CoverageSection,
		})
	}
	return out
}

// ExpandForParentSafe returns expanded linked instruments; nil catalog → nil.
func ExpandForParentSafe(cat *FamilyCatalog, parentLawID string, laws []domain.Law) []domain.LinkedInstrument {
	if cat == nil {
		return nil
	}
	return cat.ExpandForParent(parentLawID, laws)
}
