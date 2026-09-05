package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// The maintainer's rule for this file: while the kill switch is off,
// receipt-driven development does not exist, so it has no implications. The gate
// never blocks, never vetoes, and never gates; ordinary repository policy —
// hooks, tests, CI — decides. Turning reviews back on re-validates from the
// current state instead of resuming a stale obligation.
//
// Nothing here weakens the two invariants that make that safe: a disabled
// switch never fabricates approval (`allowed` stays false and no receipt, PASS,
// or authority is invented), and an unreadable *switch* still fails closed to
// the managed disposition, so a damaged mode record can never manufacture the
// disabled disposition.

// enableReviewForClone clears this clone's off-only override, turning
// receipt-driven development back on for the next candidate.
func enableReviewForClone(t *testing.T, repo string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("re-enable receipt-driven development: %v\n%s", err, output.String())
	}
	if status := decodeReviewModeResult(t, output.Bytes()).Status; status.Effective != reviewtransaction.RDDModeOn {
		t.Fatalf("kill switch did not come back on: %#v", status)
	}
}

// corruptReviewAuthorityInventory writes a truncated compact record, which is
// genuine damage to the review authority store rather than a stale receipt.
func corruptReviewAuthorityInventory(t *testing.T, repo string) {
	t.Helper()
	broken := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "corrupt-reach")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertDisabledUnmanagedGate is the single shape EVERY reclassified discovery
// outcome must produce while disabled (Wave 5 Slice 2, design decision 4):
// exit 0, no denial error, no approval, no invented receipt, the fixed
// generic disabled reason (reviewDisabledUnmanagedReason, byte-identical
// regardless of what authority-store damage or ambiguity exists — discovery
// never runs while disabled, so there is nothing scenario-specific to carry),
// and no Denial/discovery-kind detail at all. The discovery-kind-specific
// visibility this helper used to assert (a `wantCode` parameter) moved to
// each scenario's "...WhileEnabled" sibling test, where discovery genuinely
// still runs and the property is still true.
func assertDisabledUnmanagedGate(t *testing.T, runErr error, payload []byte) ReviewValidateResult {
	t.Helper()
	if runErr != nil {
		t.Fatalf("disabled gate vetoed delivery instead of reporting it: %v\n%s", runErr, string(payload))
	}
	var denied ReviewGateDeniedError
	if errors.As(runErr, &denied) {
		t.Fatalf("disabled gate reported a denial: %#v", denied)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, payload, &result)
	if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled gate delivery = %q, want %q\n%s", result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged, string(payload))
	}
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("disabled gate fabricated an approval: %#v", result)
	}
	if result.Context.Denial != nil {
		t.Fatalf("disabled gate leaked discovery-kind detail: %#v (design decision 4: no discovery kind while disabled)", result.Context.Denial)
	}
	if result.Reason != reviewDisabledUnmanagedReason {
		t.Fatalf("disabled gate reason = %q, want the fixed generic sentence %q", result.Reason, reviewDisabledUnmanagedReason)
	}
	return result
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryAtPostApply preserves the
// direct non-deciding gate coverage while reviews are disabled.
func TestReviewFacadeValidateReportsDisabledUnmanagedDeliveryAtPostApply(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	assertDisabledUnmanagedGate(t, runErr, output.Bytes())

	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &replay); err != nil {
		t.Fatalf("replayed disabled gate: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled gate is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthority is
// the deliberate decision on damaged authority. Corrupted review authority is
// damage to a system the operator switched off; blocking an ordinary commit on
// it is exactly the implication the rule removes. So the gate defers.
//
// Wave 5 Slice 2 supersession (design decision 4): the switch is now
// consulted BEFORE any authority read, so this disabled report no longer even
// discovers the corruption — the visibility assertion belongs to enabled-mode
// non-deciding gate coverage. Re-enabling still rediscovers
// the same damage and blocks, so nothing is forgiven, only deferred; while
// disabled, the damage simply never gets read in the first place.
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthority(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")

	disableReviewForClone(t, repo)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("work authored while disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "authored while disabled")
	corruptReviewAuthorityInventory(t, repo)

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	assertDisabledUnmanagedGate(t, runErr, output.Bytes())
}

// TestReviewValidateNegotiatedContractReportsDisabledUnmanagedDelivery covers
// the negotiated non-deciding gate while reviews are disabled.
func TestReviewValidateNegotiatedContractReportsDisabledUnmanagedDelivery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	runErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if runErr != nil {
		t.Fatalf("negotiated disabled gate vetoed delivery instead of reporting it: %v\n%s", runErr, output.String())
	}
	envelope := decodeReviewOperationEnvelope(t, output.Bytes())
	if err := envelope.Validate(); err != nil {
		t.Fatalf("negotiated disabled gate envelope: %v\n%s", err, output.String())
	}
	if envelope.Operation != ReviewIntegrationOperationValidate {
		t.Fatalf("negotiated disabled gate operation = %q", envelope.Operation)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, envelope.Result, &result)
	if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("negotiated disabled gate delivery = %q, want %q\n%s", result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged, output.String())
	}
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("negotiated disabled gate fabricated an approval: %#v", result)
	}
}

