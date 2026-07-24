package export

import (
	"strconv"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
)

// EditorialCitationTexts returns text from preamble / +++ units in IR.
func EditorialCitationTexts(ir IR) []string {
	var out []string
	for _, u := range ir.Units {
		if u.Kind != KindPreamble && !strings.Contains(u.Text, "(+++") {
			continue
		}
		t := strings.TrimSpace(u.Text)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// InstrumentRefsFromIR parses BGBl instrument citations from editorial +++ / preamble units.
func InstrumentRefsFromIR(ir IR) []domain.InstrumentRef {
	var refs []domain.InstrumentRef
	for _, t := range EditorialCitationTexts(ir) {
		refs = append(refs, citation.ParseLinkedInstruments(t)...)
	}
	return dedupeInstrumentRefs(refs)
}

// InstrumentRefsFromXML builds a temporary IR and extracts instrument refs (for Stand refresh path).
func InstrumentRefsFromXML(law domain.Law, xmlData []byte) ([]domain.InstrumentRef, error) {
	ir, err := BuildIR(law, "instruments", xmlData)
	if err != nil {
		return nil, err
	}
	return InstrumentRefsFromIR(ir), nil
}

func dedupeInstrumentRefs(in []domain.InstrumentRef) []domain.InstrumentRef {
	seen := map[string]struct{}{}
	var out []domain.InstrumentRef
	for _, r := range in {
		key := r.Kind + "|" + strconv.Itoa(r.Teil) + "|" + strconv.Itoa(r.Year) + "|" + r.Number
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}
