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

// TestRunSDDAttemptRescopeCarriesHistoryForwardThroughTheCLI is the CLI
// dispatch proof for the `rescope` operation (#2298, #2296 part 2): a
// terminal, zero-drift, non-decision, non-complete objective is narrowed to
// a maintainer-authorized successor without losing history.
func TestRunSDDAttemptRescopeCarriesHistoryForwardThroughTheCLI(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := "cli-rescope"

	started := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision=", "--request-id", "rescope-begin-1",
		"--work-unit", "oversized-scope", "--evidence-goal", "prove CLI rescope", "--max-attempts", "2", "--max-changed-lines", "400",
	})
	interrupted := runSDDAttemptStatus(t, []string{
		"finish", "--cwd", repo, "--change", change, "--expected-revision", started.Revision, "--request-id", "rescope-finish-1",
		"--outcome", "interrupted",
		"--diagnosis", "reverted every temporary change back to the exact original candidate", "--harness-disposition", "invalidated",
		"--cleanup-evidence", "cleanup completed", "--process-evidence", "process scan found no descendants",
	})
	if interrupted.CumulativeChangedLines != 0 || interrupted.DecisionRequired || interrupted.Complete {
		t.Fatalf("pre-rescope CLI status = %#v", interrupted)
	}

	rescoped := runSDDAttemptStatus(t, []string{
		"rescope", "--cwd", repo, "--change", change, "--expected-revision", interrupted.Revision, "--request-id", "rescope-1",
		"--work-unit", "narrower-scope", "--evidence-goal", "prove a narrower CLI rescope", "--max-attempts", "2", "--max-changed-lines", "100",
		"--reason", "maintainer split the oversized objective into a narrower successor", "--actor", "maintainer",
	})
	if rescoped.Objective == nil || rescoped.Objective.WorkUnit != "narrower-scope" || rescoped.Objective.MaxChangedLines != 100 ||
		rescoped.CumulativeAttempts != 1 || rescoped.CumulativeChangedLines != 0 || rescoped.LifetimeAttempts != 1 ||
		rescoped.NextAction != sddstatus.RuntimeActionBegin {
		t.Fatalf("rescope CLI status = %#v", rescoped)
	}
	if rescoped.LastRescope == nil || rescoped.LastRescope.MaxChangedLines != 100 {
		t.Fatalf("rescope CLI audit context = %#v", rescoped.LastRescope)
	}

	began := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision", rescoped.Revision, "--request-id", "rescope-begin-2",
		"--work-unit", "narrower-scope", "--evidence-goal", "prove a narrower CLI rescope", "--max-attempts", "2", "--max-changed-lines", "100",
	})
	if began.ActiveAttempt == nil || began.ActiveAttempt.Outcome != sddstatus.AttemptRunning || began.ActiveAttempt.Ordinal != 2 {
		t.Fatalf("begin after CLI rescope = %#v", began)
	}
}