// TestReviewValidateNegotiatedContractUsesUnmanagedDeliveryWhileEnabled proves
// that the shipped negotiated route reports ordinary repository policy without
// inspecting authority.
func TestReviewValidateNegotiatedContractUsesUnmanagedDeliveryWhileEnabled(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output); err != nil {
		t.Fatalf("negotiated enabled gate: %v\n%s", err, output.String())
	}
	envelope := decodeReviewOperationEnvelope(t, output.Bytes())
	if err := envelope.Validate(); err != nil {
		t.Fatalf("negotiated enabled gate envelope: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, envelope.Result, &result)
	if result.Result != reviewtransaction.GateInvalidated || result.Allowed || result.Action != reviewDeliveryPolicyAction ||
		result.Delivery != reviewtransaction.RDDDeliveryUnmanaged || result.Context.Gate != reviewtransaction.GatePostApply ||
		result.Context.Denial != nil || result.Context.LineageID != "" {
		t.Fatalf("negotiated enabled gate result = %#v", result)
	}
}

// TestReviewValidateReValidatesFromScratchAfterReEnabling keeps delivery
// non-deciding across a disabled window. Re-enabling never discovers a
// predecessor: a new START creates or exactly replays its own worktree-bound
// compact authority over the current candidate.
func TestReviewValidateReValidatesFromScratchAfterReEnabling(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "reviewed candidate behavior\n", 0o644)
	predecessor := runNegotiatedReviewStart(t, repo, "review-before-disabling")
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessorRecord, err := predecessorStore.Load()
	if err != nil || predecessorRecord.State.InitialAtomicStart == nil {
		t.Fatalf("pre-disable compact authority = %#v, %v", predecessorRecord, err)
	}
	beforePredecessor, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	disableReviewForClone(t, repo)
	writeReviewStartCandidate(t, repo, "tracked.txt", "changed while reviews were off\n", 0o644)

	beforeDisabledGate := snapshotShippedReviewStore(t, repo)
	disabled := runShippedNonDecidingReviewValidate(t, repo, reviewtransaction.GatePreCommit)
	afterDisabledGate := snapshotShippedReviewStore(t, repo)
	assertShippedReviewStoreUnchanged(t, beforeDisabledGate, afterDisabledGate)
	assertDisabledUnmanagedGate(t, nil, disabled)

	enableReviewForClone(t, repo)
	beforeEnabledGate := snapshotShippedReviewStore(t, repo)
	enabled := runShippedNonDecidingReviewValidate(t, repo, reviewtransaction.GatePreCommit)
	afterEnabledGate := snapshotShippedReviewStore(t, repo)
	assertShippedReviewStoreUnchanged(t, beforeEnabledGate, afterEnabledGate)
	assertEnabledUnmanagedGatePayload(t, enabled, reviewtransaction.GatePreCommit)

	created := atomicStartV2(t, repo, "")
	if created.Action != "created" || created.LineageID == predecessor.LineageID ||
		created.State != reviewtransaction.StateReviewing {
		t.Fatalf("re-enabled new START = %#v, predecessor = %#v", created, predecessor)
	}
	createdStore, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, created.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	createdRecord, err := createdStore.Load()
	if err != nil || createdRecord.State.InitialAtomicStart == nil || createdRecord.State.Recovery != nil ||
		createdRecord.State.InitialSnapshot.Identity == predecessorRecord.State.InitialSnapshot.Identity ||
		createdRecord.State.InitialAtomicStart.WorktreeIdentity != predecessorRecord.State.InitialAtomicStart.WorktreeIdentity {
		t.Fatalf("re-enabled compact authority = %#v, %v", createdRecord, err)
	}
	if after, err := os.ReadFile(predecessorStore.StatePath()); err != nil || !bytes.Equal(beforePredecessor, after) {
		t.Fatalf("new START reconstructed or mutated predecessor authority: %v", err)
	}

	beforeReplay, err := os.ReadFile(createdStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	replayed := atomicStartV2(t, repo, "")
	afterReplay, err := os.ReadFile(createdStore.StatePath())
	if err != nil || replayed.Action != "replayed" || replayed.LineageID != created.LineageID || !bytes.Equal(beforeReplay, afterReplay) {
		t.Fatalf("re-enabled exact START replay = %#v, state changed=%t, err=%v", replayed, !bytes.Equal(beforeReplay, afterReplay), err)
	}
}

// recoverFacadeReview drives the explicit maintainer recovery that re-freezes a
// superseded lineage over the current candidate.
func recoverFacadeReview(t *testing.T, repo, predecessor, successorLineage string) ReviewRecoverResult {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, err := builder.DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target, err := builder.Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor +
		"\npredecessor_revision=" + record.Revision + "\ntarget_identity=" + target.Identity +
		"\nactor=maintainer\nreason=candidate changed while reviews were off"
	var output bytes.Buffer
	if err := RunReview([]string{
		"recover", "--cwd", repo, "--predecessor-lineage", predecessor,
		"--expected-predecessor-revision", record.Revision, "--successor-lineage", successorLineage,
		"--disposition", "scope_changed", "--reason", "candidate changed while reviews were off",
		"--actor", "maintainer", "--maintainer-authorization", authorization,
	}, &output); err != nil {
		t.Fatalf("recover review after re-enabling: %v\n%s", err, output.String())
	}
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, output.Bytes(), &recovered)
	return recovered
}

