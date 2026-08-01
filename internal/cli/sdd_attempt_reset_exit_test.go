package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// TestSDDAttemptBeginNamesTheResetThatClearsDrift is the discoverability proof
// for the second block the extended benchmark corpus reported (journey
// j40-sdd-attempt-reset-after-drift). `sdd-attempt begin` refuses a drifted
// candidate with "SDD runtime objective changed without an explicit reset": it
// names the reset in PROSE and never as a command, so the operator is left with
// a non-zero exit, the name of an operation, and none of the six flags that
// operation requires.
//
// The reset itself works — 262cfe61 admitted exactly this shape — so the block
// is not a dead end, only an undiscoverable one. This test requires the refusal
// to name the reset as a runnable command and proves the naming by running it:
// the command is lifted out of the message the product printed, only the
// operator's own run facts are substituted, and `begin` must then be accepted.
func TestSDDAttemptBeginNamesTheResetThatClearsDrift(t *testing.T) {
	fixture := newDriftedObjectiveFixture(t, "cli-drifted-objective")

	message := driftedBeginRefusal(t, fixture, "drifted-begin-2", fixture.objective)

	arguments := namedRunnableGentleCommand(t, message)
	if len(arguments) < 3 || arguments[0] != "gentle-ai" || arguments[1] != "sdd-attempt" || arguments[2] != "reset" {
		t.Fatalf("refusal names %v, not a runnable sdd-attempt reset:\n%s", arguments, message)
	}
	arguments = arguments[2:]
	for _, flag := range []string{"--cwd", "--change", "--expected-revision", "--request-id", "--reason", "--actor"} {
		if !strings.Contains(strings.Join(arguments, " "), flag) {
			t.Fatalf("named reset omits %s, which the CLI requires:\n%s", flag, message)
		}
	}

	// Only the operator's own run facts may still be placeholders. Every
	// identity the product already holds has to be printed concrete.
	want := map[string]string{
		"<unique-request-id>":         "drifted-reset-1",
		"<why-the-objective-changed>": "the candidate drifted after the interrupted attempt",
		"<actor>":                     "cli-reset-exit-test",
	}
	for _, placeholder := range namedCommandPlaceholders(arguments) {
		value, known := want[placeholder]
		if !known {
			t.Fatalf("refusal leaves %s for the operator to guess; it is not one of the run's own facts:\n%s", placeholder, message)
		}
		arguments = fillNamedCommandPlaceholder(arguments, placeholder, value)
	}

	var reset bytes.Buffer
	if resetErr := RunSDDAttempt(arguments, &reset); resetErr != nil {
		t.Fatalf("the exit the refusal named does not work: sdd-attempt %v: %v\nrefusal:\n%s\noutput:\n%s",
			arguments, resetErr, message, reset.String())
	}

	// The block has to be gone, not merely differently worded: the same begin
	// the refusal blocked must now be accepted.
	after := runSDDAttemptStatus(t, []string{"status", "--cwd", fixture.repo, "--change", fixture.change})
	if after.NextAction != sddstatus.RuntimeActionBegin {
		t.Fatalf("the named reset ran but the runtime next action is %q:\n%s", after.NextAction, reset.String())
	}
	began := runSDDAttemptStatus(t, append([]string{
		"begin", "--cwd", fixture.repo, "--change", fixture.change,
		"--expected-revision", after.Revision, "--request-id", "drifted-begin-3",
	}, fixture.objective...))
	if began.ActiveAttempt == nil || began.ActiveAttempt.Outcome != sddstatus.AttemptRunning {
		t.Fatalf("the named reset ran but the drifted begin is still blocked: %#v", began)
	}
}

