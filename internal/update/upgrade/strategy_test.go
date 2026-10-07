package upgrade

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/testenv"
	"github.com/gentleman-programming/gentle-ai/v4/internal/update"
)

func TestMain(m *testing.M) {
	// Neutralize ambient agent runtime-dir overrides (PI_CODING_AGENT_DIR,
	// OPENCODE_CONFIG_DIR) up front: this package's executor tests resolve
	// Pi and OpenCode config paths through the real adapters/internal/opencode
	// package, both of which honor these overrides directly from the
	// environment. Individual tests keep their own explicit t.Setenv("", "")
	// resets, which still work fine on top of this baseline.
	testenv.Isolate()
	// Existing upgrade fixtures explicitly represent V1, never the ambient CLI.
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
	if err := os.Unsetenv("GENTLE_AI_CHANNEL"); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

// --- TestRunStrategy_BrewUpgrade ---

func TestRunStrategy_BrewUpgrade(t *testing.T) {
	mockHomebrewOwnership(t, update.HomebrewFormula)
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = args
		return mockCmd("echo", "Upgraded engram")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallBrew,
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy brew: unexpected error: %v", err)
	}

	if gotName != "brew" {
		t.Errorf("exec name = %q, want %q", gotName, "brew")
	}
	if len(gotArgs) < 3 || gotArgs[0] != "upgrade" || gotArgs[1] != "--formula" || gotArgs[2] != "engram" {
		t.Errorf("exec args = %v, want [upgrade --formula engram]", gotArgs)
	}
}

// --- TestRunStrategy_GoInstallUpgrade ---

func TestRunStrategy_GoInstallUpgrade(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = args
		return mockCmd("echo", "go install ok")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallGoInstall,
			GoImportPath:  "github.com/Gentleman-Programming/engram/cmd/engram",
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy go-install: unexpected error: %v", err)
	}

	if gotName != "go" {
		t.Errorf("exec name = %q, want %q", gotName, "go")
	}
	// Expected: go install github.com/Gentleman-Programming/engram/cmd/engram@v0.4.0
	wantArg0, wantArg1 := "install", "github.com/Gentleman-Programming/engram/cmd/engram@v0.4.0"
	if len(gotArgs) < 2 || gotArgs[0] != wantArg0 || gotArgs[1] != wantArg1 {
		t.Errorf("exec args = %v, want [%s %s]", gotArgs, wantArg0, wantArg1)
	}
}

// TestRunStrategy_GoInstallUpgradeCrossMajorDerivesSuffixFromVersion is the
// acceptance scenario for issue #4687: a stable-channel upgrade target whose
// major differs from the running binary's major must compose a /vN suffix
// matching the TARGET version, not the running binary. This is what makes a
// cross-major upgrade resolvable: a v3 binary targeting v4.0.0 composes
// .../v4/... and a v3 binary targeting v2.0.0 composes .../v2/....
//
// The composition happens inside goInstallUpgrade, which is reached from
// runStrategy only on a Windows profile with Go on PATH and a declared
// GoImportPath — gentleAISelfUpgradeMethod routes gentle-ai on Linux and
// macOS to InstallBinary (the minisign-verified release download) and the
// beta channel bypasses this path through goInstallMainUpgrade. The previous
// rewrite used a Linux profile and never reached the composition: the
// routing went straight to binaryUpgrade and the test failed on
// ErrReleaseTrustUnavailable. The preflight gate inside goInstallUpgrade is
// satisfied the same way as
// TestWindowsBetaGentleAIUpgradeUsesShippedRegistryGoTarget: a fake binary
// is written into a temp GOBIN, lookPathFn resolves "gentle-ai" to that
// path, and execCommand hands a synthetic GOBIN back to goInstallDestinationDir.
func TestRunStrategy_GoInstallUpgradeCrossMajorDerivesSuffixFromVersion(t *testing.T) {
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

	tests := []struct {
		name       string
		latestVer  string
		wantTarget string
	}{
		{
			name:       "v3 binary targeting v4.0.0 composes /v4",
			latestVer:  "4.0.0",
			wantTarget: "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0",
		},
		{
			name:       "v3 binary targeting v2.0.0 composes /v2",
			latestVer:  "2.0.0",
			wantTarget: "github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.0.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gobin := t.TempDir()
			destination := writeFakeBinary(t, gobin, "gentle-ai.exe")

			origLookPath := lookPathFn
			t.Cleanup(func() { lookPathFn = origLookPath })
			lookPathFn = func(string) (string, error) { return destination, nil }

			origExec := execCommand
			t.Cleanup(func() { execCommand = origExec })

			var gotName string
			var gotArgs []string
			execCommand = func(name string, args ...string) *exec.Cmd {
				if name == "go" && len(args) == 2 && args[0] == "env" {
					return mockCmd("echo", gobin)
				}
				gotName = name
				gotArgs = args
				return mockCmd("true")
			}

			r := update.UpdateResult{
				Tool:          tool,
				LatestVersion: tc.latestVer,
				Status:        update.UpdateAvailable,
			}
			profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true, Supported: true}

			if _, err := runStrategy(context.Background(), r, profile); err != nil {
				t.Fatalf("runStrategy: %v", err)
			}

			wantArgs := []string{"install", tc.wantTarget}
			if gotName != "go" || len(gotArgs) != len(wantArgs) || gotArgs[0] != wantArgs[0] || gotArgs[1] != wantArgs[1] {
				t.Fatalf("exec command = %q %v, want %v", gotName, gotArgs, wantArgs)
			}
		})
	}
}

