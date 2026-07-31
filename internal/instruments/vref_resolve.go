package instruments

import (
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// BGBlIndexLookup resolves BGBl citations to GII slugs.
type BGBlIndexLookup interface {
	LookupBGBlIndex(teil, year int, number string) (slug string, lawID string, ok bool)
}

// StoreBGBlIndex adapts store.LookupBGBlIndex to BGBlIndexLookup.
type StoreBGBlIndex struct {
	Lookup func(teil, year int, number string) (slug string, lawID string, ok bool)
}

func (a StoreBGBlIndex) LookupBGBlIndex(teil, year int, number string) (string, string, bool) {
	if a.Lookup == nil {
		return "", "", false
	}
	return a.Lookup(teil, year, number)
}

// ResolveOperativeVRefs matches refs to current linked children.
// annotated = AnnotateChain output (includes past/current/future).
// childStands map keyed by normalize.Key(lawID or slug).
// index may be nil.
// Does NOT set ChildConfirmed/Resolved — caller does Proof C Evaluate.
// Sets Historical=true when identity matches only a past child's notes/stand.
// MatchMethod: notes_identity | bgbl_index | ""
func ResolveOperativeVRefs(
	refs []domain.InstrumentRef,
	annotated []domain.LinkedInstrument,
	childStands map[string]domain.StandCitation,
	index BGBlIndexLookup,
	parentStand domain.StandCitation,
) []domain.VRefResolution {
	hasLinked := len(annotated) > 0
	var current, past []domain.LinkedInstrument
	for _, li := range annotated {
		switch li.Status {
		case StatusCurrent:
			current = append(current, li)
		case StatusPast:
			past = append(past, li)
		}
	}

	var out []domain.VRefResolution
	for _, ref := range refs {
		if ref.Year == 0 || (ref.Number == "" && ref.Teil == 0) {
			continue
		}
		if standReflectsRef(parentStand, ref) {
			continue
		}
		if !isOperativeRefForResolve(ref, hasLinked) {
			continue
		}

		res := domain.VRefResolution{Ref: ref}
		currentMatches := matchChildrenByIdentity(ref, current, childStands)
		ambiguousCurrent := false
		switch len(currentMatches) {
		case 1:
			res.MatchedGIISlug = currentMatches[0].GIISlug
			res.MatchMethod = "notes_identity"
		default:
			if len(currentMatches) > 1 {
				filtered := filterChildrenBySectionHint(currentMatches, ref.SectionHint)
				if len(filtered) == 1 {
					res.MatchedGIISlug = filtered[0].GIISlug
					res.MatchMethod = "notes_identity"
				} else {
					// Ambiguous current match → fail closed; do not fall through to past/historical.
					ambiguousCurrent = true
				}
			}
		}

		if res.MatchedGIISlug == "" && !ambiguousCurrent && index != nil {
			slug, lawID, ok := index.LookupBGBlIndex(ref.Teil, ref.Year, ref.Number)
			if ok {
				if child := findChildBySlugOrLawID(current, slug, lawID); child != nil {
					res.MatchedGIISlug = child.GIISlug
					res.MatchMethod = "bgbl_index"
				}
			}
		}

		// Past-only identity is historical only for non-V refs (empty-kind / bare BEK editorial).
		// Operative Kind V never bypasses Proof C via a past-chain match.
		if res.MatchedGIISlug == "" && !ambiguousCurrent && !isKindV(ref) {
			pastMatches := matchChildrenByIdentity(ref, past, childStands)
			if len(pastMatches) > 0 {
				res.Historical = true
				res.MatchMethod = "notes_identity"
			}
		}

		out = append(out, res)
	}
	return out
}

func isOperativeRefForResolve(ref domain.InstrumentRef, hasLinked bool) bool {
	kind := strings.ToUpper(strings.TrimSpace(ref.Kind))
	switch kind {
	case "G":
		return false
	case "V":
		return true
	case "BEK":
		if strings.TrimSpace(ref.SectionHint) != "" {
			return true
		}
		return hasLinked
	default:
		return hasLinked && kind == ""
	}
}

func isKindV(ref domain.InstrumentRef) bool {
	return strings.ToUpper(strings.TrimSpace(ref.Kind)) == "V"
}

func standReflectsRef(stand domain.StandCitation, ref domain.InstrumentRef) bool {
	return stand.ParseOK &&
		stand.Year == ref.Year &&
		stand.Teil == ref.Teil &&
		stand.Number == ref.Number
}

func matchChildrenByIdentity(
	ref domain.InstrumentRef,
	children []domain.LinkedInstrument,
	childStands map[string]domain.StandCitation,
) []domain.LinkedInstrument {
	var matches []domain.LinkedInstrument
	for _, child := range children {
		if childMatchesRef(ref, child, childStands) {
			matches = append(matches, child)
		}
	}
	return matches
}

func childMatchesRef(
	ref domain.InstrumentRef,
	child domain.LinkedInstrument,
	childStands map[string]domain.StandCitation,
) bool {
	for _, parsed := range citation.ParseLinkedInstruments(child.Notes) {
		if refIdentityMatches(ref, parsed) {
			return true
		}
	}
	for _, key := range childLookupKeys(child) {
		stand, ok := childStands[key]
		if !ok {
			continue
		}
		if stand.Year > 0 && stand.Number != "" {
			if refIdentityMatches(ref, domain.InstrumentRef{
				Teil: stand.Teil, Year: stand.Year, Number: stand.Number,
			}) {
				return true
			}
		}
		for _, parsed := range citation.ParseLinkedInstruments(stand.Raw) {
			if refIdentityMatches(ref, parsed) {
				return true
			}
		}
	}
	return false
}

func refIdentityMatches(ref, other domain.InstrumentRef) bool {
	if ref.Year != other.Year || ref.Number != other.Number {
		return false
	}
	if ref.Teil != 0 && other.Teil != 0 && ref.Teil != other.Teil {
		return false
	}
	return true
}

func childLookupKeys(child domain.LinkedInstrument) []string {
	var keys []string
	if id := normalize.Key(child.LawID); id != "" {
		keys = append(keys, id)
	}
	if slug := normalize.Key(child.GIISlug); slug != "" {
		keys = append(keys, slug)
	}
	return keys
}

func filterChildrenBySectionHint(children []domain.LinkedInstrument, hint string) []domain.LinkedInstrument {
	norm := normalizeSectionHint(hint)
	if norm == "*" {
		return nil
	}
	var out []domain.LinkedInstrument
	for _, child := range children {
		if normalizeSectionHint(child.SectionHint) == norm {
			out = append(out, child)
		}
	}
	return out
}

func findChildBySlugOrLawID(children []domain.LinkedInstrument, slug, lawID string) *domain.LinkedInstrument {
	slugKey := normalize.Key(slug)
	lawKey := normalize.Key(lawID)
	for i := range children {
		child := &children[i]
		if slugKey != "" && normalize.Key(child.GIISlug) == slugKey {
			return child
		}
		if lawKey != "" && normalize.Key(child.LawID) == lawKey {
			return child
		}
	}
	return nil
}

// BuildChildStands loads stand citations for annotated linked children.
func BuildChildStands(annotated []domain.LinkedInstrument, st MetaStandIssueStore) map[string]domain.StandCitation {
	if st == nil || len(annotated) == 0 {
		return map[string]domain.StandCitation{}
	}
	out := make(map[string]domain.StandCitation)
	for _, li := range annotated {
		for _, key := range childLookupKeys(li) {
			if _, ok := out[key]; ok {
				continue
			}
			if stand, ok, _ := st.GetStand(key); ok {
				out[key] = stand
			}
		}
	}
	return out
}
