// Package pathquote renders filesystem paths inside user-facing text, most
// often the printed `--cwd <path>` of a suggested invocation.
//
// fmt's %q is the wrong verb for that job: it renders a Go string literal, so
// it escapes every backslash and a Windows path prints with doubled
// separators (`C:\Users\dev\repo` becomes `"C:\\Users\\dev\\repo"`). A user
// who copies the suggested command gets a path the filesystem does not know,
// and strings.Contains(message, path) is false for the real path.
//
// Quote preserves the path's bytes exactly, so the printed invocation is the
// invocation that runs. Non-path values (lineage names, revisions, closure
// members) are Go-string territory and keep using %q; this package is for
// paths only.
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
