package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v2/internal/doctor"
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
	if got.Remedy == nil {
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
	// chmod 0o755 so the Unix PATH-executable filter (#709) treats them as
	// real binaries; Windows ignores the mode bit, so the chmod is a no-op there.
	for _, dir := range []string{dir1, dir2} {
		p := filepath.Join(dir, "engram")
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	lookPathFn = func(string) (string, error) { return filepath.Join(dir1, "engram"), nil }

	got := checkOneTool("engram", []string{dir1, dir2})

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn, got %s", got.Status)
	}
	if !strings.Contains(got.Detail, "2 copies found") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
	if got.Remedy == nil {
		t.Error("expected non-empty remedy")
	}
}

func TestDoctorToolCopies_DeduplicatesSymlinkedPathDirectories(t *testing.T) {
	origExts := executableExtsFn
	defer func() { executableExtsFn = origExts }()
	executableExtsFn = func() []string { return []string{""} }

	root := t.TempDir()
	usrBin := filepath.Join(root, "usr", "bin")
	if err := os.MkdirAll(usrBin, 0o755); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(usrBin, "code")
	if err := os.WriteFile(code, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(root, "bin")
	if err := os.Symlink(usrBin, bin); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	got := doctorToolCopies("code", []string{bin, usrBin})
	if len(got) != 1 {
		t.Fatalf("copies = %v, want one copy", got)
	}
	if want := filepath.Join(bin, "code"); got[0] != want {
		t.Fatalf("copies[0] = %q, want first PATH entry %q", got[0], want)
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

func TestCheckInstalledAssetVersion_MatchingPass(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"installed_binary_version":%q}`, AppVersion)
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	got := checkInstalledAssetVersion(homeDir)
	if got.Status != CheckStatusPass {
		t.Errorf("expected pass, got %s: %s", got.Status, got.Detail)
	}
}

func TestCheckInstalledAssetVersion_SkewWarning(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"installed_binary_version":"v0.9.0"}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	got := checkInstalledAssetVersion(homeDir)
	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn for version skew, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "v0.9.0") || !strings.Contains(got.Detail, "gentle-ai sync") {
		t.Errorf("unexpected detail: %s", got.Detail)
	}
}

// --- checkEngramReachable ---

// setStdioProbeForTest pins the stdio MCP probe outcome for one test.
func setStdioProbeForTest(t *testing.T, err error) {
	t.Helper()
	orig := engramProbeStdioFn
	engramProbeStdioFn = func(context.Context, time.Duration, string, ...string) error { return err }
	t.Cleanup(func() { engramProbeStdioFn = orig })
}

func writeDoctorEngramConfig(t *testing.T, homeDir, command string, args []string) string {
	t.Helper()
	path := filepath.Join(homeDir, ".claude.json")
	content := fmt.Sprintf(`{"mcpServers":{"engram":{"command":%q,"args":[`, command)
	for i, arg := range args {
		if i > 0 {
			content += ","
		}
		content += fmt.Sprintf("%q", arg)
	}
	content += `]}}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func doctorStdioConfig(t *testing.T) (string, []string) {
	t.Helper()
	homeDir := t.TempDir()
	writeDoctorEngramConfig(t, homeDir, "engram", []string{"mcp", "--tools=agent"})
	return homeDir, []string{"claude-code"}
}

// TestCheckEngramReachable_ExplicitHTTP_OK is matrix cell (c) of #2078: the
// user declared an HTTP deployment via ENGRAM_BASE_URL, so the check probes
// exactly that URL. Uses a real listener, not a stubbed httpGetFn, to prove
// the HTTP path by execution.
func TestCheckEngramReachable_ExplicitHTTP_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv(engramHealthEnvVar, server.URL)

	got := checkEngramReachable(context.Background(), "", nil)

	if got.Status != CheckStatusPass {
		t.Errorf("expected pass, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, server.URL) {
		t.Errorf("detail should name the probed URL %s, got %q", server.URL, got.Detail)
	}
}

func TestCheckEngramReachable_ExplicitHTTP_NonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv(engramHealthEnvVar, server.URL)

	got := checkEngramReachable(context.Background(), "", nil)

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn for 503, got %s", got.Status)
	}
}

func TestCheckEngramReachable_ExplicitHTTP_ConnectionRefused(t *testing.T) {
	// Bind a real listener, capture its address, then close it so the port is
	// known-dead.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	deadURL := server.URL
	server.Close()
	t.Setenv(engramHealthEnvVar, deadURL)

	got := checkEngramReachable(context.Background(), "", nil)

	if got.Status != CheckStatusFail {
		t.Errorf("expected fail, got %s", got.Status)
	}
	if got.Remedy == nil {
		t.Error("expected non-empty remedy")
	}
	if !strings.Contains(got.Detail, engramHealthEnvVar) {
		t.Errorf("detail should attribute the URL to %s, got %q", engramHealthEnvVar, got.Detail)
	}
}

// TestCheckEngramReachable_StdioFailure verifies that a stdio transport that
// cannot complete the initialize handshake is a genuine failure with an
// actionable remedy.
func TestCheckEngramReachable_StdioFailure(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, errors.New("engram mcp exited without answering initialize"))
	homeDir, installedAgents := doctorStdioConfig(t)

	got := checkEngramReachable(context.Background(), homeDir, installedAgents)

	if got.Status != CheckStatusFail {
		t.Errorf("expected fail, got %s: %s", got.Status, got.Detail)
	}
	if got.Remedy == nil {
		t.Error("expected non-empty remedy")
	}
}

// TestCheckEngramReachable_BinaryAbsent_Warns verifies that the doctor's
// ErrNotInstalled mapping remains a warning because tool:engram owns the
// binary-presence check.
func TestCheckEngramReachable_BinaryAbsent_Warns(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, engram.ErrNotInstalled)
	homeDir, installedAgents := doctorStdioConfig(t)

	got := checkEngramReachable(context.Background(), homeDir, installedAgents)

	if got.Status != CheckStatusWarn {
		t.Errorf("expected warn, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "not probed") {
		t.Errorf("detail should say not probed, got %q", got.Detail)
	}
}

// TestCheckEngramReachable_MissingRelativeWindowsExecutableWarns proves the
// persisted Windows executable spelling remains advisory when ProbeStdio maps
// its missing-path error to ErrNotInstalled.
func TestCheckEngramReachable_MissingRelativeWindowsExecutableWarns(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, engram.ErrNotInstalled)
	homeDir := t.TempDir()
	configPath := writeDoctorEngramConfig(t, homeDir, "engram.exe", []string{"mcp", "--tools=agent"})

	got := checkEngramReachable(context.Background(), homeDir, []string{"claude-code"})

	if got.Status != CheckStatusWarn {
		t.Fatalf("engram:reachable status = %q, detail = %q; want not-probed warning for missing relative engram.exe", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, "not probed") || !strings.Contains(got.Detail, configPath) {
		t.Fatalf("engram:reachable detail = %q, want documented not-probed warning for %q", got.Detail, configPath)
	}
}

// TestCheckEngramReachable_StdioDefault_NoHTTPListener is matrix cell (a) of
// #2078: a default install. gentle-ai only ever configures Engram as a stdio
// MCP server (engram mcp --tools=agent, written by
// internal/components/engram/inject.go); it never configures an HTTP
// transport. With the stdio transport healthy and no HTTP listener anywhere,
// the reachability check must pass instead of guessing an HTTP address and
// reporting a false unhealthy.
func TestCheckEngramReachable_StdioDefault_NoHTTPListener(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, nil)
	homeDir, installedAgents := doctorStdioConfig(t)

	httpCalls := 0
	orig := httpGetFn
	defer func() { httpGetFn = orig }()
	httpGetFn = func(url string, _ time.Duration) (int, error) {
		httpCalls++
		return 0, fmt.Errorf("Get %q: dial tcp: connect: connection refused", url)
	}

	got := checkEngramReachable(context.Background(), homeDir, installedAgents)

	if got.Status != CheckStatusPass {
		t.Fatalf("engram:reachable status = %q, detail = %q; want pass: stdio MCP is configured and healthy, no HTTP listener exists", got.Status, got.Detail)
	}
	if httpCalls != 0 {
		t.Fatalf("check attempted %d HTTP request(s) without ENGRAM_BASE_URL set; want 0 (no guessed URL)", httpCalls)
	}
}

