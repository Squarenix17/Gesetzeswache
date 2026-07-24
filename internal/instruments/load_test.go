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
milog	verordnung	milov4	Vierte Mindestlohnanpassungsverordnung (BGBl 2023 I Nr. 321)
milog	verordnung	milov5	Fünfte Mindestlohnanpassungsverordnung (BGBl 2025 I Nr. 268)
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
