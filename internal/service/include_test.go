package service

import "testing"

func TestParseInclude(t *testing.T) {
	o := ParseInclude("past,linked")
	if !o.Past || !o.Linked {
		t.Fatalf("%+v", o)
	}
	o2 := ParseInclude("")
	if o2.Past || o2.Linked || o2.Proof {
		t.Fatalf("%+v", o2)
	}
	o3 := MergeInclude([]string{"past", "linked"})
	if !o3.Past || !o3.Linked {
		t.Fatalf("%+v", o3)
	}
	o4 := ParseInclude("proof")
	if !o4.Proof || o4.Past || o4.Linked {
		t.Fatalf("%+v", o4)
	}
	o5 := ParseInclude("past,linked,proof")
	if !o5.Past || !o5.Linked || !o5.Proof {
		t.Fatalf("%+v", o5)
	}
	o6 := MergeInclude([]string{"linked", "proof"})
	if !o6.Linked || !o6.Proof || o6.Past {
		t.Fatalf("%+v", o6)
	}
}
