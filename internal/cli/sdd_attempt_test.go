package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDAttemptLifecycleIsMachineReadableAndResetExplicit(t *testing.T) {
	repo := initReviewCLIRepo(t)
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, "cli-attempt")
	if err != nil {
		t.Fatal(err)
	}

	status := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", "cli-attempt"})
	if status.Schema != sddstatus.RuntimeStatusSchema || status.Change != "cli-attempt" || status.Revision != "" || status.NextAction != sddstatus.RuntimeActionBegin {
		t.Fatalf("initial CLI status = %#v", status)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("read-only status created native authority: %v", statErr)
	}

	started := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", "cli-attempt", "--expected-revision=", "--request-id", "cli-begin",
		"--work-unit", "runtime-harness", "--evidence-goal", "prove CLI runtime evidence", "--max-attempts", "1", "--max-changed-lines", "10",
	})
	if started.ActiveAttempt == nil || started.ActiveAttempt.Ordinal != 1 || started.NextAction != sddstatus.RuntimeActionFinish {
		t.Fatalf("begin CLI status = %#v", started)
	}

	failed := runSDDAttemptStatus(t, []string{
		"finish", "--cwd", repo, "--change", "cli-attempt", "--expected-revision", started.Revision, "--request-id", "cli-finish",
		"--outcome", "failed", "--evidence-revision", cliAttemptHash('a'),
		"--diagnosis", "CLI harness reproduced the bounded runtime failure", "--harness-disposition", "reused",
		"--cleanup-evidence", "CLI cleanup completed", "--process-evidence", "CLI process scan found no descendants",
	})
	if !failed.DecisionRequired || failed.NextAction != sddstatus.RuntimeActionReset {
		t.Fatalf("finish CLI status = %#v", failed)
	}

	reset := runSDDAttemptStatus(t, []string{
		"reset", "--cwd", repo, "--change", "cli-attempt", "--expected-revision", failed.Revision, "--request-id", "cli-reset",
		"--reason", "maintainer approved a changed runtime evidence scope", "--actor", "maintainer",
	})
	if reset.Objective != nil || reset.ObjectiveGeneration != 1 || reset.CumulativeAttempts != 0 || reset.LifetimeAttempts != 1 || reset.NextAction != sddstatus.RuntimeActionBegin {
		t.Fatalf("reset CLI status = %#v", reset)
	}
}

func TestRunSDDAttemptRejectsMissingOrAmbiguousInputs(t *testing.T) {
	repo := initReviewCLIRepo(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing operation", args: nil, want: "requires status, begin, finish, or reset"},
		// The no-args refusal already enumerates every valid operation; the
		// unknown-operation refusal must do the same instead of naming only
		// the bad value with no route to the valid set.
		{name: "unknown operation", args: []string{"begn"}, want: `unknown sdd-attempt operation "begn"; want one of status, begin, finish, or reset`},
		{name: "missing change", args: []string{"status", "--cwd", repo}, want: "--change"},
		{name: "unknown flag", args: []string{"status", "--cwd", repo, "--change", "thin", "--mystery"}, want: "flag provided but not defined"},
		{name: "irrelevant flag", args: []string{"status", "--cwd", repo, "--change", "thin", "--outcome", "failed"}, want: "flag provided but not defined"},
		{name: "missing begin CAS", args: []string{"begin", "--cwd", repo, "--change", "thin", "--request-id", "begin", "--work-unit", "unit", "--evidence-goal", "goal"}, want: "--expected-revision"},
		{name: "missing finish evidence", args: []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "finish", "--outcome", "failed", "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process"}, want: "--evidence-revision"},
		{name: "partial remediation successor", args: []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "finish", "--outcome", "passed", "--evidence-revision", cliAttemptHash('c'), "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process", "--successor-lineage", "review-successor"}, want: "remediation successor requires --expected-binding-revision, --successor-lineage, and --remediates-evidence-revision together"},
		{name: "positional argument", args: []string{"status", "--cwd", repo, "--change", "thin", "extra"}, want: "unexpected sdd-attempt argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunSDDAttempt(tt.args, &output)
			if err == nil || !strings.Contains(err.Error(), tt.want) || output.Len() != 0 {
				t.Fatalf("RunSDDAttempt(%v) = output %q, err %v, want %q", tt.args, output.String(), err, tt.want)
			}
		})
	}
}

