package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestCorrectionPlanStatusIsStableAcrossRestart(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "correction-routing")
	legacy := ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity,
		SelectedLenses: started.SelectedLenses,
	}
	for order := range legacy.SelectedLenses {
		findings := []facadeFinding{}
		if order == len(legacy.SelectedLenses)-1 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate value is wrong",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, legacy, order, findings, &bytes.Buffer{})
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", started.LineageID}
	var first, replay bytes.Buffer
	if err := RunReview(args, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(args, &replay); err != nil {
		t.Fatal(err)
	}
	if first.String() != replay.String() {
		t.Fatalf("correction-plan STATUS changed after restart:\n%s\n%s", first.String(), replay.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, first.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "correction_plan_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureCorrectionPlanOperation {
		t.Fatalf("correction-plan STATUS = %#v", status.NextTransition)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("read-only correction-plan STATUS mutated authority: %v", err)
	}
}

func TestNegotiatedRestartStatusSuppliesFrozenContextForEveryMissingReviewer(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, true)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != len(record.State.SelectedLenses) {
		t.Fatalf("restart transition = %#v", status.NextTransition)
	}
	wantContext, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	for order, input := range status.NextTransition.Collect.Inputs {
		payload, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"artifact_subject", "base_tree", "candidate_tree", "changed_path_manifest"} {
			if len(document[field]) == 0 {
				t.Fatalf("restart reviewer input %d omits %q: %s", order, field, payload)
			}
		}
		var subject reviewtransaction.ArtifactSubject
		var manifest []reviewtransaction.ChangedPathManifestEntry
		if err := json.Unmarshal(document["artifact_subject"], &subject); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(document["changed_path_manifest"], &manifest); err != nil {
			t.Fatal(err)
		}
		if subject.LineageID != record.State.LineageID || subject.AuthorityRevision != record.State.CapturePhaseRevision ||
			subject.TargetIdentity != record.State.InitialSnapshot.Identity || subject.Lens != record.State.SelectedLenses[order] ||
			subject.SelectedOrder != order || subject.BaseTree != wantContext.BaseTree || subject.CandidateTree != wantContext.CandidateTree {
			t.Fatalf("restart subject %d = %#v", order, subject)
		}
		if input.BaseTree != wantContext.BaseTree || input.CandidateTree != wantContext.CandidateTree || !reflect.DeepEqual(manifest, wantContext.ChangedPathManifest) {
			t.Fatalf("restart context %d differs from frozen candidate\ngot trees=%s..%s manifest=%#v\nwant trees=%s..%s manifest=%#v", order, input.BaseTree, input.CandidateTree, manifest, wantContext.BaseTree, wantContext.CandidateTree, wantContext.ChangedPathManifest)
		}
	}
}

