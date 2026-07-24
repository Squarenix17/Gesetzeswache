package citation

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

func TestParseMiLoGStand(t *testing.T) {
	c := Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137")
	if !c.ParseOK {
		t.Fatalf("expected parse ok, notes=%s", c.ParseNotes)
	}
	if c.Year != 2026 || c.Teil != 1 || c.Number != "137" {
		t.Fatalf("got year=%d teil=%d num=%s", c.Year, c.Teil, c.Number)
	}
}

func TestParseLinkedInstruments(t *testing.T) {
	refs := ParseLinkedInstruments("§ 1 V v. 5.11.2025 I Nr. 268")
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	ref := refs[0]
	if ref.Kind != "V" || ref.Year != 2025 || ref.Teil != 1 || ref.Number != "268" {
		t.Fatalf("got %+v", ref)
	}
	if ref.SectionHint != "§ 1" {
		t.Fatalf("section hint: got %q", ref.SectionHint)
	}
}

func TestFingerprintInstruments(t *testing.T) {
	a := []domain.InstrumentRef{{Kind: "V", Teil: 1, Year: 2025, Number: "268"}}
	b := []domain.InstrumentRef{{Kind: "V", Teil: 1, Year: 2025, Number: "268"}}
	if FingerprintInstruments(a) != FingerprintInstruments(b) {
		t.Fatal("expected stable fingerprint")
	}
	if FingerprintInstruments(nil) != "" {
		t.Fatal("empty refs should yield empty fingerprint")
	}
}

func TestParseYearNumber(t *testing.T) {
	c := Parse("bgb", "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 217")
	if !c.ParseOK {
		t.Fatalf("expected parse ok, notes=%s raw fields teil=%d year=%d num=%s", c.ParseNotes, c.Teil, c.Year, c.Number)
	}
	if c.Year != 2023 || c.Teil != 1 || c.Number != "217" {
		t.Fatalf("got year=%d teil=%d num=%s", c.Year, c.Teil, c.Number)
	}
}

func TestParseTeilPage(t *testing.T) {
	c := Parse("x", "Zuletzt geändert durch Art. 2 G v. 1.1.2024 I 55")
	if !c.ParseOK {
		t.Fatalf("expected parse ok: %+v", c)
	}
	if c.Teil != 1 || c.Page != "55" || c.Year != 2024 {
		t.Fatalf("got %+v", c)
	}
}

func TestCompare(t *testing.T) {
	older := Parse("a", "BGBl. 2022 I Nr. 10")
	newer := Parse("a", "BGBl. 2023 I Nr. 1")
	cmp, ok := Compare(older, newer)
	if !ok || cmp != -1 {
		t.Fatalf("cmp=%d ok=%v", cmp, ok)
	}
}

func TestEmpty(t *testing.T) {
	c := Parse("a", "")
	if c.ParseOK {
		t.Fatal("empty should not parse ok")
	}
}
