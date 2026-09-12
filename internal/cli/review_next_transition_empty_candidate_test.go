package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// emptyWorkspaceCandidateStatus reproduces the status a clean workspace
// produces once the reviewed work is already committed: a fresh target whose
// workspace projection froze zero paths, so base and candidate trees are the
// same tree. Executing the START this classification used to return fails
// deterministically in preflight with empty_candidate_scope, which is what
// issue #2584 observed looping.
func emptyWorkspaceCandidateStatus() ReviewTargetStatusResult {
	return ReviewTargetStatusResult{
		Contract:       ReviewIntegrationContractV2,
		Applicability:  reviewtransaction.TargetApplicabilityUnrelated,
		Action:         reviewtransaction.TargetStatusActionStart,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Projection: ReviewTargetStatusProjection{
			Kind:                 reviewtransaction.TargetCurrentChanges,
			Projection:           reviewtransaction.ProjectionWorkspace,
			BaseTree:             strings.Repeat("c", 40),
			CurrentCandidateTree: strings.Repeat("c", 40),
			Paths:                nil,
		},
	}
}

// derivedCommittedRangeBaseCommit is the merge-base commit a derived
// committed-range START discloses. It is deliberately a commit-shaped OID, not
// a tree, so the test can tell the two apart.
const derivedCommittedRangeBaseCommit = "1111111111111111111111111111111111111111"

// derivedCommittedRangeStatus is the base-diff status the selectorless
// derivation hands newReviewNextTransition when the fresh workspace candidate
// froze zero paths and the remote default branch named an unambiguous committed
// range (issue #4412). Its identity is recomputed from its own published
// components so the transition carries the same target-evidence token the
// working `--base-ref --committed-only` STATUS path publishes, and so the
// committedRangeBaseRef override is exercised end to end.
func derivedCommittedRangeStatus() *ReviewTargetStatusResult {
	baseTree := strings.Repeat("c", 40)
	candidateTree := strings.Repeat("d", 40)
	pathsDigest := "sha256:" + strings.Repeat("e", 64)
	return &ReviewTargetStatusResult{
		Schema:                ReviewIntegrationStatusSchema,
		Contract:              ReviewIntegrationContractV2,
		Operation:             "review.status",
		Applicability:         reviewtransaction.TargetApplicabilityUnrelated,
		Action:                reviewtransaction.TargetStatusActionStart,
		Replayability:         reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity:        reviewtransaction.IdentityForComponents(reviewtransaction.TargetBaseDiff, reviewtransaction.ProjectionWorkspace, baseTree, candidateTree, pathsDigest),
		repositoryRoot:        "/review-empty-candidate-repo",
		committedRangeBaseRef: derivedCommittedRangeBaseCommit,
		Projection: ReviewTargetStatusProjection{
			Kind:                 reviewtransaction.TargetBaseDiff,
			Projection:           reviewtransaction.ProjectionWorkspace,
			BaseTree:             baseTree,
			CurrentCandidateTree: candidateTree,
			PathsDigest:          pathsDigest,
			Paths:                []string{"internal/cli/review_next_transition.go"},
		},
	}
}

