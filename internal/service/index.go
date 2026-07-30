package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/export"
)

// IndexOpts controls index export membership and optional section filter.
type IndexOpts struct {
	Past     bool
	Sections []string // empty = all sections
}

// IndexResult is a flat ingest-ready chunk list plus bundle freshness for gating.
type IndexResult struct {
	Matched         bool                 `json:"matched"`
	Query           string               `json:"query,omitempty"`
	Chunks          []export.IndexChunk  `json:"chunks,omitempty"`
	BundleFreshness *BundleFreshnessMeta `json:"bundle_freshness,omitempty"`
	Suggestions     []Suggestion         `json:"suggestions,omitempty"`
}

// ExportIndexChunks resolves a parent law, exports parent + current (optionally past)
// linked instruments as flat IndexChunks, then optionally filters by section.
func (s *Service) ExportIndexChunks(ctx context.Context, query string, opts IndexOpts) (IndexResult, error) {
	bundle, err := s.ExportOperativeBundle(ctx, query, []string{export.FormatChunked}, BundleOpts{Past: opts.Past})
	if err != nil {
		return IndexResult{}, err
	}
	if !bundle.Matched {
		return IndexResult{
			Matched:     false,
			Query:       query,
			Suggestions: bundle.Suggestions,
		}, nil
	}

	chunks := make([]export.IndexChunk, 0, 64)
	if bundle.Parent != nil {
		lawName := ""
		if bundle.Parent.Law != nil {
			lawName = bundle.Parent.Law.Title
		}
		parentChunks, err := chunkedFromFormats(bundle.Parent.Formats)
		if err != nil {
			return IndexResult{}, err
		}
		for _, c := range parentChunks {
			if !export.IsIndexableExportChunk(c) {
				continue
			}
			chunks = append(chunks, export.IndexChunkFromExport(c, lawName, "gesetz", "", ""))
		}
	}

	parentID := ""
	if bundle.Parent != nil && bundle.Parent.Law != nil {
		parentID = bundle.Parent.Law.ID
	}
	for _, op := range bundle.Operative {
		lawName := ""
		if op.Law != nil {
			lawName = op.Law.Title
		}
		kind := strings.TrimSpace(op.Link.Kind)
		if kind == "" {
			kind = "verordnung"
		}
		opChunks, err := chunkedFromFormats(op.Formats)
		if err != nil {
			return IndexResult{}, err
		}
		hint := op.Link.SectionHint
		for _, c := range opChunks {
			if !export.IsIndexableExportChunk(c) {
				continue
			}
			chunks = append(chunks, export.IndexChunkFromExport(c, lawName, kind, parentID, hint))
		}
	}

	chunks = export.FilterIndexChunks(chunks, opts.Sections)
	return IndexResult{
		Matched:         true,
		Query:           query,
		Chunks:          chunks,
		BundleFreshness: bundle.BundleFreshness,
	}, nil
}

func chunkedFromFormats(formats map[string]any) ([]export.Chunk, error) {
	if formats == nil {
		return nil, nil
	}
	v, ok := formats[export.FormatChunked]
	if !ok || v == nil {
		return nil, nil
	}
	chunks, ok := v.([]export.Chunk)
	if !ok {
		return nil, fmt.Errorf("index: unexpected chunked type %T", v)
	}
	return chunks, nil
}
