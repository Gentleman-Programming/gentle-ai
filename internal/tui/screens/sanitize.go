package screens

import (
	"regexp"
	"strings"
)

// ansiEscapeRe matches ANSI escape sequences (CSI and two-character escapes).
var ansiEscapeRe = regexp.MustCompile(`\x1b(?:\[[0-9;]*[A-Za-z]|.)`)

// sanitizeLabel strips ANSI escape sequences and ASCII control characters
// (0x00-0x1f, 0x7f) from a user-sourced registry label before it reaches the
// terminal, so registry entries cannot forge menu content or issue control
// sequences. It is pure and keeps printable characters as-is.
func sanitizeLabel(s string) string {
	s = ansiEscapeRe.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x20 && r != 0x7f {
			b.WriteRune(r)
		}
	}
	return b.String()
}
