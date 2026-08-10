package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Issue #2586, item 2 (the "governing-half" verification): does an EXISTING
// zero-delta approved receipt -- one an old build minted before this
// change's START-time guard existed, base_tree == final_candidate_tree,
// paths: [] -- ever get to GOVERN a delivery gate over later, genuinely
// unreviewed content that happens to land on top of that same base tree?
//
// It does not. Every gate that actually delivers something
// (post-apply/pre-commit/pre-push/pre-pr/release) builds its OWN live gate
// target from the real current repository state -- compactPushDeliveryBaseTree
// + buildPushTarget for pre-push, the live staged/workspace projection for
// pre-commit/post-apply (buildCompactGateRequestWithPushBase,
// internal/reviewtransaction/compact_gate.go) -- independent of whatever
// kind the receipt being checked was originally started against. Once real
// content exists, that live target's CandidateTree is no longer the
// receipt's own self-referencing tree, so validateDerivedGate's very first
// comparison (internal/reviewtransaction/receipt.go: "receipt.FinalCandidateTree
// != context.CandidateTree" -> GateScopeChanged) already refuses it -- for
// every gate, not only the ones with their own base-relationship checks.
//
// This test constructs exactly such a historical zero-delta receipt (via
// runLegacyFacadeStartForTest, which bypasses this change's own START guard
// on purpose -- it exists to build fixtures the guard newly forbids
// creating going forward) and proves neither pre-commit nor pre-push ever
// governs real content through it. No production code changes were needed
// for this half of #2586; this test is the proof.
func TestZeroDeltaReceiptNeverGovernsANonEmptyDeliveryGate(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)

	// Historical bytes: a zero-delta lineage started and finalized on the
	// clean worktree, exactly as an old build (pre-#2586-fix) would have
	// produced from a bare `review start`.
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "zero-delta"}, io.Discard); err != nil {
		t.Fatalf("construct historical zero-delta authority: %v", err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "zero-delta"}, io.Discard); err != nil {
		t.Fatalf("finalize historical zero-delta authority: %v", err)
	}

	// Real, unreviewed content lands on top of that exact base tree.
	writeScopeRecoveryFile(t, repo, "session.go", "package auth\n")
	runReviewCLIGit(t, repo, "add", "session.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "feat: auth")

	for _, gate := range []string{"pre-commit", "pre-push"} {
		var output bytes.Buffer
		err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", "zero-delta", "--gate", gate}, &output)
		if err == nil {
			t.Fatalf("%s gate allowed unreviewed content through a zero-delta receipt: %s", gate, output.String())
		}
		var denied ReviewGateDeniedError
		if !errors.As(err, &denied) {
			t.Fatalf("%s gate error = %T %v, want ReviewGateDeniedError", gate, err, err)
		}
		// GateScopeChanged specifically: validateDerivedGate's very first
		// comparison (receipt.FinalCandidateTree != context.CandidateTree)
		// already trips, for both gates, because the live target's tree
		// moved once real content landed -- confirming the receipt's own
		// tree/digest comparison, not a new non-governing rule, is what
		// keeps a zero-delta receipt from ever governing real content.
		if denied.Result != reviewtransaction.GateScopeChanged {
			t.Fatalf("%s gate result = %q, want scope-changed", gate, denied.Result)
		}
	}
}
