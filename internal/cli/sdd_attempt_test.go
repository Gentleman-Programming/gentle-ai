package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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
		"--outcome", "interrupted", "--evidence-revision", cliAttemptHash('d'),
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
		{name: "missing operation", args: nil, want: "requires status, begin, finish, reset, rescope, acquire, or settle"},
		// The no-args refusal already enumerates every valid operation; the
		// unknown-operation refusal must do the same instead of naming only
		// the bad value with no route to the valid set.
		{name: "unknown operation", args: []string{"begn"}, want: `unknown sdd-attempt operation "begn"; want one of status, begin, finish, reset, rescope, acquire, or settle`},
		{name: "missing change", args: []string{"status", "--cwd", repo}, want: "--change"},
		{name: "unknown flag", args: []string{"status", "--cwd", repo, "--change", "thin", "--mystery"}, want: "flag provided but not defined"},
		{name: "irrelevant flag", args: []string{"status", "--cwd", repo, "--change", "thin", "--outcome", "failed"}, want: "flag provided but not defined"},
		{name: "rescope rejects finish-only flag", args: []string{"rescope", "--cwd", repo, "--change", "thin", "--outcome", "failed"}, want: "flag provided but not defined"},
		{name: "missing begin CAS", args: []string{"begin", "--cwd", repo, "--change", "thin", "--request-id", "begin", "--work-unit", "unit", "--evidence-goal", "goal"}, want: "--expected-revision"},
		{name: "missing rescope scope", args: []string{"rescope", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('e'), "--request-id", "rescope", "--reason", "narrowing", "--actor", "maintainer"}, want: "--work-unit"},
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
	want := []string{"status", "begin", "finish", "reset", "rescope", "acquire", "settle"}
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
	if got := joinSDDAttemptOperations(); got != "status, begin, finish, reset, rescope, acquire, or settle" {
		t.Fatalf("joinSDDAttemptOperations() = %q, want %q", got, "status, begin, finish, reset, rescope, acquire, or settle")
	}
}

func TestSDDAttemptRuntimeContractAuthority(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		valid   string
		invalid string
	}{
		{name: "revision", pattern: sddstatus.RuntimeRevisionPattern, valid: cliAttemptHash('a'), invalid: "sha256:" + strings.Repeat("A", 64)},
		{name: "request id", pattern: sddstatus.RuntimeRequestIDPattern, valid: "request-1.v2", invalid: "Request ID"},
		{name: "change", pattern: sddstatus.RuntimeChangePattern, valid: "sdd-cli-help-contracts", invalid: "sdd_cli_help_contracts"},
		{name: "lineage", pattern: sddstatus.RuntimeLineagePattern, valid: "review-successor", invalid: "ReviewSuccessor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := regexp.MustCompile(tt.pattern)
			if !pattern.MatchString(tt.valid) {
				t.Fatalf("%s pattern %q rejected valid value %q", tt.name, tt.pattern, tt.valid)
			}
			if pattern.MatchString(tt.invalid) {
				t.Fatalf("%s pattern %q accepted invalid value %q", tt.name, tt.pattern, tt.invalid)
			}
		})
	}

	if got, want := sddstatus.RuntimeDefaultAttemptLimit, 2; got != want {
		t.Fatalf("default attempt limit = %d, want %d", got, want)
	}
	if got, want := sddstatus.RuntimeMaxAttemptLimit, 100; got != want {
		t.Fatalf("max attempt limit = %d, want %d", got, want)
	}
	if got, want := sddstatus.RuntimeDefaultChangedLines, 200; got != want {
		t.Fatalf("default changed lines = %d, want %d", got, want)
	}
	if got, want := sddstatus.RuntimeMaxChangedLines, 1_000_000; got != want {
		t.Fatalf("max changed lines = %d, want %d", got, want)
	}
	for name, limits := range map[string][2]int{
		"work unit":          {sddstatus.RuntimeWorkUnitLimit, 160},
		"evidence goal":      {sddstatus.RuntimeEvidenceGoalLimit, 240},
		"diagnosis":          {sddstatus.RuntimeDiagnosisLimit, 500},
		"cleanup evidence":   {sddstatus.RuntimeCleanupEvidenceLimit, 500},
		"process evidence":   {sddstatus.RuntimeProcessEvidenceLimit, 500},
		"reset reason":       {sddstatus.RuntimeResetReasonLimit, 500},
		"actor":              {sddstatus.RuntimeActorLimit, 128},
		"change identifier":  {sddstatus.RuntimeChangeLimit, 96},
		"lineage identifier": {sddstatus.RuntimeLineageLimit, 128},
	} {
		if limits[0] != limits[1] {
			t.Fatalf("%s limit = %d, want %d", name, limits[0], limits[1])
		}
	}

	if got, want := sddstatus.RuntimeTerminalOutcomes(), []string{"failed", "interrupted", "passed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal outcomes = %v, want %v", got, want)
	}
	if got, want := sddstatus.RuntimeHarnessDispositions(), []string{"reused", "invalidated"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("harness dispositions = %v, want %v", got, want)
	}
}

