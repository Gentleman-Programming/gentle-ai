package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// --- checkOneTool ---

func TestCheckOneTool_MissingBinary(t *testing.T) {
	orig := lookPathFn
	defer func() { lookPathFn = orig }()
	lookPathFn = func(string) (string, error) { return "", errors.New("not found") }

	got := checkOneTool("engram", nil)

	if got.Status != CheckStatusFail {
		t.Errorf("expected fail, got %s", got.Status)
	}
	if !strings.Contains(got.Detail, "not found in PATH") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
	if got.Remedy == "" {
		t.Error("expected non-empty remedy")
	}
}

func TestCheckOneTool_ShadowedBinary(t *testing.T) {
	orig := lookPathFn
	defer func() { lookPathFn = orig }()
	origExts := executableExtsFn
	defer func() { executableExtsFn = origExts }()
	executableExtsFn = func() []string { return []string{""} }

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Create two copies of the "engram" binary in two dirs.
	for _, dir := range []string{dir1, dir2} {
		f, err := os.Create(filepath.Join(dir, "engram"))
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}

	lookPathFn = func(string) (string, error) { return filepath.Join(dir1, "engram"), nil }

	got := checkOneTool("engram", []string{dir1, dir2})

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn, got %s", got.Status)
	}
	if !strings.Contains(got.Detail, "2 copies found") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
	if got.Remedy == "" {
		t.Error("expected non-empty remedy")
	}
}

func TestCheckOneTool_OK(t *testing.T) {
	orig := lookPathFn
	defer func() { lookPathFn = orig }()
	origExts := executableExtsFn
	defer func() { executableExtsFn = origExts }()
	executableExtsFn = func() []string { return []string{""} }

	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "engram"))
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	lookPathFn = func(string) (string, error) { return filepath.Join(dir, "engram"), nil }

	got := checkOneTool("engram", []string{dir})

	if got.Status != CheckStatusPass {
		t.Errorf("expected pass, got %s: %s", got.Status, got.Detail)
	}
}

