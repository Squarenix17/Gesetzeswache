// Package citation parses GII "Stand" strings into structured StandCitation values.
package citation

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

var (
	// Examples:
	// "Zuletzt geändert durch Art. 1 G v. 16.8.2023 I 2174"
	// "neugefasst durch Bek. v. 1.1.2024 I 1"
	// "BGBl. I S. 1234" / "BGBl. 2024 I Nr. 12"
	reBGBlFull = regexp.MustCompile(`(?i)BGBl\.?\s*(?:([I1]|[I1]{2}|Teil\s*[I12]))?\s*(?:(\d{4})\s*)?(?:Nr\.?\s*([0-9]+[a-zA-Z]?)\s*)?(?:S\.?\s*([0-9]+))?`)
	reYearNum  = regexp.MustCompile(`(?i)BGBl\.?\s*(\d{4})\s*([I12])\s*(?:Nr\.?\s*)?([0-9]+[a-zA-Z]?)`)
	reDate     = regexp.MustCompile(`(\d{1,2})\.(\d{1,2})\.(\d{4})`)
	reTeilPage = regexp.MustCompile(`(?i)\b([I12])\s+(\d{2,})\b`)
)

// Parse converts a raw Stand string into a structured citation.
// It never invents fields; ParseOK is false when insufficient structure is found.
func Parse(lawID, raw string) domain.StandCitation {
	raw = strings.TrimSpace(raw)
	out := domain.StandCitation{LawID: lawID, Raw: raw}
	if raw == "" {
		out.ParseNotes = "empty stand"
		return out
	}

	if m := reYearNum.FindStringSubmatch(raw); len(m) == 4 {
		out.Year, _ = strconv.Atoi(m[1])
		out.Teil = teilFrom(m[2])
		out.Number = m[3]
		out.ParseOK = out.Year > 0 && out.Teil > 0 && out.Number != ""
		if out.ParseOK {
			attachDate(&out, raw)
			return out
		}
	}

	if m := reBGBlFull.FindStringSubmatch(raw); len(m) >= 1 {
		if m[1] != "" {
			out.Teil = teilFrom(m[1])
		}
		if m[2] != "" {
			out.Year, _ = strconv.Atoi(m[2])
		}
		if m[3] != "" {
			out.Number = m[3]
		}
		if m[4] != "" {
			out.Page = m[4]
		}
	}

	if out.Teil == 0 || out.Year == 0 {
		if m := reTeilPage.FindStringSubmatch(raw); len(m) == 3 {
			out.Teil = teilFrom(m[1])
			out.Page = m[2]
			if out.Year == 0 {
				if dm := reDate.FindStringSubmatch(raw); len(dm) == 4 {
					out.Year, _ = strconv.Atoi(dm[3])
				}
			}
		}
	}

	attachDate(&out, raw)

	// Comparable enough if we have year + (number or page) + teil
	out.ParseOK = out.Year > 0 && out.Teil > 0 && (out.Number != "" || out.Page != "")
	if !out.ParseOK {
		out.ParseNotes = "insufficient structured fields"
	}
	return out
}

func teilFrom(s string) int {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.TrimPrefix(s, "TEIL")
	s = strings.TrimSpace(s)
	switch s {
	case "I", "1":
		return 1
	case "II", "2":
		return 2
	default:
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		return 0
	}
}

func attachDate(out *domain.StandCitation, raw string) {
	if m := reDate.FindStringSubmatch(raw); len(m) == 4 {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
		out.Date = &t
		if out.Year == 0 {
			out.Year = y
		}
	}
}

// Compare returns -1 if a is older than b, 0 if equal/incomparable, 1 if a is newer.
// Prefer year+number when both have numbers; else year+page; incomparable => 0 with ok=false.
func Compare(a, b domain.StandCitation) (cmp int, ok bool) {
	if a.Year == 0 || b.Year == 0 || a.Teil == 0 || b.Teil == 0 {
		return 0, false
	}
	if a.Teil != b.Teil {
		// Different parts are not ordered against each other for "newer amendment of same law"
		// Callers should compare issues within context; treat as incomparable here.
		return 0, false
	}
	if a.Year != b.Year {
		if a.Year < b.Year {
			return -1, true
		}
		return 1, true
	}
	if a.Number != "" && b.Number != "" {
		an, aok := parseIssueNum(a.Number)
		bn, bok := parseIssueNum(b.Number)
		if aok && bok {
			if an < bn {
				return -1, true
			}
			if an > bn {
				return 1, true
			}
			return 0, true
		}
	}
	if a.Page != "" && b.Page != "" {
		ap, _ := strconv.Atoi(a.Page)
		bp, _ := strconv.Atoi(b.Page)
		if ap < bp {
			return -1, true
		}
		if ap > bp {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func parseIssueNum(s string) (int, bool) {
	n := 0
	for i, r := range s {
		if r < '0' || r > '9' {
			if i == 0 {
				return 0, false
			}
			break
		}
		n = n*10 + int(r-'0')
	}
	return n, n > 0
}

// IssueID builds a stable gazette issue id.
func IssueID(teil, year int, number string) string {
	return "BGBl-" + strconv.Itoa(teil) + "/" + strconv.Itoa(year) + "/" + number
}