// startFacadeReviewResult runs one START over the live candidate and returns
// the typed result.
func startFacadeReviewResult(t *testing.T, repo, lineage string) ReviewFacadeStartResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &output); err != nil {
		t.Fatalf("start facade review %q: %v\n%s", lineage, err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	return started
}

// liveFacadeSnapshotIdentity computes the current workspace candidate's
// snapshot identity without persisting review authority. Fixtures use it only
// as a comparison value when proving a successor has a distinct target.
func liveFacadeSnapshotIdentity(t *testing.T, repo string) string {
	t.Helper()
	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("resolve live snapshot repository root: %v", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("build live snapshot identity: %v", err)
	}
	return snapshot.Identity
}

// TestDisabledGateNeverEmitsAllowOrCreatesReceipt sweeps every reclassified
// discovery outcome and holds the non-negotiable boundary: `disabled/unmanaged`
// exits 0 because it defers, never because it approved.
//
// Wave 5 Slice 2 (design decision 4): also the decoy-store zero-reads proof.
// These three cases stage wildly different, even damaged, authority-store
// content -- an ambiguous multi-receipt composition, a genuinely corrupted
// (truncated) compact record, and no receipt at all -- behind the SAME gate.
// Under the pre-Slice-2 contract these produced three DIFFERENT disabled
// reasons (each naming its own discovery kind). Under the new contract the
// switch is consulted before any of that content is ever read, so all three
// must produce BYTE-IDENTICAL disabled output: the store's content
// contributes nothing to what is reported, which is the portable,
// CI-safe equivalent of Wave 4's own strace-based "zero authority-store
// opens while disabled" verifier proof.
func TestDisabledGateNeverEmitsAllowOrCreatesReceipt(t *testing.T) {
	cases := []struct {
		name  string
		stage func(t *testing.T, repo string)
	}{
		{
			name: "active atomic authority",
			stage: func(t *testing.T, repo string) {
				writeReviewStartCandidate(t, repo, "docs/sweep.md", "reviewed\n", 0o644)
				runNegotiatedReviewStart(t, repo, "review-disabled-sweep-active")
			},
		},
		{
			name: "corrupted authority",
			stage: func(t *testing.T, repo string) {
				corruptReviewAuthorityInventory(t, repo)
			},
		},
		{
			name:  "no receipt at all",
			stage: func(t *testing.T, repo string) {},
		},
	}
	outputs := make(map[string][]byte, len(cases))
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			testCase.stage(t, repo)
			disableReviewForClone(t, repo)
			before := reviewAuthorityFingerprint(t, repo)

			var output bytes.Buffer
			runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
			assertDisabledUnmanagedGate(t, runErr, output.Bytes())
			if bytes.Contains(output.Bytes(), []byte(`"allow"`)) {
				t.Fatalf("disabled gate emitted an allow: %s", output.String())
			}
			if after := reviewAuthorityFingerprint(t, repo); after != before {
				t.Fatalf("disabled gate mutated review authority:\nbefore: %s\nafter:  %s", before, after)
			}
			outputs[testCase.name] = append([]byte(nil), output.Bytes()...)
		})
	}
	first := outputs[cases[0].name]
	for _, testCase := range cases[1:] {
		if !bytes.Equal(outputs[testCase.name], first) {
			t.Fatalf("decoy-store zero-reads proof failed: disabled output differs by store content\n%s:\n%s\n%s:\n%s",
				cases[0].name, first, testCase.name, outputs[testCase.name])
		}
	}
}

