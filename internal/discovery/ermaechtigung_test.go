package discovery

import (
	"strings"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

func TestParseErmaechtigung_PBAV(t *testing.T) {
	text := "Auf Grund des § 55 Absatz 1 Satz 2 in Verbindung mit Absatz 1a des Elften Buches Sozialgesetzbuch – Soziale Pflegeversicherung –"

	got := ParseErmaechtigung(text)
	if len(got) != 1 {
		t.Fatalf("len(got)=%d want 1; got=%+v", len(got), got)
	}

	e := got[0]
	if e.Section != "55" {
		t.Fatalf("Section=%q want 55", e.Section)
	}
	if e.Absatz != "1" {
		t.Fatalf("Absatz=%q want 1", e.Absatz)
	}
	if e.Satz != "2" {
		t.Fatalf("Satz=%q want 2", e.Satz)
	}
	if e.LawTitlePhrase == "" {
		t.Fatal("LawTitlePhrase should not be empty")
	}
	if !containsFold(e.LawTitlePhrase, "Elften Buches Sozialgesetzbuch") {
		t.Fatalf("LawTitlePhrase=%q want Elften Buches Sozialgesetzbuch", e.LawTitlePhrase)
	}
	if e.Raw == "" {
		t.Fatal("Raw should not be empty")
	}
}

func TestParseErmaechtigung_PBAV_withAmendmentClause(t *testing.T) {
	text := "Auf Grund des § 55 Absatz 1 Satz 2 in Verbindung mit Absatz 1a des Elften Buches Sozialgesetzbuch – Soziale Pflegeversicherung –, der durch Artikel 1 Nummer 20 des Gesetzes vom 19. Juni 2023 (BGBl. 2023 I Nr. 155) eingefügt worden ist, verordnet die Bundesregierung:"

	got := ParseErmaechtigung(text)
	if len(got) != 1 {
		t.Fatalf("len(got)=%d want 1; got=%+v", len(got), got)
	}
	if !containsFold(got[0].LawTitlePhrase, "Elften Buches Sozialgesetzbuch") {
		t.Fatalf("LawTitlePhrase=%q want Elften Buches Sozialgesetzbuch", got[0].LawTitlePhrase)
	}
}

func TestParseErmaechtigung_MiLoG(t *testing.T) {
	text := "Auf Grund des § 11 Absatz 1 des Mindestlohngesetzes"

	got := ParseErmaechtigung(text)
	if len(got) != 1 {
		t.Fatalf("len(got)=%d want 1; got=%+v", len(got), got)
	}

	e := got[0]
	if e.Section != "11" {
		t.Fatalf("Section=%q want 11", e.Section)
	}
	if e.Absatz != "1" {
		t.Fatalf("Absatz=%q want 1", e.Absatz)
	}
	if e.Satz != "" {
		t.Fatalf("Satz=%q want empty", e.Satz)
	}
	if !containsFold(e.LawTitlePhrase, "Mindestlohngesetz") {
		t.Fatalf("LawTitlePhrase=%q want Mindestlohngesetz", e.LawTitlePhrase)
	}
}

func TestParseErmaechtigung_AsphAusbV_NotMiLoG(t *testing.T) {
	text := "Auf Grund des § 25 des Berufsbildungsgesetzes vom 14. August 1969 (BGBl. I S. 1112), der zuletzt durch § 24 Nr. 1 des Gesetzes vom 24. August 1976 (BGBl. I S. 2525) geändert worden ist, wird im Einvernehmen mit dem Bundesminister für Bildung und Wissenschaft verordnet:"

	got := ParseErmaechtigung(text)
	if len(got) == 0 {
		t.Fatal("expected section refs")
	}
	if !containsFold(got[0].LawTitlePhrase, "Berufsbildungsgesetz") {
		t.Fatalf("LawTitlePhrase=%q want Berufsbildungsgesetz", got[0].LawTitlePhrase)
	}
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "milog", Abbreviation: "MILOG", Title: "Mindestlohngesetz"},
			{ID: "bbig", Abbreviation: "BBiG", Title: "Berufsbildungsgesetz"},
		},
	}
	lawID, unique := ResolveParent(got[0], lookup)
	if lawID == "milog" {
		t.Fatalf("must not resolve asphalt training V to milog; got %q unique=%v phrase=%q", lawID, unique, got[0].LawTitlePhrase)
	}
	if !unique || lawID != "bbig" {
		t.Fatalf("ResolveParent=%q unique=%v want bbig", lawID, unique)
	}
}

