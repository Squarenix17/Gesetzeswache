package export

import (
	"fmt"
	"strings"
)

// OperativeComposeMember is one linked Verordnung to optionally embed for display.
type OperativeComposeMember struct {
	LawID         string
	Abbreviation  string
	SectionHint   string
	Status        string
	EffectiveFrom string
	Hierarchical  string // full VO hierarchical export (headings stripped for body)
}

// PlacementAfterParentSection and PlacementDocumentEnd describe where a VO relates to the parent.
const (
	PlacementAfterParentSection = "after_parent_section"
	PlacementDocumentEnd        = "document_end"
)

// StripVOHierarchicalBody removes VO title (# …) and section (## …) / Abs (### Abs.) headings
// so the body cannot be confused with parent Gesetz section numbers.
func StripVOHierarchicalBody(hierarchical string) string {
	var b strings.Builder
	for _, line := range strings.Split(hierarchical, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// SectionHintMatchesHeader reports whether a hierarchical ## header line covers sectionHint (e.g. "§ 1").
func SectionHintMatchesHeader(headerLine, sectionHint string) bool {
	hint := normalizeSectionToken(sectionHint)
	if hint == "" {
		return false
	}
	h := strings.TrimSpace(headerLine)
	h = strings.TrimPrefix(h, "##")
	h = normalizeSectionToken(h)
	if h == hint || strings.HasPrefix(h, hint+" ") {
		return true
	}
	return false
}

func normalizeSectionToken(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

// PlacementForHint returns after_parent_section if parentHierarchical contains a matching ## section.
func PlacementForHint(parentHierarchical, sectionHint string) string {
	if strings.TrimSpace(sectionHint) == "" {
		return PlacementDocumentEnd
	}
	for _, line := range strings.Split(parentHierarchical, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") && SectionHintMatchesHeader(line, sectionHint) {
			return PlacementAfterParentSection
		}
	}
	return PlacementDocumentEnd
}

// ComposeOperativeHierarchical inserts display-only Verordnung callouts after matching parent
// sections (or at document end). Parent body text is not rewritten. Not for vector ingest.
func ComposeOperativeHierarchical(parentHierarchical, parentAbbr string, members []OperativeComposeMember) string {
	if len(members) == 0 {
		return parentHierarchical
	}

	type item struct {
		hint  string
		block string
	}
	var sectionItems []item
	var endBlocks []string
	for _, m := range members {
		block := formatVerordnungCallout(parentAbbr, m)
		if PlacementForHint(parentHierarchical, m.SectionHint) == PlacementAfterParentSection {
			sectionItems = append(sectionItems, item{hint: m.SectionHint, block: block})
		} else {
			endBlocks = append(endBlocks, block)
		}
	}

	var out strings.Builder
	lines := strings.Split(parentHierarchical, "\n")
	for i := 0; i < len(lines); {
		line := lines[i]
		out.WriteString(line)
		out.WriteByte('\n')
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "## ") {
			i++
			continue
		}
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			out.WriteString(lines[i])
			out.WriteByte('\n')
			i++
		}
		remain := sectionItems[:0]
		for _, it := range sectionItems {
			if SectionHintMatchesHeader(line, it.hint) {
				out.WriteByte('\n')
				out.WriteString(it.block)
				out.WriteString("\n\n")
			} else {
				remain = append(remain, it)
			}
		}
		sectionItems = remain
	}

	for _, it := range sectionItems {
		endBlocks = append(endBlocks, it.block)
	}
	for _, block := range endBlocks {
		out.WriteByte('\n')
		out.WriteString(block)
		out.WriteString("\n\n")
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

func formatVerordnungCallout(parentAbbr string, m OperativeComposeMember) string {
	abbr := strings.TrimSpace(m.Abbreviation)
	if abbr == "" {
		abbr = m.LawID
	}
	parent := strings.TrimSpace(parentAbbr)
	if parent == "" {
		parent = "Gesetz"
	}
	body := StripVOHierarchicalBody(m.Hierarchical)
	var meta strings.Builder
	meta.WriteString(fmt.Sprintf("**Verordnung (nicht Teil des %s)** — %s (`%s`)", parent, abbr, m.LawID))
	if h := strings.TrimSpace(m.SectionHint); h != "" {
		meta.WriteString(" · Parent ")
		meta.WriteString(h)
	}
	if s := strings.TrimSpace(m.Status); s != "" {
		meta.WriteString(" · ")
		meta.WriteString(s)
	}
	if e := strings.TrimSpace(m.EffectiveFrom); e != "" {
		meta.WriteString(" · ab ")
		meta.WriteString(e)
	}
	var b strings.Builder
	b.WriteString("> ")
	b.WriteString(meta.String())
	b.WriteString("\n\n")
	if body != "" {
		for _, line := range strings.Split(body, "\n") {
			b.WriteString("> ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
