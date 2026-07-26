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

func TestParseErmaechtigung_LAG_FassungTrim(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantPhrase   string
		wantSection  string
		wantParentID string
	}{
		{
			name:         "fassung clause without date",
			text:         "Auf Grund des § 267 des Lastenausgleichsgesetzes in der Fassung der Bekanntmachung",
			wantPhrase:   "Lastenausgleichsgesetzes",
			wantSection:  "267",
			wantParentID: "lag",
		},
		{
			name:         "fassung clause with vom date",
			text:         "Auf Grund des § 267 des Lastenausgleichsgesetzes in der Fassung der Bekanntmachung vom 1. Januar 1990",
			wantPhrase:   "Lastenausgleichsgesetzes",
			wantSection:  "267",
			wantParentID: "lag",
		},
	}

	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "lag", Abbreviation: "LAG", Title: "Gesetz über den Lastenausgleich", GIIPath: "lag"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseErmaechtigung(tt.text)
			if len(got) != 1 {
				t.Fatalf("len(got)=%d want 1; got=%+v", len(got), got)
			}
			e := got[0]
			if e.Section != tt.wantSection {
				t.Fatalf("Section=%q want %q", e.Section, tt.wantSection)
			}
			if !containsFold(e.LawTitlePhrase, tt.wantPhrase) {
				t.Fatalf("LawTitlePhrase=%q want containing %q", e.LawTitlePhrase, tt.wantPhrase)
			}
			if strings.Contains(strings.ToLower(e.LawTitlePhrase), "fassung") {
				t.Fatalf("LawTitlePhrase must not contain Fassung tail: %q", e.LawTitlePhrase)
			}
			lawID, unique := ResolveParent(e, lookup)
			if !unique || lawID != tt.wantParentID {
				t.Fatalf("ResolveParent=%q unique=%v want %q unique", lawID, unique, tt.wantParentID)
			}
		})
	}
}

func TestResolveParent_Lastenausgleichsgesetz_compoundGenitive(t *testing.T) {
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "lag", Abbreviation: "LAG", Title: "Gesetz über den Lastenausgleich", GIIPath: "lag"},
		},
	}
	tests := []struct {
		phrase string
	}{
		{phrase: "Lastenausgleichsgesetzes"},
		{phrase: "Lastenausgleichsgesetz"},
		{phrase: "Gesetzes über den Lastenausgleich"},
	}
	for _, tt := range tests {
		t.Run(tt.phrase, func(t *testing.T) {
			e := Ermaechtigung{LawTitlePhrase: tt.phrase, Section: "267"}
			lawID, unique := ResolveParent(e, lookup)
			if !unique || lawID != "lag" {
				t.Fatalf("ResolveParent=%q unique=%v want lag", lawID, unique)
			}
		})
	}
}

