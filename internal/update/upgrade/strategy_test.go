package upgrade

import (
	"context"
	"errors"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- TestRunStrategy_BrewUpgrade ---

func TestRunStrategy_BrewUpgrade(t *testing.T) {
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
	if len(gotArgs) < 2 || gotArgs[0] != "upgrade" || gotArgs[1] != "engram" {
		t.Errorf("exec args = %v, want [upgrade engram]", gotArgs)
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

func TestRunStrategy_GoInstallAndroidPIE(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "go" {
			gotArgs = args
		}
		return mockCmd("true")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallGoInstall,
			GoImportPath:  "github.com/Gentleman-Programming/engram/cmd/engram",
		},
		LatestVersion: "0.4.0",
	}
	// Simulate Android/Termux environment — GOOS=android is the canonical path.
	profile := system.PlatformProfile{OS: "android", LinuxDistro: system.LinuxDistroTermux}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy: %v", err)
	}

	// Should contain -ldflags="-extldflags=-pie"
	foundPIE := false
	for _, arg := range gotArgs {
		if strings.Contains(arg, "-extldflags=-pie") {
			foundPIE = true
			break
		}
	}
	if !foundPIE {
		t.Errorf("expected PIE flags in go install args for Android, got: %v", gotArgs)
	}
}

func TestRunStrategy_GentleAIAndroidSelfUpgradeUsesGoInstallWithPIE(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return mockCmd("true")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gentle-ai",
			InstallMethod: update.InstallBinary,
			GoImportPath:  "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai",
		},
		LatestVersion: "1.36.4",
	}
	profile := system.PlatformProfile{OS: "android", LinuxDistro: system.LinuxDistroTermux, PackageManager: "apt", GoAvailable: true}

	_, err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy: %v", err)
	}

	wantArgs := []string{"install", "-ldflags=-extldflags=-pie", "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai@v1.36.4"}
	if gotName != "go" {
		t.Fatalf("exec name = %q, want go", gotName)
	}
	if strings.Join(gotArgs, " ") != strings.Join(wantArgs, " ") {
		t.Fatalf("exec args = %v, want %v", gotArgs, wantArgs)
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

// --- TestEffectiveMethod_GentleAIOnWindowsUsesInstaller ---

// TestEffectiveMethod_GentleAIOnWindowsUsesInstaller verifies that gentle-ai
// on Windows uses InstallInstaller (auto-upgrade via PowerShell)
func TestEffectiveMethod_GentleAIOnWindowsUsesInstaller(t *testing.T) {
	tests := []struct {
		name string
		tool update.ToolInfo
		want update.InstallMethod
	}{
		{
			name: "binary becomes installer",
			tool: update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary},
			want: update.InstallInstaller,
		},
		{
			name: "script becomes installer",
			tool: update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallScript},
			want: update.InstallInstaller,
		},
		{
			name: "go-install becomes installer",
			tool: update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallGoInstall},
			want: update.InstallInstaller,
		},
		{
			name: "installer stays installer",
			tool: update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallInstaller},
			want: update.InstallInstaller,
		},
		{
			name: "go available still uses installer",
			tool: update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary, GoImportPath: "github.com/Gentleman-Programming/gentle-ai/cmd/gentle-ai"},
			want: update.InstallInstaller,
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

// --- TestEffectiveMethod_NonGentleAIToolsOnWindowsUseBinary ---