func TestRunStrategy_BetaGentleAISelfUpgradeUsesGoInstallMain(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var gotName string
	var gotArgs []string
	var gotCmd *exec.Cmd
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = args
		gotCmd = mockCmd("true")
		return gotCmd
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gentle-ai",
			Owner:         "Gentleman-Programming",
			Repo:          "gentle-ai",
			InstallMethod: update.InstallBinary,
		},
		LatestVersion:  "main@972997650b51",
		BetaCommit:     "972997650b51abcdef0123456789abcdef012345",
		BetaModulePath: "github.com/gentleman-programming/gentle-ai/v4",
		Status:         update.UpdateAvailable,
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt", Supported: true}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy beta gentle-ai: unexpected error: %v", err)
	}

	if gotName != "go" {
		t.Fatalf("exec name = %q, want %q", gotName, "go")
	}
	wantArgs := []string{"install", "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@972997650b51abcdef0123456789abcdef012345"}
	if len(gotArgs) != len(wantArgs) || gotArgs[0] != wantArgs[0] || gotArgs[1] != wantArgs[1] {
		t.Fatalf("exec args = %v, want %v", gotArgs, wantArgs)
	}
	for _, want := range []string{
		"GONOSUMDB=github.com/gentleman-programming/gentle-ai/v4",
		"GOPRIVATE=github.com/gentleman-programming/gentle-ai/v4",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai/v4",
	} {
		if !envContains(gotCmd.Env, want) {
			t.Fatalf("go install env missing %q in %v", want, gotCmd.Env)
		}
	}
}

func TestBetaInstallRejectsUnverifiedTargetBeforeMutation(t *testing.T) {
	origExec := execCommand
	t.Cleanup(func() { execCommand = origExec })
	called := false
	execCommand = func(name string, args ...string) *exec.Cmd { called = true; return mockCmd("true") }
	tool := update.ToolInfo{Name: "gentle-ai", Owner: "Gentleman-Programming", Repo: "gentle-ai", InstallMethod: update.InstallBinary}
	cases := []struct{ name, sha, module, version string }{
		{name: "no metadata", version: "main@972997650b51"},
		{name: "wrong module", sha: "972997650b51abcdef0123456789abcdef012345", module: "github.com/other/gentle-ai/v4", version: "main@972997650b51"},
		{name: "mismatched short revision", sha: "972997650b51abcdef0123456789abcdef012345", module: "github.com/gentleman-programming/gentle-ai/v4", version: "main@6eff4a1ba110"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := update.UpdateResult{Tool: tool, LatestVersion: tc.version, BetaCommit: tc.sha, BetaModulePath: tc.module, Status: update.UpdateAvailable}
			_, err := runStrategy(context.Background(), r, system.PlatformProfile{OS: "linux", PackageManager: "apt"})
			if err == nil || called {
				t.Fatalf("unverified target should not execute: err=%v, called=%v", err, called)
			}
		})
	}
}

func envContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func TestGoProxyBypassEnvPreservesExistingPatterns(t *testing.T) {
	module := "github.com/gentleman-programming/gentle-ai/v3"
	env := goProxyBypassEnv([]string{
		"PATH=/usr/bin",
		"GONOSUMDB=example.com/private",
		"GOPRIVATE=github.com/acme/*",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai/v3",
	}, module)

	for _, want := range []string{
		"PATH=/usr/bin",
		"GONOSUMDB=github.com/gentleman-programming/gentle-ai/v3,example.com/private",
		"GOPRIVATE=github.com/gentleman-programming/gentle-ai/v3,github.com/acme/*",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai/v3",
	} {
		if !envContains(env, want) {
			t.Fatalf("env missing %q in %v", want, env)
		}
	}
}

// --- TestRunStrategy_GoInstallMissingImportPath ---

func TestRunStrategy_GoInstallMissingImportPath(t *testing.T) {
	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallGoInstall,
			GoImportPath:  "", // missing
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	_, err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when GoImportPath is empty, got nil")
	}
}

// --- TestRunStrategy_UnsupportedMethodManualFallback ---

func TestRunStrategy_UnsupportedMethodManualFallback(t *testing.T) {
	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "some-tool",
			InstallMethod: update.InstallMethod("unsupported-method"),
		},
		LatestVersion: "1.0.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	_, err := runStrategy(context.Background(), r, profile)
	// Unsupported method → manual fallback error.
	if err == nil {
		t.Errorf("expected error for unsupported install method, got nil")
	}
}

// --- TestRunStrategy_BrewUpgradeFailure ---

func TestRunStrategy_BrewUpgradeFailure(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		return mockCmd("false") // always fails
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallBrew,
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	_, err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when brew upgrade fails, got nil")
	}
}

// --- TestRunStrategy_GoInstallFailure ---

func TestRunStrategy_GoInstallFailure(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		return mockCmd("false")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallGoInstall,
			GoImportPath:  "github.com/Gentleman-Programming/engram/cmd/engram",
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	_, err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when go install fails, got nil")
	}
}

// TestEffectiveMethodGentleAIOnWindowsUsesFailClosedBinaryPolicy verifies that
// Windows never routes gentle-ai through a remote installer, and that when no
// usable `go install` target is declared it falls back to the binary strategy —
// which on Windows is an explicit refusal naming a runnable source-install
// command, not a download.
func TestEffectiveMethodGentleAIOnWindowsUsesFailClosedBinaryPolicy(t *testing.T) {
	tests := []struct {
		name string
		tool update.ToolInfo
	}{
		{
			name: "binary remains policy boundary",
			tool: update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary},
		},
		{
			name: "legacy script declaration is disabled",
			tool: update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallScript},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true}
			method := effectiveMethod(tc.tool, profile)
			if method != update.InstallBinary {
				t.Errorf("effectiveMethod(%q) = %q, want %q", tc.tool.Name, method, update.InstallBinary)
			}
		})
	}

	// Renamed from "Go availability still requires an explicit source install",
	// which encoded the previous policy: Windows refused to self-upgrade even
	// with Go on PATH. That policy has been changed deliberately. No signed
	// Windows binary is published, so there is no asset to download and verify
	// with minisign; a pinned `go install <importPath>@vX.Y.Z` — still checked
	// against the Go checksum database, since goInstallUpgrade does not touch
	// cmd.Env — is the only automatic upgrade path Windows has.
	t.Run("Go availability upgrades through a pinned go install", func(t *testing.T) {
		tool := update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary, GoImportPath: "github.com/Gentleman-Programming/gentle-ai/v3/cmd/gentle-ai"}
		profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true}
		method := effectiveMethod(tool, profile)
		if method != update.InstallGoInstall {
			t.Errorf("effectiveMethod(%q) = %q, want %q", tool.Name, method, update.InstallGoInstall)
		}
	})
}

