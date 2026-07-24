package giiurl

import (
	"strings"
	"testing"
)

func TestValidSlug(t *testing.T) {
	if !ValidSlug("bgb") || ValidSlug("../etc") || ValidSlug("a/b") {
		t.Fatal("slug validation")
	}
	if ValidSlug(strings.Repeat("a", MaxSlugLen+1)) {
		t.Fatal("expected overlong slug to be rejected")
	}
	if !ValidSlug(strings.Repeat("a", MaxSlugLen)) {
		t.Fatal("expected max-length slug to be accepted")
	}
}

func TestSlugFromTOC(t *testing.T) {
	if SlugFromTOCLink("http://www.gesetze-im-internet.de/bgb/xml.zip") != "bgb" {
		t.Fatal(SlugFromTOCLink("http://www.gesetze-im-internet.de/bgb/xml.zip"))
	}
}

func TestXMLZip(t *testing.T) {
	u, err := XMLZip("https://www.gesetze-im-internet.de", "bgb")
	if err != nil || u != "https://www.gesetze-im-internet.de/bgb/xml.zip" {
		t.Fatalf("%s %v", u, err)
	}
}