// TestEffectiveMethod_NonGentleAIToolsOnWindowsUseBinary verifies that tools
// OTHER than gentle-ai on Windows still use their declared install method
// (binary, script, etc.) - they don't get InstallInstaller.
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
	tests := []struct {
		name    string
		tool    update.ToolInfo
		profile system.PlatformProfile
		want    update.InstallMethod
	}{
		{
			name:    "brew profile overrides go-install",
			tool:    update.ToolInfo{Name: "engram", InstallMethod: update.InstallGoInstall},
			profile: system.PlatformProfile{PackageManager: "brew"},
			want:    update.InstallBrew,
		},
		{
			name:    "brew profile overrides binary",
			tool:    update.ToolInfo{Name: "gga", InstallMethod: update.InstallBinary},
			profile: system.PlatformProfile{PackageManager: "brew"},
			want:    update.InstallBrew,
		},
		{
			name:    "brew profile overrides script",
			tool:    update.ToolInfo{Name: "gga", InstallMethod: update.InstallScript},
			profile: system.PlatformProfile{PackageManager: "brew"},
			want:    update.InstallBrew,
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
		{
			name:    "brew profile does not override OpenCode plugin method",
			tool:    update.ToolInfo{Name: "opencode-subagent-statusline", InstallMethod: update.InstallOpenCodePlugin, NpmPackage: "opencode-subagent-statusline"},
			profile: system.PlatformProfile{PackageManager: "brew"},
			want:    update.InstallOpenCodePlugin,
		},
		// Auto-detect order: brew → go-install → binary (issue #246).
		{
			name:    "auto-detect: brew available → brew wins regardless of GoImportPath",
			tool:    update.ToolInfo{Name: "mytool", InstallMethod: update.InstallBinary, GoImportPath: "github.com/example/mytool/cmd/mytool"},
			profile: system.PlatformProfile{PackageManager: "brew", GoAvailable: true},
			want:    update.InstallBrew,
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
		{
			name:    "gentle-ai on Android uses go-install because no release asset exists",
			tool:    update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary, GoImportPath: "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai"},
			profile: system.PlatformProfile{OS: "android", PackageManager: "apt", GoAvailable: true},
			want:    update.InstallGoInstall,
		},
		{
			name:    "gentle-ai on Linux keeps binary even when go is available",
			tool:    update.ToolInfo{Name: "gentle-ai", InstallMethod: update.InstallBinary, GoImportPath: "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai"},
			profile: system.PlatformProfile{OS: "linux", PackageManager: "apt", GoAvailable: true},
			want:    update.InstallBinary,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveMethod(tc.tool, tc.profile)
			if got != tc.want {
				t.Errorf("effectiveMethod = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunStrategyOpenCodePluginManualFallback(t *testing.T) {
	origHomeDir := openCodeHomeDir
	origLookPath := lookPathCommand
	origExecCommand := execCommand
	t.Cleanup(func() {
		openCodeHomeDir = origHomeDir
		lookPathCommand = origLookPath
		execCommand = origExecCommand
	})

	openCodeHomeDir = func() (string, error) { return t.TempDir(), nil }
	lookPathCommand = func(file string) (string, error) { return "", errors.New("not found") }
	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return mockCmd("echo", "should not run")
	}

	_, err := runStrategy(context.Background(), update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "opencode-subagent-statusline",
			InstallMethod: update.InstallOpenCodePlugin,
			NpmPackage:    "opencode-subagent-statusline",
		},
		UpdateHint: "restart OpenCode",
	}, system.PlatformProfile{PackageManager: "brew"})

	if err == nil {
		t.Fatal("expected manual fallback error, got nil")
	}
	if !containsAny(err.Error(), "OpenCode", "restart", "reload") {
		t.Fatalf("manual fallback should mention OpenCode restart/reload, got: %v", err)
	}
	if execCalled {
		t.Fatal("OpenCode plugin fallback should not run a package manager when config is missing")
	}

}

func TestRunStrategyOpenCodePluginUpgradesMaterializedPackage(t *testing.T) {
	origHomeDir := openCodeHomeDir
	origLookPath := lookPathCommand
	origExecCommand := execCommand
	t.Cleanup(func() {
		openCodeHomeDir = origHomeDir
		lookPathCommand = origLookPath
		execCommand = origExecCommand
	})

	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	pkg := "opencode-subagent-statusline"
	pkgDir := filepath.Join(opencodeDir, "node_modules", pkg)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"version":"0.1.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(home, ".cache", "opencode", "packages")
	targetCache := filepath.Join(cacheRoot, pkg+"@latest")
	otherPluginCache := filepath.Join(cacheRoot, "opencode-sdd-engram-manage@latest")
	versionedCache := filepath.Join(cacheRoot, pkg+"@0.5.2")
	for _, dir := range []string{targetCache, otherPluginCache, versionedCache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cwdFile := filepath.Join(t.TempDir(), "cwd.txt")

	openCodeHomeDir = func() (string, error) { return home, nil }
	lookPathCommand = func(file string) (string, error) {
		if file == "bun" {
			return "/usr/bin/bun", nil
		}
		return "", errors.New("not found")
	}

	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		cmd := exec.Command(os.Args[0], "-test.run=TestOpenCodePluginUpgradeHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GENTLE_AI_UPGRADE_HELPER=1",
			"GENTLE_AI_UPGRADE_HELPER_CWD_FILE="+cwdFile,
		)
		return cmd
	}

	_, err := runStrategy(context.Background(), update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          pkg,
			InstallMethod: update.InstallOpenCodePlugin,
			NpmPackage:    pkg,
		},
		InstalledVersion: "0.1.0",
		LatestVersion:    "0.2.0",
	}, system.PlatformProfile{PackageManager: "brew"})
	if err != nil {
		t.Fatalf("runStrategy OpenCode plugin: unexpected error: %v", err)
	}

	if gotName != "bun" {
		t.Fatalf("exec name = %q, want bun", gotName)
	}
	wantArgs := []string{"add", pkg + "@latest", "@opencode-ai/plugin@latest"}
	if strings.Join(gotArgs, " ") != strings.Join(wantArgs, " ") {
		t.Fatalf("exec args = %v, want %v", gotArgs, wantArgs)
	}
	if _, err := os.Stat(targetCache); !os.IsNotExist(err) {
		t.Fatalf("target OpenCode cache %s should be removed after upgrade, stat err: %v", targetCache, err)
	}
	for _, dir := range []string{otherPluginCache, versionedCache} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("non-target cache %s should remain, stat err: %v", dir, err)
		}
	}
	cwd, err := os.ReadFile(cwdFile)
	if err != nil {
		t.Fatalf("read helper cwd: %v", err)
	}
	gotCwd, err := filepath.EvalSymlinks(string(cwd))
	if err != nil {
		t.Fatalf("resolve helper cwd: %v", err)
	}
	wantCwd, err := filepath.EvalSymlinks(opencodeDir)
	if err != nil {
		t.Fatalf("resolve OpenCode dir: %v", err)
	}
	if gotCwd != wantCwd {
		t.Fatalf("command cwd = %q, want %q", gotCwd, wantCwd)
	}
}

