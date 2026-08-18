package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNewLineageRollbackSafetyStaysReadableAndFinalizableWhileSwitchIsOff is
// task 5.11: an in-flight new-lineage authority created while the activation
// switch was ON must remain fully readable, still govern its candidate at
// validate, and remain finalizable to a terminal receipt after the switch is
// turned back off (design decision 5, "Rollback Disables New Starts Only";
// spec rdd-new-lineage-activation, "Rollback Disables New Starts Only").
//
// resolveGoverningAuthority and ReviewCore never read the activation switch
// at all — only the CLI's start branch (review_facade_new_lineage.go) does —
// so this proves that invariant end-to-end rather than by code inspection
// alone.
func TestNewLineageRollbackSafetyStaysReadableAndFinalizableWhileSwitchIsOff(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineageID = "rollback-safety-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("rollback-safety candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Task 6.7 widened governingAuthorityLiveEvidence to read the STAGED
	// projection specifically at pre-commit (matching legacy's own
	// buildCompactGateRequestWithPushBase convention), so constructing a
	// live candidate identity for this gate now requires the modification
	// to actually be staged first — an unstaged-only change would resolve
	// to no changed paths at all under GatePreCommit.
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")
	live, _, err := governingAuthorityLiveEvidence(context.Background(), repo, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateReviewing
		next.CandidateIdentity = live
		next.Tier = reviewtransaction.RiskLow
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Rollback: the activation switch goes back off. No new start would use
	// v3, but this lineage already exists.
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "")

	if _, err := store.Load(); err != nil {
		t.Fatalf("in-flight new lineage unreadable after rollback: %v", err)
	}

	// Still readable and still the GOVERNING authority at validate time —
	// resolveGoverningAuthority/runReviewFacadeValidate never consult the
	// activation switch. C5 (default-deny at gates) means an in-flight,
	// still-`reviewing` (never approved, no receipt) lineage now correctly
	// DENIES here rather than allows — governing (never falling through to
	// a fabricated legacy authority) is the property this step actually
	// pins, not a bare allow. The full loop's positive control lives below,
	// once the lineage is genuinely finalized.
	var output bytes.Buffer
	err = RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineageID, "--gate", "pre-commit"}, &output)
	if err == nil {
		t.Fatalf("in-flight (reviewing) new lineage must deny at validate after rollback (default-deny), got allow:\n%s", output.String())
	}
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) || denied.Result == reviewtransaction.GateAllow {
		t.Fatalf("in-flight new lineage validate error after rollback = %v, want a ReviewGateDeniedError that never allows", err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2")); !os.IsNotExist(statErr) {
		t.Fatalf("rollback validate fabricated a legacy authority instead of using the in-flight v3 one: %v", statErr)
	}

	// Still finalizable: drive it to a terminal state and issue the
	// terminal receipt, with the switch still off.
	if _, err = store.Mutate(context.Background(), revision, func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateApproved
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	transition, err := (reviewtransaction.ReviewCore{}).Next(context.Background(), record.Authority, reviewtransaction.CoreRequest{Kind: reviewtransaction.CoreRequestFinalize})
	if err != nil {
		t.Fatalf("in-flight new lineage refused to finalize after rollback: %v", err)
	}
	if transition.Kind != reviewtransaction.CoreTransitionApprove || transition.Receipt == nil {
		t.Fatalf("finalize(approved) after rollback = %#v", transition)
	}
	if err := store.WriteReceipt(context.Background(), reviewtransaction.NewLineageReceipt{
		Schema: reviewtransaction.NewLineageReceiptSchema, LineageID: record.Authority.LineageID,
		TerminalState: record.Authority.State, AuthorityRevision: transition.Receipt.AuthorityRevision,
		CandidateIdentity: record.Authority.CandidateIdentity,
	}); err != nil {
		t.Fatalf("terminal receipt refused after rollback: %v", err)
	}
	if _, err := os.Stat(store.ReceiptPath()); err != nil {
		t.Fatalf("terminal receipt not published after rollback: %v", err)
	}

	// Positive control, closing the loop C5 opened above: once genuinely
	// approved AND receipted, the identical exact candidate now allows —
	// proving default-deny denies an in-flight lineage specifically because
	// it was never approved, not because v3 lineages can never allow.
	var afterFinalize bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineageID, "--gate", "pre-commit"}, &afterFinalize); err != nil {
		t.Fatalf("approved, receipted in-flight lineage must allow after rollback: %v\n%s", err, afterFinalize.String())
	}
	assertReviewGateResult(t, afterFinalize.Bytes(), reviewtransaction.GateAllow)
}
