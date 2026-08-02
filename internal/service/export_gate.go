package service

import (
	"fmt"
	"strings"
)

// ExportGateOpts controls fail-closed export refusal overrides.
type ExportGateOpts struct {
	AllowStale bool
}

// ExportFreshnessOpts are HTTP/CLI freshness override flags for bundle, index, and export.
type ExportFreshnessOpts struct {
	AllowStale bool
	ParentOnly bool
}

// ParseExportFreshnessOpts parses allow_stale, parent_only, and profile query/body fields.
// profile=ingest sets AllowStale only (case-insensitive).
func ParseExportFreshnessOpts(allowStaleVals, parentOnlyVals []string, profile string) ExportFreshnessOpts {
	var o ExportFreshnessOpts
	for _, v := range allowStaleVals {
		if parseBoolFlag(v) {
			o.AllowStale = true
		}
	}
	for _, v := range parentOnlyVals {
		if parseBoolFlag(v) {
			o.ParentOnly = true
		}
	}
	if strings.EqualFold(strings.TrimSpace(profile), "ingest") {
		o.AllowStale = true
	}
	return o
}

func parseBoolFlag(v string) bool {
	v = strings.TrimSpace(v)
	return v == "1" || strings.EqualFold(v, "true")
}

// OperativeBundleMemberRef identifies one operative bundle member in size errors.
type OperativeBundleMemberRef struct {
	LawID       string `json:"law_id,omitempty"`
	GIISlug     string `json:"gii_slug,omitempty"`
	SectionHint string `json:"section_hint,omitempty"`
}

// OperativeBundleTooLargeError is returned when linked Verordnungen exceed the configured cap.
type OperativeBundleTooLargeError struct {
	Max     int
	Actual  int
	Members []OperativeBundleMemberRef
}

func (e *OperativeBundleTooLargeError) Error() string {
	return fmt.Sprintf("operative bundle too large (max %d)", e.Max)
}