// --- TestEffectiveMethod_NonGentleAIToolsOnWindowsUseBinary ---

// TestEffectiveMethod_NonGentleAIToolsOnWindowsUseBinary verifies that tools
// OTHER than gentle-ai on Windows still use their declared install method
// (binary, script, etc.).
func TestEffectiveMethod_NonGentleAIToolsOnWindowsUseBinary(t *testing.T) {
	tests := []struct {
		name string
		tool update.ToolInfo
		want update.InstallMethod
	}{
		{
			name: "engram uses binary",
			tool: update.ToolInfo{Name: "engram", InstallMethod: update.InstallBinary},
			want: update.InstallBinary,
		},
		{
			name: "gga uses script",
			tool: update.ToolInfo{Name: "gga", InstallMethod: update.InstallScript},
			want: update.InstallScript,
		},
		{
			name: "unknown tool uses binary",
			tool: update.ToolInfo{Name: "other", InstallMethod: update.InstallBinary},
			want: update.InstallBinary,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true}
			method := effectiveMethod(tc.tool, profile)
			if method != tc.want {
				t.Errorf("effectiveMethod(%q) = %q, want %q", tc.tool.Name, method, tc.want)
			}
		})
	}
}

// --- TestEffectiveMethod ---

func TestEffectiveMethod(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })

	tests := []struct {
		name          string
		tool          update.ToolInfo
		profile       system.PlatformProfile
		brewInstalled bool
		want          update.InstallMethod
	}{
		{
			name:          "brew-owned package overrides go-install",
			tool:          update.ToolInfo{Name: "engram", InstallMethod: update.InstallGoInstall},
			profile:       system.PlatformProfile{PackageManager: "brew"},
			brewInstalled: true,
			want:          update.InstallBrew,
		},
		{
			name:          "brew-owned package overrides binary",
			tool:          update.ToolInfo{Name: "gga", InstallMethod: update.InstallBinary},
			profile:       system.PlatformProfile{PackageManager: "brew"},
			brewInstalled: true,
			want:          update.InstallBrew,
		},
		{
			name:          "brew-owned package overrides script",
			tool:          update.ToolInfo{Name: "gga", InstallMethod: update.InstallScript},
			profile:       system.PlatformProfile{PackageManager: "brew"},
			brewInstalled: true,
			want:          update.InstallBrew,
		},
		{
			name:    "brew profile without package ownership respects declared method",
			tool:    update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary},
			profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew"},
			want:    update.InstallBinary,
		},
		{
			name:    "brew profile without package ownership can use go-install fallback",
			tool:    update.ToolInfo{Name: "mytool", InstallMethod: update.InstallBinary, GoImportPath: "github.com/example/mytool/cmd/mytool"},
			profile: system.PlatformProfile{PackageManager: "brew", GoAvailable: true},
			want:    update.InstallGoInstall,
		},
		{
			name:    "apt profile respects declared method (go-install)",
			tool:    update.ToolInfo{Name: "engram", InstallMethod: update.InstallGoInstall},
			profile: system.PlatformProfile{PackageManager: "apt"},
			want:    update.InstallGoInstall,
		},
		{
			name:    "apt profile respects declared method (binary)",
			tool:    update.ToolInfo{Name: "gga", InstallMethod: update.InstallBinary},
			profile: system.PlatformProfile{PackageManager: "apt"},
			want:    update.InstallBinary,
		},
		{
			name:    "apt profile respects declared method (script)",
			tool:    update.ToolInfo{Name: "gga", InstallMethod: update.InstallScript},
			profile: system.PlatformProfile{PackageManager: "apt"},
			want:    update.InstallScript,
		},
		// Auto-detect order: brew-owned package → go-install → binary (issue #246).
		{
			name:          "auto-detect: brew-owned package wins regardless of GoImportPath",
			tool:          update.ToolInfo{Name: "mytool", InstallMethod: update.InstallBinary, GoImportPath: "github.com/example/mytool/cmd/mytool"},
			profile:       system.PlatformProfile{PackageManager: "brew", GoAvailable: true},
			brewInstalled: true,
			want:          update.InstallBrew,
		},
		{
			name:    "auto-detect: brew missing + go available + GoImportPath set → go-install",
			tool:    update.ToolInfo{Name: "mytool", InstallMethod: update.InstallBinary, GoImportPath: "github.com/example/mytool/cmd/mytool"},
			profile: system.PlatformProfile{PackageManager: "apt", GoAvailable: true},
			want:    update.InstallGoInstall,
		},
		{
			name:    "auto-detect: brew missing + go missing + GoImportPath set → binary fallback",
			tool:    update.ToolInfo{Name: "mytool", InstallMethod: update.InstallBinary, GoImportPath: "github.com/example/mytool/cmd/mytool"},
			profile: system.PlatformProfile{PackageManager: "apt", GoAvailable: false},
			want:    update.InstallBinary,
		},
		{
			name:    "auto-detect: go available but GoImportPath empty → binary (no upgrade)",
			tool:    update.ToolInfo{Name: "mytool", InstallMethod: update.InstallBinary, GoImportPath: ""},
			profile: system.PlatformProfile{PackageManager: "apt", GoAvailable: true},
			want:    update.InstallBinary,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			homebrewPackageInstalled = func(toolName string) bool {
				return toolName == tc.tool.Name && tc.brewInstalled
			}

			got := effectiveMethod(tc.tool, tc.profile)
			if got != tc.want {
				t.Errorf("effectiveMethod = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHomebrewPackageInstalledWithRequiresActiveBrewPath(t *testing.T) {
	brewPrefix := filepath.Join(t.TempDir(), "opt", "gentle-ai")
	brewBin := filepath.Join(brewPrefix, "bin", "gentle-ai")
	nonBrewBin := filepath.Join(t.TempDir(), "gentle-ai")

	run := func(name string, args ...string) *exec.Cmd {
		if name != "brew" {
			return mockCmd("false")
		}
		if len(args) >= 3 && args[0] == "list" && args[1] == "--formula" && args[2] == "gentle-ai" {
			return mockCmd("true")
		}
		if len(args) == 2 && args[0] == "--prefix" && args[1] == "gentle-ai" {
			return mockCmd("echo", brewPrefix)
		}
		return mockCmd("false")
	}

	if !homebrewPackageInstalledWith(run, func(string) (string, error) { return brewBin, nil }, "gentle-ai") {
		t.Fatal("expected brew-owned active path to be treated as Homebrew installed")
	}
	if homebrewPackageInstalledWith(run, func(string) (string, error) { return nonBrewBin, nil }, "gentle-ai") {
		t.Fatal("expected shadowing non-brew active path to avoid Homebrew")
	}
	if homebrewPackageInstalledWith(func(string, ...string) *exec.Cmd { return mockCmd("false") }, func(string) (string, error) { return brewBin, nil }, "gentle-ai") {
		t.Fatal("expected brew list failure to avoid Homebrew")
	}
	if homebrewPackageInstalledWith(func(string, ...string) *exec.Cmd { return mockCmd("true") }, func(string) (string, error) { return "", errors.New("not found") }, "gentle-ai") {
		t.Fatal("expected active path lookup failure to avoid Homebrew")
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// --- TestBrewUpgrade_RunsUpdateBeforeUpgrade ---

// TestBrewUpgrade_RunsUpdateBeforeUpgrade verifies that brewUpgrade calls
// `brew update` BEFORE `brew upgrade <toolName>`, and that the order is correct.
func TestBrewUpgrade_RunsUpdateBeforeUpgrade(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var callOrder []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "brew" && len(args) > 0 {
			callOrder = append(callOrder, args[0]) // "update" or "upgrade"
		}
		return mockCmd("echo", "ok")
	}

	err := brewUpgrade(context.Background(), update.UpdateResult{Tool: update.ToolInfo{Name: "gentle-ai"}}, update.HomebrewFormula)
	if err != nil {
		t.Fatalf("brewUpgrade: unexpected error: %v", err)
	}

	// Must have called brew tap, scoped trust, brew update AND brew upgrade — in that order.
	if len(callOrder) < 4 {
		t.Fatalf("expected 4 brew calls (tap, trust, update, upgrade), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[1] != "trust" {
		t.Errorf("second brew call = %q, want %q", callOrder[1], "trust")
	}
	if callOrder[2] != "update" {
		t.Errorf("third brew call = %q, want %q", callOrder[2], "update")
	}
	if callOrder[3] != "upgrade" {
		t.Errorf("fourth brew call = %q, want %q", callOrder[3], "upgrade")
	}
}

// --- TestBrewUpgrade_UpdateFailureIsNonFatal ---

// TestBrewUpgrade_UpdateFailureIsNonFatal verifies that when `brew update` fails
// but `brew upgrade` succeeds, the overall result is success (non-fatal update failure).
func TestBrewUpgrade_UpdateFailureIsNonFatal(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var callArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "brew" && len(args) > 0 {
			callArgs = append(callArgs, args[0])
			if args[0] == "update" {
				// brew update fails (e.g. no network).
				return mockCmd("false")
			}
		}
		// brew upgrade succeeds.
		return mockCmd("echo", "Upgraded gentle-ai")
	}

	err := brewUpgrade(context.Background(), update.UpdateResult{Tool: update.ToolInfo{Name: "gentle-ai"}}, update.HomebrewFormula)
	// brew update failed but brew upgrade succeeded → overall success.
	if err != nil {
		t.Errorf("expected success when brew update fails but brew upgrade succeeds, got: %v", err)
	}

	// Brew trust, update, and upgrade must have been called (after the tap).
	if len(callArgs) < 4 {
		t.Fatalf("expected 4 brew calls, got %d: %v", len(callArgs), callArgs)
	}
	if callArgs[1] != "trust" {
		t.Errorf("second brew call = %q, want %q", callArgs[1], "trust")
	}
	if callArgs[2] != "update" {
		t.Errorf("third brew call = %q, want %q", callArgs[2], "update")
	}
	if callArgs[3] != "upgrade" {
		t.Errorf("fourth brew call = %q, want %q", callArgs[3], "upgrade")
	}
}

// --- TestBrewUpgrade_TapsBeforeUpdateAndUpgrade ---

// TestBrewUpgrade_TapsAndTrustsBeforeUpdateAndUpgrade verifies that brewUpgrade calls
// `brew tap Gentleman-Programming/homebrew-tap` and scoped artifact trust BEFORE
// `brew update` and `brew upgrade <toolName>`. This makes the upgrade idempotent
// when a user has lost the tap and works with Homebrew tap trust enforcement.
func TestBrewUpgrade_TapsAndTrustsBeforeUpdateAndUpgrade(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	type call struct {
		subcommand string
		args       []string
	}
	var calls []call
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "brew" && len(args) > 0 {
			c := call{subcommand: args[0], args: append([]string(nil), args[1:]...)}
			calls = append(calls, c)
		}
		if name == "engram" {
			return mockCmd("echo", "engram 1.2.3")
		}
		return mockCmd("echo", "ok")
	}

	if err := brewUpgrade(context.Background(), update.UpdateResult{Tool: update.ToolInfo{Name: "engram", DetectCmd: []string{"engram", "version"}}, LatestVersion: "1.2.3"}, update.HomebrewCask); err != nil {
		t.Fatalf("brewUpgrade: unexpected error: %v", err)
	}

	if len(calls) < 4 {
		t.Fatalf("expected 4 brew calls (tap, trust, update, upgrade), got %d: %+v", len(calls), calls)
	}
	if calls[0].subcommand != "tap" {
		t.Errorf("first brew call subcommand = %q, want %q", calls[0].subcommand, "tap")
	}
	if len(calls[0].args) != 1 || calls[0].args[0] != "Gentleman-Programming/homebrew-tap" {
		t.Errorf("first brew call args = %v, want [Gentleman-Programming/homebrew-tap]", calls[0].args)
	}
	if calls[1].subcommand != "trust" {
		t.Errorf("second brew call = %q, want %q", calls[1].subcommand, "trust")
	}
	if len(calls[1].args) != 2 || calls[1].args[0] != "--cask" || calls[1].args[1] != "gentleman-programming/tap/engram" {
		t.Errorf("second brew call args = %v, want [--cask gentleman-programming/tap/engram]", calls[1].args)
	}
	if calls[2].subcommand != "update" {
		t.Errorf("third brew call = %q, want %q", calls[2].subcommand, "update")
	}
	if calls[3].subcommand != "upgrade" {
		t.Errorf("fourth brew call = %q, want %q", calls[3].subcommand, "upgrade")
	}
}

func TestBrewUpgrade_FormulaToolUsesFormulaTrust(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var trustArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "brew" && len(args) > 0 && args[0] == "trust" {
			trustArgs = append([]string(nil), args[1:]...)
		}
		return mockCmd("echo", "ok")
	}

	if err := brewUpgrade(context.Background(), update.UpdateResult{Tool: update.ToolInfo{Name: "gentle-ai"}}, update.HomebrewFormula); err != nil {
		t.Fatalf("brewUpgrade: unexpected error: %v", err)
	}

	if len(trustArgs) != 2 || trustArgs[0] != "--formula" || trustArgs[1] != "gentleman-programming/tap/gentle-ai" {
		t.Fatalf("brew trust args = %v, want [--formula gentleman-programming/tap/gentle-ai]", trustArgs)
	}
}

func TestHomebrewFailureAdviceTapTrust(t *testing.T) {
	output := `Error: Refusing to load formula gentleman-programming/tap/gentle-ai from untrusted tap.
Run brew trust --formula gentleman-programming/tap/gentle-ai to trust it.`
	advice := homebrewFailureAdvice("gentle-ai", output)
	for _, want := range []string{
		"brew trust --formula gentleman-programming/tap/gentle-ai",
		"brew upgrade --formula gentle-ai",
	} {
		if !strings.Contains(advice, want) {
			t.Fatalf("tap trust advice missing %q:\n%s", want, advice)
		}
	}
}

func TestHomebrewFailureAdviceCaskTapTrust(t *testing.T) {
	output := `Error: Refusing to load cask gentleman-programming/tap/engram from untrusted tap.
Run brew trust --cask gentleman-programming/tap/engram to trust it.`
	advice := homebrewFailureAdvice("engram", output)
	for _, want := range []string{
		"brew trust --cask gentleman-programming/tap/engram",
		"brew upgrade --cask engram",
	} {
		if !strings.Contains(advice, want) {
			t.Fatalf("cask tap trust advice missing %q:\n%s", want, advice)
		}
	}
	if strings.Contains(advice, "--formula") {
		t.Fatalf("cask tap trust advice must not suggest --formula:\n%s", advice)
	}
}

func TestHomebrewFailureAdviceBubblewrap(t *testing.T) {
	output := `Error: Bubblewrap is installed but cannot create a rootless sandbox.
Homebrew's Linux sandbox requires rootless Bubblewrap and unprivileged user namespaces.`
	advice := homebrewFailureAdvice("gentle-ai", output)
	if strings.Contains(strings.ToLower(advice), "preferred fix") {
		t.Fatalf("bubblewrap advice must not frame host policy changes as preferred defaults:\n%s", advice)
	}
	for _, want := range []string{
		"explicit admin/security decision",
		"sudo sysctl -w kernel.unprivileged_userns_clone=1",
		"sudo sysctl -w user.max_user_namespaces=28633",
		"sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0 || true",
		"HOMEBREW_NO_SANDBOX_LINUX=1 brew upgrade --formula gentle-ai",
	} {
		if !strings.Contains(advice, want) {
			t.Fatalf("bubblewrap advice missing %q:\n%s", want, advice)
		}
	}
}

// --- verify exec.Cmd.Run() failure is correctly wrapped ---
func TestRunStrategy_ExecErrorWrapped(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		return mockCmd("false")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallBrew,
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	_, err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Error should have a non-empty message.
	if err.Error() == "" {
		t.Errorf("error should have a message")
	}

	// Error should wrap an *exec.ExitError (from running "false").
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Logf("note: error is not directly an ExitError (may be wrapped): %v", err)
	}
}

