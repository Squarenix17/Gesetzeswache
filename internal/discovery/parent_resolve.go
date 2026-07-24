package discovery

import (
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// ParentLookup resolves parent law IDs from jurabk or title phrase hints.
type ParentLookup interface {
	ByJurabk(jurabk string) []string
	ByTitlePhrase(phrase string) []string
}

var builtInTitleParents = map[string]string{
	normalize.Key("Elften Buches"):      "SGB XI",
	normalize.Key("Elften Buches Sozialgesetzbuch"): "SGB XI",
	normalize.Key("Elftes Buch"):        "SGB XI",
	normalize.Key("Mindestlohngesetz"):  "milog",
	normalize.Key("Mindestlohngesetzes"): "milog",
}

// ResolveParent returns a canonical parent law ID when uniquely resolvable.
// Precise signals (jurabk + built-in book phrases) win over fuzzy title search so
// common tokens like "Sozialgesetzbuch" do not make SGB XI-class parents ambiguous.
func ResolveParent(e Ermaechtigung, lookup ParentLookup) (lawID string, unique bool) {
	if id, ok := uniqueFrom(preciseParentCandidates(e, lookup)); ok {
		return id, true
	}
	if id, ok := uniqueFrom(titleParentCandidates(e, lookup)); ok {
		return id, true
	}
	return "", false
}

func uniqueFrom(ids []string) (string, bool) {
	ids = uniqueIDs(ids)
	if len(ids) == 1 {
		return ids[0], true
	}
	return "", false
}

func preciseParentCandidates(e Ermaechtigung, lookup ParentLookup) []string {
	var out []string
	if e.Jurabk != "" {
		out = append(out, lookup.ByJurabk(e.Jurabk)...)
	}
	phraseKey := normalize.Key(e.LawTitlePhrase)
	if phraseKey == "" {
		return out
	}
	if mapped := builtInTitleParents[phraseKey]; mapped != "" {
		out = append(out, resolveBuiltInParent(mapped, lookup)...)
	}
	// Phrase must contain the built-in book/name token (never the reverse: short
	// phrases must not match because they are substrings of "Mindestlohngesetz").
	for key, mapped := range builtInTitleParents {
		if phraseKey == key || strings.Contains(phraseKey, key) {
			out = append(out, resolveBuiltInParent(mapped, lookup)...)
		}
	}
	return out
}

func resolveBuiltInParent(mapped string, lookup ParentLookup) []string {
	if strings.EqualFold(mapped, "milog") {
		return []string{normalize.Key(mapped)}
	}
	return lookup.ByJurabk(mapped)
}

func titleParentCandidates(e Ermaechtigung, lookup ParentLookup) []string {
	var out []string
	for _, phrase := range titlePhraseVariants(e.LawTitlePhrase) {
		out = append(out, lookup.ByTitlePhrase(phrase)...)
	}
	return out
}

func titlePhraseVariants(phrase string) []string {
	phrase = strings.TrimSpace(phrase)
	if phrase == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		k := normalize.Key(s)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, s)
	}

	add(phrase)
	if dash := strings.Index(phrase, "–"); dash >= 0 {
		add(phrase[:dash])
	}
	return out
}

func uniqueIDs(ids []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, id := range ids {
		id = normalize.Key(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ScoreConfidence rates discovery confidence as low, medium, or high.
// High requires a unique parent, section hint, and fundstelle confirmation.
// Multi-parent matches cap at medium. editorialAgrees can bump medium to high
// when the other prerequisites are already satisfied.
func ScoreConfidence(parentUnique bool, sectionHint string, fundstelleOK bool, editorialAgrees bool) string {
	if !parentUnique {
		if sectionHint != "" || fundstelleOK {
			return "medium"
		}
		return "low"
	}

	score := "low"
	if sectionHint != "" || fundstelleOK {
		score = "medium"
	}
	if sectionHint != "" && fundstelleOK {
		score = "high"
	}
	if editorialAgrees && sectionHint != "" && fundstelleOK && score == "medium" {
		score = "high"
	}
	return score
}
