package discovery

import (
	"regexp"
	"strings"
)

var (
	reSectionRef   = regexp.MustCompile(`(?i)§\s*(\d+[a-zA-Z]?)`)
	reAbsatz       = regexp.MustCompile(`(?i)\bAbsatz\s+(\d+[a-zA-Z]?)\b`)
	reSatz         = regexp.MustCompile(`(?i)\bSatz\s+(\d+)\b`)
	reJurabk       = regexp.MustCompile(`(?i)\bSGB\s+([IVXLC]+|\d+)\b`)
	reAufGrund     = regexp.MustCompile(`(?i)\b(?:auf\s+grund|aufgrund)\b`)
	reDashDesPara  = regexp.MustCompile(`(?i)[–\-]\s*des\s+§`)
	reDesPara      = regexp.MustCompile(`(?i)des\s+§`)
	reAbbrDesPara  = regexp.MustCompile(`(?i)\bd\.\s*§`)
	reVerordnet    = regexp.MustCompile(`(?i)\bverordnet\s+(?:die|das)\b`)
)

type sectionRef struct {
	section string
	absatz  string
	satz    string
	start   int
	end     int
}

// ParseErmaechtigung extracts Ermächtigung references from German ordinance preamble text.
// Multi-parent preambles (dash lists / repeated Auf Grund) are split into segments so each
// parent book keeps its own § refs and title phrase.
func ParseErmaechtigung(text string) []Ermaechtigung {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var out []Ermaechtigung
	for _, seg := range splitErmaechtigungSegments(text) {
		scope := authorizationScope(seg)
		parentPhrase, jurabk := extractParentPhrase(scope)
		if jurabk == "" {
			// Fall back to full segment for explicit "SGB …" tokens outside the cut scope.
			if j := extractJurabk(seg); j != "" {
				jurabk = j
			} else {
				jurabk = inferJurabkFromPhrase(parentPhrase)
			}
		}
		refs := extractSectionRefs(scope)
		if len(refs) == 0 {
			continue
		}
		for _, ref := range refs {
			out = append(out, Ermaechtigung{
				Section:        ref.section,
				Absatz:         ref.absatz,
				Satz:           ref.satz,
				LawTitlePhrase: parentPhrase,
				Jurabk:         jurabk,
				Raw:            strings.TrimSpace(scope[ref.start:min(ref.end, len(scope))]),
			})
		}
	}
	return out
}

// splitErmaechtigungSegments splits a preamble into per-parent authorization clauses.
// Single-parent preambles (PBAV, MiLoV) yield one segment (the whole text).
func splitErmaechtigungSegments(text string) []string {
	starts := segmentStartIndexes(text)
	if len(starts) <= 1 {
		return []string{text}
	}

	segs := make([]string, 0, len(starts))
	for i, start := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		chunk := text[start:end]
		if m := reVerordnet.FindStringIndex(chunk); m != nil {
			chunk = chunk[:m[0]]
		}
		chunk = strings.TrimSpace(strings.Trim(chunk, "–-,; "))
		if chunk == "" {
			continue
		}
		segs = append(segs, chunk)
	}
	if len(segs) == 0 {
		return []string{text}
	}
	return segs
}

func segmentStartIndexes(text string) []int {
	seen := map[int]struct{}{}
	var starts []int
	add := func(pos int) {
		if pos < 0 || pos >= len(text) {
			return
		}
		if _, ok := seen[pos]; ok {
			return
		}
		seen[pos] = struct{}{}
		starts = append(starts, pos)
	}

	// "Auf Grund … des §" / "aufgrund … des §" (dash optional between marker and des §).
	for _, m := range reAufGrund.FindAllStringIndex(text, -1) {
		rest := text[m[0]:]
		loc := reDesPara.FindStringIndex(rest)
		if loc == nil {
			loc = reAbbrDesPara.FindStringIndex(rest)
		}
		if loc != nil && loc[0] < 80 {
			add(m[0] + loc[0])
		}
	}
	// Dash-list items: "– des § …"
	for _, m := range reDashDesPara.FindAllStringIndex(text, -1) {
		rest := text[m[0]:]
		if loc := reDesPara.FindStringIndex(rest); loc != nil {
			add(m[0] + loc[0])
		}
	}

	if len(starts) == 0 {
		return nil
	}
	// Sort ascending (insertion may interleave Auf-Grund and dash matches).
	for i := 0; i < len(starts); i++ {
		for j := i + 1; j < len(starts); j++ {
			if starts[j] < starts[i] {
				starts[i], starts[j] = starts[j], starts[i]
			}
		}
	}
	return starts
}

func extractParentPhrase(text string) (phrase, jurabk string) {
	scope := authorizationScope(text)
	phrase = pickLawTitleAfterDes(scope)
	phrase = strings.TrimRight(phrase, "–-:,; ")
	phrase = trimTrailingSubtitle(phrase)
	phrase = trimTrailingVomClause(phrase)
	jurabk = extractJurabk(scope)
	if jurabk == "" {
		jurabk = extractJurabk(text)
	}
	if jurabk == "" {
		jurabk = inferJurabkFromPhrase(phrase)
	}
	return phrase, jurabk
}

