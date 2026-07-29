package upgrade

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

// TestEffectiveMethodWindowsPrecedenceIsUnchanged pins the rules that run before
// and after the Windows branch, so any future change to that branch has to keep
// plugin, brew-ownership, and declared-method precedence intact.
func TestEffectiveMethodWindowsPrecedenceIsUnchanged(t *testing.T) {
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
			name:          "OpenCode plugin wins over go-install on Windows",
			tool:          update.ToolInfo{Name: "opencode-subagent-statusline", InstallMethod: update.InstallOpenCodePlugin, NpmPackage: "opencode-subagent-statusline", GoImportPath: "github.com/example/plugin/cmd/plugin"},
			profile:       system.PlatformProfile{OS: "windows", PackageManager: "brew", GoAvailable: true},
			brewInstalled: true,
			want:          update.InstallOpenCodePlugin,
		},
		{
			name:          "brew-owned package wins over go-install on Windows",
			tool:          update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary, GoImportPath: "github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai"},
			profile:       system.PlatformProfile{OS: "windows", PackageManager: "brew", GoAvailable: true},
			brewInstalled: true,
			want:          update.InstallBrew,
		},
		{
			name:    "no Go on Windows keeps the declared method",
			tool:    update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary, GoImportPath: "github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai"},
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: false},
			want:    update.InstallBinary,
		},
		{
			name:    "no import path on Windows keeps the declared method",
			tool:    update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary},
			profile: system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true},
			want:    update.InstallBinary,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			homebrewPackageInstalled = func(string) bool { return tc.brewInstalled }
			if got := effectiveMethod(tc.tool, tc.profile); got != tc.want {
				t.Errorf("effectiveMethod(%q) = %q, want %q", tc.tool.Name, got, tc.want)
			}
		})
	}
}

// gentleAIImportPath is the module path gentle-ai publishes its command under.
// It is asserted against the registry below so the tests and the shipped
// declaration cannot drift apart.
const gentleAIImportPath = "github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai"

// registryGentleAI returns the shipped gentle-ai registry entry. Routing tests
// use the real declaration rather than a hand-built ToolInfo so a regression in
// registry.go cannot hide behind a synthetic fixture.
func registryGentleAI(t *testing.T) update.ToolInfo {
	t.Helper()
	for _, tool := range update.Tools {
		if tool.Name == "gentle-ai" {
			return tool
		}
	}
	t.Fatal("gentle-ai is missing from the tool registry")
	return update.ToolInfo{}
}

// TestRegistryDeclaresGentleAIGoImportPath pins the declaration that makes the
// Windows self-upgrade possible. Without it, Windows has no import path to
// install from and falls back to the source-install refusal.
func TestRegistryDeclaresGentleAIGoImportPath(t *testing.T) {
	if got := registryGentleAI(t).GoImportPath; got != gentleAIImportPath {
		t.Fatalf("gentle-ai GoImportPath = %q, want %q", got, gentleAIImportPath)
	}
}

// TestGentleAIOnWindowsWithGoAvailableUpgradesThroughGoInstall is the approved
// policy change: Windows publishes no signed binaries, so `go install` from the
// pinned release tag is the only automatic upgrade path available there.
func TestGentleAIOnWindowsWithGoAvailableUpgradesThroughGoInstall(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })
	homebrewPackageInstalled = func(string) bool { return false }

	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true}
	if got := effectiveMethod(registryGentleAI(t), profile); got != update.InstallGoInstall {
		t.Fatalf("effectiveMethod on Windows with Go = %q, want %q", got, update.InstallGoInstall)
	}
}

// TestGentleAIOnLinuxNeverRoutesToGoInstall is a regression guard for the
// authenticated-download requirement. Linux upgrades must keep downloading the
// signed release asset and verifying it with minisign; declaring a GoImportPath
// for the Windows path must never move Linux off that trust anchor.
//
// Do not delete this test. It is the only thing standing between a one-line
// routing edit and silently dropping minisign verification on Linux.
func TestGentleAIOnLinuxNeverRoutesToGoInstall(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })
	homebrewPackageInstalled = func(string) bool { return false }

	for _, packageManager := range []string{"apt", "pacman", "dnf", ""} {
		profile := system.PlatformProfile{OS: "linux", PackageManager: packageManager, GoAvailable: true}
		if got := effectiveMethod(registryGentleAI(t), profile); got != update.InstallBinary {
			t.Errorf("effectiveMethod on linux/%s = %q, want %q (minisign-verified download)", packageManager, got, update.InstallBinary)
		}
	}
}

// TestGentleAIOnMacOSNeverRoutesToGoInstall is the macOS half of the same
// regression guard. See TestGentleAIOnLinuxNeverRoutesToGoInstall.
func TestGentleAIOnMacOSNeverRoutesToGoInstall(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })
	homebrewPackageInstalled = func(string) bool { return false }

	for _, packageManager := range []string{"brew", ""} {
		profile := system.PlatformProfile{OS: "darwin", PackageManager: packageManager, GoAvailable: true}
		if got := effectiveMethod(registryGentleAI(t), profile); got != update.InstallBinary {
			t.Errorf("effectiveMethod on darwin/%s = %q, want %q (minisign-verified download)", packageManager, got, update.InstallBinary)
		}
	}
}

