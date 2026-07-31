package export

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
)

func TestExtractAusfertigungDatum_milov5Fixture(t *testing.T) {
	got := ExtractAusfertigungDatum(fixtures.MustRead("milov5_snippet.xml"))
	if got != "2025-11-05" {
		t.Fatalf("got %q want 2025-11-05", got)
	}
}

func TestExtractAusfertigungDatum_emptyAndMalformed(t *testing.T) {
	cases := []struct {
		name string
		xml  string
	}{
		{"empty document", `<dokument><norm><metadaten></metadaten></norm></dokument>`},
		{"malformed date", `<dokument><norm><metadaten><ausfertigung-datum>05.11.2025</ausfertigung-datum></metadaten></norm></dokument>`},
		{"not xml", "not xml"},
		{"partial date", `<dokument><norm><metadaten><ausfertigung-datum>2025-11</ausfertigung-datum></metadaten></norm></dokument>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractAusfertigungDatum([]byte(tc.xml)); got != "" {
				t.Fatalf("got %q want empty", got)
			}
		})
	}
}
