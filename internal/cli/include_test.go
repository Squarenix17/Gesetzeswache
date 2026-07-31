package cli

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/service"
)

func TestPeelIncludeFlags_proof(t *testing.T) {
	opts, rest := peelIncludeFlags([]string{"--include=proof", "MiLoG"})
	if len(rest) != 1 || rest[0] != "MiLoG" {
		t.Fatalf("rest=%v", rest)
	}
	if !opts.Proof {
		t.Fatalf("opts=%+v want Proof=true", opts)
	}

	opts2, rest2 := peelIncludeFlags([]string{"--include=past,linked,proof", "arbzg"})
	if len(rest2) != 1 || rest2[0] != "arbzg" {
		t.Fatalf("rest=%v", rest2)
	}
	want := service.IncludeOpts{Past: true, Linked: true, Proof: true}
	if opts2 != want {
		t.Fatalf("opts=%+v want %+v", opts2, want)
	}
}
