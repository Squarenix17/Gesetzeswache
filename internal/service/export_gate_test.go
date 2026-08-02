package service

import "testing"

func TestParseExportFreshnessOpts(t *testing.T) {
	ingest := ParseExportFreshnessOpts(nil, nil, "INGEST")
	if !ingest.AllowStale || ingest.ParentOnly {
		t.Fatalf("profile ingest: %+v", ingest)
	}
	withStale := ParseExportFreshnessOpts([]string{"true"}, nil, "")
	if !withStale.AllowStale || withStale.ParentOnly {
		t.Fatalf("allow_stale: %+v", withStale)
	}
	withParent := ParseExportFreshnessOpts(nil, []string{"1"}, "")
	if withParent.AllowStale || !withParent.ParentOnly {
		t.Fatalf("parent_only: %+v", withParent)
	}
}

func TestOperativeBundleTooLargeError_message(t *testing.T) {
	err := &OperativeBundleTooLargeError{Max: 8, Actual: 9}
	if err.Error() != "operative bundle too large (max 8)" {
		t.Fatalf("msg=%q", err.Error())
	}
}
