package instruments

import (
	"fmt"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

// LawUpserter is the store surface needed to ensure seeded child laws exist.
type LawUpserter interface {
	GetLaw(id string) (domain.Law, bool, error)
	UpsertLawIfAbsent(law domain.Law) (created bool, err error)
}

// EnsureLawFromSlug creates a minimal Law stub for a GII slug when missing.
// Returns the law, whether it was newly created, and any error.
func EnsureLawFromSlug(st LawUpserter, giiBase, slug string) (domain.Law, bool, error) {
	slug = strings.TrimSpace(slug)
	if !giiurl.ValidSlug(slug) {
		return domain.Law{}, false, fmt.Errorf("invalid gii slug %q", slug)
	}
	id := normalize.Key(slug)
	if law, ok, err := st.GetLaw(id); err != nil {
		return domain.Law{}, false, err
	} else if ok {
		return law, false, nil
	}
	indexURL, err := giiurl.IndexURL(giiBase, slug)
	if err != nil {
		return domain.Law{}, false, err
	}
	stub := domain.Law{
		ID:           id,
		Abbreviation: strings.ToUpper(id),
		Title:        slug,
		GIIPath:      slug,
		GIIURL:       indexURL,
	}
	created, err := st.UpsertLawIfAbsent(stub)
	if err != nil {
		return domain.Law{}, false, err
	}
	if !created {
		// Lost the race to TOC sync (or another ensure); re-read winner.
		law, ok, err := st.GetLaw(id)
		if err != nil {
			return domain.Law{}, false, err
		}
		if !ok {
			return domain.Law{}, false, fmt.Errorf("law %s missing after UpsertLawIfAbsent", id)
		}
		return law, false, nil
	}
	return stub, true, nil
}

// EnsureSeededChildren ensures every seeded gii_slug for parent exists as a Law.
// Returns how many stubs were newly created.
func EnsureSeededChildren(st LawUpserter, cat *Catalog, giiBase, parentLawID string) (int, error) {
	created := 0
	for _, li := range ForParentSafe(cat, parentLawID) {
		_, neu, err := EnsureLawFromSlug(st, giiBase, li.GIISlug)
		if err != nil {
			return created, err
		}
		if neu {
			created++
		}
	}
	return created, nil
}
