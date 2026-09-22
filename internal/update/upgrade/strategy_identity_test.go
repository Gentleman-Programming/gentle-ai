package upgrade

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

// TestStrategyMessagesNeverNameUpstream scans the real output of the strategy
// functions for upstream identity strings (F6.1, gate 1).
func TestStrategyMessagesNeverNameUpstream(t *testing.T) {
	origHomebrew := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrew })
	homebrewPackageInstalled = func(string) bool { return false }

	origDetectOS := detectOS
	t.Cleanup(func() { detectOS = origDetectOS })
	detectOS = func() string { return "linux" }

	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })
	lookPathFn = func(string) (string, error) { return t.TempDir() + `/axiom`, nil }

	origExec := execCommand
	t.Cleanup(func() { execCommand = origExec })
	execCommand = mockGoEnvError() // no go install / no downloads

	forkTool := update.ToolInfo{
		Name:         "axiom",
		Owner:        "IGutierrezZ",
		Repo:         "axiom",
		GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom",
		GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
	}

	forbidden := []string{"Gentleman-Programming/gentle-ai", "cmd/gentle-ai"}

	collect := func(name string, err error) {
		if err == nil {
			return
		}
		msg := err.Error()
		for _, f := range forbidden {
			if strings.Contains(msg, f) {
				t.Errorf("%s message names upstream %q: %s", name, f, msg)
			}
		}
	}

	// binaryUpgrade on Windows with self-tool.
	r := update.UpdateResult{Tool: forkTool, LatestVersion: "1.0.0", Status: update.UpdateAvailable}
	_, err := runStrategy(context.Background(), r, system.PlatformProfile{OS: "windows", GoAvailable: true})
	collect("runStrategy Windows self-tool", err)

	// binaryUpgrade on Linux with self-tool (falls to downloadAndReplace which
	// will fail without network — that error is allowed).
	_, err = runStrategy(context.Background(), r, system.PlatformProfile{OS: "linux", GoAvailable: true})
	collect("runStrategy Linux self-tool", err)

	// Default fallback (unsupported method).
	_, err = runStrategy(context.Background(), r, system.PlatformProfile{OS: "windows", GoAvailable: true, PackageManager: "winget"})
	collect("runStrategy unsupported method fallback", err)
}

// TestGoInstallMainUpgradeOnlyWhenResolvable verifies that goInstallMainUpgrade
// composes `go install <GoImportPath>@main` ONLY when GoInstallResolvable()
// (REQ-22.2).
func TestGoInstallMainUpgradeOnlyWhenResolvable(t *testing.T) {
	origExec := execCommand
	t.Cleanup(func() { execCommand = origExec })
	origDetectOS := detectOS
	t.Cleanup(func() { detectOS = origDetectOS })
	detectOS = func() string { return "linux" }

	t.Run("unresolvable module returns ManualFallbackError without go install", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			t.Fatalf("unresolvable beta path executed %s %v", name, args)
			return nil
		}
		tool := update.ToolInfo{
			Name:         "axiom",
			Owner:        "IGutierrezZ",
			Repo:         "axiom",
			GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom",
			GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
		}
		err := goInstallMainUpgrade(tool)
		if err == nil {
			t.Fatal("expected ManualFallbackError for unresolvable go install")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
	})
}

// TestIsSelfBetaUpgradeDetectsBothNames verifies the beta channel predicate
// works for axiom and gentle-ai (F6.2).
func TestIsSelfBetaUpgradeDetectsBothNames(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		version  string
		want     bool
	}{
		{name: "axiom main@", toolName: "axiom", version: "main@abc123", want: true},
		{name: "gentle-ai main@", toolName: "gentle-ai", version: "main@abc123", want: true},
		{name: "axiom stable", toolName: "axiom", version: "1.0.0", want: false},
		{name: "engram main@", toolName: "engram", version: "main@abc123", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := update.UpdateResult{
				Tool:          update.ToolInfo{Name: tc.toolName},
				LatestVersion: tc.version,
			}
			if got := IsSelfBetaUpgrade(r); got != tc.want {
				t.Errorf("IsSelfBetaUpgrade(%q, %q) = %v, want %v", tc.toolName, tc.version, got, tc.want)
			}
		})
	}
}
