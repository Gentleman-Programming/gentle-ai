//go:build windows

package privatefile

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// parseWindowsPrivateDestination recognizes only literal drive-absolute syntax.
// The returned root is a PATH STRING, not a trusted handle: drive letters may
// name redirected/remote volumes. Do not open it as authority or use these
// components for a report until same-handle root provenance is established.
func parseWindowsPrivateDestination(destination string) (root string, parents []string, leaf string, err error) {
	invalid := func() (string, []string, string, error) {
		return "", nil, "", fmt.Errorf("ambiguous Windows private report path: %w", ErrInvalidDestination)
	}
	if len(destination) < 4 || !utf8.ValidString(destination) ||
		!((destination[0] >= 'A' && destination[0] <= 'Z') || (destination[0] >= 'a' && destination[0] <= 'z')) ||
		destination[1:3] != `:\` || strings.Contains(destination, "/") {
		return invalid()
	}
	parts := strings.Split(destination[3:], `\`)
	// A report directly under the system-owned volume root has no private
	// parent. Syntax alone does not establish any parent ACL or owner.
	if len(parts) < 2 {
		return invalid()
	}
	for _, part := range parts {
		if !windowsPrivateComponentSafe(part) || !windowsPrivateLiteralNameSafe(part) {
			return invalid()
		}
	}
	return destination[:3], parts[:len(parts)-1], parts[len(parts)-1], nil
}

// Reject additional DOS device spellings and short-name alias syntax not
// covered by the parent walker's component check. This is only a syntax
// filter; handle-relative checks must still refuse actual reparse children.
func windowsPrivateLiteralNameSafe(name string) bool {
	if strings.Contains(name, "~") {
		return false
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch base {
	case "CONIN$", "CONOUT$", "CLOCK$", "COM0", "LPT0",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return false
	}
	return true
}