func TestResolveParentFromChildTitle_UhAnpV_nachDemLAG(t *testing.T) {
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "lag", Abbreviation: "LAG", Title: "Gesetz über den Lastenausgleich", GIIPath: "lag"},
		},
	}
	title := "Vierundzwanzigste Verordnung zur Anpassung der Unterhaltshilfe nach dem Lastenausgleichsgesetz"
	lawID, unique := ResolveParentFromChildTitle(title, lookup)
	if !unique || lawID != "lag" {
		t.Fatalf("ResolveParentFromChildTitle=%q unique=%v want lag", lawID, unique)
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
	if len(got) != 1 {
		t.Fatalf("len(got)=%d want 1 (amendment § 24 must not leak); got=%+v", len(got), got)
	}
	if got[0].Section != "25" {
		t.Fatalf("Section=%q want 25", got[0].Section)
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

func TestParseErmaechtigung_WoGV_abbreviatedDesPara(t *testing.T) {
	text := "Die V wurde aufgrund d. § 36 Nr. 1 u. 2 G v. 14.12.1970 I 1637 von der Bundesregierung mit Zustimmung des Bundesrates erlassen"

	got := ParseErmaechtigung(text)
	if len(got) != 1 {
		t.Fatalf("len(got)=%d want 1; got=%+v", len(got), got)
	}
	if got[0].Section != "36" {
		t.Fatalf("Section=%q want 36", got[0].Section)
	}
	if got[0].LawTitlePhrase != "" || got[0].Jurabk != "" {
		t.Fatalf("abbreviated fussnoten must have empty parent hints; got phrase=%q jurabk=%q", got[0].LawTitlePhrase, got[0].Jurabk)
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

func TestParseErmaechtigung_SVBezGrV_MultiParentSegments(t *testing.T) {
	text := `Auf Grund – des § 69 Absatz 2 in Verbindung mit § 68 Absatz 2 Satz 1 und § 228b sowie des § 160 Nummer 2 in Verbindung mit § 159 und § 68 Absatz 2 Satz 1 des Sechsten Buches Sozialgesetzbuch, von denen § 69 Absatz 2 zuletzt durch Artikel 3 Nummer 2 des Gesetzes vom 24. Oktober 2024 (BGBl. 2024 I Nr. 329) geändert worden sind, – des § 6 Absatz 6 und 7 des Fünften Buches Sozialgesetzbuch, dessen Absatz 7 durch Artikel 1 Nummer 1 Buchstabe c des Gesetzes vom 23. Dezember 2002 (BGBl. I S. 4637) eingefügt worden ist, verordnet die Bundesregierung und auf Grund – des § 17 Absatz 2 Satz 1 in Verbindung mit § 18 des Vierten Buches Sozialgesetzbuch, dessen § 18 durch Artikel 3 Nummer 4 des Gesetzes vom 17. Juli 2017 (BGBl. I S. 2575) geändert worden ist, verordnet das Bundesministerium für Arbeit und Soziales:`

	got := ParseErmaechtigung(text)
	if len(got) < 3 {
		t.Fatalf("len(got)=%d want >=3; got=%+v", len(got), got)
	}

	byJurabk := map[string][]Ermaechtigung{}
	for _, e := range got {
		if e.Jurabk == "" || e.LawTitlePhrase == "" {
			t.Fatalf("empty parent signal: %+v", e)
		}
		if strings.HasPrefix(strings.TrimSpace(e.LawTitlePhrase), "§") {
			t.Fatalf("LawTitlePhrase must not start with §: %q", e.LawTitlePhrase)
		}
		byJurabk[e.Jurabk] = append(byJurabk[e.Jurabk], e)
	}
	for _, want := range []string{"SGB IV", "SGB V", "SGB VI"} {
		if len(byJurabk[want]) == 0 {
			t.Fatalf("missing segment for %s; jurabks=%v", want, keysOf(byJurabk))
		}
	}

	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb2", Abbreviation: "SGB_2", Title: "Sozialgesetzbuch (SGB) Zweites Buch (II)"},
			{ID: "sgb4", Abbreviation: "SGB_4", Title: "Sozialgesetzbuch (SGB) Viertes Buch (IV)"},
			{ID: "sgb5", Abbreviation: "SGB_5", Title: "Sozialgesetzbuch (SGB) Fünftes Buch (V)"},
			{ID: "sgb6", Abbreviation: "SGB_6", Title: "Sozialgesetzbuch (SGB) Sechstes Buch (VI)"},
			{ID: "sgb11", Abbreviation: "SGB_11", Title: "Sozialgesetzbuch (SGB) Elftes Buch (XI)"},
		},
		Variants: []domain.LawVariant{
			{Variant: "SGB IV", LawID: "sgb4"},
			{Variant: "SGB V", LawID: "sgb5"},
			{Variant: "SGB VI", LawID: "sgb6"},
		},
	}

	wantParents := map[string]bool{"sgb4": false, "sgb5": false, "sgb6": false}
	for _, e := range got {
		id, unique := ResolveParent(e, lookup)
		if !unique {
			t.Fatalf("ResolveParent not unique for %+v", e)
		}
		if _, ok := wantParents[id]; !ok {
			t.Fatalf("unexpected parent %q for %+v", id, e)
		}
		wantParents[id] = true
	}
	for id, seen := range wantParents {
		if !seen {
			t.Fatalf("missing resolved parent %s", id)
		}
	}

	// Amendment-tail §§ after ", von denen" / ", dessen" must not appear.
	for _, e := range byJurabk["SGB IV"] {
		if e.Section != "17" && e.Section != "18" {
			t.Fatalf("SGB IV unexpected section %q (amendment leak?)", e.Section)
		}
	}
	for _, e := range byJurabk["SGB V"] {
		if e.Section != "6" {
			t.Fatalf("SGB V unexpected section %q", e.Section)
		}
	}
}

func TestResolveParent_ViertenBuches_UniqueAgainstNoise(t *testing.T) {
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb2", Abbreviation: "SGB_2", Title: "Sozialgesetzbuch (SGB) Zweites Buch (II)"},
			{ID: "sgb4", Abbreviation: "SGB_4", Title: "Sozialgesetzbuch (SGB) Viertes Buch (IV)"},
			{ID: "sgb5", Abbreviation: "SGB_5", Title: "Sozialgesetzbuch (SGB) Fünftes Buch (V)"},
			{ID: "sgb11", Abbreviation: "SGB_11", Title: "Sozialgesetzbuch (SGB) Elftes Buch (XI)"},
		},
		Variants: []domain.LawVariant{
			{Variant: "SGB IV", LawID: "sgb4"},
		},
	}
	e := Ermaechtigung{
		LawTitlePhrase: "Vierten Buches Sozialgesetzbuch",
		Section:        "18",
	}
	id, unique := ResolveParent(e, lookup)
	if !unique || id != "sgb4" {
		t.Fatalf("ResolveParent=%q unique=%v want sgb4", id, unique)
	}
	if got := inferJurabkFromPhrase(e.LawTitlePhrase); got != "SGB IV" {
		t.Fatalf("inferJurabkFromPhrase=%q want SGB IV", got)
	}
}

