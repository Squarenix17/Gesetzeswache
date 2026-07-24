package export

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
)

func TestExtractStandRaw_fromFixture(t *testing.T) {
	raw := ExtractStandRaw(fixtures.MustRead("arbzg_snippet.xml"))
	want := "Zuletzt geändert durch Art. 1 G v. 20.7.2022 BGBl. I S. 1170"
	if raw != want {
		t.Fatalf("got %q want %q", raw, want)
	}
}

func TestExtractStandRaw_empty(t *testing.T) {
	if got := ExtractStandRaw([]byte(`<dokument><norm></norm></dokument>`)); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := ExtractStandRaw([]byte(`not xml`)); got != "" {
		t.Fatalf("got %q", got)
	}
}
