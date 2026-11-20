package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestExplicitCommittedRangeStatusPreservesFrozenUntrackedExclusion is the
// RED-first proof for #6058. A committed-range correction that froze an empty
// intended-untracked EXCLUDE must keep that decision when the bound STATUS
// names the same committed base explicitly -- both the exact continuation the
// final capture returned and the commit START was pinned to -- instead of
// demanding a fresh live declaration because an unrelated untracked file
// exists. A genuinely different base must never inherit the frozen decision.
func TestExplicitCommittedRangeStatusPreservesFrozenUntrackedExclusion(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		baseRef        string // continuation, pinned-commit, selectorless, or a literal ref
		wantCorrection bool
	}{
		{name: "exact-returned-continuation", baseRef: "continuation", wantCorrection: true},
		{name: "pinned-starting-commit", baseRef: "pinned-commit", wantCorrection: true},
		{name: "selectorless-re-entry", baseRef: "selectorless", wantCorrection: true},
		{name: "conflicting-base-does-not-inherit", baseRef: "HEAD"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			const lineage = "committed-range-correction-explicit-bound-6058"
			closure := closeCommittedBaseDiffLineageIntoCorrectionRequired(t, repo, lineage)
			record := loadCompactReviewRecordForTest(t, repo, lineage)
			if record.State.State != reviewtransaction.StateCorrectionRequired ||
				record.State.CurrentSnapshot.Kind != reviewtransaction.TargetBaseDiff ||
				len(record.State.InitialSnapshot.IntendedUntracked) != 0 {
				t.Fatalf("fixture authority = state %q kind %q untracked %v, want correction_required base-diff with a frozen empty selection",
					record.State.State, record.State.CurrentSnapshot.Kind, record.State.InitialSnapshot.IntendedUntracked)
			}
			if closure == nil || closure.StatusContinuation == nil || closure.StatusContinuation.Operation != "review.status" {
				t.Fatalf("final reviewer capture closure = %#v, want the bound review.status continuation", closure)
			}

			continuation := []string{strings.TrimPrefix(closure.StatusContinuation.Operation, "review.")}
			for _, argument := range closure.StatusContinuation.Arguments {
				continuation = append(continuation, argument.Token)
			}
			args := continuation
			switch testCase.baseRef {
			case "continuation":
				// The provider-issued shape, replayed unchanged, must itself name
				// the frozen committed selector rather than fall back to
				// selectorless.
				if !reviewArgsContain(continuation, "--base-ref="+record.State.CurrentSnapshot.BaseTree) ||
					!reviewArgsContain(continuation, "--committed-only=true") {
					t.Fatalf("provider-issued continuation = %v, want the frozen committed selector", continuation)
				}
			case "pinned-commit":
				// The issue shape: the operator replays the commit START was
				// pinned to instead of the frozen tree the continuation carries.
				args = replaceReviewStatusBaseRef(args, strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD~1")))
			case "selectorless":
				// The previous selectorless re-entry the same authority answered.
				args = []string{
					"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
					"--next-transition", "--lineage", lineage,
				}
			default:
				args = replaceReviewStatusBaseRef(args, testCase.baseRef)
			}

			// The unrelated untracked file from the report: outside the frozen
			// EXCLUDE decision it can neither enter the candidate nor reopen it.
			writeUndeclaredWorkspaceFile(t, repo, "unrelated-scratch.txt", "local-only notes\n", 0o644)

			var output bytes.Buffer
			err := RunReview(args, &output)
			if !testCase.wantCorrection {
				if err == nil {
					var rejected ReviewTargetStatusResult
					decodeStrictReviewJSON(t, output.Bytes(), &rejected)
					if rejected.NextTransition != nil && rejected.NextTransition.ReasonCode == "correction_plan_required" {
						t.Fatalf("conflicting base inherited the frozen correction decision: %#v", rejected.NextTransition)
					}
					if rejected.TargetIdentity == record.State.CurrentSnapshot.Identity {
						t.Fatalf("conflicting base resolved to the frozen target identity %q", rejected.TargetIdentity)
					}
				}
				after := loadCompactReviewRecordForTest(t, repo, lineage)
				if after.Revision != record.Revision {
					t.Fatalf("rejected conflicting base changed authority revision %q -> %q", record.Revision, after.Revision)
				}
				return
			}
			if err != nil {
				t.Fatalf("bound committed-range STATUS: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if err := status.Validate(); err != nil {
				t.Fatalf("bound committed-range STATUS validate: %v", err)
			}
			if status.Authority == nil || status.Authority.LineageID != lineage ||
				status.Authority.State != reviewtransaction.StateCorrectionRequired ||
				status.Authority.Revision != record.Revision {
				t.Fatalf("bound committed-range STATUS authority = %#v, want correction_required %q at revision %q", status.Authority, lineage, record.Revision)
			}
			transition := status.NextTransition
			if transition == nil || transition.Kind != reviewNextTransitionCollect ||
				transition.ReasonCode != "correction_plan_required" || transition.CorrectionRequest == nil {
				t.Fatalf("bound committed-range STATUS transition = %#v, want collect/correction_plan_required", transition)
			}
			if status.TargetIdentity != record.State.CurrentSnapshot.Identity ||
				transition.CorrectionRequest.TargetIdentity != record.State.CurrentSnapshot.Identity {
				t.Fatalf("bound committed-range bindings = status %q request %q, want frozen target %q",
					status.TargetIdentity, transition.CorrectionRequest.TargetIdentity, record.State.CurrentSnapshot.Identity)
			}

			// The subsequent submission (the offered correction plan) must
			// retain the same frozen binding.
			planArgs := append(reviewTransitionInputTokens(t, repo, transition.Collect.Inputs[0]), "--correction-lines", "1")
			var planOutput bytes.Buffer
			if err := RunReviewCaptureCorrectionPlan(planArgs, &planOutput); err != nil {
				t.Fatalf("capture the offered correction plan: %v\n%s", err, planOutput.String())
			}
			var planned reviewCorrectionPlanCaptureResult
			decodeStrictReviewJSON(t, planOutput.Bytes(), &planned)
			if planned.LineageID != lineage || planned.TargetIdentity != record.State.CurrentSnapshot.Identity ||
				planned.State != reviewtransaction.StateCorrectionRequired {
				t.Fatalf("accepted correction plan = %#v, want lineage %q bound to frozen target %q", planned, lineage, record.State.CurrentSnapshot.Identity)
			}
		})
	}
}

// replaceReviewStatusBaseRef rewrites the committed selector of one STATUS
// invocation, so a test can replay the same bound continuation under a
// different spelling of the base range.
func replaceReviewStatusBaseRef(args []string, baseRef string) []string {
	replaced := append([]string(nil), args...)
	for index, argument := range replaced {
		if strings.HasPrefix(argument, "--base-ref=") {
			replaced[index] = "--base-ref=" + baseRef
			return replaced
		}
	}
	return append(replaced, "--base-ref="+baseRef, "--committed-only")
}

func reviewArgsContain(args []string, want string) bool {
	for _, argument := range args {
		if argument == want {
			return true
		}
	}
	return false
}