func TestReviewNextTransitionStateTable(t *testing.T) {
	t.Parallel()

	status := func(applicability reviewtransaction.TargetApplicability, state reviewtransaction.State, action reviewtransaction.TargetStatusAction, replayability reviewtransaction.Replayability) ReviewTargetStatusResult {
		return ReviewTargetStatusResult{
			Applicability: applicability, Action: action, Replayability: replayability,
			TargetIdentity: "sha256:" + strings.Repeat("b", 64), Candidates: []string{"first", "second"},
			Authority:  &ReviewTargetStatusAuthority{LineageID: "review-next-transition", Revision: "sha256:" + strings.Repeat("a", 64), State: state},
			Frozen:     &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
			Projection: ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
		}
	}
	for _, tt := range []struct {
		name          string
		status        ReviewTargetStatusResult
		lenses        []string
		artifacts     []ReviewTransitionArtifact
		wantKind      string
		wantOperation string
	}{
		{"unreviewed workspace", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"unreviewed staged", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"unreviewed base ref", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"unreviewed overlay", status(reviewtransaction.TargetApplicabilityUnrelated, "", reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionExecute, "review.start"},
		{"reviewing low partial", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, "", reviewtransaction.ReplayabilityNotReplayable), []string{reviewtransaction.LensReliability}, nil, reviewNextTransitionCollect, ""},
		{"reviewing all captured", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, "", reviewtransaction.ReplayabilityNotReplayable), []string{reviewtransaction.LensReliability}, []ReviewTransitionArtifact{{}}, reviewNextTransitionStop, ""},
		{"reviewing high partial", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, "", reviewtransaction.ReplayabilityNotReplayable), []string{reviewtransaction.LensReliability}, nil, reviewNextTransitionCollect, ""},
		{"correction required without provider request", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateCorrectionRequired, "", reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionStop, ""},
		{"unchanged corrected authority", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateCorrectionRequired, reviewtransaction.TargetStatusActionStop, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionStop, ""},
		{"validating", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateValidating, "", reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionStop, ""},
		{"reviewing without selected lenses is terminal", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateReviewing, "", reviewtransaction.ReplayabilityStatusRequired), nil, nil, reviewNextTransitionStop, ""},
		{"approved authority is terminal", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateApproved, reviewtransaction.TargetStatusActionValidate, reviewtransaction.ReplayabilityNotReplayable), nil, nil, reviewNextTransitionStop, ""},
		{"invalidated", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateInvalidated, reviewtransaction.TargetStatusActionRecover, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionExecute, "review.recover"},
		{"escalated unchanged", status(reviewtransaction.TargetApplicabilityCurrent, reviewtransaction.StateEscalated, reviewtransaction.TargetStatusActionStop, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionStop, ""},
		{"ambiguous", status(reviewtransaction.TargetApplicabilityAmbiguous, "", reviewtransaction.TargetStatusActionSelectLineage, reviewtransaction.ReplayabilityStatusRequired), nil, nil, reviewNextTransitionCollect, ""},
		{"corrupt", status(reviewtransaction.TargetApplicabilityCorrupted, "", reviewtransaction.TargetStatusActionRepairAuthority, reviewtransaction.ReplayabilityManualActionRequired), nil, nil, reviewNextTransitionStop, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := reviewNextTransitionInput{}
			if tt.status.Authority != nil && tt.status.Authority.State == reviewtransaction.StateReviewing {
				input.RepositoryContext = "rctx1_" + strings.Repeat("d", 64)
				input.CaptureContext = nextTransitionTestCaptureContext(t, tt.status, tt.lenses)
			}
			if tt.status.Action == reviewtransaction.TargetStatusActionRecover {
				input = reviewNextTransitionInput{Successor: "review-next-successor", Reason: "authorized recovery", Actor: "maintainer"}
				input.Authorization = "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + tt.status.Authority.LineageID + "\npredecessor_revision=" + tt.status.Authority.Revision + "\ntarget_identity=" + tt.status.TargetIdentity + "\nactor=" + input.Actor + "\nreason=" + input.Reason
			}
			tt.status.repositoryRoot = "/review-next-transition-repo"
			got := newReviewNextTransition(tt.status, tt.lenses, tt.artifacts, nil, input)
			if tt.wantKind == "" {
				if got != (ReviewNextTransition{}) {
					t.Fatalf("approved status transition = %#v, want omitted", got)
				}
				return
			}
			if got.Kind != tt.wantKind || got.Execute != nil && got.Execute.Operation != tt.wantOperation {
				t.Fatalf("next transition = %#v", got)
			}
			if err := got.Validate(); err != nil {
				t.Fatal(err)
			}
			if got.Kind == reviewNextTransitionStop && (got.Execute != nil || got.Collect != nil) {
				t.Fatalf("stop exposed a command or template: %#v", got)
			}
		})
	}
}

func TestReviewNextTransitionDefaultWorkspaceOverlayCollectsTargetBeforeAuthorization(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:           reviewtransaction.TargetApplicabilityCurrent,
		Action:                  reviewtransaction.TargetStatusActionRecover,
		ActionDisposition:       reviewtransaction.RecoveryEscalated,
		TargetIdentity:          "sha256:" + strings.Repeat("b", 64),
		AuthorityTargetIdentity: "sha256:" + strings.Repeat("a", 64),
		Authority: &ReviewTargetStatusAuthority{
			LineageID: "review-overlay", Revision: "sha256:" + strings.Repeat("c", 64), State: reviewtransaction.StateEscalated,
		},
		Projection: ReviewTargetStatusProjection{
			Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionWorkspace,
		},
	}
	input := reviewNextTransitionInput{
		Successor: "review-overlay-successor", Reason: "recover workspace overlay", Actor: "maintainer",
		Selector: &reviewTransitionSelector{
			Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "main",
			Recovery: &reviewtransaction.Target{Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "main"},
		},
	}
	binding := reviewTransitionBinding(status.Authority, status.TargetIdentity, "")
	input.Authorization = reviewTransitionRecoveryAuthorization(binding, input.Successor, input.Actor, input.Reason)

	status.repositoryRoot = "/review-next-transition-repo"

	got := newReviewNextTransition(status, nil, nil, nil, input)
	if got.Kind != reviewNextTransitionCollect || got.ReasonCode != "recovery_target_unrepresentable" ||
		got.Collect == nil || len(got.Collect.Inputs) != 1 || got.Collect.Inputs[0].Name != "recovery_target_selector" {
		t.Fatalf("default workspace-overlay recovery transition = %#v, want target selection before authorization", got)
	}
}