func TestRunSDDAttemptRejectsMissingOrAmbiguousInputs(t *testing.T) {
	repo := initReviewCLIRepo(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing operation", args: nil, want: "requires status, begin, finish, handoff, reset, rescope, repair, acquire, settle, or grant"},
		// The no-args refusal already enumerates every valid operation; the
		// unknown-operation refusal must do the same instead of naming only
		// the bad value with no route to the valid set.
		{name: "unknown operation", args: []string{"begn"}, want: `unknown sdd-attempt operation "begn"; want one of status, begin, finish, handoff, reset, rescope, repair, acquire, settle, or grant`},
		{name: "missing change", args: []string{"status", "--cwd", repo}, want: "--change"},
		{name: "unknown flag", args: []string{"status", "--cwd", repo, "--change", "thin", "--mystery"}, want: "flag provided but not defined"},
		{name: "irrelevant flag", args: []string{"status", "--cwd", repo, "--change", "thin", "--outcome", "failed"}, want: "flag provided but not defined"},
		{name: "missing begin CAS", args: []string{"begin", "--cwd", repo, "--change", "thin", "--request-id", "begin", "--work-unit", "unit", "--evidence-goal", "goal"}, want: "--expected-revision"},
		{name: "missing rescope scope", args: []string{"rescope", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('e'), "--request-id", "rescope", "--reason", "narrowing", "--actor", "maintainer"}, want: "--work-unit"},
		{name: "missing finish evidence", args: []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "finish", "--outcome", "failed", "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process"}, want: "--evidence-revision"},
		{name: "interrupted malformed evidence", args: []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "finish", "--outcome", "interrupted", "--evidence-revision", "not-a-revision", "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process"}, want: "empty or a canonical legacy sha256 revision"},
		{name: "retired remediation successor flag", args: []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "finish", "--outcome", "passed", "--evidence-revision", cliAttemptHash('c'), "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process", "--successor-lineage", "review-successor"}, want: "flag provided but not defined"},
		{name: "positional argument", args: []string{"status", "--cwd", repo, "--change", "thin", "extra"}, want: "unexpected sdd-attempt argument"},
		// Grant's missing-flag refusal follows acquire/settle: it enumerates
		// every missing flag and names the rerunnable continuation.
		{name: "missing grant roots", args: []string{"grant", "--cwd", repo, "--change", "thin", "--change-instance", "instance-token", "--request-id", "grant", "--actor", "maintainer", "--reason", "rollout"}, want: "sdd-attempt grant requires --root; rerun `gentle-ai sdd-attempt grant` with those missing flags"},
		{name: "missing grant instance and audit fields", args: []string{"grant", "--cwd", repo, "--change", "thin", "--root", repo}, want: "sdd-attempt grant requires --change-instance, --request-id, --actor, --reason"},
		{name: "irrelevant grant flag", args: []string{"grant", "--cwd", repo, "--change", "thin", "--root", repo, "--change-instance", "instance-token", "--request-id", "grant", "--actor", "maintainer", "--reason", "rollout", "--work-unit", "unit"}, want: "flag provided but not defined"},
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

func TestMissingSDDAttemptEvidenceRecognizesBothOutcomeSpellings(t *testing.T) {
	base := []string{"--cwd", "/tmp/repo", "--change", "change", "--expected-revision", cliAttemptHash('a'), "--request-id", "request", "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process"}
	for _, outcome := range [][]string{{"--outcome", "interrupted"}, {"--outcome=interrupted"}} {
		args := append([]string{"finish"}, base...)
		args = append(args, outcome...)
		if missing := missingSDDAttemptOperationFlags(args[1:], "finish", sddAttemptOutcome(args[1:])); len(missing) != 0 {
			t.Fatalf("outcome %v missing flags = %v", outcome, missing)
		}
	}
}

