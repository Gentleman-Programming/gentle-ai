package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// namedRunnableGentleCommand extracts the first backtick-delimited
// `gentle-ai ...` command a refusal names. A refusal that names an internal
// operation identifier, or names nothing at all, is the defect this file
// exists to catch: the operator is left with a non-zero exit and no door.
func namedRunnableGentleCommand(t *testing.T, message string) []string {
	t.Helper()
	rest := message
	for {
		open := strings.Index(rest, "`")
		if open < 0 {
			break
		}
		remainder := rest[open+1:]
		closing := strings.Index(remainder, "`")
		if closing < 0 {
			break
		}
		span := remainder[:closing]
		if strings.HasPrefix(span, "gentle-ai ") {
			return splitNamedCommand(t, span)
		}
		rest = remainder[closing+1:]
	}
	t.Fatalf("refusal names no runnable `gentle-ai ...` command:\n%s", message)
	return nil
}

// splitNamedCommand tokenizes exactly like a shell would for the double-quoted
// forms these messages use, so the arguments the test dispatches are the
// arguments an operator would get by pasting the line.
func splitNamedCommand(t *testing.T, command string) []string {
	t.Helper()
	fields := []string{}
	var current strings.Builder
	quoted, started := false, false
	for _, symbol := range command {
		switch {
		case symbol == '"':
			quoted = !quoted
			started = true
		case symbol == ' ' && !quoted:
			if started {
				fields = append(fields, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(symbol)
			started = true
		}
	}
	if quoted {
		t.Fatalf("named command has unbalanced quoting: %s", command)
	}
	if started {
		fields = append(fields, current.String())
	}
	return fields
}

// namedCommandPlaceholders returns every `<...>` token the operator still has
// to choose. Anything else in the command must already be a concrete value the
// product knows, because a value the product knows and refuses to print is a
// round trip the operator pays for nothing.
func namedCommandPlaceholders(arguments []string) []string {
	placeholders := []string{}
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "<") && strings.HasSuffix(argument, ">") {
			placeholders = append(placeholders, argument)
		}
	}
	return placeholders
}

func fillNamedCommandPlaceholder(arguments []string, placeholder, value string) []string {
	filled := append([]string(nil), arguments...)
	for index := range filled {
		if filled[index] == placeholder {
			filled[index] = value
		}
	}
	return filled
}

// TestSDDAttemptBoundFinishNamesTheExitThatWorks is the discoverability proof
// for decode2's reported deadlock (PR #1801). The refusal a bound passing
// finish emits when the candidate moved after the attempt began used to read
// "requires an atomic approved recovery successor" and named no command at
// all — and the word "recovery" pointed at `review recover`, which is the
// first step of the cycle that has no exit.
//
// b37aa8e4 made the exit legal: the bound lineage itself is an acceptable
// --successor-lineage once the corrected candidate is approved on it. This
// test requires the refusal to NAME that exit, and proves the naming by
// running it: the command is taken out of the message the product printed,
// only the operator's own run facts are substituted, and the block must then
// clear with complete: true. Asserting on wording alone is the exact defect
// class this branch exists to kill.
func TestSDDAttemptBoundFinishNamesTheExitThatWorks(t *testing.T) {
	fixture := newBoundRemediationFixture(t, "cli-named-exit")
	lineage, failedEvidence := fixture.lineage, fixture.failedEvidence

	message := boundPassingFinishRefusal(t, fixture, "named-exit-finish-2")

	// The word "recovery" sent the reporter to `review recover`, which is the
	// first step of the dead cycle.
	if strings.Contains(message, "recovery successor") {
		t.Fatalf("refusal still points at recovery, which is the wrong door:\n%s", message)
	}
	for _, flag := range []string{"--expected-binding-revision", "--successor-lineage", "--remediates-evidence-revision"} {
		if !strings.Contains(message, flag) {
			t.Fatalf("refusal omits %s from the flag set it advertises:\n%s", flag, message)
		}
	}
	// The key, unobvious fact: the bound lineage itself is the successor.
	if !strings.Contains(message, lineage) {
		t.Fatalf("refusal never names the bound lineage as the successor:\n%s", message)
	}

	arguments := namedRunnableGentleCommand(t, message)
	if len(arguments) < 3 || arguments[0] != "gentle-ai" || arguments[1] != "sdd-attempt" || arguments[2] != "finish" {
		t.Fatalf("refusal names %v, not a runnable sdd-attempt finish:\n%s", arguments, message)
	}
	arguments = arguments[2:]

	// Only the operator's own run facts may still be placeholders. Every
	// identity the product already holds has to be printed concrete.
	want := map[string]string{
		"<unique-request-id>":         "named-exit-finish-3",
		"<corrected-evidence-sha256>": cliAttemptHash('b'),
		"<proven-diagnosis>":          "bounded self remediation passed corrected verification",
		"<reused|invalidated>":        "reused",
		"<cleanup-evidence>":          "self remediation cleanup completed",
		"<process-evidence>":          "self remediation process scan found no descendants",
	}
	for _, placeholder := range namedCommandPlaceholders(arguments) {
		value, known := want[placeholder]
		if !known {
			t.Fatalf("refusal leaves %s for the operator to guess; it is not one of the run's own facts:\n%s", placeholder, message)
		}
		arguments = fillNamedCommandPlaceholder(arguments, placeholder, value)
	}

	var completed bytes.Buffer
	if runErr := RunSDDAttempt(arguments, &completed); runErr != nil {
		t.Fatalf("the exit the refusal named does not work: sdd-attempt %v: %v\nrefusal:\n%s\noutput:\n%s",
			arguments, runErr, message, completed.String())
	}
	var status sddstatus.RuntimeStatus
	decodeStrictReviewJSON(t, completed.Bytes(), &status)
	if !status.Complete || status.ActiveAttempt != nil || status.NextAction != sddstatus.RuntimeActionComplete {
		t.Fatalf("the exit the refusal named did not clear the block: %#v", status)
	}
	if status.Binding == nil || status.Binding.Lineage != lineage {
		t.Fatalf("the exit the refusal named rebound the change: %#v", status.Binding)
	}
	last := status.Attempts[len(status.Attempts)-1]
	if last.Outcome != sddstatus.AttemptPassed || last.RemediatesEvidenceRevision != failedEvidence {
		t.Fatalf("the exit the refusal named recorded %#v", last)
	}
}

