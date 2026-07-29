// Package export builds a single IR from GII XML and emits RAG-ready formats.
package export

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- deterministic export unit IDs, not a security primitive
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/xmlsafe"
)

const (
	FormatHierarchical = "hierarchical"
	FormatChunked      = "chunked"
	FormatFlat         = "flat"
	FormatNormtext     = "normtext"
)

// UnitKind classifies an IR unit for RAG filtering.
type UnitKind string

const (
	KindPreamble       UnitKind = "preamble"
	KindSectionHeading UnitKind = "section_heading"
	KindNormtext       UnitKind = "normtext"
	KindFootnote       UnitKind = "footnote"
	KindNoise          UnitKind = "noise"
)

// Unit is one paragraph-level IR node.
type Unit struct {
	ID           string   `json:"id"`
	SectionKey   string   `json:"section_key"`
	SectionRef   string   `json:"section_ref,omitempty"`
	SectionTitle string   `json:"section_title"`
	ParagraphNum string   `json:"paragraph_num"`
	Text         string   `json:"text"`
	Kind         UnitKind `json:"kind"`
	Ambiguous    bool     `json:"ambiguous,omitempty"`
}

// IR is the single intermediate representation for one law export.
type IR struct {
	LawID               string
	ContentID           string
	Title               string
	Abbreviation        string
	Units               []Unit
	StructuralAmbiguity bool
	BuiltAt             time.Time
}

// Cache is an ephemeral in-process IR cache.
type Cache struct {
	mu      sync.Mutex
	max     int
	entries map[string]IR // key: lawID+"|"+contentID
}

func NewCache(max int) *Cache {
	if max <= 0 {
		max = 64
	}
	return &Cache{max: max, entries: map[string]IR{}}
}

func cacheKey(lawID, contentID string) string {
	return lawID + "|" + contentID
}

func (c *Cache) Get(lawID, contentID string) (IR, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ir, ok := c.entries[cacheKey(lawID, contentID)]
	return ir, ok
}

func (c *Cache) Put(ir IR) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		n := 0
		for k := range c.entries {
			delete(c.entries, k)
			n++
			if n >= c.max/2 {
				break
			}
		}
	}
	c.entries[cacheKey(ir.LawID, ir.ContentID)] = ir
}

func (c *Cache) InvalidateLaw(lawID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if strings.HasPrefix(k, lawID+"|") {
			delete(c.entries, k)
		}
	}
}

// ContentIDFromStand builds cache content identity from Stand.
func ContentIDFromStand(stand domain.StandCitation) string {
	if stand.Year == 0 && stand.Number == "" && stand.Page == "" && stand.Teil == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d/%d/%s/%s", stand.Teil, stand.Year, stand.Number, stand.Page)
}

// BuildIR parses GII norm XML into IR. Text content is preserved from source nodes
// (whitespace normalized between runs; never paraphrased).
func BuildIR(law domain.Law, contentID string, xmlData []byte) (IR, error) {
	units, amb, title, err := extractUnits(law.ID, law.Abbreviation, xmlData)
	if err != nil {
		return IR{}, err
	}
	if title == "" {
		title = law.Title
	}
	return IR{
		LawID:               law.ID,
		ContentID:           contentID,
		Title:               title,
		Abbreviation:        law.Abbreviation,
		Units:               units,
		StructuralAmbiguity: amb,
		BuiltAt:             time.Now().UTC(),
	}, nil
}

var (
	reSectionHeading = regexp.MustCompile(`^\d{2,}[A-ZÄÖÜ]`)
	reSectionRef     = regexp.MustCompile(`(?i)^§\s*\d+[a-z]?`)
	reFootnote       = regexp.MustCompile(`(?i)^(fn|fußnote|fussnote)\b`)
)