// --- TestRunStrategy_ScriptUpgradeSuccess ---

func TestRunStrategy_ScriptUpgradeSuccess(t *testing.T) {
	origExecCommand := execCommand
	origHTTPClient := scriptHTTPClient
	origInstallScriptURL := installScriptURLFn
	t.Cleanup(func() {
		execCommand = origExecCommand
		scriptHTTPClient = origHTTPClient
		installScriptURLFn = origInstallScriptURL
	})

	// Serve a fake install.sh that succeeds.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("#!/bin/bash\necho 'install ok'\n"))
	}))
	defer server.Close()

	scriptHTTPClient = server.Client()

	// Override installScriptURL to point to our test server.
	installScriptURLFn = func(owner, repo, version string) (string, error) {
		return server.URL + "/install.sh", nil
	}

	var gotScriptContent string
	execCommand = func(name string, args ...string) *exec.Cmd {
		// Capture the script content passed via bash -c.
		if name == "bash" && len(args) >= 2 && args[0] == "-c" {
			gotScriptContent = args[1]
		}
		return mockCmd("echo", "ok")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gga",
			Owner:         "Gentleman-Programming",
			Repo:          "gentleman-guardian-angel",
			InstallMethod: update.InstallScript,
		},
		LatestVersion: "2.8.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	err := scriptUpgrade(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("scriptUpgrade: unexpected error: %v", err)
	}

	// Verify that bash was called with the install.sh content.
	if !containsAny(gotScriptContent, "install ok", "#!/bin/bash") {
		t.Errorf("bash -c did not receive install.sh content; got: %q", gotScriptContent)
	}
}