// TestSDDAttemptBeginNamesTheObjectiveItAlreadyHolds pins the other half of the
// same refusal. When the objective parameters change while the candidate has
// NOT drifted, `reset` is refused one layer deeper — an elective early reset
// would launder the per-objective budget — so naming it there would be exactly
// the defect this file exists to kill. The runnable continuation is the begin
// the ledger already accepts, with the parameters it already holds, and the
// refusal has to print those.
func TestSDDAttemptBeginNamesTheObjectiveItAlreadyHolds(t *testing.T) {
	fixture := newUndriftedObjectiveFixture(t, "cli-changed-objective")

	changed := []string{
		"--work-unit", "a different work unit", "--evidence-goal", fixture.evidenceGoal,
		"--max-attempts", "3", "--max-changed-lines", "40",
	}
	message := driftedBeginRefusal(t, fixture, "changed-begin-2", changed)

	arguments := namedRunnableGentleCommand(t, message)
	if len(arguments) < 3 || arguments[0] != "gentle-ai" || arguments[1] != "sdd-attempt" || arguments[2] != "begin" {
		t.Fatalf("refusal names %v, not the begin the ledger already accepts:\n%s", arguments, message)
	}
	if strings.Contains(message, "gentle-ai sdd-attempt reset") {
		t.Fatalf("refusal names a reset that is refused one layer deeper:\n%s", message)
	}
	arguments = arguments[2:]

	want := map[string]string{"<unique-request-id>": "changed-begin-3"}
	for _, placeholder := range namedCommandPlaceholders(arguments) {
		value, known := want[placeholder]
		if !known {
			t.Fatalf("refusal leaves %s for the operator to guess; it is not one of the run's own facts:\n%s", placeholder, message)
		}
		arguments = fillNamedCommandPlaceholder(arguments, placeholder, value)
	}
	var began bytes.Buffer
	if beginErr := RunSDDAttempt(arguments, &began); beginErr != nil {
		t.Fatalf("the exit the refusal named does not work: sdd-attempt %v: %v\nrefusal:\n%s\noutput:\n%s",
			arguments, beginErr, message, began.String())
	}
	var status sddstatus.RuntimeStatus
	decodeStrictReviewJSON(t, began.Bytes(), &status)
	if status.ActiveAttempt == nil || status.ActiveAttempt.Outcome != sddstatus.AttemptRunning {
		t.Fatalf("the exit the refusal named did not clear the block: %#v", status)
	}
}

type driftedObjectiveFixture struct {
	repo         string
	change       string
	changeRoot   string
	evidenceGoal string
	objective    []string
}

// newUndriftedObjectiveFixture leaves one terminal interrupted attempt behind
// with the candidate exactly where that attempt closed it.
func newUndriftedObjectiveFixture(t *testing.T, change string) driftedObjectiveFixture {
	t.Helper()
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	changeRoot := filepath.Join(repo, "openspec", "changes", change)
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n")
	runReviewCLIGit(t, repo, "add", ".")
	runReviewCLIGit(t, repo, "commit", "-qm", "seed change")

	fixture := driftedObjectiveFixture{
		repo: repo, change: change, changeRoot: changeRoot,
		evidenceGoal: "prove the runtime objective",
		objective: []string{
			"--work-unit", change, "--evidence-goal", "prove the runtime objective",
			"--max-attempts", "3", "--max-changed-lines", "40",
		},
	}
	started := runSDDAttemptStatus(t, append([]string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision=", "--request-id", "drifted-begin-1",
	}, fixture.objective...))
	runSDDAttemptStatus(t, []string{
		"finish", "--cwd", repo, "--change", change, "--expected-revision", started.Revision,
		"--request-id", "drifted-finish-1", "--outcome", "interrupted", "--evidence-revision", cliAttemptHash('a'),
		"--diagnosis", "the attempt was interrupted with budget to spare", "--harness-disposition", "reused",
		"--cleanup-evidence", "cleanup completed", "--process-evidence", "process scan found no descendants",
	})
	return fixture
}

// newDriftedObjectiveFixture adds the drift: the candidate moves after the
// terminal attempt closed, which is the shape `begin` refuses.
func newDriftedObjectiveFixture(t *testing.T, change string) driftedObjectiveFixture {
	t.Helper()
	fixture := newUndriftedObjectiveFixture(t, change)
	writeCLIAttemptFile(t, filepath.Join(fixture.changeRoot, "tasks.md"),
		"- [x] 1.1 Done\n# the candidate drifted after the attempt closed\n")
	return fixture
}

// driftedBeginRefusal runs the begin the operator reaches for and returns the
// refusal text it printed.
func driftedBeginRefusal(t *testing.T, fixture driftedObjectiveFixture, requestID string, objective []string) string {
	t.Helper()
	status := runSDDAttemptStatus(t, []string{"status", "--cwd", fixture.repo, "--change", fixture.change})
	var blocked bytes.Buffer
	err := RunSDDAttempt(append([]string{
		"begin", "--cwd", fixture.repo, "--change", fixture.change,
		"--expected-revision", status.Revision, "--request-id", requestID,
	}, objective...), &blocked)
	if err == nil {
		t.Fatalf("begin against a changed objective was accepted:\n%s", blocked.String())
	}
	if !errors.Is(err, sddstatus.ErrRuntimeObjectiveChange) {
		t.Fatalf("begin refused for the wrong reason: %T %v", err, err)
	}
	return err.Error()
}
