package discovery

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/xmlsafe"
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
// Strong matches win over weak ones:
//  1. exact title, phrase-contains-full-title, or title words covered by phrase (inflection-tolerant)
//  2. else title-contains-phrase
//
// Both tiers skip Verordnungen (citing ordinances are not Ermächtigung parents).
func (c CatalogLookup) ByTitlePhrase(phrase string) []string {
	key := normalize.Key(phrase)
	if len(key) < 12 {
		// Avoid matching ubiquitous tokens like "sozialgesetzbuch".
		return nil
	}
	var strong, weak []string
	seenStrong := map[string]struct{}{}
	seenWeak := map[string]struct{}{}
	add := func(dst *[]string, seen map[string]struct{}, id string) {
		id = normalize.Key(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		*dst = append(*dst, id)
	}
	for _, law := range c.Laws {
		titleKey := normalize.Key(law.Title)
		if len(titleKey) < 8 {
			continue
		}
		if titleKey == key ||
			(len(titleKey) >= 12 && strings.Contains(key, titleKey)) ||
			titleWordsCoveredByPhrase(law.Title, phrase) {
			if LooksLikeVerordnung(law) {
				continue
			}
			add(&strong, seenStrong, law.ID)
			continue
		}
		if strings.Contains(titleKey, key) && !LooksLikeVerordnung(law) {
			add(&weak, seenWeak, law.ID)
		}
	}
	if len(strong) > 0 {
		return strong
	}
	return weak
}

// titleWordsCoveredByPhrase reports whether every substantive word of title
// appears (with light German inflection folding) as a word in phrase.
// Short statute titles ("Bürgerliches Gesetzbuch") match genitive phrases
// ("Bürgerlichen Gesetzbuchs"); long citing ordinance titles do not.
func titleWordsCoveredByPhrase(title, phrase string) bool {
	titleWords := letterWords(title)
	if len(titleWords) == 0 {
		return false
	}
	phraseStems := map[string]struct{}{}
	for _, w := range letterWords(phrase) {
		phraseStems[germanStemKey(normalize.Key(w))] = struct{}{}
	}
	matched := 0
	for _, w := range titleWords {
		k := normalize.Key(w)
		if len(k) < 4 {
			continue
		}
		stem := germanStemKey(k)
		if _, ok := phraseStems[stem]; !ok {
			return false
		}
		matched++
	}
	return matched > 0
}

func letterWords(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// germanStemKey lightly folds common German adjective/noun endings on a
// normalize.Key string so "buergerliches" and "buergerlichen" share a stem.
func germanStemKey(k string) string {
	if len(k) < 5 {
		return k
	}
	for _, suf := range []string{"lichen", "liches", "ischen", "isches", "ischem", "ischer", "en", "em", "er", "es", "e", "s"} {
		if strings.HasSuffix(k, suf) && len(k)-len(suf) >= 4 {
			return k[:len(k)-len(suf)]
		}
	}
	return k
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

// ExtractPreambleText streams XML CharData and returns text containing an Ermächtigung
// marker ("Auf Grund" / "aufgrund"), or the first ~8KB of norm text content when no
// marker is found. Marker-bearing <fussnoten> within <norm> wins over <text> so body
// phrases like "auf Grund eines Vertrages" do not mask historical footnote Ermächtigungen.
func ExtractPreambleText(xmlData []byte) string {
	if err := xmlsafe.RejectUnsafeXML(xmlData); err != nil {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var inNorm, inText, inFussnoten bool
	var textBuf, fussnotenBuf, collected strings.Builder
	textMarker, fussnotenMarker := false, false

	appendChunk := func(b *strings.Builder, s string) {
		if b.Len() >= preambleScanLimit {
			return
		}
		chunk := s
		remain := preambleScanLimit - b.Len()
		if len(chunk) > remain {
			chunk = chunk[:remain]
		}
		b.WriteString(chunk)
		b.WriteByte(' ')
	}

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
			case "fussnoten":
				if inNorm {
					inFussnoten = true
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "text":
				inText = false
			case "fussnoten":
				inFussnoten = false
			case "norm":
				inNorm = false
			}
		case xml.CharData:
			if !inNorm {
				continue
			}
			s := strings.TrimSpace(string(t))
			if s == "" {
				continue
			}
			switch {
			case inFussnoten:
				if isErmaechtigungMarker(s) {
					fussnotenMarker = true
				}
				if fussnotenMarker {
					appendChunk(&fussnotenBuf, s)
				}
			case inText:
				if collected.Len() < preambleScanLimit {
					appendChunk(&collected, s)
				}
				if isErmaechtigungMarker(s) {
					textMarker = true
				}
				if textMarker {
					appendChunk(&textBuf, s)
				}
			}
		}
	}
	if fussnotenMarker {
		return strings.TrimSpace(fussnotenBuf.String())
	}
	if textMarker {
		return strings.TrimSpace(textBuf.String())
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
	if err := xmlsafe.RejectUnsafeXML(xmlData); err != nil {
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

	// Aggregate § hints per uniquely resolved parent so multi-§ segments do not
	// overwrite each other on the parent|slug store key.
	type parentAgg struct {
		hints []string
		raw   string
	}
	byParent := map[string]*parentAgg{}
	hintSeen := map[string]map[string]struct{}{}

	for _, e := range refs {
		parentID, parentUnique := ResolveParent(e, lookup)
		// Title fallback is only for abbreviated fussnoten ("d. § … G v.") with no
		// explicit parent phrase or jurabk. Do not mask ambiguous explicit phrases.
		if (parentID == "" || !parentUnique) &&
			strings.TrimSpace(e.LawTitlePhrase) == "" &&
			strings.TrimSpace(e.Jurabk) == "" {
			parentID, parentUnique = ResolveParentFromChildTitle(law.Title, lookup)
		}
		if parentID == "" || !parentUnique {
			continue
		}
		agg := byParent[parentID]
		if agg == nil {
			agg = &parentAgg{raw: e.Raw}
			byParent[parentID] = agg
			hintSeen[parentID] = map[string]struct{}{}
		}
		if h := e.SectionHint(); h != "" {
			if _, ok := hintSeen[parentID][h]; !ok {
				hintSeen[parentID][h] = struct{}{}
				agg.hints = append(agg.hints, h)
			}
		}
	}

	// When body § refs lack a resolvable parent phrase (e.g. UhAnpV "des Gesetzes"),
	// fall back once to the Verordnung title ("nach dem …gesetz"). Skip when refs
	// carried explicit but unresolved parent phrases (ambiguous Ermächtigung).
	if len(byParent) == 0 {
		hadExplicitPhrase := false
		for _, e := range refs {
			if strings.TrimSpace(e.LawTitlePhrase) != "" || strings.TrimSpace(e.Jurabk) != "" {
				hadExplicitPhrase = true
				break
			}
		}
		if !hadExplicitPhrase {
			parentID, parentUnique := ResolveParentFromChildTitle(law.Title, lookup)
			if parentUnique && parentID != "" {
				agg := &parentAgg{}
				hintSeen[parentID] = map[string]struct{}{}
				for _, e := range refs {
					if h := e.SectionHint(); h != "" {
						if _, ok := hintSeen[parentID][h]; !ok {
							hintSeen[parentID][h] = struct{}{}
							agg.hints = append(agg.hints, h)
						}
					}
				}
				byParent[parentID] = agg
			}
		}
	}

	var toPersist []domain.DiscoveredEdge
	for parentID, agg := range byParent {
		sectionHint := strings.Join(agg.hints, "; ")
		confidence := ScoreConfidence(true, sectionHint, fundstelleOK, false)
		// Persist only high-confidence edges (medium/low are parse noise for metrics).
		if confidence != ConfidenceHigh {
			continue
		}

		notes := fundNotes
		if notes == "" {
			notes = agg.raw
		}

		// Do not map ausfertigung-datum onto EffectiveFrom: that field is Inkrafttreten.
		// Ausfertigung often precedes in-force (e.g. MiLoV5 2025-11-05 vs 2026-01-01).
		// Leave empty so AnnotateChain keeps Status=""; Phase B empty-status match +
		// FilterBundleMembers (discovered+high) still cover undated high-confidence edges.
		toPersist = append(toPersist, domain.DiscoveredEdge{
			ParentLawID: parentID,
			GIISlug:     law.GIIPath,
			SectionHint: sectionHint,
			Notes:       notes,
			EdgeType:    EdgeErmaechtigung,
			Confidence:  confidence,
			ChildLawID:  law.ID,
		})
	}

	// Delete prior edges only when we have high replacements (avoid wipe-on-noise).
	if len(toPersist) > 0 {
		if err := st.DeleteDiscoveredBySlug(law.GIIPath); err != nil {
			return 0, err
		}
	}
	for _, edge := range toPersist {
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
