package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestResolveGoverningAuthorityAbsentWithoutMarkerCostsNoGitCall proves the
// cheap common case at the CLI wiring layer: with no explicit --lineage
// marker, resolveGoverningAuthority returns "legacy governs unchanged"
// without resolving a repository root or issuing any Git command — the
// switch-off-equivalence goldens (tasks 5.6-5.10) depend on this being true
// for the ordinary hook-invoked gate call shape.
func TestResolveGoverningAuthorityAbsentWithoutMarkerCostsNoGitCall(t *testing.T) {
	governs, _, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), "/does/not/exist", "", reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if governs || discoveryErr != nil || evaluation != (reviewtransaction.NativeGateEvaluation{}) {
		t.Fatalf("resolveGoverningAuthority(no marker) = governs=%v evaluation=%#v discoveryErr=%v, want a pure no-op", governs, evaluation, discoveryErr)
	}
}

// TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy is
// task 5.2's MANDATORY RED test at the CLI integration layer (design
// decision 4's non-matrix discovery-integrity clause): a new-lineage marker
// (the v3/<lineage> directory) is present for the exact lineage id an
// otherwise-valid, otherwise-governing legacy receipt ALSO claims — but the
// v3 record itself was removed. The gate MUST deny and MUST NEVER fall
// through to the legacy receipt, even though that legacy receipt alone would
// have allowed this exact candidate.
func TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineageID = "shared-legacy-and-new-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate reviewed under legacy and named by a corrupted v3 marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a real, terminal, GOVERNING legacy receipt at this exact
	// lineage id: without the v3 marker below, this candidate would allow.
	finalizeApprovedFacadeReview(t, repo, lineageID)
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	var legacyOnly bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &legacyOnly); err != nil {
		t.Fatalf("legacy-only baseline before adding the v3 marker must allow: %v\n%s", err, legacyOnly.String())
	}
	assertReviewGateResult(t, legacyOnly.Bytes(), reviewtransaction.GateAllow)

	// Now plant the new-lineage marker under the IDENTICAL lineage id and
	// corrupt it: the directory (the marker) exists, but its record does
	// not — task 5.2's exact shape ("new-lineage marker present, v3 record
	// removed, legacy receipt present").
	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineageID)
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	if _, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateReviewing
		next.CandidateIdentity = reviewtransaction.CandidateIdentity{
			RepositoryID: "corrupted-marker-repo", BaseTree: head, CandidateTree: head,
			ChangedPathsModesDigest: "sha256:" + head, PolicyHash: "unknown",
		}
		next.Tier = reviewtransaction.RiskLow
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.StatePath()); err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(store.Dir); statErr != nil || !info.IsDir() {
		t.Fatalf("v3 marker directory must survive removing only its record: stat=%v err=%v", info, statErr)
	}

	var output bytes.Buffer
	err = RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	if err == nil {
		t.Fatalf("discovery-integrity corruption must deny, got allow:\n%s", output.String())
	}
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("discovery-integrity corruption error = %T %v, want ReviewGateDeniedError", err, err)
	}
	if denied.Result == reviewtransaction.GateAllow {
		t.Fatalf("discovery-integrity corruption fell through to the legacy allow: %#v", denied)
	}
	// Task 5.3's decision: the discovery-integrity denial reuses the
	// existing ReviewAuthorityCorrupted constant — this assertion is that
	// decision's regression guard, not a silent default.
	if denied.Context.Denial == nil || denied.Context.Denial.Code != string(ReviewAuthorityCorrupted) {
		t.Fatalf("discovery-integrity corruption denial code = %#v, want %q", denied.Context.Denial, ReviewAuthorityCorrupted)
	}

	// Replay stability: the same denial, byte-identical, and no mutation of
	// either authority system.
	var replay bytes.Buffer
	replayErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &replay)
	if replayErr == nil || !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("discovery-integrity corruption denial is not replay-stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
}