func classifyUnit(text, abbr, sectionRef string, isGliederung bool) UnitKind {
	t := strings.TrimSpace(text)
	if t == "" {
		return KindNoise
	}
	if abbr != "" && t == abbr {
		return KindNoise
	}
	if strings.Contains(t, "(+++") {
		return KindPreamble
	}
	if strings.EqualFold(t, "Inhaltsübersicht") {
		return KindPreamble
	}
	if !reSectionRef.MatchString(sectionRef) && (strings.HasPrefix(t, "Stand ") || strings.HasPrefix(t, "Stand:")) {
		return KindPreamble
	}
	if isGliederung || reSectionHeading.MatchString(t) {
		return KindSectionHeading
	}
	if reFootnote.MatchString(t) {
		return KindFootnote
	}
	if reSectionRef.MatchString(sectionRef) {
		return KindNormtext
	}
	// Citation-only orphans (no § context) — not operative normtext.
	if strings.Contains(t, "BGBl") || strings.Contains(t, "CELEX") {
		return KindPreamble
	}
	if len([]rune(t)) <= 8 && !strings.Contains(t, " ") {
		return KindNoise
	}
	return KindNormtext
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func appendCharData(buf *strings.Builder, last *rune, s string) {
	if s == "" {
		return
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		if buf.Len() > 0 && *last != 0 && !unicode.IsSpace(*last) {
			buf.WriteByte(' ')
			*last = ' '
		}
		return
	}
	first, _ := utf8.DecodeRuneInString(s)
	if buf.Len() > 0 && *last != 0 {
		if !unicode.IsSpace(*last) && !unicode.IsSpace(first) && !isClosingPunct(first) {
			// Insert space between adjacent text runs (GII often omits it).
			if !isOpeningPunct(*last) {
				buf.WriteByte(' ')
				*last = ' '
			}
		}
	}
	buf.WriteString(s)
	*last, _ = utf8.DecodeLastRuneInString(s)
}

func isClosingPunct(r rune) bool {
	switch r {
	case ',', '.', ';', ':', '!', '?', ')', ']', '}', '»', '"', '\'':
		return true
	}
	return false
}

func isOpeningPunct(r rune) bool {
	switch r {
	case '(', '[', '{', '«', '"', '\'':
		return true
	}
	return false
}

func extractUnits(lawID, abbr string, data []byte) ([]Unit, bool, string, error) {
	if err := xmlsafe.RejectUnsafeXML(data); err != nil {
		return nil, false, "", err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var units []Unit
	ambiguous := false
	docTitle := ""

	var stack []string
	in := func(name string) bool {
		for _, s := range stack {
			if s == name {
				return true
			}
		}
		return false
	}

	var (
		sectionRef, sectionTitle, sectionKey string
		isGliederung                         bool
		paraNum                              string
		inP                                  bool
		textBuf                              strings.Builder
		lastRune                             rune
		captureMeta                          string
		metaBuf                              strings.Builder
	)

	resetNorm := func() {
		sectionRef, sectionTitle, sectionKey = "", "", ""
		isGliederung = false
		paraNum = ""
		inP = false
		textBuf.Reset()
		lastRune = 0
		captureMeta = ""
		metaBuf.Reset()
	}

	emit := func(text, pnum string, glied bool, forceKind UnitKind) {
		txt := normalizeWhitespace(text)
		if txt == "" {
			return
		}
		kind := forceKind
		if kind == "" {
			kind = classifyUnit(txt, abbr, sectionRef, glied)
		}
		sk := sectionKey
		st := sectionTitle
		sr := sectionRef
		if sk == "" && sr != "" {
			sk = sr
		}
		path := sk + "/" + pnum
		amb := false
		if sk == "" && pnum == "" && sr == "" {
			ambiguous = true
			amb = true
			path = fmt.Sprintf("orphan/%d", len(units)+1)
		}
		units = append(units, Unit{
			ID:           unitID(lawID, path, txt),
			SectionKey:   sk,
			SectionRef:   sr,
			SectionTitle: st,
			ParagraphNum: pnum,
			Text:         txt,
			Kind:         kind,
			Ambiguous:    amb,
		})
	}

	flushP := func() {
		if !inP && textBuf.Len() == 0 {
			return
		}
		emit(textBuf.String(), paraNum, isGliederung, "")
		textBuf.Reset()
		lastRune = 0
		paraNum = ""
		inP = false
	}

	flushOrphanText := func() {
		if textBuf.Len() == 0 {
			return
		}
		emit(textBuf.String(), "", false, "")
		textBuf.Reset()
		lastRune = 0
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return units, ambiguous, docTitle, fmt.Errorf("parse gii xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			stack = append(stack, name)

			switch name {
			case "norm":
				flushP()
				flushOrphanText()
				resetNorm()
			case "enbez":
				captureMeta = "enbez"
				metaBuf.Reset()
				if id := attrVal(t, "id"); id != "" {
					sectionKey = id
				}
			case "titel", "title":
				if in("metadaten") {
					captureMeta = "titel"
					metaBuf.Reset()
				}
			case "langue":
				captureMeta = "langue"
				metaBuf.Reset()
			case "metangabe":
				captureMeta = "metangabe"
				metaBuf.Reset()
			case "gliederungseinheit":
				isGliederung = true
				kenn := attrVal(t, "gliederungskennzahl")
				bez := attrVal(t, "gliederungsbez")
				titel := attrVal(t, "gliederungstitel")
				if kenn != "" {
					sectionKey = kenn
				} else {
					sectionKey = "gliederung"
				}
				sectionTitle = strings.TrimSpace(strings.Join(filterEmpty(bez, titel), " — "))
				if bez != "" {
					sectionRef = bez
				}
			case "p", "absatz":
				flushOrphanText()
				captureMeta = ""
				inP = true
				paraNum = attrVal(t, "nr")
				textBuf.Reset()
				lastRune = 0
			case "content":
				flushOrphanText()
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "enbez":
				sectionRef = normalizeWhitespace(metaBuf.String())
				if sectionKey == "" {
					sectionKey = sectionRef
				}
				captureMeta = ""
				metaBuf.Reset()
			case "titel", "title":
				if captureMeta == "titel" {
					sectionTitle = normalizeWhitespace(metaBuf.String())
				}
				captureMeta = ""
				metaBuf.Reset()
			case "langue":
				if docTitle == "" {
					docTitle = normalizeWhitespace(metaBuf.String())
				}
				captureMeta = ""
				metaBuf.Reset()
			case "metangabe":
				emit(metaBuf.String(), "", false, KindPreamble)
				captureMeta = ""
				metaBuf.Reset()
			case "p", "absatz":
				flushP()
			case "content", "text", "textdaten":
				flushOrphanText()
			case "norm":
				flushP()
				flushOrphanText()
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			s := string(t)
			// Paragraph body wins over open meta capture (nested enbez/P layouts).
			if inP {
				appendCharData(&textBuf, &lastRune, s)
				continue
			}
			if captureMeta != "" {
				metaBuf.WriteString(s)
				continue
			}
			if in("metadaten") {
				continue
			}
			if in("textdaten") && strings.TrimSpace(s) != "" {
				appendCharData(&textBuf, &lastRune, s)
			}
		}
	}
	flushP()
	flushOrphanText()

	if len(units) == 0 {
		plain := normalizeWhitespace(stripTags(string(data)))
		if plain != "" {
			ambiguous = true
			units = append(units, Unit{
				ID:        unitID(lawID, "full", plain),
				Text:      plain,
				Kind:      KindNormtext,
				Ambiguous: true,
			})
		}
	}
	return units, ambiguous, docTitle, nil
}

func filterEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

func attrVal(t xml.StartElement, key string) string {
	for _, a := range t.Attr {
		if strings.EqualFold(a.Name.Local, key) && a.Value != "" {
			return a.Value
		}
	}
	return ""
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func unitID(lawID, path, text string) string {
	h := sha1.Sum([]byte(lawID + "|" + path + "|" + text)) // #nosec G401 -- non-cryptographic content-ID hash
	return lawID + "-" + hex.EncodeToString(h[:8])
}

// EmitHierarchical produces retrieval-optimized marked text.
func EmitHierarchical(ir IR) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(ir.Abbreviation)
	b.WriteString(" — ")
	b.WriteString(ir.Title)
	b.WriteByte('\n')
	curSec := ""
	for _, u := range ir.Units {
		header := u.SectionTitle
		if header == "" {
			header = u.SectionRef
		}
		if header == "" {
			header = u.SectionKey
		}
		if u.SectionKey != curSec {
			curSec = u.SectionKey
			if header != "" && !strings.HasPrefix(header, "enbez-") && !strings.HasPrefix(header, "gliederungseinheit-") {
				b.WriteString("\n## ")
				switch {
				case u.SectionTitle != "" && u.SectionRef != "" && strings.Contains(u.SectionTitle, u.SectionRef):
					b.WriteString(u.SectionTitle)
				case u.SectionRef != "" && u.SectionTitle != "" && u.SectionRef != u.SectionTitle:
					b.WriteString(u.SectionRef)
					b.WriteString(" ")
					b.WriteString(u.SectionTitle)
				default:
					b.WriteString(header)
				}
				b.WriteByte('\n')
			}
		}
		if u.Kind == KindSectionHeading {
			continue
		}
		if u.ParagraphNum != "" {
			b.WriteString("\n### Abs. ")
			b.WriteString(u.ParagraphNum)
			b.WriteByte('\n')
		}
		b.WriteString(u.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// Chunk is a vector-DB oriented payload.
type Chunk struct {
	UnitID       string   `json:"unit_id"`
	LawID        string   `json:"law_id"`
	Abbreviation string   `json:"abbreviation"`
	SectionRef   string   `json:"section_ref,omitempty"`
	ParagraphNum string   `json:"paragraph_num"`
	SectionTitle string   `json:"section_title"`
	Kind         UnitKind `json:"kind"`
	StandRaw     string   `json:"stand_raw"`
	Freshness    string   `json:"freshness_state"`
	Text         string   `json:"text"`
}

func chunkFromUnit(ir IR, u Unit, stand domain.StandCitation, fresh domain.FreshnessRecord) Chunk {
	return Chunk{
		UnitID:       u.ID,
		LawID:        ir.LawID,
		Abbreviation: ir.Abbreviation,
		SectionRef:   u.SectionRef,
		ParagraphNum: u.ParagraphNum,
		SectionTitle: u.SectionTitle,
		Kind:         u.Kind,
		StandRaw:     stand.Raw,
		Freshness:    string(fresh.State),
		Text:         u.Text,
	}
}

// EmitChunked builds chunk payloads with identical unit boundaries.
func EmitChunked(ir IR, stand domain.StandCitation, fresh domain.FreshnessRecord) []Chunk {
	out := make([]Chunk, 0, len(ir.Units))
	for _, u := range ir.Units {
		out = append(out, chunkFromUnit(ir, u, stand, fresh))
	}
	return out
}

// EmitNormtext returns only embeddable normtext units (filtered IR projection).
func EmitNormtext(ir IR, stand domain.StandCitation, fresh domain.FreshnessRecord) []Chunk {
	out := make([]Chunk, 0, len(ir.Units))
	for _, u := range ir.Units {
		if u.Kind != KindNormtext {
			continue
		}
		out = append(out, chunkFromUnit(ir, u, stand, fresh))
	}
	return out
}

// EmitFlat produces minimally marked flat text for diffing.
func EmitFlat(ir IR) string {
	var b strings.Builder
	for _, u := range ir.Units {
		b.WriteString("@@ ")
		b.WriteString(u.ID)
		b.WriteByte('\n')
		b.WriteString(u.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// UnitIDs returns ordered unit ids (for invariant checks).
func UnitIDs(ir IR) []string {
	ids := make([]string, len(ir.Units))
	for i, u := range ir.Units {
		ids[i] = u.ID
	}
	return ids
}

// ChunkJSON helper.
func ChunkJSON(chunks []Chunk) string {
	b, _ := json.MarshalIndent(chunks, "", "  ")
	return string(b)
}
