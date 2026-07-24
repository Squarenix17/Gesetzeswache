package discovery

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
	"github.com/Squarenix17/gesetzeswache/internal/store"
)

// IngestStore persists BGBl index rows and discovered parent→child edges.
type IngestStore interface {
	UpsertBGBlIndex(e store.BGBlIndexEntry) error
	UpsertDiscoveredLink(e domain.DiscoveredEdge) error
	DeleteDiscoveredBySlug(slug string) error
}

// CatalogLookup implements ParentLookup from catalog laws and variants.
type CatalogLookup struct {
	Laws     []domain.Law
	Variants []domain.LawVariant
}

var (
	reVerordnungSlug = regexp.MustCompile(`(?i)(?:^|_)v(?:\d|_|$)|(?:^|_)v\d{4}|milov\d|pbav_\d{4}`)
	reEndsWithV      = regexp.MustCompile(`(?i)v$`)
)

// ByJurabk returns law IDs matching a jurabk or abbreviation.
// SGB roman/arabic aliases are tried (SGB XI ↔ SGB 11 ↔ SGB_11).
func (c CatalogLookup) ByJurabk(jurabk string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(id string) {
		id = normalize.Key(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, candidate := range jurabkLookupKeys(jurabk) {
		for _, law := range c.Laws {
			if normalize.Key(law.Abbreviation) == candidate || normalize.Key(law.ID) == candidate {
				add(law.ID)
			}
		}
		for _, v := range c.Variants {
			if normalize.Key(v.Variant) == candidate {
				add(v.LawID)
			}
		}
	}
	return out
}

func jurabkLookupKeys(jurabk string) []string {
	base := strings.TrimSpace(jurabk)
	if base == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		k := normalize.Key(s)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	add(base)
	upper := strings.ToUpper(base)
	if strings.HasPrefix(upper, "SGB") {
		rest := strings.TrimSpace(base[3:])
		rest = strings.TrimLeft(rest, " _-")
		if rest != "" {
			add("SGB " + rest)
			add("SGB_" + rest)
			if n, ok := romanToInt(rest); ok {
				add(fmt.Sprintf("SGB %d", n))
				add(fmt.Sprintf("SGB_%d", n))
			}
			if n, err := strconv.Atoi(rest); err == nil {
				if r, ok := intToRoman(n); ok {
					add("SGB " + r)
					add("SGB_" + r)
				}
			}
		}
	}
	return out
}

func romanToInt(s string) (int, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" || strings.IndexFunc(s, func(r rune) bool {
		return !strings.ContainsRune("IVXLCDM", r)
	}) >= 0 {
		return 0, false
	}
	values := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	total := 0
	prev := 0
	for i := len(s) - 1; i >= 0; i-- {
		v := values[s[i]]
		if v < prev {
			total -= v
		} else {
			total += v
		}
		prev = v
	}
	if total <= 0 {
		return 0, false
	}
	return total, true
}

