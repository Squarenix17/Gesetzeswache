package normalize

import "testing"

func containsKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

func TestAlternateKeys_SGBRomanAndArabic(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"SGB III", "sgb3"},
		{"SGB 3", "sgb3"},
		{"sgb3", "sgbiii"},
		{"sgb_3", "sgb3"},
		{"SGB V", "sgb5"},
		{"sgb_5", "sgb5"},
	}
	for _, tc := range cases {
		keys := AlternateKeys(tc.query)
		if !containsKey(keys, tc.want) {
			t.Fatalf("AlternateKeys(%q)=%v want to include %q", tc.query, keys, tc.want)
		}
	}
}

func TestParseSGBBookQuery(t *testing.T) {
	cases := []struct {
		in   string
		num  int
		want bool
	}{
		{"SGB III", 3, true},
		{"sgb_3", 3, true},
		{"sgb3", 3, true},
		{"SGB V", 5, true},
		{"SGB 11", 11, true},
		{"SGB XI", 11, true},
		{"BGB", 0, false},
		{"SGB", 0, false},
		{"SGB III § 1", 0, false},
	}
	for _, tc := range cases {
		num, ok := ParseSGBBookQuery(tc.in)
		if ok != tc.want || (tc.want && num != tc.num) {
			t.Fatalf("ParseSGBBookQuery(%q)=(%d,%v) want (%d,%v)", tc.in, num, ok, tc.num, tc.want)
		}
	}
}

func TestRomanToInt(t *testing.T) {
	if n, ok := RomanToInt("XI"); !ok || n != 11 {
		t.Fatalf("RomanToInt(XI)=(%d,%v)", n, ok)
	}
	if n, ok := RomanToInt("III"); !ok || n != 3 {
		t.Fatalf("RomanToInt(III)=(%d,%v)", n, ok)
	}
}