// TestResolveGoverningAuthorityInFlightDeniesEveryGate is CRITICAL C5's
// primary RED/GREEN evidence: the verify report's exact repro
// (`GENTLE_AI_RDD_NEW_LINEAGE=1 gentle-ai review start --lineage inflight`,
// nothing else — no reviewer, no finalize, no review-receipt.json) reached
// GateAllow at all five gates including release. A lineage that was started
// but never finalized is `reviewing`, never `approved`, and has no receipt —
// default-deny requires every one of the five gates to deny it.
func TestResolveGoverningAuthorityInFlightDeniesEveryGate(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "inflight-default-deny-lineage"
	startNewLineageForFinalizeTest(t, repo, lineage)

	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
	} {
		t.Run(string(gate), func(t *testing.T) {
			governs, _, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), repo, lineage, reviewtransaction.NativeGateRequestInput{Gate: gate})
			if !governs {
				t.Fatalf("gate %q: an in-flight (reviewing) v3 record must still govern (deny), got governs=false", gate)
			}
			if discoveryErr != nil {
				return
			}
			if evaluation.Result == reviewtransaction.GateAllow {
				t.Fatalf("gate %q: an unapproved, receipt-less new-lineage authority must never allow, got %#v", gate, evaluation)
			}
		})
	}
}

// TestResolveGoverningAuthorityApprovedWithoutReceiptDenies proves
// default-deny's "AND a validated receipt" half: a persisted state of
// `approved` alone is not sufficient — an integrity gap between
// review-state.json and a never-published review-receipt.json must never be
// trusted as authorization.
func TestResolveGoverningAuthorityApprovedWithoutReceiptDenies(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "approved-without-receipt-denies-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("approved-without-receipt fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	live, _, err := governingAuthorityLiveEvidence(context.Background(), repo, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateApproved
		next.CandidateIdentity = live
		next.Tier = reviewtransaction.RiskLow
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	governs, _, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), repo, lineage, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if !governs {
		t.Fatal("an approved-without-receipt authority must still govern (deny), got governs=false")
	}
	if discoveryErr != nil {
		return
	}
	if evaluation.Result == reviewtransaction.GateAllow {
		t.Fatalf("approved state without a published receipt must never allow, got %#v", evaluation)
	}
}

// TestResolveGoverningAuthorityApprovedWithValidReceiptAllowsExactCandidate
// is default-deny's positive control: an authority that genuinely reached
// `approved` with a matching, structurally valid receipt on disk still
// allows an exact live candidate — default-deny narrows WHICH state
// authorizes, it does not turn v3 lineages into a universal denial.
func TestResolveGoverningAuthorityApprovedWithValidReceiptAllowsExactCandidate(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "approved-with-receipt-allows-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("approved-with-receipt fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	live, _, err := governingAuthorityLiveEvidence(context.Background(), repo, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateApproved
		next.CandidateIdentity = live
		next.Tier = reviewtransaction.RiskLow
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteReceipt(context.Background(), reviewtransaction.NewLineageReceipt{
		Schema: reviewtransaction.NewLineageReceiptSchema, LineageID: lineage,
		TerminalState: reviewtransaction.NewLineageStateApproved, AuthorityRevision: revision,
		CandidateIdentity: live,
	}); err != nil {
		t.Fatal(err)
	}

	governs, _, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), repo, lineage, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if !governs || discoveryErr != nil || evaluation.Result != reviewtransaction.GateAllow {
		t.Fatalf("approved authority with a valid receipt and exact live candidate must allow, got governs=%v evaluation=%#v discoveryErr=%v", governs, evaluation, discoveryErr)
	}
}