// --- TestRunStrategy_ScriptUpgradeDownloadFailure ---

func TestRunStrategy_ScriptUpgradeDownloadFailure(t *testing.T) {
	origHTTPClient := scriptHTTPClient
	origInstallScriptURL := installScriptURLFn
	t.Cleanup(func() {
		scriptHTTPClient = origHTTPClient
		installScriptURLFn = origInstallScriptURL
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	scriptHTTPClient = server.Client()
	installScriptURLFn = func(owner, repo, version string) (string, error) {
		return server.URL + "/install.sh", nil
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gga",
			Owner:         "Gentleman-Programming",
			Repo:          "gentleman-guardian-angel",
			InstallMethod: update.InstallScript,
		},
		LatestVersion: "2.8.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	err := scriptUpgrade(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when install.sh download fails, got nil")
	}
}

// --- TestRunStrategy_ScriptUpgradeWindowsManualFallback ---

func TestRunStrategy_ScriptUpgradeWindowsManualFallback(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return mockCmd("echo", "should not run")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gga",
			Owner:         "Gentleman-Programming",
			Repo:          "gentleman-guardian-angel",
			InstallMethod: update.InstallScript,
		},
		LatestVersion: "2.8.0",
	}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}

	err := scriptUpgrade(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected manual fallback error for Windows script upgrade, got nil")
	}

	if execCalled {
		t.Errorf("exec should NOT be called for Windows script manual fallback")
	}
}

