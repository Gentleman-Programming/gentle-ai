package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is coverage closure for spec rdd-review-core-transitions ->
// "Consent-Gated Freeze With Immutable Tier, Lenses, and Budget" -> "Frozen
// tier is never recomputed". No production behavior changes here — this is
// a new test only.

// TestNewLineageFrozenTierIsNeverRecomputedAfterFreeze freezes a candidate at
// RiskLow, drives the working tree to something a FRESH risk computation
// would classify as RiskHigh, and proves every subsequent read — a direct
// authority-store read (the closest analogue to STATUS this wave ships;
// `review status` has no v3 wiring yet, mirroring design decision 8's own
// unwired-shape precedent for OfferReviewAfterVerify) and `review validate`
// (CLI-wired) — still reports the exact tier, lens set, and correction
// budget frozen at start.
func TestNewLineageFrozenTierIsNeverRecomputedAfterFreeze(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "frozen-tier-lineage"
	// Freeze at RiskLow: a single passive documentation file, the exact
	// shape ClassifyRisk's OnlyPassiveContentChanges arm requires.
	startNewLineageForFinalizeTest(t, repo, lineage)

	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	afterStart, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterStart.Authority.Tier != reviewtransaction.RiskLow {
		t.Fatalf("fixture sanity check failed: frozen tier at start = %q, want low", afterStart.Authority.Tier)
	}

	// Finalize to approved so the later `review validate` genuinely drives
	// ReviewCore.Next(validate)'s relation computation instead of being
	// short-circuited by default-deny before ever reaching it (a merely
	// `reviewing` authority denies outright and never calls Next(validate) —
	// see resolveGoverningAuthority's default-deny switch).
	var finalizeOut bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &finalizeOut); err != nil {
		t.Fatalf("new-lineage finalize: %v\n%s", err, finalizeOut.String())
	}
	afterFinalize, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterFinalize.Authority.Tier != reviewtransaction.RiskLow {
		t.Fatalf("frozen tier after finalize = %q, want low (unchanged)", afterFinalize.Authority.Tier)
	}
	frozenRevision := afterFinalize.Revision

	// Drift the live working tree to something ClassifyRisk's own hot-path
	// arm (hasHighSignal / touchesHotPath) classifies as RiskHigh — proving
	// the fixture is non-vacuous: a fresh computation over the CURRENT tree
	// genuinely disagrees with the frozen tier, not merely "happens to
	// agree either way".
	authPath := filepath.Join(repo, "internal", "auth", "new_handler.go")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, []byte("package auth\n\nfunc Handle() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "internal/auth/new_handler.go")

	freshTier, err := reviewtransaction.ClassifyRisk(reviewtransaction.RiskInput{
		Stats: []reviewtransaction.DiffStat{{Path: "internal/auth/new_handler.go", Additions: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if freshTier != reviewtransaction.RiskHigh {
		t.Fatalf("fixture sanity check failed: a fresh risk computation over the drifted tree = %q, want high (otherwise this test cannot tell frozen from recomputed)", freshTier)
	}

	afterDrift, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterDrift.Authority.Tier != reviewtransaction.RiskLow ||
		afterDrift.Authority.CorrectionBudgetLines != afterFinalize.Authority.CorrectionBudgetLines ||
		!reflect.DeepEqual(afterDrift.Authority.SelectedLenses, afterFinalize.Authority.SelectedLenses) ||
		afterDrift.Revision != frozenRevision {
		t.Fatalf("authority-store read after drift = %#v (revision %s), want the exact frozen tier/lenses/budget/revision (tier=low, revision=%s)",
			afterDrift.Authority, afterDrift.Revision, frozenRevision)
	}

	// `review validate`: the candidate now diverges (an added file), so this
	// genuinely drives ReviewCore.Next(validate)'s relation computation, not
	// a short-circuited denial. The result (allow/deny/scope-changed) is
	// irrelevant to this test; only whether the read afterward still shows
	// the frozen tier untouched.
	var validateOut bytes.Buffer
	_ = RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePostApply)}, &validateOut)

	afterValidate, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterValidate.Authority.Tier != reviewtransaction.RiskLow ||
		afterValidate.Authority.CorrectionBudgetLines != afterFinalize.Authority.CorrectionBudgetLines ||
		!reflect.DeepEqual(afterValidate.Authority.SelectedLenses, afterFinalize.Authority.SelectedLenses) ||
		afterValidate.Revision != frozenRevision {
		t.Fatalf("authority-store read after validate = %#v (revision %s), want the exact frozen tier/lenses/budget/revision (tier=low, revision=%s)",
			afterValidate.Authority, afterValidate.Revision, frozenRevision)
	}
}