func TestCheckEngramReachable_StdioUsesPersistedConfiguration(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	homeDir := t.TempDir()
	configPath := writeDoctorEngramConfig(t, homeDir, "/configured/engram", []string{"mcp", "--tools=custom"})

	var gotCommand string
	var gotArgs []string
	orig := engramProbeStdioFn
	engramProbeStdioFn = func(_ context.Context, _ time.Duration, command string, args ...string) error {
		gotCommand = command
		gotArgs = args
		return nil
	}
	t.Cleanup(func() { engramProbeStdioFn = orig })

	got := checkEngramReachable(context.Background(), homeDir, []string{"claude-code"})

	if got.Status != CheckStatusPass {
		t.Fatalf("expected pass, got %s: %s", got.Status, got.Detail)
	}
	if gotCommand != "/configured/engram" || strings.Join(gotArgs, "\x00") != "mcp\x00--tools=custom" {
		t.Fatalf("probe = %q %v, want persisted command and args", gotCommand, gotArgs)
	}
	if !strings.Contains(got.Detail, configPath) {
		t.Fatalf("detail = %q, want persisted configuration path %q", got.Detail, configPath)
	}
}

func TestCheckEngramReachable_WhitespaceBaseURLUsesStdio(t *testing.T) {
	t.Setenv(engramHealthEnvVar, " \t\n ")
	setStdioProbeForTest(t, nil)
	homeDir, installedAgents := doctorStdioConfig(t)

	httpCalls := 0
	orig := httpGetFn
	t.Cleanup(func() { httpGetFn = orig })
	httpGetFn = func(string, time.Duration) (int, error) {
		httpCalls++
		return 0, errors.New("HTTP must not be selected for whitespace")
	}

	got := checkEngramReachable(context.Background(), homeDir, installedAgents)
	if got.Status != CheckStatusPass {
		t.Fatalf("expected stdio pass for whitespace-only %s, got %s: %s", engramHealthEnvVar, got.Status, got.Detail)
	}
	if httpCalls != 0 {
		t.Fatalf("HTTP calls = %d, want 0 for whitespace-only %s", httpCalls, engramHealthEnvVar)
	}
}

