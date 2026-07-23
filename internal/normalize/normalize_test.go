package normalize

import "testing"

func TestKeyUmlaut(t *testing.T) {
	if Key("Bürgerliches Gesetzbuch") != Key("Buergerliches Gesetzbuch") {
		t.Fatalf("umlaut fold mismatch: %q vs %q", Key("Bürgerliches Gesetzbuch"), Key("Buergerliches Gesetzbuch"))
	}
	if Key("Straße") != Key("Strasse") {
		t.Fatalf("ß fold mismatch")
	}
}