func TestRunSDDAttemptHelpContracts(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		want     []string
		unwanted []string
	}{
		{
			name: "top level short help",
			args: []string{"-h"},
			want: []string{
				"Usage: gentle-ai sdd-attempt <operation> [flags]", "status", "begin", "finish", "reset", "rescope", "acquire", "settle",
				"--cwd", "--change", "acquire a bounded attempt", "settle a bounded attempt",
				"change grammar", "lineage grammar",
				"Request IDs are idempotency keys",
			},
			unwanted: []string{"--token", "--successor-lineage", "--expected-binding-revision"},
		},
		{
			name: "top level long help",
			args: []string{"--help"},
			want: []string{
				"Usage: gentle-ai sdd-attempt <operation> [flags]", "acquire", "settle",
				"change grammar", "lineage grammar",
			},
			// The detailed acquire/settle flag contract is intentionally not
			// surfaced at the top level; it lives only behind the compact
			// operations themselves (decision: issue #1937 scope).
			unwanted: []string{"--token", "--request-id", "--successor-lineage", "Acquire and settle are compact orchestration operations"},
		},
		{
			name: "status",
			args: []string{"status", "--help"},
			want: []string{
				"Usage: gentle-ai sdd-attempt status [flags]", "--cwd", "--change", "revision", "active_attempt",
				"active_attempt.ordinal is generated by runtime, starts at 1, and callers do not supply it",
			},
			unwanted: []string{"--outcome", "--reason", "--request-id"},
		},
		{
			name: "begin",
			args: []string{"begin", "--help"},
			want: []string{
				"Usage: gentle-ai sdd-attempt begin [flags]", "--expected-revision", "--request-id", "--work-unit",
				"--evidence-goal", "--max-attempts", "--max-changed-lines", "default 2", "default 200",
				"A non-nil RuntimeStatus.active_attempt blocks begin.",
				"change grammar", "lineage grammar",
				"Request IDs are idempotency keys: replaying the same request with the same ID returns its committed result; reusing an ID with different fields is rejected",
				`gentle-ai sdd-attempt begin --cwd "$REPO_DIR" --change runtime-demo --expected-revision="" --request-id begin-runtime-demo-1`,
			},
			unwanted: []string{"--outcome", "--reason", "--token"},
		},
		{
			name: "finish",
			args: []string{"finish", "--help"},
			want: []string{
				"Usage: gentle-ai sdd-attempt finish [flags]", "--expected-revision", "--request-id", "--outcome",
				"--evidence-revision", "--diagnosis", "--harness-disposition", "--cleanup-evidence",
				"--process-evidence", "--expected-binding-revision", "--successor-lineage",
				"--remediates-evidence-revision", "failed|interrupted|passed", "reused|invalidated", "sha256:",
				"active_attempt", "binding_revision", "evidence_revision",
				"finish requires a non-nil, running RuntimeStatus.active_attempt.",
				"change grammar", "lineage grammar",
				"Request IDs are idempotency keys: identical request digests may replay under the same request ID, while different fields are rejected",
				`gentle-ai sdd-attempt finish --cwd "$REPO_DIR" --change runtime-demo --expected-revision "${CURRENT_REVISION:?set from status.revision}" --request-id finish-runtime-demo-1`,
			},
			unwanted: []string{"--max-attempts", "--reason", "planning relationship"},
		},
		{
			name: "reset",
			args: []string{"reset", "--help"},
			want: []string{
				"Usage: gentle-ai sdd-attempt reset [flags]", "--expected-revision", "--request-id", "--reason", "--actor",
				"A non-nil RuntimeStatus.active_attempt blocks reset.",
				"change grammar", "lineage grammar",
				"Request IDs are idempotency keys: replaying the same request with the same ID returns its committed result; reusing an ID with different fields is rejected",
			},
			unwanted: []string{"--outcome", "--work-unit", "--token"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunSDDAttempt(tt.args, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%v) error = %v", tt.args, err)
			}
			text := output.String()
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("help missing %q:\n%s", want, text)
				}
			}
			for _, unwanted := range tt.unwanted {
				if strings.Contains(text, unwanted) {
					t.Errorf("help unexpectedly contains %q:\n%s", unwanted, text)
				}
			}
		})
	}
}

