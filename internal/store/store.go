// Package store provides an embedded bbolt persistence layer.
package store

import (
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

var (
	bucketLaws      = []byte("laws")
	bucketVariants  = []byte("variants")
	bucketStand     = []byte("stand")
	bucketIssues    = []byte("issues")
	bucketLinks     = []byte("links") // key: issueID|lawID
	bucketFreshness = []byte("freshness")
	bucketSyncMeta  = []byte("sync_meta")
	bucketSyncLog   = []byte("sync_log")
)

// Store is an ACID embedded database.
type Store struct {
	db *bbolt.DB
}

func Open(path string) (*Store, error) {
	db, err := bbolt.Open(path, 0o600, &bbolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, b := range [][]byte{bucketLaws, bucketVariants, bucketStand, bucketIssues, bucketLinks, bucketFreshness, bucketSyncMeta, bucketSyncLog, bucketBGBlIndex, bucketDiscoveredLinks} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func putJSON(b *bbolt.Bucket, key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}

func getJSON(b *bbolt.Bucket, key string, v any) (bool, error) {
	data := b.Get([]byte(key))
	if data == nil {
		return false, nil
	}
	return true, json.Unmarshal(data, v)
}

func (s *Store) UpsertLaws(laws []domain.Law) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketLaws)
		for _, l := range laws {
			if err := putJSON(b, l.ID, l); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertLawIfAbsent inserts law only when id is missing (atomic vs concurrent TOC sync).
// Returns true if a new record was written.
func (s *Store) UpsertLawIfAbsent(law domain.Law) (created bool, err error) {
	err = s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketLaws)
		if b.Get([]byte(law.ID)) != nil {
			created = false
			return nil
		}
		created = true
		return putJSON(b, law.ID, law)
	})
	return created, err
}

func (s *Store) ListLaws() ([]domain.Law, error) {
	var out []domain.Law
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketLaws)
		return b.ForEach(func(k, v []byte) error {
			var l domain.Law
			if err := json.Unmarshal(v, &l); err != nil {
				return err
			}
			out = append(out, l)
			return nil
		})
	})
	return out, err
}

func (s *Store) GetLaw(id string) (domain.Law, bool, error) {
	var l domain.Law
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		ok, err := getJSON(tx.Bucket(bucketLaws), id, &l)
		found = ok
		return err
	})
	return l, found, err
}

func (s *Store) ReplaceVariants(variants []domain.LawVariant) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		_ = tx.DeleteBucket(bucketVariants)
		b, err := tx.CreateBucket(bucketVariants)
		if err != nil {
			return err
		}
		for i, v := range variants {
			key := fmt.Sprintf("%d:%s", i, v.Variant)
			if err := putJSON(b, key, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListVariants() ([]domain.LawVariant, error) {
	var out []domain.LawVariant
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketVariants)
		return b.ForEach(func(_, v []byte) error {
			var x domain.LawVariant
			if err := json.Unmarshal(v, &x); err != nil {
				return err
			}
			out = append(out, x)
			return nil
		})
	})
	return out, err
}

func (s *Store) UpsertStand(c domain.StandCitation) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx.Bucket(bucketStand), c.LawID, c)
	})
}

func (s *Store) GetStand(lawID string) (domain.StandCitation, bool, error) {
	var c domain.StandCitation
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		ok, err := getJSON(tx.Bucket(bucketStand), lawID, &c)
		found = ok
		return err
	})
	return c, found, err
}

func (s *Store) UpsertIssue(iss domain.GazetteIssue) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx.Bucket(bucketIssues), iss.ID, iss)
	})
}

func (s *Store) GetIssue(id string) (domain.GazetteIssue, bool, error) {
	var iss domain.GazetteIssue
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		ok, err := getJSON(tx.Bucket(bucketIssues), id, &iss)
		found = ok
		return err
	})
	return iss, found, err
}

