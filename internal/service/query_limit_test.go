package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Squarenix17/gesetzeswache/internal/config"
)

func TestValidateQueryLength(t *testing.T) {
	if err := validateQueryLength(strings.Repeat("a", MaxQueryRunes)); err != nil {
		t.Fatal(err)
	}
	err := validateQueryLength(strings.Repeat("ä", MaxQueryRunes+1))
	if err == nil {
		t.Fatal("expected error for 513 runes")
	}
	if !strings.Contains(err.Error(), "query too long") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolve_QueryTooLong(t *testing.T) {
	s := &Service{CFG: config.Config{MatchThreshold: 0.75}}
	_, err := s.Resolve(context.Background(), strings.Repeat("x", MaxQueryRunes+1), IncludeOpts{})
	if err == nil || !strings.Contains(err.Error(), "query too long") {
		t.Fatalf("err=%v", err)
	}
}