// TestCheckOneTool_ShadowedWindowsExt reproduces the Windows bug: binaries on
// disk carry an executable extension (e.g. gentle-ai.exe / gentle-ai.cmd), so a
// bare-name scan misses them and shadowing is reported as [ok]. With PATHEXT
// extensions the duplicate copies are detected and a warning is produced.
func TestCheckOneTool_ShadowedWindowsExt(t *testing.T) {
	origLook := lookPathFn
	origGOOS := doctorGOOS
	origExts := executableExtsFn
	defer func() {
		lookPathFn = origLook
		doctorGOOS = origGOOS
		executableExtsFn = origExts
	}()
	doctorGOOS = "windows"
	executableExtsFn = func() []string { return []string{".exe", ".cmd"} }

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	for _, p := range []string{filepath.Join(dir1, "gentle-ai.exe"), filepath.Join(dir2, "gentle-ai.cmd")} {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}

	lookPathFn = func(string) (string, error) { return filepath.Join(dir1, "gentle-ai.exe"), nil }

	got := checkOneTool("gentle-ai", []string{dir1, dir2})

	if got.Status != CheckStatusWarn {
		t.Fatalf("expected warn for extensioned shadow, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "2 copies found") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
}

func TestCheckOneTool_WindowsPowerShellShimFallback(t *testing.T) {
	origLook := lookPathFn
	origGOOS := doctorGOOS
	origExts := executableExtsFn
	defer func() {
		lookPathFn = origLook
		doctorGOOS = origGOOS
		executableExtsFn = origExts
	}()
	doctorGOOS = "windows"
	executableExtsFn = func() []string { return []string{".exe", ".cmd"} }

	dir := t.TempDir()
	ps1Path := filepath.Join(dir, "gga.ps1")
	if err := os.WriteFile(ps1Path, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	lookPathFn = func(file string) (string, error) {
		if file == "gga.ps1" {
			return ps1Path, nil
		}
		return "", errors.New("not found")
	}

	got := checkOneTool("gga", []string{dir})

	if got.Status != CheckStatusPass {
		t.Fatalf("expected pass, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "PowerShell shim") {
		t.Fatalf("expected PowerShell shim detail, got %q", got.Detail)
	}
}

func TestCheckOneTool_WindowsShimVariantsInSameDirAreNotDuplicates(t *testing.T) {
	origLook := lookPathFn
	origGOOS := doctorGOOS
	origExts := executableExtsFn
	defer func() {
		lookPathFn = origLook
		doctorGOOS = origGOOS
		executableExtsFn = origExts
	}()
	doctorGOOS = "windows"
	executableExtsFn = func() []string { return []string{".cmd"} }

	dir := t.TempDir()
	cmdPath := filepath.Join(dir, "gga.cmd")
	for _, path := range []string{cmdPath, filepath.Join(dir, "gga.ps1")} {
		if err := os.WriteFile(path, []byte("fake"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	lookPathFn = func(file string) (string, error) {
		if file == "gga" {
			return cmdPath, nil
		}
		return "", errors.New("not found")
	}

	got := checkOneTool("gga", []string{dir})

	if got.Status != CheckStatusPass {
		t.Fatalf("expected pass for same-directory shim variants, got %s: %s", got.Status, got.Detail)
	}
}

// TestExecutableExtensions verifies the per-platform extension set.
func TestExecutableExtensions(t *testing.T) {
	exts := executableExtensions()
	if len(exts) == 0 {
		t.Fatal("expected at least one extension")
	}
	if runtime.GOOS == "windows" {
		var hasExe bool
		for _, e := range exts {
			if e == ".exe" {
				hasExe = true
			}
		}
		if !hasExe {
			t.Errorf("expected .exe among Windows extensions, got %v", exts)
		}
	} else if len(exts) != 1 || exts[0] != "" {
		t.Errorf(`expected [""] on non-Windows, got %v`, exts)
	}
}

func TestExecutableExtensionsFor(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		pathext string
		want    []string
	}{
		{
			name: "non-windows uses bare name",
			goos: "darwin",
			want: []string{""},
		},
		{
			name: "windows default PATHEXT",
			goos: "windows",
			want: []string{".com", ".exe", ".bat", ".cmd"},
		},
		{
			name:    "windows normalizes PATHEXT case and missing dots",
			goos:    "windows",
			pathext: "EXE;.Cmd; ;BAT",
			want:    []string{".exe", ".cmd", ".bat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executableExtensionsFor(tt.goos, tt.pathext)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// --- checkStateJSON ---

func TestCheckStateJSON_Missing(t *testing.T) {
	homeDir := t.TempDir()

	got := checkStateJSON(homeDir)

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn for missing state, got %s", got.Status)
	}
	if !strings.Contains(got.Detail, "not found") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
}

func TestCheckStateJSON_Malformed(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := checkStateJSON(homeDir)

	if got.Status != CheckStatusFail {
		t.Errorf("expected fail for malformed state, got %s", got.Status)
	}
	if !strings.Contains(got.Detail, "failed to parse") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
}

func TestCheckStateJSON_AgentConfigDirMissing(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// claude-code config dir will NOT exist
	payload := `{"installed_agents":["claude-code"]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	got := checkStateJSON(homeDir)

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn for missing config dir, got %s", got.Status)
	}
	if !strings.Contains(got.Detail, "config dirs are missing") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
}

func TestCheckStateJSON_OK(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create config dir for claude-code
	claudeDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"installed_agents":["claude-code"]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	got := checkStateJSON(homeDir)

	if got.Status != CheckStatusPass {
		t.Errorf("expected pass, got %s: %s", got.Status, got.Detail)
	}
}

// --- checkEngramReachable ---

func TestCheckEngramReachable_ConnectionRefused(t *testing.T) {
	orig := httpGetFn
	defer func() { httpGetFn = orig }()
	httpGetFn = func(url string, _ time.Duration) (int, error) {
		return 0, errors.New("connection refused")
	}

	got := checkEngramReachable()

	if got.Status != CheckStatusFail {
		t.Errorf("expected fail, got %s", got.Status)
	}
	if got.Remedy == "" {
		t.Error("expected non-empty remedy")
	}
}

func TestCheckEngramReachable_OK(t *testing.T) {
	orig := httpGetFn
	defer func() { httpGetFn = orig }()
	httpGetFn = func(url string, _ time.Duration) (int, error) {
		return 200, nil
	}

	got := checkEngramReachable()

	if got.Status != CheckStatusPass {
		t.Errorf("expected pass, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckEngramReachable_NonSuccessStatus(t *testing.T) {
	orig := httpGetFn
	defer func() { httpGetFn = orig }()
	httpGetFn = func(url string, _ time.Duration) (int, error) {
		return 503, nil
	}

	got := checkEngramReachable()

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn for 503, got %s", got.Status)
	}
}

// --- checkDiskSpace ---

func TestCheckDiskSpace_CriticallyLow(t *testing.T) {
	orig := availableBytesFn
	defer func() { availableBytesFn = orig }()
	availableBytesFn = func(string) (int64, error) { return diskFailThreshold - 1, nil }

	got := checkDiskSpace(t.TempDir())

	if got.Status != CheckStatusFail {
		t.Errorf("expected fail, got %s", got.Status)
	}
	if got.Remedy == "" {
		t.Error("expected non-empty remedy")
	}
}

func TestCheckDiskSpace_Low(t *testing.T) {
	orig := availableBytesFn
	defer func() { availableBytesFn = orig }()
	// Between fail and warn thresholds
	availableBytesFn = func(string) (int64, error) { return diskFailThreshold + 1, nil }

	got := checkDiskSpace(t.TempDir())

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn, got %s", got.Status)
	}
}

func TestCheckDiskSpace_OK(t *testing.T) {
	orig := availableBytesFn
	defer func() { availableBytesFn = orig }()
	availableBytesFn = func(string) (int64, error) { return diskWarnThreshold * 2, nil }

	got := checkDiskSpace(t.TempDir())

	if got.Status != CheckStatusPass {
		t.Errorf("expected pass, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckDiskSpace_StatError(t *testing.T) {
	orig := availableBytesFn
	defer func() { availableBytesFn = orig }()
	availableBytesFn = func(string) (int64, error) { return 0, errors.New("stat error") }

	got := checkDiskSpace(t.TempDir())

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn for stat error, got %s", got.Status)
	}
}

// --- RunDoctor integration test ---

func TestRunDoctor_IntegrationAllMocked(t *testing.T) {
	// Mock all external dependencies.
	origLookPath := lookPathFn
	origAvail := availableBytesFn
	origHTTP := httpGetFn
	origPathDirs := pathDirsFn
	origHomeDir := osUserHomeDirDoctor
	defer func() {
		lookPathFn = origLookPath
		availableBytesFn = origAvail
		httpGetFn = origHTTP
		pathDirsFn = origPathDirs
		osUserHomeDirDoctor = origHomeDir
	}()

	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"installed_agents":["claude-code"]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	lookPathFn = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	availableBytesFn = func(string) (int64, error) { return 1024 * 1024 * 1024, nil } // 1 GB
	httpGetFn = func(string, time.Duration) (int, error) { return 200, nil }
	pathDirsFn = func() []string { return []string{"/usr/local/bin"} }
	osUserHomeDirDoctor = func() (string, error) { return homeDir, nil }

	var buf bytes.Buffer
	if err := RunDoctor(context.Background(), &buf); err != nil {
		t.Fatalf("RunDoctor returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "gentle-ai doctor") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "Summary:") {
		t.Error("expected summary in output")
	}
	if !strings.Contains(output, "Status:") {
		t.Error("expected status in output")
	}
}

func TestRunDoctor_HomeDirError(t *testing.T) {
	orig := osUserHomeDirDoctor
	defer func() { osUserHomeDirDoctor = orig }()
	osUserHomeDirDoctor = func() (string, error) { return "", errors.New("no home dir") }

	var buf bytes.Buffer
	err := RunDoctor(context.Background(), &buf)
	if err == nil {
		t.Error("expected error when home dir fails")
	}
}
