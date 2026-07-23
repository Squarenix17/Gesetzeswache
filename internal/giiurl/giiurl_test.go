package giiurl

import "testing"

func TestValidSlug(t *testing.T) {
	if !ValidSlug("bgb") || ValidSlug("../etc") || ValidSlug("a/b") {
		t.Fatal("slug validation")
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
