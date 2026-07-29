package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/doctor"
)

// TestCheckOneTool_GentleAINamesTheInvokedExecutable closes fisidj finding 5
// (organic-dx Phase 3f task 3f.5): an RC tester who invokes gentle-ai by an
// absolute path may have a DIFFERENT gentle-ai on PATH -- doctor previously
// reported only the PATH-resolved copy as healthy, so a report could describe
// a build the tester never actually ran. The gentle-ai tool check must also
// name the invoked executable's own path (and version) so RC reports are
// unambiguous about which build was under test.
func TestCheckOneTool_GentleAINamesTheInvokedExecutable(t *testing.T) {
	origLook := lookPathFn
	origExec := osExecutableDoctor
	defer func() { lookPathFn = origLook; osExecutableDoctor = origExec }()

	pathCopy := filepath.Join(t.TempDir(), "gentle-ai")
	invokedCopy := filepath.Join(t.TempDir(), "gentle-ai")

	lookPathFn = func(string) (string, error) { return pathCopy, nil }
	osExecutableDoctor = func() (string, error) { return invokedCopy, nil }

	got := checkOneTool("gentle-ai", nil)

	if got.Status != CheckStatusPass {
		t.Fatalf("expected pass, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, pathCopy) {
		t.Fatalf("Detail dropped the PATH-resolved copy: %q", got.Detail)
	}
	if !strings.Contains(got.Detail, invokedCopy) {
		t.Fatalf("Detail does not name the invoked executable's own path: %q", got.Detail)
	}
}

// TestCheckOneTool_GentleAIFlagsWhenInvokedDiffersFromPath proves the exact
// ambiguity this task closes: when the invoked binary and the PATH-resolved
// binary are different files, the report says so explicitly instead of
// silently reporting only the PATH copy as healthy.
func TestCheckOneTool_GentleAIFlagsWhenInvokedDiffersFromPath(t *testing.T) {
	origLook := lookPathFn
	origExec := osExecutableDoctor
	defer func() { lookPathFn = origLook; osExecutableDoctor = origExec }()

	dir := t.TempDir()
	pathCopy := filepath.Join(dir, "system-install", "gentle-ai")
	invokedCopy := filepath.Join(dir, "rc-build", "gentle-ai")

	lookPathFn = func(string) (string, error) { return pathCopy, nil }
	osExecutableDoctor = func() (string, error) { return invokedCopy, nil }

	got := checkOneTool("gentle-ai", nil)

	if !strings.Contains(got.Detail, "differs") {
		t.Fatalf("Detail does not flag that the invoked build differs from the PATH copy: %q", got.Detail)
	}
}

// TestCheckOneTool_GentleAISameExecutableAsPathIsNotFlagged proves the common
// case (invoked == PATH-resolved) stays unambiguous and does not spuriously
// claim a mismatch.
func TestCheckOneTool_GentleAISameExecutableAsPathIsNotFlagged(t *testing.T) {
	origLook := lookPathFn
	origExec := osExecutableDoctor
	defer func() { lookPathFn = origLook; osExecutableDoctor = origExec }()

	same := filepath.Join(t.TempDir(), "gentle-ai")
	lookPathFn = func(string) (string, error) { return same, nil }
	osExecutableDoctor = func() (string, error) { return same, nil }

	got := checkOneTool("gentle-ai", nil)

	if strings.Contains(got.Detail, "differs") {
		t.Fatalf("Detail spuriously flags a mismatch when invoked == PATH-resolved: %q", got.Detail)
	}
}

// TestCheckOneTool_OtherToolsUnaffected proves the new clause is scoped to
// the gentle-ai tool only -- every other tool's Detail is unchanged.
func TestCheckOneTool_OtherToolsUnaffected(t *testing.T) {
	origLook := lookPathFn
	origExec := osExecutableDoctor
	defer func() { lookPathFn = origLook; osExecutableDoctor = origExec }()

	resolved := filepath.Join(t.TempDir(), "engram")
	called := false
	lookPathFn = func(string) (string, error) { return resolved, nil }
	osExecutableDoctor = func() (string, error) {
		called = true
		return "", errors.New("must not be called for other tools")
	}

	got := checkOneTool("engram", nil)

	want := "engram found at " + resolved
	if got.Detail != want {
		t.Fatalf("Detail = %q, want %q", got.Detail, want)
	}
	if called {
		t.Fatal("osExecutableDoctor was called for a non-gentle-ai tool")
	}
}

// TestCheckOneTool_GentleAIDuplicatesStillNameInvokedExecutable reproduces
// the reported regression: when 2+ copies of gentle-ai are found in PATH the
// check moves to the Warn/duplicate branch, which never called
// doctorInvokedGentleAIClause, so the one piece of information that
// disambiguates which build is actually running disappeared exactly when
// ambiguity is guaranteed. The invoked-executable clause must survive the
// duplicate branch without changing the duplicate detection, its severity,
// or the remedy.
func TestCheckOneTool_GentleAIDuplicatesStillNameInvokedExecutable(t *testing.T) {
	origLook := lookPathFn
	origExec := osExecutableDoctor
	origExts := executableExtsFn
	defer func() {
		lookPathFn = origLook
		osExecutableDoctor = origExec
		executableExtsFn = origExts
	}()
	executableExtsFn = func() []string { return []string{""} }

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	for _, dir := range []string{dir1, dir2} {
		p := filepath.Join(dir, "gentle-ai")
		if err := os.WriteFile(p, []byte("fake"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	invokedCopy := filepath.Join(t.TempDir(), "gentle-ai")
	lookPathFn = func(string) (string, error) { return filepath.Join(dir1, "gentle-ai"), nil }
	osExecutableDoctor = func() (string, error) { return invokedCopy, nil }

	got := checkOneTool("gentle-ai", []string{dir1, dir2})

	if got.Status != CheckStatusWarn {
		t.Fatalf("expected warn for duplicate copies, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "2 copies found") {
		t.Fatalf("duplicate detection message missing: %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "invoked executable: "+invokedCopy) {
		t.Fatalf("Detail dropped the invoked-executable clause on the duplicate branch: %q", got.Detail)
	}
	if got.Remedy == nil || got.Remedy.ID != doctor.RemedyRemoveDuplicates {
		t.Fatalf("expected the unchanged remove-duplicates remedy, got %+v", got.Remedy)
	}
}

// TestCheckOneTool_GentleAINotFoundNamesInvokedExecutableWithoutComparison
// covers the Fail branch: when gentle-ai cannot be resolved via PATH at all,
// there is no PATH-resolved copy to compare against, but the executable
// currently running THIS doctor check is still independently derivable via
// osExecutableDoctor. The clause must name it honestly, without fabricating
// a "differs from the PATH-resolved copy" comparison that has nothing to
// compare against.
func TestCheckOneTool_GentleAINotFoundNamesInvokedExecutableWithoutComparison(t *testing.T) {
	origLook := lookPathFn
	origExec := osExecutableDoctor
	defer func() { lookPathFn = origLook; osExecutableDoctor = origExec }()

	invokedCopy := filepath.Join(t.TempDir(), "gentle-ai")
	lookPathFn = func(string) (string, error) { return "", errors.New("not found") }
	osExecutableDoctor = func() (string, error) { return invokedCopy, nil }

	got := checkOneTool("gentle-ai", nil)

	if got.Status != CheckStatusFail {
		t.Fatalf("expected fail, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "not found in PATH") {
		t.Fatalf("Detail dropped the not-found message: %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "invoked executable: "+invokedCopy) {
		t.Fatalf("Detail dropped the invoked-executable clause on the not-found branch: %q", got.Detail)
	}
	if strings.Contains(got.Detail, "differs") {
		t.Fatalf("Detail fabricated a PATH-resolved comparison with nothing to compare against: %q", got.Detail)
	}
}

// TestCheckOneTool_GentleAIExecutableUnresolvable proves the honesty
// contract: when osExecutableDoctor itself cannot resolve the invoked
// executable, the clause stays empty on every branch rather than fabricating
// a version — this covers the duplicate branch, which previously had no
// clause call at all to exercise this path.
func TestCheckOneTool_GentleAIExecutableUnresolvable(t *testing.T) {
	origLook := lookPathFn
	origExec := osExecutableDoctor
	origExts := executableExtsFn
	defer func() {
		lookPathFn = origLook
		osExecutableDoctor = origExec
		executableExtsFn = origExts
	}()
	executableExtsFn = func() []string { return []string{""} }

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	for _, dir := range []string{dir1, dir2} {
		p := filepath.Join(dir, "gentle-ai")
		if err := os.WriteFile(p, []byte("fake"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	lookPathFn = func(string) (string, error) { return filepath.Join(dir1, "gentle-ai"), nil }
	osExecutableDoctor = func() (string, error) { return "", errors.New("cannot resolve") }

	got := checkOneTool("gentle-ai", []string{dir1, dir2})

	if strings.Contains(got.Detail, "invoked executable") {
		t.Fatalf("Detail fabricated an invoked-executable clause despite resolution failure: %q", got.Detail)
	}
}
