package store

import (
	"encoding/json"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
	"go.etcd.io/bbolt"
)

var (
	bucketBGBlIndex       = []byte("bgbl_index")
	bucketDiscoveredLinks = []byte("discovered_links")
)

// BGBlIndexEntry maps a BGBl citation to a GII slug and optional law id.
type BGBlIndexEntry struct {
	Teil    int    `json:"teil"`
	Year    int    `json:"year"`
	Number  string `json:"number"`
	GIISlug string `json:"gii_slug"`
	LawID   string `json:"law_id,omitempty"`
}

func bgblIndexKey(teil, year int, number string) string {
	return citation.IssueID(teil, year, number)
}

func discoveredKey(parent, slug string) string {
	return normalize.Key(parent) + "|" + slug
}

func (s *Store) ensureDiscoveryBuckets(tx *bbolt.Tx) error {
	for _, b := range [][]byte{bucketBGBlIndex, bucketDiscoveredLinks} {
		if _, err := tx.CreateBucketIfNotExists(b); err != nil {
			return err
		}
	}
	return nil
}

// UpsertBGBlIndex stores or updates a BGBl citation → GII slug mapping.
func (s *Store) UpsertBGBlIndex(e BGBlIndexEntry) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		if err := s.ensureDiscoveryBuckets(tx); err != nil {
			return err
		}
		return putJSON(tx.Bucket(bucketBGBlIndex), bgblIndexKey(e.Teil, e.Year, e.Number), e)
	})
}

// LookupBGBlIndex returns a BGBl index entry by citation parts.
func (s *Store) LookupBGBlIndex(teil, year int, number string) (BGBlIndexEntry, bool, error) {
	var e BGBlIndexEntry
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketBGBlIndex)
		if b == nil {
			return nil
		}
		ok, err := getJSON(b, bgblIndexKey(teil, year, number), &e)
		found = ok
		return err
	})
	return e, found, err
}

// UpsertDiscoveredLink stores or updates a discovered parent→child edge (idempotent by key).
func (s *Store) UpsertDiscoveredLink(e domain.DiscoveredEdge) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		if err := s.ensureDiscoveryBuckets(tx); err != nil {
			return err
		}
		return putJSON(tx.Bucket(bucketDiscoveredLinks), discoveredKey(e.ParentLawID, e.GIISlug), e)
	})
}

// DeleteDiscoveredBySlug removes all discovered edges pointing at a child GII slug.
// Used on re-ingest so a corrected parent attribution does not leave stale parents.
func (s *Store) DeleteDiscoveredBySlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketDiscoveredLinks)
		if b == nil {
			return nil
		}
		var toDelete [][]byte
		err := b.ForEach(func(k, v []byte) error {
			var e domain.DiscoveredEdge
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			if e.GIISlug == slug {
				toDelete = append(toDelete, append([]byte(nil), k...))
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// DiscoveredForParent returns all discovered edges for a parent law id.
func (s *Store) DiscoveredForParent(parent string) ([]domain.DiscoveredEdge, error) {
	parentKey := normalize.Key(parent)
	var out []domain.DiscoveredEdge
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketDiscoveredLinks)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var e domain.DiscoveredEdge
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			if normalize.Key(e.ParentLawID) == parentKey {
				out = append(out, e)
			}
			return nil
		})
	})
	return out, err
}

// ClearDiscoveredLinks removes all discovered parent→child edges (maintenance / re-bootstrap).
func (s *Store) ClearDiscoveredLinks() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		_ = tx.DeleteBucket(bucketDiscoveredLinks)
		_, err := tx.CreateBucketIfNotExists(bucketDiscoveredLinks)
		return err
	})
}

// CountDiscoveredLinks returns the number of persisted discovered parent→child edges.
func (s *Store) CountDiscoveredLinks() (int, error) {
	var n int
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketDiscoveredLinks)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, _ []byte) error {
			n++
			return nil
		})
	})
	return n, err
}

// CountBGBlIndex returns the number of BGBl citation → GII slug index entries.
func (s *Store) CountBGBlIndex() (int, error) {
	var n int
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketBGBlIndex)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, _ []byte) error {
			n++
			return nil
		})
	})
	return n, err
}
