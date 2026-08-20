package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestCorrectionForecastDescriptorBindsFrozenCandidateAfterEdit covers issue
// #3194. A consumer cannot forecast a line count before writing the lines, so
// the bounded correction is normally already in the working tree by the time
// the forecast is submitted. Every token of the correction transition must
// therefore name the frozen reviewed candidate the correction request and the
// repository-context handle already commit to, never the moved live snapshot,
// and the descriptor STATUS issues must execute exactly as issued.
func TestCorrectionForecastDescriptorBindsFrozenCandidateAfterEdit(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store := submissionDescriptorCorrectionFixture(t)

	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	frozen := record.State.CurrentSnapshot.Identity

	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)

	status := submissionDescriptorStatus(t, repo, started.LineageID)
	if status.TargetIdentity == frozen {
		t.Fatal("fixture never moved the live workspace snapshot, so it cannot cover the reported defect")
	}
	input := submissionDescriptorInput(t, status)
	if status.NextTransition.ReasonCode != "correction_plan_required" || status.NextTransition.CorrectionRequest == nil {
		t.Fatalf("transition after the correction edit = %#v", status.NextTransition)
	}
	if request := status.NextTransition.CorrectionRequest; request.TargetIdentity != frozen {
		t.Fatalf("correction request target = %s, want the frozen reviewed candidate %s", request.TargetIdentity, frozen)
	}

	if target := submissionDescriptorTokenValue(t, *input.Submission, "--target="); target != frozen {
		t.Fatalf("descriptor --target = %s, want the frozen reviewed candidate %s", target, frozen)
	}
	handle := submissionDescriptorTokenValue(t, *input.Submission, "--repository-context=")
	_, bound, err := reviewtransaction.ResolveReviewRepositoryContextBinding(context.Background(), handle)
	if err != nil {
		t.Fatalf("resolve descriptor repository context: %v", err)
	}
	if bound.TargetIdentity != frozen {
		t.Fatalf("repository-context handle commits to %s, want the frozen reviewed candidate %s", bound.TargetIdentity, frozen)
	}
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["target"] != frozen {
		t.Fatalf("collect argument target = %s, want the frozen reviewed candidate %s", arguments["target"], frozen)
	}

	if output, err := runSubmissionDescriptor(t, *input.Submission, "1"); err != nil {
		t.Fatalf("provider-issued correction descriptor is not executable as issued: %v\n%s", err, output)
	}
	forecasted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if forecasted.State.ProposedCorrectionLines == nil || *forecasted.State.ProposedCorrectionLines != 1 ||
		len(forecasted.State.CorrectionAttempts) != 0 || forecasted.State.CumulativeCorrectionLines != 0 {
		t.Fatalf("forecast state = %#v", forecasted.State)
	}
	// The frozen candidate still governs: the forecast admitted no content and
	// must not have advanced the reviewed snapshot to the edited tree.
	if forecasted.State.CurrentSnapshot.Identity != frozen {
		t.Fatalf("forecast moved the reviewed candidate to %s", forecasted.State.CurrentSnapshot.Identity)
	}

	// The lifecycle keeps routing on the already-corrected tree instead of
	// dead-ending, which is what the reported occurrences could never reach.
	next := submissionDescriptorStatus(t, repo, started.LineageID)
	if next.NextTransition == nil || next.NextTransition.Kind != reviewNextTransitionCollect ||
		next.NextTransition.ReasonCode != "correction_repository_verification_required" {
		t.Fatalf("transition after the accepted forecast = %#v", next.NextTransition)
	}
}

// TestCorrectionForecastRefusesTargetsOutsideTheFrozenCandidate keeps the
// negative controls the #3194 fix must not weaken: the forecast still binds to
// one exact candidate, so a substituted target is refused and nothing is
// written.
func TestCorrectionForecastRefusesTargetsOutsideTheFrozenCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store := submissionDescriptorCorrectionFixture(t)

	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
	status := submissionDescriptorStatus(t, repo, started.LineageID)
	input := submissionDescriptorInput(t, status)

	for _, test := range []struct {
		name   string
		target string
	}{
		{name: "live workspace snapshot", target: status.TargetIdentity},
		{name: "unrelated identity", target: "sha256:" + strings.Repeat("a", 64)},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := readReviewOperationFile(t, store.StatePath())
			tampered := replaceSubmissionToken(t, *input.Submission, "--target=", "--target="+test.target)
			output, err := runSubmissionDescriptor(t, tampered, "1")
			assertSubmissionNotStarted(t, err, output, store, before)
		})
	}
}

func submissionDescriptorTokenValue(t *testing.T, descriptor ReviewTransitionSubmission, prefix string) string {
	t.Helper()
	for _, token := range descriptor.ArgumentTokens {
		if value, found := strings.CutPrefix(token, prefix); found {
			return value
		}
	}
	t.Fatalf("descriptor tokens lack %q: %v", prefix, descriptor.ArgumentTokens)
	return ""
}