// --- TestGGAScriptUpgradeUsesGitClone ---

// TestGGAScriptUpgradeUsesGitClone verifies that ggaScriptUpgrade:
// 1. First calls `git clone --depth=1 --branch v<version> <repo-url> <tmpDir>`
// 2. Then calls `bash <path-to-install.sh>`
// — not `bash -c <script-content>` like the generic scriptUpgrade.
// The clone is pinned to the target release tag so that install.sh matches the
// version being upgraded to, not whatever is on main at upgrade time.
func TestGGAScriptUpgradeUsesGitClone(t *testing.T) {
	origExecCommand := execCommand
	origDetectOS := detectOS
	t.Cleanup(func() {
		execCommand = origExecCommand
		detectOS = origDetectOS
	})
	detectOS = func() string { return "linux" }

	type call struct {
		name string
		args []string
	}
	var calls []call

	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, call{name: name, args: args})
		return mockCmd("echo", "ok")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gga",
			Owner:         "Gentleman-Programming",
			Repo:          "gentleman-guardian-angel",
			InstallMethod: update.InstallScript,
		},
		LatestVersion: "2.8.0",
	}

	err := ggaScriptUpgrade(context.Background(), r)
	if err != nil {
		t.Fatalf("ggaScriptUpgrade: unexpected error: %v", err)
	}

	// Must have at least 4 exec calls (git init, git fetch, git checkout, bash install.sh).
	if len(calls) < 4 {
		t.Fatalf("expected at least 4 exec calls (git init + fetch + checkout + bash install.sh), got %d: %v", len(calls), calls)
	}

	// First call: git init
	if calls[0].name != "git" || len(calls[0].args) == 0 || calls[0].args[0] != "init" {
		t.Errorf("first exec call = %v, want git init", calls[0])
	}

	// Second call: git -C <dir> fetch --depth=1 <repoURL> refs/tags/v2.8.0:refs/tags/v2.8.0
	fetchArgs := calls[1].args
	foundRepoURL := false
	foundTagRef := false
	for _, a := range fetchArgs {
		if containsAny(a, "gentleman-guardian-angel") {
			foundRepoURL = true
		}
		if a == "refs/tags/v2.8.0:refs/tags/v2.8.0" {
			foundTagRef = true
		}
	}
	if !foundRepoURL {
		t.Errorf("git fetch args %v should include the repo URL (gentleman-guardian-angel)", fetchArgs)
	}
	if !foundTagRef {
		t.Errorf("git fetch args %v should include refs/tags/v2.8.0:refs/tags/v2.8.0 to pin to the release tag", fetchArgs)
	}

	// Third call: git -C <dir> checkout -f refs/tags/v2.8.0
	checkoutArgs := calls[2].args
	foundCheckoutTag := false
	for _, a := range checkoutArgs {
		if a == "refs/tags/v2.8.0" {
			foundCheckoutTag = true
		}
	}
	if !foundCheckoutTag {
		t.Errorf("git checkout args %v should include refs/tags/v2.8.0", checkoutArgs)
	}

	// Fourth call must be `bash <path-to-install.sh>` (not bash -c <content>).
	if calls[3].name != "bash" {
		t.Errorf("fourth exec call name = %q, want %q", calls[3].name, "bash")
	}
	if len(calls[3].args) == 0 {
		t.Fatalf("fourth exec call has no args")
	}
	installScriptArg := calls[3].args[0]
	if !containsAny(installScriptArg, "install.sh") {
		t.Errorf("bash arg = %q, want path containing install.sh", installScriptArg)
	}
	// Must NOT be bash -c (inline script content) — must be a file path.
	if installScriptArg == "-c" {
		t.Errorf("bash was called with -c (inline script), expected a file path to install.sh")
	}
}

