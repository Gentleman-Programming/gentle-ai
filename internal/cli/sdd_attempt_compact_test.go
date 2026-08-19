package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

type compactAttemptOutput struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Token  string `json:"token,omitempty"`
	Exit   string `json:"exit,omitempty"`
	Detail string `json:"detail,omitempty"`
	// SettleObligation rides the proceed envelope (#2912): what this attempt's
	// passing settle will already owe, named while the attempt is still
	// unspent.
	SettleObligation string                    `json:"settle_obligation,omitempty"`
	ItemSettlement   *sddstatus.ItemSettlement `json:"item_settlement,omitempty"`
}

func TestRunSDDAttemptCompactOutputStaysBoundedAcrossHistory(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const change = "compact-history"

	var acquireSize, settleSize int
	for attempt := 1; attempt <= 10; attempt++ {
		acquired, acquirePayload := runCompactSDDAttempt(t, []string{
			"acquire", "--cwd", repo, "--change", change,
			"--request-id", fmt.Sprintf("compact-acquire-%d", attempt),
			"--work-unit", "runtime-proof", "--evidence-goal", "prove compact orchestration",
			"--max-attempts", "12", "--max-changed-lines", "200",
		})
		if acquired.State != "proceed" || acquired.Reason != "" || !strings.HasPrefix(acquired.Token, "sha256:") {
			t.Fatalf("acquire %d = %#v", attempt, acquired)
		}
		assertCompactPayloadKeys(t, acquirePayload, "state", "token")
		if attempt == 1 {
			acquireSize = len(acquirePayload)
		} else if len(acquirePayload) != acquireSize {
			t.Fatalf("acquire output grew from %d to %d bytes at attempt %d", acquireSize, len(acquirePayload), attempt)
		}

		settled, settlePayload := runCompactSDDAttempt(t, []string{
			"settle", "--cwd", repo, "--change", change, "--token", acquired.Token,
			"--request-id", fmt.Sprintf("compact-settle-%d", attempt), "--outcome", "failed",
			"--evidence-revision", cliAttemptHash(byte('a' + attempt%6)),
			"--diagnosis", "bounded execution produced retryable evidence", "--harness-disposition", "reused",
			"--cleanup-evidence", "process group exited", "--process-evidence", "no descendants remained",
		})
		if settled != (compactAttemptOutput{State: "proceed"}) {
			t.Fatalf("settle %d = %#v", attempt, settled)
		}
		assertCompactPayloadKeys(t, settlePayload, "state")
		if attempt == 1 {
			settleSize = len(settlePayload)
		} else if len(settlePayload) != settleSize {
			t.Fatalf("settle output grew from %d to %d bytes at attempt %d", settleSize, len(settlePayload), attempt)
		}
	}

	var legacy bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", change}, &legacy); err != nil {
		t.Fatal(err)
	}
	var status sddstatus.RuntimeStatus
	if err := json.Unmarshal(legacy.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Attempts) != 10 || acquireSize > 160 || settleSize > 80 || acquireSize >= legacy.Len() {
		t.Fatalf("bounded sizes acquire=%d settle=%d legacy=%d attempts=%d", acquireSize, settleSize, legacy.Len(), len(status.Attempts))
	}
}

func TestRunSDDAttemptLegacyStatusJSONIsUnchanged(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", "legacy-json"}, &output); err != nil {
		t.Fatal(err)
	}
	want := `{
  "schema": "gentle-ai.sdd-runtime-status/v1",
  "change": "legacy-json",
  "revision": "",
  "attempts": [],
  "objective_generation": 0,
  "next_ordinal": 1,
  "cumulative_attempts": 0,
  "cumulative_changed_lines": 0,
  "lifetime_attempts": 0,
  "lifetime_changed_lines": 0,
  "evidence_revision": "",
  "decision_required": false,
  "complete": false,
  "next_action": "begin",
  "binding_revision": ""
}
`
	if output.String() != want {
		t.Fatalf("legacy status JSON changed:\n%s", output.String())
	}
}

func TestRunSDDAttemptCompactBlocksWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string, string, sddstatus.RuntimeStore) (args []string, wantReason, wantToken string)
	}{
		{
			name: "active attempt",
			prepare: func(t *testing.T, repo, change string, _ sddstatus.RuntimeStore) ([]string, string, string) {
				started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "active-owner", 2))
				return compactAcquireArgs(repo, change, "active-contender", 2), "active_attempt", started.Token
			},
		},
		{
			name: "maintainer decision",
			prepare: func(t *testing.T, repo, change string, _ sddstatus.RuntimeStore) ([]string, string, string) {
				started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "decision-acquire", 1))
				settled, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, started.Token, "decision-settle", "failed"))
				if settled.Reason != "maintainer_decision" {
					t.Fatalf("exhausting settle = %#v", settled)
				}
				return compactAcquireArgs(repo, change, "decision-retry", 1), "maintainer_decision", ""
			},
		},
		{
			name: "corrupt authority",
			prepare: func(t *testing.T, repo, change string, store sddstatus.RuntimeStore) ([]string, string, string) {
				runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "corrupt-acquire", 2))
				if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte("corrupt\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return compactAcquireArgs(repo, change, "corrupt-retry", 2), "corrupt_authority", ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			change := "blocked-" + strings.ReplaceAll(tt.name, " ", "-")
			store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			args, wantReason, wantToken := tt.prepare(t, repo, change, store)
			before := snapshotRuntimeAuthorityFiles(t, store.Dir)
			result, payload := runCompactSDDAttempt(t, args)
			after := snapshotRuntimeAuthorityFiles(t, store.Dir)
			if result.State != "blocked" || result.Reason != wantReason || result.Token != wantToken {
				t.Fatalf("blocked result = %#v, want reason=%q token=%q", result, wantReason, wantToken)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("blocked operation mutated authority\nbefore=%v\nafter=%v", before, after)
			}
			// Exit-naming audit fix #2: compactBlocked now names a runnable
			// continuation for every reason it produces (previously a bare
			// {"state":"blocked","reason":"<code>"} with nothing behind it —
			// 21 call sites, zero tests). Every blocked result therefore
			// carries non-empty exit/detail alongside state/reason.
			if result.Exit == "" || result.Detail == "" {
				t.Fatalf("blocked result = %#v, want non-empty Exit/Detail", result)
			}
			keys := []string{"state", "reason", "exit", "detail"}
			if wantToken != "" {
				keys = append(keys, "token")
			}
			assertCompactPayloadKeys(t, payload, keys...)
		})
	}
}

func TestCompactHandoffRefusalPreservesTypedDetailAndRunnableExit(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "compact-handoff-refusal"
	started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "handoff-owner", 2))
	foreign := initReviewCLIRepo(t)

	var output bytes.Buffer
	if err := RunSDDAttempt([]string{
		"handoff", "--cwd", repo, "--change", change, "--expected-revision", started.Token,
		"--request-id", "handoff-foreign", "--destination-worktree", foreign,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var result compactAttemptOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "blocked" || result.Reason != "invalid_continuation" || result.Detail == "" || result.Exit != result.Detail {
		t.Fatalf("foreign compact handoff = %#v", result)
	}
	wantExit := "gentle-ai sdd-attempt status --cwd \"" + repo + "\" --change \"" + change + "\""
	if !strings.Contains(result.Exit, wantExit) {
		t.Fatalf("handoff exit = %q, want runnable %q", result.Exit, wantExit)
	}
	var status bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", change}, &status); err != nil {
		t.Fatalf("handoff exit did not name a runnable status command: %v", err)
	}
}

