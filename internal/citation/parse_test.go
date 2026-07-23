package citation

import "testing"

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
