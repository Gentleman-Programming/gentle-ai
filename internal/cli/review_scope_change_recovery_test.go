package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestPrePushScopeChangeNamedRecoveryReachesAllow is the guard the existing
// suite was missing. Every earlier test asserted only that a scope-changed
// denial EMITTED a recovery continuation; none of them ever executed the
// recovery it named. Both pre-push scope-changed classes shipped a message
// that dead-ended: the receipt-binding class named a bare `review.recover`
// that freezes an empty current-changes successor (base_tree ==
// candidate_tree) and re-trips the same one-commit delivery rule, and the
// delivery-shape class named nothing at all.
//
// This test drives the real facade entry points the CLI dispatches to
// (RunReviewFacadeValidate, RunReviewRecover, RunReviewFacadeFinalize) over a
// real Git repository with a real bare publication remote. It reads the
// recovery out of the frozen diagnostics the denial itself carries -- never a
// recovery the test knows independently -- runs exactly that recovery, and
// requires the same gate to then return allow. A future scope-changed class
// whose named recovery dead-ends fails here.
func TestPrePushScopeChangeNamedRecoveryReachesAllow(t *testing.T) {
	tests := []struct {
		name       string
		wantDenial reviewtransaction.GateDenial
		stage      func(t *testing.T, repo string)
	}{
		{
			// Two commits past the reviewed base AND different bytes:
			// buildPushTarget ERRORS with ErrReviewedDeliveryNotOneCommit
			// before any actual snapshot exists, so discovery buckets this as
			// deliveryShape, where the receipt-binding derivation is not even
			// callable.
			name:       "delivery shape mismatch",
			wantDenial: reviewtransaction.GateDenial{Stage: "delivery-derivation", Code: "delivery-shape-mismatch"},
			stage: func(t *testing.T, repo string) {
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
				writeScopeRecoveryFile(t, repo, "beta.txt", "beta\n")
				runReviewCLIGit(t, repo, "add", "-N", "alpha.txt", "beta.txt")
				finalizeFacadeReviewForRepo(t, repo)
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha changed after approval\n")
				runReviewCLIGit(t, repo, "add", "alpha.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
				runReviewCLIGit(t, repo, "add", "beta.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: beta")
			},
		},
		{
			// The second, previously untested route into the same
			// context-less bucket: a commit inside the publication range
			// reproduces the reviewed base tree exactly, so
			// reviewedDeliveryBase finds two candidate base commits and
			// raises GateDeliveryBaseResolutionError. Like the one-commit
			// rule it fails before any actual snapshot exists.
			name:       "delivery base ambiguous",
			wantDenial: reviewtransaction.GateDenial{Stage: "delivery-derivation", Code: "delivery-base-ambiguous"},
			stage: func(t *testing.T, repo string) {
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
				runReviewCLIGit(t, repo, "add", "-N", "alpha.txt")
				finalizeFacadeReviewForRepo(t, repo)
				runReviewCLIGit(t, repo, "add", "alpha.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
				// Restore the reviewed base tree inside the range: this
				// commit and the publication base now both carry it.
				runReviewCLIGit(t, repo, "rm", "-q", "alpha.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "revert: alpha")
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha reworked after approval\n")
				runReviewCLIGit(t, repo, "add", "alpha.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha again")
			},
		},
		{
			// The community Flow 9 sequence (Refresh 7, embedded revision
			// 1b1a8676): the reviewed delivery is FULLY PUBLISHED before the
			// next change exists. The publication boundary then already
			// contains the delivery, so no commit in the range carries the
			// reviewed base tree and the assessment errors with the ambiguous
			// published delivery base; the denial names the committed
			// base-diff recovery frozen against the derived merge-base.
			//
			// This is the sequence whose follow-through regressed: every
			// earlier case in this table leaves the predecessor delivery
			// unpushed, so the composed recovery chain base still equals the
			// live publication base and the whole-chain delivery verification
			// under the final-authorization lock stayed satisfiable. With the
			// predecessor's segment published, that check (then gated on
			// recoveryBound -- chain composes -- instead of on the rebind the
			// evaluation actually relied on) became unsatisfiable, and the
			// gate answered the recovery it had itself named with
			// "compact recovery delivery changed during final authorization".
			// The boundary here is DERIVED, exactly as the testing guide runs
			// it -- no explicit --base-ref anywhere.
			name:       "published predecessor delivery then unreviewed commit",
			wantDenial: reviewtransaction.GateDenial{Stage: "delivery-derivation", Code: "delivery-base-ambiguous"},
			stage: func(t *testing.T, repo string) {
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
				runReviewCLIGit(t, repo, "add", "-N", "alpha.txt")
				finalizeFacadeReviewForRepo(t, repo)
				runReviewCLIGit(t, repo, "add", "alpha.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
				branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
				runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
				writeScopeRecoveryFile(t, repo, "beta.txt", "beta\n")
				runReviewCLIGit(t, repo, "add", "beta.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: beta unreviewed")
			},
		},
		{
			// Exactly one commit past the reviewed base, but carrying
			// different bytes: the assessment COMPLETES and reports a
			// candidate mismatch, so discovery buckets this as scopeChanged
			// with full diagnostics.
			name:       "receipt binding mismatch",
			wantDenial: reviewtransaction.GateDenial{Stage: "receipt-binding", Code: "candidate-or-paths-mismatch"},
			stage: func(t *testing.T, repo string) {
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
				runReviewCLIGit(t, repo, "add", "-N", "alpha.txt")
				finalizeFacadeReviewForRepo(t, repo)
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha changed after approval\n")
				runReviewCLIGit(t, repo, "add", "alpha.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
			configureCLIReviewPublicationRemote(t, repo, branch)
			test.stage(t, repo)

			denial, scope := prePushScopeChangeDenial(t, repo)
			if denial.Context.Denial == nil || *denial.Context.Denial != test.wantDenial {
				t.Fatalf("pre-push denial = %#v, want %#v", denial.Context.Denial, test.wantDenial)
			}
			if scope == nil {
				t.Fatalf("scope-changed denial carries no recovery diagnostics: %s", denial.Error())
			}
			if scope.RecoveryOperation != "review.recover" {
				t.Fatalf("recovery operation = %q, want review.recover", scope.RecoveryOperation)
			}

			// Execute EXACTLY the recovery the frozen diagnostics name.
			runNamedScopeChangeRecovery(t, repo, *scope, "recovered-successor")

			var validated bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-push"}, &validated); err != nil {
				t.Fatalf("gate still denied after running the recovery the message named: %v\n%s", err, validated.String())
			}
			assertReviewGateResult(t, validated.Bytes(), reviewtransaction.GateAllow)
		})
	}
}

// TestPrePushRecoveredPublishedDeliveryDriftStaysRefused is the negative
// direction of the published-predecessor case above. Narrowing the
// final-authorization chain re-verification to the rebind must never admit a
// delivery that actually drifted: after the named recovery reaches allow, the
// delivered bytes are rewritten so the push would publish content the
// successor's receipt never bound. The gate must refuse again -- an allow here
// would mean the recovery receipt authorized bytes nobody reviewed.
func TestPrePushRecoveredPublishedDeliveryDriftStaysRefused(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)

	writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
	runReviewCLIGit(t, repo, "add", "-N", "alpha.txt")
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "alpha.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
	runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
	writeScopeRecoveryFile(t, repo, "beta.txt", "beta\n")
	runReviewCLIGit(t, repo, "add", "beta.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "feat: beta unreviewed")

	_, scope := prePushScopeChangeDenial(t, repo)
	if scope == nil {
		t.Fatal("published-delivery denial carries no recovery diagnostics")
	}
	runNamedScopeChangeRecovery(t, repo, *scope, "drift-successor")
	var validated bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-push"}, &validated); err != nil {
		t.Fatalf("gate denied the recovery it named: %v\n%s", err, validated.String())
	}
	assertReviewGateResult(t, validated.Bytes(), reviewtransaction.GateAllow)

	// Rewrite the delivered commit: same topology, different bytes. The
	// successor's receipt binds the pre-drift candidate tree, so this is a
	// genuine drift between authorization and publication.
	writeScopeRecoveryFile(t, repo, "beta.txt", "beta drifted after approval\n")
	runReviewCLIGit(t, repo, "add", "beta.txt")
	runReviewCLIGit(t, repo, "commit", "-q", "--amend", "-m", "feat: beta drifted")

	var denied bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-push"}, &denied)
	if err == nil {
		t.Fatalf("pre-push allowed a drifted delivery the receipt never bound: %s", denied.String())
	}
	var gateErr ReviewGateDeniedError
	if !errors.As(err, &gateErr) {
		t.Fatalf("drifted delivery denial = %T %v, want ReviewGateDeniedError", err, err)
	}
	if gateErr.Result == reviewtransaction.GateAllow {
		t.Fatalf("drifted delivery reported allow: %s", denied.String())
	}
	var result ReviewValidateResult
	if decodeErr := json.Unmarshal(denied.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode drifted-delivery denial envelope: %v\n%s", decodeErr, denied.String())
	}
	if result.Allowed {
		t.Fatalf("drifted-delivery denial envelope reports allowed: %s", denied.String())
	}
}

// TestPreCommitScopeChangeKeepsBareCurrentChangesRecovery pins the
// gate-conditional half of the contract. A bare `review.recover` genuinely
// works at pre-commit with a dirty workspace, so pre-commit must keep naming
// it and must never acquire the committed base-diff flags it does not need.
func TestPreCommitScopeChangeKeepsBareCurrentChangesRecovery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)

	writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
	runReviewCLIGit(t, repo, "add", "-N", "alpha.txt")
	finalizeFacadeReviewForRepo(t, repo)
	// Dirty the workspace after approval so the receipt stops governing it.
	writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha changed after approval\n")
	runReviewCLIGit(t, repo, "add", "alpha.txt")

	var denied bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &denied)
	if err == nil {
		t.Fatalf("pre-commit allowed a candidate no receipt governs: %s", denied.String())
	}
	var gateErr ReviewGateDeniedError
	if !errors.As(err, &gateErr) || gateErr.Result != reviewtransaction.GateScopeChanged {
		t.Fatalf("pre-commit denial = %T %v, want a scope-changed gate denial", err, err)
	}
	scope := gateErr.Context.ScopeChange
	if scope == nil {
		t.Fatalf("pre-commit scope-changed denial carries no diagnostics: %s", denied.String())
	}
	if scope.RecoveryBaseRef != "" || scope.RecoveryScope != "" {
		t.Fatalf("pre-commit named committed base-diff recovery inputs it does not need: base_ref=%q scope=%q",
			scope.RecoveryBaseRef, scope.RecoveryScope)
	}
	// The bare recovery pre-commit names must still reach allow.
	runNamedScopeChangeRecovery(t, repo, *scope, "precommit-successor")
	var validated bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &validated); err != nil {
		t.Fatalf("pre-commit still denied after the bare recovery it named: %v\n%s", err, validated.String())
	}
	assertReviewGateResult(t, validated.Bytes(), reviewtransaction.GateAllow)
}