func TestRunSDDAttemptRescopeHelpContractIsAuthoritative(t *testing.T) {
	missingRepo := filepath.Join(t.TempDir(), "does-not-exist")
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "operation before long help",
			args: []string{"rescope", "--cwd", missingRepo, "--change", "runtime-help", "--help"},
		},
		{
			name: "value-taking flag before short help",
			args: []string{"--work-unit", "ignored", "-h", "rescope", "--cwd", missingRepo, "--change", "runtime-help"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunSDDAttempt(tt.args, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%v) error = %v", tt.args, err)
			}
			text := output.String()
			want := []string{
				"Usage: gentle-ai sdd-attempt rescope [flags]",
				"narrow a terminal zero-drift runtime objective",
				"--cwd", "--change", "--expected-revision", "--request-id", "--work-unit", "--evidence-goal",
				"--max-attempts", "--max-changed-lines", "--reason", "--actor",
				"not decision-required or complete",
				"terminal failed or interrupted",
				"candidate has not drifted",
				"required explicit ceilings",
				"zero is invalid rather than defaulted",
				"carry forward unchanged",
				"maintainer-authorized successor scope",
				"Request IDs are idempotency keys",
				`gentle-ai sdd-attempt rescope --cwd "$REPO_DIR"`,
				sddstatus.RuntimeRevisionPattern,
				sddstatus.RuntimeRequestIDPattern,
				sddstatus.RuntimeChangePattern,
				sddstatus.RuntimeLineagePattern,
			}
			for _, token := range want {
				if !strings.Contains(text, token) {
					t.Errorf("rescope help missing %q:\n%s", token, text)
				}
			}
			if strings.Contains(text, "Usage: gentle-ai sdd-attempt <operation> [flags]") {
				t.Errorf("rescope help unexpectedly fell back to top-level help:\n%s", text)
			}
			if _, err := os.Stat(missingRepo); !os.IsNotExist(err) {
				t.Fatalf("help accessed or created nonexistent repository %q: %v", missingRepo, err)
			}
		})
	}
}

func TestRunSDDAttemptHelpIsPositionIndependentAndDependencyFree(t *testing.T) {
	missingRepo := filepath.Join(t.TempDir(), "does-not-exist")
	for _, args := range [][]string{
		{"--help", "status", "--cwd", missingRepo, "--change", "help-contract"},
		{"status", "--cwd", missingRepo, "--help", "--change", "help-contract"},
		{"--cwd", missingRepo, "--change", "help-contract", "status", "--help"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var output bytes.Buffer
			if err := RunSDDAttempt(args, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%v) error = %v", args, err)
			}
			if !strings.Contains(output.String(), "Usage: gentle-ai sdd-attempt status [flags]") {
				t.Fatalf("status help not selected for %v:\n%s", args, output.String())
			}
			if _, err := os.Stat(missingRepo); !os.IsNotExist(err) {
				t.Fatalf("help accessed or created nonexistent repository %q: %v", missingRepo, err)
			}
		})
	}
}

