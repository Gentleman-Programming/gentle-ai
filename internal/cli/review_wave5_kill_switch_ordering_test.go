package cli

// Wave 5 (Gate Cutover), Slice 2: named per-gate regression tests (#2222/#2239
// supersession evidence, tasks.md Gate Regression Test Index). Each of the 5
// gates gets its own disabled-before-authority-read test and its own
// switch-off double-eval byte-equivalence test, using the exact names
// tasks.md specifies.
//
// Fixture: a corrupted (truncated) compact authority record present on disk,
// per corruptReviewAuthorityInventory -- if the kill-switch check were not
// genuinely first, this damage would surface (as it did before Slice 2; see
// the removed reviewDisabledUnmanagedDeliveryReason-based visibility this
// batch's other reconciled tests moved to their enabled-mode siblings). No
// governing receipt is needed: the property under test is ORDERING (the
// switch is consulted before any authority read happens at all), which a
// no-receipt-plus-decoy-corruption fixture proves for every gate uniformly.

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewValidateRejectsUnsupportedGateBeforeRepositoryAndModeResolution(t *testing.T) {
	fixtures := []struct {
		name string
		cwd  func(*testing.T) string
	}{
		{
			name: "repository discovery",
			cwd: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
		},
		{
			name: "disabled mode bypass with corrupt authority",
			cwd:  killSwitchOrderingFixture,
		},
	}
	forms := []struct {
		name       string
		negotiated bool
	}{
		{name: "plain"},
		{name: "negotiated", negotiated: true},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for _, form := range forms {
				t.Run(form.name, func(t *testing.T) {
					args := []string{"validate", "--cwd", fixture.cwd(t), "--gate", "unsupported"}
					if form.negotiated {
						args = append(args, "--contract", ReviewIntegrationContractV1)
					}
					var output bytes.Buffer
					err := RunReview(args, &output)
					if err == nil {
						t.Fatalf("unsupported gate succeeded:\n%s", output.String())
					}

					reason := err.Error()
					if form.negotiated {
						failure := decodeReviewIntegrationFailure(t, output.Bytes())
						if failure.Phase != "preflight" || failure.Code != reviewIntegrationInvalidRequestCode ||
							failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "not_evaluated" {
							t.Fatalf("unsupported gate failure = %#v", failure)
						}
						reason = failure.Cause
					} else if output.Len() != 0 {
						t.Fatalf("plain unsupported gate emitted output before refusal: %q", output.String())
					}
					for _, gate := range reviewIntegrationGateNames() {
						if !strings.Contains(reason, gate) {
							t.Fatalf("unsupported gate refusal = %q, missing supported gate %q", reason, gate)
						}
					}
				})
			}
		})
	}
}

// killSwitchOrderingFixture builds a repo with a corrupted authority decoy
// and returns it disabled, ready for a validate call at any gate.
func killSwitchOrderingFixture(t *testing.T) string {
	t.Helper()
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	corruptReviewAuthorityInventory(t, repo)
	disableReviewForClone(t, repo)
	return repo
}

func assertGateDisabledBeforeAuthorityRead(t *testing.T, gate reviewtransaction.GateKind) {
	t.Helper()
	repo := killSwitchOrderingFixture(t)
	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &output)
	if err != nil {
		t.Fatalf("%s disabled gate vetoed delivery instead of reporting it: %v\n%s", gate, err, output.String())
	}
	// assertDisabledUnmanagedGate itself proves no discovery-kind detail
	// leaked -- the decoy corruption present on disk never surfaces, which
	// is only possible if the switch was consulted before it was ever read.
	assertDisabledUnmanagedGate(t, err, output.Bytes())
}

func assertDisabledDoubleEvalByteEquivalent(t *testing.T, gate reviewtransaction.GateKind) {
	t.Helper()
	repo := killSwitchOrderingFixture(t)
	args := []string{"--cwd", repo, "--gate", string(gate)}
	var first bytes.Buffer
	if err := RunReviewFacadeValidate(args, &first); err != nil {
		t.Fatalf("%s first disabled evaluation vetoed delivery: %v\n%s", gate, err, first.String())
	}
	var second bytes.Buffer
	if err := RunReviewFacadeValidate(args, &second); err != nil {
		t.Fatalf("%s second disabled evaluation vetoed delivery: %v\n%s", gate, err, second.String())
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("%s disabled output is not byte-equivalent across two evaluations of the identical fixture (only the toggle stayed off -- idempotence proof, zero mutation on repeat):\nfirst:\n%s\nsecond:\n%s",
			gate, first.String(), second.String())
	}
}