// TestNegotiatedStatusDerivesCommittedRangeStartForEmptyWorkspaceCandidate pins
// the new policy (issue #4412): when the remote default branch names an
// unambiguous committed range, the unroutable empty_candidate_base_ref_required
// collect is replaced by the executable committed-range START the working
// `--base-ref --committed-only` STATUS path already publishes. Its arguments
// disclose the derived base commit and committed-only=true, and it never carries
// the empty workspace target.
func TestNegotiatedStatusDerivesCommittedRangeStartForEmptyWorkspaceCandidate(t *testing.T) {
	status := emptyWorkspaceCandidateStatus()
	derived := derivedCommittedRangeStatus()
	status.derivedCommittedRange = derived

	got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if got.Kind != reviewNextTransitionExecute || got.ReasonCode != "fresh_target_ready" {
		t.Fatalf("derived empty workspace transition = %q/%q, want %q/%q; an unambiguous committed range must not fall back to the unroutable collect", got.Kind, got.ReasonCode, reviewNextTransitionExecute, "fresh_target_ready")
	}
	if got.Execute == nil || got.Execute.Operation != "review.start" {
		t.Fatalf("derived empty workspace execute = %#v, want review.start", got.Execute)
	}
	arguments, err := reviewTransitionArgumentMap(got.Execute.Arguments, got.Execute.Operation)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["base-ref"] != derivedCommittedRangeBaseCommit {
		t.Fatalf("derived START base-ref = %q, want the derived base commit %q (never the tree object)", arguments["base-ref"], derivedCommittedRangeBaseCommit)
	}
	if arguments["committed-only"] != "true" {
		t.Fatalf("derived START committed-only = %q, want true", arguments["committed-only"])
	}
	if arguments["target"] != derived.TargetIdentity {
		t.Fatalf("derived START target = %q, want the base-diff identity %q", arguments["target"], derived.TargetIdentity)
	}
	if arguments["target"] == status.TargetIdentity {
		t.Fatalf("derived START carried the empty workspace target %q", status.TargetIdentity)
	}
	if got.Execute.Binding.TargetIdentity != derived.TargetIdentity {
		t.Fatalf("derived START binding target = %q, want %q", got.Execute.Binding.TargetIdentity, derived.TargetIdentity)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("derived empty workspace transition Validate() = %v, want nil", err)
	}
}

// TestNegotiatedStatusCollectsBaseRefForAmbiguousCommittedRange pins the
// retained fallback: with no derived status (no origin/HEAD, a criss-cross
// history, an empty range, or a Git fault) the transition stays exactly today's
// external.select_base_ref collect.
func TestNegotiatedStatusCollectsBaseRefForAmbiguousCommittedRange(t *testing.T) {
	got := newReviewNextTransition(emptyWorkspaceCandidateStatus(), nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if got.Kind != reviewNextTransitionCollect {
		t.Fatalf("ambiguous empty workspace candidate transition kind = %q, want %q; the fallback must stay the collect", got.Kind, reviewNextTransitionCollect)
	}
	if got.ReasonCode != "empty_candidate_base_ref_required" {
		t.Fatalf("ambiguous empty workspace candidate reason code = %q, want %q", got.ReasonCode, "empty_candidate_base_ref_required")
	}
	if got.Execute != nil {
		t.Fatalf("ambiguous empty workspace candidate carries an executable transition %#v, want none", got.Execute)
	}
	if got.Collect == nil || len(got.Collect.Inputs) != 1 {
		t.Fatalf("ambiguous empty workspace candidate collection = %#v, want exactly one input", got.Collect)
	}
	input := got.Collect.Inputs[0]
	if input.Name != "base_ref" || input.CaptureOperation != "external.select_base_ref" {
		t.Fatalf("ambiguous collected input = %#v, want the base_ref external selection", input)
	}
	if input.Submission != nil {
		t.Fatalf("collected base_ref input carries submission descriptor %#v, want none: no submission is schema-legal for external.select_base_ref", input.Submission)
	}
	for _, argument := range input.Arguments {
		if argument.Name == "base_ref" || argument.Name == "base-ref" {
			t.Fatalf("collected base_ref input pre-supplies %q=%q, want the caller to choose the base", argument.Name, argument.Value)
		}
	}
}

// The emitted continuation has to survive the same contract validation every
// negotiated transition crosses; an unrepresentable collection would strand
// the caller exactly like the looping START did.
func TestNegotiatedEmptyCandidateCollectionIsRepresentable(t *testing.T) {
	got := newReviewNextTransition(emptyWorkspaceCandidateStatus(), nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if err := got.Validate(); err != nil {
		t.Fatalf("empty workspace candidate transition Validate() = %v, want nil", err)
	}
}

// The status contract validates a fresh target's transition separately from
// the transition's own shape, and it demanded an executable START for every
// fresh target that was not already stopping. Emitting the collection without
// teaching that validator about it would move the same disagreement from
// preflight into status validation.
func TestNegotiatedEmptyCandidateCollectionSatisfiesStatusTargetValidation(t *testing.T) {
	result := emptyWorkspaceCandidateStatus()
	result.NextTransition = &ReviewNextTransition{}
	*result.NextTransition = newReviewNextTransition(result, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if err := result.validateNextTransitionTargets(); err != nil {
		t.Fatalf("empty workspace candidate validateNextTransitionTargets() = %v, want nil", err)
	}
}

// The derived committed-range execute is the other admissible shape for the
// zero-path workspace branch, so status-target validation must accept it while
// still refusing every malformed variant of both forms.
func TestNegotiatedDerivedCommittedRangeSatisfiesStatusTargetValidation(t *testing.T) {
	result := emptyWorkspaceCandidateStatus()
	result.derivedCommittedRange = derivedCommittedRangeStatus()
	result.NextTransition = &ReviewNextTransition{}
	*result.NextTransition = newReviewNextTransition(result, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if result.NextTransition.Kind != reviewNextTransitionExecute {
		t.Fatalf("derived status transition kind = %q, want %q", result.NextTransition.Kind, reviewNextTransitionExecute)
	}
	if err := result.validateNextTransitionTargets(); err != nil {
		t.Fatalf("derived committed-range validateNextTransitionTargets() = %v, want nil", err)
	}
}

func TestNegotiatedEmptyCandidateCollectionRejectsMalformedStatusTarget(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReviewNextTransition)
	}{
		{
			name: "missing collection",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect = nil
			},
		},
		{
			name: "wrong input name",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Name = "lineage_selection"
			},
		},
		{
			name: "wrong target arguments",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Arguments[0].Value = "sha256:" + strings.Repeat("a", 64)
			},
		},
		{
			name: "missing target arguments",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Arguments = nil
			},
		},
		{
			name: "multiple inputs",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs = append(transition.Collect.Inputs, transition.Collect.Inputs[0])
			},
		},
		{
			name: "unexpected submission descriptor",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Submission = &ReviewTransitionSubmission{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := emptyWorkspaceCandidateStatus()
			result.NextTransition = &ReviewNextTransition{}
			*result.NextTransition = newReviewNextTransition(result, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})
			tt.mutate(result.NextTransition)

			if err := result.validateNextTransitionTargets(); err == nil {
				t.Fatal("malformed empty workspace candidate transition passed status-target validation")
			}
		})
	}
}

