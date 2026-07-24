package service

import "strings"

// IncludeOpts controls linked-instrument response shaping.
type IncludeOpts struct {
	Past   bool // include status=past instruments
	Linked bool // attach child law pointers (law_id, gii_url, resolve_ok)
}

// ParseInclude parses comma-separated include tokens (e.g. "past,linked").
func ParseInclude(raw string) IncludeOpts {
	var o IncludeOpts
	for _, part := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "past":
			o.Past = true
		case "linked":
			o.Linked = true
		}
	}
	return o
}

// MergeInclude merges multiple include query values (e.g. ?include=past&include=linked).
func MergeInclude(values []string) IncludeOpts {
	var o IncludeOpts
	for _, v := range values {
		p := ParseInclude(v)
		o.Past = o.Past || p.Past
		o.Linked = o.Linked || p.Linked
	}
	return o
}
