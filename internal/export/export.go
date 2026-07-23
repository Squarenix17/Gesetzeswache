// Package export builds a single IR from GII XML and emits RAG-ready formats.
package export

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

const (
	FormatHierarchical = "hierarchical"
	FormatChunked      = "chunked"
	FormatFlat         = "flat"
)

// Unit is one paragraph-level IR node.
type Unit struct {
	ID           string `json:"id"`
	SectionKey   string `json:"section_key"`
	SectionTitle string `json:"section_title"`
	ParagraphNum string `json:"paragraph_num"`
	Text         string `json:"text"`
	Ambiguous    bool   `json:"ambiguous,omitempty"`
}

// IR is the single intermediate representation for one law export.
type IR struct {
	LawID            string
	ContentID        string
	Title            string
	Abbreviation     string
	Units            []Unit
	StructuralAmbiguity bool
	BuiltAt          time.Time
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
		// drop arbitrary oldest-ish: clear half
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
	return fmt.Sprintf("%d/%d/%s/%s", stand.Teil, stand.Year, stand.Number, stand.Page)
}

// BuildIR parses GII norm XML into IR. Text content is preserved as-is from source nodes.
func BuildIR(law domain.Law, contentID string, xmlData []byte) (IR, error) {
	units, amb, title := extractUnits(law.ID, xmlData)
	if title == "" {
		title = law.Title
	}
	ir := IR{
		LawID:               law.ID,
		ContentID:           contentID,
		Title:               title,
		Abbreviation:        law.Abbreviation,
		Units:               units,
		StructuralAmbiguity: amb,
		BuiltAt:             time.Now().UTC(),
	}
	return ir, nil
}

func extractUnits(lawID string, data []byte) ([]Unit, bool, string) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var units []Unit
	ambiguous := false
	title := ""
	var sectionKey, sectionTitle, paraNum string
	var textBuf strings.Builder
	depth := 0
	flush := func() {
		txt := strings.TrimSpace(textBuf.String())
		textBuf.Reset()
		if txt == "" {
			return
		}
		path := sectionKey + "/" + paraNum
		if sectionKey == "" && paraNum == "" {
			ambiguous = true
			path = fmt.Sprintf("orphan/%d", len(units)+1)
		}
		id := unitID(lawID, path, txt)
		units = append(units, Unit{
			ID:           id,
			SectionKey:   sectionKey,
			SectionTitle: sectionTitle,
			ParagraphNum: paraNum,
			Text:         txt,
			Ambiguous:    sectionKey == "" && paraNum == "",
		})
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "titel", "title":
				var s string
				_ = dec.DecodeElement(&s, &t)
				depth--
				if title == "" {
					title = strings.TrimSpace(stripTags(s))
				}
			case "enbez", "section", "gliederungseinheit":
				flush()
				sectionKey = attrOrLocal(t, "id", t.Name.Local+fmt.Sprintf("-%d", depth))
				sectionTitle = ""
			case "metangabe":
				// ignore
			case "p", "absatz", "text", "Content":
				paraNum = attrOrLocal(t, "nr", fmt.Sprintf("%d", len(units)+1))
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if name == "p" || name == "absatz" || name == "text" {
				flush()
				paraNum = ""
			}
			depth--
		case xml.CharData:
			s := string(t)
			if strings.TrimSpace(s) != "" {
				textBuf.WriteString(s)
			}
		}
	}
	flush()
	if len(units) == 0 {
		// fallback: treat whole document text as one ambiguous unit (no paraphrase — raw stripped tags)
		plain := stripTags(string(data))
		plain = strings.TrimSpace(plain)
		if plain != "" {
			ambiguous = true
			units = append(units, Unit{
				ID:        unitID(lawID, "full", plain),
				Text:      plain,
				Ambiguous: true,
			})
		}
	}
	return units, ambiguous, title
}

func attrOrLocal(t xml.StartElement, key, def string) string {
	for _, a := range t.Attr {
		if strings.EqualFold(a.Name.Local, key) && a.Value != "" {
			return a.Value
		}
	}
	return def
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
	h := sha1.Sum([]byte(lawID + "|" + path + "|" + text))
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
		if u.SectionKey != curSec {
			curSec = u.SectionKey
			if u.SectionTitle != "" || u.SectionKey != "" {
				b.WriteString("\n## ")
				if u.SectionTitle != "" {
					b.WriteString(u.SectionTitle)
				} else {
					b.WriteString(u.SectionKey)
				}
				b.WriteByte('\n')
			}
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
	UnitID       string `json:"unit_id"`
	LawID        string `json:"law_id"`
	Abbreviation string `json:"abbreviation"`
	ParagraphNum string `json:"paragraph_num"`
	SectionTitle string `json:"section_title"`
	StandRaw     string `json:"stand_raw"`
	Freshness    string `json:"freshness_state"`
	Text         string `json:"text"`
}

// EmitChunked builds chunk payloads with identical unit boundaries.
func EmitChunked(ir IR, stand domain.StandCitation, fresh domain.FreshnessRecord) []Chunk {
	out := make([]Chunk, 0, len(ir.Units))
	for _, u := range ir.Units {
		out = append(out, Chunk{
			UnitID:       u.ID,
			LawID:        ir.LawID,
			Abbreviation: ir.Abbreviation,
			ParagraphNum: u.ParagraphNum,
			SectionTitle: u.SectionTitle,
			StandRaw:     stand.Raw,
			Freshness:    string(fresh.State),
			Text:         u.Text,
		})
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
