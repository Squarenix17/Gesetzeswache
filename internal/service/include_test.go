package service

import "testing"

func TestParseInclude(t *testing.T) {
	o := ParseInclude("past,linked")
	if !o.Past || !o.Linked {
		t.Fatalf("%+v", o)
	}
	o2 := ParseInclude("")
	if o2.Past || o2.Linked {
		t.Fatalf("%+v", o2)
	}
	o3 := MergeInclude([]string{"past", "linked"})
	if !o3.Past || !o3.Linked {
		t.Fatalf("%+v", o3)
	}
}