// TestRunSDDAttemptCompactOperationHelpIsOperationSpecific proves acquire and
// settle render their own operation-specific help — not the generic top-level
// fallback — for both help aliases, at arbitrary positions, without touching the
// repository, with their correct flags and without unrelated operation flags.
func TestRunSDDAttemptCompactOperationHelpIsOperationSpecific(t *testing.T) {
	missingRepo := filepath.Join(t.TempDir(), "does-not-exist")
	tests := []struct {
		name      string
		operation string
		helpAlias string
		// helpPosition embeds the help alias at an arbitrary position among
		// real-looking flags that must never be read during help.
		helpPosition func(operation, alias string) []string
		want         []string
		unwanted     []string
	}{
		{
			name: "acquire short help anywhere", operation: "acquire", helpAlias: "-h",
			helpPosition: func(operation, alias string) []string {
				return []string{operation, "--cwd", missingRepo, "--change", "help-contract", alias, "--request-id", "should-not-be-read"}
			},
			want: []string{
				"Usage: gentle-ai sdd-attempt acquire [flags]",
				"--cwd", "--change", "--token", "--request-id", "--work-unit", "--evidence-goal",
				"--max-attempts", "--max-changed-lines",
				"acquire a bounded attempt",
				"Acquire claims one bounded attempt without exposing the growing runtime history",
				"A supplied --token proves the caller continues the same active attempt",
				"token identifies that exact begin record for settle",
				"Replaying the same --request-id returns its committed CompactAttemptResult",
				"blocks acquire and returns CompactAttemptResult.state=blocked",
				"change grammar", "lineage grammar",
				`gentle-ai sdd-attempt acquire --cwd "$REPO_DIR"`,
			},
			unwanted: []string{
				"--outcome", "--evidence-revision", "--diagnosis",
				"--harness-disposition", "--cleanup-evidence", "--process-evidence",
				"--successor-lineage", "--remediates-evidence-revision", "--expected-binding-revision",
				"--reason", "--actor", "--expected-revision",
				"Usage: gentle-ai sdd-attempt <operation> [flags]",
				"settle a bounded attempt",
			},
		},
		{
			name: "acquire long help anywhere", operation: "acquire", helpAlias: "--help",
			helpPosition: func(operation, alias string) []string {
				return []string{"--cwd", missingRepo, alias, "--change", "help-contract", operation, "--request-id", "should-not-be-read"}
			},
			want: []string{
				"Usage: gentle-ai sdd-attempt acquire [flags]",
				"--token",
				"acquire a bounded attempt",
				"Acquire claims one bounded attempt without exposing the growing runtime history",
			},
			unwanted: []string{"Usage: gentle-ai sdd-attempt <operation> [flags]"},
		},
		{
			name: "settle short help anywhere", operation: "settle", helpAlias: "-h",
			helpPosition: func(operation, alias string) []string {
				return []string{operation, "--token", "sha256:" + strings.Repeat("a", 64), alias, "--cwd", missingRepo, "--change", "help-contract", "--request-id", "should-not-be-read"}
			},
			want: []string{
				"Usage: gentle-ai sdd-attempt settle [flags]",
				"--cwd", "--change", "--token", "--request-id", "--outcome",
				"--evidence-revision", "--diagnosis", "--harness-disposition",
				"--cleanup-evidence", "--process-evidence", "--successor-lineage",
				"--remediates-evidence-revision",
				"settle a bounded attempt",
				"Settle closes the attempt selected by --token through the ordinary Finish transition",
				"current binding and failed-evidence revisions are derived inside the authority",
				"Replaying the same --request-id returns its committed CompactAttemptResult",
				"name a distinct recovery lineage only",
				"failed|interrupted|passed", "reused|invalidated",
				"change grammar", "lineage grammar",
				`gentle-ai sdd-attempt settle --cwd "$REPO_DIR"`,
			},
			unwanted: []string{
				"--work-unit", "--evidence-goal", "--max-attempts", "--max-changed-lines",
				"--reason", "--actor", "--expected-revision", "--expected-binding-revision",
				"Usage: gentle-ai sdd-attempt <operation> [flags]",
				"acquire a bounded attempt",
			},
		},
		{
			name: "settle long help anywhere", operation: "settle", helpAlias: "--help",
			helpPosition: func(operation, alias string) []string {
				return []string{alias, operation, "--cwd", missingRepo, "--change", "help-contract", "--token", "should-not-be-read"}
			},
			want: []string{
				"Usage: gentle-ai sdd-attempt settle [flags]",
				"settle a bounded attempt",
				"Settle closes the attempt selected by --token through the ordinary Finish transition",
			},
			unwanted: []string{"--work-unit", "Usage: gentle-ai sdd-attempt <operation> [flags]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.helpPosition(tt.operation, tt.helpAlias)
			var output bytes.Buffer
			if err := RunSDDAttempt(args, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%v) error = %v", args, err)
			}
			text := output.String()
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("help missing %q:\n%s", want, text)
				}
			}
			for _, unwanted := range tt.unwanted {
				if strings.Contains(text, unwanted) {
					t.Errorf("help unexpectedly contains %q:\n%s", unwanted, text)
				}
			}
			if _, err := os.Stat(missingRepo); !os.IsNotExist(err) {
				t.Fatalf("help accessed or created nonexistent repository %q: %v", missingRepo, err)
			}
		})
	}
}