// TestActiveAttemptBlockedExitNamesAGenuinelyRunnableCommand is the
// execution-based RED-first proof for adversarial finding F2: the
// active_attempt Exit text used to print `gentle-ai sdd-attempt acquire
// --token <t>` and `gentle-ai sdd-attempt settle --token <t>` as if those
// were complete commands, when both actually require five more required
// flags each (--cwd, --change, then either --request-id/--work-unit/
// --evidence-goal for acquire or --request-id/--outcome/--evidence-revision/
// --diagnosis/--harness-disposition/--cleanup-evidence/--process-evidence
// for settle) -- confirmed by executing both against this real CLI. This
// test triggers a genuine active_attempt block, then actually EXECUTES the
// one command the fixed text is required to name in full
// (`sdd-attempt status --cwd <repo> --change <change>`, with real values
// substituted for the placeholders) through RunSDDAttempt -- the same
// dispatch path the compiled binary uses -- and requires the text to never
// claim the acquire/settle forms are complete on their own.
func TestActiveAttemptBlockedExitNamesAGenuinelyRunnableCommand(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "active-attempt-exit-text"
	started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "exit-text-owner", 2))
	blocked, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "exit-text-contender", 2))
	if blocked.State != "blocked" || blocked.Reason != "active_attempt" || blocked.Token != started.Token {
		t.Fatalf("active-attempt setup = %#v, want blocked/active_attempt/%s", blocked, started.Token)
	}
	if blocked.Exit == "" {
		t.Fatal("active_attempt result carries no Exit text to verify")
	}

	// The text must never claim the bare acquire/settle forms are complete:
	// that is exactly the class of defect this test exists to catch.
	for _, incomplete := range []string{
		"run `gentle-ai sdd-attempt acquire --token",
		"run `gentle-ai sdd-attempt settle --token",
	} {
		if strings.Contains(blocked.Exit, incomplete) {
			t.Fatalf("active_attempt Exit still claims an incomplete command is runnable as printed (%q): %q", incomplete, blocked.Exit)
		}
	}

	// The one command the text is allowed to print as complete must
	// actually run. Extract it with real placeholder substitution and
	// execute it through RunSDDAttempt -- not just parse its flags.
	const wantCommand = "gentle-ai sdd-attempt status --cwd <repo> --change <change>"
	if !strings.Contains(blocked.Exit, wantCommand) {
		t.Fatalf("active_attempt Exit does not name %q: %q", wantCommand, blocked.Exit)
	}
	var statusOutput bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", change}, &statusOutput); err != nil {
		t.Fatalf("executing the named command with real --cwd/--change substituted for <repo>/<change> failed: %v\n%s", err, statusOutput.String())
	}
}

func TestRunSDDAttemptCompactPreservesTokenCASAndIdempotentReplay(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "compact-replay"
	acquireArgs := compactAcquireArgs(repo, change, "replay-acquire", 2)
	first, firstPayload := runCompactSDDAttempt(t, acquireArgs)
	replayed, replayedPayload := runCompactSDDAttempt(t, acquireArgs)
	if first.State != "proceed" || first.Token == "" || replayed != first || !bytes.Equal(firstPayload, replayedPayload) {
		t.Fatalf("acquire replay first=%#v replayed=%#v", first, replayed)
	}

	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	beforeWrongToken := snapshotRuntimeAuthorityFiles(t, store.Dir)
	wrong, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, cliAttemptHash('f'), "wrong-token", "passed"))
	if wrong.State != "blocked" || wrong.Reason != "active_attempt" || wrong.Token != first.Token {
		t.Fatalf("wrong-token settle = %#v", wrong)
	}
	if after := snapshotRuntimeAuthorityFiles(t, store.Dir); !reflect.DeepEqual(beforeWrongToken, after) {
		t.Fatal("wrong-token settle mutated authority")
	}

	settleArgs := compactSettleArgs(repo, change, first.Token, "replay-settle", "passed")
	completed, completedPayload := runCompactSDDAttempt(t, settleArgs)
	completedReplay, completedReplayPayload := runCompactSDDAttempt(t, settleArgs)
	if completed != (compactAttemptOutput{State: "complete"}) || completedReplay != completed || !bytes.Equal(completedPayload, completedReplayPayload) {
		t.Fatalf("settle replay completed=%#v replayed=%#v", completed, completedReplay)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Attempts) != 1 || status.ActiveAttempt != nil || !status.Complete {
		t.Fatalf("replayed compact lifecycle status = %#v", status)
	}
}

// TestRunSDDAttemptAcquireTokenBreaksParentActorDeadlock reproduces #2291's
// exact CLI-level deadlock: a parent process runs `sdd-attempt acquire` and
// gets back proceed + a token, then launches an actor as a distinct process
// (its own --request-id). Presenting the parent's token via the new --token
// flag must let the actor proceed under the SAME attempt with zero authority
// mutation, instead of colliding with active_attempt.
func TestRunSDDAttemptAcquireTokenBreaksParentActorDeadlock(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "deadlock-2291"
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}

	parent, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "deadlock-parent", 2))
	if parent.State != "proceed" || parent.Token == "" {
		t.Fatalf("parent acquire = %#v", parent)
	}

	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	actor, actorPayload := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "deadlock-actor",
		"--work-unit", "compact-unit", "--evidence-goal", "prove compact attempt",
		"--max-attempts", "2", "--max-changed-lines", "20", "--token", parent.Token,
	})
	after := snapshotRuntimeAuthorityFiles(t, store.Dir)

	if actor.State != "proceed" || actor.Token != parent.Token || actor.Reason != "" {
		t.Fatalf("actor acquire-with-token = %#v, want proceed with parent token %q", actor, parent.Token)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("actor acquire-with-token mutated authority\nbefore=%v\nafter=%v", before, after)
	}
	assertCompactPayloadKeys(t, actorPayload, "state", "token")
}

