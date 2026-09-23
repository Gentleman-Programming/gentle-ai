package upgrade

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

// mockGoEnv returns an execCommand stub that answers `go env KEY` from the
// provided map. Missing keys produce an empty line (matching `go env` for an
// unset variable).
func mockGoEnv(values map[string]string) func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		if name == "go" && len(args) == 2 && args[0] == "env" {
			return mockCmd("echo", values[args[1]])
		}
		return mockCmd("false")
	}
}

// mockGoEnvError returns an execCommand stub where every `go env` call fails.
func mockGoEnvError() func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		return mockCmd("false")
	}
}

// TestPreflightWindowsSelfBinaryWrite covers the single provenance gate shared
// by every write path (D-03, T-5). The guarantee: never create a second
// PATH-visible binary (REQ-22.1).
func TestPreflightWindowsSelfBinaryWrite(t *testing.T) {
	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })

	origExec := execCommand
	t.Cleanup(func() { execCommand = origExec })

	tool := update.ToolInfo{
		Name:         "axiom",
		Owner:        "IGutierrezZ",
		Repo:         "axiom",
		GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom",
		GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
	}

	t.Run("destination and active both resolvable and distinct returns ManualFallbackError naming both paths", func(t *testing.T) {
		goPath := t.TempDir()
		// filepath.Join so the expectation is exactly what
		// preflightWindowsSelfBinaryWrite computes on the running host.
		destination := filepath.Join(goPath, "bin", "axiom.exe")
		active := filepath.Join(t.TempDir(), "active", "axiom.exe")

		execCommand = mockGoEnv(map[string]string{"GOBIN": "", "GOPATH": goPath})
		lookPathFn = func(string) (string, error) { return active, nil }

		profile := system.PlatformProfile{OS: "windows"}
		_, err := preflightWindowsSelfBinaryWrite(tool, profile)
		if err == nil {
			t.Fatal("expected ManualFallbackError when destination and active differ")
		}
		hint, ok := AsManualFallback(err)
		if !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
		if !strings.Contains(hint, active) || !strings.Contains(hint, destination) {
			t.Errorf("hint must name both paths:\n  active: %s\n  dest: %s\n  hint: %s", active, destination, hint)
		}
	})

	t.Run("active unresolvable returns ManualFallbackError", func(t *testing.T) {
		goPath := t.TempDir()
		execCommand = mockGoEnv(map[string]string{"GOBIN": "", "GOPATH": goPath})
		lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }

		profile := system.PlatformProfile{OS: "windows"}
		_, err := preflightWindowsSelfBinaryWrite(tool, profile)
		if err == nil {
			t.Fatal("expected ManualFallbackError when active binary is unresolvable")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
	})

	t.Run("destination unresolvable returns ManualFallbackError", func(t *testing.T) {
		execCommand = mockGoEnvError()
		lookPathFn = func(string) (string, error) { return t.TempDir() + `\axiom.exe`, nil }

		profile := system.PlatformProfile{OS: "windows"}
		_, err := preflightWindowsSelfBinaryWrite(tool, profile)
		if err == nil {
			t.Fatal("expected ManualFallbackError when destination is unresolvable")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
	})

	t.Run("same path returns nil", func(t *testing.T) {
		goPath := t.TempDir()
		// Deliberately backslash-separated: the same path as the filepath.Join
		// form preflightWindowsSelfBinaryWrite computes for the destination.
		// Windows folds both separators into one path, so the comparison has to
		// as well — on every host, not only on Windows.
		destination := goPath + `\bin\axiom.exe`
		execCommand = mockGoEnv(map[string]string{"GOBIN": "", "GOPATH": goPath})
		lookPathFn = func(string) (string, error) { return destination, nil }

		profile := system.PlatformProfile{OS: "windows"}
		if _, err := preflightWindowsSelfBinaryWrite(tool, profile); err != nil {
			t.Fatalf("preflightWindowsSelfBinaryWrite() = %v, want nil for matching paths", err)
		}
	})

	t.Run("non-Windows returns nil without checking", func(t *testing.T) {
		execCommand = mockGoEnvError() // would fail if called
		lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }

		profile := system.PlatformProfile{OS: "linux"}
		if _, err := preflightWindowsSelfBinaryWrite(tool, profile); err != nil {
			t.Fatalf("preflightWindowsSelfBinaryWrite() = %v, want nil on linux", err)
		}
	})

	t.Run("non-self-tool returns nil without checking", func(t *testing.T) {
		execCommand = mockGoEnvError()
		lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }

		otherTool := update.ToolInfo{Name: "engram"}
		profile := system.PlatformProfile{OS: "windows"}
		if _, err := preflightWindowsSelfBinaryWrite(otherTool, profile); err != nil {
			t.Fatalf("preflightWindowsSelfBinaryWrite() = %v, want nil for non-self-tool", err)
		}
	})
}

// TestInstallSourceBuildIsDistinctEnumValue pins the enum membership (F4).
func TestInstallSourceBuildIsDistinctEnumValue(t *testing.T) {
	if update.InstallSourceBuild == update.InstallGoInstall {
		t.Error("InstallSourceBuild must differ from InstallGoInstall")
	}
	if update.InstallSourceBuild == update.InstallBinary {
		t.Error("InstallSourceBuild must differ from InstallBinary")
	}
	if string(update.InstallSourceBuild) != "source-build" {
		t.Errorf("InstallSourceBuild = %q, want %q", string(update.InstallSourceBuild), "source-build")
	}
}
