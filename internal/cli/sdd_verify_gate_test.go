package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// seedSDDVerifyGateChange builds an OpenSpec change whose tasks are fully
// complete; verifyReport, when supplied non-empty, is written verbatim so
// callers can produce clean, critical, or malformed evidence.
func seedSDDVerifyGateChange(t *testing.T, root, name, verifyReport string) string {
	t.Helper()
	changeRoot := filepath.Join(root, "openspec", "changes", name)
	writeSDDStatusFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "spec.md"), "# Spec\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n")
	if verifyReport != "" {
		writeSDDStatusFile(t, filepath.Join(changeRoot, "verify-report.md"), verifyReport)
	}
	return changeRoot
}

var sddVerifyGateCleanReport = "```yaml\n" +
	"schema: gentle-ai.verify-result/v1\n" +
	"evidence_revision: sha256:" + repeatHexDigit("a") + "\n" +
	"verdict: pass\n" +
	"blockers: 0\n" +
	"critical_findings: 0\n" +
	"requirements: 0/0\n" +
	"scenarios: 0/0\n" +
	"test_command: go test ./...\n" +
	"test_exit_code: 0\n" +
	"test_output_hash: sha256:" + repeatHexDigit("b") + "\n" +
	"build_command: go vet ./...\n" +
	"build_exit_code: 0\n" +
	"build_output_hash: sha256:" + repeatHexDigit("c") + "\n" +
	"```"

var sddVerifyGateCriticalReport = "```yaml\n" +
	"schema: gentle-ai.verify-result/v1\n" +
	"evidence_revision: sha256:" + repeatHexDigit("a") + "\n" +
	"verdict: fail\n" +
	"blockers: 1\n" +
	"critical_findings: 2\n" +
	"requirements: 0/0\n" +
	"scenarios: 0/0\n" +
	"test_command: go test ./...\n" +
	"test_exit_code: 1\n" +
	"test_output_hash: sha256:" + repeatHexDigit("b") + "\n" +
	"build_command: go vet ./...\n" +
	"build_exit_code: 0\n" +
	"build_output_hash: sha256:" + repeatHexDigit("c") + "\n" +
	"```"

func repeatHexDigit(digit string) string {
	out := ""
	for len(out) < 64 {
		out += digit
	}
	return out
}

func TestRunSDDVerifyGateCleanReportPassesAndReturnsNil(t *testing.T) {
	root := t.TempDir()
	seedSDDVerifyGateChange(t, root, "clean-change", sddVerifyGateCleanReport)

	var stdout bytes.Buffer
	err := RunSDDVerifyGate([]string{"clean-change", "--cwd", root}, &stdout)
	if err != nil {
		t.Fatalf("RunSDDVerifyGate() error = %v, want nil for clean report", err)
	}
	if !strings.Contains(stdout.String(), "clean-change") {
		t.Fatalf("stdout = %q, want change name mentioned", stdout.String())
	}
}

func TestRunSDDVerifyGateCriticalReportReturnsErrorForExitCode1(t *testing.T) {
	root := t.TempDir()
	seedSDDVerifyGateChange(t, root, "critical-change", sddVerifyGateCriticalReport)

	var stdout bytes.Buffer
	err := RunSDDVerifyGate([]string{"critical-change", "--cwd", root}, &stdout)
	if err == nil {
		t.Fatal("RunSDDVerifyGate() error = nil, want non-nil for CRITICAL report (must exit 1 via main.go)")
	}
}

func TestRunSDDVerifyGateAdvisoryModeNeverFails(t *testing.T) {
	root := t.TempDir()
	seedSDDVerifyGateChange(t, root, "critical-advisory", sddVerifyGateCriticalReport)

	var stdout bytes.Buffer
	err := RunSDDVerifyGate([]string{"critical-advisory", "--cwd", root, "--advisory"}, &stdout)
	if err != nil {
		t.Fatalf("RunSDDVerifyGate(--advisory) error = %v, want nil even for CRITICAL report", err)
	}
	if !strings.Contains(stdout.String(), "critical-advisory") {
		t.Fatalf("advisory stdout = %q, want verdict rendered", stdout.String())
	}
}

func TestRunSDDVerifyGateMissingVerifyReportBlocks(t *testing.T) {
	root := t.TempDir()
	seedSDDVerifyGateChange(t, root, "no-report", "")

	var stdout bytes.Buffer
	err := RunSDDVerifyGate([]string{"no-report", "--cwd", root}, &stdout)
	if err == nil {
		t.Fatal("RunSDDVerifyGate() error = nil, want non-nil when verify-report.md is absent")
	}
}

func TestRunSDDVerifyGateJSONOutputCarriesVerdict(t *testing.T) {
	root := t.TempDir()
	seedSDDVerifyGateChange(t, root, "json-change", sddVerifyGateCleanReport)

	var stdout bytes.Buffer
	if err := RunSDDVerifyGate([]string{"json-change", "--cwd", root, "--json"}, &stdout); err != nil {
		t.Fatalf("RunSDDVerifyGate(--json) error = %v", err)
	}
	var verdict struct {
		ChangeName string `json:"changeName"`
		Passing    bool   `json:"passing"`
		Advisory   bool   `json:"advisory"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &verdict); err != nil {
		t.Fatalf("decode JSON verdict: %v\n%s", err, stdout.String())
	}
	if verdict.ChangeName != "json-change" || !verdict.Passing || verdict.Advisory {
		t.Fatalf("verdict = %#v, want passing json-change verdict", verdict)
	}
}

func TestRunSDDVerifyGateRequiresChangeName(t *testing.T) {
	var stdout bytes.Buffer
	if err := RunSDDVerifyGate([]string{"--cwd", t.TempDir()}, &stdout); err == nil {
		t.Fatal("RunSDDVerifyGate() error = nil, want non-nil without <change>")
	}
}

func TestRunSDDVerifyGateRejectsNonexistentCWD(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	var stdout bytes.Buffer
	if err := RunSDDVerifyGate([]string{"anything", "--cwd", missing}, &stdout); err == nil {
		t.Fatal("RunSDDVerifyGate() error = nil, want non-nil for nonexistent cwd")
	}
}
