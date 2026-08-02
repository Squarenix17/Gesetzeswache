package freshness

import (
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

// UnresolvedInstrumentRef classifies an instrument citation that blocks or informs freshness.
type UnresolvedInstrumentRef struct {
	Ref            domain.InstrumentRef `json:"ref"`
	Classification string               `json:"classification"`
}

// CollectUnresolvedRefs derives operative and ops-relevant instrument ref classifications.
func CollectUnresolvedRefs(in Input) []UnresolvedInstrumentRef {
	if len(in.InstrumentRefs) == 0 {
		return nil
	}
	var out []UnresolvedInstrumentRef
	for _, ref := range in.InstrumentRefs {
		if ref.Year == 0 || (ref.Number == "" && ref.Teil == 0) {
			continue
		}
		if standReflectsRef(in.Stand, ref) {
			continue
		}
		kind := strings.ToUpper(strings.TrimSpace(ref.Kind))
		if kind == "BEK" && strings.TrimSpace(ref.SectionHint) == "" {
			out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "ignored_bare_bek"})
			continue
		}
		if kind == "G" {
			continue
		}
		if kind == "" && !in.HasSeededLinkedInstruments {
			out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "missing_seed"})
			continue
		}
		if !isOperativeInstrumentRef(ref, in.HasSeededLinkedInstruments) {
			continue
		}
		if in.VRefResolutions == nil {
			out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "unmatched"})
			continue
		}
		res, ok := findVRefResolution(in.VRefResolutions, ref)
		if !ok {
			if kind == "" && !in.HasSeededLinkedInstruments {
				out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "missing_seed"})
			} else {
				out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "unmatched"})
			}
			continue
		}
		if res.Historical {
			out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "historical"})
			continue
		}
		if res.Resolved && res.ChildConfirmed {
			continue
		}
		if res.MatchedGIISlug != "" {
			out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "child_not_current"})
			continue
		}
		if kind == "" && !in.HasSeededLinkedInstruments {
			out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "missing_seed"})
		} else {
			out = append(out, UnresolvedInstrumentRef{Ref: ref, Classification: "unmatched"})
		}
	}
	return out
}