// TestReviewTransitionArgumentToken verifies that executable transition
// arguments remain literal argv tokens while preconditions remain assertions.
func TestReviewTransitionArgumentToken(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		status     ReviewTargetStatusResult
		input      reviewNextTransitionInput
		wantTokens map[string]string
	}{
		{
			name: "fresh start token",
			status: ReviewTargetStatusResult{
				Applicability:  reviewtransaction.TargetApplicabilityUnrelated,
				TargetIdentity: "sha256:" + strings.Repeat("b", 64),
				Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace},
			},
			input:      reviewNextTransitionInput{StartLineage: "review-token-start"},
			wantTokens: map[string]string{"target": "--target=sha256:" + strings.Repeat("b", 64), "lineage": "--lineage=review-token-start"},
		},
		{
			name: "recovery token",
			status: ReviewTargetStatusResult{
				Applicability:  reviewtransaction.TargetApplicabilityCurrent,
				Action:         reviewtransaction.TargetStatusActionRecover,
				TargetIdentity: "sha256:" + strings.Repeat("b", 64),
				Authority:      &ReviewTargetStatusAuthority{LineageID: "review-token", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateInvalidated},
				Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
				Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace},
			},
			input: reviewNextTransitionInput{
				Successor: "review-token-successor", Reason: "authorized recovery", Actor: "maintainer",
				Authorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=review-token\npredecessor_revision=sha256:" + strings.Repeat("a", 64) + "\ntarget_identity=sha256:" + strings.Repeat("b", 64) + "\nactor=maintainer\nreason=authorized recovery",
			},
			wantTokens: map[string]string{"predecessor-lineage": "--predecessor-lineage=review-token", "successor-lineage": "--successor-lineage=review-token-successor"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.status.repositoryRoot = "/review-next-transition-repo"
			got := newReviewNextTransition(tt.status, nil, nil, nil, tt.input)
			if got.Kind != reviewNextTransitionExecute || got.Execute == nil {
				t.Fatalf("next transition = %#v, want an execute transition", got)
			}
			seen := map[string]bool{}
			for _, argument := range got.Execute.Arguments {
				if want, checked := tt.wantTokens[argument.Name]; checked {
					if argument.Token != want {
						t.Fatalf("argument %q token = %q, want %q", argument.Name, argument.Token, want)
					}
					seen[argument.Name] = true
				}
				if strings.TrimSpace(argument.Token) == "" {
					t.Fatalf("execute argument %q carries no literal argv token", argument.Name)
				}
			}
			for name := range tt.wantTokens {
				if !seen[name] {
					t.Fatalf("expected argument %q was not present in execute arguments", name)
				}
			}
			for _, precondition := range got.Execute.Preconditions {
				if precondition.Token != "" {
					t.Fatalf("precondition %q carries an argv token", precondition.Name)
				}
			}
		})
	}
}

func TestReviewCaptureTransitionArgumentToken(t *testing.T) {
	t.Parallel()

	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-capture", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace},
	}
	lenses := []string{reviewtransaction.LensReliability}
	status.repositoryRoot = "/review-next-transition-repo"
	got := newReviewNextTransition(status, lenses, nil, nil, reviewNextTransitionInput{
		CaptureContext: nextTransitionTestCaptureContext(t, status, lenses),
	})
	if got.Kind != reviewNextTransitionCollect || got.Collect == nil || len(got.Collect.Inputs) != 1 {
		t.Fatalf("next transition = %#v, want one capture input", got)
	}
	for _, argument := range got.Collect.Inputs[0].Arguments {
		if strings.TrimSpace(argument.Token) == "" {
			t.Fatalf("capture argument %q carries no literal argv token", argument.Name)
		}
	}
}

