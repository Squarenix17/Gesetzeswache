package sync

import (
	"encoding/xml"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/giiurl"
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
