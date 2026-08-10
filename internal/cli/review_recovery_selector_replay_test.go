package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRecoveryAuthorizationCollectionPreservesNormalizedSelectors(t *testing.T) {
	tests := []struct {
		name     string
		recovery reviewtransaction.Target
		want     []ReviewTransitionArgument
	}{
		{name: "current workspace over staged predecessor", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace}, want: []ReviewTransitionArgument{{Name: "projection", Value: "workspace"}}},
		{name: "current changes without projection", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges}},
		{name: "current changes with invalid projection", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: "future"}},
		{name: "staged projection", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionStaged}, want: []ReviewTransitionArgument{{Name: "projection", Value: "staged"}}},
		{name: "base diff", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseDiff, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "main"}, want: []ReviewTransitionArgument{{Name: "base-ref", Value: "main"}, {Name: "committed-only", Value: "true"}}},
		{name: "workspace overlay", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "main"}, want: []ReviewTransitionArgument{{Name: "base-ref", Value: "main"}, {Name: "projection", Value: "workspace"}, {Name: "workspace-overlay", Value: "true"}}},
		{name: "staged workspace overlay", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionStaged, BaseRef: "main"}, want: []ReviewTransitionArgument{{Name: "base-ref", Value: "main"}, {Name: "projection", Value: "staged"}, {Name: "workspace-overlay", Value: "true"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.want == nil {
				if _, representable := (reviewTransitionSelector{Recovery: &test.recovery}).recoveryArguments(); representable {
					t.Fatalf("invalid recovery projection %q was representable", test.recovery.Projection)
				}
				return
			}
			status := recoverySelectorReplayStatus(t, test.recovery)
			input := reviewNextTransitionInput{Selector: &reviewTransitionSelector{Projection: reviewtransaction.ProjectionStaged, Recovery: &test.recovery}}
			collectedTransition := newReviewNextTransition(status, nil, nil, nil, nil, input)
			var collected ReviewNextTransition
			decodeStrictReviewJSON(t, []byte(mustReviewJSON(t, collectedTransition)), &collected)
			if collected.Kind != reviewNextTransitionCollect || collected.Collect == nil || len(collected.Collect.Inputs) != 1 || collected.Collect.Inputs[0].SelectorArguments == nil || !reflect.DeepEqual(*collected.Collect.Inputs[0].SelectorArguments, test.want) {
				t.Fatalf("recovery authorization transition = %#v, want selectors %#v", collected, test.want)
			}
			if test.name == "staged workspace overlay" {
				validateRecoverySelectorReplaySchemas(t, collectedTransition)
			}
			status.NextTransition = &collectedTransition
			if err := status.Validate(); err != nil {
				t.Fatalf("public STATUS rejected recovery authorization collect: %v", err)
			}
			for _, argument := range collected.Collect.Inputs[0].Arguments {
				if argument.Name == "base-ref" || argument.Name == "committed-only" || argument.Name == "projection" || argument.Name == "workspace-overlay" {
					t.Fatalf("selector %q leaked into canonical authorization arguments", argument.Name)
				}
			}

			const successor, actor, reason = "selector-replay-successor", "maintainer", "authorize exact recovery target"
			binding := reviewTransitionBinding(status.Authority, status.TargetIdentity)
			input.Successor, input.Actor, input.Reason = successor, actor, reason
			input.Authorization = reviewTransitionRecoveryAuthorization(binding, successor, actor, reason)
			authorizedTransition := newReviewNextTransition(status, nil, nil, nil, nil, input)
			var authorized ReviewNextTransition
			decodeStrictReviewJSON(t, []byte(mustReviewJSON(t, authorizedTransition)), &authorized)
			if authorized.Kind != reviewNextTransitionExecute || authorized.Execute == nil || authorized.Execute.SelectorArguments == nil || !reflect.DeepEqual(*authorized.Execute.SelectorArguments, test.want) {
				t.Fatalf("authorized recovery transition = %#v, want execute selectors %#v", authorized, test.want)
			}
			arguments, err := reviewTransitionArgumentMap(authorized.Execute.Arguments)
			if err != nil || arguments["maintainer-authorization"] != input.Authorization {
				t.Fatalf("selector replay changed the canonical recovery authorization bytes: %v", err)
			}
			status.NextTransition = &authorizedTransition
			if err := status.Validate(); err != nil {
				t.Fatalf("public STATUS rejected authorized selector replay: %v", err)
			}
		})
	}
}