// TestRunSDDAttemptAcquireForeignTokenStaysBlockedWithNamedExit covers the
// converse: a --token that does not match the live active attempt must not
// grant ownership. It stays blocked with the REAL active token (not the
// foreign one supplied) and a named Exit/Detail explaining how to proceed,
// with zero authority mutation for the losing check.
func TestRunSDDAttemptAcquireForeignTokenStaysBlockedWithNamedExit(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "deadlock-2291-foreign"
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}

	active, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "foreign-owner", 2))
	if active.State != "proceed" || active.Token == "" {
		t.Fatalf("owner acquire = %#v", active)
	}

	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	blocked, blockedPayload := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "foreign-contender",
		"--work-unit", "compact-unit", "--evidence-goal", "prove compact attempt",
		"--max-attempts", "2", "--max-changed-lines", "20", "--token", cliAttemptHash('f'),
	})
	after := snapshotRuntimeAuthorityFiles(t, store.Dir)

	if blocked.State != "blocked" || blocked.Reason != "active_attempt" || blocked.Token != active.Token {
		t.Fatalf("foreign-token acquire = %#v, want blocked active_attempt with owner token %q", blocked, active.Token)
	}
	if blocked.Exit == "" || blocked.Detail == "" {
		t.Fatalf("foreign-token acquire missing named exit: %#v", blocked)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("foreign-token acquire mutated authority\nbefore=%v\nafter=%v", before, after)
	}
	assertCompactPayloadKeys(t, blockedPayload, "state", "reason", "token", "exit", "detail")
}

