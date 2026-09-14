package main

import (
	"os"
	"strings"
	"testing"
)

func TestVersionMatchesVERSIONFile(t *testing.T) {
	wantBytes, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(wantBytes))
	got := strings.TrimSpace(version)
	if got != want {
		t.Errorf("version var = %q; want %q from VERSION file", got, want)
	}
}

func TestCIBuildsMultiArchImage(t *testing.T) {
	ci, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	if !strings.Contains(string(ci), "linux/arm64") {
		t.Error("ci.yml must build linux/arm64 images (multi-arch GHCR publish)")
	}
}
