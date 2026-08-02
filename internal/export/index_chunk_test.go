package export

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSectionRefs(t *testing.T) {
	if got := ParseSectionRefs(""); got != nil {
		t.Fatalf("empty want nil, got %#v", got)
	}
	got := ParseSectionRefs("§ 1, § 2,")
	if len(got) != 2 || got[0] != "§ 1" || got[1] != "§ 2" {
		t.Fatalf("got %#v", got)
	}
	got = ParseSectionRefs("§1")
	if len(got) != 1 || got[0] != "§1" {
		t.Fatalf("got %#v", got)
	}
}

func TestSectionRefKey(t *testing.T) {
	if sectionRefKey("§ 1") != sectionRefKey("§1") {
		t.Fatal("§ 1 and §1 should normalize equal")
	}
	if sectionRefKey("§  2") != sectionRefKey("§ 2") {
		t.Fatal("extra spaces should collapse")
	}
	if sectionRefKey("§ 1") == sectionRefKey("§ 2") {
		t.Fatal("different sections must not match")
	}
}

func TestIndexChunkFromExport_parentOmitsParentFields(t *testing.T) {
	src := Chunk{
		UnitID:       "milog:s1/1",
		LawID:        "milog",
		Abbreviation: "MiLoG",
		SectionRef:   "§ 1",
		ParagraphNum: "1",
		SectionTitle: "Mindestlohn",
		Kind:         KindNormtext,
		StandRaw:     "Stand …",
		Freshness:    "uncertain",
		Text:         "body",
	}
	c := IndexChunkFromExport(src, "Mindestlohngesetz", "gesetz", "", "")
	if c.ChunkID != "milog:s1/1" || c.LawID != "milog" || c.LawName != "Mindestlohngesetz" {
		t.Fatalf("%+v", c)
	}
	if c.InstrumentKind != "gesetz" || c.SectionRef != "§ 1" || c.SectionName != "Mindestlohn" || c.Text != "body" {
		t.Fatalf("%+v", c)
	}
	if c.ParentLawID != "" || c.ParentSectionHint != "" {
		t.Fatalf("parent fields must be empty: %+v", c)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, bad := range []string{
		"abbreviation", "paragraph_num", "stand_raw", "freshness", "safe_to_serve",
		"parent_law_id", "parent_section_hint", "chunk_index", "content_hash",
	} {
		if strings.Contains(s, `"`+bad+`"`) {
			t.Fatalf("forbidden field %q in %s", bad, s)
		}
	}
}

func TestIndexChunkFromExport_operativeParentFields(t *testing.T) {
	src := Chunk{UnitID: "milov5:u1", LawID: "milov5", SectionRef: "§ 1", SectionTitle: "Höhe", Text: "13,90"}
	c := IndexChunkFromExport(src, "Fünfte Mindestlohnanpassungsverordnung", "verordnung", "milog", "§ 1")
	if c.ParentLawID != "milog" || c.ParentSectionHint != "§ 1" {
		t.Fatalf("%+v", c)
	}
	raw, _ := json.Marshal(c)
	if !strings.Contains(string(raw), `"parent_law_id":"milog"`) {
		t.Fatalf("missing parent_law_id: %s", raw)
	}
}

func TestFilterIndexChunks(t *testing.T) {
	chunks := []IndexChunk{
		{ChunkID: "p1", SectionRef: "§ 1", Text: "parent1"},
		{ChunkID: "p2", SectionRef: "§ 2", Text: "parent2"},
		{ChunkID: "v1", SectionRef: "§ 3", ParentLawID: "milog", ParentSectionHint: "§ 1", Text: "vo"},
		{ChunkID: "v2", SectionRef: "§ 4", ParentLawID: "milog", ParentSectionHint: "§ 2", Text: "vo2"},
	}
	all := FilterIndexChunks(chunks, nil)
	if len(all) != 4 {
		t.Fatalf("empty filter want 4, got %d", len(all))
	}
	got := FilterIndexChunks(chunks, ParseSectionRefs("§ 1"))
	if len(got) != 2 {
		t.Fatalf("§1 want 2, got %+v", got)
	}
	ids := map[string]bool{}
	for _, c := range got {
		ids[c.ChunkID] = true
	}
	if !ids["p1"] || !ids["v1"] {
		t.Fatalf("want p1+v1 via hint, got %+v", got)
	}
	got = FilterIndexChunks(chunks, ParseSectionRefs("§1,§ 2"))
	if len(got) != 4 {
		t.Fatalf("multi want 4, got %d", len(got))
	}
	got = FilterIndexChunks(chunks, ParseSectionRefs("§ 99"))
	if len(got) != 0 {
		t.Fatalf("no match want 0, got %+v", got)
	}
}

func TestDedupeChunkIDs(t *testing.T) {
	chunks := []IndexChunk{
		{ChunkID: "milog:s1/1", Text: "first", LawID: "milog"},
		{ChunkID: "milog:s1/1", Text: "second", LawID: "milov5"},
		{ChunkID: "milog:s1/1", Text: "third", LawID: "milov4"},
		{ChunkID: "milog:s2/1", Text: "unique", LawID: "milog"},
		{ChunkID: "milog:s2/1", Text: "dup-s2", LawID: "milov5"},
	}
	got := DedupeChunkIDs(chunks)
	if len(got) != len(chunks) {
		t.Fatalf("len=%d want %d", len(got), len(chunks))
	}
	seen := make(map[string]struct{}, len(got))
	for _, c := range got {
		if _, dup := seen[c.ChunkID]; dup {
			t.Fatalf("duplicate chunk_id %q after dedupe", c.ChunkID)
		}
		seen[c.ChunkID] = struct{}{}
	}
	if got[0].ChunkID != "milog:s1/1" || got[0].Text != "first" {
		t.Fatalf("first occurrence unchanged: %+v", got[0])
	}
	if got[1].ChunkID != "milog:s1/1-2" || got[1].Text != "second" {
		t.Fatalf("second disambiguation: %+v", got[1])
	}
	if got[2].ChunkID != "milog:s1/1-3" || got[2].Text != "third" {
		t.Fatalf("third disambiguation: %+v", got[2])
	}
	if got[3].ChunkID != "milog:s2/1" {
		t.Fatalf("unique id unchanged: %+v", got[3])
	}
	if got[4].ChunkID != "milog:s2/1-2" {
		t.Fatalf("second s2 disambiguation: %+v", got[4])
	}
	// Deterministic: same input yields same output.
	got2 := DedupeChunkIDs(chunks)
	for i := range got {
		if got[i].ChunkID != got2[i].ChunkID {
			t.Fatalf("index %d: %q vs %q", i, got[i].ChunkID, got2[i].ChunkID)
		}
	}
	// Immutable: input slice unchanged.
	if chunks[1].ChunkID != "milog:s1/1" {
		t.Fatalf("input mutated: %+v", chunks[1])
	}
}

func TestDedupeChunkIDs_emptyAndSingle(t *testing.T) {
	if got := DedupeChunkIDs(nil); got != nil {
		t.Fatalf("nil want nil, got %#v", got)
	}
	if got := DedupeChunkIDs([]IndexChunk{}); len(got) != 0 {
		t.Fatalf("empty want empty, got %#v", got)
	}
	single := []IndexChunk{{ChunkID: "a", Text: "only"}}
	got := DedupeChunkIDs(single)
	if len(got) != 1 || got[0].ChunkID != "a" {
		t.Fatalf("single unchanged: %+v", got)
	}
}

func TestDedupeChunkIDs_suffixCollision(t *testing.T) {
	// ["a","a-2","a"] must not emit two "a-2" values.
	chunks := []IndexChunk{
		{ChunkID: "a", Text: "1"},
		{ChunkID: "a-2", Text: "2"},
		{ChunkID: "a", Text: "3"},
	}
	got := DedupeChunkIDs(chunks)
	seen := map[string]struct{}{}
	for _, c := range got {
		if _, dup := seen[c.ChunkID]; dup {
			t.Fatalf("duplicate chunk_id %q in %+v", c.ChunkID, got)
		}
		seen[c.ChunkID] = struct{}{}
	}
	if got[0].ChunkID != "a" || got[1].ChunkID != "a-2" || got[2].ChunkID != "a-3" {
		t.Fatalf("got ids %q %q %q", got[0].ChunkID, got[1].ChunkID, got[2].ChunkID)
	}
}

func TestIsIndexableExportChunk_excludesFormulary(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{"§ 1", true},
		{"Eingangsformel", false},
		{"eingangsformel", false},
		{"Schlussformel", false},
		{"Schlußformel", false},
		{"Unterschrift", false},
		{"Ausfertigungsvermerk", false},
		{"", true},
	}
	for _, tc := range cases {
		got := IsIndexableExportChunk(Chunk{SectionRef: tc.ref})
		if got != tc.want {
			t.Fatalf("ref=%q got %v want %v", tc.ref, got, tc.want)
		}
	}
}