func (s *Store) ListIssues() ([]domain.GazetteIssue, error) {
	var out []domain.GazetteIssue
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketIssues).ForEach(func(_, v []byte) error {
			var iss domain.GazetteIssue
			if err := json.Unmarshal(v, &iss); err != nil {
				return err
			}
			out = append(out, iss)
			return nil
		})
	})
	return out, err
}

func (s *Store) UpsertLink(link domain.IssueLawLink) error {
	key := link.IssueID + "|" + link.LawID
	return s.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx.Bucket(bucketLinks), key, link)
	})
}

func (s *Store) LinksForLaw(lawID string) ([]domain.IssueLawLink, error) {
	var out []domain.IssueLawLink
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketLinks).ForEach(func(_, v []byte) error {
			var l domain.IssueLawLink
			if err := json.Unmarshal(v, &l); err != nil {
				return err
			}
			if l.LawID == lawID {
				out = append(out, l)
			}
			return nil
		})
	})
	return out, err
}

func (s *Store) ListLinks() ([]domain.IssueLawLink, error) {
	var out []domain.IssueLawLink
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketLinks).ForEach(func(_, v []byte) error {
			var l domain.IssueLawLink
			if err := json.Unmarshal(v, &l); err != nil {
				return err
			}
			out = append(out, l)
			return nil
		})
	})
	return out, err
}

func (s *Store) PutFreshness(r domain.FreshnessRecord) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx.Bucket(bucketFreshness), r.LawID, r)
	})
}

func (s *Store) GetFreshness(lawID string) (domain.FreshnessRecord, bool, error) {
	var r domain.FreshnessRecord
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		ok, err := getJSON(tx.Bucket(bucketFreshness), lawID, &r)
		found = ok
		return err
	})
	return r, found, err
}

func (s *Store) ListFreshnessByState(state domain.FreshnessState) ([]domain.FreshnessRecord, error) {
	var out []domain.FreshnessRecord
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketFreshness).ForEach(func(_, v []byte) error {
			var r domain.FreshnessRecord
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			if r.State == state {
				out = append(out, r)
			}
			return nil
		})
	})
	return out, err
}

// CountFreshnessByState returns counts of freshness records per known state.
func (s *Store) CountFreshnessByState() (map[domain.FreshnessState]int, error) {
	out := map[domain.FreshnessState]int{
		domain.FreshnessConfirmedCurrent: 0,
		domain.FreshnessConfirmedStale:   0,
		domain.FreshnessUncertain:        0,
	}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketFreshness).ForEach(func(_, v []byte) error {
			var r domain.FreshnessRecord
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			out[r.State]++
			return nil
		})
	})
	return out, err
}

func (s *Store) SetMetaTime(key string, t time.Time) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketSyncMeta).Put([]byte(key), []byte(t.UTC().Format(time.RFC3339Nano)))
	})
}

func (s *Store) GetMetaTime(key string) (time.Time, bool, error) {
	var t time.Time
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(bucketSyncMeta).Get([]byte(key))
		if v == nil {
			return nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, string(v))
		if err != nil {
			return err
		}
		t = parsed
		found = true
		return nil
	})
	return t, found, err
}

// SetMeta stores an opaque string in sync_meta (e.g. editorial instrument fingerprint).
func (s *Store) SetMeta(key, val string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketSyncMeta).Put([]byte(key), []byte(val))
	})
}

// GetMeta reads an opaque string from sync_meta.
func (s *Store) GetMeta(key string) (string, bool, error) {
	var out string
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(bucketSyncMeta).Get([]byte(key))
		if v == nil {
			return nil
		}
		out = string(v)
		found = true
		return nil
	})
	return out, found, err
}

func (s *Store) AppendSyncAttempt(a domain.SyncAttempt) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSyncLog)
		id, _ := b.NextSequence()
		a.ID = fmt.Sprintf("%d", id)
		return putJSON(b, a.ID, a)
	})
}
