package export

import (
	"strconv"
	"strings"
)

// IndexChunk is an ingest-ready vector payload (flat; no freshness per chunk).
type IndexChunk struct {
	ChunkID           string `json:"chunk_id"`
	Text              string `json:"text"`
	LawID             string `json:"law_id"`
	LawName           string `json:"law_name"`
	InstrumentKind    string `json:"instrument_kind"`
	SectionRef        string `json:"section_ref,omitempty"`
	SectionName       string `json:"section_name,omitempty"`
	ParentLawID       string `json:"parent_law_id,omitempty"`
	ParentSectionHint string `json:"parent_section_hint,omitempty"`
}

// ParseSectionRefs splits a comma-separated section filter (e.g. "§ 1,§ 2").
// Empty input returns nil (no filter).
func ParseSectionRefs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sectionRefKey(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), "")
}

// MatchSectionFilter reports whether c matches any requested section ref.
// Empty refs means keep all. A chunk matches if its section_ref matches, or
// (when set) its parent_section_hint matches.
func MatchSectionFilter(c IndexChunk, refs []string) bool {
	if len(refs) == 0 {
		return true
	}
	keys := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		k := sectionRefKey(r)
		if k == "" {
			continue
		}
		keys[k] = struct{}{}
	}
	if len(keys) == 0 {
		return true
	}
	if _, ok := keys[sectionRefKey(c.SectionRef)]; ok {
		return true
	}
	if c.ParentSectionHint != "" {
		if _, ok := keys[sectionRefKey(c.ParentSectionHint)]; ok {
			return true
		}
	}
	return false
}

// FilterIndexChunks returns chunks matching refs (or all when refs empty).
func FilterIndexChunks(chunks []IndexChunk, refs []string) []IndexChunk {
	if len(refs) == 0 {
		out := make([]IndexChunk, len(chunks))
		copy(out, chunks)
		return out
	}
	out := make([]IndexChunk, 0, len(chunks))
	for _, c := range chunks {
		if MatchSectionFilter(c, refs) {
			out = append(out, c)
		}
	}
	return out
}

// DedupeChunkIDs returns a copy of chunks with unique ChunkID values. The first
// occurrence of each ID is unchanged when free; later collisions get a stable
// "-2", "-3", … suffix that does not collide with any already-assigned ID.
func DedupeChunkIDs(chunks []IndexChunk) []IndexChunk {
	if len(chunks) == 0 {
		return nil
	}
	out := make([]IndexChunk, len(chunks))
	used := make(map[string]struct{}, len(chunks))
	for i, c := range chunks {
		out[i] = c
		id := c.ChunkID
		if _, taken := used[id]; !taken {
			used[id] = struct{}{}
			continue
		}
		base := c.ChunkID
		for n := 2; ; n++ {
			cand := base + "-" + strconv.Itoa(n)
			if _, taken := used[cand]; taken {
				continue
			}
			out[i].ChunkID = cand
			used[cand] = struct{}{}
			break
		}
	}
	return out
}

// isIndexFormularyRef reports GII formulary/boilerplate section labels that are
// not statute body for vector ingest (any law). Kept in chunked/normtext for
// other consumers; dropped only from the dedicated index projection.
func isIndexFormularyRef(sectionRef string) bool {
	ref := strings.TrimSpace(sectionRef)
	switch {
	case strings.EqualFold(ref, "Eingangsformel"),
		strings.EqualFold(ref, "Schlussformel"),
		strings.EqualFold(ref, "Schlußformel"),
		strings.EqualFold(ref, "Unterschrift"),
		strings.EqualFold(ref, "Ausfertigungsvermerk"):
		return true
	default:
		return false
	}
}

// IsIndexableExportChunk reports whether a chunked/normtext Chunk belongs in the
// flat index ingest payload (excludes formulary chrome such as Eingangsformel).
func IsIndexableExportChunk(c Chunk) bool {
	return !isIndexFormularyRef(c.SectionRef)
}

// IndexChunkFromExport maps an export Chunk to an IndexChunk.
// parentLawID / parentSectionHint must be empty for parent Gesetz chunks.
func IndexChunkFromExport(c Chunk, lawName, instrumentKind, parentLawID, parentSectionHint string) IndexChunk {
	return IndexChunk{
		ChunkID:           c.UnitID,
		Text:              c.Text,
		LawID:             c.LawID,
		LawName:           lawName,
		InstrumentKind:    instrumentKind,
		SectionRef:        c.SectionRef,
		SectionName:       c.SectionTitle,
		ParentLawID:       parentLawID,
		ParentSectionHint: parentSectionHint,
	}
}
