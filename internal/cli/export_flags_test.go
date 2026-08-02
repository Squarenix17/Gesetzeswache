package cli

import (
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/service"
)

func TestPeelBundleFlags_exportFreshness(t *testing.T) {
	opts, rest := peelBundleFlags([]string{"--allow-stale", "--parent-only", "--profile=ingest", "MiLoG"})
	if len(rest) != 1 || rest[0] != "MiLoG" {
		t.Fatalf("rest=%v", rest)
	}
	if !opts.AllowStale || !opts.ParentOnly {
		t.Fatalf("opts=%+v want AllowStale and ParentOnly", opts)
	}

	opts2, _ := peelBundleFlags([]string{"--profile=INGEST", "x"})
	if !opts2.AllowStale {
		t.Fatalf("profile=ingest should set AllowStale: %+v", opts2)
	}
	if opts2.ParentOnly {
		t.Fatal("profile=ingest must not set ParentOnly")
	}
}

func TestPeelIndexFlags_exportFreshness(t *testing.T) {
	opts, rest := peelIndexFlags([]string{"--allow-stale=true", "--section=§1", "ArbZG"})
	if len(rest) != 1 || rest[0] != "ArbZG" {
		t.Fatalf("rest=%v", rest)
	}
	if !opts.AllowStale {
		t.Fatalf("opts=%+v", opts)
	}
	want := service.IndexOpts{AllowStale: true, Sections: []string{"§1"}}
	if opts.AllowStale != want.AllowStale || len(opts.Sections) != 1 || opts.Sections[0] != want.Sections[0] {
		t.Fatalf("opts=%+v want %+v", opts, want)
	}
}

func TestPeelIncludeFlags_allowStale(t *testing.T) {
	gate, rest := peelExportGateFlags([]string{"--profile=ingest", "BGB"})
	if !gate.AllowStale {
		t.Fatalf("gate=%+v", gate)
	}
	if len(rest) != 1 || rest[0] != "BGB" {
		t.Fatalf("rest=%v", rest)
	}
}
