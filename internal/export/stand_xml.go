package export

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// ExtractStandRaw finds the first Stand-type standangabe/standkommentar in GII norm XML.
// Returns empty string when absent. Does not invent citations.
func ExtractStandRaw(xmlData []byte) string {
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
