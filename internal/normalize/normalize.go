// Package normalize provides German-aware string normalization for search.
package normalize

import (
	"strings"
	"unicode"
)

// Key folds case, maps umlauts/`ß`, strips punctuation/spaces for comparison.
func Key(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'ä':
			b.WriteString("ae")
		case 'ö':
			b.WriteString("oe")
		case 'ü':
			b.WriteString("ue")
		case 'ß':
			b.WriteString("ss")
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// ExpandAE forms also match ae->ä style queries by producing alternate keys.
func AlternateKeys(s string) []string {
	k := Key(s)
	out := []string{k}
	// Also accept literal umlaut forms already folded; add reverse expansions for common digraphs in input.
	repl := []struct{ from, to string }{
		{"ae", "a"}, {"oe", "o"}, {"ue", "u"},
	}
	for _, r := range repl {
		if strings.Contains(k, r.from) {
			out = append(out, strings.ReplaceAll(k, r.from, r.to))
		}
	}
	return unique(out)
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