func TestRunStrategyOpenCodePluginRegisteredPendingRunsPackageManager(t *testing.T) {
	origHomeDir := openCodeHomeDir
	origLookPath := lookPathCommand
	origExecCommand := execCommand
	t.Cleanup(func() {
		openCodeHomeDir = origHomeDir
		lookPathCommand = origLookPath
		execCommand = origExecCommand
	})

	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	pkg := "opencode-sdd-engram-manage"
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opencodeDir, "tui.json"), []byte(`{"plugin":["opencode-sdd-engram-manage"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	openCodeHomeDir = func() (string, error) { return home, nil }
	lookPathCommand = func(file string) (string, error) {
		if file == "npm" {
			return "/usr/bin/npm", nil
		}
		return "", errors.New("not found")
	}
	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return mockCmd("true")
	}

	_, err := runStrategy(context.Background(), update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          pkg,
			InstallMethod: update.InstallOpenCodePlugin,
			NpmPackage:    pkg,
		},
		Status: update.RegisteredNotMaterialized,
	}, system.PlatformProfile{})
	if err != nil {
		t.Fatalf("registered OpenCode plugin should be npm-managed during upgrade, got: %v", err)
	}
	if gotName != "npm" {
		t.Fatalf("exec name = %q, want npm", gotName)
	}
	wantArgs := []string{"install", "--save", "--no-audit", "--no-fund", pkg + "@latest", "@opencode-ai/plugin@latest"}
	if strings.Join(gotArgs, " ") != strings.Join(wantArgs, " ") {
		t.Fatalf("exec args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestRunStrategyOpenCodePluginFallsBackWithoutPackageManager(t *testing.T) {
	origHomeDir := openCodeHomeDir
	origLookPath := lookPathCommand
	origExecCommand := execCommand
	t.Cleanup(func() {
		openCodeHomeDir = origHomeDir
		lookPathCommand = origLookPath
		execCommand = origExecCommand
	})

	home := t.TempDir()
	opencodeDir := filepath.Join(home, ".config", "opencode")
	pkg := "opencode-sdd-engram-manage"
	if err := os.MkdirAll(filepath.Join(opencodeDir, "node_modules", pkg), 0o755); err != nil {
		t.Fatal(err)
	}

	openCodeHomeDir = func() (string, error) { return home, nil }
	lookPathCommand = func(file string) (string, error) { return "", errors.New("not found") }
	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return mockCmd("echo", "should not run")
	}

	_, err := runStrategy(context.Background(), update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          pkg,
			InstallMethod: update.InstallOpenCodePlugin,
			NpmPackage:    pkg,
		},
	}, system.PlatformProfile{})
	if err == nil {
		t.Fatal("expected manual fallback when bun/npm are unavailable, got nil")
	}
	if !containsAny(err.Error(), "bun", "npm", "package manager") {
		t.Fatalf("fallback should mention missing package manager, got: %v", err)
	}
	if execCalled {
		t.Fatal("OpenCode plugin fallback should not run a package manager when none is available")
	}
}

func TestSelectOpenCodePackageManagerPrefersPackageMetadata(t *testing.T) {
	origLookPath := lookPathCommand
	t.Cleanup(func() { lookPathCommand = origLookPath })

	opencodeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(opencodeDir, "package.json"), []byte(`{"packageManager":"npm@10.8.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	lookPathCommand = func(file string) (string, error) {
		switch file {
		case "bun", "npm":
			return filepath.Join("/usr/bin", file), nil
		default:
			return "", errors.New("not found")
		}
	}

	pm, err := selectOpenCodePackageManager(opencodeDir)
	if err != nil {
		t.Fatalf("selectOpenCodePackageManager: unexpected error: %v", err)
	}
	if pm != "npm" {
		t.Fatalf("package manager = %q, want npm from package.json metadata", pm)
	}
}