// TestNewReviewNextTransitionEscalatedRouting is the RED-first proof for
// 1800 (StateEscalated used to dead-end with Stop("escalated_authority")
// unconditionally, unlike StateInvalidated which routes to recovery) plus the
// organic-dx stop-invariant sweep's follow-up fix for the "third case" 1800
// left unsoftened: native STATUS (target_status.go:176-199) only ever sets
// Action == TargetStatusActionRecover for StateEscalated when either the
// target changed OR the authority is an accounting-only escalation eligible
// for RecoverCompactAuthority's record-derived edge (issue found while
// investigating "escalated_recovery_requires_changed_target" as a Phase 3
// SUSPECT stop code) — TargetStatusActionStop is the only other outcome, and
// it is routed away to native_stop_required before this switch is ever
// reached (see the TargetStatusActionStop branch above). So this switch case
// can trust status.Action unconditionally, exactly like every other case:
// StateEscalated always routes to reviewRecoveryCollection with the
// disposition forced to RecoveryEscalated, regardless of whether the target
// changed. When a Selector is present and the target has not changed, the
// generic recovery_scope_unchanged guard already inside reviewRecoveryCollection
// still applies — that guard is unrelated to StateEscalated and is not
// softened by this fix.
func TestNewReviewNextTransitionEscalatedRouting(t *testing.T) {
	t.Parallel()

	baseStatus := func(target, authorityTarget string) ReviewTargetStatusResult {
		return ReviewTargetStatusResult{
			Applicability: reviewtransaction.TargetApplicabilityCurrent, Action: reviewtransaction.TargetStatusActionRecover,
			Replayability:           reviewtransaction.ReplayabilityManualActionRequired,
			TargetIdentity:          target,
			AuthorityTargetIdentity: authorityTarget,
			Authority:               &ReviewTargetStatusAuthority{LineageID: "review-escalated", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateEscalated},
			Frozen:                  &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
			Projection:              ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
		}
	}
	unchangedTarget := "sha256:" + strings.Repeat("b", 64)
	changedTarget := "sha256:" + strings.Repeat("e", 64)

	t.Run("changed target routes to recovery with disposition forced to escalated", func(t *testing.T) {
		status := baseStatus(changedTarget, unchangedTarget)
		input := reviewNextTransitionInput{
			Successor: "review-escalated-successor", Reason: "authorized recovery", Actor: "maintainer",
			Authorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=review-escalated\npredecessor_revision=sha256:" + strings.Repeat("a", 64) + "\ntarget_identity=" + changedTarget + "\nactor=maintainer\nreason=authorized recovery",
		}
		status.repositoryRoot = "/review-next-transition-repo"
		got := newReviewNextTransition(status, nil, nil, nil, input)
		if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.recover" {
			t.Fatalf("escalated changed-target transition = %#v, want an execute review.recover transition", got)
		}
		arguments, err := reviewTransitionArgumentMap(got.Execute.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		if arguments["disposition"] != string(reviewtransaction.RecoveryEscalated) {
			t.Fatalf("escalated recovery disposition = %q, want %q", arguments["disposition"], reviewtransaction.RecoveryEscalated)
		}
		if err := got.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unchanged target without a selector still routes to recovery (accounting-only edge)", func(t *testing.T) {
		status := baseStatus(unchangedTarget, unchangedTarget)
		input := reviewNextTransitionInput{
			Successor: "review-escalated-successor", Reason: "authorized recovery", Actor: "maintainer",
			Authorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=review-escalated\npredecessor_revision=sha256:" + strings.Repeat("a", 64) + "\ntarget_identity=" + unchangedTarget + "\nactor=maintainer\nreason=authorized recovery",
		}
		status.repositoryRoot = "/review-next-transition-repo"
		got := newReviewNextTransition(status, nil, nil, nil, input)
		if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.recover" {
			t.Fatalf("escalated unchanged-target transition (no selector) = %#v, want an execute review.recover transition — status.Action already vetted this as legal (accounting-only escalation), so this switch must not re-derive a target-changed requirement", got)
		}
		arguments, err := reviewTransitionArgumentMap(got.Execute.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		if arguments["disposition"] != string(reviewtransaction.RecoveryEscalated) {
			t.Fatalf("escalated recovery disposition = %q, want %q", arguments["disposition"], reviewtransaction.RecoveryEscalated)
		}
	})

	t.Run("unchanged target with a selector still stops via the generic recovery_scope_unchanged guard", func(t *testing.T) {
		status := baseStatus(unchangedTarget, unchangedTarget)
		input := reviewNextTransitionInput{Selector: &reviewTransitionSelector{Recovery: &reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges}}}
		status.repositoryRoot = "/review-next-transition-repo"
		got := newReviewNextTransition(status, nil, nil, nil, input)
		if got.Kind != reviewNextTransitionStop || got.Execute != nil || got.Collect != nil {
			t.Fatalf("escalated unchanged-target transition (selector) = %#v, want a bare stop", got)
		}
		if got.ReasonCode != "recovery_scope_unchanged" {
			t.Fatalf("escalated unchanged-target reason (selector) = %q, want the generic reviewRecoveryCollection guard reason %q", got.ReasonCode, "recovery_scope_unchanged")
		}
	})
}

// TestReviewNextTransitionCollectArgumentsValidateAgainstPublishedSchema
// keeps native reviewer-capture argv tokens valid under the published v1
// transition schema.
func TestReviewNextTransitionCollectArgumentsValidateAgainstPublishedSchema(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-schema", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
	}
	lenses := []string{reviewtransaction.LensReliability}
	status.repositoryRoot = "/review-next-transition-repo"
	got := newReviewNextTransition(status, lenses, nil, nil, reviewNextTransitionInput{
		CaptureContext: nextTransitionTestCaptureContext(t, status, lenses),
	})
	if got.Kind != reviewNextTransitionCollect || got.Collect == nil || len(got.Collect.Inputs) != 1 ||
		got.Collect.Inputs[0].CaptureOperation != reviewCaptureResultCaptureOperation {
		t.Fatalf("next transition = %#v, want one reviewer capture", got)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, payload)
}