func TestResolveParent_ZwoelftenBuches_NotSGBXI(t *testing.T) {
	lookup := CatalogLookup{
		Laws: []domain.Law{
			{ID: "sgb11", Abbreviation: "SGB_11", Title: "Sozialgesetzbuch (SGB) Elftes Buch (XI)"},
		},
		Variants: []domain.LawVariant{
			{Variant: "SGB XI", LawID: "sgb11"},
			{Variant: "SGB XII", LawID: "sgb12"},
		},
	}
	e := Ermaechtigung{
		LawTitlePhrase: "Zwölften Buches Sozialgesetzbuch",
		Section:        "1",
	}
	id, unique := ResolveParent(e, lookup)
	if id == "sgb11" {
		t.Fatal("Zwölften Buches must not resolve to sgb11 via elften substring")
	}
	// Catalog has no sgb12 law row — only variant pointing at missing id is ok;
	// unique resolve to sgb12 via variant is acceptable, or empty if variant LawID absent from laws.
	if unique && id == "sgb11" {
		t.Fatal("must not uniquely resolve XII to XI")
	}
	if got := inferJurabkFromPhrase(e.LawTitlePhrase); got != "SGB XII" {
		t.Fatalf("inferJurabkFromPhrase=%q want SGB XII", got)
	}
	// With only sgb11 in catalog and no sgb12 law, ByJurabk(SGB XII) may return sgb12 from variant.
	if unique && id != "sgb12" {
		t.Fatalf("ResolveParent=%q unique=%v; want sgb12 or non-unique", id, unique)
	}
}

func keysOf(m map[string][]Ermaechtigung) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
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

func TestScoreConfidence_AmbiguousParentMedium(t *testing.T) {
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
				t.Fatal("ambiguous parent candidates must never score high")
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