func TestOpenCodePluginUpgradeHelperProcess(t *testing.T) {
	if os.Getenv("GENTLE_AI_UPGRADE_HELPER") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("GENTLE_AI_UPGRADE_HELPER_CWD_FILE"), []byte(cwd), 0o644); err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(2)
	}
	os.Exit(0)
}

// --- TestManualFallbackHint ---
//
// Removed: TestManualFallbackHint previously verified that Windows binary
// self-replace for gentle-ai returns a manual fallback error. The Windows
// installer method (PR #257) now routes gentle-ai to installerUpgrade, which
// downloads and launches the PowerShell installer. The manual-fallback path
// remains exercised by binaryUpgrade for non-gentle-ai tools on Windows
// (see TestRunStrategy_UnsupportedMethodManualFallback and
// TestRunStrategy_ScriptUpgradeWindowsManualFallback).

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

	err := brewUpgrade(context.Background(), "gentle-ai")
	if err != nil {
		t.Fatalf("brewUpgrade: unexpected error: %v", err)
	}

	// Must have called brew tap, brew update AND brew upgrade — in that order.
	if len(callOrder) < 3 {
		t.Fatalf("expected 3 brew calls (tap, update, upgrade), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[1] != "update" {
		t.Errorf("second brew call = %q, want %q", callOrder[1], "update")
	}
	if callOrder[2] != "upgrade" {
		t.Errorf("third brew call = %q, want %q", callOrder[2], "upgrade")
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

	err := brewUpgrade(context.Background(), "gentle-ai")
	// brew update failed but brew upgrade succeeded → overall success.
	if err != nil {
		t.Errorf("expected success when brew update fails but brew upgrade succeeds, got: %v", err)
	}

	// Both brew update and brew upgrade must have been called (after the tap).
	if len(callArgs) < 3 {
		t.Fatalf("expected 3 brew calls, got %d: %v", len(callArgs), callArgs)
	}
	if callArgs[1] != "update" {
		t.Errorf("second brew call = %q, want %q", callArgs[1], "update")
	}
	if callArgs[2] != "upgrade" {
		t.Errorf("third brew call = %q, want %q", callArgs[2], "upgrade")
	}
}

// --- TestBrewUpgrade_TapsBeforeUpdateAndUpgrade ---

// TestBrewUpgrade_TapsBeforeUpdateAndUpgrade verifies that brewUpgrade calls
// `brew tap Gentleman-Programming/homebrew-tap` BEFORE `brew update` and
// `brew upgrade <toolName>`. This makes the upgrade idempotent when a user
// has lost the tap (untap, machine swap, brew cleanup). See issue #455.
func TestBrewUpgrade_TapsBeforeUpdateAndUpgrade(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	type call struct {
		subcommand string
		arg        string
	}
	var calls []call
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "brew" && len(args) > 0 {
			c := call{subcommand: args[0]}
			if len(args) > 1 {
				c.arg = args[1]
			}
			calls = append(calls, c)
		}
		return mockCmd("echo", "ok")
	}

	if err := brewUpgrade(context.Background(), "engram"); err != nil {
		t.Fatalf("brewUpgrade: unexpected error: %v", err)
	}

	if len(calls) < 3 {
		t.Fatalf("expected 3 brew calls (tap, update, upgrade), got %d: %+v", len(calls), calls)
	}
	if calls[0].subcommand != "tap" {
		t.Errorf("first brew call subcommand = %q, want %q", calls[0].subcommand, "tap")
	}
	if calls[0].arg != "Gentleman-Programming/homebrew-tap" {
		t.Errorf("first brew call arg = %q, want %q", calls[0].arg, "Gentleman-Programming/homebrew-tap")
	}
	if calls[1].subcommand != "update" {
		t.Errorf("second brew call = %q, want %q", calls[1].subcommand, "update")
	}
	if calls[2].subcommand != "upgrade" {
		t.Errorf("third brew call = %q, want %q", calls[2].subcommand, "upgrade")
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

	// Must have at least 2 exec calls.
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 exec calls (git clone + bash install.sh), got %d: %v", len(calls), calls)
	}

	// First call must be `git clone`.
	if calls[0].name != "git" {
		t.Errorf("first exec call name = %q, want %q", calls[0].name, "git")
	}
	if len(calls[0].args) == 0 || calls[0].args[0] != "clone" {
		t.Errorf("first exec args[0] = %q, want %q", calls[0].args[0], "clone")
	}
	// The clone args must include the target tag via --branch.
	cloneArgs := calls[0].args
	foundRepoURL := false
	foundTag := false
	for i, a := range cloneArgs {
		if containsAny(a, "gentleman-guardian-angel") {
			foundRepoURL = true
		}
		if a == "--branch" && i+1 < len(cloneArgs) && cloneArgs[i+1] == "v2.8.0" {
			foundTag = true
		}
	}
	if !foundRepoURL {
		t.Errorf("git clone args %v should include the repo URL (gentleman-guardian-angel)", cloneArgs)
	}
	if !foundTag {
		t.Errorf("git clone args %v should include --branch v2.8.0 to pin to the release tag", cloneArgs)
	}

	// Second call must be `bash <path-to-install.sh>` (not bash -c <content>).
	if calls[1].name != "bash" {
		t.Errorf("second exec call name = %q, want %q", calls[1].name, "bash")
	}
	if len(calls[1].args) == 0 {
		t.Fatalf("second exec call has no args")
	}
	installScriptArg := calls[1].args[0]
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

	// Must have used git clone (not bash -c).
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 calls (git clone + bash), got %d: %v", len(calls), calls)
	}
	if calls[0].name != "git" || (len(calls[0].args) > 0 && calls[0].args[0] != "clone") {
		t.Errorf("expected first call to be `git clone`, got: %q %v", calls[0].name, calls[0].args)
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

// --- TestInstallerUpgrade_Success ---

func TestInstallerUpgrade_Success(t *testing.T) {
	origExecCommand := execCommand
	origHTTPClient := scriptHTTPClient
	origGoos := runtime.GOOS
	t.Cleanup(func() {
		execCommand = origExecCommand
		scriptHTTPClient = origHTTPClient
	})

	if origGoos != "windows" {
		t.Skip("skipping Windows-only installer test on non-windows platform")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Write-Output 'installer ok'\n"))
	}))
	defer server.Close()

	scriptHTTPClient = server.Client()

	execCalled := false
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		gotArgs = append(gotArgs, args...)
		return mockCmd("echo", "ok")
	}

	tool := update.ToolInfo{
		Name:          "gentle-ai",
		Owner:         "Gentleman-Programming",
		Repo:          "gentle-ai",
		InstallMethod: update.InstallInstaller,
	}

	// Change URL to use the local test server for the test.
	// Since installerUpgrade constructs the URL directly, we mock the HTTP client and use a round tripper
	// or we just trust the mock HTTP client will handle the request.
	// Wait, installerUpgrade builds scriptURL := "https://raw.githubusercontent.com/...".
	// The HTTP client needs to redirect this or respond directly.
	// We'll create a custom RoundTripper so any URL returns our mock response.
	scriptHTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "text/plain")
		rec.WriteHeader(http.StatusOK)
		rec.WriteString("Write-Output 'installer ok'\n")
		return rec.Result(), nil
	})

	exitReq, err := installerUpgrade(context.Background(), tool, "")
	if err != nil {
		t.Fatalf("installerUpgrade: unexpected error: %v", err)
	}

	if !exitReq {
		t.Errorf("expected exitReq to be true on success")
	}
	if !execCalled {
		t.Errorf("expected execCommand to be called")
	}

	// Check if the temp file path is passed
	filePassed := false
	for i, arg := range gotArgs {
		if arg == "-File" && i+1 < len(gotArgs) {
			if strings.Contains(gotArgs[i+1], "gentle-ai-install") {
				filePassed = true
			}
		}
	}
	if !filePassed {
		t.Errorf("expected -File argument with temp file path, got args: %v", gotArgs)
	}
}