// TestRunSDDAttemptHelpAliasInValueSlotSelectsOperationHelp proves the help
// alias is detected even when it occupies the would-be value position of a
// value-taking flag, and the operation token later in argv still selects
// operation-specific help. Mirrors issue #1937 position-independent help.
func TestRunSDDAttemptHelpAliasInValueSlotSelectsOperationHelp(t *testing.T) {
	missingRepo := filepath.Join(t.TempDir(), "does-not-exist")
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "cwd short help value slot then status",
			args: []string{"--cwd", "-h", "status"},
			want: []string{"Usage: gentle-ai sdd-attempt status [flags]", "--cwd", "--change", "show the current native runtime status"},
		},
		{
			name: "cwd long help value slot then status",
			args: []string{"--cwd", "--help", "status"},
			want: []string{"Usage: gentle-ai sdd-attempt status [flags]", "show the current native runtime status"},
		},
		{
			name: "status then cwd short help value slot",
			args: []string{"status", "--cwd", "-h"},
			want: []string{"Usage: gentle-ai sdd-attempt status [flags]", "show the current native runtime status"},
		},
		{
			name: "change short help value slot then status",
			args: []string{"--change", "-h", "status"},
			want: []string{"Usage: gentle-ai sdd-attempt status [flags]", "--change", "show the current native runtime status"},
		},
		{
			name: "request-id long help value slot then begin",
			args: []string{"--request-id", "--help", "begin"},
			want: []string{"Usage: gentle-ai sdd-attempt begin [flags]", "start a bounded runtime attempt", "--expected-revision", "--work-unit"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunSDDAttempt(tt.args, &output); err != nil {
				t.Fatalf("RunSDDAttempt(%v) error = %v", tt.args, err)
			}
			text := output.String()
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("help missing %q:\n%s", want, text)
				}
			}
			if !strings.Contains(text, "Usage: gentle-ai sdd-attempt status [flags]") && !strings.Contains(text, "Usage: gentle-ai sdd-attempt begin [flags]") {
				t.Fatalf("operation-specific help not selected for %v:\n%s", tt.args, text)
			}
			if _, err := os.Stat(missingRepo); !os.IsNotExist(err) {
				t.Fatalf("help accessed or created nonexistent repository %q: %v", missingRepo, err)
			}
		})
	}
}