// --- TestGGAScriptUpgradeWindowsManualFallback ---

// TestGGAScriptUpgradeWindowsManualFallback verifies that on Windows,
// ggaScriptUpgrade returns a ManualFallbackError without calling exec.
func TestGGAScriptUpgradeWindowsManualFallback(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return mockCmd("echo", "should not run")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gga",
			Owner:         "Gentleman-Programming",
			Repo:          "gentleman-guardian-angel",
			InstallMethod: update.InstallScript,
		},
		LatestVersion: "2.8.0",
	}

	err := ggaScriptUpgradeForOS(context.Background(), r, "windows")
	if err == nil {
		t.Errorf("expected ManualFallbackError for Windows, got nil")
	}
	var mfe *ManualFallbackError
	if !errors.As(err, &mfe) {
		t.Errorf("expected *ManualFallbackError, got %T: %v", err, err)
	}
	if execCalled {
		t.Errorf("exec should NOT be called on Windows for ggaScriptUpgrade")
	}
}

// --- TestRunStrategy_GGAUsesGitClone ---

// TestRunStrategy_GGAUsesGitClone verifies that when runStrategy is called with
// a GGA tool (InstallScript), it routes to ggaScriptUpgrade (git clone approach)
// rather than the generic scriptUpgrade (bash -c <content>).
func TestRunStrategy_GGAUsesGitClone(t *testing.T) {
	origExecCommand := execCommand
	origDetectOS := detectOS
	t.Cleanup(func() {
		execCommand = origExecCommand
		detectOS = origDetectOS
	})
	detectOS = func() string { return "linux" }

	type call struct {
		name string
		args []string
	}
	var calls []call

	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, call{name: name, args: args})
		return mockCmd("echo", "ok")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gga",
			Owner:         "Gentleman-Programming",
			Repo:          "gentleman-guardian-angel",
			InstallMethod: update.InstallScript,
		},
		LatestVersion: "2.8.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy GGA: unexpected error: %v", err)
	}

	// Must have used git init + fetch (not bash -c).
	if len(calls) < 4 {
		t.Fatalf("expected at least 4 calls (git init + fetch + checkout + bash), got %d: %v", len(calls), calls)
	}
	if calls[0].name != "git" || (len(calls[0].args) > 0 && calls[0].args[0] != "init") {
		t.Errorf("expected first call to be `git init`, got: %q %v", calls[0].name, calls[0].args)
	}
}

// --- TestInstallScriptURL ---

func TestInstallScriptURL(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		repo        string
		version     string
		wantURL     string
		wantErr     bool
		wantContain string
	}{
		{
			name:        "pins to release tag",
			owner:       "Gentleman-Programming",
			repo:        "gentleman-guardian-angel",
			version:     "1.31.0",
			wantURL:     "https://raw.githubusercontent.com/Gentleman-Programming/gentleman-guardian-angel/v1.31.0/install.sh",
			wantContain: "v1.31.0",
		},
		{
			name:    "empty version returns error",
			owner:   "Gentleman-Programming",
			repo:    "gentle-ai",
			version: "",
			wantErr: true,
		},
		{
			name:    "whitespace-only version returns error",
			owner:   "Gentleman-Programming",
			repo:    "gentle-ai",
			version: "   ",
			wantErr: true,
		},
		{
			name:        "does not reference main",
			owner:       "Gentleman-Programming",
			repo:        "gentle-ai",
			version:     "2.0.0",
			wantContain: "v2.0.0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, err := installScriptURL(tc.owner, tc.repo, tc.version)
			if tc.wantErr {
				if err == nil {
					t.Errorf("installScriptURL(%q, %q, %q): want error, got nil (url=%q)", tc.owner, tc.repo, tc.version, url)
				}
				return
			}
			if err != nil {
				t.Fatalf("installScriptURL(%q, %q, %q): unexpected error: %v", tc.owner, tc.repo, tc.version, err)
			}
			if tc.wantURL != "" && url != tc.wantURL {
				t.Errorf("installScriptURL = %q, want %q", url, tc.wantURL)
			}
			if tc.wantContain != "" && !containsAny(url, tc.wantContain) {
				t.Errorf("installScriptURL = %q, want it to contain %q", url, tc.wantContain)
			}
			if containsAny(url, "/main/") {
				t.Errorf("installScriptURL = %q must NOT reference /main/", url)
			}
		})
	}
}

// --- TestEngramUpgradeUsesDownloadNotGoInstall ---

