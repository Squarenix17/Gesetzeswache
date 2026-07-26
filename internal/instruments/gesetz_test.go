package instruments

import "testing"

func TestIsGesetzChild(t *testing.T) {
	tests := []struct {
		title string
		want  bool
	}{
		{"Gesetz über die Zertifizierung von Altersvorsorgeverträgen", true},
		{"Gesetz zur Regelung der Rechtsverhältnisse der in der Steuerverwaltung tätigen Personen", true},
		{"Asylbewerberleistungsgesetz", true},
		{"Anfechtungsgesetz", true},
		{"Einkommensteuergesetz", true},
		{"Fünfte Verordnung zur Anpassung des Mindestlohns", false},
		{"Verordnung zur Festlegung des Mindestunterhalts minderjähriger Kinder", false},
		{"Pflegeberufe-Ausbildungs- und Prüfungsverordnung", false},
		{"Fünfte Mindestlohnanpassungsverordnung", false},
		{"Bürgerliches Gesetzbuch", false},
		{"Mindestlohngesetz", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			if got := IsGesetzChild(tt.title); got != tt.want {
				t.Fatalf("IsGesetzChild(%q)=%v want %v", tt.title, got, tt.want)
			}
		})
	}
}
