package config

import (
	"strings"
	"unicode"
)

// Slugify lowercases ASCII, maps spaces/underscores to '-', strips other
// non [a-z0-9-], collapses repeated '-', and trims edge '-'.
func Slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '_' || r == '-':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				// non-ASCII alnum: skip (ASCII-only slug)
				continue
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// DefaultPrefix returns <slug>/<timestamp> or <instanceID>/<timestamp> when slug is empty.
func DefaultPrefix(nameOrEmpty, instanceID, timestamp string) string {
	slug := Slugify(nameOrEmpty)
	if slug == "" {
		slug = instanceID
	}
	return slug + "/" + timestamp
}