func sddAttemptOutcome(args []string) string {
	for index, argument := range args {
		if strings.HasPrefix(argument, "--outcome=") {
			return strings.TrimPrefix(argument, "--outcome=")
		}
		if argument == "--outcome" && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

// TestSDDAttemptOperationsCanonicalSourceEnumeratesConsistently proves the
// no-args refusal and the unknown-operation refusal both derive from the
// same ordered source, so they cannot drift apart the way they did before
// (unknown-operation named only the bad value; the empty case enumerated
// all four). Mirrors the reviewIntegrationGatesInOrder /
// reviewIntegrationGateNames pattern in review_operation_contract.go.
func TestSDDAttemptOperationsCanonicalSourceEnumeratesConsistently(t *testing.T) {
	want := []string{"status", "begin", "finish", "handoff", "reset", "rescope", "repair", "acquire", "settle", "grant"}
	if got := sddAttemptOperationNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("sddAttemptOperationNames() = %v, want %v", got, want)
	}
	for _, operation := range want {
		if !validSDDAttemptOperation(operation) {
			t.Fatalf("validSDDAttemptOperation(%q) = false, want true", operation)
		}
	}
	if validSDDAttemptOperation("begn") {
		t.Fatal(`validSDDAttemptOperation("begn") = true, want false`)
	}
	if got := joinSDDAttemptOperations(); got != "status, begin, finish, handoff, reset, rescope, repair, acquire, settle, or grant" {
		t.Fatalf("joinSDDAttemptOperations() = %q, want %q", got, "status, begin, finish, handoff, reset, rescope, repair, acquire, settle, or grant")
	}
}

// TestRunSDDAttemptGrantPersistsAndReplaysThroughTheCLI is the CLI dispatch
// proof for the `grant` operation (#2540 S3) under #2557's instance-identity
// containment: a first grant on a fresh ledger needs no --expected-revision
// (the S2 API admits an empty CAS token against an empty chain) but always
// needs --change-instance, the persisted roots round-trip through
// `sdd-attempt status --change-instance`, an exact duplicate --request-id
// replays the committed revision idempotently, and a second grant reusing
// the SAME instance token chains --expected-revision on the first and
// accumulates roots in grant order, deduplicating already-granted ones.
// The interim contract before S4b derives markers natively: the caller
// mints one opaque token per change instance and reuses that exact token
// for widening grants within the change lifecycle; a different token (a
// recreated change, or a reader that declares none) projects nothing.
func TestRunSDDAttemptGrantPersistsAndReplaysThroughTheCLI(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := "cli-grant"
	instance := "cli-grant-instance-token"
	sibling, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	grantArgs := []string{
		"grant", "--cwd", repo, "--change", change, "--root", sibling, "--change-instance", instance,
		"--actor", "maintainer", "--reason", "sequential multi-repository rollout", "--request-id", "cli-grant-1",
	}
	granted := runSDDAttemptStatus(t, grantArgs)
	if len(granted.GrantedRoots) != 1 || granted.GrantedRoots[0] != sibling || granted.Revision == "" {
		t.Fatalf("grant CLI status = %#v, want granted root %q with a committed revision", granted, sibling)
	}

	// Status with the same instance token replays the persisted chain: the
	// grant survives the process boundary between the mutating call and a
	// later read that declares the instance it serves.
	status := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change, "--change-instance", instance})
	if !reflect.DeepEqual(status.GrantedRoots, []string{sibling}) || status.Revision != granted.Revision {
		t.Fatalf("post-grant CLI status = %#v, want granted roots [%q] at revision %s", status, sibling, granted.Revision)
	}

	// Status WITHOUT an instance declaration projects no granted roots: the
	// conservative containment for undeclared readers (#2557).
	undeclared := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	if undeclared.GrantedRoots != nil {
		t.Fatalf("undeclared-instance CLI status projected granted roots: %#v", undeclared.GrantedRoots)
	}

	// Status under a DIFFERENT instance token projects nothing either: a
	// recreated change reusing this archived name inherits no authority.
	recreated := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change, "--change-instance", "recreated-instance-token"})
	if recreated.GrantedRoots != nil {
		t.Fatalf("recreated-instance CLI status resurrected granted roots: %#v", recreated.GrantedRoots)
	}

	// An exact duplicate request-id is idempotent through the CLI: same
	// committed revision, no second record.
	replayed := runSDDAttemptStatus(t, grantArgs)
	if replayed.Revision != granted.Revision || !reflect.DeepEqual(replayed.GrantedRoots, []string{sibling}) {
		t.Fatalf("grant CLI replay = %#v, want committed revision %s", replayed, granted.Revision)
	}

	// A widening grant reusing the SAME instance token chains on the first
	// revision, accumulates the new root after the already-granted one, and
	// deduplicates the repeat.
	second, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	accumulated := runSDDAttemptStatus(t, []string{
		"grant", "--cwd", repo, "--change", change, "--expected-revision", granted.Revision,
		"--root", second, "--root", sibling, "--change-instance", instance,
		"--actor", "maintainer", "--reason", "maintainer widened the change to a second sibling", "--request-id", "cli-grant-2",
	})
	if !reflect.DeepEqual(accumulated.GrantedRoots, []string{sibling, second}) {
		t.Fatalf("accumulated CLI granted roots = %#v, want [%q %q]", accumulated.GrantedRoots, sibling, second)
	}
}

