// Package pathquote renders filesystem paths inside user-facing text, most
// often the printed `--cwd <path>` of a suggested invocation.
//
// fmt's %q is the wrong verb for that job: it renders a Go string literal, so
// it escapes every backslash and a Windows path prints with doubled
// separators (`C:\Users\dev\repo` becomes `"C:\\Users\\dev\\repo"`). A user
// who copies the suggested command gets a path the filesystem does not know,
// and strings.Contains(message, path) is false for the real path.
//
// Quote preserves a path's bytes exactly, while ShellWord safely renders any
// dynamic value in an executable POSIX-shell continuation.
package pathquote

import "strings"

// Quote wraps path in double quotes for copy-paste into a printed command
// invocation, preserving the path's bytes exactly: separators are never
// escaped, so a Windows path renders with single backslashes and
// strings.Contains(message, path) holds on every platform. The only rewritten
// byte is an embedded double quote, escaped as \" so the quoted token
// survives a POSIX shell; Windows forbids double quotes in paths, so Windows
// paths are always byte-identical inside the quotes.
func Quote(path string) string {
	return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
}

// ShellWord renders one value as a POSIX shell word. It quotes by allowlist so
// values copied from a status message cannot split, expand, or execute.
func ShellWord(value string) string {
	if value != "" && !strings.ContainsFunc(value, shellWordUnsafe) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shellWordUnsafe(char rune) bool {
	switch {
	case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		return false
	}
	return !strings.ContainsRune("-_=./:,+@", char)
}
