package app

import (
	"bytes"
	"strings"
	"testing"
)

// The help text pins the full contract, including the local Git exclusion
// init adds so the index never leaks into diffs (C7).
func TestRunArgsCodeGraphHelp(t *testing.T) {
	const want = `gentle-ai codegraph — Initialize a project's CodeGraph index

USAGE
  gentle-ai codegraph init --cwd <project-root>

OPTIONS
  --cwd <project-root>  Existing Git project directory (required for initialization)
  --help, -h            Show this help without initializing an index

Running 'gentle-ai codegraph' or 'gentle-ai codegraph init' without arguments shows this help.
Only 'init --cwd <project-root>' is supported; other CodeGraph commands are not forwarded.
Initialization adds .codegraph/ to the worktree's local Git exclude file (never .gitignore)
so the index stays out of git status; a failure there is reported and initialization continues.
`
	for _, args := range [][]string{
		{"codegraph"},
		{"codegraph", "init"},
		{"codegraph", "--help"},
		{"codegraph", "-h"},
		{"codegraph", "init", "--help"},
		{"codegraph", "init", "-h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var output bytes.Buffer
			if err := RunArgs(args, &output); err != nil {
				t.Fatalf("RunArgs(%v) error = %v", args, err)
			}
			if output.String() != want {
				t.Fatalf("stdout = %q, want %q", output.String(), want)
			}
		})
	}
}