func TestRunSDDAttemptHelpContractsCoverEveryOperation(t *testing.T) {
	tests := []struct {
		operation string
		flags     []string
		contracts []string
	}{
		{"status", []string{"cwd", "change", "change-instance", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines"}, []string{"optional", "128 bytes"}},
		{"begin", []string{"cwd", "change", "expected-revision", "request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines", "untracked-scope", "expected-untracked-inventory", "intended-untracked"}, []string{"default 2", "default 200", "1..100", "1..1000000"}},
		{"finish", []string{"cwd", "change", "expected-revision", "request-id", "outcome", "evidence-revision", "diagnosis", "harness-disposition", "cleanup-evidence", "process-evidence", "remediates-evidence-revision"}, []string{"failed, interrupted, or passed", "reused or invalidated", "empty or canonical legacy sha256 revision", "500 bytes"}},
		{"handoff", []string{"cwd", "change", "expected-revision", "request-id", "destination-worktree"}, []string{"registered linked worktree", "Git common directory"}},
		{"reset", []string{"cwd", "change", "expected-revision", "request-id", "reason", "actor"}, []string{"500 bytes", "128 bytes"}},
		{"rescope", []string{"cwd", "change", "expected-revision", "request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines", "reason", "actor"}, []string{"explicit limit", "cannot exceed current objective"}},
		{"acquire", []string{"cwd", "change", "token", "request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines", "remediates-evidence-revision", "untracked-scope", "expected-untracked-inventory", "intended-untracked"}, []string{"default 2", "default 200", "failed evidence correction"}},
		{"repair", []string{"cwd", "change", "expected-revision", "request-id", "reason", "actor"}, []string{"unreadable sha256", "500 bytes", "128 bytes"}},
		{"settle", []string{"cwd", "change", "token", "request-id", "outcome", "evidence-revision", "diagnosis", "harness-disposition", "cleanup-evidence", "process-evidence", "remediates-evidence-revision"}, []string{"opaque token returned by acquire", "required for failed/passed; omit for interrupted"}},
		{"grant", []string{"cwd", "change", "expected-revision", "root", "change-instance", "request-id", "actor", "reason"}, []string{"repeatable", "1..32", "4096 bytes"}},
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunSDDAttempt([]string{tt.operation, "--help"}, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%s --help): %v", tt.operation, err)
			}
			if want := "Usage: gentle-ai sdd-attempt " + tt.operation + " [flags]"; !strings.Contains(output.String(), want) {
				t.Fatalf("help missing %q:\n%s", want, output.String())
			}
			if got := sddAttemptHelpFlagNames(output.String()); !reflect.DeepEqual(got, tt.flags) {
				t.Fatalf("help flags = %v, want %v\n%s", got, tt.flags, output.String())
			}
			for _, contract := range tt.contracts {
				if !strings.Contains(output.String(), contract) {
					t.Fatalf("help missing contract %q:\n%s", contract, output.String())
				}
			}
		})
	}

	var overview bytes.Buffer
	if err := RunSDDAttempt([]string{"--help"}, &overview); err != nil {
		t.Fatalf("RunSDDAttempt(--help): %v", err)
	}
	for _, operation := range sddAttemptOperationNames() {
		if !strings.Contains(overview.String(), operation) {
			t.Fatalf("overview omits %q:\n%s", operation, overview.String())
		}
	}
}

func TestRunSDDAttemptHelpAliasesArePositionIndependentAndRepositoryFree(t *testing.T) {
	missingRepository := filepath.Join(t.TempDir(), "does-not-exist")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"parent long alias", []string{"--help", "--cwd", missingRepository}, "Usage: gentle-ai sdd-attempt <"},
		{"parent short alias", []string{"-h", "--change", "missing"}, "Usage: gentle-ai sdd-attempt <"},
		{"operation after help", []string{"--help", "begin", "--cwd", missingRepository, "--change", "missing"}, "Usage: gentle-ai sdd-attempt begin [flags]"},
		{"operation before short alias", []string{"finish", "--cwd", missingRepository, "-h", "--change", "missing"}, "Usage: gentle-ai sdd-attempt finish [flags]"},
		{"help between status inputs", []string{"status", "--change-instance", "instance", "--help", "--cwd", missingRepository, "--change", "missing"}, "Usage: gentle-ai sdd-attempt status [flags]"},
		{"trailing grant help", []string{"grant", "--cwd", missingRepository, "--root", missingRepository, "--change", "missing", "--help"}, "Usage: gentle-ai sdd-attempt grant [flags]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunSDDAttempt(tt.args, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%v): %v", tt.args, err)
			}
			if !strings.Contains(output.String(), tt.want) {
				t.Fatalf("help missing %q:\n%s", tt.want, output.String())
			}
		})
	}
}