// TestPrePushIdenticalContentDeliveryKeepsHonestFallback pins the one pre-push
// sub-case where no single `review recover` invocation resolves the block, so
// naming one would be a lie.
//
// The delivery carries byte-identical content to the approved candidate and
// differs only in commit topology (two commits instead of one). The committed
// base-diff successor a recovery would freeze is therefore the SAME scope the
// predecessor already approved, and the recovery authority refuses it with
// "approved predecessor scope has not changed" (validateCompactRecoveryEdge ->
// compactRecoveryScopeChanged). The denial must keep the terminal
// "requires explicit maintainer action" wording rather than send the operator
// at a command that exits non-zero.
func TestPrePushIdenticalContentDeliveryKeepsHonestFallback(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)

	writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
	writeScopeRecoveryFile(t, repo, "beta.txt", "beta\n")
	runReviewCLIGit(t, repo, "add", "-N", "alpha.txt", "beta.txt")
	finalizeFacadeReviewForRepo(t, repo)
	// Deliver exactly the approved bytes, split across two commits.
	runReviewCLIGit(t, repo, "add", "alpha.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
	runReviewCLIGit(t, repo, "add", "beta.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "feat: beta")

	denial, scope := prePushScopeChangeDenial(t, repo)
	if scope != nil {
		t.Fatalf("named a recovery for a delivery whose scope is unchanged: %#v", scope)
	}
	// No recovery exists, so no command is named. What the operator gets
	// instead is the exact reason the JSON envelope already carried, rather
	// than a maintainer hand-off that tells them nothing.
	const want = "review lifecycle gate denied: scope-changed: terminal review receipts do not exactly match the live gate target: reviewed delivery is not exactly one commit from its reviewed base"
	if denial.Error() != want {
		t.Fatalf("denial message = %q, want %q", denial.Error(), want)
	}
	assertNamesNoDottedOperation(t, denial.Error())
}

// TestPrePushEmptyPublicationRangeAllowsOnlyWhenNothingIsDelivered is the j06
// block, inverted. It used to pin a denial: once the reviewed commit was
// already published, no commit in the publication range could carry the
// reviewed base tree, so the gate answered scope-changed/delivery-base-ambiguous
// and exited 1. That denial was a false positive. An empty publication range
// means this push transfers nothing, and a push that delivers nothing has
// nothing to approve and nothing to refuse -- Git itself answers "Everything
// up-to-date" in the same state.
//
// The allow is deliberately NOT a statement that anything was reviewed. It is
// derived before receipt discovery runs, it names no lineage, binds no trees,
// and carries no receipt, so nothing downstream can mistake it for an approval.
//
// "Empty publication range" is only a safe proxy for "nothing is delivered"
// when the push destination is the same repository the boundary was read from.
// The two are derived from independent configuration -- the boundary from the
// branch's tracking upstream, the destination from pushRemote/pushDefault -- so
// the last subtest pins the narrowing that keeps a cross-remote push denied.
func TestPrePushEmptyPublicationRangeAllowsOnlyWhenNothingIsDelivered(t *testing.T) {
	// deliverReviewedCommit drives a real review to a terminal receipt and
	// commits it, leaving the delivery unpublished.
	deliverReviewedCommit := func(t *testing.T, repo string) {
		t.Helper()
		writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
		runReviewCLIGit(t, repo, "add", "-N", "alpha.txt")
		finalizeFacadeReviewForRepo(t, repo)
		runReviewCLIGit(t, repo, "add", "alpha.txt")
		runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
	}

	t.Run("fully published delivery allows and approves nothing", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
		configureCLIReviewPublicationRemote(t, repo, branch)
		deliverReviewedCommit(t, repo)
		// Publish the reviewed delivery in full, exactly as the journey does.
		runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/"+branch)

		var allowed bytes.Buffer
		if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-push"}, &allowed); err != nil {
			t.Fatalf("pre-push denied a push that delivers nothing: %v\n%s", err, allowed.String())
		}
		assertReviewGateResult(t, allowed.Bytes(), reviewtransaction.GateAllow)

		var result ReviewValidateResult
		if err := json.Unmarshal(allowed.Bytes(), &result); err != nil {
			t.Fatalf("decode allowed gate envelope: %v\n%s", err, allowed.String())
		}
		if !strings.Contains(result.Reason, "publication range is empty") {
			t.Fatalf("allow reason = %q, want it to state that the publication range is empty\n%s",
				result.Reason, allowed.String())
		}
		if !strings.Contains(result.Reason, "delivers nothing") {
			t.Fatalf("allow reason = %q, want it to state that the push delivers nothing\n%s",
				result.Reason, allowed.String())
		}
		// The allow must never read as a review approval: no lineage, no bound
		// trees, no receipt hashes, and no claim about a base relationship.
		if result.Context.LineageID != "" || result.Context.Generation != 0 {
			t.Fatalf("allow names review authority (%q gen %d) it did not evaluate\n%s",
				result.Context.LineageID, result.Context.Generation, allowed.String())
		}
		if result.Context.BaseTree != "" || result.Context.CandidateTree != "" || result.Context.PathsDigest != "" {
			t.Fatalf("allow binds review scope it never checked: %#v\n%s", result.Context, allowed.String())
		}
		if result.Context.EvidenceHash != "" || result.Context.LedgerHash != "" || result.Context.PolicyHash != "" {
			t.Fatalf("allow binds receipt artifacts it never read: %#v\n%s", result.Context, allowed.String())
		}
		if result.Context.BaseRelationshipValid {
			t.Fatalf("allow claims a validated base relationship it never derived\n%s", allowed.String())
		}
	})

	t.Run("unpublished commit past the publication boundary still denies", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
		configureCLIReviewPublicationRemote(t, repo, branch)
		deliverReviewedCommit(t, repo)
		runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
		// One unreviewed commit on top of the fully published delivery: the
		// range is no longer empty, so the empty-range allow must not apply and
		// the pre-existing denial must survive byte for byte.
		writeScopeRecoveryFile(t, repo, "beta.txt", "beta\n")
		runReviewCLIGit(t, repo, "add", "beta.txt")
		runReviewCLIGit(t, repo, "commit", "-qm", "feat: beta unreviewed")

		var denied bytes.Buffer
		err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-push"}, &denied)
		if err == nil {
			t.Fatalf("pre-push allowed an unpublished unreviewed commit: %s", denied.String())
		}
		var gateErr ReviewGateDeniedError
		if !errors.As(err, &gateErr) {
			t.Fatalf("pre-push denial = %T %v, want ReviewGateDeniedError", err, err)
		}
		var result ReviewValidateResult
		if decodeErr := json.Unmarshal(denied.Bytes(), &result); decodeErr != nil {
			t.Fatalf("decode denied gate envelope: %v\n%s", decodeErr, denied.String())
		}
		if result.Allowed {
			t.Fatalf("denied gate envelope reports allowed: %s", denied.String())
		}
		if result.Context.Denial == nil {
			t.Fatalf("denial names no stage or code: %s", denied.String())
		}
		assertNamesNoDottedOperation(t, gateErr.Error())
	})

	t.Run("empty range against a different push destination still denies", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
		configureCLIReviewPublicationRemote(t, repo, branch)
		// A second real remote that already carries the base commit but none of
		// the delivery, so it is an ordinary publication target rather than the
		// empty-remote bootstrap case.
		fork := filepath.Join(t.TempDir(), "fork.git")
		runReviewCLIGit(t, repo, "clone", "--bare", repo, fork)
		runReviewCLIGit(t, repo, "remote", "add", "fork", fork)

		deliverReviewedCommit(t, repo)
		// The tracked upstream now contains every commit reachable from HEAD,
		// so the publication range against it is empty...
		runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
		// ...but `git push` would send the delivery to the fork, which has none
		// of it. Emptiness against the boundary is not emptiness of delivery.
		runReviewCLIGit(t, repo, "config", "branch."+branch+".pushRemote", "fork")

		var denied bytes.Buffer
		err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-push"}, &denied)
		if err == nil {
			t.Fatalf("pre-push allowed a delivery to a remote that has none of it: %s", denied.String())
		}
		var gateErr ReviewGateDeniedError
		if !errors.As(err, &gateErr) {
			t.Fatalf("pre-push denial = %T %v, want ReviewGateDeniedError", err, err)
		}
		var result ReviewValidateResult
		if decodeErr := json.Unmarshal(denied.Bytes(), &result); decodeErr != nil {
			t.Fatalf("decode denied gate envelope: %v\n%s", decodeErr, denied.String())
		}
		if result.Allowed {
			t.Fatalf("denied gate envelope reports allowed: %s", denied.String())
		}
	})
}

