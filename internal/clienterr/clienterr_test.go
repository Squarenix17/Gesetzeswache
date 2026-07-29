package clienterr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/service"
)

func TestSanitize_intentionalExact(t *testing.T) {
	for _, msg := range []string{
		"query too long",
		"query required",
		"catalog not ready",
		"law id required",
		"law not found",
		"export refused: law confirmed_stale",
		"recheck timed out",
	} {
		if got := Sanitize(errors.New(msg)); got != msg {
			t.Fatalf("Sanitize(%q)=%q want %q", msg, got, msg)
		}
	}
}

func TestSanitize_sentinels(t *testing.T) {
	if got := Sanitize(fmt.Errorf("wrap: %w", service.ErrQueryTooLong)); got != "query too long" {
		t.Fatalf("got %q", got)
	}
	if got := Sanitize(service.ErrLawNotFound); got != "law not found" {
		t.Fatalf("got %q", got)
	}
	if got := Sanitize(service.ErrRecheckTimeout); got != "recheck timed out" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitize_masksInternal(t *testing.T) {
	for _, msg := range []string{
		"open /var/lib/gesetzeswache.db: permission denied",
		"bbolt: database closed",
		"unexpected bucket error",
	} {
		if got := Sanitize(errors.New(msg)); got != Internal {
			t.Fatalf("Sanitize(%q)=%q want %q", msg, got, Internal)
		}
	}
}

func TestSanitize_unknownFormatPrefix(t *testing.T) {
	msg := `unknown format "foo"`
	if got := Sanitize(errors.New(msg)); got != msg {
		t.Fatalf("got %q", got)
	}
}