// TestRunSDDAttemptFinishHelpContractIsAuthoritative proves the finish
// help exposes the authoritative runtime contract: the ten required flags,
// the conditional remediation trio, exact authority-derived patterns and
// enums, the invalid literal none, the replay-digest rule, the 500-byte
// (not character) boundaries, the status mappings, and the shell-runnable
// example. Mirrors the sdd-finish-help-contract spec requirements.
func TestRunSDDAttemptFinishHelpContractIsAuthoritative(t *testing.T) {
	var output bytes.Buffer
	if err := RunSDDAttempt([]string{"finish", "--help"}, &output); err != nil {
		t.Fatalf("RunSDDAttempt finish --help error = %v", err)
	}
	text := output.String()
	// Ten required flags MUST be marked with the [required] annotation and be
	// individually discoverable. Asserting the exact marker (not the loose
	// "required" substring) forces a real annotation rather than a trivial
	// hit on existing wording like "required lowercase sha256".
	required := []string{
		"--cwd", "--change", "--expected-revision", "--request-id", "--outcome",
		"--evidence-revision", "--diagnosis", "--harness-disposition",
		"--cleanup-evidence", "--process-evidence",
	}
	for _, flag := range required {
		if !strings.Contains(text, flag) {
			t.Errorf("finish help missing required flag %q:\n%s", flag, text)
		}
	}
	if !strings.Contains(text, "[required]") {
		t.Errorf("finish help missing [required] annotation:\n%s", text)
	}
	// The conditional remediation trio MUST carry the [conditional] marker and
	// be described as all-or-none and valid only for passed attempts.
	conditional := []string{
		"--expected-binding-revision", "--successor-lineage", "--remediates-evidence-revision",
	}
	for _, flag := range conditional {
		if !strings.Contains(text, flag) {
			t.Errorf("finish help missing conditional flag %q:\n%s", flag, text)
		}
	}
	if !strings.Contains(text, "[conditional]") {
		t.Errorf("finish help missing [conditional] annotation:\n%s", text)
	}
	if !strings.Contains(text, "all-or-none") {
		t.Errorf("finish help missing all-or-none rule:\n%s", text)
	}
	if !strings.Contains(text, "passed") {
		t.Errorf("finish help missing passed-only remediation rule:\n%s", text)
	}
	// Authority-derived patterns and enums MUST appear verbatim, including the
	// request-ID grammar which the prior finish contract omitted.
	authority := []string{
		sddstatus.RuntimeRevisionPattern,
		sddstatus.RuntimeRequestIDPattern,
		sddstatus.RuntimeChangePattern,
		sddstatus.RuntimeLineagePattern,
		strings.Join(sddstatus.RuntimeTerminalOutcomes(), "|"),
		strings.Join(sddstatus.RuntimeHarnessDispositions(), "|"),
	}
	for _, token := range authority {
		if !strings.Contains(text, token) {
			t.Errorf("finish help missing authority token %q:\n%s", token, text)
		}
	}
	// The literal none MUST be explicitly stated invalid.
	if !strings.Contains(text, "none") || !strings.Contains(text, "invalid") {
		t.Errorf("finish help missing invalid none statement:\n%s", text)
	}
	// The replay-digest rule MUST state identical request digests may replay
	// under one request ID while different fields are rejected.
	if !strings.Contains(text, "replay") {
		t.Errorf("finish help missing replay rule:\n%s", text)
	}
	// The bounded text fields MUST state byte semantics (inclusive 500 bytes),
	// explicitly, and MUST NOT keep the misleading "500 characters" wording.
	// (Other "characters" wordings on shared limits are out of scope and
	// untouched so unrelated operations stay stable.)
	if !strings.Contains(text, "500 bytes") {
		t.Errorf("finish help missing 500 bytes invariant:\n%s", text)
	}
	if strings.Contains(text, "500 characters") {
		t.Errorf("finish help must state 500 bytes not 500 characters:\n%s", text)
	}
	// Status mappings MUST be described explicitly, not merely present as flag
	// names in the flag list.
	mappings := []string{
		"status.revision",
		"active_attempt",
		"binding_revision",
		"evidence_revision",
		"--expected-binding-revision",
		"--remediates-evidence-revision",
		"--evidence-revision",
		"--successor-lineage",
		"--expected-revision",
	}
	for _, mapping := range mappings {
		if !strings.Contains(text, mapping) {
			t.Errorf("finish help missing status mapping %q:\n%s", mapping, text)
		}
	}
	if !strings.Contains(text, "maps to") {
		t.Errorf("finish help missing explicit map wording:\n%s", text)
	}
	// The shell-runnable failed example MUST be present and complete.
	if !strings.Contains(text, `gentle-ai sdd-attempt finish --cwd "$REPO_DIR"`) {
		t.Errorf("finish help missing runnable example:\n%s", text)
	}
	if !strings.Contains(text, "--outcome failed") {
		t.Errorf("finish help example missing failed outcome:\n%s", text)
	}
}