// TestEngramUpgradeUsesDownloadNotGoInstall verifies that on Windows (non-brew),
// engram upgrade calls the binary download function, NOT go install.
// This is the regression test for issue #160.
func TestEngramUpgradeUsesDownloadNotGoInstall(t *testing.T) {
	origExecCommand := execCommand
	origEngramDownloadFn := engramDownloadFn
	t.Cleanup(func() {
		execCommand = origExecCommand
		engramDownloadFn = origEngramDownloadFn
	})

	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return mockCmd("echo", "should not be called")
	}

	downloadCalled := false
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		downloadCalled = true
		return "/fake/path/engram.exe", nil
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			Owner:         "Gentleman-Programming",
			Repo:          "engram",
			InstallMethod: update.InstallBinary, // should be InstallBinary after fix
		},
		LatestVersion: "0.5.0",
	}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy engram windows: unexpected error: %v", err)
	}

	// Must call binary download, NOT go install.
	if !downloadCalled {
		t.Errorf("expected engramDownloadFn to be called, but it was not")
	}
	if execCalled {
		t.Errorf("exec (go install) should NOT be called for engram on Windows — use binary download")
	}
}

// --- TestEngramUpgradeLinuxUsesDownload ---

// TestEngramUpgradeLinuxUsesDownload verifies that on Linux (non-brew),
// engram upgrade uses the binary download function, not go install.
func TestEngramUpgradeLinuxUsesDownload(t *testing.T) {
	origExecCommand := execCommand
	origEngramDownloadFn := engramDownloadFn
	t.Cleanup(func() {
		execCommand = origExecCommand
		engramDownloadFn = origEngramDownloadFn
	})

	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return mockCmd("echo", "should not be called")
	}

	downloadCalled := false
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		downloadCalled = true
		return "/home/user/.local/bin/engram", nil
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			Owner:         "Gentleman-Programming",
			Repo:          "engram",
			InstallMethod: update.InstallBinary, // should be InstallBinary after fix
		},
		LatestVersion: "0.5.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy engram linux: unexpected error: %v", err)
	}

	if !downloadCalled {
		t.Errorf("expected engramDownloadFn to be called for engram on Linux, but it was not")
	}
	if execCalled {
		t.Errorf("exec (go install) should NOT be called for engram on Linux — use binary download")
	}
}

// --- TestRunStrategy_ScriptUpgradeExecFailure ---

func TestRunStrategy_ScriptUpgradeExecFailure(t *testing.T) {
	origExecCommand := execCommand
	origHTTPClient := scriptHTTPClient
	origInstallScriptURL := installScriptURLFn
	t.Cleanup(func() {
		execCommand = origExecCommand
		scriptHTTPClient = origHTTPClient
		installScriptURLFn = origInstallScriptURL
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("#!/bin/bash\nexit 1\n"))
	}))
	defer server.Close()
	scriptHTTPClient = server.Client()
	installScriptURLFn = func(owner, repo, version string) (string, error) {
		return server.URL + "/install.sh", nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		return mockCmd("false")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gga",
			Owner:         "Gentleman-Programming",
			Repo:          "gentleman-guardian-angel",
			InstallMethod: update.InstallScript,
		},
		LatestVersion: "2.8.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	err := scriptUpgrade(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when install.sh execution fails, got nil")
	}
}

// --- TestEngramBinaryUpgrade_ChannelRouting (Slice 3) ---

// TestEngramBinaryUpgrade_StableChannelCallsDownloadFn verifies that when
// GENTLE_AI_CHANNEL is unset or "stable", engramBinaryUpgrade delegates to
// engramDownloadFn (the release-download path) and NOT go install @main.
func TestEngramBinaryUpgrade_StableChannelCallsDownloadFn(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
	}{
		{name: "channel unset", envVal: ""},
		{name: "channel explicit stable", envVal: "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GENTLE_AI_CHANNEL", tt.envVal)

			origDownloadFn := engramDownloadFn
			origExecCommand := execCommand
			t.Cleanup(func() {
				engramDownloadFn = origDownloadFn
				execCommand = origExecCommand
			})

			downloadCalled := false
			engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
				downloadCalled = true
				return "/tmp/engram", nil
			}

			// go install must NOT be called for stable channel.
			execCommand = func(name string, args ...string) *exec.Cmd {
				t.Errorf("execCommand called unexpectedly for stable channel: %s %v", name, args)
				return mockCmd("echo", "unexpected")
			}

			profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
			err := engramBinaryUpgrade(profile)
			if err != nil {
				t.Fatalf("engramBinaryUpgrade stable: unexpected error: %v", err)
			}
			if !downloadCalled {
				t.Error("expected engramDownloadFn to be called for stable channel, but it was not")
			}
		})
	}
}

// TestEngramBinaryUpgrade_BetaChannelUsesGoInstallMain verifies that when
// GENTLE_AI_CHANNEL=beta, engramBinaryUpgrade delegates to
// engramBetaInstallFn (the consolidated beta path, backed by
// engram.DownloadLatestBinary(profile, true) in production). The stable
// engramDownloadFn must NOT be called.
func TestEngramBinaryUpgrade_BetaChannelUsesGoInstallMain(t *testing.T) {
	t.Setenv("GENTLE_AI_CHANNEL", "beta")

	origDownloadFn := engramDownloadFn
	origBetaFn := engramBetaInstallFn
	t.Cleanup(func() {
		engramDownloadFn = origDownloadFn
		engramBetaInstallFn = origBetaFn
	})

	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		t.Error("engramDownloadFn (stable path) must NOT be called for beta channel")
		return "", nil
	}

	var betaCalled bool
	engramBetaInstallFn = func(profile system.PlatformProfile) (string, error) {
		betaCalled = true
		return "/tmp/engram-beta", nil
	}

	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}
	err := engramBinaryUpgrade(profile)
	if err != nil {
		t.Fatalf("engramBinaryUpgrade beta: unexpected error: %v", err)
	}
	if !betaCalled {
		t.Fatal("expected engramBetaInstallFn (beta path) to be called, but it was not")
	}
}