// TestSDDAttemptBoundFinishRefusesToNameAnExitThatWouldFail pins the other
// half of the contract. Once the bound lineage has gained a real recovery
// successor it is no longer the compact recovery leaf and its live post-apply
// gate no longer allows, so the self-successor finish is refused one layer
// deeper. That state is reachable — `review recover` mints it as soon as the
// candidate moves again after approval — so the refusal must NOT print the
// self-successor command there. It has to say which lineage is missing its
// approval and hand over the review router, and that router has to run.
func TestSDDAttemptBoundFinishRefusesToNameAnExitThatWouldFail(t *testing.T) {
	fixture := newBoundRemediationFixture(t, "cli-nonleaf-exit")

	// A later scope change hands the bound lineage a true recovery successor
	// that nobody has approved yet.
	writeCLIAttemptFile(t, filepath.Join(fixture.changeRoot, "tasks.md"),
		"- [x] 1.1 Done\n# bounded self remediation\n# a later scope change\n")
	createCLIRecoverySuccessor(t, fixture.repo, fixture.lineage, fixture.lineage+"-successor")

	message := boundPassingFinishRefusal(t, fixture, "nonleaf-finish-2")
	if strings.Contains(message, "gentle-ai sdd-attempt finish") {
		t.Fatalf("refusal names the self-successor finish in a state where it is refused:\n%s", message)
	}
	if !strings.Contains(message, fixture.lineage) {
		t.Fatalf("refusal never says which lineage is missing its approval:\n%s", message)
	}
	for _, flag := range []string{"--expected-binding-revision", "--successor-lineage", "--remediates-evidence-revision"} {
		if !strings.Contains(message, flag) {
			t.Fatalf("refusal omits %s from the flag set the finish still needs:\n%s", flag, message)
		}
	}

	arguments := namedRunnableGentleCommand(t, message)
	if len(arguments) < 3 || arguments[0] != "gentle-ai" || arguments[1] != "review" || arguments[2] != "status" {
		t.Fatalf("refusal names %v, not the review router:\n%s", arguments, message)
	}
	if placeholders := namedCommandPlaceholders(arguments); len(placeholders) != 0 {
		t.Fatalf("refusal leaves %v for the operator to guess in a command it already knows:\n%s", placeholders, message)
	}
	var routed bytes.Buffer
	if routeErr := RunReview(arguments[2:], &routed); routeErr != nil {
		t.Fatalf("the router the refusal named does not run: review %v: %v\nrefusal:\n%s\noutput:\n%s",
			arguments[2:], routeErr, message, routed.String())
	}
	if !bytes.Contains(routed.Bytes(), []byte("\"action\"")) {
		t.Fatalf("the router the refusal named published no next action:\n%s", routed.String())
	}
}

// boundRemediationFixture is decode2's exact state, built through the real
// CLI: failed evidence recorded, a second attempt open, the bounded correction
// landed, and the same lineage holding the approved content-bound review of
// the corrected candidate.
type boundRemediationFixture struct {
	repo           string
	change         string
	changeRoot     string
	lineage        string
	failedEvidence string
}

