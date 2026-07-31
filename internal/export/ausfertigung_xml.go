package export

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/xmlsafe"
)

var reYYYYMMDD = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ExtractAusfertigungDatum finds the first ausfertigung-datum CharData in GII norm XML.
// Returns YYYY-MM-DD when parseable; otherwise empty string.
func ExtractAusfertigungDatum(xmlData []byte) string {
	if err := xmlsafe.RejectUnsafeXML(xmlData); err != nil {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var inAusfertigungDatum bool
	var buf string
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
			if strings.EqualFold(t.Name.Local, "ausfertigung-datum") {
				inAusfertigungDatum = true
				buf = ""
			}
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "ausfertigung-datum") {
				inAusfertigungDatum = false
				raw := strings.TrimSpace(buf)
				if reYYYYMMDD.MatchString(raw) {
					return raw
				}
				return ""
			}
		case xml.CharData:
			if inAusfertigungDatum {
				buf += string(t)
			}
		}
	}
	return ""
}
