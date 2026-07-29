// Package xmlsafe provides shared guards for untrusted XML from the network.
package xmlsafe

import (
	"bytes"
	"fmt"
)

// RejectUnsafeXML rejects internal entity declarations (billion-laughs style expansion).
// External SYSTEM DOCTYPE declarations remain allowed (GII norms use them).
func RejectUnsafeXML(xmlData []byte) error {
	lower := bytes.ToLower(xmlData)
	for i := 0; i+2 < len(lower); i++ {
		if lower[i] != '<' || lower[i+1] != '!' {
			continue
		}
		j := i + 2
		for j < len(lower) && isXMLSpace(lower[j]) {
			j++
		}
		if j+6 <= len(lower) && bytes.HasPrefix(lower[j:], []byte("entity")) {
			return fmt.Errorf("xml contains entity declarations")
		}
	}
	return nil
}

func isXMLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
