package export

import (
	"strings"
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

func TestExtractStandRaw_fundstelleFallback_milov5(t *testing.T) {
	raw := ExtractStandRaw(fixtures.MustRead("milov5_snippet.xml"))
	want := "BGBl. 2025 I Nr. 268"
	if raw != want {
		t.Fatalf("got %q want %q", raw, want)
	}
}

func TestExtractStandRaw_preferStandangabeOverFundstelle(t *testing.T) {
	raw := ExtractStandRaw(fixtures.MustRead("milog_snippet.xml"))
	if !strings.Contains(raw, "Nr. 137") {
		t.Fatalf("expected standangabe Nr. 137, got %q", raw)
	}
	if strings.Contains(raw, "2014") {
		t.Fatalf("must not fall back to fundstelle when standangabe present: %q", raw)
	}
}

func TestFormatFundstelleStand(t *testing.T) {
	cases := []struct {
		periodikum, zit, want string
	}{
		{"BGBl. I", "2025 Nr. 268", "BGBl. 2025 I Nr. 268"},
		{"BGBl I", "2025, Nr. 268", "BGBl. 2025 I Nr. 268"},
		{"BGBl. I", "2014 S. 1348", "BGBl. 2014 I S. 1348"},
		{"BGBl. II", "2024, 12", "BGBl. 2024 II Nr. 12"},
		{"", "2025 Nr. 1", ""},
		{"BGBl. I", "", ""},
	}
	for _, tc := range cases {
		if got := formatFundstelleStand(tc.periodikum, tc.zit); got != tc.want {
			t.Fatalf("periodikum=%q zit=%q got %q want %q", tc.periodikum, tc.zit, got, tc.want)
		}
	}
}