// prePushScopeChangeDenial runs the pre-push gate, requires a scope-changed
// denial, and returns it with the frozen recovery diagnostics it carries.
func prePushScopeChangeDenial(t *testing.T, repo string) (ReviewGateDeniedError, *reviewtransaction.GateScopeChangeDiagnostics) {
	t.Helper()
	var denied bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-push"}, &denied)
	if err == nil {
		t.Fatalf("pre-push allowed a candidate no receipt governs: %s", denied.String())
	}
	var gateErr ReviewGateDeniedError
	if !errors.As(err, &gateErr) {
		t.Fatalf("pre-push denial = %T %v, want ReviewGateDeniedError", err, err)
	}
	if gateErr.Result != reviewtransaction.GateScopeChanged {
		t.Fatalf("pre-push gate result = %q, want scope-changed\n%s", gateErr.Result, denied.String())
	}
	return gateErr, gateErr.Context.ScopeChange
}

// runNamedScopeChangeRecovery executes the recovery the diagnostics name and
// drives the successor to a terminal approved receipt. Every argument is read
// out of the diagnostics, so the test can only pass when the recommendation
// itself is the one that works.
func runNamedScopeChangeRecovery(t *testing.T, repo string, scope reviewtransaction.GateScopeChangeDiagnostics, successor string) {
	t.Helper()
	args := []string{
		"--cwd", repo,
		"--predecessor-lineage", scope.PredecessorLineageID,
		"--expected-predecessor-revision", scope.PredecessorRevision,
		"--successor-lineage", successor,
		"--disposition", "scope_changed",
	}
	if scope.RecoveryBaseRef != "" {
		args = append(args, "--base-ref", scope.RecoveryBaseRef)
	}
	if scope.RecoveryScope == reviewtransaction.RecoveryScopeCommittedBaseDiff {
		args = append(args, "--committed-only")
	}
	var recovered bytes.Buffer
	if err := RunReviewRecover(args, &recovered); err != nil {
		t.Fatalf("named recovery %v failed: %v\n%s", args, err, recovered.String())
	}
	// The successor is a fresh review: drive whatever proportional plan its
	// own frozen state selected, exactly as a real operator would.
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, successor)
	if err != nil {
		t.Fatalf("open recovered successor: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatalf("load recovered successor: %v", err)
	}
	started := ReviewFacadeStartResult{SelectedLenses: record.State.SelectedLenses}
	finalizeArgs := append([]string{"--cwd", repo, "--lineage", successor}, facadeReviewerResultArgs(t, repo, started)...)
	if len(started.SelectedLenses) > 0 {
		evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
		if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		finalizeArgs = append(finalizeArgs, "--evidence", evidencePath)
	}
	if err := RunReviewFacadeFinalize(finalizeArgs, io.Discard); err != nil {
		t.Fatalf("finalize recovery successor: %v", err)
	}
}

func writeScopeRecoveryFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
		t.Fatal(fmt.Errorf("write %s: %w", name, err))
	}
}
