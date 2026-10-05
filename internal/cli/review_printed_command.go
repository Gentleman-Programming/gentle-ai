package cli

import (
	"fmt"
	"strings"
)

// SplitPrintedCommandWords splits one printed command line into the argv a
// POSIX shell would hand the product.
//
// It understands exactly the quoting reviewTransitionShellWord emits and
// nothing more: POSIX single quotes, and a backslash escape for the one
// character single quotes cannot contain (an embedded quote renders as '\”).
// Anything else, such as an unterminated quote or a trailing escape, is a
// line that would not run as printed, and saying so is the point.
//
// This is the same grammar bench's splitPrintedCommandWords enforces for
// envelope-printed commands (issue #1865). It lives here, beside the emitter
// it inverts, because the bench module cannot import the product module's
// internals; the two implementations must stay in step with
// reviewTransitionShellWord and with each other.
func SplitPrintedCommandWords(line string) ([]string, error) {
	words := []string{}
	var word strings.Builder
	quoted, escaped, started := false, false, false
	for _, char := range line {
		switch {
		case quoted && char == '\'':
			quoted = false
		case quoted:
			word.WriteRune(char)
		case escaped:
			word.WriteRune(char)
			escaped = false
		case char == '\\':
			escaped, started = true, true
		case char == '\'':
			quoted, started = true, true
		case char == ' ' || char == '\t' || char == '\n' || char == '\r':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if quoted {
		return nil, fmt.Errorf("printed a command with an unterminated quote: %q", line) // refusal:by-design world-action: the defect is the printed line itself; the exit is fixing its source, not a command
	}
	if escaped {
		return nil, fmt.Errorf("printed a command with a trailing escape: %q", line) // refusal:by-design world-action: the defect is the printed line itself; the exit is fixing its source, not a command
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

// ReviewOperationFlagSurface returns the set of flag names the negotiated
// review operation owning the given CLI verb declares in
// reviewIntegrationOperationRegistry, or found=false when no negotiated row
// with a declared flag surface owns that verb. The registry is the single
// policy source for the negotiated route (see its own comment), so a
// documented invocation of a negotiated verb can be checked against the
// exact flags negotiated routing recognises without re-deriving them.
func ReviewOperationFlagSurface(verb string) (map[string]bool, bool) {
	for _, metadata := range reviewIntegrationOperationRegistry {
		if !metadata.Negotiated || metadata.Command != verb {
			continue
		}
		flags := map[string]bool{}
		for _, name := range metadata.ValueFlags {
			flags[name] = true
		}
		for _, name := range metadata.BoolFlags {
			flags[name] = true
		}
		for _, name := range metadata.IntFlags {
			flags[name] = true
		}
		if len(flags) == 0 {
			return nil, false
		}
		return flags, true
	}
	return nil, false
}
