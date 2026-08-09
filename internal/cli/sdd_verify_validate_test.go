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
	root := t.TempDir()
	specDir := filepath.Join(root, "openspec", "changes", "thin", "specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("### Requirement: One\n#### Scenario: One\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := "```yaml\nschema: gentle-ai.verify-result/v1\nevidence_revision: sha256:" + strings.Repeat("a", 64) + "\nverdict: fail\nblockers: 1\ncritical_findings: 0\nrequirements: 1/1\nscenarios: 1/1\ntest_command: go test ./...\ntest_exit_code: 0\ntest_output_hash: sha256:" + strings.Repeat("b", 64) + "\nbuild_command: go vet ./...\nbuild_exit_code: 0\nbuild_output_hash: sha256:" + strings.Repeat("c", 64) + "\n```"
	baseArgs := []string{"--input", "-", "--cwd", root, "--change", "thin"}
	argsWith := func(extra ...string) []string {
		args := append([]string(nil), baseArgs...)
		return append(args, extra...)
	}
	var output bytes.Buffer
	if err := runSDDVerifyValidate(argsWith(), strings.NewReader(report), &output); err != nil {
		t.Fatalf("valid failure: %v", err)
	}
	if got := output.String(); !strings.Contains(got, `"valid": true`) || !strings.Contains(got, `"verdict": "fail"`) {
		t.Fatalf("output = %s", got)
	}
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"missing authority", []string{"--input", "-", "--cwd", root}, "requires --cwd and --change"},
		{"unknown retired total", argsWith("--requirements", "1"), "not defined"},
		{"slice requires id", argsWith("--scope", "slice"), "requires a slice_id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := runSDDVerifyValidate(tt.args, strings.NewReader(report), &bytes.Buffer{})
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

func TestRunSDDVerifyValidateHelpIsSuccessfulAndInputFree(t *testing.T) {
	stdin := &sddVerifyValidateReadSpy{}
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"--help"}, stdin, &output); err != nil {
		t.Fatal(err)
	}
	if stdin.reads != 0 {
		t.Fatalf("help read stdin %d times", stdin.reads)
	}
	for _, want := range []string{"--input <path|->", "--cwd <repo>", "--change <name>", "--scope <whole|slice>", "--slice-id <id>", "scope, slice_id"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "--requirements") || strings.Contains(output.String(), "--scenarios") {
		t.Fatalf("help retained caller totals:\n%s", output.String())
	}
}

func TestRunSDDVerifyValidateRequiredFlagsRemainRequired(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{nil, "requires --input"},
		{[]string{"--input", "-"}, "requires --cwd and --change"},
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
