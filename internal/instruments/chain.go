package instruments

import (
	"sort"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

const (
	StatusPast    = "past"
	StatusCurrent = "current"
	StatusFuture  = "future"

	CoverageSection = "section"
)

// AnnotateChain labels each linked instrument past|current|future per section_hint group and asOf.
func AnnotateChain(rows []domain.LinkedInstrument, asOf time.Time) []domain.LinkedInstrument {
	if len(rows) == 0 {
		return nil
	}
	out := make([]domain.LinkedInstrument, len(rows))
	copy(out, rows)
	for i := range out {
		if out[i].Coverage == "" {
			out[i].Coverage = CoverageSection
		}
	}

	asOfDate := dateOnlyUTC(asOf)
	type groupKey struct {
		key string
		idx int
	}
	groups := map[string][]int{}
	order := []groupKey{}
	for i, row := range out {
		key := normalizeSectionHint(row.SectionHint)
		if _, ok := groups[key]; !ok {
			order = append(order, groupKey{key: key, idx: len(order)})
		}
		groups[key] = append(groups[key], i)
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].idx < order[j].idx })

	for _, gk := range order {
		idxs := groups[gk.key]
		sort.SliceStable(idxs, func(i, j int) bool {
			di, oki := parseEffectiveDate(out[idxs[i]].EffectiveFrom)
			dj, okj := parseEffectiveDate(out[idxs[j]].EffectiveFrom)
			if oki && okj {
				return di.Before(dj)
			}
			if oki {
				return true
			}
			if okj {
				return false
			}
			return idxs[i] < idxs[j]
		})

		for pos, idx := range idxs {
			effective, ok := parseEffectiveDate(out[idx].EffectiveFrom)
			if !ok {
				continue
			}
			if effective.After(asOfDate) {
				out[idx].Status = StatusFuture
				continue
			}
			superseded := false
			for later := pos + 1; later < len(idxs); later++ {
				laterDate, laterOK := parseEffectiveDate(out[idxs[later]].EffectiveFrom)
				if laterOK && !laterDate.After(asOfDate) {
					superseded = true
					break
				}
			}
			if superseded {
				out[idx].Status = StatusPast
			} else {
				out[idx].Status = StatusCurrent
			}
		}
	}
	return out
}

// FilterLinkedForResponse drops past instruments unless includePast is true.
func FilterLinkedForResponse(rows []domain.LinkedInstrument, includePast bool) []domain.LinkedInstrument {
	if includePast {
		return append([]domain.LinkedInstrument(nil), rows...)
	}
	out := make([]domain.LinkedInstrument, 0, len(rows))
	for _, row := range rows {
		if row.Status == StatusPast {
			continue
		}
		out = append(out, row)
	}
	return out
}

// FilterBundleMembers keeps status=current; keeps past iff includePast; always drops future and empty status.
// Bundle membership is stricter than FilterLinkedForResponse (which retains future).
func FilterBundleMembers(rows []domain.LinkedInstrument, includePast bool) []domain.LinkedInstrument {
	out := make([]domain.LinkedInstrument, 0, len(rows))
	for _, row := range rows {
		switch row.Status {
		case StatusCurrent:
			out = append(out, row)
		case StatusPast:
			if includePast {
				out = append(out, row)
			}
		default:
			// StatusFuture and empty status are excluded from operative bundles.
		}
	}
	return out
}

func normalizeSectionHint(hint string) string {
	h := strings.TrimSpace(hint)
	if h == "" {
		return "*"
	}
	return h
}

func parseEffectiveDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return dateOnlyUTC(t), true
}

func dateOnlyUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
