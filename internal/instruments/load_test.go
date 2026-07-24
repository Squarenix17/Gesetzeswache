package instruments

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "linked.tsv")
	content := `# comment
milog	verordnung	milov4	Vierte Mindestlohnanpassungsverordnung (BGBl 2023 I Nr. 321)	2024-01-01	§ 1
milog	verordnung	milov5	Fünfte Mindestlohnanpassungsverordnung (BGBl 2025 I Nr. 268)	2026-01-01	§ 1
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadTSV(path)
	if err != nil {
		t.Fatal(err)
	}
	got := c.ForParent("MiLoG")
	if len(got) != 2 {
		t.Fatalf("got %d want 2", len(got))
	}
	if got[0].GIISlug != "milov4" || got[1].GIISlug != "milov5" {
		t.Fatalf("%+v", got)
	}
}

func TestLoadTSV_multiRowRequiresEffectiveFromAndSectionHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "linked.tsv")
	content := `milog	verordnung	milov4	Vierte Mindestlohnanpassungsverordnung (BGBl 2023 I Nr. 321)
milog	verordnung	milov5	Fünfte Mindestlohnanpassungsverordnung (BGBl 2025 I Nr. 268)
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTSV(path); err == nil {
		t.Fatal("expected error when multi-row parent lacks effective_from/section_hint")
	}
}

func TestLoadTSV_missing(t *testing.T) {
	_, err := LoadTSV(filepath.Join(t.TempDir(), "nope.tsv"))
	if err == nil {
		t.Fatal("expected error for missing configured path")
	}
}

func TestLoadTSV_emptyPath(t *testing.T) {
	c, err := LoadTSV("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Fatal("expected empty catalog")
	}
}

func TestLoadTSV_sixColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "linked.tsv")
	content := `milog	verordnung	milov4	Vierte Mindestlohnanpassungsverordnung (BGBl 2023 I Nr. 321)	2024-01-01	§ 1
milog	verordnung	milov5	Fünfte Mindestlohnanpassungsverordnung (BGBl 2025 I Nr. 268)	2026-01-01	§ 1
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadTSV(path)
	if err != nil {
		t.Fatal(err)
	}
	got := c.ForParent("milog")
	if len(got) != 2 {
		t.Fatalf("got %d want 2", len(got))
	}
	if got[0].EffectiveFrom != "2024-01-01" || got[0].SectionHint != "§ 1" {
		t.Fatalf("milov4: %+v", got[0])
	}
	if got[1].EffectiveFrom != "2026-01-01" || got[1].SectionHint != "§ 1" {
		t.Fatalf("milov5: %+v", got[1])
	}
	if got[0].Coverage != CoverageSection || got[1].Coverage != CoverageSection {
		t.Fatalf("coverage: %+v", got)
	}
}

func TestLoadTSV_invalidEffectiveFrom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.tsv")
	content := "milog\tverordnung\tmilov4\tV v. 1.1.2023 I Nr. 321\tnot-a-date\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTSV(path); err == nil {
		t.Fatal("expected error for invalid effective_from")
	}
}

func TestLoadTSV_notesRequireCitation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.tsv")
	if err := os.WriteFile(path, []byte("milog\tverordnung\tmilov5\tno citation here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTSV(path); err == nil {
		t.Fatal("expected error when notes lack BGBl citation")
	}
}

func TestLoadTSV_invalidSlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.tsv")
	if err := os.WriteFile(path, []byte("milog\tverordnung\t../x\tBGBl 2025 I Nr. 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTSV(path); err == nil {
		t.Fatal("expected invalid slug error")
	}
}