// TestSDDAttemptOperationsCanonicalSourceEnumeratesConsistently proves the
// no-args refusal and the unknown-operation refusal both derive from the
// same ordered source, so they cannot drift apart the way they did before
// (unknown-operation named only the bad value; the empty case enumerated
// all four). Mirrors the reviewIntegrationGatesInOrder /
// reviewIntegrationGateNames pattern in review_operation_contract.go.
func TestSDDAttemptOperationsCanonicalSourceEnumeratesConsistently(t *testing.T) {
	want := []string{"status", "begin", "finish", "reset"}
	if !reflect.DeepEqual(sddAttemptOperationsInOrder, want) {
		t.Fatalf("sddAttemptOperationsInOrder = %v, want %v", sddAttemptOperationsInOrder, want)
	}
	for _, operation := range want {
		if !validSDDAttemptOperation(operation) {
			t.Fatalf("validSDDAttemptOperation(%q) = false, want true", operation)
		}
	}
	if validSDDAttemptOperation("begn") {
		t.Fatal(`validSDDAttemptOperation("begn") = true, want false`)
	}
	if got := joinSDDAttemptOperations(); got != "status, begin, finish, or reset" {
		t.Fatalf("joinSDDAttemptOperations() = %q, want %q", got, "status, begin, finish, or reset")
	}
}

func runSDDAttemptStatus(t *testing.T, args []string) sddstatus.RuntimeStatus {
	t.Helper()
	var output bytes.Buffer
	if err := RunSDDAttempt(args, &output); err != nil {
		t.Fatalf("RunSDDAttempt(%v): %v", args, err)
	}
	var status sddstatus.RuntimeStatus
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		t.Fatalf("decode SDD attempt status: %v\n%s", err, output.String())
	}
	if !bytes.HasSuffix(output.Bytes(), []byte("\n")) {
		t.Fatalf("SDD attempt JSON lacks trailing newline: %q", output.Bytes())
	}
	return status
}

func cliAttemptHash(char byte) string {
	return "sha256:" + strings.Repeat(string(char), 64)
}

