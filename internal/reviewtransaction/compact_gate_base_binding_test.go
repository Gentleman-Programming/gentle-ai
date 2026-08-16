package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

// TestCompactCorrectedPreCommitAllowNamesTheReviewedBase pins the root cause of
// the community report. After a correction the pre-commit target is a fix diff,
// so the gate's own `base_tree` is the pre-correction candidate, not the tree
// the review was performed against. Both values are legitimate and both are
// called "base tree" by something, which is exactly why an operator who takes
// "the base" from a successful gate and builds a publication there lands in a
// refusal. The allow envelope must therefore name the reviewed base whenever it
// is not the value already shown.
func TestCompactCorrectedPreCommitAllowNamesTheReviewedBase(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := correctedCompactTestStateWithIntended(t, repo, "compact-corrected-base-binding", []string{})
	receipt := persistCorrectedCompactFixture(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt")

	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePreCommit, LineageID: state.LineageID,
	})
	if got.Result != GateAllow {
		t.Fatalf("corrected pre-commit target = %#v", got)
	}
	if got.Context.BaseTree != state.CurrentSnapshot.BaseTree {
		t.Fatalf("emitted base_tree = %q, want the derived fix-diff base %q", got.Context.BaseTree, state.CurrentSnapshot.BaseTree)
	}
	if got.Context.BaseTree == receipt.BaseTree {
		t.Fatalf("fixture no longer diverges: fix-diff base and reviewed base are both %q", receipt.BaseTree)
	}
	if got.Context.ReceiptBaseTree != receipt.BaseTree {
		t.Fatalf("allow envelope receipt_base_tree = %q, want the reviewed base %q", got.Context.ReceiptBaseTree, receipt.BaseTree)
	}
}

// TestCompactUncorrectedGateOmitsRedundantReceiptBaseTree pins the other half of
// the same contract: absence means `base_tree` IS the reviewed base. Emitting
// the same hash twice would make the new field noise instead of a signal.
func TestCompactUncorrectedGateOmitsRedundantReceiptBaseTree(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, _, receipt := approvedCompactRevisionFixture(t, repo, "compact-uncorrected-base-binding")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePostApply, LineageID: state.LineageID,
	})
	if got.Result != GateAllow {
		t.Fatalf("uncorrected post-apply target = %#v", got)
	}
	if got.Context.BaseTree != receipt.BaseTree {
		t.Fatalf("uncorrected base_tree = %q, want the reviewed base %q", got.Context.BaseTree, receipt.BaseTree)
	}
	if got.Context.ReceiptBaseTree != "" {
		t.Fatalf("receipt_base_tree = %q, want it omitted when it repeats base_tree", got.Context.ReceiptBaseTree)
	}
}

// TestCompactPrePRAllowsCorrectionOnlyDeliveryOverPreCorrectionBase is issue
// #2017 at the unit level. The publication base is the pre-correction
// candidate — the receipt-derived correction-only projection. This used to be
// framed as an operator error ("took the base from the pre-commit allow
// envelope"), and the gate refused it; the maintainer decision on #2017
// reverses that judgment: the receipt covers the original candidate plus its
// one bounded correction, so the correction-only publication over the merged
// original is exactly what the receipt approves. No other base is relaxed.
func TestCompactPrePRAllowsCorrectionOnlyDeliveryOverPreCorrectionBase(t *testing.T) {
	repo, state, receipt, baseRef := approvedCompactFixDiffFixture(t, "compact-pre-pr-correction-only")
	remote := strings.TrimSpace(gitSnapshot(t, repo, "remote", "get-url", "origin"))
	preCorrection := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver corrected candidate")
	gitSnapshot(t, repo, "--git-dir", remote, "update-ref", "refs/heads/"+strings.TrimPrefix(baseRef, "origin/"), preCorrection)

	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef,
	})
	if got.Result != GateAllow {
		t.Fatalf("correction-only delivery over the pre-correction candidate = %#v, want allow (#2017)", got)
	}
}