func TestReviewNextTransitionCollectContextValidatesAgainstPublishedSchema(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-context-schema", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
	}
	lenses := []string{reviewtransaction.LensReliability}
	status.repositoryRoot = "/review-next-transition-repo"
	got := newReviewNextTransition(status, lenses, nil, nil, reviewNextTransitionInput{
		CaptureContext: nextTransitionTestCaptureContext(t, status, lenses),
	})
	if got.Collect == nil || len(got.Collect.Inputs) != 1 || got.Collect.Inputs[0].ArtifactSubject == nil ||
		got.Collect.Inputs[0].ChangedPathManifest == nil {
		t.Fatalf("capture context = %#v", got)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, payload)
}

// TestReviewNextTransitionRecoverySelectorArgumentsValidateAgainstPublishedSchema
// keeps the published schema bound to an executable authorized recovery.
func TestReviewNextTransitionRecoverySelectorArgumentsValidateAgainstPublishedSchema(t *testing.T) {
	t.Run("authorized recovery selector executes and validates", func(t *testing.T) {
		status := ReviewTargetStatusResult{
			Applicability:           reviewtransaction.TargetApplicabilityCurrent,
			Action:                  reviewtransaction.TargetStatusActionRecover,
			ActionDisposition:       reviewtransaction.RecoveryScopeChanged,
			Replayability:           reviewtransaction.ReplayabilityNotReplayable,
			TargetIdentity:          "sha256:" + strings.Repeat("b", 64),
			AuthorityTargetIdentity: "sha256:" + strings.Repeat("c", 64),
			Authority:               &ReviewTargetStatusAuthority{LineageID: "review-selector-schema", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateApproved},
			Frozen:                  &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
			Projection:              ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, Kind: reviewtransaction.TargetBaseDiff, BaseTree: strings.Repeat("d", 40), CurrentCandidateTree: strings.Repeat("e", 40)},
		}
		input := reviewNextTransitionInput{
			Gate:      reviewtransaction.GatePrePR,
			Successor: "review-selector-successor",
			Reason:    "expand approved scope",
			Actor:     "maintainer",
			Selector: &reviewTransitionSelector{
				Kind: reviewtransaction.TargetBaseDiff, BaseRef: "main", PrePRRepresentable: true,
				Recovery: &reviewtransaction.Target{Kind: reviewtransaction.TargetBaseDiff, BaseRef: "main", Projection: reviewtransaction.ProjectionWorkspace},
			},
		}
		binding := reviewTransitionBinding(status.Authority, status.TargetIdentity, "")
		input.Authorization = reviewTransitionRecoveryAuthorization(binding, input.Successor, input.Actor, input.Reason)
		status.repositoryRoot = "/review-next-transition-repo"
		got := newReviewNextTransition(status, nil, nil, nil, input)
		wantSelectors := []ReviewTransitionArgument{{Name: "base-ref", Value: "main"}, {Name: "committed-only", Value: "true"}}
		if got.Kind != reviewNextTransitionExecute || got.Execute == nil || got.Execute.Operation != "review.recover" ||
			got.Execute.SelectorArguments == nil || !reflect.DeepEqual(*got.Execute.SelectorArguments, wantSelectors) {
			t.Fatalf("next transition = %#v, want an executable recovery with selectors %#v", got, wantSelectors)
		}
		payload, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		validateAgainstPublishedNextTransitionSchema(t, payload)
	})
}

// TestReviewNextTransitionCollectArgumentsValidateAgainstPublishedV2Schema
// pins native reviewer-capture arguments to the v2 transition schema.
func TestReviewNextTransitionCollectArgumentsValidateAgainstPublishedV2Schema(t *testing.T) {
	status := ReviewTargetStatusResult{
		Contract:       ReviewIntegrationContractV2,
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Authority:      &ReviewTargetStatusAuthority{LineageID: "review-schema-v2", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		Frozen:         &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskMedium},
		Projection:     ReviewTargetStatusProjection{Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("c", 40), CurrentCandidateTree: strings.Repeat("d", 40)},
	}
	lenses := []string{reviewtransaction.LensReliability}
	status.repositoryRoot = "/review-next-transition-repo"
	got := newReviewNextTransition(status, lenses, nil, nil, reviewNextTransitionInput{
		CaptureContext: nextTransitionTestCaptureContext(t, status, lenses),
	})
	if got.Kind != reviewNextTransitionCollect || got.Collect == nil || len(got.Collect.Inputs) != 1 {
		t.Fatalf("next transition = %#v, want one reviewer capture", got)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, payload)
}