func TestCheckEngramReachable_ExplicitHTTPSurroundingWhitespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv(engramHealthEnvVar, " \t"+server.URL+"\n")

	got := checkEngramReachable(context.Background(), "", nil)
	if got.Status != CheckStatusPass {
		t.Fatalf("expected pass for surrounding whitespace, got %s: %s", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, server.URL) {
		t.Fatalf("detail = %q, want trimmed URL %q", got.Detail, server.URL)
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
	if got.Remedy == nil {
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
	origExecutable := osExecutableDoctor
	defer func() {
		lookPathFn = origLookPath
		availableBytesFn = origAvail
		httpGetFn = origHTTP
		pathDirsFn = origPathDirs
		osUserHomeDirDoctor = origHomeDir
		osExecutableDoctor = origExecutable
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
	configPath := writeDoctorEngramConfig(t, homeDir, "engram", []string{"mcp", "--tools=agent"})

	lookPathFn = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	availableBytesFn = func(string) (int64, error) { return 1024 * 1024 * 1024, nil } // 1 GB
	httpGetFn = func(string, time.Duration) (int, error) { return 200, nil }
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, nil)
	pathSnapshots := 0
	pathDirsFn = func() []string { pathSnapshots++; return []string{"/usr/local/bin"} }
	osUserHomeDirDoctor = func() (string, error) { return homeDir, nil }
	// organic-dx Phase 3f task 3f.5 added an invoked-executable clause to the
	// gentle-ai tool check; this integration test is about RunDoctor's overall
	// rendering shape, not that feature, so it mocks the invoked executable to
	// match the PATH-resolved copy exactly (the common case) to keep the
	// expected output byte-identical to before that feature landed. The new
	// feature has its own dedicated tests in doctor_invoked_binary_test.go.
	osExecutableDoctor = func() (string, error) { return "/usr/local/bin/gentle-ai", nil }

	var buf bytes.Buffer
	if err := RunDoctor(context.Background(), &buf); err != nil {
		t.Fatalf("RunDoctor returned error: %v", err)
	}

	want := fmt.Sprintf(`gentle-ai doctor — system health check
=======================================

  [ok]  tool:gentle-ai                 gentle-ai found at /usr/local/bin/gentle-ai; invoked executable: /usr/local/bin/gentle-ai (version dev)
  [ok]  tool:gga                       gga found at /usr/local/bin/gga
  [ok]  tool:engram                    engram found at /usr/local/bin/engram
  [ok]  tool:claude                    claude found at /usr/local/bin/claude
  [ok]  state:json                     state file OK — 1 agent(s) installed: claude-code
  [ok]  installed:asset_version        no installed binary version recorded in state file — check skipped
  [ok]  engram:reachable               engram MCP (stdio) answered the initialize handshake for persisted configuration: %s
  [ok]  disk:space                     1024 MB free on %s filesystem

Summary: 8 passed, 0 failed, 0 warnings
Status:  healthy
`, configPath, filepath.Join(homeDir, ".gentle-ai"))
	if got := buf.String(); got != want {
		t.Fatalf("RunDoctor output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	if pathSnapshots != 1 {
		t.Fatalf("PATH snapshots = %d, want 1", pathSnapshots)
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

// --- #709: derive required agents from state.json ---

// TestCheckToolBinaries_DerivesAgentsFromInstalled verifies that agents
// listed in state.json's InstalledAgents field are added to the required-tool
// set and produce a fail check when their binary is not on PATH (#709).
func TestCheckToolBinaries_DerivesAgentsFromInstalled(t *testing.T) {
	orig := lookPathFn
	origGOOS := doctorGOOS
	defer func() {
		lookPathFn = orig
		doctorGOOS = origGOOS
	}()
	// Pin to a non-Windows platform so the PowerShell-shim fallback in
	// resolveDoctorTool does not mask a missing binary.
	doctorGOOS = "darwin"
	lookPathFn = func(name string) (string, error) {
		if name == "claude" {
			return "", errors.New("not found")
		}
		return "/usr/local/bin/" + name, nil
	}

	results := checkToolBinaries([]string{"/usr/local/bin"}, []string{"claude-code"})

	var claudeFailed bool
	for _, r := range results {
		if r.Name == "tool:claude" && r.Status == CheckStatusFail {
			claudeFailed = true
		}
	}
	if !claudeFailed {
		t.Errorf("expected tool:claude to fail when claude-code is installed but binary is missing; got %+v", results)
	}
}

// TestCheckToolBinaries_AgentNotInState_NotReported verifies that agents NOT
// in state.json's InstalledAgents are not added to the required set, so a
// user with only pi installed is not flagged as unhealthy because opencode
// is missing (#709).
func TestCheckToolBinaries_AgentNotInState_NotReported(t *testing.T) {
	orig := lookPathFn
	defer func() { lookPathFn = orig }()
	// Pretend everything is missing — only the actually required tools should
	// appear in the report.
	lookPathFn = func(string) (string, error) { return "", errors.New("not found") }

	results := checkToolBinaries([]string{"/usr/local/bin"}, []string{"pi"})

	for _, r := range results {
		if r.Name == "tool:opencode" {
			t.Errorf("did not expect tool:opencode to appear when state lists only pi; got %+v", r)
		}
		if r.Name == "tool:claude" {
			t.Errorf("did not expect tool:claude to appear when state lists only pi; got %+v", r)
		}
	}

	// Core tools must still be required.
	var sawGentleAI, sawEngram, sawPi bool
	for _, r := range results {
		switch r.Name {
		case "tool:gentle-ai":
			sawGentleAI = true
		case "tool:engram":
			sawEngram = true
		case "tool:pi":
			sawPi = true
		}
	}
	if !sawGentleAI || !sawEngram || !sawPi {
		t.Errorf("expected core+pi checks present; got sawGentleAI=%v sawEngram=%v sawPi=%v", sawGentleAI, sawEngram, sawPi)
	}
}

// TestCheckToolBinaries_StateMissing_ChecksCoreOnly verifies the
// first-time-install contract from #709: when state.json is unreadable the
// doctor must still report the always-required core tools but should not
// pretend the user has selected any agents (RunDoctor handles this by
// passing a nil/empty list to checkToolBinaries).
func TestCheckToolBinaries_StateMissing_ChecksCoreOnly(t *testing.T) {
	orig := lookPathFn
	defer func() { lookPathFn = orig }()
	lookPathFn = func(string) (string, error) { return "/usr/local/bin/" + "missing", nil }

	results := checkToolBinaries([]string{"/usr/local/bin"}, nil)

	required := make(map[string]struct{}, len(results))
	for _, r := range results {
		required[string(r.Name)] = struct{}{}
	}
	for _, core := range []string{"tool:gentle-ai", "tool:gga", "tool:engram"} {
		if _, ok := required[core]; !ok {
			t.Errorf("expected %s in core-only output, got %+v", core, required)
		}
	}
	for _, forbidden := range []string{"tool:claude", "tool:opencode"} {
		if _, ok := required[forbidden]; ok {
			t.Errorf("did not expect %s when state is missing, got %+v", forbidden, required)
		}
	}
}

// TestCheckOneTool_DirectoryWithToolName_NotCountedAsDuplicate verifies that
// a directory whose name happens to match a tool (e.g. an existing
// $HOME/gentle-ai/ on PATH) is not treated as a duplicate binary copy by
// the doctor (#709).
func TestCheckOneTool_DirectoryWithToolName_NotCountedAsDuplicate(t *testing.T) {
	orig := lookPathFn
	origExts := executableExtsFn
	origGOOS := doctorGOOS
	defer func() {
		lookPathFn = orig
		executableExtsFn = origExts
		doctorGOOS = origGOOS
	}()
	doctorGOOS = "darwin"
	executableExtsFn = func() []string { return []string{""} }

	dirWithFile := t.TempDir()
	dirWithDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dirWithFile, "gentle-ai"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dirWithDir, "gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}

	lookPathFn = func(string) (string, error) {
		return filepath.Join(dirWithFile, "gentle-ai"), nil
	}

	got := checkOneTool("gentle-ai", []string{dirWithFile, dirWithDir})

	if got.Status != CheckStatusPass {
		t.Fatalf("expected pass when only one real binary exists; got %s: %s", got.Status, got.Detail)
	}
	if strings.Contains(got.Detail, "copies") {
		t.Errorf("did not expect duplicate-copy warning when match is a directory, got %s", got.Detail)
	}
}

// TestCheckOneTool_FileWithoutExecBit_NotCountedAsDuplicate verifies that on
// non-Windows platforms a regular file without an execute bit is not counted
// as a binary by the doctor — only directories were excluded before #709;
// non-executable files were still flagged as duplicates.
func TestCheckOneTool_FileWithoutExecBit_NotCountedAsDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix execute-bit semantics do not apply on Windows")
	}

	orig := lookPathFn
	origExts := executableExtsFn
	origGOOS := doctorGOOS
	defer func() {
		lookPathFn = orig
		executableExtsFn = origExts
		doctorGOOS = origGOOS
	}()
	doctorGOOS = "darwin"
	executableExtsFn = func() []string { return []string{""} }

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// One executable copy, one non-executable copy.
	if err := os.WriteFile(filepath.Join(dir1, "gentle-ai"), []byte("#!/bin/sh\nexit 0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "gentle-ai"), []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}

	lookPathFn = func(string) (string, error) { return filepath.Join(dir1, "gentle-ai"), nil }

	got := checkOneTool("gentle-ai", []string{dir1, dir2})

	if got.Status != CheckStatusPass {
		t.Fatalf("expected pass when second copy lacks the execute bit; got %s: %s", got.Status, got.Detail)
	}
	if strings.Contains(got.Detail, "copies") {
		t.Errorf("did not expect duplicate-copy warning for non-executable file, got %s", got.Detail)
	}
}

// TestRunDoctor_OnlySelectedAgentsAreRequired wires the full RunDoctor flow
// end-to-end with state.json containing pi and verifies that opencode —
// although not on PATH — is NOT in the rendered report (because it is not
// in state.json's InstalledAgents). This is the headline scenario from #709.
func TestRunDoctor_OnlySelectedAgentsAreRequired(t *testing.T) {
	origLook := lookPathFn
	origAvail := availableBytesFn
	origHTTP := httpGetFn
	origPath := pathDirsFn
	origHome := osUserHomeDirDoctor
	origGOOS := doctorGOOS
	origExts := executableExtsFn
	defer func() {
		lookPathFn = origLook
		availableBytesFn = origAvail
		httpGetFn = origHTTP
		pathDirsFn = origPath
		osUserHomeDirDoctor = origHome
		doctorGOOS = origGOOS
		executableExtsFn = origExts
	}()
	doctorGOOS = "darwin"
	executableExtsFn = func() []string { return []string{""} }

	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	piDir := filepath.Join(homeDir, ".pi")
	if err := os.MkdirAll(piDir, 0o755); err != nil {
		t.Fatal(err)
	}
	piMCPPath := filepath.Join(homeDir, ".pi", "agent", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(piMCPPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(piMCPPath, []byte(`{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := `{"installed_agents":["pi"]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	// Resolve all binaries successfully except opencode, which is not in
	// state.json anyway and must therefore never appear in the report.
	lookPathFn = func(name string) (string, error) {
		if name == "opencode" {
			return "", errors.New("opencode intentionally missing")
		}
		return "/usr/local/bin/" + name, nil
	}
	availableBytesFn = func(string) (int64, error) { return 1024 * 1024 * 1024, nil }
	httpGetFn = func(string, time.Duration) (int, error) { return 200, nil }
	pathDirsFn = func() []string { return []string{"/usr/local/bin"} }
	osUserHomeDirDoctor = func() (string, error) { return homeDir, nil }
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, nil)

	var buf bytes.Buffer
	if err := RunDoctor(context.Background(), &buf); err != nil {
		t.Fatalf("RunDoctor returned error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "tool:opencode") {
		t.Errorf("did not expect tool:opencode in the rendered report when opencode is not in state.json; got:\n%s", output)
	}
	if strings.Contains(output, "tool:claude") {
		t.Errorf("did not expect tool:claude in the rendered report when claude-code is not in state.json; got:\n%s", output)
	}
	if !strings.Contains(output, "tool:pi") {
		t.Errorf("expected tool:pi in the rendered report; got:\n%s", output)
	}
	if !strings.Contains(output, "Status:  healthy") {
		t.Errorf("expected healthy status for pi-only install with all binaries present; got:\n%s", output)
	}
}

func TestRenderDoctorReportDoesNotRenderRemedyMetadata(t *testing.T) {
	var buf bytes.Buffer
	renderDoctorReport(&buf, DoctorReport{Checks: []CheckResult{{Name: doctor.CheckDiskSpace, Status: CheckStatusFail, Detail: "cleanup needed", Remedy: doctor.NewRemedy(doctor.RemedyFreeDiskSpace, "Free disk space")}}})
	want := "gentle-ai doctor — system health check\n=======================================\n\n  [xx]  disk:space                     cleanup needed\n       Remedy: Free disk space\n\nSummary: 0 passed, 1 failed, 0 warnings\nStatus:  unhealthy\n"
	if got := buf.String(); got != want {
		t.Fatalf("rendered report mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
