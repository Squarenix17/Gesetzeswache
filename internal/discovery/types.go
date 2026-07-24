// Package discovery parses Verordnung Eingangsformel Ermächtigungen and resolves parent laws.
package discovery

// Ermaechtigung is one statutory authorization reference from an ordinance preamble.
type Ermaechtigung struct {
	Section, Absatz, Satz string
	LawTitlePhrase, Jurabk string
	Raw                     string
}

// SectionHint returns the normalized section citation (e.g. "§ 55").
func (e Ermaechtigung) SectionHint() string {
	if e.Section == "" {
		return ""
	}
	return "§ " + e.Section
}