// validateAgainstPublishedNextTransitionSchema validates payload against the
// live $defs/next_transition subtree read straight out of the shipped
// contracts/review-integration/v1/schemas/status.schema.json, so the
// assertion stays honest as that file evolves. It deliberately compiles a
// synthetic root document ($defs/next_transition's own keys merged with the
// file's real, unmodified $defs) instead of status.schema.json's top-level
// object schema: that top-level schema's unrelated "projection" property
// resolves contracts/review-integration/v1/schemas/projection.schema.json,
// whose $defs/paths/items pattern uses a negative-lookahead regex Go's RE2
// engine cannot compile, which would fail metaschema validation for a
// reason wholly unrelated to what this test checks.
func validateAgainstPublishedNextTransitionSchema(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v1", "status.schema.json", payload)
}

// validateAgainstPublishedNextTransitionSchemaV2 performs the identical
// live-schema validation as validateAgainstPublishedNextTransitionSchema, but
// against contracts/review-integration/v1/schemas/status-v2.schema.json's own
// $defs/next_transition subtree, so the v2 wire shape stays pinned too.
func validateAgainstPublishedNextTransitionSchemaV2(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v1", "status-v2.schema.json", payload)
}

func validateAgainstPublishedNextTransitionSchemaV4(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v2", "status-v4.schema.json", payload)
}

func validateAgainstPublishedNextTransitionSchemaV5(t *testing.T, payload []byte) {
	t.Helper()
	validateAgainstPublishedStatusNextTransitionSchema(t, "v2", "status-v5.schema.json", payload)
}

// validateAgainstPublishedStatusNextTransitionSchema is the shared engine
// behind both the v1 and v2 published-schema validators above. It registers
// every schema file that either version's $defs/next_transition subtree can
// $ref (targeted-validation-request.schema.json for both; artifact-subject
// .schema.json and start-v2.schema.json for v2's $defs/transition_input),
// deliberately excluding the top-level schema's "projection" property so
// compiling never touches projection.schema.json's RE2-incompatible
// negative-lookahead regex (see the comment above the v1 wrapper).
func validateAgainstPublishedStatusNextTransitionSchema(t *testing.T, version, schemaFile string, payload []byte) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration"))
	if err != nil {
		t.Fatal(err)
	}
	statusSchemaBytes, err := os.ReadFile(filepath.Join(root, version, "schemas", schemaFile))
	if err != nil {
		t.Fatal(err)
	}
	var statusSchema map[string]any
	if err := json.Unmarshal(statusSchemaBytes, &statusSchema); err != nil {
		t.Fatal(err)
	}
	defs, ok := statusSchema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no $defs object: %#v", schemaFile, statusSchema["$defs"])
	}
	nextTransition, ok := defs["next_transition"].(map[string]any)
	if !ok {
		t.Fatalf("%s $defs.next_transition is missing or not an object: %#v", schemaFile, defs["next_transition"])
	}

	location := "https://gentle-ai.dev/contracts/review-integration/" + version + "/schemas/_test-next-transition.schema.json"
	synthetic := map[string]any{"$schema": statusSchema["$schema"], "$id": location, "$defs": defs}
	for key, value := range nextTransition {
		synthetic[key] = value
	}

	compiler := jsonschema.NewCompiler()
	resources := []struct{ version, name string }{
		{"v1", "status-v2.schema.json"},
		{"v1", "transition-execution.schema.json"},
		{"v1", "targeted-validation-request.schema.json"},
		{"v1", "correction-plan-request.schema.json"},
		{"v1", "artifact-subject.schema.json"},
		{"v1", "start-v2.schema.json"},
	}
	if version == "v2" {
		resources = append(resources,
			struct{ version, name string }{"v2", "artifact-subject.schema.json"},
			struct{ version, name string }{"v2", "start.schema.json"},
			struct{ version, name string }{"v2", "transition-binding.schema.json"},
			struct{ version, name string }{"v2", "transition-execution.schema.json"},
		)
	}
	for _, resource := range resources {
		refBytes, err := os.ReadFile(filepath.Join(root, resource.version, "schemas", resource.name))
		if err != nil {
			t.Fatal(err)
		}
		var refSchema any
		if err := json.Unmarshal(refBytes, &refSchema); err != nil {
			t.Fatal(err)
		}
		if resource.version == "v1" && resource.name == "status-v2.schema.json" {
			document := refSchema.(map[string]any)
			refSchema = map[string]any{"$schema": document["$schema"], "$id": document["$id"], "$defs": document["$defs"]}
		}
		if err := compiler.AddResource("https://gentle-ai.dev/contracts/review-integration/"+resource.version+"/schemas/"+resource.name, refSchema); err != nil {
			t.Fatal(err)
		}
	}
	if err := compiler.AddResource(location, synthetic); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("published next_transition schema (%s) rejected the emitted transition: %v", schemaFile, err)
	}
}