func TestRunSDDAttemptAcquireProjectedItemDerivesAndSettlesBoundScope(t *testing.T) {
	repo, change := projectedItemCLIChange(t)
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	result, _ := runCompactSDDAttempt(t, []string{"acquire", "--cwd", repo, "--change", change, "--item", "build", "--request-id", "item-acquire"})
	if result.State != "proceed" || result.Token == "" {
		t.Fatalf("item acquire = %#v", result)
	}
	status, err := store.Status()
	if err != nil || status.Objective == nil || status.Objective.ItemID != "build" || status.Objective.WorkUnit != "build" || status.Objective.EvidenceGoal != "compile" ||
		status.Objective.MaxAttempts != 1 || status.Objective.MaxChangedLines != 100 || !reflect.DeepEqual(status.Objective.ItemEditRoots, []string{filepath.Join(repo, "src", "future")}) {
		t.Fatalf("bound runtime status = %#v, %v", status, err)
	}
	if reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
		t.Fatal("item acquire did not create its runtime attempt")
	}
	continued, _ := runCompactSDDAttempt(t, []string{"acquire", "--cwd", repo, "--change", change, "--item", "build", "--request-id", "item-actor", "--token", result.Token})
	if continued.State != "proceed" || continued.Token != result.Token {
		t.Fatalf("item continuation = %#v", continued)
	}
	replayed, _ := runCompactSDDAttempt(t, []string{"acquire", "--cwd", repo, "--change", change, "--item", "build", "--request-id", "item-acquire"})
	if replayed != result {
		t.Fatalf("item replay = %#v, want %#v", replayed, result)
	}
	tasksPath := filepath.Join(repo, "openspec", "changes", change, "tasks.md")
	tasks, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasksPath, []byte(strings.Replace(string(tasks), `"maxChangedLines":100`, `"maxChangedLines":101`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunSDDAttempt([]string{"acquire", "--cwd", repo, "--change", change, "--item", "build", "--request-id", "item-acquire"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("metadata drift replay error = %v", err)
	}
	writeProjectedItemTasks(t, repo, change, false)
	settleArgs := compactSettleArgs(repo, change, result.Token, "item-settle", "passed")
	settled, settlePayload := runCompactSDDAttempt(t, settleArgs)
	if settled.State != "proceed" || settled.ItemSettlement == nil || settled.ItemSettlement.ItemID != "build" ||
		settled.ItemSettlement.WorkUnit != "build" || settled.ItemSettlement.ObjectiveID != status.Objective.ID ||
		settled.ItemSettlement.ObjectiveGeneration != status.Objective.Generation || settled.ItemSettlement.AttemptOrdinal != 1 ||
		settled.ItemSettlement.EvidenceRevision != cliAttemptHash('e') || settled.ItemSettlement.SettlementRequestID != "item-settle" {
		t.Fatalf("item settle = %#v", settled)
	}
	assertCompactPayloadKeys(t, settlePayload, "state", "item_settlement")
	replayed, replayPayload := runCompactSDDAttempt(t, settleArgs)
	if !reflect.DeepEqual(replayed, settled) || !bytes.Equal(replayPayload, settlePayload) {
		t.Fatalf("item settle replay = %#v %s, want %#v %s", replayed, replayPayload, settled, settlePayload)
	}
	projected, err := sddstatus.Resolve(sddstatus.ResolveOptions{CWD: repo, ChangeName: change, ReviewDisabled: true})
	if err != nil || !projected.Items[0].Blocked || projected.Items[0].Ready || projected.Items[0].Done || projected.Items[0].EvidenceRevision == "" {
		t.Fatalf("unchecked settled item = %#v, %v", projected.Items, err)
	}
	writeProjectedItemTasks(t, repo, change, true)
	projected, err = sddstatus.Resolve(sddstatus.ResolveOptions{CWD: repo, ChangeName: change, ReviewDisabled: true})
	if err != nil || !projected.Items[0].Done || projected.Items[0].Active || projected.Items[0].EvidenceRevision != "" {
		t.Fatalf("checked item = %#v, %v", projected.Items, err)
	}
}

func TestRunSDDAttemptConcurrentDisjointItemsSettleByToken(t *testing.T) {
	repo, change := concurrentItemCLIChange(t)
	a, _ := runCompactSDDAttempt(t, []string{"acquire", "--cwd", repo, "--change", change, "--item", "a", "--request-id", "concurrent-a"})
	b, _ := runCompactSDDAttempt(t, []string{"acquire", "--cwd", repo, "--change", change, "--item", "b", "--request-id", "concurrent-b"})
	if a.State != "proceed" || b.State != "proceed" || a.Token == "" || b.Token == "" || a.Token == b.Token {
		t.Fatalf("disjoint acquires a=%#v b=%#v", a, b)
	}

	var status sddstatus.RuntimeStatus
	var output bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", change}, &output); err != nil || json.Unmarshal(output.Bytes(), &status) != nil ||
		status.ActiveAttempt == nil || status.ActiveAttempt.Ordinal != 1 || status.Objective == nil || status.Objective.Generation != 1 || len(status.Attempts) != 2 {
		t.Fatalf("concurrent status=%#v err=%v output=%s", status, err, output.String())
	}
	if status.Attempts[0].Ordinal != 1 || status.Attempts[1].Ordinal != 2 || status.Attempts[0].ObjectiveGeneration != 1 || status.Attempts[1].ObjectiveGeneration != 2 {
		t.Fatalf("concurrent attempts=%#v", status.Attempts)
	}

	settleA := compactSettleArgs(repo, change, a.Token, "concurrent-a-settle", "passed")
	first, firstPayload := runCompactSDDAttempt(t, settleA)
	if first.State != "proceed" || first.ItemSettlement == nil || first.ItemSettlement.ItemID != "a" || first.ItemSettlement.AttemptOrdinal != 1 || first.ItemSettlement.ObjectiveGeneration != 1 {
		t.Fatalf("non-projected settlement=%#v", first)
	}
	output.Reset()
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", change}, &output); err != nil || json.Unmarshal(output.Bytes(), &status) != nil || status.ActiveAttempt == nil || status.ActiveAttempt.Ordinal != 2 || status.Attempts[0].Outcome != sddstatus.AttemptPassed || status.Attempts[1].Outcome != sddstatus.AttemptRunning || status.Complete {
		t.Fatalf("remaining owner=%#v err=%v", status, err)
	}

	second, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, b.Token, "concurrent-b-settle", "passed"))
	if second.State != "proceed" || second.ItemSettlement == nil || second.ItemSettlement.ItemID != "b" || second.ItemSettlement.AttemptOrdinal != 2 {
		t.Fatalf("remaining settlement=%#v", second)
	}
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	beforeReplay := snapshotRuntimeAuthorityFiles(t, store.Dir)
	replayed, replayPayload := runCompactSDDAttempt(t, settleA)
	afterReplay := snapshotRuntimeAuthorityFiles(t, store.Dir)
	if !reflect.DeepEqual(replayed.ItemSettlement, first.ItemSettlement) || !reflect.DeepEqual(beforeReplay, afterReplay) {
		t.Fatalf("first settlement replay=%#v payload=%s first=%s\nbefore=%v\nafter=%v", replayed, replayPayload, firstPayload, beforeReplay, afterReplay)
	}
	for _, token := range []string{"", cliAttemptHash('f')} {
		before := snapshotRuntimeAuthorityFiles(t, store.Dir)
		var foreignOutput bytes.Buffer
		err := RunSDDAttempt(compactSettleArgs(repo, change, token, "foreign-"+fmt.Sprint(len(token)), "passed"), &foreignOutput)
		var blocked compactAttemptOutput
		_ = json.Unmarshal(foreignOutput.Bytes(), &blocked)
		if (err == nil && blocked.State != "blocked") || !reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
			t.Fatalf("foreign token=%q err=%v output=%s", token, err, foreignOutput.String())
		}
	}
}

