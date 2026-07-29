package export

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/xmlsafe"
)

var (
	reFundstelleNr   = regexp.MustCompile(`(?i)(\d{4})\s*,?\s*(?:Nr\.?\s*)?([0-9]+[a-zA-Z]?)`)
	reFundstellePage = regexp.MustCompile(`(?i)(\d{4})\s+S\.?\s*([0-9]+)`)
)

// ExtractStandRaw finds the first Stand-type standangabe/standkommentar in GII norm XML.
// When standangabe is absent (common for Verordnungen), falls back to the first
// fundstelle (periodikum + zit/zitstelle) formatted as a parseable BGBl citation.
// Returns empty string when absent. Does not invent citations.
func ExtractStandRaw(xmlData []byte) string {
	if err := xmlsafe.RejectUnsafeXML(xmlData); err != nil {
		return ""
	}
	if raw := extractStandangabe(xmlData); raw != "" {
		return raw
	}
	return extractFundstelleAsStand(xmlData)
}

func extractStandangabe(xmlData []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var inStandangabe, inStandtyp, inStandkommentar bool
	var standtyp, kommentar string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "standangabe":
				inStandangabe = true
				standtyp, kommentar = "", ""
			case "standtyp":
				if inStandangabe {
					inStandtyp = true
				}
			case "standkommentar":
				if inStandangabe {
					inStandkommentar = true
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "standtyp":
				inStandtyp = false
			case "standkommentar":
				inStandkommentar = false
			case "standangabe":
				inStandangabe = false
				if strings.EqualFold(strings.TrimSpace(standtyp), "Stand") {
					raw := strings.TrimSpace(kommentar)
					if raw != "" {
						return raw
					}
				}
			}
		case xml.CharData:
			if inStandtyp {
				standtyp += string(t)
			}
			if inStandkommentar {
				kommentar += string(t)
			}
		}
	}
	return ""
}

func extractFundstelleAsStand(xmlData []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var inFundstelle, inPeriodikum, inZit bool
	var periodikum, zit string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "fundstelle":
				inFundstelle = true
				periodikum, zit = "", ""
			case "periodikum":
				if inFundstelle {
					inPeriodikum = true
				}
			case "zit", "zitstelle":
				if inFundstelle {
					inZit = true
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "periodikum":
				inPeriodikum = false
			case "zit", "zitstelle":
				inZit = false
			case "fundstelle":
				inFundstelle = false
				if raw := formatFundstelleStand(periodikum, zit); raw != "" {
					return raw
				}
			}
		case xml.CharData:
			if inPeriodikum {
				periodikum += string(t)
			}
			if inZit {
				zit += string(t)
			}
		}
	}
	return ""
}

// formatFundstelleStand builds a citation.Parse-friendly string from GII fundstelle fields.
// Examples: periodikum "BGBl. I" + zit "2025 Nr. 268" → "BGBl. 2025 I Nr. 268"
func formatFundstelleStand(periodikum, zit string) string {
	periodikum = strings.TrimSpace(periodikum)
	zit = strings.TrimSpace(zit)
	if periodikum == "" || zit == "" {
		return ""
	}
	teil := fundstelleTeil(periodikum)
	if teil == "" {
		return ""
	}
	if m := reFundstellePage.FindStringSubmatch(zit); len(m) == 3 {
		return "BGBl. " + m[1] + " " + teil + " S. " + m[2]
	}
	if m := reFundstelleNr.FindStringSubmatch(zit); len(m) == 3 {
		return "BGBl. " + m[1] + " " + teil + " Nr. " + m[2]
	}
	return ""
}

func fundstelleTeil(periodikum string) string {
	u := strings.ToUpper(strings.TrimSpace(periodikum))
	u = strings.ReplaceAll(u, ".", " ")
	fields := strings.Fields(u)
	for i, f := range fields {
		switch f {
		case "II", "2":
			return "II"
		case "I", "1":
			return "I"
		case "TEIL":
			if i+1 < len(fields) {
				switch fields[i+1] {
				case "II", "2":
					return "II"
				case "I", "1":
					return "I"
				}
			}
		}
	}
	return ""
}