func TestParseErmaechtigung_MiLoV5_aufgrund(t *testing.T) {
	text := "Die Bundesregierung verordnet aufgrund des § 11 des Mindestlohngesetzes vom 11. August 2014 (BGBl. I S. 1348), das zuletzt durch Artikel 2 des Gesetzes vom 28. Juni 2023 (BGBl. 2023 I Nr. 172) geändert worden ist:"

	got := ParseErmaechtigung(text)
	if len(got) != 1 {
		t.Fatalf("len(got)=%d want 1; got=%+v", len(got), got)
	}
	e := got[0]
	if e.Section != "11" {
		t.Fatalf("Section=%q want 11", e.Section)
	}
	if !containsFold(e.LawTitlePhrase, "Mindestlohngesetz") {
		t.Fatalf("LawTitlePhrase=%q want Mindestlohngesetz", e.LawTitlePhrase)
	}
	lookup := CatalogLookup{
		Laws: []domain.Law{{ID: "milog", Abbreviation: "MILOG", Title: "Mindestlohngesetz"}},
	}
	lawID, unique := ResolveParent(e, lookup)
	if !unique || lawID != "milog" {
		t.Fatalf("ResolveParent=%q unique=%v want milog", lawID, unique)
	}
}

func TestExtractPreambleText_aufgrund(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0"?><dokumente><norm><textdaten><text><Content><P>Die Bundesregierung verordnet aufgrund des § 11 des Mindestlohngesetzes:</P></Content></text></textdaten></norm></dokumente>`)
	pre := ExtractPreambleText(xmlData)
	if !strings.Contains(strings.ToLower(pre), "aufgrund") {
		t.Fatalf("preamble=%q want aufgrund marker", pre)
	}
	if !strings.Contains(pre, "§ 11") {
		t.Fatalf("preamble=%q want § 11", pre)
	}
}

func TestParseErmaechtigung_MultiSectionSameParent(t *testing.T) {
	text := "Auf Grund des § 28a und § 34 Absatz 3a des Elften Buches Sozialgesetzbuch – Soziale Pflegeversicherung –"

	got := ParseErmaechtigung(text)
	if len(got) != 2 {
		t.Fatalf("len(got)=%d want 2; got=%+v", len(got), got)
	}

	want := []struct {
		section string
		absatz  string
	}{
		{section: "28a", absatz: ""},
		{section: "34", absatz: "3a"},
	}
	for i, w := range want {
		if got[i].Section != w.section {
			t.Fatalf("got[%d].Section=%q want %q", i, got[i].Section, w.section)
		}
		if got[i].Absatz != w.absatz {
			t.Fatalf("got[%d].Absatz=%q want %q", i, got[i].Absatz, w.absatz)
		}
		if got[i].LawTitlePhrase != got[0].LawTitlePhrase {
			t.Fatalf("got[%d].LawTitlePhrase=%q want same parent %q", i, got[i].LawTitlePhrase, got[0].LawTitlePhrase)
		}
	}
}

func TestResolveParent_SGBXI(t *testing.T) {
	lookup := stubLookup{
		byJurabk: map[string][]string{
			normalize.Key("SGB XI"): {"sgb11"},
		},
		byTitle: map[string][]string{
			normalize.Key("Elften Buches Sozialgesetzbuch"): {"sgb11"},
		},
	}

	tests := []struct {
		name string
		e    Ermaechtigung
	}{
		{
			name: "jurabk",
			e: Ermaechtigung{
				Jurabk:         "SGB XI",
				LawTitlePhrase: "Elften Buches Sozialgesetzbuch",
			},
		},
		{
			name: "title phrase only",
			e: Ermaechtigung{
				LawTitlePhrase: "Elften Buches Sozialgesetzbuch",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lawID, unique := ResolveParent(tt.e, lookup)
			if !unique {
				t.Fatal("expected unique parent")
			}
			if lawID != "sgb11" {
				t.Fatalf("lawID=%q want sgb11", lawID)
			}
		})
	}
}

func TestResolveParent_MultiParentNotUnique(t *testing.T) {
	lookup := stubLookup{
		byTitle: map[string][]string{
			normalize.Key("Sozialgesetzbuch"): {"sgb5", "sgb11"},
		},
	}

	e := Ermaechtigung{LawTitlePhrase: "Sozialgesetzbuch"}
	lawID, unique := ResolveParent(e, lookup)
	if unique {
		t.Fatal("expected non-unique parent")
	}
	if lawID != "" {
		t.Fatalf("lawID=%q want empty when ambiguous", lawID)
	}
}