func nextTransitionTestCaptureContext(t *testing.T, status ReviewTargetStatusResult, lenses []string) *reviewCaptureContext {
	t.Helper()
	baseTree, candidateTree := strings.Repeat("c", 40), strings.Repeat("d", 40)
	frozen := reviewtransaction.FrozenCandidateContext{
		BaseTree: baseTree, CandidateTree: candidateTree,
		ChangedPathManifest: []reviewtransaction.ChangedPathManifestEntry{{
			Path: "tracked.txt", Status: reviewtransaction.CandidatePathModified, OldMode: "100644", NewMode: "100644",
		}},
	}
	state := reviewtransaction.CompactState{
		LineageID: status.Authority.LineageID,
		InitialSnapshot: reviewtransaction.Snapshot{
			Identity: status.TargetIdentity, BaseTree: baseTree, CandidateTree: candidateTree, Paths: []string{"tracked.txt"},
		},
		SelectedLenses: append([]string{}, lenses...),
	}
	context, err := newReviewCaptureContext(state, status.Authority.Revision, frozen)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func TestReviewNextTransitionRefusesTargetDriftAndUnverifiableCaptures(t *testing.T) {
	status := ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Authority:      &ReviewTargetStatusAuthority{LineageID: "target-drift", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateReviewing},
		TargetIdentity: "sha256:" + strings.Repeat("b", 64), Frozen: &ReviewTargetStatusFrozen{Tier: reviewtransaction.RiskHigh},
	}
	status.repositoryRoot = "/review-next-transition-repo"
	got := newReviewNextTransition(status, []string{reviewtransaction.LensRisk}, nil, errors.New("tampered capture"), reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionStop || got.ReasonCode != "captured_artifacts_unverifiable" || got.Execute != nil || got.Collect != nil {
		t.Fatalf("target drift transition = %#v", got)
	}
}

func TestReviewForecastMirrorsExactlyOneNextTransition(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  ReviewTargetStatusResult
		horizon ReviewForecastHorizon
	}{
		{name: "stop", status: ReviewTargetStatusResult{Applicability: reviewtransaction.TargetApplicabilityCorrupted}, horizon: ForecastHorizonTerminal},
		{name: "execute", status: ReviewTargetStatusResult{Applicability: reviewtransaction.TargetApplicabilityUnrelated, TargetIdentity: "sha256:" + strings.Repeat("a", 64)}, horizon: ForecastHorizonPartial},
	} {
		t.Run(tt.name, func(t *testing.T) {
			head := newReviewNextTransition(tt.status, nil, nil, nil, reviewNextTransitionInput{})
			forecast := newReviewForecast(head)
			if forecast.Horizon != tt.horizon || len(forecast.Steps) != 1 {
				t.Fatalf("forecast = %#v, want one %s step", forecast, tt.horizon)
			}
			step := forecast.Steps[0]
			if step.Step != 1 || step.Kind != head.Kind || step.ReasonCode != head.ReasonCode || strings.TrimSpace(step.Description) == "" {
				t.Fatalf("forecast step = %#v, head = %#v", step, head)
			}
		})
	}
}

