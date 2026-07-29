package service

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const MaxQueryRunes = 512

// ErrQueryTooLong is returned when a user-supplied query or law id exceeds MaxQueryRunes.
var ErrQueryTooLong = errors.New("query too long")

func validateQueryLength(s string) error {
	if utf8.RuneCountInString(s) > MaxQueryRunes {
		return fmt.Errorf("%w (max %d runes)", ErrQueryTooLong, MaxQueryRunes)
	}
	return nil
}