// --- TestInstallerUpgrade_DownloadFailure ---

func TestInstallerUpgrade_DownloadFailure(t *testing.T) {
	origHTTPClient := scriptHTTPClient
	origGoos := runtime.GOOS
	t.Cleanup(func() {
		scriptHTTPClient = origHTTPClient
	})

	if origGoos != "windows" {
		t.Skip("skipping Windows-only installer test on non-windows platform")
	}

	scriptHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusNotFound)
			return rec.Result(), nil
		}),
	}

	tool := update.ToolInfo{
		Name:          "gentle-ai",
		Owner:         "Gentleman-Programming",
		Repo:          "gentle-ai",
		InstallMethod: update.InstallInstaller,
	}

	exitReq, err := installerUpgrade(context.Background(), tool, "")
	if err == nil {
		t.Errorf("expected error when installer download fails, got nil")
	}
	if exitReq {
		t.Errorf("expected exitReq to be false on error")
	}
}

// --- TestInstallerUpgrade_NonWindows ---

func TestInstallerUpgrade_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping non-Windows test on Windows platform")
	}
	tool := update.ToolInfo{Name: "gentle-ai"}
	exitReq, err := installerUpgrade(context.Background(), tool, "")
	if err == nil {
		t.Errorf("expected error when calling installerUpgrade on non-windows, got nil")
	}
	if exitReq {
		t.Errorf("expected exitReq to be false")
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
