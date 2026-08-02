package normalize

import (
	"regexp"
	"strconv"
	"strings"
)

var reSGBBookQuery = regexp.MustCompile(`(?i)^\s*sgb\s*[_\s-]*([ivxlc\d]+)\s*$`)

// ParseSGBBookQuery reports whether s is a bare SGB book reference (e.g. "SGB V", "sgb_3").
func ParseSGBBookQuery(s string) (bookNum int, ok bool) {
	m := reSGBBookQuery.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) != 2 {
		return 0, false
	}
	rest := strings.TrimSpace(m[1])
	if rest == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(rest); err == nil {
		return n, n > 0
	}
	if n, ok := RomanToInt(rest); ok {
		return n, true
	}
	return 0, false
}

// RomanToInt parses a roman numeral in [I..XX].
func RomanToInt(s string) (int, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" || strings.IndexFunc(s, func(r rune) bool {
		return !strings.ContainsRune("IVXLCDM", r)
	}) >= 0 {
		return 0, false
	}
	values := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	total := 0
	prev := 0
	for i := len(s) - 1; i >= 0; i-- {
		v := values[s[i]]
		if v < prev {
			total -= v
		} else {
			total += v
		}
		prev = v
	}
	if total <= 0 {
		return 0, false
	}
	return total, true
}

// IntToRoman formats n as a roman numeral for SGB book aliases (1..20).
func IntToRoman(n int) (string, bool) {
	if n <= 0 || n > 20 {
		return "", false
	}
	vals := []int{10, 9, 5, 4, 1}
	syms := []string{"X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String(), true
}

func sgbBookAlternateKeys(s string) []string {
	book, ok := ParseSGBBookQuery(s)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(parts ...string) {
		k := Key(strings.Join(parts, ""))
		if k == "" {
			return
		}
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	add("sgb", strconv.Itoa(book))
	add("sgb_", strconv.Itoa(book))
	if roman, ok := IntToRoman(book); ok {
		add("sgb", roman)
		add("sgb_", roman)
	}
	return out
}

// SGBBookLawIDKeys returns normalized lookup keys for an SGB book law id/abbreviation.
func SGBBookLawIDKeys(lawID, abbreviation string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		k := Key(s)
		if k == "" {
			return
		}
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	add(lawID)
	add(abbreviation)
	id := strings.TrimSpace(lawID)
	if strings.HasPrefix(strings.ToLower(id), "sgb") {
		rest := strings.TrimLeft(id[3:], "_")
		if n, err := strconv.Atoi(rest); err == nil {
			add("sgb " + strconv.Itoa(n))
			add("sgb_" + strconv.Itoa(n))
			if roman, ok := IntToRoman(n); ok {
				add("sgb " + roman)
				add("sgb_" + roman)
			}
		}
	}
	if book, ok := ParseSGBBookQuery(abbreviation); ok {
		add("sgb" + strconv.Itoa(book))
		add("sgb_" + strconv.Itoa(book))
	}
	return out
}