// TestGentleAILegacyScriptDeclarationNeverReachesScriptUpgradeOnWindows covers
// the second job of the Windows branch: a legacy InstallScript declaration must
// not route to scriptUpgrade, which on Windows has no bash and would send the
// user to a releases page that publishes no Windows assets.
func TestGentleAILegacyScriptDeclarationNeverReachesScriptUpgradeOnWindows(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })
	homebrewPackageInstalled = func(string) bool { return false }

	tests := []struct {
		name         string
		goAvailable  bool
		goImportPath string
		want         update.InstallMethod
	}{
		{name: "with Go available", goAvailable: true, goImportPath: gentleAIImportPath, want: update.InstallGoInstall},
		{name: "without Go available", goAvailable: false, goImportPath: gentleAIImportPath, want: update.InstallBinary},
		{name: "without an import path", goAvailable: true, want: update.InstallBinary},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := update.ToolInfo{
				Name:          "gentle-ai",
				Owner:         "Gentleman-Programming",
				Repo:          "gentle-ai",
				InstallMethod: update.InstallScript,
				GoImportPath:  tc.goImportPath,
			}
			profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: tc.goAvailable}
			got := effectiveMethod(tool, profile)
			if got == update.InstallScript {
				t.Fatal("a legacy script declaration reached scriptUpgrade on Windows")
			}
			if got != tc.want {
				t.Fatalf("effectiveMethod = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGentleAIWindowsGoInstallVerifiesDestination proves the destination check
// is reached on the new Windows go-install path and that a mismatch names both
// absolute paths. No real Windows execution happens here: the platform profile
// and detectOS are synthetic, which is the only way to exercise the Windows
// ".exe" naming and case-insensitive comparison from a Linux host.
func TestGentleAIWindowsGoInstallVerifiesDestination(t *testing.T) {
	origHomebrewPackageInstalled := homebrewPackageInstalled
	t.Cleanup(func() { homebrewPackageInstalled = origHomebrewPackageInstalled })
	homebrewPackageInstalled = func(string) bool { return false }

	origDetectOS := detectOS
	t.Cleanup(func() { detectOS = origDetectOS })
	detectOS = func() string { return "windows" }

	gobin := t.TempDir()
	stale := t.TempDir()
	installed := writeFakeBinary(t, gobin, "gentle-ai.exe")
	shadowing := writeFakeBinary(t, stale, "gentle-ai.exe")

	var installTarget string
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "go" && len(args) == 2 && args[0] == "env" {
			if args[1] == "GOBIN" {
				return mockCmd("echo", gobin)
			}
			return mockCmd("echo", "")
		}
		if name == "go" && len(args) == 2 && args[0] == "install" {
			installTarget = args[1]
			return mockCmd("true")
		}
		t.Fatalf("Windows go-install path executed unexpected command %s %v", name, args)
		return nil
	}

	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })
	lookPathFn = func(string) (string, error) { return shadowing, nil }

	r := update.UpdateResult{
		Tool:          registryGentleAI(t),
		LatestVersion: "2.2.0",
		Status:        update.UpdateAvailable,
	}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", Supported: true, GoAvailable: true}

	var upgradeErr error
	stderr := captureStderr(t, func() {
		_, upgradeErr = runStrategy(context.Background(), r, profile)
	})

	if upgradeErr != nil {
		t.Fatalf("Windows go-install upgrade returned error: %v", upgradeErr)
	}
	if want := gentleAIImportPath + "@v2.2.0"; installTarget != want {
		t.Errorf("go install target = %q, want the pinned release %q", installTarget, want)
	}
	if !strings.Contains(stderr, "WARNING") {
		t.Fatalf("a destination mismatch on Windows must warn; stderr = %q", stderr)
	}
	for _, want := range []string{installed, shadowing} {
		if !strings.Contains(stderr, want) {
			t.Errorf("warning must name %q; stderr = %q", want, stderr)
		}
	}
}

// TestGentleAIWindowsWithoutGoNamesRunnableSourceInstall covers the remaining
// honest refusal: with no Go on PATH the tool cannot self-upgrade, and the
// message must name a runnable command instead of a releases page that
// publishes no Windows assets.
func TestGentleAIWindowsWithoutGoNamesRunnableSourceInstall(t *testing.T) {
	origExec := execCommand
	t.Cleanup(func() { execCommand = origExec })
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("Windows refusal executed %s %v", name, args)
		return nil
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gentle-ai",
			Owner:         "Gentleman-Programming",
			Repo:          "gentle-ai",
			InstallMethod: update.InstallBinary,
			GoImportPath:  "github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai",
		},
		LatestVersion: "2.2.0",
		Status:        update.UpdateAvailable,
	}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", Supported: true, GoAvailable: false}

	if got := effectiveMethod(r.Tool, profile); got != update.InstallBinary {
		t.Fatalf("effectiveMethod without Go = %q, want %q", got, update.InstallBinary)
	}

	result := executeOne(context.Background(), r, profile, false)
	if result.Status != UpgradeSkipped || result.Err != nil {
		t.Fatalf("executeOne = %#v, want a non-error skip", result)
	}
	for _, required := range []string{
		"Windows binary distribution and Scoop are temporarily unavailable",
		"go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.2.0",
	} {
		if !strings.Contains(result.ManualHint, required) {
			t.Errorf("manual hint is missing %q: %s", required, result.ManualHint)
		}
	}
	if strings.Contains(result.ManualHint, "/releases") {
		t.Errorf("manual hint points at a releases page with no Windows assets: %s", result.ManualHint)
	}
}