// authorizationScope truncates amendment tails so § refs and parent phrases
// come only from the Ermächtigung clause itself.
func authorizationScope(text string) string {
	lower := strings.ToLower(text)
	for _, cut := range []string{
		", von denen", ", von dem", ", von der",
		" von denen", " von dem", " von der",
		", dessen", ", deren",
		", der zuletzt", ", die zuletzt", ", das zuletzt",
		", der durch", ", die durch", ", das durch",
	} {
		if i := strings.Index(lower, cut); i >= 0 {
			return text[:i]
		}
	}
	return text
}

// pickLawTitleAfterDes chooses the best "des <LawName>" clause: prefer names that
// look like statutes (…gesetz, …buch, …ordnung) over generic "des Gesetzes vom …".
func pickLawTitleAfterDes(scope string) string {
	lower := strings.ToLower(scope)
	type hit struct {
		phrase string
		score  int
	}
	var best hit
	for i := 0; i < len(lower); {
		idx := strings.Index(lower[i:], " des ")
		if idx < 0 {
			break
		}
		start := i + idx + len(" des ")
		raw := strings.TrimSpace(scope[start:])
		raw = strings.TrimRight(raw, "–-:,; ")
		raw = trimTrailingSubtitle(raw)
		raw = trimTrailingVomClause(raw)
		score := scoreLawTitlePhrase(raw)
		if score > best.score {
			best = hit{phrase: raw, score: score}
		}
		i = start
	}
	return best.phrase
}

func scoreLawTitlePhrase(phrase string) int {
	key := strings.ToLower(strings.TrimSpace(phrase))
	if key == "" {
		return 0
	}
	// Reject §-led chunks that swallowed the whole "des § … des <Buch>" span.
	if strings.HasPrefix(key, "§") || strings.HasPrefix(key, "paragraph") {
		return 0
	}
	// Reject bare "Gesetz(es)" without a distinctive name.
	if key == "gesetz" || key == "gesetzes" || strings.HasPrefix(key, "gesetzes vom") || strings.HasPrefix(key, "gesetz vom") {
		return 1
	}
	score := 2
	switch {
	case strings.Contains(key, "sozialgesetzbuch"), strings.Contains(key, "buches"), strings.Contains(key, "buch "):
		score = 10
	case strings.HasSuffix(key, "gesetzes"), strings.HasSuffix(key, "gesetz"):
		score = 8
	case strings.Contains(key, "gesetz"):
		score = 6
	case strings.HasSuffix(key, "ordnung"), strings.HasSuffix(key, "verordnung"):
		score = 5
	}
	if len(key) > 20 {
		score++
	}
	// Prefer compact book titles over long spans that still contain "buches".
	if score >= 10 && len(key) < 60 {
		score += 2
	}
	return score
}

func trimTrailingVomClause(phrase string) string {
	lower := strings.ToLower(phrase)
	if i := strings.Index(lower, " vom "); i >= 0 {
		return strings.TrimSpace(phrase[:i])
	}
	return phrase
}

func trimTrailingSubtitle(phrase string) string {
	if dash := strings.Index(phrase, "–"); dash >= 0 {
		return strings.TrimSpace(phrase[:dash])
	}
	if dash := strings.Index(phrase, "-"); dash >= 0 {
		rest := strings.TrimSpace(phrase[dash+1:])
		if strings.Contains(strings.ToLower(rest), "sozial") ||
			strings.Contains(strings.ToLower(rest), "versicherung") {
			return strings.TrimSpace(phrase[:dash])
		}
	}
	return phrase
}

func extractJurabk(text string) string {
	if m := reJurabk.FindStringSubmatch(text); len(m) == 2 {
		roman := strings.ToUpper(strings.TrimSpace(m[1]))
		return "SGB " + roman
	}
	return ""
}

func inferJurabkFromPhrase(phrase string) string {
	key := strings.ToLower(phrase)
	for _, e := range sgbBookPhraseEntries {
		for _, token := range e.tokens {
			if strings.Contains(key, token) {
				return e.jurabk
			}
		}
	}
	return ""
}

func extractSectionRefs(text string) []sectionRef {
	matches := reSectionRef.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}

	refs := make([]sectionRef, 0, len(matches))
	for i, m := range matches {
		section := text[m[2]:m[3]]
		start := m[0]
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}

		chunk := text[start:end]
		absatz := firstMatch(reAbsatz, chunk)
		satz := firstMatch(reSatz, chunk)

		refs = append(refs, sectionRef{
			section: section,
			absatz:  absatz,
			satz:    satz,
			start:   start,
			end:     end,
		})
	}
	return refs
}

func firstMatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