// reviewAuthorityFingerprint lists every file under this clone's review
// authority root so a test can prove a disabled run created no receipt and
// mutated no state.
func reviewAuthorityFingerprint(t *testing.T, repo string) string {
	t.Helper()
	root := filepath.Join(repo, ".git", "gentle-ai", "review-transactions")
	var entries []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		entries = append(entries, path+":"+facadePayloadHash(payload))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Join(entries, "\n")
}

// reflectDeepEqualStrings keeps this file free of a reflect import for the one
// slice comparison it needs.
func reflectDeepEqualStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// TestSDDAttemptFinishHonorsTheKillSwitchOverRemediationObligations is the
// end-to-end proof that the switch actually REACHES the SDD runtime ledger. The
// unit test in internal/sddstatus proves the ledger obeys the flag; this proves
// the CLI resolves the switch and sets it, which is the part that was missing
// entirely — internal/sddstatus never consulted the kill switch in any form.
//
// The shape is the reporter's: a clone holds a review binding, work continues,
// the attempt changes the candidate tree, and the passing attempt is closed
// without remediation flags. With reviews ON that still demands an approved
// recovery successor. With reviews OFF it closes, because the successor could
// only come from `review start`, which the same switch refuses.
func TestSDDAttemptFinishImposesNoRemediationObligationEitherWay(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		disable  bool
		wantFail bool
	}{
		{name: "reviews enabled impose no obligation", disable: false, wantFail: false},
		{name: "reviews disabled impose no obligation", disable: true, wantFail: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			change := "cli-kill-switch-remediation"
			changeRoot := filepath.Join(repo, "openspec", "changes", change)
			writeCLIAttemptFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
			writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n")
			runReviewCLIGit(t, repo, "add", ".")
			runReviewCLIGit(t, repo, "commit", "-qm", "seed change")

			bound := runSDDAttemptStatus(t, []string{
				"begin", "--cwd", repo, "--change", change, "--expected-revision=", "--request-id", "switch-begin-1",
				"--work-unit", "cli-kill-switch", "--evidence-goal", "close a bound attempt",
				"--max-attempts", "3", "--max-changed-lines", "40",
			})

			// Work that changes the candidate tree during the attempt: this is
			// exactly what arms the implicit successor demand.
			writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# more work\n")

			if testCase.disable {
				disableReviewForClone(t, repo)
			}

			var output bytes.Buffer
			err := RunSDDAttempt([]string{
				"finish", "--cwd", repo, "--change", change, "--expected-revision", bound.Revision,
				"--request-id", "switch-finish-1", "--outcome", "passed", "--evidence-revision", cliAttemptHash('c'),
				"--diagnosis", "attempt passed", "--harness-disposition", "reused",
				"--cleanup-evidence", "cleanup completed", "--process-evidence", "process scan found no descendants",
			}, &output)
			if err != nil {
				t.Fatalf("bound finish demanded a review obligation: %T %v\n%s", err, err, output.String())
			}
			var status sddstatus.RuntimeStatus
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.ActiveAttempt != nil {
				t.Fatalf("disabled bound finish left the attempt open: %#v", status.ActiveAttempt)
			}
		})
	}
}
