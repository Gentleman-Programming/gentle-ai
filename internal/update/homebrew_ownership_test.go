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
	formula, cask, formulaRoot, caskPrefix, fail string
}

func (p ownershipProbe) run(_ string, args ...string) ([]byte, error) {
	command := strings.Join(args, " ")
	if command == p.fail {
		return nil, errors.New("probe failed")
	}
	outputs := map[string]string{
		"list --formula --full-name": p.formula,
		"list --cask --full-name":    p.cask,
		"--prefix engram":            p.formulaRoot,
		"--prefix":                   p.caskPrefix,
	}
	return []byte(outputs[command]), nil
}

func TestDetectHomebrewOwnershipWith(t *testing.T) {
	root := t.TempDir()
	brew := filepath.Join(root, "homebrew")
	formula := filepath.Join(brew, "Cellar/engram/1.2.3")
	cask := filepath.Join(brew, "Caskroom/engram/1.2.3")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(formula, "bin/engram"), filepath.Join(cask, "bin/engram"), filepath.Join(root, "local/engram")} {
		must(os.MkdirAll(filepath.Dir(path), 0o755))
		must(os.WriteFile(path, []byte("binary"), 0o755))
	}
	formulaLink, caskLink := filepath.Join(root, "formula-link"), filepath.Join(root, "cask-link")
	must(os.Symlink(filepath.Join(formula, "bin/engram"), formulaLink))
	must(os.Symlink(filepath.Join(cask, "bin/engram"), caskLink))
	tests := []struct {
		name, formulaList, caskList, formulaRoot, caskPrefix, fail, active string
		want                                                               HomebrewOwnership
		err                                                                bool
	}{
		{"both select formula symlink", "engram", "engram", formula, brew, "", formulaLink, HomebrewFormula, false},
		{"both select cask", "engram", "engram", formula, brew, "", caskLink, HomebrewCask, false},
		{"both roots match", "engram", "engram", brew, brew, "", caskLink, HomebrewNone, true},
		{"shadowed", "engram", "", formula, brew, "", filepath.Join(root, "local/engram"), HomebrewNone, true},
		{"cask list failure", "", "", formula, brew, "list --cask --full-name", "", HomebrewNone, true},
		{"formula prefix failure", "engram", "", formula, brew, "--prefix engram", formulaLink, HomebrewNone, true},
		{"formula prefix empty", "engram", "", "", brew, "", formulaLink, HomebrewNone, true},
		{"cask prefix failure", "", "engram", formula, brew, "--prefix", caskLink, HomebrewNone, true},
		{"cask prefix empty", "", "engram", formula, "", "", caskLink, HomebrewNone, true},
		{"cask room missing", "", "engram", formula, filepath.Join(root, "missing"), "", caskLink, HomebrewNone, true},
	}
	for _, tt := range tests {
		probe := ownershipProbe{tt.formulaList, tt.caskList, tt.formulaRoot, tt.caskPrefix, tt.fail}
		got, err := detectHomebrewOwnershipWith(probe.run, func(string) (string, error) { return tt.active, nil }, "engram")
		if got != tt.want || (err != nil) != tt.err {
			t.Fatalf("%s: ownership=%q error=%v; want %q error=%v", tt.name, got, err, tt.want, tt.err)
		}
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
