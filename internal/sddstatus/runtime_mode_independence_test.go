package sddstatus

import (
	"context"
	"testing"
)

// TestCompactSettleContractIsIndependentOfReviewMode holds the same immutable
// failed-evidence chain constant while toggling the review-mode value before
// acquire and before settle. Review mode must not alter the obligation named
// with a token or the result of settling that token.
func TestCompactSettleContractIsIndependentOfReviewMode(t *testing.T) {
	type contract struct {
		obligation string
		state      CompactAttemptState
		reason     CompactBlockReason
	}
	cases := []struct {
		name             string
		acquireReviewOff bool
		settleReviewOff  bool
	}{
		{name: "off to off", acquireReviewOff: true, settleReviewOff: true},
		{name: "on to on", acquireReviewOff: false, settleReviewOff: false},
		{name: "off to on", acquireReviewOff: true, settleReviewOff: false},
		{name: "on to off", acquireReviewOff: false, settleReviewOff: true},
	}

	var baseline contract
	for index, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			store := mustRuntimeStore(t, repo, "mode-independent-contract")
			store.ReviewDisabled = test.acquireReviewOff
			failedEvidence := runtimeTestHash('a')

			failedBegin, err := store.Begin(context.Background(), BeginAttemptRequest{
				RequestID: "failed-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
				MaxAttempts: 1, MaxChangedLines: 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			failed, err := store.Finish(context.Background(), FinishAttemptRequest{
				ExpectedRevision: failedBegin.Revision, RequestID: "failed-settle", Outcome: AttemptFailed,
				EvidenceRevision: failedEvidence, Diagnosis: "verification found a correction",
				HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
				ExpectedRevision: failed.Revision, RequestID: "failed-reset",
				Reason: "maintainer authorized exact-evidence remediation", Actor: "maintainer",
			}); err != nil {
				t.Fatal(err)
			}

			acquired, err := store.Acquire(context.Background(), CompactAcquireRequest{
				BeginAttemptRequest: BeginAttemptRequest{
					RequestID: "correction-acquire", WorkUnit: "verify", EvidenceGoal: "independent verification",
					MaxAttempts: 1, MaxChangedLines: 20,
				},
				RemediatesEvidenceRevision: failedEvidence,
			})
			if err != nil || acquired.State != CompactStateProceed || acquired.Token == "" {
				t.Fatalf("correction acquire = %#v err=%v", acquired, err)
			}
			store.ReviewDisabled = test.settleReviewOff
			appendRuntimeLedgerFile(t, repo, "corrected implementation\n")
			settled, err := store.Settle(context.Background(), CompactSettleRequest{
				Token: acquired.Token, RequestID: "correction-settle", Outcome: AttemptPassed,
				EvidenceRevision: runtimeTestHash('b'), Diagnosis: "corrected verification passed",
				HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
				RemediatesEvidenceRevision: failedEvidence,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := contract{obligation: acquired.SettleObligation, state: settled.State, reason: settled.Reason}
			if index == 0 {
				baseline = got
				return
			}
			if got != baseline {
				t.Fatalf("mode transition changed the compact settle contract: got %#v, baseline %#v", got, baseline)
			}
		})
	}
}