func newBoundRemediationFixture(t *testing.T, change string) boundRemediationFixture {
	t.Helper()
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	changeRoot := filepath.Join(repo, "openspec", "changes", change)
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n")
	runReviewCLIGit(t, repo, "add", ".")
	runReviewCLIGit(t, repo, "commit", "-qm", "seed change")

	started := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision=", "--request-id", "bound-begin-1",
		"--work-unit", change, "--evidence-goal", "repair failed verification evidence",
		"--max-attempts", "3", "--max-changed-lines", "40",
	})
	failedEvidence := cliAttemptHash('a')
	failed := runSDDAttemptStatus(t, []string{
		"finish", "--cwd", repo, "--change", change, "--expected-revision", started.Revision,
		"--request-id", "bound-finish-1", "--outcome", "failed", "--evidence-revision", failedEvidence,
		"--diagnosis", "failed verification reproduced before bounded self remediation", "--harness-disposition", "reused",
		"--cleanup-evidence", "predecessor cleanup completed",
		"--process-evidence", "predecessor process scan found no descendants",
	})
	runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision", failed.Revision,
		"--request-id", "bound-begin-2", "--work-unit", change,
		"--evidence-goal", "repair failed verification evidence",
		"--max-attempts", "3", "--max-changed-lines", "40",
	})

	// The bounded correction lands during the attempt, and the SAME lineage
	// reviews and approves the corrected candidate.
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# bounded self remediation\n")
	lineage := change + "-lineage"
	writeCLIApprovedCompactAuthority(t, repo, lineage)
	if _, err := sddstatus.BindApprovedReview(context.Background(), repo, change, lineage, ""); err != nil {
		t.Fatal(err)
	}
	bound := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	if bound.Binding == nil || bound.EvidenceRevision != failedEvidence {
		t.Fatalf("bound CLI status = %#v", bound)
	}
	return boundRemediationFixture{
		repo: repo, change: change, changeRoot: changeRoot, lineage: lineage, failedEvidence: failedEvidence,
	}
}

// boundPassingFinishRefusal runs the plain passing finish the operator reaches
// for and returns the refusal text it printed.
func boundPassingFinishRefusal(t *testing.T, fixture boundRemediationFixture, requestID string) string {
	t.Helper()
	bound := runSDDAttemptStatus(t, []string{"status", "--cwd", fixture.repo, "--change", fixture.change})
	var blocked bytes.Buffer
	err := RunSDDAttempt([]string{
		"finish", "--cwd", fixture.repo, "--change", fixture.change, "--expected-revision", bound.Revision,
		"--request-id", requestID, "--outcome", "passed", "--evidence-revision", cliAttemptHash('b'),
		"--diagnosis", "bounded self remediation passed corrected verification", "--harness-disposition", "reused",
		"--cleanup-evidence", "self remediation cleanup completed",
		"--process-evidence", "self remediation process scan found no descendants",
	}, &blocked)
	if err == nil {
		t.Fatalf("bound passing finish was allowed without the remediation trio:\n%s", blocked.String())
	}
	if !errors.Is(err, sddstatus.ErrRuntimeRemediationSuccessorRequired) {
		t.Fatalf("bound passing finish refused for the wrong reason: %T %v", err, err)
	}
	return err.Error()
}

// createCLIRecoverySuccessor mints an unapproved scope-changed recovery
// successor for the live candidate, which is what displaces the predecessor
// from the compact recovery leaf.
func createCLIRecoverySuccessor(t *testing.T, repo, predecessorLineage, successorLineage string) {
	t.Helper()
	ctx := context.Background()
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, predecessorLineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	switch risk {
	case reviewtransaction.RiskMedium:
		lenses = []string{reviewtransaction.LensReliability}
	case reviewtransaction.RiskHigh:
		lenses = []string{
			reviewtransaction.LensRisk, reviewtransaction.LensResilience,
			reviewtransaction.LensReadability, reviewtransaction.LensReliability,
		}
	}
	successor, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: successorLineage, Mode: reviewtransaction.ModeOrdinaryBounded,
		Generation: predecessor.State.Generation + 1, Snapshot: snapshot,
		PolicyHash: cliAttemptHash('c'), RiskLevel: risk, SelectedLenses: lenses,
		OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewtransaction.RecoverCompactAuthority(ctx, repo, reviewtransaction.CompactRecoveryRequest{
		PredecessorLineageID: predecessorLineage, ExpectedPredecessorRevision: predecessor.Revision,
		Successor: successor, Disposition: reviewtransaction.RecoveryScopeChanged,
		Reason: "the candidate moved again after approval", Actor: "cli-remediation-exit-test",
	}); err != nil {
		t.Fatal(err)
	}
}
