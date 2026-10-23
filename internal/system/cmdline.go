package system

import "strings"

// IsWindowsBatchFile reports whether path names a Windows batch script rather
// than a native executable. Only .bat and .cmd carry the quoting hazard this
// file exists to work around; matching is case-insensitive because Windows
// path extensions are.
func IsWindowsBatchFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd")
}

// WindowsBatchCommandLine builds the single command-line string Windows must
// hand to cmd.exe to launch a .bat/.cmd target whose path contains a space.
//
// Windows cannot run a batch file directly: it re-invokes cmd.exe with the
// already-built command-line text appended after "/c". That text follows the
// CommandLineToArgvW-compatible quoting Go's exec.Command builds for ordinary
// executables, but cmd.exe's own command-name lookup does not honor it --
// cmd.exe recognizes a leading quote only when the ENTIRE line is wrapped in
// one more, outer pair of quotes. Without that outer pair, cmd.exe splits the
// quoted path on its first space: a client shim under a path containing a space
// fails with `"C:\Program" is not recognized as an internal or external command`.
//
// Go's own exec.Command doc names this exact gap: "msiexec.exe and cmd.exe
// (and thus, all batch files) ... have a different unquoting algorithm. ...
// you can do the quoting yourself and provide the full command line in
// SysProcAttr.CmdLine". This function provides that quoting.
func WindowsBatchCommandLine(binary string, args []string) string {
	tokens := make([]string, 0, len(args)+1)
	tokens = append(tokens, QuoteWindowsArgument(binary))
	for _, arg := range args {
		tokens = append(tokens, QuoteWindowsArgument(arg))
	}
	return `"` + strings.Join(tokens, " ") + `"`
}

// QuoteWindowsArgument applies CommandLineToArgvW-compatible escaping:
// a backslash run is doubled only when it immediately precedes a double quote
// (or the closing quote), and any double quote is itself escaped. An argument
// with no space, tab, or quote needs no quoting at all, which also keeps
// empty-flag-value arguments (`--tools ""`) rendering as a literal `""`
// rather than vanishing.
func QuoteWindowsArgument(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	var quoted strings.Builder
	quoted.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
			quoted.WriteRune(r)
		case '"':
			for ; backslashes > 0; backslashes-- {
				quoted.WriteByte('\\')
			}
			quoted.WriteString(`\"`)
		default:
			backslashes = 0
			quoted.WriteRune(r)
		}
	}
	for ; backslashes > 0; backslashes-- {
		quoted.WriteByte('\\')
	}
	quoted.WriteByte('"')
	return quoted.String()
}