func TestNegotiatedDerivedCommittedRangeRejectsMalformedStatusTarget(t *testing.T) {
	setArgument := func(transition *ReviewNextTransition, name, value string) {
		for index := range transition.Execute.Arguments {
			if transition.Execute.Arguments[index].Name == name {
				transition.Execute.Arguments[index].Value = value
				return
			}
		}
		t.Fatalf("derived execute has no %q argument to mutate", name)
	}
	tests := []struct {
		name   string
		mutate func(*ReviewTargetStatusResult)
	}{
		{
			name: "execute without the derived committed range",
			mutate: func(result *ReviewTargetStatusResult) {
				result.derivedCommittedRange = nil
			},
		},
		{
			name: "wrong base-ref",
			mutate: func(result *ReviewTargetStatusResult) {
				setArgument(result.NextTransition, "base-ref", strings.Repeat("2", 40))
			},
		},
		{
			name: "wrong target",
			mutate: func(result *ReviewTargetStatusResult) {
				setArgument(result.NextTransition, "target", "sha256:"+strings.Repeat("a", 64))
			},
		},
		{
			name: "dropped committed-only",
			mutate: func(result *ReviewTargetStatusResult) {
				setArgument(result.NextTransition, "committed-only", "false")
			},
		},
		{
			name: "wrong operation",
			mutate: func(result *ReviewTargetStatusResult) {
				result.NextTransition.Execute.Operation = "review.validate"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := emptyWorkspaceCandidateStatus()
			result.derivedCommittedRange = derivedCommittedRangeStatus()
			result.NextTransition = &ReviewNextTransition{}
			*result.NextTransition = newReviewNextTransition(result, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})
			tt.mutate(&result)

			if err := result.validateNextTransitionTargets(); err == nil {
				t.Fatal("malformed derived committed-range transition passed status-target validation")
			}
		})
	}
}