// TestRunSDDAttemptFinishRefusalHintsFinishHelp proves the exact
// `gentle-ai sdd-attempt finish --help` pointer appears only on the three
// finish pre-validation refusal classes (unknown/invalid finish flag,
// missing required flag, and partial remediation trio) and never on
// post-store CAS/runtime errors or unrelated operations. Mirrors the
// spec "Hints stop at pre-validation" scenario.
func TestRunSDDAttemptFinishRefusalHintsFinishHelp(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const pointer = "gentle-ai sdd-attempt finish --help"
	tests := []struct {
		name      string
		args      []string
		wantHint  bool
		wantError string
	}{
		{
			name:      "unknown finish flag",
			args:      []string{"finish", "--cwd", repo, "--change", "thin", "--mystery"},
			wantHint:  true,
			wantError: "flag provided but not defined",
		},
		{
			name:      "missing required finish flag",
			args:      []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "finish", "--outcome", "failed", "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process"},
			wantHint:  true,
			wantError: "--evidence-revision",
		},
		{
			name:      "partial remediation trio",
			args:      []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "finish", "--outcome", "passed", "--evidence-revision", cliAttemptHash('c'), "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process", "--successor-lineage", "review-successor"},
			wantHint:  true,
			wantError: "remediation successor requires --expected-binding-revision, --successor-lineage, and --remediates-evidence-revision together",
		},
		{
			name:      "post-store CAS error no hint",
			args:      []string{"finish", "--cwd", repo, "--change", "thin", "--expected-revision", "sha256:" + strings.Repeat("a", 64), "--request-id", "finish-cas", "--outcome", "failed", "--evidence-revision", cliAttemptHash('z'), "--diagnosis", "diagnosis", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup", "--process-evidence", "process"},
			wantHint:  false,
			wantError: "sdd-attempt finish:",
		},
		{
			name:      "unrelated begin operation no hint",
			args:      []string{"begin", "--cwd", repo, "--change", "thin", "--request-id", "begin", "--work-unit", "unit", "--evidence-goal", "goal"},
			wantHint:  false,
			wantError: "--expected-revision",
		},
		{
			name:      "unrelated reset operation no hint",
			args:      []string{"reset", "--cwd", repo, "--change", "thin", "--expected-revision", cliAttemptHash('b'), "--request-id", "reset", "--reason", "reason", "--actor", "actor"},
			wantHint:  false,
			wantError: "sdd-attempt reset:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunSDDAttempt(tt.args, &output)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("RunSDDAttempt(%v) = output %q, err %v, want error containing %q", tt.args, output.String(), err, tt.wantError)
			}
			containsHint := strings.Contains(err.Error(), pointer)
			if tt.wantHint && !containsHint {
				t.Fatalf("refusal %q missing finish-help pointer:\nerr=%v", tt.name, err)
			}
			if !tt.wantHint && containsHint {
				t.Fatalf("non-pre-validation error %q unexpectedly contains finish-help pointer:\nerr=%v", tt.name, err)
			}
		})
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
