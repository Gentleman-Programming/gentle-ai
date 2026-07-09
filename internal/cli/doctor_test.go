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

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/model"
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

// --- estimateTokens ---

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name  string
		chars int
		want  int
	}{
		{name: "zero chars", chars: 0, want: 0},
		{name: "exact multiple of four", chars: 400, want: 100},
		{name: "non-multiple rounds down", chars: 10, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.chars)
			if got != tt.want {
				t.Errorf("estimateTokens(%d) = %d, want %d", tt.chars, got, tt.want)
			}
		})
	}
}

// --- collectFootprint / footprintCheckResult ---

func footprintSubsetRegistry() (*agents.Registry, error) {
	return agents.NewRegistry(claude.NewAdapter(), opencode.NewAdapter())
}

func writeDoctorState(t *testing.T, homeDir string, agentIDs []string) {
	t.Helper()
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	quoted := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		quoted[i] = `"` + id + `"`
	}
	payload := `{"installed_agents":[` + strings.Join(quoted, ",") + `]}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeDoctorFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupFootprintSeams(t *testing.T, homeDir string) {
	t.Helper()
	origHome := osUserHomeDirDoctor
	origRegistry := newDoctorRegistry
	t.Cleanup(func() {
		osUserHomeDirDoctor = origHome
		newDoctorRegistry = origRegistry
	})
	osUserHomeDirDoctor = func() (string, error) { return homeDir, nil }
	newDoctorRegistry = footprintSubsetRegistry
}

func TestCollectFootprint(t *testing.T) {
	tests := []struct {
		name       string
		agentIDs   []string // nil skips writing state.json entirely
		fixtures   map[string]string
		wantStatus CheckStatus
		check      func(t *testing.T, sum FootprintSummary, result CheckResult)
	}{
		{
			name:     "healthy two agents",
			agentIDs: []string{"claude-code", "opencode"},
			fixtures: map[string]string{
				"claude-code": "<!-- gentle-ai:persona -->\nsome persona text\n<!-- /gentle-ai:persona -->\n",
				"opencode":    "<!-- gentle-ai:engram-protocol -->\nprotocol text here\n<!-- /gentle-ai:engram-protocol -->\n",
			},
			wantStatus: CheckStatusPass,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if sum.TotalBlocks != 2 || sum.AgentsCovered != 2 {
					t.Fatalf("expected 2 blocks across 2 agents, got %d/%d", sum.TotalBlocks, sum.AgentsCovered)
				}
				if sum.HasFail || sum.HasWarn {
					t.Fatalf("expected no anomalies, got HasFail=%v HasWarn=%v", sum.HasFail, sum.HasWarn)
				}
				if !strings.Contains(result.Detail, "2 block") {
					t.Errorf("expected detail to mention block count, got %q", result.Detail)
				}
			},
		},
		{
			name:       "orphan open fails",
			agentIDs:   []string{"claude-code"},
			fixtures:   map[string]string{"claude-code": "<!-- gentle-ai:persona -->\nunclosed content\n"},
			wantStatus: CheckStatusFail,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if !sum.HasFail {
					t.Fatal("expected HasFail true for unclosed marker")
				}
				if !strings.Contains(result.Detail, "claude-code") || !strings.Contains(result.Detail, "persona") {
					t.Errorf("expected detail to name agent and section id, got %q", result.Detail)
				}
				if result.Remedy == "" {
					t.Error("expected non-empty remedy")
				}
			},
		},
		{
			name:       "mismatch anomaly fails",
			agentIDs:   []string{"claude-code"},
			fixtures:   map[string]string{"claude-code": "<!-- gentle-ai:persona -->\ncontent\n<!-- /gentle-ai:other -->\n"},
			wantStatus: CheckStatusFail,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if !sum.HasFail {
					t.Fatal("expected HasFail true for id mismatch")
				}
				if !strings.Contains(result.Detail, "claude-code") || !strings.Contains(result.Detail, "other") {
					t.Errorf("expected detail to name agent and the offending closer id, got %q", result.Detail)
				}
				if result.Remedy == "" {
					t.Error("expected non-empty remedy")
				}
			},
		},
		{
			name:       "stray closer warns",
			agentIDs:   []string{"claude-code"},
			fixtures:   map[string]string{"claude-code": "<!-- /gentle-ai:persona -->\nsome text\n"},
			wantStatus: CheckStatusWarn,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if sum.HasFail {
					t.Fatal("expected HasFail false for stray closer")
				}
				if !sum.HasWarn {
					t.Fatal("expected HasWarn true for stray closer")
				}
			},
		},
		{
			name:       "state missing",
			wantStatus: CheckStatusWarn,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if !sum.StateMissing {
					t.Fatal("expected StateMissing true")
				}
				if !strings.Contains(result.Remedy, "install") {
					t.Errorf("expected install remedy, got %q", result.Remedy)
				}
			},
		},
		{
			name:       "no agents",
			agentIDs:   []string{},
			wantStatus: CheckStatusWarn,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if !sum.NoAgents {
					t.Fatal("expected NoAgents true")
				}
			},
		},
		{
			name:       "unresolved agent is tracked but not scanned",
			agentIDs:   []string{"unknown-agent"},
			wantStatus: CheckStatusPass,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if len(sum.Agents) != 1 || !sum.Agents[0].Unresolved {
					t.Fatalf("expected 1 unresolved agent entry, got %+v", sum.Agents)
				}
				if sum.AgentsCovered != 0 {
					t.Errorf("expected unresolved agent not counted in AgentsCovered, got %d", sum.AgentsCovered)
				}
			},
		},
		{
			name:       "absent instruction file is informational",
			agentIDs:   []string{"claude-code"},
			wantStatus: CheckStatusPass,
			check: func(t *testing.T, sum FootprintSummary, result CheckResult) {
				if len(sum.Agents) != 1 || sum.Agents[0].Present {
					t.Fatalf("expected Present false when instruction file is absent, got %+v", sum.Agents)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			setupFootprintSeams(t, homeDir)
			if tt.agentIDs != nil {
				writeDoctorState(t, homeDir, tt.agentIDs)
			}
			reg, _ := footprintSubsetRegistry()
			for id, content := range tt.fixtures {
				adapter, ok := reg.Get(model.AgentID(id))
				if !ok {
					t.Fatalf("no adapter for %q in subset registry", id)
				}
				writeDoctorFixture(t, adapter.SystemPromptFile(homeDir), content)
			}

			sum := collectFootprint(homeDir)
			result := footprintCheckResult(sum)
			if result.Status != tt.wantStatus {
				t.Fatalf("expected status %s, got %s: %s", tt.wantStatus, result.Status, result.Detail)
			}
			tt.check(t, sum, result)
		})
	}
}

func TestCollectFootprint_MalformedStateJSONIsNotConflatedWithEmptyAgents(t *testing.T) {
	homeDir := t.TempDir()
	setupFootprintSeams(t, homeDir)
	stateDir := filepath.Join(homeDir, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum := collectFootprint(homeDir)
	result := footprintCheckResult(sum)

	if !sum.StateUnreadable {
		t.Fatal("expected StateUnreadable true for malformed state.json")
	}
	if sum.NoAgents {
		t.Fatal("expected NoAgents false for malformed state.json — must not conflate a corrupt state file with a genuinely empty agent list")
	}
	if result.Status != CheckStatusFail {
		t.Errorf("expected Fail status for malformed state.json (mirroring checkStateJSON's severity), got %s", result.Status)
	}
	if strings.Contains(result.Detail, "no installed agents") {
		t.Errorf("malformed state.json must not reuse the NoAgents wording, got detail=%q", result.Detail)
	}
	if result.Remedy == "Run 'gentle-ai install' to configure agents" {
		t.Errorf("malformed state.json must not reuse the NoAgents remedy verbatim, got remedy=%q", result.Remedy)
	}
}

func TestCollectFootprint_RegistryConstructionFailureIsWarnNotNoAgents(t *testing.T) {
	homeDir := t.TempDir()
	origRegistry := newDoctorRegistry
	t.Cleanup(func() { newDoctorRegistry = origRegistry })
	newDoctorRegistry = func() (*agents.Registry, error) {
		return nil, errors.New("registry construction failed")
	}

	sum := collectFootprint(homeDir)
	result := footprintCheckResult(sum)

	if !sum.RegistryUnavailable {
		t.Fatal("expected RegistryUnavailable true when newDoctorRegistry fails")
	}
	if sum.NoAgents {
		t.Fatal("expected NoAgents false when registry construction fails")
	}
	if result.Status != CheckStatusWarn {
		t.Errorf("expected Warn status for registry construction failure, got %s", result.Status)
	}
}
