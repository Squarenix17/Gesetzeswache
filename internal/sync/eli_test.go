package sync

import (
	"testing"
)

func TestParseELI(t *testing.T) {
	teil, year, num := parseELI("https://www.recht.bund.de/eli/bund/bgbl-1/2026/216")
	if teil != 1 || year != 2026 || num != "216" {
		t.Fatalf("got %d %d %s", teil, year, num)
	}
}
