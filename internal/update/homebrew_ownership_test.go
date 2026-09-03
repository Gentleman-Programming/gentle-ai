package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type ownershipProbe struct {
	formula, cask, formulaRoot, caskPrefix, brewRoot, fail string
}

// run dispatches brew commands used by DetectHomebrewOwnership. The expected
// artifact name is derived from the formula output (first "tap/<name>" or
// bare "<name>" entry), which lets the same probe serve both "engram" and
// "gentle-ai" tests. The `--prefix` (no-args) command always answers with
// the Homebrew root in production; the probe falls back to `caskPrefix` only
// when `brewRoot` is empty so legacy tests can simulate a Homebrew root that
// points at a path we never created (so the derived cask root fails
// EvalSymlinks).
func (p ownershipProbe) run(_ string, args ...string) ([]byte, error) {
	command := strings.Join(args, " ")
	if command == p.fail {
		return nil, errors.New("probe failed")
	}
	outputs := map[string]string{
		"list --formula --full-name": p.formula,
		"list --cask --full-name":    p.cask,
	}
	if p.formula != "" {
		artifact := firstArtifact(p.formula)
		if artifact != "" {
			outputs["--prefix "+artifact] = p.formulaRoot
		}
	}
	// `brew --prefix` (no args) answers with the Homebrew root.
	if p.brewRoot != "" {
		outputs["--prefix"] = p.brewRoot
	} else {
		outputs["--prefix"] = p.caskPrefix
	}
	return []byte(outputs[command]), nil
}

func firstArtifact(list string) string {
	for _, line := range strings.Split(list, "\n") {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		return candidate
	}
	return ""
}

func TestDetectHomebrewOwnershipWith(t *testing.T) {
	root := t.TempDir()
	brew := filepath.Join(root, "homebrew")
	formula := filepath.Join(brew, "Cellar/engram/1.2.3")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Materialize the Caskroom layout once so cases that need a real cask
	// resolution succeed; cases that need a missing cask root use a probe
	// value that points at a path we never create.
	cask := filepath.Join(brew, "Caskroom/engram/1.2.3")
	for _, path := range []string{filepath.Join(formula, "bin/engram"), filepath.Join(cask, "bin/engram"), filepath.Join(root, "local/engram")} {
		must(os.MkdirAll(filepath.Dir(path), 0o755))
		must(os.WriteFile(path, []byte("binary"), 0o755))
	}
	formulaLink, caskLink := filepath.Join(root, "formula-link"), filepath.Join(root, "cask-link")
	must(os.Symlink(filepath.Join(formula, "bin/engram"), formulaLink))
	must(os.Symlink(filepath.Join(cask, "bin/engram"), caskLink))
	missingCaskRoot := filepath.Join(brew, "never-created")
	tests := []struct {
		name, formulaList, caskList, formulaRoot, caskPrefix, brewRoot, fail, active string
		want                                                                         HomebrewOwnership
		err                                                                          bool
	}{
		{"both select formula symlink", "engram", "engram", formula, brew, brew, "", formulaLink, HomebrewFormula, false},
		{"both select cask", "engram", "engram", formula, brew, brew, "", caskLink, HomebrewCask, false},
		{"both roots match", "engram", "engram", brew, brew, brew, "", caskLink, HomebrewNone, true},
		{"shadowed", "engram", "", formula, brew, brew, "", filepath.Join(root, "local/engram"), HomebrewNone, true},
		{"cask list failure", "", "", formula, brew, brew, "list --cask --full-name", "", HomebrewNone, true},
		{"formula prefix failure", "engram", "", formula, brew, brew, "--prefix engram", formulaLink, HomebrewNone, true},
		{"formula prefix empty", "engram", "", "", brew, brew, "", formulaLink, HomebrewNone, true},
		{"cask prefix failure", "", "engram", formula, brew, brew, "--prefix", caskLink, HomebrewNone, true},
		// "cask prefix empty" previously fed an empty `brew --prefix` answer.
		// After the brew-root refactor, `brew --prefix` (no args) returns the
		// Homebrew root used to derive the bin directory; an empty answer there
		// is a fresh failure mode and must surface as an error.
		{"brew root empty", "", "engram", formula, "", "", "", caskLink, HomebrewNone, true},
		// Force the derived cask root to a path we never created; EvalSymlinks
		// must fail and the ownership check must surface an error. brewRoot is
		// intentionally left empty so the probe's `--prefix` answer points at
		// the missing path instead of the real brew root.
		{"cask root nonexistent", "", "engram", formula, missingCaskRoot, "", "", caskLink, HomebrewNone, true},
	}
	for _, tt := range tests {
		probe := ownershipProbe{tt.formulaList, tt.caskList, tt.formulaRoot, tt.caskPrefix, tt.brewRoot, tt.fail}
		got, err := detectHomebrewOwnershipWith(probe.run, func(string) (string, error) { return tt.active, nil }, "engram")
		if got != tt.want || (err != nil) != tt.err {
			t.Fatalf("%s: ownership=%q error=%v; want %q error=%v", tt.name, got, err, tt.want, tt.err)
		}
	}
}

