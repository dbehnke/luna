package id

import (
	"time"

	"github.com/oklog/ulid/v2"
)

// NewULID generates a new ULID string.
// ULIDs are time-sortable unique identifiers.
func NewULID() string {
	return ulid.Make().String()
}

// NewULIDAt generates a ULID at a specific time.
// Useful for testing or restoring with known timestamps.
func NewULIDAt(t time.Time) string {
	return ulid.MustNew(ulid.Timestamp(t), entropy).String()
}

// Parse validates and parses a ULID string.
// Returns an error if the string is invalid.
func Parse(s string) (ulid.ULID, error) {
	return ulid.Parse(s)
}

// IsValid checks if a string is a valid ULID.
func IsValid(s string) bool {
	_, err := ulid.Parse(s)
	return err == nil
}

// MustParse parses a ULID string and panics on error.
func MustParse(s string) ulid.ULID {
	return ulid.MustParse(s)
}

// entropy is used for ULID generation.
// Using the default entropy source from oklog/ulid.
var entropy = ulid.DefaultEntropy()
