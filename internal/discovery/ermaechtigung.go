package discovery

import (
	"regexp"
	"strings"
)

var (
	reSectionRef = regexp.MustCompile(`(?i)§\s*(\d+[a-zA-Z]?)`)
	reAbsatz     = regexp.MustCompile(`(?i)\bAbsatz\s+(\d+[a-zA-Z]?)\b`)
	reSatz       = regexp.MustCompile(`(?i)\bSatz\s+(\d+)\b`)
	reJurabk     = regexp.MustCompile(`(?i)\bSGB\s+([IVXLC]+|\d+)\b`)
)

type sectionRef struct {
	section string
	absatz  string
	satz    string
	start   int
	end     int
}

// ParseErmaechtigung extracts Ermächtigung references from German ordinance preamble text.
func ParseErmaechtigung(text string) []Ermaechtigung {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	parentPhrase, jurabk := extractParentPhrase(text)
	refs := extractSectionRefs(text)
	if len(refs) == 0 {
		return nil
	}

	out := make([]Ermaechtigung, 0, len(refs))
	for _, ref := range refs {
		out = append(out, Ermaechtigung{
			Section:        ref.section,
			Absatz:         ref.absatz,
			Satz:           ref.satz,
			LawTitlePhrase: parentPhrase,
			Jurabk:         jurabk,
			Raw:            strings.TrimSpace(text[ref.start:min(ref.end, len(text))]),
		})
	}
	return out
}

func extractParentPhrase(text string) (phrase, jurabk string) {
	lower := strings.ToLower(text)
	scope := text
	for _, cut := range []string{
		", der zuletzt", ", die zuletzt", ", das zuletzt",
		", der durch", ", die durch", ", das durch",
	} {
		if i := strings.Index(lower, cut); i >= 0 {
			scope = text[:i]
			lower = strings.ToLower(scope)
			break
		}
	}
	phrase = pickLawTitleAfterDes(scope)
	phrase = strings.TrimRight(phrase, "–-:,; ")
	phrase = trimTrailingSubtitle(phrase)
	phrase = trimTrailingVomClause(phrase)
	jurabk = extractJurabk(text)
	if jurabk == "" {
		jurabk = inferJurabkFromPhrase(phrase)
	}
	return phrase, jurabk
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
	switch {
	case strings.Contains(key, "elften buches"):
		return "SGB XI"
	case strings.Contains(key, "elftes buch"):
		return "SGB XI"
	default:
		return ""
	}
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