func TestPostApplyGate_Disabled_ReportsUnmanagedBeforeAuthorityRead(t *testing.T) {
	assertGateDisabledBeforeAuthorityRead(t, reviewtransaction.GatePostApply)
}
func TestPreCommitGate_Disabled_ReportsUnmanagedBeforeAuthorityRead(t *testing.T) {
	assertGateDisabledBeforeAuthorityRead(t, reviewtransaction.GatePreCommit)
}
func TestPrePushGate_Disabled_ReportsUnmanagedBeforeAuthorityRead(t *testing.T) {
	assertGateDisabledBeforeAuthorityRead(t, reviewtransaction.GatePrePush)
}
func TestPrePRGate_Disabled_ReportsUnmanagedBeforeAuthorityRead(t *testing.T) {
	assertGateDisabledBeforeAuthorityRead(t, reviewtransaction.GatePrePR)
}
func TestReleaseGate_Disabled_ReportsUnmanagedBeforeAuthorityRead(t *testing.T) {
	assertGateDisabledBeforeAuthorityRead(t, reviewtransaction.GateRelease)
}

func TestDisabledGateOutput_DoubleEval_ByteEquivalent_PostApply(t *testing.T) {
	assertDisabledDoubleEvalByteEquivalent(t, reviewtransaction.GatePostApply)
}
func TestDisabledGateOutput_DoubleEval_ByteEquivalent_PreCommit(t *testing.T) {
	assertDisabledDoubleEvalByteEquivalent(t, reviewtransaction.GatePreCommit)
}
func TestDisabledGateOutput_DoubleEval_ByteEquivalent_PrePush(t *testing.T) {
	assertDisabledDoubleEvalByteEquivalent(t, reviewtransaction.GatePrePush)
}
func TestDisabledGateOutput_DoubleEval_ByteEquivalent_PrePR(t *testing.T) {
	assertDisabledDoubleEvalByteEquivalent(t, reviewtransaction.GatePrePR)
}
func TestDisabledGateOutput_DoubleEval_ByteEquivalent_Release(t *testing.T) {
	assertDisabledDoubleEvalByteEquivalent(t, reviewtransaction.GateRelease)
}

// TestDisabledOutputIsByteIdenticalRegardlessOfAuthorityStoreContent is the
// decoy-store zero-reads proof referenced by emitDisabledUnmanagedDelivery's
// doc comment: the SAME gate call, across a clean repo and a repo carrying a
// corrupted authority decoy, must produce byte-identical disabled output.
// The store's content contributes nothing to what is reported, which is the
// portable, CI-safe equivalent of Wave 4's own strace-based "zero authority-
// store opens while disabled" verifier proof (TestDisabledGateNeverEmitsAllowOrCreatesReceipt
// in review_disabled_reach_test.go proves the same property across a wider
// fixture sweep at post-apply specifically; this proves it across all 5
// gates using the minimal clean-vs-decoy pair).
func TestDisabledOutputIsByteIdenticalRegardlessOfAuthorityStoreContent(t *testing.T) {
	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
		reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
	} {
		t.Run(string(gate), func(t *testing.T) {
			reviewModeHome(t)
			clean := initReviewCLIRepo(t)
			disableReviewForClone(t, clean)
			var cleanOutput bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", clean, "--gate", string(gate)}, &cleanOutput); err != nil {
				t.Fatalf("%s clean disabled evaluation vetoed delivery: %v\n%s", gate, err, cleanOutput.String())
			}

			decoy := killSwitchOrderingFixture(t)
			var decoyOutput bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", decoy, "--gate", string(gate)}, &decoyOutput); err != nil {
				t.Fatalf("%s decoy disabled evaluation vetoed delivery: %v\n%s", gate, err, decoyOutput.String())
			}
			if !bytes.Equal(cleanOutput.Bytes(), decoyOutput.Bytes()) {
				t.Fatalf("%s disabled output differs between a clean repo and a corrupted-decoy repo:\nclean:\n%s\ndecoy:\n%s",
					gate, cleanOutput.String(), decoyOutput.String())
			}
		})
	}
}