// TestCompactPrePRBaseMismatchNamesExpectedAndActualBase pins the denial
// diagnostics contract: a refusal must publish what the gate required, not
// just what it found. Pre-PR derives its base as the merge-base with HEAD,
// so the reachable foreign base is an OLDER ancestor — a stale mainline
// behind the reviewed base. That base is neither the reviewed base nor the
// #2017 receipt-derived correction-only base, and must still deny.
func TestCompactPrePRBaseMismatchNamesExpectedAndActualBase(t *testing.T) {
	repo, state, receipt, baseRef := approvedCompactFixDiffFixture(t, "compact-pre-pr-base-mismatch")
	stale := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD~2"))
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver corrected candidate")
	gitSnapshot(t, repo, "push", "-q", "-f", "origin", stale+":refs/heads/"+strings.TrimPrefix(baseRef, "origin/"))

	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef,
	})
	if got.Result != GateInvalidated || got.Context.Denial == nil ||
		got.Context.Denial.Stage != "receipt-binding" || got.Context.Denial.Code != "base-mismatch" {
		t.Fatalf("pre-PR base mismatch = %#v", got)
	}
	if got.Context.BaseMismatch == nil {
		t.Fatal("base-mismatch denial carries no expected/actual diagnostics")
	}
	if got.Context.BaseMismatch.Expected != receipt.BaseTree {
		t.Fatalf("expected base = %q, want the reviewed base %q", got.Context.BaseMismatch.Expected, receipt.BaseTree)
	}
	if got.Context.BaseMismatch.Actual != got.Context.BaseTree || got.Context.BaseMismatch.Actual == state.InitialSnapshot.CandidateTree {
		t.Fatalf("actual base = %q, want the live foreign base %q", got.Context.BaseMismatch.Actual, got.Context.BaseTree)
	}
	if got.Context.ReceiptBaseTree != receipt.BaseTree {
		t.Fatalf("denial receipt_base_tree = %q, want the reviewed base %q", got.Context.ReceiptBaseTree, receipt.BaseTree)
	}
}

// TestGateContextRoundTripsBaseBindingDiagnostics keeps the additive fields
// inside the same guarded validation every other optional block gets. Persisted
// invalidation evidence is re-parsed with DisallowUnknownFields and compared by
// value, so an unvalidated field would be a hole in that round trip.
func TestGateContextRoundTripsBaseBindingDiagnostics(t *testing.T) {
	base := strings.Repeat("a", 40)
	actual := strings.Repeat("b", 40)
	candidate := strings.Repeat("c", 40)
	digest := "sha256:" + strings.Repeat("d", 64)
	valid := `{"gate":"pre-pr","lineage_id":"lineage-one","generation":1,"store_revision":"` + digest +
		`","genesis_revision":"` + digest + `","chain_identity":"` + digest + `","bundle_digest":"` + digest +
		`","base_tree":"` + actual + `","candidate_tree":"` + candidate + `","paths_digest":"` + digest +
		`","fix_delta_hash":"` + digest + `","policy_hash":"` + digest + `","ledger_hash":"` + digest +
		`","evidence_hash":"` + digest + `","base_relationship_valid":false,"receipt_base_tree":"` + base +
		`","denial":{"stage":"receipt-binding","code":"base-mismatch"},"base_mismatch":{"expected":"` + base +
		`","actual":"` + actual + `"}}`
	parsed, err := ParseGateContext([]byte(valid))
	if err != nil {
		t.Fatalf("valid base-binding context: %v", err)
	}
	if parsed.ReceiptBaseTree != base || parsed.BaseMismatch == nil ||
		parsed.BaseMismatch.Expected != base || parsed.BaseMismatch.Actual != actual {
		t.Fatalf("parsed base-binding context = %#v", parsed)
	}

	for name, payload := range map[string]string{
		"base mismatch without the matching denial code": strings.Replace(valid, `"code":"base-mismatch"`, `"code":"candidate-or-paths-mismatch"`, 1),
		"base mismatch actual disagrees with base_tree":  strings.Replace(valid, `"actual":"`+actual+`"}}`, `"actual":"`+candidate+`"}}`, 1),
		"base mismatch expected is not a tree hash":      strings.Replace(valid, `"expected":"`+base+`"`, `"expected":"not-a-tree"`, 1),
		"receipt base tree repeats base_tree":            strings.Replace(valid, `"receipt_base_tree":"`+base+`"`, `"receipt_base_tree":"`+actual+`"`, 1),
		"receipt base tree is not a tree hash":           strings.Replace(valid, `"receipt_base_tree":"`+base+`"`, `"receipt_base_tree":"not-a-tree"`, 1),
	} {
		if _, err := ParseGateContext([]byte(payload)); err == nil {
			t.Fatalf("%s parsed as a valid gate context", name)
		}
	}
}