func intToRoman(n int) (string, bool) {
	if n <= 0 || n > 20 {
		return "", false
	}
	vals := []int{10, 9, 5, 4, 1}
	syms := []string{"X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String(), true
}

// ByTitlePhrase returns law IDs whose title matches the phrase.
// Only exact title match, title-contains-phrase, or phrase-contains-full-title
// when the title token is substantive (avoids short false positives).
func (c CatalogLookup) ByTitlePhrase(phrase string) []string {
	key := normalize.Key(phrase)
	if len(key) < 12 {
		// Avoid matching ubiquitous tokens like "sozialgesetzbuch".
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, law := range c.Laws {
		titleKey := normalize.Key(law.Title)
		if len(titleKey) < 8 {
			continue
		}
		match := titleKey == key || strings.Contains(titleKey, key)
		// Genitive phrases ("Berufsbildungsgesetzes") contain the nominative title.
		if !match && len(titleKey) >= 12 && strings.Contains(key, titleKey) {
			match = true
		}
		if !match {
			continue
		}
		id := normalize.Key(law.ID)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// LooksLikeVerordnung reports whether a law is likely a Verordnung.
func LooksLikeVerordnung(law domain.Law) bool {
	if strings.Contains(strings.ToLower(law.Title), "verordnung") {
		return true
	}
	abbr := strings.TrimSpace(law.Abbreviation)
	if abbr != "" && reEndsWithV.MatchString(abbr) {
		return true
	}
	slug := strings.ToLower(strings.TrimSpace(law.GIIPath))
	if slug != "" && reVerordnungSlug.MatchString(slug) {
		return true
	}
	return false
}

const preambleScanLimit = 8 << 10 // ~8KB

func rejectUnsafeXML(xmlData []byte) error {
	// GII norms declare an external SYSTEM DTD; that is fine.
	// Reject internal entity declarations (billion-laughs style expansion).
	lower := bytes.ToLower(xmlData)
	if bytes.Contains(lower, []byte("<!entity")) {
		return fmt.Errorf("xml contains entity declarations")
	}
	return nil
}

// ExtractPreambleText streams XML CharData and returns text containing an Ermächtigung
// marker ("Auf Grund" / "aufgrund"), or the first ~8KB of norm text content when no
// marker is found.
func ExtractPreambleText(xmlData []byte) string {
	if err := rejectUnsafeXML(xmlData); err != nil {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var inNorm, inText bool
	var buf strings.Builder
	var collected strings.Builder
	foundMarker := false

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "norm":
				inNorm = true
			case "text":
				if inNorm {
					inText = true
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "text":
				inText = false
			case "norm":
				inNorm = false
				if foundMarker {
					return strings.TrimSpace(buf.String())
				}
				buf.Reset()
			}
		case xml.CharData:
			if !inNorm || !inText {
				continue
			}
			s := strings.TrimSpace(string(t))
			if s == "" {
				continue
			}
			if collected.Len() < preambleScanLimit {
				chunk := s
				remain := preambleScanLimit - collected.Len()
				if len(chunk) > remain {
					chunk = chunk[:remain]
				}
				collected.WriteString(chunk)
				collected.WriteByte(' ')
			}
			if isErmaechtigungMarker(s) {
				foundMarker = true
			}
			if foundMarker && buf.Len() < preambleScanLimit {
				chunk := s
				remain := preambleScanLimit - buf.Len()
				if len(chunk) > remain {
					chunk = chunk[:remain]
				}
				buf.WriteString(chunk)
				buf.WriteByte(' ')
			}
		}
	}
	if foundMarker {
		return strings.TrimSpace(buf.String())
	}
	return strings.TrimSpace(collected.String())
}

func isErmaechtigungMarker(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "auf grund") || strings.Contains(lower, "aufgrund")
}

// FundstelleFromXML parses the first fundstelle in GII norm XML.
func FundstelleFromXML(xmlData []byte) (teil, year int, number string, ok bool) {
	raw := export.ExtractStandRaw(xmlData)
	if raw == "" {
		return 0, 0, "", false
	}
	parsed := citation.Parse("", raw)
	if !parsed.ParseOK {
		return 0, 0, "", false
	}
	return parsed.Teil, parsed.Year, parsed.Number, true
}

// IngestLawXML indexes fundstelle metadata and discovers Ermächtigung edges from preamble text.
func IngestLawXML(st IngestStore, lookup ParentLookup, law domain.Law, xmlData []byte) (nLinks int, err error) {
	if !giiurl.ValidSlug(law.GIIPath) {
		return 0, fmt.Errorf("invalid gii slug %q", law.GIIPath)
	}
	if err := rejectUnsafeXML(xmlData); err != nil {
		return 0, err
	}
	fundstelleOK := false
	var fundNotes string

	raw := export.ExtractStandRaw(xmlData)
	if raw != "" {
		parsed := citation.Parse(law.ID, raw)
		if parsed.ParseOK {
			fundstelleOK = true
			fundNotes = formatFundstelleNotes(parsed.Teil, parsed.Year, parsed.Number)
			if err := st.UpsertBGBlIndex(store.BGBlIndexEntry{
				Teil:    parsed.Teil,
				Year:    parsed.Year,
				Number:  parsed.Number,
				GIISlug: law.GIIPath,
				LawID:   law.ID,
			}); err != nil {
				return 0, err
			}
		}
	}

	preamble := ExtractPreambleText(xmlData)
	refs := ParseErmaechtigung(preamble)
	if len(refs) > 0 {
		if err := st.DeleteDiscoveredBySlug(law.GIIPath); err != nil {
			return 0, err
		}
	}
	for _, e := range refs {
		parentID, parentUnique := ResolveParent(e, lookup)
		sectionHint := e.SectionHint()
		confidence := ScoreConfidence(parentUnique, sectionHint, fundstelleOK, false)
		// Persist only high-confidence edges (medium/low are parse noise for metrics).
		if confidence != ConfidenceHigh {
			continue
		}
		if parentID == "" {
			continue
		}

		notes := fundNotes
		if notes == "" {
			notes = e.Raw
		}

		edge := domain.DiscoveredEdge{
			ParentLawID: parentID,
			GIISlug:     law.GIIPath,
			SectionHint: sectionHint,
			Notes:       notes,
			EdgeType:    EdgeErmaechtigung,
			Confidence:  confidence,
			ChildLawID:  law.ID,
		}
		if err := st.UpsertDiscoveredLink(edge); err != nil {
			return nLinks, err
		}
		nLinks++
	}
	return nLinks, nil
}

func formatFundstelleNotes(teil, year int, number string) string {
	if teil == 0 || year == 0 || number == "" {
		return ""
	}
	teilStr := "I"
	if teil == 2 {
		teilStr = "II"
	}
	return fmt.Sprintf("BGBl. %d %s Nr. %s", year, teilStr, number)
}