// Regression: a fresh candidate that really has changed paths keeps the
// executable START it always returned.
func TestNegotiatedStatusKeepsStartForNonEmptyWorkspaceCandidate(t *testing.T) {
	status := emptyWorkspaceCandidateStatus()
	status.Projection.Paths = []string{"internal/cli/review_next_transition.go"}
	status.Projection.CurrentCandidateTree = strings.Repeat("d", 40)

	got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if got.Kind != reviewNextTransitionExecute || got.ReasonCode != "fresh_target_ready" {
		t.Fatalf("non-empty fresh candidate transition = %q/%q, want %q/%q", got.Kind, got.ReasonCode, reviewNextTransitionExecute, "fresh_target_ready")
	}
	if got.Execute == nil || got.Execute.Operation != "review.start" {
		t.Fatalf("non-empty fresh candidate execute = %#v, want review.start", got.Execute)
	}
}

// Issue #3102: the empty-root bootstrap policy from #1641 requires a typed
// STOP when a committed base-diff names the candidate itself. START rejects
// this exact zero-path scope before authority evaluation, so STATUS must not
// offer that START or collect the already-supplied base reference again.
func TestNegotiatedStatusStopsZeroPathBaseDiffCandidate(t *testing.T) {
	status := emptyWorkspaceCandidateStatus()
	status.Projection.Kind = reviewtransaction.TargetBaseDiff
	status.Action = reviewtransaction.TargetStatusActionStop
	status.Replayability = reviewtransaction.ReplayabilityManualActionRequired

	got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-base-diff"})

	if got.Kind != reviewNextTransitionStop {
		t.Fatalf("zero-path base-diff transition kind = %q, want %q", got.Kind, reviewNextTransitionStop)
	}
	if got.ReasonCode != "empty_base_diff_bootstrap_required" {
		t.Fatalf("zero-path base-diff reason code = %q, want empty_base_diff_bootstrap_required", got.ReasonCode)
	}
	if got.Execute != nil || got.Collect != nil {
		t.Fatalf("zero-path base-diff STOP exposed continuation data: %#v", got)
	}
	status.NextTransition = &got
	if err := status.validateNextTransitionTargets(); err != nil {
		t.Fatalf("zero-path base-diff validateNextTransitionTargets() = %v, want nil", err)
	}
}

// committedRangeReviewRepo builds a repository whose remote default branch
// (origin/main) sits behind HEAD, so the selectorless workspace candidate is
// empty while an unambiguous committed range exists above it.
func committedRangeReviewRepo(t *testing.T) string {
	t.Helper()
	repo := initReviewCLIRepo(t)
	origin := t.TempDir()
	runReviewCLIGit(t, origin, "init", "--bare", "-q")
	runReviewCLIGit(t, repo, "remote", "add", "origin", origin)
	runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/main")
	runReviewCLIGit(t, repo, "fetch", "-q", "origin")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeReviewStartCandidate(t, repo, "tracked.txt", "base\ncommitted\n", 0o644)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "committed candidate")
	return repo
}

