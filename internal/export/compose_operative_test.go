package export

import (
	"strings"
	"testing"
)

func TestStripVOHierarchicalBody(t *testing.T) {
	in := "# MiLoV5 — Title\n\n## § 1 Höhe\n\n### Abs. 1\n\nDer Mindestlohn beträgt 13,90 Euro.\n"
	got := StripVOHierarchicalBody(in)
	if got != "Der Mindestlohn beträgt 13,90 Euro." {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "§") {
		t.Fatalf("body should not retain section markers: %q", got)
	}
}

func TestSectionHintMatchesHeader(t *testing.T) {
	if !SectionHintMatchesHeader("## § 1 Mindestlohn", "§ 1") {
		t.Fatal("expected match")
	}
	if SectionHintMatchesHeader("## § 2", "§ 1") {
		t.Fatal("unexpected match")
	}
}

func TestComposeOperativeHierarchical_insertsAfterSection(t *testing.T) {
	parent := "# MiLoG — Mindestlohngesetz\n\n## § 1 Mindestlohn\n\n### Abs. 1\n\nAnspruch.\n\n### Abs. 2\n\n12 Euro.\n\n## § 2\n\nAndere.\n"
	members := []OperativeComposeMember{{
		LawID: "milov5", Abbreviation: "MiLoV5", SectionHint: "§ 1",
		Status: "current", EffectiveFrom: "2026-01-01",
		Hierarchical: "# MiLoV5\n\n## § 1\n\n13,90 Euro.\n",
	}}
	got := ComposeOperativeHierarchical(parent, "MiLoG", members)
	if !strings.Contains(got, "Verordnung (nicht Teil des MiLoG)") {
		t.Fatalf("missing banner:\n%s", got)
	}
	if !strings.Contains(got, "ab 2026-01-01\n\n> 13,90") {
		t.Fatalf("expected blank line after preview header:\n%s", got)
	}
	if !strings.Contains(got, "13,90 Euro") {
		t.Fatalf("missing VO body:\n%s", got)
	}
	if strings.Contains(got, "## § 1\n\n13,90") {
		t.Fatalf("VO section heading leaked:\n%s", got)
	}
	idxVO := strings.Index(got, "13,90 Euro")
	idxS2 := strings.Index(got, "## § 2")
	if idxVO < 0 || idxS2 < 0 || idxVO > idxS2 {
		t.Fatalf("VO should appear before § 2; vo=%d s2=%d\n%s", idxVO, idxS2, got)
	}
	if PlacementForHint(parent, "§ 1") != PlacementAfterParentSection {
		t.Fatal("placement")
	}
}

func TestComposeOperativeHierarchical_unknownHintGoesToEnd(t *testing.T) {
	parent := "# X\n\n## § 1\n\nBody.\n"
	members := []OperativeComposeMember{{
		LawID: "child", Abbreviation: "Child", SectionHint: "§ 99",
		Hierarchical: "# Child\n\nOnly body.\n",
	}}
	got := ComposeOperativeHierarchical(parent, "X", members)
	if PlacementForHint(parent, "§ 99") != PlacementDocumentEnd {
		t.Fatal("placement")
	}
	if strings.Index(got, "Only body") < strings.Index(got, "## § 1") {
		t.Fatalf("expected end placement:\n%s", got)
	}
}
