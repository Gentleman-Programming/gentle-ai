package upgrade

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

// TestSourceBuildUpgradeFailuresReturnManualFallback verifies that every
// failure before the atomic replace returns a ManualFallbackError with zero
// files touched (T-2, T-7).
func TestSourceBuildUpgradeFailuresReturnManualFallback(t *testing.T) {
	origLookPathCommand := lookPathCommand
	t.Cleanup(func() { lookPathCommand = origLookPathCommand })

	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })
	lookPathFn = func(string) (string, error) { return t.TempDir() + `\axiom.exe`, nil }

	origExec := execCommand
	t.Cleanup(func() { execCommand = origExec })
	origMkdirTemp := sourceBuildMkdirTemp
	t.Cleanup(func() { sourceBuildMkdirTemp = origMkdirTemp })

	tool := update.ToolInfo{
		Name:         "axiom",
		Owner:        "IGutierrezZ",
		Repo:         "axiom",
		GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom",
		GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
	}
	r := update.UpdateResult{Tool: tool, LatestVersion: "9.9.9", Status: update.UpdateAvailable}
	profile := system.PlatformProfile{OS: "windows", GoAvailable: true}

	t.Run("git absent returns ManualFallbackError without touching anything", func(t *testing.T) {
		lookPathCommand = func(name string) (string, error) {
			if name == "git" {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + name, nil
		}
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "go" && len(args) >= 1 && args[0] == "env" {
				return mockCmd("echo", "")
			}
			t.Fatalf("git-absent path executed %s %v", name, args)
			return nil
		}
		err := sourceBuildUpgrade(context.Background(), r, profile, "refs/tags/v9.9.9")
		if err == nil {
			t.Fatal("expected ManualFallbackError when git is absent")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
		if !strings.Contains(err.Error(), "IGutierrezZ/axiom") {
			t.Errorf("hint must name the fork: %s", err.Error())
		}
	})

	t.Run("go absent returns ManualFallbackError", func(t *testing.T) {
		lookPathCommand = func(name string) (string, error) {
			if name == "go" {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + name, nil
		}
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "go" && len(args) >= 1 && args[0] == "env" {
				return mockCmd("echo", "")
			}
			t.Fatalf("go-absent path executed %s %v", name, args)
			return nil
		}
		err := sourceBuildUpgrade(context.Background(), r, profile, "refs/tags/v9.9.9")
		if err == nil {
			t.Fatal("expected ManualFallbackError when go is absent")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
	})

	t.Run("go build failure returns ManualFallbackError without half-built binary", func(t *testing.T) {
		lookPathCommand = func(name string) (string, error) { return "/usr/bin/" + name, nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "go" && len(args) >= 1 && args[0] == "env" {
				return mockCmd("echo", "")
			}
			if name == "git" {
				return mockCmd("true")
			}
			if name == "go" && len(args) >= 1 && args[0] == "build" {
				return mockCmd("false")
			}
			t.Fatalf("unexpected command %s %v", name, args)
			return nil
		}
		err := sourceBuildUpgrade(context.Background(), r, profile, "refs/tags/v9.9.9")
		if err == nil {
			t.Fatal("expected ManualFallbackError when go build fails")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
	})

	t.Run("context cancelled returns ManualFallbackError", func(t *testing.T) {
		lookPathCommand = func(name string) (string, error) { return "/usr/bin/" + name, nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "go" && len(args) >= 1 && args[0] == "env" {
				return mockCmd("echo", "")
			}
			t.Fatalf("cancelled context executed %s %v", name, args)
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := sourceBuildUpgrade(ctx, r, profile, "refs/tags/v9.9.9")
		if err == nil {
			t.Fatal("expected ManualFallbackError when context is cancelled")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
	})

	t.Run("git fetch failure returns ManualFallbackError without residue", func(t *testing.T) {
		lookPathCommand = func(name string) (string, error) { return "/usr/bin/" + name, nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "go" && len(args) >= 1 && args[0] == "env" {
				return mockCmd("echo", "")
			}
			if name == "git" && len(args) >= 1 && args[0] == "init" {
				return mockCmd("true")
			}
			if name == "git" && len(args) >= 1 && args[0] == "-C" {
				for _, a := range args {
					if a == "fetch" {
						return mockCmd("false") // tag v9.9.9 does not exist
					}
				}
				return mockCmd("true")
			}
			t.Fatalf("unexpected command %s %v", name, args)
			return nil
		}
		sourceBuildMkdirTemp = func() (string, error) { return t.TempDir(), nil }
		err := sourceBuildUpgrade(context.Background(), r, profile, "refs/tags/v9.9.9")
		if err == nil {
			t.Fatal("expected ManualFallbackError when git fetch fails")
		}
		if _, ok := AsManualFallback(err); !ok {
			t.Fatalf("error = %T %v, want ManualFallbackError", err, err)
		}
	})
}