func TestRunSDDAttemptHelpDoesNotSelectOperationLikeFlagValues(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		value string
	}{
		{"path", []string{"--help", "--cwd", "status"}, "status"},
		{"change", []string{"--help", "--change", "begin"}, "begin"},
		{"revision", []string{"--help", "--expected-revision", "finish"}, "finish"},
		{"request ID", []string{"--help", "--request-id", "reset"}, "reset"},
		{"work unit", []string{"--help", "--work-unit", "rescope"}, "rescope"},
		{"reason", []string{"--help", "--reason", "acquire"}, "acquire"},
		{"actor", []string{"--help", "--actor", "settle"}, "settle"},
		{"outcome", []string{"--help", "--outcome", "grant"}, "grant"},
		{"missing value before help", []string{"--change", "--help", "begin"}, "begin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunSDDAttempt(tt.args, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%v): %v", tt.args, err)
			}
			if !strings.Contains(output.String(), "Usage: gentle-ai sdd-attempt <") {
				t.Fatalf("operation-like value %q selected operation help:\n%s", tt.value, output.String())
			}
		})
	}
}

func sddAttemptHelpFlagNames(output string) []string {
	flags := []string{}
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "  --") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 0 {
			flags = append(flags, strings.TrimPrefix(fields[0], "--"))
		}
	}
	return flags
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

	// The bounded correction lands during the attempt. SDD completion is
	// independent from review receipt authority.
	writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# bounded self remediation\n")

	finishArgs := []string{
		"finish", "--cwd", repo, "--change", change, "--expected-revision", active.Revision, "--request-id", "self-finish-2",
		"--outcome", "passed", "--evidence-revision", cliAttemptHash('b'),
		"--diagnosis", "bounded self remediation passed corrected verification", "--harness-disposition", "reused",
		"--cleanup-evidence", "self remediation cleanup completed", "--process-evidence", "self remediation process scan found no descendants",
		"--remediates-evidence-revision", failedEvidence,
	}
	completed := runSDDAttemptStatus(t, finishArgs)
	if !completed.Complete || completed.ActiveAttempt != nil || completed.NextAction != sddstatus.RuntimeActionComplete {
		t.Fatalf("self-remediation CLI completion = %#v", completed)
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
	if err := state.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-review", state); err != nil {
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

// TestRunSDDAttemptTrimsWhitespaceFromRevisionShapedFlags is the RED
// reproduction of the CLI-boundary half of #2294: a reporter pasting
// --evidence-revision from PowerShell `Get-FileHash` or `shasum` output often
// carries incidental leading/trailing whitespace, which the sha256:<64-hex>
// pattern then rejects for a reason that has nothing to do with the actual
// evidence-revision defect being reported. The CLI boundary must trim
// identity/revision-shaped flag values before they ever reach sddstatus,
// which stays a pure validator with no normalization of its own.
func TestRunSDDAttemptTrimsWhitespaceFromRevisionShapedFlags(t *testing.T) {
	repo := initReviewCLIRepo(t)
	hash := cliAttemptHash('a')
	padded := "  " + hash + "\n"

	started := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", "cli-trim", "--expected-revision=", "--request-id", "trim-begin",
		"--work-unit", "trim-unit", "--evidence-goal", "prove trimmed CLI revisions", "--max-attempts", "1", "--max-changed-lines", "10",
	})

	finished := runSDDAttemptStatus(t, []string{
		"finish", "--cwd", repo, "--change", "cli-trim", "--expected-revision", started.Revision, "--request-id", "trim-finish",
		"--outcome", "failed", "--evidence-revision", padded,
		"--diagnosis", "diagnosis", "--harness-disposition", "reused",
		"--cleanup-evidence", "cleanup", "--process-evidence", "process",
	})
	last := finished.Attempts[len(finished.Attempts)-1]
	if last.EvidenceRevision != hash {
		t.Fatalf("finish with a whitespace-padded --evidence-revision = %#v, want trimmed evidence_revision %q", last, hash)
	}
}
