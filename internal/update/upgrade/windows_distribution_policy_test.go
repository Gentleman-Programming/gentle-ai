package upgrade

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
	"github.com/gentleman-programming/gentle-ai/v5/internal/update"
)

func TestWindowsBetaSourceRecoveryIsPinned(t *testing.T) {
	tool := update.ToolInfo{Name: "gentle-ai", Owner: "Gentleman-Programming", Repo: "gentle-ai"}
	sha := "972997650b51abcdef0123456789abcdef012345"
	result := update.UpdateResult{Tool: tool, LatestVersion: "main@" + sha[:12], BetaCommit: sha, BetaModulePath: "github.com/gentleman-programming/gentle-ai/v5"}
	hint := gentleAIWindowsSourceInstallHint(result)
	want := "go install github.com/gentleman-programming/gentle-ai/v5/cmd/gentle-ai@" + sha
	if !strings.Contains(hint, want) || strings.Contains(hint, "@main") {
		t.Fatalf("Windows recovery command not pinned: %s", hint)
	}
	result.BetaModulePath = ""
	if hint := gentleAIWindowsSourceInstallHint(result); strings.Contains(hint, "go install") {
		t.Fatalf("unverified Windows recovery command: %s", hint)
	}
}

func TestWindowsBetaGentleAIUpgradeUsesShippedRegistryGoTarget(t *testing.T) {
	const (
		mainSHA = "972997650b51abcdef0123456789abcdef012345"
		module  = "github.com/gentleman-programming/gentle-ai/v5"
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

	r := update.UpdateResult{Tool: tool, LatestVersion: "main@" + mainSHA[:12], BetaCommit: mainSHA, BetaModulePath: module, Status: update.UpdateAvailable}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true, Supported: true}
	if _, err := runStrategy(context.Background(), r, profile); err != nil {
		t.Fatalf("runStrategy beta Windows self-upgrade: %v", err)
	}

	wantTarget := module + "/cmd/gentle-ai@" + mainSHA
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
