package reviewerprovider

import "testing"

// Issue #4039: a `.cmd`/`.bat` client shim under a path containing a space
// broke every capture, because cmd.exe's own command-line reparsing does not
// honor the quoting Go's exec.Command builds for an ordinary executable. These
// tests exercise the pure quoting logic directly -- no process launch, no
// build-tagged file, portable to any OS this suite runs on -- so the exact
// cmd.exe-compatible string this fix produces is pinned regardless of host.

func TestIsWindowsBatchFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\Program Files\nodejs\claude.cmd`, true},
		{`C:\tools\claude.CMD`, true},
		{`C:\tools\claude.bat`, true},
		{`C:\tools\claude.exe`, false},
		{`/usr/local/bin/claude`, false},
	}
	for _, test := range tests {
		if got := isWindowsBatchFile(test.path); got != test.want {
			t.Errorf("isWindowsBatchFile(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

func TestQuoteWindowsArgument(t *testing.T) {
	tests := []struct{ arg, want string }{
		{"--print", "--print"}, // plain tokens need no quoting at all
		{"", `""`},
		{"a b", `"a b"`},
		{`say "hi"`, `"say \"hi\""`},
		{`c:\path with space\`, `"c:\path with space\\"`},
	}
	for _, test := range tests {
		if got := quoteWindowsArgument(test.arg); got != test.want {
			t.Errorf("quoteWindowsArgument(%q) = %q, want %q", test.arg, got, test.want)
		}
	}
}

func TestWindowsBatchCommandLineWrapsQuotedPathInOneMoreOuterPair(t *testing.T) {
	got := windowsBatchCommandLine(`C:\Program Files\nodejs\claude.cmd`, []string{"--print", "--tools", ""})
	want := `""C:\Program Files\nodejs\claude.cmd" --print --tools """`
	if got != want {
		t.Fatalf("windowsBatchCommandLine() = %q, want %q", got, want)
	}
}
