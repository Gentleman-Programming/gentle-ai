package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDVerifyValidate(t *testing.T) {
	report := "```yaml\nschema: gentle-ai.verify-result/v1\nevidence_revision: sha256:" + strings.Repeat("a", 64) + "\nverdict: fail\nblockers: 1\ncritical_findings: 0\nrequirements: 1/1\nscenarios: 1/1\ntest_command: go test ./...\ntest_exit_code: 0\ntest_output_hash: sha256:" + strings.Repeat("b", 64) + "\nbuild_command: go vet ./...\nbuild_exit_code: 0\nbuild_output_hash: sha256:" + strings.Repeat("c", 64) + "\n```"
	baseArgs := []string{"--input", "-", "--requirements", "1", "--scenarios", "1"}
	var output bytes.Buffer
	if err := runSDDVerifyValidate(baseArgs, strings.NewReader(report), &output); err != nil {
		t.Fatalf("valid failure: %v", err)
	}
	if got := output.String(); !strings.Contains(got, `"valid": true`) || !strings.Contains(got, `"verdict": "fail"`) {
		t.Fatalf("output = %s", got)
	}
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runSDDVerifyValidate([]string{"--input", path, "--requirements", "1", "--scenarios", "1"}, strings.NewReader("unused"), &bytes.Buffer{}); err != nil {
		t.Fatalf("file input: %v", err)
	}
	for _, tt := range []struct {
		name  string
		args  []string
		input string
		want  string
	}{
		{"front matter", baseArgs, "---\n" + report, "front matter"},
		{"missing requirements", []string{"--input", "-"}, report, "requires --requirements"},
		{"missing scenarios", []string{"--input", "-", "--requirements", "1"}, report, "requires --scenarios"},
		{"negative count", []string{"--input", "-", "--requirements", "-1", "--scenarios", "1"}, report, "nonnegative"},
		{"count mismatch", []string{"--input", "-", "--requirements", "2", "--scenarios", "1"}, report, "actual requirement count 2"},
		{"whole slice ID", append(baseArgs, "--slice-id", "slice-a"), report, "whole verification does not accept a slice_id"},
		{"whole slice metadata", baseArgs, strings.Replace(report, "\n```", "\nscope: slice\nslice_id: slice-a\n```", 1), "invalid slice scope extension"},
		{"slice missing authority", []string{"--input", "-", "--scope", "slice", "--slice-id", "slice-a"}, report, "requires --cwd and --change"},
		{"slice missing ID", []string{"--input", "-", "--cwd", t.TempDir(), "--change", "thin", "--scope", "slice"}, report, "requires --slice-id"},
		{"slice caller requirements", []string{"--input", "-", "--cwd", t.TempDir(), "--change", "thin", "--scope", "slice", "--slice-id", "slice-a", "--requirements", "1"}, report, "does not accept --requirements"},
		{"slice caller scenarios", []string{"--input", "-", "--cwd", t.TempDir(), "--change", "thin", "--scope", "slice", "--slice-id", "slice-a", "--scenarios", "1"}, report, "does not accept --requirements or --scenarios"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := runSDDVerifyValidate(tt.args, strings.NewReader(tt.input), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunSDDVerifyValidateScopedProofPersistsAndRefusesInvalidReport(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := "scoped-proof"
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), sddstatus.BeginAttemptRequest{
		RequestID: "scoped-proof-begin", WorkUnit: "slice-a", EvidenceGoal: "prove scoped validation", MaxAttempts: 1, MaxChangedLines: 20,
		Scope: &sddstatus.RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scoped-proof.txt"), []byte("proof\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	passed, err := store.Finish(context.Background(), sddstatus.FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "scoped-proof-finish", Outcome: sddstatus.AttemptPassed, EvidenceRevision: cliAttemptHash('a'),
		Diagnosis: "scoped proof passed", HarnessDisposition: sddstatus.HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := cliScopedVerifyReport(passed.Objective.ID)
	baseArgs := []string{"--input", "-", "--cwd", repo, "--change", change}
	sliceArgs := append([]string(nil), baseArgs...)
	sliceArgs = append(sliceArgs, "--scope", "slice", "--slice-id", passed.Objective.ID)
	var output bytes.Buffer
	if err := runSDDVerifyValidate(sliceArgs, strings.NewReader(report), &output); err != nil {
		t.Fatalf("scoped validation = %v", err)
	}
	if !strings.Contains(output.String(), `"valid": true`) {
		t.Fatalf("scoped validation output = %s", output.String())
	}
	for _, args := range [][]string{
		append(append([]string(nil), sliceArgs...), "--requirements", "1"),
		append(append([]string(nil), sliceArgs...), "--scenarios", "1"),
	} {
		if err := runSDDVerifyValidate(args, strings.NewReader(report), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "slice verification does not accept --requirements or --scenarios") {
			t.Fatalf("slice caller total error = %v", err)
		}
	}
	accepted, err := store.Status()
	if err != nil || len(accepted.SliceProofs) != 1 {
		t.Fatalf("persisted scoped proof = %#v err=%v", accepted, err)
	}
	invalid := strings.Replace(report, passed.Objective.ID, cliAttemptHash('b'), 1)
	if err := runSDDVerifyValidate(sliceArgs, strings.NewReader(invalid), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "admission denied") {
		t.Fatalf("invalid scoped validation error = %v", err)
	}
	afterInvalid, err := store.Status()
	if err != nil || afterInvalid.Revision != accepted.Revision || len(afterInvalid.SliceProofs) != 1 {
		t.Fatalf("invalid scoped validation changed accepted state = %#v err=%v", afterInvalid, err)
	}
}

func TestRunSDDVerifyValidateScopedFailAdmitsWithoutProofOrAdvancing(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := "scoped-proof-fail"
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), sddstatus.BeginAttemptRequest{
		RequestID: "scoped-proof-fail-begin", WorkUnit: "slice-fail", EvidenceGoal: "prove failed scoped validation", MaxAttempts: 1, MaxChangedLines: 20,
		Scope: &sddstatus.RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scoped-proof-fail.txt"), []byte("failed proof\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Finish(context.Background(), sddstatus.FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "scoped-proof-fail-finish", Outcome: sddstatus.AttemptPassed, EvidenceRevision: cliAttemptHash('a'),
		Diagnosis: "scoped validation completed", HarnessDisposition: sddstatus.HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := strings.Replace(cliScopedVerifyReport(completed.Objective.ID), "verdict: pass", "verdict: fail", 1)
	report = strings.Replace(report, "blockers: 0", "blockers: 1", 1)
	var output bytes.Buffer
	args := []string{"--input", "-", "--cwd", repo, "--change", change, "--scope", "slice", "--slice-id", completed.Objective.ID}
	if err := runSDDVerifyValidate(args, strings.NewReader(report), &output); err != nil {
		t.Fatalf("failing scoped validation = %v", err)
	}
	if !strings.Contains(output.String(), `"valid": true`) || !strings.Contains(output.String(), `"verdict": "fail"`) {
		t.Fatalf("failing scoped validation output = %s", output.String())
	}
	status, err := store.Status()
	if err != nil || status.Revision != completed.Revision || len(status.SliceProofs) != 0 {
		t.Fatalf("failing scoped validation mutated state = %#v err=%v", status, err)
	}
	if _, err := store.Begin(context.Background(), sddstatus.BeginAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: "scoped-proof-fail-successor", WorkUnit: "slice-successor", EvidenceGoal: "prove successor remains blocked", MaxAttempts: 1, MaxChangedLines: 20,
		Scope: &sddstatus.RuntimeScope{Tasks: []string{"1.2"}, Requirements: []string{"REQ-B"}, Scenarios: []string{"scenario-b"}},
	}); !errors.Is(err, sddstatus.ErrRuntimeObjectiveDone) {
		t.Fatalf("advance after failing scoped validation = %v, want ErrRuntimeObjectiveDone", err)
	}
}

func TestRunSDDVerifyValidatePassWithWarningsPersistsOneScopedProof(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := "scoped-proof-warnings"
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), sddstatus.BeginAttemptRequest{
		RequestID: "scoped-proof-warnings-begin", WorkUnit: "slice-warnings", EvidenceGoal: "prove warning admission", MaxAttempts: 1, MaxChangedLines: 20,
		Scope: &sddstatus.RuntimeScope{Tasks: []string{"1.1"}, Requirements: []string{"REQ-A"}, Scenarios: []string{"scenario-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scoped-proof-warnings.txt"), []byte("proof\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Finish(context.Background(), sddstatus.FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "scoped-proof-warnings-finish", Outcome: sddstatus.AttemptPassed, EvidenceRevision: cliAttemptHash('a'),
		Diagnosis: "scoped warning proof passed", HarnessDisposition: sddstatus.HarnessReused, CleanupEvidence: "cleanup complete", ProcessEvidence: "processes stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := strings.Replace(cliScopedVerifyReport(completed.Objective.ID), "verdict: pass", "verdict: pass_with_warnings", 1)
	args := []string{"--input", "-", "--cwd", repo, "--change", change, "--scope", "slice", "--slice-id", completed.Objective.ID}
	var output bytes.Buffer
	if err := runSDDVerifyValidate(args, strings.NewReader(report), &output); err != nil {
		t.Fatalf("warning scoped validation = %v", err)
	}
	status, err := store.Status()
	if err != nil || len(status.SliceProofs) != 1 || status.SliceProofs[0].ObjectiveID != completed.Objective.ID || !strings.Contains(output.String(), `"verdict": "pass_with_warnings"`) {
		t.Fatalf("warning scoped proof = %#v output=%s err=%v", status.SliceProofs, output.String(), err)
	}
}

func TestRunSDDVerifyValidateHelpIsSuccessfulAndInputFree(t *testing.T) {
	stdin := &sddVerifyValidateReadSpy{}
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"--help"}, stdin, &output); err != nil {
		t.Fatal(err)
	}
	if stdin.reads != 0 {
		t.Fatalf("help read stdin %d times", stdin.reads)
	}
	for _, want := range []string{"--input <path|->", "--requirements <n>", "--scenarios <n>", "--cwd <repo>", "--change <name>", "--scope <whole|slice>", "--slice-id <id>", "scope, slice_id", "whole mode requires caller-supplied", "slice mode forbids caller totals"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, output.String())
		}
	}
}

func TestRunSDDVerifyValidateRequiredFlagsRemainRequired(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{nil, "requires --input"},
		{[]string{"--input", "-"}, "requires --requirements"},
		{[]string{"--input", "-", "--requirements", "1"}, "requires --scenarios"},
	} {
		err := runSDDVerifyValidate(tt.args, strings.NewReader(""), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("runSDDVerifyValidate(%v) = %v, want %q", tt.args, err, tt.want)
		}
	}
}

type sddVerifyValidateReadSpy struct{ reads int }

func (spy *sddVerifyValidateReadSpy) Read([]byte) (int, error) {
	spy.reads++
	return 0, errors.New("help must not read stdin")
}

func cliScopedVerifyReport(sliceID string) string {
	return "```yaml\nschema: gentle-ai.verify-result/v1\nevidence_revision: " + cliAttemptHash('a') + "\nverdict: pass\nblockers: 0\ncritical_findings: 0\nrequirements: 1/1\nscenarios: 1/1\ntest_command: go test ./internal/example\ntest_exit_code: 0\ntest_output_hash: " + cliAttemptHash('b') + "\nbuild_command: go test ./cmd/gentle-ai\nbuild_exit_code: 0\nbuild_output_hash: " + cliAttemptHash('c') + "\nscope: slice\nslice_id: " + sliceID + "\n```"
}