// TestRunSDDAttemptFinishAcceptsApprovedSelfRemediationSuccessor drives the
// decode2 lifecycle triangle end-to-end through the CLI: failed evidence is
// recorded, the bounded correction lands on the same lineage, that lineage
// holds the approved content-bound review of the corrected candidate, and the
// passing bound finish must complete by accepting the approved self-successor
// instead of demanding an impossible distinct recovery lineage.
func TestRunSDDAttemptFinishAcceptsApprovedSelfRemediationSuccessor(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := "cli-self-remediation"
	changeRoot := filepath.Join(repo, "openspec", "changes", change)
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n")
	runReviewCLIGit(t, repo, "add", ".")
	runReviewCLIGit(t, repo, "commit", "-qm", "seed change")

	started := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision=", "--request-id", "self-begin-1",
		"--work-unit", "cli-self-remediation", "--evidence-goal", "repair failed verification evidence",
		"--max-attempts", "3", "--max-changed-lines", "40",
	})
	failedEvidence := cliAttemptHash('a')
	failed := runSDDAttemptStatus(t, []string{
		"finish", "--cwd", repo, "--change", change, "--expected-revision", started.Revision, "--request-id", "self-finish-1",
		"--outcome", "failed", "--evidence-revision", failedEvidence,
		"--diagnosis", "failed verification reproduced before bounded self remediation", "--harness-disposition", "reused",
		"--cleanup-evidence", "predecessor cleanup completed", "--process-evidence", "predecessor process scan found no descendants",
	})
	active := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision", failed.Revision, "--request-id", "self-begin-2",
		"--work-unit", "cli-self-remediation", "--evidence-goal", "repair failed verification evidence",
		"--max-attempts", "3", "--max-changed-lines", "40",
	})
	if active.ActiveAttempt == nil || active.EvidenceRevision != failedEvidence {
		t.Fatalf("pre-remediation CLI status = %#v", active)
	}

	// The bounded correction lands during the attempt on the same lineage.
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# bounded self remediation\n")
	lineage := "cli-self-lineage"
	writeCLIApprovedCompactAuthority(t, repo, lineage)
	binding, err := sddstatus.BindApprovedReview(context.Background(), repo, change, lineage, "")
	if err != nil {
		t.Fatal(err)
	}
	postBind := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	if postBind.Binding == nil || postBind.Binding.Revision != binding.Revision {
		t.Fatalf("post-bind CLI status = %#v", postBind)
	}

	finishArgs := []string{
		"finish", "--cwd", repo, "--change", change, "--expected-revision", postBind.Revision, "--request-id", "self-finish-2",
		"--outcome", "passed", "--evidence-revision", cliAttemptHash('b'),
		"--diagnosis", "bounded self remediation passed corrected verification", "--harness-disposition", "reused",
		"--cleanup-evidence", "self remediation cleanup completed", "--process-evidence", "self remediation process scan found no descendants",
		"--expected-binding-revision", binding.Revision, "--successor-lineage", lineage,
		"--remediates-evidence-revision", failedEvidence,
	}
	completed := runSDDAttemptStatus(t, finishArgs)
	if !completed.Complete || completed.ActiveAttempt != nil || completed.NextAction != sddstatus.RuntimeActionComplete {
		t.Fatalf("self-remediation CLI completion = %#v", completed)
	}
	if completed.Binding == nil || completed.Binding.Lineage != lineage || completed.Binding.Revision != binding.Revision {
		t.Fatalf("self-remediation CLI binding = %#v", completed.Binding)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.Outcome != sddstatus.AttemptPassed || last.RemediatesEvidenceRevision != failedEvidence ||
		last.EvidenceRevision != cliAttemptHash('b') || last.ChangedLines == 0 {
		t.Fatalf("self-remediation CLI attempt = %#v", last)
	}

	replayed := runSDDAttemptStatus(t, finishArgs)
	if replayed.Revision != completed.Revision || !replayed.Complete {
		t.Fatalf("self-remediation CLI replay = %#v, want revision %s", replayed, completed.Revision)
	}
}

func writeCLIAttemptFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCLIApprovedCompactAuthority(t *testing.T, repo, lineage string) {
	t.Helper()
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == reviewtransaction.RiskMedium {
		lenses = []string{reviewtransaction.LensReliability}
	} else if risk == reviewtransaction.RiskHigh {
		lenses = []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: cliAttemptHash('c'), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]reviewtransaction.LensResult, len(lenses))
	for index, lens := range lenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"review complete"}}
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{
		LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("cli self remediation verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
}

func TestRunSDDAttemptStatusPathUsesRepositoryCommonDir(t *testing.T) {
	repo := initReviewCLIRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, repo, "worktree", "add", "-q", "--detach", linked)
	started := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", "linked-attempt", "--expected-revision=", "--request-id", "linked-begin",
		"--work-unit", "linked-work-unit", "--evidence-goal", "prove linked worktree authority", "--max-attempts", "2", "--max-changed-lines", "10",
	})
	fromLinked := runSDDAttemptStatus(t, []string{"status", "--cwd", linked, "--change", "linked-attempt"})
	if fromLinked.Revision != started.Revision || fromLinked.ActiveAttempt == nil || fromLinked.ActiveAttempt.Ordinal != 1 {
		t.Fatalf("linked status = %#v, want revision %s", fromLinked, started.Revision)
	}
}
