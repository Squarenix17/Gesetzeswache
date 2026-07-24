package instruments

import (
	"path/filepath"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/store"
)

type memLawStore struct {
	laws map[string]domain.Law
}

func (m *memLawStore) GetLaw(id string) (domain.Law, bool, error) {
	l, ok := m.laws[id]
	return l, ok, nil
}

func (m *memLawStore) UpsertLawIfAbsent(law domain.Law) (bool, error) {
	if m.laws == nil {
		m.laws = map[string]domain.Law{}
	}
	if _, ok := m.laws[law.ID]; ok {
		return false, nil
	}
	m.laws[law.ID] = law
	return true, nil
}

func TestEnsureLawFromSlug_createsStub(t *testing.T) {
	st := &memLawStore{}
	law, neu, err := EnsureLawFromSlug(st, "https://www.gesetze-im-internet.de", "milov5")
	if err != nil {
		t.Fatal(err)
	}
	if !neu {
		t.Fatal("expected newly created")
	}
	if law.ID != "milov5" || law.GIIPath != "milov5" {
		t.Fatalf("%+v", law)
	}
	if law.GIIURL != "https://www.gesetze-im-internet.de/milov5/" {
		t.Fatalf("url=%s", law.GIIURL)
	}
	_, neu2, err := EnsureLawFromSlug(st, "https://www.gesetze-im-internet.de", "milov5")
	if err != nil || neu2 {
		t.Fatalf("second call should be no-op neu=%v err=%v", neu2, err)
	}
}

func TestEnsureSeededChildren_fromTSV(t *testing.T) {
	path := filepath.Join("..", "..", "variants", "linked_instruments.tsv")
	cat, err := LoadTSV(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	n, err := EnsureSeededChildren(st, cat, "https://www.gesetze-im-internet.de", "milog")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("created=%d want 2", n)
	}
	for _, slug := range []string{"milov4", "milov5"} {
		if _, ok, _ := st.GetLaw(slug); !ok {
			t.Fatalf("missing %s", slug)
		}
	}
	n2, err := EnsureSeededChildren(st, cat, "https://www.gesetze-im-internet.de", "milog")
	if err != nil || n2 != 0 {
		t.Fatalf("idempotent: n=%d err=%v", n2, err)
	}
}

func TestEnsureLawFromSlug_invalid(t *testing.T) {
	_, _, err := EnsureLawFromSlug(&memLawStore{}, "https://www.gesetze-im-internet.de", "../x")
	if err == nil {
		t.Fatal("expected error")
	}
}