func TestRunSDDAttemptAcquireProjectedItemRefusesCallerScopeAndUnavailableItems(t *testing.T) {
	repo, change := projectedItemCLIChange(t)
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"acquire", "--cwd", repo, "--change", change, "--item", "build", "--request-id", "override", "--work-unit", "forged"},
		{"acquire", "--cwd", repo, "--change", change, "--item", "missing", "--request-id", "missing"},
		{"acquire", "--cwd", repo, "--change", change, "--item", "verify", "--request-id", "blocked"},
	} {
		before := snapshotRuntimeAuthorityFiles(t, store.Dir)
		var output bytes.Buffer
		if err := RunSDDAttempt(args, &output); err == nil || !strings.Contains(err.Error(), "item-selected acquire") {
			t.Fatalf("RunSDDAttempt(%v) error = %v", args, err)
		}
		if after := snapshotRuntimeAuthorityFiles(t, store.Dir); !reflect.DeepEqual(before, after) {
			t.Fatalf("refused item acquire mutated authority\nbefore=%v\nafter=%v", before, after)
		}
	}
	legacy := []string{"acquire", "--cwd", repo, "--change", change, "--request-id", "legacy"}
	var output bytes.Buffer
	if err := RunSDDAttempt(legacy, &output); err == nil || !strings.Contains(err.Error(), "--work-unit") || !strings.Contains(err.Error(), "--evidence-goal") {
		t.Fatalf("legacy acquire required flags changed: %v", err)
	}
}

func TestRunSDDAttemptAcquireProjectedItemRefusesUnboundActiveContinuation(t *testing.T) {
	repo, change := projectedItemCLIChange(t)
	legacy, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "legacy-active",
		"--work-unit", "build", "--evidence-goal", "compile", "--max-attempts", "1", "--max-changed-lines", "100",
	})
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Status()
	if err != nil || before.Objective == nil || before.ActiveAttempt == nil || before.Objective.ItemID != "" || before.ActiveAttempt.ItemID != "" {
		t.Fatalf("legacy active status = %#v, %v", before, err)
	}
	files := snapshotRuntimeAuthorityFiles(t, store.Dir)
	var output bytes.Buffer
	err = RunSDDAttempt([]string{"acquire", "--cwd", repo, "--change", change, "--item", "build", "--request-id", "item-continuation", "--token", legacy.Token}, &output)
	if err == nil || !strings.Contains(err.Error(), "lacks the selected immutable binding") {
		t.Fatalf("unbound item continuation error = %v", err)
	}
	after, err := store.Status()
	if err != nil || !reflect.DeepEqual(after, before) || !reflect.DeepEqual(snapshotRuntimeAuthorityFiles(t, store.Dir), files) {
		t.Fatalf("unbound continuation mutated runtime\nbefore=%#v\nafter=%#v\nerr=%v", before, after, err)
	}
}