func TestStatusAcceptsNormalizedOrdinaryWorkspaceOverlaySelectors(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "overlay\n", 0o644)
	status := selectorTransitionStatus(t, repo, "--base-ref", strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD")), "--projection", "workspace", "--workspace-overlay")
	if status.Projection.Kind != reviewtransaction.TargetBaseWorkspaceOverlay || status.Projection.Projection != reviewtransaction.ProjectionWorkspace {
		t.Fatalf("normalized ordinary overlay selectors resolved %#v", status.Projection)
	}
}

func recoverySelectorReplayStatus(t *testing.T, recovery reviewtransaction.Target) ReviewTargetStatusResult {
	t.Helper()
	const changedLines = 20
	authorityTarget := "sha256:" + strings.Repeat("0", 64)
	target := "sha256:" + strings.Repeat("1", 64)
	return ReviewTargetStatusResult{
		Schema: ReviewIntegrationStatusSchemaV2, Contract: ReviewIntegrationContractV1, Operation: "review.status",
		Applicability: reviewtransaction.TargetApplicabilityCurrent, Action: reviewtransaction.TargetStatusActionRecover,
		ActionDisposition: reviewtransaction.RecoveryInvalidated, Replayability: reviewtransaction.ReplayabilityManualActionRequired,
		TargetIdentity: target, AuthorityTargetIdentity: authorityTarget, Candidates: []string{},
		Authority: &ReviewTargetStatusAuthority{
			Version: reviewtransaction.AuthorityVersionCompact, LineageID: "selector-replay-source", Generation: 1,
			Revision: "sha256:" + strings.Repeat("2", 64), State: reviewtransaction.StateInvalidated,
		},
		Receipt: ReviewTargetStatusReceipt{Status: ReviewReceiptExpectedMissing},
		Frozen:  &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium, OriginalChangedLines: changedLines, CorrectionBudget: changedLines / 2},
		Repair:  reviewtransaction.UnsupportedAuthorityRepairAssessment(),
		Projection: ReviewTargetStatusProjection{
			Schema: ReviewIntegrationProjectionSchema, Kind: recovery.Kind, Projection: recovery.Projection,
			BaseTree: strings.Repeat("3", 40), InitialReviewTree: strings.Repeat("4", 40), CurrentCandidateTree: strings.Repeat("5", 40),
			PathsDigest: "sha256:" + strings.Repeat("6", 64), Paths: []string{"candidate.go"}, IntendedUntracked: []string{},
			IntendedUntrackedProof: "sha256:" + strings.Repeat("7", 64), InitialSnapshotIdentity: authorityTarget, CurrentSnapshotIdentity: target,
		},
	}
}

func validateRecoverySelectorReplaySchemas(t *testing.T, transition ReviewNextTransition) {
	t.Helper()
	payload := []byte(mustReviewJSON(t, transition))
	selector := &(*transition.Collect.Inputs[0].SelectorArguments)[0]
	selector.Token = "--base-ref=main"
	tokenized := []byte(mustReviewJSON(t, transition))
	selector.Token = ""
	if mustReviewJSON(t, transition) != string(payload) {
		t.Fatal("tokenized fixture changed more than one selector argument")
	}
	for _, schema := range []struct{ version, file string }{
		{version: "v1", file: "status.schema.json"},
		{version: "v1", file: "status-v2.schema.json"},
		{version: "v2", file: "status.schema.json"},
		{version: "v2", file: "status-v4.schema.json"},
		{version: "v2", file: "status-v5.schema.json"},
	} {
		t.Run(schema.version+"/"+schema.file, func(t *testing.T) {
			validateAgainstPublishedStatusNextTransitionSchema(t, schema.version, schema.file, payload)
			validateAgainstPublishedStatusNextTransitionSchema(t, schema.version, schema.file, tokenized, true)
		})
	}
}
