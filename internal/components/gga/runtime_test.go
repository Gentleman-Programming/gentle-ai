package gga

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

func TestEnsureRuntimeAssetsCreatesPRModeWhenMissing(t *testing.T) {
	home := t.TempDir()
	path := RuntimePRModePath(home)

	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("EnsureRuntimeAssets() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	text := string(content)
	if !strings.Contains(text, "detect_base_branch") {
		t.Fatalf("runtime pr_mode.sh missing expected content")
	}
}

// TestEnsureRuntimeAssetsOverwritesStalePRMode verifies the always-write behavior:
// when an existing pr_mode.sh has stale content (differs from the embedded asset),
// EnsureRuntimeAssets must overwrite it to keep the runtime current.
// WriteFileAtomic ensures this is a no-op when content already matches.
func TestEnsureRuntimeAssetsOverwritesStalePRMode(t *testing.T) {
	home := t.TempDir()
	path := RuntimePRModePath(home)
	if err := os.MkdirAll(RuntimeLibDir(home), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	const stale = "#!/usr/bin/env bash\n# stale-version\n"
	if err := os.WriteFile(path, []byte(stale), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("EnsureRuntimeAssets() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	// The stale content must have been replaced with the embedded asset.
	if string(content) == stale {
		t.Fatalf("EnsureRuntimeAssets did not overwrite stale pr_mode.sh")
	}
	if !strings.Contains(string(content), "detect_base_branch") {
		t.Fatalf("overwritten pr_mode.sh missing expected embedded content")
	}
}

// TestEnsureRuntimeAssetsIsNoOpWhenContentMatches verifies idempotency:
// when pr_mode.sh already contains the correct embedded content,
// EnsureRuntimeAssets must not modify it (WriteFileAtomic no-op).
func TestEnsureRuntimeAssetsIsNoOpWhenContentMatches(t *testing.T) {
	home := t.TempDir()

	// First call creates the file from the embedded asset.
	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("first EnsureRuntimeAssets() error = %v", err)
	}

	path := RuntimePRModePath(home)
	contentAfterFirst, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Get file mod time to detect if it was re-written.
	stat1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// Second call — should be a no-op because content matches.
	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("second EnsureRuntimeAssets() error = %v", err)
	}

	contentAfterSecond, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(contentAfterFirst) != string(contentAfterSecond) {
		t.Fatalf("content changed between two calls with identical embedded content")
	}

	stat2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// WriteFileAtomic returns early when content matches, so the file is not
	// replaced and the modification time must not change.
	if stat2.ModTime() != stat1.ModTime() {
		t.Fatalf("EnsureRuntimeAssets re-wrote the file even though content was identical")
	}
}

// ---------------------------------------------------------------------------
// RuntimeBinDir / RuntimePS1Path helpers
// ---------------------------------------------------------------------------

func TestRuntimeBinDir(t *testing.T) {
	tests := []struct {
		homeDir    string
		wantSuffix string
	}{
		{"/home/user", filepath.Join(".local", "share", "gga", "bin")},
		{"/root", filepath.Join(".local", "share", "gga", "bin")},
	}
	for _, tc := range tests {
		got := RuntimeBinDir(tc.homeDir)
		if !strings.HasSuffix(got, tc.wantSuffix) {
			t.Errorf("RuntimeBinDir(%q) = %q, want suffix %q", tc.homeDir, got, tc.wantSuffix)
		}
	}
}

func TestRuntimePS1Path(t *testing.T) {
	tests := []struct {
		homeDir    string
		wantSuffix string
	}{
		{"/home/user", filepath.Join("bin", "gga.ps1")},
		{"/root", filepath.Join("bin", "gga.ps1")},
	}
	for _, tc := range tests {
		got := RuntimePS1Path(tc.homeDir)
		if !strings.HasSuffix(got, tc.wantSuffix) {
			t.Errorf("RuntimePS1Path(%q) = %q, want suffix %q", tc.homeDir, got, tc.wantSuffix)
		}
	}
}

func TestRuntimeCMDPath(t *testing.T) {
	tests := []struct {
		homeDir    string
		wantSuffix string
	}{
		{"/home/user", filepath.Join("bin", "gga.cmd")},
		{"/root", filepath.Join("bin", "gga.cmd")},
	}
	for _, tc := range tests {
		got := RuntimeCMDPath(tc.homeDir)
		if !strings.HasSuffix(got, tc.wantSuffix) {
			t.Errorf("RuntimeCMDPath(%q) = %q, want suffix %q", tc.homeDir, got, tc.wantSuffix)
		}
	}
}

// ---------------------------------------------------------------------------
// EnsurePowerShellShim
// ---------------------------------------------------------------------------

func TestEnsurePowerShellShimCreatesFileWhenMissing(t *testing.T) {
	home := t.TempDir()
	path := RuntimePS1Path(home)

	if err := EnsurePowerShellShim(home); err != nil {
		t.Fatalf("EnsurePowerShellShim() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	// Verify file contains the expected shim sentinel content.
	text := string(content)
	if !strings.Contains(text, "Get-Command git") {
		t.Fatalf("gga.ps1 missing expected content, got: %s", text)
	}
}

func TestEnsurePowerShellShimOverwritesStaleShim(t *testing.T) {
	home := t.TempDir()
	path := RuntimePS1Path(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	const stale = "# stale-shim\n"
	if err := os.WriteFile(path, []byte(stale), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := EnsurePowerShellShim(home); err != nil {
		t.Fatalf("EnsurePowerShellShim() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	// The stale content must have been replaced.
	if string(content) == stale {
		t.Fatalf("EnsurePowerShellShim did not overwrite stale gga.ps1")
	}
	if !strings.Contains(string(content), "Get-Command git") {
		t.Fatalf("overwritten gga.ps1 missing expected embedded content")
	}
}

// TestEnsurePowerShellShimIsNoOpWhenContentMatches verifies idempotency:
// when gga.ps1 already contains the correct embedded content,
// EnsurePowerShellShim must not modify it (WriteFileAtomic no-op).
func TestEnsureCommandShimCreatesFileWhenMissing(t *testing.T) {
	home := t.TempDir()
	path := RuntimeCMDPath(home)

	if err := EnsureCommandShim(home); err != nil {
		t.Fatalf("EnsureCommandShim() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	text := string(content)
	if !strings.Contains(text, "gga.ps1") {
		t.Fatalf("gga.cmd missing expected PowerShell shim delegation, got: %s", text)
	}
}

func TestEnsureCommandShimOverwritesStaleShim(t *testing.T) {
	home := t.TempDir()
	path := RuntimeCMDPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	const stale = "@echo stale\r\n"
	if err := os.WriteFile(path, []byte(stale), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := EnsureCommandShim(home); err != nil {
		t.Fatalf("EnsureCommandShim() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	if string(content) == stale {
		t.Fatalf("EnsureCommandShim did not overwrite stale gga.cmd")
	}
	if !strings.Contains(string(content), "gga.ps1") {
		t.Fatalf("overwritten gga.cmd missing expected embedded content")
	}
}

func TestEnsurePowerShellShimIsNoOpWhenContentMatches(t *testing.T) {
	home := t.TempDir()

	// First call creates the file from the embedded asset.
	if err := EnsurePowerShellShim(home); err != nil {
		t.Fatalf("first EnsurePowerShellShim() error = %v", err)
	}

	path := RuntimePS1Path(home)
	contentAfterFirst, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	stat1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// Second call — should be a no-op because content matches.
	if err := EnsurePowerShellShim(home); err != nil {
		t.Fatalf("second EnsurePowerShellShim() error = %v", err)
	}

	contentAfterSecond, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(contentAfterFirst) != string(contentAfterSecond) {
		t.Fatalf("content changed between two calls with identical embedded content")
	}

	stat2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// WriteFileAtomic returns early when content matches, so the file is not
	// replaced and the modification time must not change.
	if stat2.ModTime() != stat1.ModTime() {
		t.Fatalf("EnsurePowerShellShim re-wrote the file even though content was identical")
	}
}

func TestPowerShellShimResolvesGitBash(t *testing.T) {
	if os.Getenv("OS") != "Windows_NT" {
		t.Skip("PowerShell shim is Windows-only")
	}

	powerShell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}
	helper := buildWindowsShimHelper(t)

	tests := []struct {
		name          string
		gitPath       string
		bashPaths     []string
		pathBash      bool
		helperExit    int
		wantBash      string
		wantExit      int
		wantErrorText string
	}{
		{name: "cmd Git uses root bin Bash", gitPath: `Git\cmd\git.exe`, bashPaths: []string{`Git\bin\bash.exe`}, wantBash: `Git\bin\bash.exe`},
		{name: "bin Git uses root bin Bash", gitPath: `Git\bin\git.exe`, bashPaths: []string{`Git\bin\bash.exe`}, wantBash: `Git\bin\bash.exe`},
		{name: "mingw64 bin Git uses root bin Bash case insensitively", gitPath: `Git\MINGW64\BIN\git.exe`, bashPaths: []string{`Git\bin\bash.exe`}, wantBash: `Git\bin\bash.exe`},
		{name: "root bin is preferred", gitPath: `Git\cmd\git.exe`, bashPaths: []string{`Git\bin\bash.exe`, `Git\usr\bin\bash.exe`}, wantBash: `Git\bin\bash.exe`},
		{name: "usr bin is fallback", gitPath: `Git\cmd\git.exe`, bashPaths: []string{`Git\usr\bin\bash.exe`}, wantBash: `Git\usr\bin\bash.exe`},
		{name: "installation path may contain spaces", gitPath: `Git Install\cmd\git.exe`, bashPaths: []string{`Git Install\bin\bash.exe`}, wantBash: `Git Install\bin\bash.exe`},
		{name: "Git absent", wantExit: 1, wantErrorText: "Git not found on PATH"},
		{name: "Git Bash absent", gitPath: `Git\cmd\git.exe`, wantExit: 1, wantErrorText: "Git Bash not found"},
		{name: "PATH Bash alias is not selected", gitPath: `Git\cmd\git.exe`, pathBash: true, wantExit: 1, wantErrorText: "Git Bash not found"},
		{name: "helper exit code is propagated", gitPath: `Git\cmd\git.exe`, bashPaths: []string{`Git\bin\bash.exe`}, helperExit: 37, wantBash: `Git\bin\bash.exe`, wantExit: 37},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			shim := filepath.Join(root, "gga.ps1")
			content, err := assets.Read("gga/gga.ps1")
			if err != nil {
				t.Fatalf("assets.Read() error = %v", err)
			}
			if err := os.WriteFile(shim, []byte(content), 0o644); err != nil {
				t.Fatalf("WriteFile(shim) error = %v", err)
			}

			pathDirs := []string{filepath.Join(root, "empty-path")}
			if tt.gitPath != "" {
				git := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(tt.gitPath, `\`, "/")))
				copyShimHelper(t, helper, git)
				pathDirs = append([]string{filepath.Dir(git)}, pathDirs...)
			}
			for _, path := range tt.bashPaths {
				copyShimHelper(t, helper, filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(path, `\`, "/"))))
			}
			if tt.pathBash {
				aliasDir := filepath.Join(root, "WindowsApps")
				copyShimHelper(t, helper, filepath.Join(aliasDir, "bash.exe"))
				pathDirs = append(pathDirs, aliasDir)
			}

			capture := filepath.Join(root, "selected-bash.txt")
			cmd := exec.Command(powerShell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", shim)
			cmd.Env = append(os.Environ(),
				"PATH="+strings.Join(pathDirs, string(os.PathListSeparator)),
				"GGA_TEST_CAPTURE="+capture,
				"GGA_TEST_EXIT="+strconv.Itoa(tt.helperExit),
			)
			output, runErr := cmd.CombinedOutput()
			gotExit := 0
			if runErr != nil {
				var exitErr *exec.ExitError
				if !errors.As(runErr, &exitErr) {
					t.Fatalf("PowerShell harness error = %v; output: %s", runErr, output)
				}
				gotExit = exitErr.ExitCode()
			}
			if gotExit != tt.wantExit {
				t.Fatalf("exit code = %d, want %d; output: %s", gotExit, tt.wantExit, output)
			}
			if tt.wantErrorText != "" && !strings.Contains(strings.Join(strings.Fields(string(output)), " "), tt.wantErrorText) {
				t.Fatalf("output = %q, want text %q", output, tt.wantErrorText)
			}
			if tt.wantBash != "" {
				selected, err := os.ReadFile(capture)
				if err != nil {
					t.Fatalf("ReadFile(capture) error = %v; output: %s", err, output)
				}
				want := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(tt.wantBash, `\`, "/")))
				if !strings.EqualFold(strings.TrimSpace(string(selected)), want) {
					t.Fatalf("selected Bash = %q, want %q", selected, want)
				}
			}
		})
	}
}

func buildWindowsShimHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	program := `package main
import ("os"; "strconv")
func main() {
	if path := os.Getenv("GGA_TEST_CAPTURE"); path != "" { _ = os.WriteFile(path, []byte(os.Args[0]), 0644) }
	code, _ := strconv.Atoi(os.Getenv("GGA_TEST_EXIT"))
	os.Exit(code)
}`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatalf("WriteFile(helper source) error = %v", err)
	}
	helper := filepath.Join(dir, "shim-helper.exe")
	cmd := exec.Command("go", "build", "-o", helper, source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build shim helper: %v\n%s", err, output)
	}
	return helper
}

func copyShimHelper(t *testing.T, source, destination string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(destination), err)
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", source, err)
	}
	if err := os.WriteFile(destination, content, 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", destination, err)
	}
}

// ---------------------------------------------------------------------------
// Asset embedding
// ---------------------------------------------------------------------------

// TestAssetGGAPS1IsEmbeddedAndReadable verifies the gga.ps1 asset is
// correctly embedded and can be read via the assets package.
func TestAssetGGAPS1IsEmbeddedAndReadable(t *testing.T) {
	content, err := assets.Read("gga/gga.ps1")
	if err != nil {
		t.Fatalf("assets.Read(\"gga/gga.ps1\") error = %v", err)
	}
	if content == "" {
		t.Fatal("assets.Read(\"gga/gga.ps1\") returned empty content")
	}
	if !strings.Contains(content, "Get-Command git") {
		t.Fatalf("embedded gga.ps1 missing expected content, got: %s", content)
	}
}
