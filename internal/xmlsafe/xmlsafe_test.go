package xmlsafe

import "testing"

func TestRejectUnsafeXML_EntityBlocked(t *testing.T) {
	cases := []string{
		`<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe "xxe">]><dokumente></dokumente>`,
		`<?xml version="1.0"?><! ENTITY x "y"><dokumente></dokumente>`,
		`<?xml version="1.0"?><!  ENTITY x "y"><dokumente></dokumente>`,
		"<?xml version=\"1.0\"?><!\tENTITY x \"y\"><dokumente></dokumente>",
		`<?xml version="1.0"?><!EnTiTy x "y"><dokumente></dokumente>`,
	}
	for _, xmlData := range cases {
		if err := RejectUnsafeXML([]byte(xmlData)); err == nil {
			t.Fatalf("expected error for %q", xmlData)
		}
	}
}

func TestRejectUnsafeXML_DoctypeAllowed(t *testing.T) {
	safe := []byte(`<?xml version="1.0"?><!DOCTYPE dokumente SYSTEM "http://www.gesetze-im-internet.de/dtd/1.01/gii-norm.dtd"><dokumente></dokumente>`)
	if err := RejectUnsafeXML(safe); err != nil {
		t.Fatalf("SYSTEM doctype should be allowed: %v", err)
	}
	spaced := []byte(`<?xml version="1.0"?><! DOCTYPE dokumente SYSTEM "http://example.com/x.dtd"><dokumente></dokumente>`)
	if err := RejectUnsafeXML(spaced); err != nil {
		t.Fatalf("<! DOCTYPE should be allowed: %v", err)
	}
}
