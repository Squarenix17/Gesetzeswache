// Package search provides an in-memory atomic snapshot for law resolution.
package search

import (
	"sort"
	"sync/atomic"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// Candidate is a ranked match.
type Candidate struct {
	Law   domain.Law
	Score float64
}

// Snapshot is an immutable search index swapped atomically.
type Snapshot struct {
	laws     []domain.Law
	byID     map[string]domain.Law
	byAbbr   map[string][]domain.Law
	byTitle  map[string][]domain.Law
	variants map[string]string // normalized variant -> law id
}

// Engine holds the current atomic snapshot.
type Engine struct {
	snap atomic.Pointer[Snapshot]
}

func NewEngine() *Engine {
	e := &Engine{}
	e.Swap(nil, nil)
	return e
}

// Swap builds and installs a new snapshot from laws and variants.
func (e *Engine) Swap(laws []domain.Law, variants []domain.LawVariant) {
	s := build(laws, variants)
	e.snap.Store(s)
}

func (e *Engine) Current() *Snapshot {
	return e.snap.Load()
}

func build(laws []domain.Law, variants []domain.LawVariant) *Snapshot {
	s := &Snapshot{
		laws:     append([]domain.Law(nil), laws...),
		byID:     map[string]domain.Law{},
		byAbbr:   map[string][]domain.Law{},
		byTitle:  map[string][]domain.Law{},
		variants: map[string]string{},
	}
	for _, l := range laws {
		s.byID[l.ID] = l
		ak := normalize.Key(l.Abbreviation)
		s.byAbbr[ak] = append(s.byAbbr[ak], l)
		tk := normalize.Key(l.Title)
		s.byTitle[tk] = append(s.byTitle[tk], l)
	}
	for _, v := range variants {
		s.variants[normalize.Key(v.Variant)] = v.LawID
	}
	return s
}

// Resolve returns the best match above threshold, or suggestions below it.
func (s *Snapshot) Resolve(query string, threshold float64) (best *Candidate, suggestions []Candidate) {
	if s == nil || query == "" {
		return nil, nil
	}
	qkeys := normalize.AlternateKeys(query)
	scores := map[string]float64{}

	for _, qk := range qkeys {
		if laws, ok := s.byAbbr[qk]; ok {
			for _, l := range laws {
				scores[l.ID] = max(scores[l.ID], 1.0)
			}
		}
		if laws, ok := s.byTitle[qk]; ok {
			for _, l := range laws {
				scores[l.ID] = max(scores[l.ID], 0.98)
			}
		}
		if id, ok := s.variants[qk]; ok {
			scores[id] = max(scores[id], 0.95)
		}
	}

	// Fuzzy: token / levenshtein-lite against abbr and title
	for _, l := range s.laws {
		for _, qk := range qkeys {
			ab := normalize.Key(l.Abbreviation)
			ti := normalize.Key(l.Title)
			scores[l.ID] = max(scores[l.ID], similarity(qk, ab)*0.92)
			scores[l.ID] = max(scores[l.ID], similarity(qk, ti)*0.85)
			if stringsContainsFold(ti, qk) || stringsContainsFold(qk, ab) {
				scores[l.ID] = max(scores[l.ID], 0.8)
			}
		}
	}

	var all []Candidate
	for id, sc := range scores {
		if sc <= 0 {
			continue
		}
		all = append(all, Candidate{Law: s.byID[id], Score: sc})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Score == all[j].Score {
			return all[i].Law.Abbreviation < all[j].Law.Abbreviation
		}
		return all[i].Score > all[j].Score
	})
	if len(all) == 0 {
		return nil, nil
	}
	if all[0].Score >= threshold {
		best = &all[0]
		if len(all) > 1 {
			suggestions = all[1:]
			if len(suggestions) > 10 {
				suggestions = suggestions[:10]
			}
		}
		return best, suggestions
	}
	if len(all) > 10 {
		all = all[:10]
	}
	return nil, all
}

func (s *Snapshot) ByID(id string) (domain.Law, bool) {
	if s == nil {
		return domain.Law{}, false
	}
	l, ok := s.byID[id]
	return l, ok
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func stringsContainsFold(hay, needle string) bool {
	return needle != "" && (hay == needle || (len(hay) >= len(needle) && contains(hay, needle)))
}

func contains(h, n string) bool {
	return len(n) > 0 && (h == n || indexOf(h, n) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// similarity is a cheap normalized score in [0,1].
func similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	// prefix bonus
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	pref := 0
	for pref < n && a[pref] == b[pref] {
		pref++
	}
	dist := levenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	score := 1 - float64(dist)/float64(maxLen)
	if pref > 0 {
		score = max(score, float64(pref)/float64(maxLen)*0.9)
	}
	if score < 0 {
		return 0
	}
	return score
}

func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = min3(del, ins, sub)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}