func projectedItemCLIChange(t *testing.T) (string, string) {
	t.Helper()
	repo, change := initReviewCLIRepo(t), "projected-item"
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{"proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/item/spec.md": "### Requirement: Item\n#### Scenario: Acquire\n"} {
		writeReviewStartCandidate(t, repo, filepath.Join("openspec", "changes", change, path), content, 0o644)
	}
	writeProjectedItemTasks(t, repo, change, false)
	return repo, change
}

func concurrentItemCLIChange(t *testing.T) (string, string) {
	t.Helper()
	repo, change := initReviewCLIRepo(t), "concurrent-items"
	for _, root := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(repo, root), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{"proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/item/spec.md": "### Requirement: Item\n#### Scenario: Acquire\n"} {
		writeReviewStartCandidate(t, repo, filepath.Join("openspec", "changes", change, path), content, 0o644)
	}
	tasks := "- [ ] a: A\n- [ ] b: B\n<!-- gentle-ai.sdd-items/v1\n{\"items\":[{\"id\":\"a\",\"dependsOn\":[],\"workUnit\":\"a\",\"editRoots\":[\"a\"],\"maxAttempts\":1,\"maxChangedLines\":100,\"evidenceGoal\":\"prove a\"},{\"id\":\"b\",\"dependsOn\":[],\"workUnit\":\"b\",\"editRoots\":[\"b\"],\"maxAttempts\":1,\"maxChangedLines\":100,\"evidenceGoal\":\"prove b\"}]}\n-->"
	writeReviewStartCandidate(t, repo, filepath.Join("openspec", "changes", change, "tasks.md"), tasks, 0o644)
	return repo, change
}

func writeProjectedItemTasks(t *testing.T, repo, change string, done bool) {
	t.Helper()
	mark := " "
	if done {
		mark = "x"
	}
	tasks := fmt.Sprintf("- [%s] build: Build\n- [ ] verify: Verify\n<!-- gentle-ai.sdd-items/v1\n{\"items\":[{\"id\":\"build\",\"dependsOn\":[],\"workUnit\":\"build\",\"editRoots\":[\"src/future\"],\"maxAttempts\":1,\"maxChangedLines\":100,\"evidenceGoal\":\"compile\"},{\"id\":\"verify\",\"dependsOn\":[\"build\"],\"workUnit\":\"verify\",\"editRoots\":[\"src\"],\"maxAttempts\":1,\"maxChangedLines\":50,\"evidenceGoal\":\"test\"}]}\n-->", mark)
	writeReviewStartCandidate(t, repo, filepath.Join("openspec", "changes", change, "tasks.md"), tasks, 0o644)
}

func compactAcquireArgs(repo, change, requestID string, maxAttempts int) []string {
	return []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", requestID,
		"--work-unit", "compact-unit", "--evidence-goal", "prove compact attempt",
		"--max-attempts", fmt.Sprint(maxAttempts), "--max-changed-lines", "20",
	}
}

func compactSettleArgs(repo, change, token, requestID, outcome string) []string {
	return []string{
		"settle", "--cwd", repo, "--change", change, "--token", token, "--request-id", requestID,
		"--outcome", outcome, "--evidence-revision", cliAttemptHash('e'),
		"--diagnosis", "compact attempt produced conclusive evidence", "--harness-disposition", "reused",
		"--cleanup-evidence", "process group exited", "--process-evidence", "no descendants remained",
	}
}

func runCompactSDDAttempt(t *testing.T, args []string) (compactAttemptOutput, []byte) {
	t.Helper()
	var output bytes.Buffer
	if err := RunSDDAttempt(args, &output); err != nil {
		t.Fatalf("RunSDDAttempt(%v): %v", args, err)
	}
	var result compactAttemptOutput
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode compact SDD attempt: %v\n%s", err, output.String())
	}
	return result, append([]byte(nil), output.Bytes()...)
}

func assertCompactPayloadKeys(t *testing.T, payload []byte, keys ...string) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if len(document) != len(keys) {
		t.Fatalf("compact keys = %v, want %v", document, keys)
	}
	for _, key := range keys {
		if _, ok := document[key]; !ok {
			t.Fatalf("compact output missing %q: %s", key, payload)
		}
	}
}

func snapshotRuntimeAuthorityFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == "LOCK" {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[relative] = string(payload)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return snapshot
}
