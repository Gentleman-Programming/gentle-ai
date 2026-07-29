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

// TestCompactPrePRBaseMismatchNamesExpectedAndActualBase rebuilds the community
// fixture: a correction receipt whose publication base is the pre-correction
// candidate — the exact tree the pre-commit allow envelope reported. The gate is
// right to refuse, but the refusal published only what it found and never what
// it required, so the operator read their own value echoed back with no
// expectation anywhere in the envelope.
func TestCompactPrePRBaseMismatchNamesExpectedAndActualBase(t *testing.T) {
	repo, state, receipt, baseRef := approvedCompactFixDiffFixture(t, "compact-pre-pr-base-mismatch")
	remote := strings.TrimSpace(gitSnapshot(t, repo, "remote", "get-url", "origin"))
	preCorrection := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver corrected candidate")
	gitSnapshot(t, repo, "--git-dir", remote, "update-ref", "refs/heads/"+strings.TrimPrefix(baseRef, "origin/"), preCorrection)

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
	if got.Context.BaseMismatch.Actual != got.Context.BaseTree || got.Context.BaseMismatch.Actual != state.InitialSnapshot.CandidateTree {
		t.Fatalf("actual base = %q, want the live derived base %q", got.Context.BaseMismatch.Actual, got.Context.BaseTree)
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