// TestNegotiatedStatusDerivedCommittedRangePayloadValidatesAgainstPublishedSchema
// is the execution-based proof that the real selectorless STATUS payload for a
// committed-range candidate still validates against the published status-v7
// schema, and that the emitted START discloses the derived merge-base commit and
// committed-only=true.
func TestNegotiatedStatusDerivedCommittedRangePayloadValidatesAgainstPublishedSchema(t *testing.T) {
	reviewEnabledHome(t)
	repo := committedRangeReviewRepo(t)
	expectedBase := strings.TrimSpace(runReviewCLIGit(t, repo, "merge-base", "HEAD", "refs/remotes/origin/main"))

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "claude-code", "--next-transition"}, &output); err != nil {
		t.Fatalf("selectorless STATUS for a committed-range candidate: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Projection.Kind != reviewtransaction.TargetCurrentChanges || len(status.Projection.Paths) != 0 {
		t.Fatalf("envelope projection = %#v, want the classified empty workspace candidate", status.Projection)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "fresh_target_ready" || status.NextTransition.Execute == nil {
		t.Fatalf("next_transition = %#v, want an executable fresh_target_ready START", status.NextTransition)
	}
	arguments, err := reviewTransitionArgumentMap(status.NextTransition.Execute.Arguments, status.NextTransition.Execute.Operation)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["base-ref"] != expectedBase {
		t.Fatalf("derived base-ref = %q, want the remote default branch merge-base %q", arguments["base-ref"], expectedBase)
	}
	if arguments["committed-only"] != "true" {
		t.Fatalf("derived committed-only = %q, want true", arguments["committed-only"])
	}
	if arguments["base-ref"] == status.Projection.BaseTree {
		t.Fatalf("derived base-ref=%q disclosed the tree object instead of the derived commit", arguments["base-ref"])
	}
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v7.schema.json"), output.Bytes())
}

// crissCrossReviewRepo builds a repository with two merge commits that share
// two merge bases, so `git merge-base --all HEAD origin/main` names more than
// one candidate and the committed-range derivation must fall back.
func crissCrossReviewRepo(t *testing.T) string {
	t.Helper()
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	runReviewCLIGit(t, repo, "checkout", "-q", "-b", "x")
	writeReviewStartCandidate(t, repo, "x.txt", "x\n", 0o644)
	runReviewCLIGit(t, repo, "add", "x.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "x")
	xCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	runReviewCLIGit(t, repo, "checkout", "-q", "-b", "y", base)
	writeReviewStartCandidate(t, repo, "y.txt", "y\n", 0o644)
	runReviewCLIGit(t, repo, "add", "y.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "y")
	yCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	runReviewCLIGit(t, repo, "checkout", "-q", "x")
	runReviewCLIGit(t, repo, "merge", "-q", "--no-ff", "-m", "merge y into x", yCommit)
	runReviewCLIGit(t, repo, "checkout", "-q", "y")
	runReviewCLIGit(t, repo, "merge", "-q", "--no-ff", "-m", "merge x into y", xCommit)
	origin := t.TempDir()
	runReviewCLIGit(t, origin, "init", "--bare", "-q")
	runReviewCLIGit(t, repo, "remote", "add", "origin", origin)
	runReviewCLIGit(t, repo, "push", "-q", "origin", "y:refs/heads/main")
	runReviewCLIGit(t, repo, "fetch", "-q", "origin")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runReviewCLIGit(t, repo, "checkout", "-q", "x")
	if merges := strings.Fields(runReviewCLIGit(t, repo, "merge-base", "--all", "HEAD", "refs/remotes/origin/main")); len(merges) != 2 {
		t.Fatalf("criss-cross fixture produced %d merge bases %q, want exactly two", len(merges), merges)
	}
	return repo
}

// TestNegotiatedStatusAmbiguousCommittedRangeFallsBackToCollect pins the
// retained fallback for every ambiguous repository shape: the transition stays
// today's external.select_base_ref collect, the envelope still validates, and no
// executable START is offered.
func TestNegotiatedStatusAmbiguousCommittedRangeFallsBackToCollect(t *testing.T) {
	shapes := []struct {
		name    string
		arrange func(t *testing.T) string
	}{
		{
			name: "no origin/HEAD",
			arrange: func(t *testing.T) string {
				return initReviewCLIRepo(t)
			},
		},
		{
			name: "empty committed range",
			arrange: func(t *testing.T) string {
				repo := initReviewCLIRepo(t)
				origin := t.TempDir()
				runReviewCLIGit(t, origin, "init", "--bare", "-q")
				runReviewCLIGit(t, repo, "remote", "add", "origin", origin)
				runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/main")
				runReviewCLIGit(t, repo, "fetch", "-q", "origin")
				runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
				return repo
			},
		},
		{
			name: "origin/HEAD names a missing branch",
			arrange: func(t *testing.T) string {
				repo := initReviewCLIRepo(t)
				runReviewCLIGit(t, repo, "remote", "add", "origin", t.TempDir())
				runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/does-not-exist")
				return repo
			},
		},
		{
			name:    "criss-cross merge bases",
			arrange: crissCrossReviewRepo,
		},
	}

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := shape.arrange(t)
			var output bytes.Buffer
			if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "claude-code", "--next-transition"}, &output); err != nil {
				t.Fatalf("selectorless STATUS for an ambiguous committed range: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
				status.NextTransition.ReasonCode != "empty_candidate_base_ref_required" {
				t.Fatalf("ambiguous committed range transition = %#v, want the empty_candidate_base_ref_required collect", status.NextTransition)
			}
			if status.NextTransition.Execute != nil {
				t.Fatalf("ambiguous committed range offered an executable START: %#v", status.NextTransition.Execute)
			}
			validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v7.schema.json"), output.Bytes())
		})
	}
}