func TestReviewStatusValidateRejectsMalformedForecast(t *testing.T) {
	item := ReviewForecastItem{Step: 1, Kind: "stop", ReasonCode: "corrupted_or_unverifiable_authority", Description: "desc"}
	tests := []struct {
		name, wantErr string
		forecast      ReviewForecast
		next          bool
	}{
		{"missing next", "forecast without next_transition is invalid", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{item}}, false},
		{"invalid horizon", `invalid forecast horizon "invalid_horizon"`, ReviewForecast{Horizon: "invalid_horizon", Steps: []ReviewForecastItem{item}}, true},
		{"complete horizon", `invalid forecast horizon "complete"`, ReviewForecast{Horizon: "complete", Steps: []ReviewForecastItem{item}}, true},
		{"multiple steps", "forecast must contain exactly one step", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{item, item}}, true},
		{"step is not one", "forecast step must be 1, got 2", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 2, Kind: item.Kind, ReasonCode: item.ReasonCode, Description: item.Description}}}, true},
		{"invalid kind", `forecast step 1 has invalid kind "invalid_kind"`, ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: "invalid_kind", ReasonCode: item.ReasonCode, Description: item.Description}}}, true},
		{"empty reason", "forecast step 1 has empty reason_code", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: item.Kind, Description: item.Description}}}, true},
		{"empty description", "forecast step 1 has empty description", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: item.Kind, ReasonCode: item.ReasonCode}}}, true},
		{"head divergence", "forecast head (execute/corrupted_or_unverifiable_authority) diverges", ReviewForecast{Horizon: ForecastHorizonTerminal, Steps: []ReviewForecastItem{{Step: 1, Kind: "execute", ReasonCode: item.ReasonCode, Description: item.Description}}}, true},
		{"stop partial", "stop transition requires a terminal forecast", ReviewForecast{Horizon: ForecastHorizonPartial, Steps: []ReviewForecastItem{item}}, true},
	}
	status := newReviewTargetStatusResultForContract(reviewtransaction.TargetStatusResult{Applicability: reviewtransaction.TargetApplicabilityCorrupted, AuthorityVersion: reviewtransaction.AuthorityVersionCompact, Action: reviewtransaction.TargetStatusActionStop, Replayability: reviewtransaction.ReplayabilityNotReplayable}, ReviewIntegrationContractV2)
	status.TargetIdentity = "sha256:" + strings.Repeat("a", 64)
	status.Projection = ReviewTargetStatusProjection{Schema: ReviewIntegrationProjectionSchema, Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("a", 40), InitialReviewTree: strings.Repeat("b", 40), CurrentCandidateTree: strings.Repeat("c", 40), PathsDigest: "sha256:" + strings.Repeat("a", 64), IntendedUntrackedProof: "sha256:" + strings.Repeat("b", 64), InitialSnapshotIdentity: status.TargetIdentity, CurrentSnapshotIdentity: status.TargetIdentity, Paths: []string{"tracked.txt"}, IntendedUntracked: []string{}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := status
			got.Forecast = &tt.forecast
			if tt.next {
				got.NextTransition = &ReviewNextTransition{Kind: item.Kind, ReasonCode: item.ReasonCode}
			}
			err := got.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNegotiatedStatusForecastStaysStructuralAndV1StaysFrozen(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("forecast\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stderr := captureReviewProcessStderr(t)

	var stdout bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &stdout); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("status envelope is invalid: %v", err)
	}
	if status.NextTransition == nil || status.Forecast == nil || len(status.Forecast.Steps) != 1 {
		t.Fatalf("status forecast = %#v, transition = %#v", status.Forecast, status.NextTransition)
	}
	step := status.Forecast.Steps[0]
	if status.Forecast.Horizon != ForecastHorizonPartial || step.Step != 1 || step.Kind != status.NextTransition.Kind || step.ReasonCode != status.NextTransition.ReasonCode || strings.TrimSpace(step.Description) == "" {
		t.Fatalf("status forecast = %#v, transition = %#v", status.Forecast, status.NextTransition)
	}
	// The forecast is structural only: a successful negotiated STATUS writes
	// zero bytes to stderr (gentle-pi fails closed on any stderr a successful
	// native process writes).
	if got := stderr(); got != "" {
		t.Errorf("negotiated STATUS narrated the forecast to stderr, want zero bytes:\n%q", got)
	}

	var legacy bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition"}, &legacy); err != nil {
		t.Fatal(err)
	}
	var v1 map[string]any
	if err := json.Unmarshal(legacy.Bytes(), &v1); err != nil {
		t.Fatal(err)
	}
	if _, found := v1["forecast"]; found {
		t.Fatalf("v1 status gained forecast: %s", legacy.String())
	}
}

func TestNativeStatusSchemasValidateWholeForecastEnvelope(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "status-v5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"status-v4.schema.json", "status-v5.schema.json"} {
		t.Run(name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(fixture, &document); err != nil {
				t.Fatal(err)
			}
			document["schema"] = "gentle-ai.review-integration.status/v" + strings.TrimSuffix(strings.TrimPrefix(name, "status-v"), ".schema.json")
			if name == "status-v4.schema.json" {
				document["receipt"] = map[string]any{"status": "expected_missing"}
			}
			schema := compileWholeNativeStatusSchema(t, name)
			if err := schema.Validate(document); err != nil {
				t.Fatalf("whole %s envelope rejected fixture: %v", name, err)
			}
			document["unknown"] = true
			if err := schema.Validate(document); err == nil {
				t.Fatal("whole schema accepted an unknown property")
			}
		})
	}
}

func compileWholeNativeStatusSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration"))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(reviewSchemaRegexpEngine)
	for _, version := range []string{"v1", "v2"} {
		paths, err := filepath.Glob(filepath.Join(root, version, "schemas", "*.schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatal(err)
			}
			if err := compiler.AddResource(document["$id"].(string), document); err != nil {
				t.Fatal(err)
			}
		}
	}
	id := "https://gentle-ai.dev/contracts/review-integration/v2/schemas/" + name
	schema, err := compiler.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

type reviewSchemaRegexp struct {
	pattern string
	re      *regexp.Regexp
}

func (r reviewSchemaRegexp) String() string { return r.pattern }

func (r reviewSchemaRegexp) MatchString(value string) bool {
	if r.re == nil {
		return value != "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\n") && !slices.Contains(strings.Split(value, "/"), "..")
	}
	return r.re.MatchString(value)
}

func reviewSchemaRegexpEngine(pattern string) (jsonschema.Regexp, error) {
	if pattern == "^(?!/)(?!.*(?:^|/)\\.\\.(?:/|$)).+$" {
		return reviewSchemaRegexp{pattern: pattern}, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return reviewSchemaRegexp{pattern: pattern, re: re}, nil
}
