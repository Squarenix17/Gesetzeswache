// Package giiurl builds and validates GII/recht.bund.de URLs safely.
package giiurl

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const MaxSlugLen = 128

var slugRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidSlug returns true if slug is a single safe path segment.
func ValidSlug(slug string) bool {
	return slug != "" && len(slug) <= MaxSlugLen && slugRe.MatchString(slug) && !strings.Contains(slug, "..")
}

// XMLZip builds https://{host}/{slug}/xml.zip from configured GII base.
func XMLZip(giiBase, slug string) (string, error) {
	if !ValidSlug(slug) {
		return "", fmt.Errorf("invalid gii slug")
	}
	base, err := url.Parse(giiBase)
	if err != nil || base.Host == "" {
		return "", fmt.Errorf("invalid gii base")
	}
	base.Scheme = "https"
	base.Path = "/" + slug + "/xml.zip"
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

// IndexURL builds https://{host}/{slug}/ for HTML Stand extraction.
func IndexURL(giiBase, slug string) (string, error) {
	if !ValidSlug(slug) {
		return "", fmt.Errorf("invalid gii slug")
	}
	base, err := url.Parse(giiBase)
	if err != nil || base.Host == "" {
		return "", fmt.Errorf("invalid gii base")
	}
	base.Scheme = "https"
	base.Path = "/" + slug + "/"
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

// SlugFromTOCLink extracts slug from a TOC xml.zip link; does not trust host.
func SlugFromTOCLink(link string) string {
	link = strings.TrimSpace(link)
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// expect {slug}/xml.zip or {slug}
	for i, p := range parts {
		if strings.EqualFold(p, "xml.zip") && i > 0 {
			cand := parts[i-1]
			if ValidSlug(cand) {
				return cand
			}
		}
	}
	if len(parts) == 1 && ValidSlug(parts[0]) {
		return parts[0]
	}
	if len(parts) >= 1 {
		cand := parts[0]
		if ValidSlug(cand) {
			return cand
		}
	}
	return ""
}