func TestResolveParent_CatalogLookup_PBAVLiveAmbiguity(t *testing.T) {
	// Live GII titles all contain "Sozialgesetzbuch"; fuzzy title matching must not
	// drown the built-in Elftes-Buch → SGB XI signal.
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb2", Abbreviation: "SGB_2", Title: "Sozialgesetzbuch (SGB) Zweites Buch (II)"},
			{ID: "sgb5", Abbreviation: "SGB_5", Title: "Sozialgesetzbuch (SGB) Fünftes Buch (V)"},
			{ID: "sgb11", Abbreviation: "SGB_11", Title: "Sozialgesetzbuch (SGB) - Elftes Buch (XI) - Soziale Pflegeversicherung"},
		},
		Variants: []domain.LawVariant{
			{Variant: "SGB XI", LawID: "sgb11"},
			{Variant: "SGB 11", LawID: "sgb11"},
		},
	}
	e := Ermaechtigung{
		Jurabk:         "SGB XI",
		LawTitlePhrase: "Elften Buches Sozialgesetzbuch",
		Section:        "55",
	}
	lawID, unique := ResolveParent(e, lookup)
	if !unique || lawID != "sgb11" {
		t.Fatalf("ResolveParent=%q unique=%v want sgb11 unique", lawID, unique)
	}
}

func TestCatalogLookup_ByJurabk_SGBRomanAndArabic(t *testing.T) {
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb11", Abbreviation: "SGB_11", Title: "Elftes Buch"},
		},
	}
	got := lookup.ByJurabk("SGB XI")
	if len(got) != 1 || got[0] != "sgb11" {
		t.Fatalf("ByJurabk(SGB XI)=%v want [sgb11]", got)
	}
}

func TestScoreConfidence_HighRequiresAll(t *testing.T) {
	tests := []struct {
		name            string
		parentUnique    bool
		sectionHint     string
		fundstelleOK    bool
		editorialAgrees bool
		want            string
	}{
		{
			name:            "all required signals",
			parentUnique:    true,
			sectionHint:     "§ 55",
			fundstelleOK:    true,
			editorialAgrees: true,
			want:            "high",
		},
		{
			name:            "missing section hint",
			parentUnique:    true,
			sectionHint:     "",
			fundstelleOK:    true,
			editorialAgrees: true,
			want:            "medium",
		},
		{
			name:            "missing fundstelle",
			parentUnique:    true,
			sectionHint:     "§ 55",
			fundstelleOK:    false,
			editorialAgrees: true,
			want:            "medium",
		},
		{
			name:            "not unique parent",
			parentUnique:    false,
			sectionHint:     "§ 55",
			fundstelleOK:    true,
			editorialAgrees: true,
			want:            "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScoreConfidence(tt.parentUnique, tt.sectionHint, tt.fundstelleOK, tt.editorialAgrees)
			if got != tt.want {
				t.Fatalf("ScoreConfidence()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestScoreConfidence_MultiParentMedium(t *testing.T) {
	tests := []struct {
		name            string
		sectionHint     string
		fundstelleOK    bool
		editorialAgrees bool
		want            string
	}{
		{
			name:            "all signals but ambiguous parent",
			sectionHint:     "§ 55",
			fundstelleOK:    true,
			editorialAgrees: true,
			want:            "medium",
		},
		{
			name:            "weak signals ambiguous parent",
			sectionHint:     "",
			fundstelleOK:    false,
			editorialAgrees: false,
			want:            "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScoreConfidence(false, tt.sectionHint, tt.fundstelleOK, tt.editorialAgrees)
			if got != tt.want {
				t.Fatalf("ScoreConfidence()=%q want %q", got, tt.want)
			}
			if got == "high" {
				t.Fatal("multi-parent must never score high")
			}
		})
	}
}

type stubLookup struct {
	byJurabk map[string][]string
	byTitle  map[string][]string
}

func (s stubLookup) ByJurabk(jurabk string) []string {
	if s.byJurabk == nil {
		return nil
	}
	return append([]string(nil), s.byJurabk[normalize.Key(jurabk)]...)
}

func (s stubLookup) ByTitlePhrase(phrase string) []string {
	if s.byTitle == nil {
		return nil
	}
	return append([]string(nil), s.byTitle[normalize.Key(phrase)]...)
}

func containsFold(haystack, needle string) bool {
	return normalize.Key(haystack) == normalize.Key(needle) ||
		len(normalize.Key(haystack)) >= len(normalize.Key(needle)) &&
			containsKey(haystack, needle)
}

func containsKey(haystack, needle string) bool {
	h := normalize.Key(haystack)
	n := normalize.Key(needle)
	if len(n) == 0 {
		return true
	}
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
