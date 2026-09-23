package upgrade

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

func TestGentleAIWindowsUpgradeFailsClosedToSourceInstall(t *testing.T) {
	originalExec := execCommand
	t.Cleanup(func() { execCommand = originalExec })
	originalLookPathCommand := lookPathCommand
	t.Cleanup(func() { lookPathCommand = originalLookPathCommand })
	lookPathCommand = func(string) (string, error) { return "", exec.ErrNotFound }

	originalLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = originalLookPath })
	lookPathFn = func(string) (string, error) { return t.TempDir() + `\axiom.exe`, nil }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "go" && len(args) == 2 && args[0] == "env" {
			return mockCmd("echo", "")
		}
		t.Fatalf("source-build fallback executed %s %v", name, args)
		return nil
	}

	tests := []struct {
		name          string
		latestVersion string
		wantTarget    string
		wantInstall   string
	}{
		{
			name:          "stable release",
			latestVersion: "2.2.0",
			wantTarget:    "v2.2.0",
			// No declared GoModulePath: REQ-22.2 forbids emitting a go install
			// the toolchain cannot resolve, so the manual way degrades to
			// clone-and-build (D-04) and must stay off any releases page.
			wantInstall: "git clone https://github.com/Gentleman-Programming/gentle-ai && cd gentle-ai && go build -o gentle-ai ./cmd/gentle-ai",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := update.UpdateResult{
				Tool: update.ToolInfo{
					Name:          "gentle-ai",
					Owner:         "Gentleman-Programming",
					Repo:          "gentle-ai",
					InstallMethod: update.InstallBinary,
				},
				LatestVersion: tc.latestVersion,
				Status:        update.UpdateAvailable,
			}
			profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", Supported: true, GoAvailable: true}

			exitRequested, err := runStrategy(context.Background(), r, profile)
			if exitRequested {
				t.Fatal("Windows omission policy requested process exit")
			}
			hint, ok := AsManualFallback(err)
			if !ok {
				t.Fatalf("runStrategy error = %T %v, want ManualFallbackError", err, err)
			}
			for _, required := range []string{
				tc.wantInstall,
			} {
				if !strings.Contains(hint, required) {
					t.Errorf("manual hint is missing %q: %s", required, hint)
				}
			}
			if strings.Contains(hint, "go install") {
				t.Fatalf("manual hint must not offer an unresolvable go install: %s", hint)
			}
			if strings.Contains(hint, "/releases") || strings.Contains(hint, "install.ps1") {
				t.Fatalf("manual hint recommends forbidden Windows distribution: %s", hint)
			}

			result := executeOne(context.Background(), r, profile, false)
			if result.Status != UpgradeSkipped || result.Err != nil || result.ExitRequested || result.Method != update.InstallSourceBuild {
				t.Fatalf("executeOne result = %#v, want non-error source-build skip", result)
			}
			if !strings.Contains(result.ManualHint, "Gentleman-Programming/gentle-ai") {
				t.Fatalf("executeOne manual hint = %q, want fork/repo reference", result.ManualHint)
			}
		})
	}
}

func TestWindowsBetaGentleAIUpgradeUsesShippedRegistryGoTarget(t *testing.T) {
	const (
		mainSHA = "abc1234"
		module  = "github.com/gentleman-programming/gentle-ai/v3"
	)

	var tool update.ToolInfo
	for _, candidate := range update.Tools {
		if candidate.Name == "gentle-ai" {
			tool = candidate
			break
		}
	}
	if tool.GoImportPath == "" {
		t.Fatal("shipped gentle-ai registry entry must declare GoImportPath")
	}

	gobin := t.TempDir()
	destination := writeFakeBinary(t, gobin, "gentle-ai.exe")
	originalLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = originalLookPath })
	lookPathFn = func(string) (string, error) { return destination, nil }

	originalExec := execCommand
	t.Cleanup(func() { execCommand = originalExec })
	var gotName string
	var gotArgs []string
	var gotCmd *exec.Cmd
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "go" && len(args) == 2 && args[0] == "env" {
			return mockCmd("echo", gobin)
		}
		gotName = name
		gotArgs = args
		gotCmd = mockCmd("true")
		return gotCmd
	}

	r := update.UpdateResult{Tool: tool, LatestVersion: "main@" + mainSHA, Status: update.UpdateAvailable}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true, Supported: true}
	if _, err := runStrategy(context.Background(), r, profile); err != nil {
		t.Fatalf("runStrategy beta Windows self-upgrade: %v", err)
	}

	wantTarget := tool.GoImportPath + "@main"
	if gotName != "go" || len(gotArgs) != 2 || gotArgs[0] != "install" || gotArgs[1] != wantTarget {
		t.Fatalf("go command = %q %v, want go install %s", gotName, gotArgs, wantTarget)
	}
	for _, want := range []string{
		"GONOSUMDB=" + module,
		"GOPRIVATE=" + module,
		"GONOPROXY=" + module,
	} {
		if gotCmd == nil || !envContains(gotCmd.Env, want) {
			t.Fatalf("go install env missing %q in %v", want, gotCmd.Env)
		}
	}
}
