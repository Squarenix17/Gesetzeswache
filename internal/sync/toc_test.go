package sync

import (
	"encoding/xml"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
	"github.com/Squarenix17/gesetzeswache/internal/normalize"
)

func TestTOCUnmarshal(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><items>
<item><title>Bürgerliches Gesetzbuch</title><link>http://www.gesetze-im-internet.de/bgb/xml.zip</link></item>
</items>`)
	var root tocRoot
	if err := xml.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Items) != 1 {
		t.Fatalf("items=%d", len(root.Items))
	}
	if giiurl.SlugFromTOCLink(root.Items[0].Link) != "bgb" {
		t.Fatalf("slug=%s", giiurl.SlugFromTOCLink(root.Items[0].Link))
	}
}

func TestGuessAbbr_underscoreSlugs(t *testing.T) {
	tests := []struct {
		title, slug, want string
	}{
		{title: "Gesetz zur Änderung des Arbeitnehmerüberlassungsgesetzes", slug: "_arbvtrg", want: "ARBVTRG"},
		{title: "Aktiengesetz", slug: "_ag", want: "AG"},
		{title: "Approximationsverordnung", slug: "_appro_2002", want: "APPRO_2002"},
		{title: "Bürgerliches Gesetzbuch", slug: "bgb", want: "BGB"},
		{title: "Long slug without jurabk", slug: "_verylongslugname", want: "verylongslugname"},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			got := guessAbbr(tt.title, tt.slug)
			if got != tt.want {
				t.Fatalf("guessAbbr(%q, %q)=%q want %q", tt.title, tt.slug, got, tt.want)
			}
			// Law ID from abbr must stay equal to Key(slug) so underscore GII resolve is unchanged.
			if normalize.Key(got) != normalize.Key(tt.slug) {
				t.Fatalf("normalize.Key(abbr)=%q Key(slug)=%q", normalize.Key(got), normalize.Key(tt.slug))
			}
		})
	}
}