// TestResolveGoverningAuthorityCandidateIdentityMismatchDenies is absorbed
// N3 (W3/W4 verify): the receipt cross-check at
// review_governing_authority.go's `approved` case compared only LineageID,
// AuthorityRevision, and TerminalState — never CandidateIdentity. A receipt
// on disk whose candidate_identity has been tampered with (or corrupted)
// while LineageID/AuthorityRevision/TerminalState still happen to match the
// authority record passed this cross-check and reached GateAllow. This
// pins the fix: the receipt's CandidateIdentity must also match the
// authority record's frozen CandidateIdentity, or the gate denies exactly
// like every other receipt-integrity gap (reviewFacadeReceiptNotAvailableReason,
// approved_without_receipt).
func TestResolveGoverningAuthorityCandidateIdentityMismatchDenies(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "candidate-identity-mismatch-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate-identity-mismatch fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	live, _, err := governingAuthorityLiveEvidence(context.Background(), repo, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateApproved
		next.CandidateIdentity = live
		next.Tier = reviewtransaction.RiskLow
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Write a genuinely valid receipt first (proving WriteReceipt itself
	// still enforces the matching CandidateIdentity at issuance time), then
	// tamper with the published bytes directly on disk — the same technique
	// TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy
	// uses to simulate storage corruption/tampering that never goes through
	// the store's own write path.
	if err := store.WriteReceipt(context.Background(), reviewtransaction.NewLineageReceipt{
		Schema: reviewtransaction.NewLineageReceiptSchema, LineageID: lineage,
		TerminalState: reviewtransaction.NewLineageStateApproved, AuthorityRevision: revision,
		CandidateIdentity: live,
	}); err != nil {
		t.Fatal(err)
	}
	tampered := live
	tampered.BaseTree = strings.Repeat("f", len(live.BaseTree))
	tamperedReceipt := reviewtransaction.NewLineageReceipt{
		Schema: reviewtransaction.NewLineageReceiptSchema, LineageID: lineage,
		TerminalState: reviewtransaction.NewLineageStateApproved, AuthorityRevision: revision,
		CandidateIdentity: tampered,
	}
	payload, err := json.MarshalIndent(tamperedReceipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ReceiptPath(), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	governs, _, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), repo, lineage, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if !governs {
		t.Fatal("a candidate-identity-mismatched receipt must still govern (deny), got governs=false")
	}
	if discoveryErr != nil {
		return
	}
	if evaluation.Result == reviewtransaction.GateAllow {
		t.Fatalf("a receipt whose candidate identity does not match the authority record must never allow, got %#v", evaluation)
	}
}

// TestResolveGoverningAuthorityCorruptReceiptNamesFreshLineageNotFinalize is
// W-7 (Wave 5 fix cycle 2, verify-report #10186): a PRESENT but tampered/
// mismatched receipt used to name the exact same continuation as a genuinely
// ABSENT one (reviewFacadeReceiptNotAvailableReason, "run gentle-ai review
// finalize --lineage %s"). Investigated: WriteReceipt's own publishImmutable
// (authority_store.go, store.go's publishNoReplace path) refuses to
// overwrite EXISTING content that differs from what it would (re)publish --
// it returns ImmutablePublicationConflictError rather than repairing the
// tamper. So for an approved lineage whose receipt exists but does not match
// the frozen authority, `review finalize --lineage <id>` would itself
// refuse: the named continuation was a dead end, not a repair. This pins the
// honest replacement: a distinct denial code (approved_receipt_corrupt, not
// approved_without_receipt) and a message naming the only continuation that
// actually clears it -- a fresh lineage; v3 has no reopen/repair machinery
// for a corrupted receipt (the same "no reopen" boundary
// AuthorityStore.CaptureLensResult enforces).
func TestResolveGoverningAuthorityCorruptReceiptNamesFreshLineageNotFinalize(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "corrupt-receipt-names-fresh-lineage"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("corrupt-receipt fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	live, _, err := governingAuthorityLiveEvidence(context.Background(), repo, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Mutate(context.Background(), "", func(next *reviewtransaction.NewLineageAuthority) error {
		next.State = reviewtransaction.NewLineageStateApproved
		next.CandidateIdentity = live
		next.Tier = reviewtransaction.RiskLow
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteReceipt(context.Background(), reviewtransaction.NewLineageReceipt{
		Schema: reviewtransaction.NewLineageReceiptSchema, LineageID: lineage,
		TerminalState: reviewtransaction.NewLineageStateApproved, AuthorityRevision: revision,
		CandidateIdentity: live,
	}); err != nil {
		t.Fatal(err)
	}
	tampered := live
	tampered.BaseTree = strings.Repeat("f", len(live.BaseTree))
	tamperedPayload, err := json.MarshalIndent(reviewtransaction.NewLineageReceipt{
		Schema: reviewtransaction.NewLineageReceiptSchema, LineageID: lineage,
		TerminalState: reviewtransaction.NewLineageStateApproved, AuthorityRevision: revision,
		CandidateIdentity: tampered,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ReceiptPath(), append(tamperedPayload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	governs, _, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), repo, lineage, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if !governs {
		t.Fatal("a corrupted receipt must still govern (deny), got governs=false")
	}
	if discoveryErr != nil {
		t.Fatalf("unexpected discovery error: %v", discoveryErr)
	}
	if evaluation.Result == reviewtransaction.GateAllow {
		t.Fatalf("a tampered receipt must never allow, got %#v", evaluation)
	}
	if evaluation.Context.Denial == nil || evaluation.Context.Denial.Code != "approved_receipt_corrupt" {
		t.Fatalf("denial code = %#v, want approved_receipt_corrupt (distinct from the genuinely-absent approved_without_receipt case)", evaluation.Context.Denial)
	}
	if strings.Contains(evaluation.Reason, "review finalize --lineage") {
		t.Fatalf("reason = %q, must not name finalize -- WriteReceipt refuses to overwrite the differing on-disk bytes, so finalize would itself refuse", evaluation.Reason)
	}
	if !strings.Contains(evaluation.Reason, "gentle-ai review start") {
		t.Fatalf("reason = %q, must name the only continuation that actually clears a corrupted receipt: a fresh lineage via review start", evaluation.Reason)
	}
}

// TestRunReviewFacadeValidateNewLineageGovernsOverAnAllowingLegacyReceipt is
// Wave 5 Slice 4's receipt-precedence proof: a v3 record AND a legacy v1
// chain both exist under the IDENTICAL lineage id and the SAME exact
// candidate tree, so ResolveGoverningAuthority's relation classification
// (new_lineage_discovery.go:120-138, unchanged by this slice) sees the v3
// record as RELATED ("exact"), not unrelated -- which selects
// GoverningAuthorityKindNew unconditionally ("Amendment C: a new-lineage
// candidate is never authorized by legacy"). The v3 record here is
// deliberately left non-terminal (reviewing, never finalized), so v3's own
// default-deny state switch denies it -- even though the legacy receipt
// alone, for the byte-identical candidate, allows. This proves v3 always
// governs once it relates to the live candidate, and this slice's new
// EvaluateLegacyGate path is correctly UNREACHABLE for a contested lineage
// id -- precedence is decided entirely upstream, before legacy projection
// ever runs. TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy
// (task 5.2) already pins the sibling corrupted-marker shape of the same
// "never fall back to legacy" invariant.
func TestRunReviewFacadeValidateNewLineageGovernsOverAnAllowingLegacyReceipt(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "shared-lineage-legacy-precedence"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("legacy would govern this exact candidate alone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Legacy: a real, terminal, exactly-governing v1 chain for this lineage
	// -- the baseline this test proves is NOT reached once the v3 record
	// below claims the identical lineage id and candidate.
	finalizeApprovedFacadeReview(t, repo, lineage)
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	var legacyOnly bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePreCommit)}, &legacyOnly); err != nil {
		t.Fatalf("legacy-only baseline before adding the v3 record must allow: %v\n%s", err, legacyOnly.String())
	}
	assertReviewGateResult(t, legacyOnly.Bytes(), reviewtransaction.GateAllow)

	// v3: a record for the IDENTICAL lineage id AND the identical staged
	// candidate, started but deliberately never finalized (reviewing).
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")
	var start bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &start); err != nil {
		t.Fatalf("new-lineage start for the same lineage id: %v\n%s", err, start.String())
	}

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	if err == nil {
		t.Fatalf("an in-flight v3 record sharing a legacy-governed lineage id must deny, got allow:\n%s", output.String())
	}
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("contested-lineage denial = %T %v, want ReviewGateDeniedError", err, err)
	}
	if denied.Result == reviewtransaction.GateAllow {
		t.Fatalf("contested-lineage denial fell through to the legacy allow: %#v", denied)
	}
	if denied.Context.Denial == nil || denied.Context.Denial.Stage != "new-lineage-validate" {
		t.Fatalf("contested-lineage denial = %#v, want the new-lineage-validate stage (v3 governing, legacy never reached)", denied.Context.Denial)
	}
}
