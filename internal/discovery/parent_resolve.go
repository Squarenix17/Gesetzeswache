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

// sgbBookPhraseEntry maps German ordinal SGB book phrases to jurabk.
type sgbBookPhraseEntry struct {
	jurabk     string
	tokens     []string // lowercase substrings matched in title phrases (longest first)
	fullTitles []string // canonical "… Buches Sozialgesetzbuch" forms
}

// Longer / more distinctive tokens first so "zwölften" is not confused with "elften"
// after normalize.Key umlaut folding (zwoelften contains elften as substring).
var sgbBookPhraseEntries = []sgbBookPhraseEntry{
	{jurabk: "SGB XII", tokens: []string{"zwölften buches", "zwoelften buches", "zwölftes buch", "zwoelftes buch"},
		fullTitles: []string{"Zwölften Buches Sozialgesetzbuch", "Zwölftes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB XI", tokens: []string{"elften buches", "elftes buch"},
		fullTitles: []string{"Elften Buches Sozialgesetzbuch", "Elftes Buch Sozialgesetzbuch", "Elften Buches", "Elftes Buch"}},
	{jurabk: "SGB X", tokens: []string{"zehnten buches", "zehntes buch"},
		fullTitles: []string{"Zehnten Buches Sozialgesetzbuch", "Zehntes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB IX", tokens: []string{"neunten buches", "neuntes buch"},
		fullTitles: []string{"Neunten Buches Sozialgesetzbuch", "Neuntes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB VIII", tokens: []string{"achten buches", "achtes buch"},
		fullTitles: []string{"Achten Buches Sozialgesetzbuch", "Achtes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB VII", tokens: []string{"siebenten buches", "siebten buches", "siebentes buch", "siebtes buch"},
		fullTitles: []string{"Siebten Buches Sozialgesetzbuch", "Siebtes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB VI", tokens: []string{"sechsten buches", "sechstes buch"},
		fullTitles: []string{"Sechsten Buches Sozialgesetzbuch", "Sechstes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB V", tokens: []string{"fünften buches", "fuenften buches", "fünftes buch", "fuenftes buch"},
		fullTitles: []string{"Fünften Buches Sozialgesetzbuch", "Fünftes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB IV", tokens: []string{"vierten buches", "viertes buch"},
		fullTitles: []string{"Vierten Buches Sozialgesetzbuch", "Viertes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB III", tokens: []string{"dritten buches", "drittes buch"},
		fullTitles: []string{"Dritten Buches Sozialgesetzbuch", "Drittes Buch Sozialgesetzbuch"}},
	{jurabk: "SGB II", tokens: []string{"zweiten buches", "zweites buch"},
		fullTitles: []string{"Zweiten Buches Sozialgesetzbuch", "Zweites Buch Sozialgesetzbuch"}},
	{jurabk: "SGB I", tokens: []string{"ersten buches", "erstes buch"},
		fullTitles: []string{"Ersten Buches Sozialgesetzbuch", "Erstes Buch Sozialgesetzbuch"}},
}

type builtinParentName struct {
	names  []string // nominative + genitive display forms
	mapped string   // canonical law ID or jurabk
}

var builtinParentNames = []builtinParentName{
	{names: []string{"Mindestlohngesetz", "Mindestlohngesetzes"}, mapped: "milog"},
	{names: []string{"Lastenausgleichsgesetz", "Lastenausgleichsgesetzes"}, mapped: "lag"},
	{names: []string{"Arbeitnehmer-Entsendegesetz", "Arbeitnehmer-Entsendegesetzes"}, mapped: "aentg2009"},
}

var (
	builtInTitleParents    = map[string]string{}
	builtinDirectParentIDs = map[string]struct{}{}
)

func init() {
	for _, e := range builtinParentNames {
		builtinDirectParentIDs[normalize.Key(e.mapped)] = struct{}{}
		for _, name := range e.names {
			builtInTitleParents[normalize.Key(name)] = e.mapped
		}
	}
	for _, e := range sgbBookPhraseEntries {
		for _, token := range e.tokens {
			builtInTitleParents[normalize.Key(token)] = e.jurabk
		}
		for _, title := range e.fullTitles {
			builtInTitleParents[normalize.Key(title)] = e.jurabk
		}
	}
}

// ResolveParentFromChildTitle derives a parent law from a Verordnung title when the
// preamble only names § refs (abbreviated "G v." fussnoten). Titles ending in
// "…verordnung" yield "…gesetz" / "…gesetzes" candidates; non-unique or unmatched
// phrases return empty.
func ResolveParentFromChildTitle(childTitle string, lookup ParentLookup) (lawID string, unique bool) {
	for _, phrase := range parentPhrasesFromVerordnungTitle(childTitle) {
		if id, ok := uniqueFrom(lookup.ByTitlePhrase(phrase)); ok {
			return id, true
		}
		if mapped := builtInTitleParents[normalize.Key(phrase)]; mapped != "" {
			if id, ok := uniqueFrom(resolveBuiltInParent(mapped, lookup)); ok {
				return id, true
			}
		}
	}
	return "", false
}

func parentPhrasesFromVerordnungTitle(title string) []string {
	title = strings.TrimSpace(title)
	if title == "" {
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

	lower := strings.ToLower(title)
	if idx := strings.Index(lower, "nach dem "); idx >= 0 {
		name := strings.TrimSpace(title[idx+len("nach dem "):])
		if cut := strings.IndexAny(name, ",;"); cut >= 0 {
			name = strings.TrimSpace(name[:cut])
		}
		add(name)
		if !strings.HasSuffix(strings.ToLower(name), "gesetz") {
			add(name + "gesetz")
		}
		if !strings.HasSuffix(strings.ToLower(name), "gesetzes") {
			add(name + "gesetzes")
		}
	}

	const suffix = "verordnung"
	if strings.HasSuffix(lower, suffix) {
		base := strings.TrimSpace(title[:len(title)-len(suffix)])
		if base != "" {
			add(base + "gesetz")
			add(base + "gesetzes")
		}
	}
	return out
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
	// Exact builtin key containment for MiLoG-style names (not SGB ordinals):
	// only match when the phrase contains the full normalized name token.
	for _, entry := range builtinParentNames {
		for _, name := range entry.names {
			key := normalize.Key(name)
			if phraseKey == key || strings.Contains(phraseKey, key) {
				out = append(out, resolveBuiltInParent(entry.mapped, lookup)...)
				break
			}
		}
	}
	// SGB ordinals: match on raw lowercase so umlaut folding cannot make
	// "zwölften" collide with "elften" (zwoelften contains elften).
	lower := strings.ToLower(e.LawTitlePhrase)
	for _, entry := range sgbBookPhraseEntries {
		for _, token := range entry.tokens {
			if strings.Contains(lower, token) {
				out = append(out, resolveBuiltInParent(entry.jurabk, lookup)...)
				break
			}
		}
	}
	return out
}

func resolveBuiltInParent(mapped string, lookup ParentLookup) []string {
	if _, direct := builtinDirectParentIDs[normalize.Key(mapped)]; direct {
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
// High requires a unique parent (per Ermächtigung segment), section hint, and
// fundstelle confirmation. Ambiguous parent candidates within a segment cap at
// medium. editorialAgrees can bump medium to high when the other prerequisites
// are already satisfied.
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
