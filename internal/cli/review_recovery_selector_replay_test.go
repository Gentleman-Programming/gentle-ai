package cli

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRecoveryAuthorizationCollectionPreservesNormalizedSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		recovery reviewtransaction.Target
		want     []ReviewTransitionArgument
	}{
		{name: "current changes", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace}, want: []ReviewTransitionArgument{}},
		{name: "staged projection", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionStaged}, want: []ReviewTransitionArgument{{Name: "projection", Value: "staged"}}},
		{name: "base diff", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseDiff, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "main"}, want: []ReviewTransitionArgument{{Name: "base-ref", Value: "main"}, {Name: "committed-only", Value: "true"}}},
		{name: "workspace overlay", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "main"}, want: []ReviewTransitionArgument{{Name: "base-ref", Value: "main"}, {Name: "projection", Value: "workspace"}, {Name: "workspace-overlay", Value: "true"}}},
		{name: "staged workspace overlay", recovery: reviewtransaction.Target{Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionStaged, BaseRef: "main"}, want: []ReviewTransitionArgument{{Name: "base-ref", Value: "main"}, {Name: "projection", Value: "staged"}, {Name: "workspace-overlay", Value: "true"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := recoverySelectorReplayStatus(t, test.recovery)
			input := reviewNextTransitionInput{Selector: &reviewTransitionSelector{Recovery: &test.recovery}}
			collectedTransition := newReviewNextTransition(status, nil, nil, nil, nil, input)
			collected := decodeRecoverySelectorReplayTransition(t, collectedTransition)
			if collected.Kind != reviewNextTransitionCollect || collected.Collect == nil || len(collected.Collect.Inputs) != 1 {
				t.Fatalf("recovery authorization transition = %#v, want one collect input", collected)
			}
			selectors := collected.Collect.Inputs[0].SelectorArguments
			if selectors == nil || !reflect.DeepEqual(*selectors, test.want) {
				t.Fatalf("recovery authorization selectors = %#v, want %#v", selectors, test.want)
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
			authorized := decodeRecoverySelectorReplayTransition(t, authorizedTransition)
			if authorized.Kind != reviewNextTransitionExecute || authorized.Execute == nil {
				t.Fatalf("authorized recovery transition = %#v, want execute", authorized)
			}
			if authorized.Execute.SelectorArguments == nil || !reflect.DeepEqual(*authorized.Execute.SelectorArguments, test.want) {
				t.Fatalf("authorized recovery selectors = %#v, want %#v", authorized.Execute.SelectorArguments, test.want)
			}
			arguments, err := reviewTransitionArgumentMap(authorized.Execute.Arguments)
			if err != nil {
				t.Fatal(err)
			}
			if arguments["maintainer-authorization"] != input.Authorization {
				t.Fatal("selector replay changed the canonical recovery authorization bytes")
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
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "tracked.txt", "overlay\n", 0o644)
	status := selectorTransitionStatus(t, repo, "--base-ref", base, "--projection", "workspace", "--workspace-overlay")
	if status.Projection.Kind != reviewtransaction.TargetBaseWorkspaceOverlay || status.Projection.Projection != reviewtransaction.ProjectionWorkspace {
		t.Fatalf("normalized ordinary overlay selectors resolved %#v", status.Projection)
	}
}

func TestReviewRecoverAcceptsNormalizedOrdinaryWorkspaceOverlaySelectors(t *testing.T) {
	repo, predecessor := approvedWorkspaceOverlayRecoveryPredecessor(t, "selector-replay-recover-source")
	writeReviewStartCandidate(t, repo, "new.txt", "expanded overlay\n", 0o644)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^"))
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "selector-replay-recover-successor",
		"--disposition", "scope_changed", "--base-ref", base, "--projection", "workspace", "--workspace-overlay",
	}, io.Discard)
	if err != nil {
		t.Fatalf("normalized ordinary overlay RECOVER: %v", err)
	}
}

func recoverySelectorReplayStatus(t *testing.T, recovery reviewtransaction.Target) ReviewTargetStatusResult {
	t.Helper()
	const changedLines = 20
	budget, err := reviewtransaction.CorrectionBudget(changedLines)
	if err != nil {
		t.Fatal(err)
	}
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
		Frozen:  &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium, OriginalChangedLines: changedLines, CorrectionBudget: budget},
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
	marshal := func(transition ReviewNextTransition) []byte {
		payload, err := json.Marshal(transition)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	payload := marshal(transition)
	selector := &(*transition.Collect.Inputs[0].SelectorArguments)[0]
	selector.Token = "--base-ref=main"
	tokenized := marshal(transition)
	selector.Token = ""
	if string(marshal(transition)) != string(payload) {
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

func decodeRecoverySelectorReplayTransition(t *testing.T, transition ReviewNextTransition) ReviewNextTransition {
	t.Helper()
	payload, err := json.Marshal(transition)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ReviewNextTransition
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}
