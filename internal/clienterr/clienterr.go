// Package clienterr sanitizes errors for client-facing HTTP and MCP responses.
package clienterr

import (
	"errors"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/service"
)

// Internal is the generic client message for unexpected failures.
const Internal = "internal error"

// exactAllowlist holds intentional client error strings (byte-identical to API contract).
var exactAllowlist = map[string]struct{}{
	"query too long":                      {},
	"query required":                      {},
	"catalog not ready":                   {},
	"law id required":                     {},
	"law not found":                       {},
	"invalid json body":                   {},
	"POST required":                       {},
	"export disabled":                     {},
	"empty format list":                   {},
	"export refused: law confirmed_stale": {},
	"export refused: bundle member confirmed_stale":                 {},
	"unauthorized: set GEW_SHARED_SECRET and X-Gesetzeswache-Token": {},
	"recheck timed out":              {},
	"parse error":                    {},
	"parse error: message too large": {},
}

// Sanitize maps err to an intentional client message or Internal.
func Sanitize(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, service.ErrQueryTooLong) {
		return "query too long"
	}
	if errors.Is(err, service.ErrLawNotFound) {
		return "law not found"
	}
	if errors.Is(err, service.ErrRecheckTimeout) {
		return "recheck timed out"
	}
	msg := err.Error()
	if isAllowed(msg) {
		return msg
	}
	return Internal
}

func isAllowed(msg string) bool {
	if _, ok := exactAllowlist[msg]; ok {
		return true
	}
	for _, prefix := range []string{
		"unknown format ",
		"unknown tool ",
		"method not found: ",
		"operative bundle too large",
	} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}