// TestDetectHomebrewOwnershipBrewCreatedSymlinkToExternalBinary reproduces
// the scenario from the bug report: a tool installed via `brew install` whose
// /opt/homebrew/bin/<tool> symlink was redirected by the user to a local build
// (e.g. /Users/<name>/go/bin/<tool>). The package is still Homebrew-owned —
// `brew install` placed the symlink and `brew upgrade` will replace it — but
// following the symlink leads outside the Homebrew prefix. Prior to the fix
// this returned an error and broke `gentle-ai update`/`gentle-ai sync` with
// "active executable %q is outside installed Homebrew paths".
func TestDetectHomebrewOwnershipBrewCreatedSymlinkToExternalBinary(t *testing.T) {
	brew := t.TempDir()
	mustMkdir(t, filepath.Join(brew, "Cellar/gentle-ai/2.10.1/bin"))
	mustMkdir(t, filepath.Join(brew, "opt/gentle-ai")) // formula root must exist on disk
	mustMkdir(t, filepath.Join(brew, "bin"))

	// External binary the user points the symlink at (e.g. a `go install` build).
	external := filepath.Join(t.TempDir(), "local-bin")
	mustMkdir(t, external)
	mustWrite(t, filepath.Join(external, "gentle-ai"), []byte("binary"))

	// Symlink placed by `brew install` in <brew-root>/bin, pointing outside.
	brewSymlink := filepath.Join(brew, "bin", "gentle-ai")
	mustSymlink(t, filepath.Join(external, "gentle-ai"), brewSymlink)

	probe := ownershipProbe{
		formula:     "gentleman-programming/tap/gentle-ai",
		cask:        "",
		formulaRoot: filepath.Join(brew, "opt/gentle-ai"),
		caskPrefix:  brew,
		brewRoot:    brew,
	}
	got, err := detectHomebrewOwnershipWith(
		probe.run,
		func(string) (string, error) { return brewSymlink, nil },
		"gentle-ai",
	)
	if err != nil {
		t.Fatalf("detectHomebrewOwnershipWith returned error: %v", err)
	}
	if got != HomebrewFormula {
		t.Fatalf("ownership = %q, want %q", got, HomebrewFormula)
	}
}

// TestPathWithinPrefixBrewSymlinkTargetOutsidePrefix is the unit-level version
// of the same scenario. The symlink lives inside <brew-root>/bin, but its
// resolved target does not. The function must still report "within prefix"
// because brew created the symlink and brew upgrade will overwrite it.
func TestPathWithinPrefixBrewSymlinkTargetOutsidePrefix(t *testing.T) {
	brew := t.TempDir()
	bin := filepath.Join(brew, "bin")
	mustMkdir(t, bin)

	external := filepath.Join(t.TempDir(), "go/bin")
	mustMkdir(t, external)

	brewSymlink := filepath.Join(bin, "gentle-ai")
	mustSymlink(t, filepath.Join(external, "gentle-ai"), brewSymlink)

	if !pathWithinPrefix(brewSymlink, brew, bin) {
		t.Fatalf("pathWithinPrefix(%q, %q, %q) = false; want true (Homebrew symlink to external target must count as Homebrew-owned)", brewSymlink, brew, bin)
	}
	// Negative control: a binary in a fully unrelated location must still be
	// rejected even when the brew bin directory exists.
	stranger := filepath.Join(t.TempDir(), "elsewhere/gentle-ai")
	mustMkdir(t, filepath.Dir(stranger))
	mustWrite(t, stranger, []byte("binary"))
	if pathWithinPrefix(stranger, brew, bin) {
		t.Fatalf("pathWithinPrefix(%q, %q, %q) = true; want false", stranger, brew, bin)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func mockNoHomebrew(t *testing.T) {
	original := homebrewOwnershipDetector
	t.Cleanup(func() { homebrewOwnershipDetector = original })
	homebrewOwnershipDetector = func(string) (HomebrewOwnership, error) { return HomebrewNone, nil }
}

func TestCheckSingleToolHomebrewBoundary(t *testing.T) {
	original := homebrewOwnershipDetector
	t.Cleanup(func() { homebrewOwnershipDetector = original })
	tool := ToolInfo{Name: "engram", Owner: "x", Repo: "y", DetectCmd: []string{"false"}}
	for _, kind := range []HomebrewOwnership{HomebrewFormula, HomebrewCask} {
		homebrewOwnershipDetector = func(string) (HomebrewOwnership, error) { return kind, nil }
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		want := "brew upgrade --" + string(kind) + " engram"
		if got := checkSingleTool(ctx, tool, "", system.PlatformProfile{PackageManager: "brew"}).UpdateHint; got != want {
			t.Errorf("ownership %s hint=%q, want %q", kind, got, want)
		}
	}
	homebrewOwnershipDetector = func(string) (HomebrewOwnership, error) { return HomebrewNone, errors.New("cask list denied") }
	result := checkSingleTool(t.Context(), tool, "", system.PlatformProfile{PackageManager: "brew"})
	output := RenderCLI([]UpdateResult{result})
	for _, want := range []string{"cask list denied", "brew list --cask --full-name", "command -v engram"} {
		if result.Status != CheckFailed || !strings.Contains(output, want) {
			t.Fatalf("result=%#v output missing %q:\n%s", result, want, output)
		}
	}
}
