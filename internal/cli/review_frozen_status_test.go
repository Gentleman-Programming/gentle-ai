package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestExplicitFrozenReviewingStatusResumesPendingCandidateAfterDrift(t *testing.T) {
	reviewEnabledHome(t)
	for _, targetKind := range []reviewtransaction.TargetKind{
		reviewtransaction.TargetCurrentChanges,
		reviewtransaction.TargetBaseDiff,
		reviewtransaction.TargetBaseWorkspaceOverlay,
	} {
		t.Run(string(targetKind), func(t *testing.T) {
			repo, _, record := frozenReviewingStatusFixture(t, targetKind, nil)
			inventory := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
			before := frozenCollectEnvelope(t, explicitFrozenReviewingStatus(t, repo, record.State.LineageID))
			writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'live drift'\n", 0o644)
			writeUndeclaredWorkspaceFile(t, repo, "live-untracked.txt", "live drift\n", 0o644)

			status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
			if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
				status.Authority.LineageID != record.State.LineageID || status.Authority.Revision != record.Revision ||
				status.TargetIdentity != record.State.InitialSnapshot.Identity ||
				status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
				status.NextTransition.ReasonCode != "reviewer_results_required" {
				t.Fatalf("explicit frozen status = %#v", status)
			}
			if status.Projection.Kind != record.State.InitialSnapshot.Kind ||
				status.Projection.BaseTree != record.State.InitialSnapshot.BaseTree ||
				status.Projection.InitialReviewTree != record.State.InitialSnapshot.CandidateTree ||
				status.Projection.CurrentCandidateTree != record.State.InitialSnapshot.CandidateTree ||
				status.Projection.PathsDigest != record.State.InitialSnapshot.PathsDigest ||
				!reflect.DeepEqual(status.Projection.Paths, record.State.InitialSnapshot.Paths) ||
				!reflect.DeepEqual(status.Projection.IntendedUntracked, record.State.InitialSnapshot.IntendedUntracked) ||
				status.Projection.IntendedUntrackedProof != record.State.InitialSnapshot.IntendedUntrackedProof {
				t.Fatalf("frozen projection = %#v, authority = %#v", status.Projection, record.State.InitialSnapshot)
			}
			input := status.NextTransition.Collect.Inputs[0]
			frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(t.Context(), record.State.InitialSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			wantSubject, err := reviewtransaction.NewArtifactSubject(record.State, record.Revision, frozen, record.State.SelectedLenses[0], 0, "")
			if err != nil || input.ArtifactSubject == nil || !reflect.DeepEqual(*input.ArtifactSubject, wantSubject) {
				t.Fatalf("capture binding = %#v, want %#v, err = %v", input, wantSubject, err)
			}
			if !bytes.Equal(before, frozenCollectEnvelope(t, status)) {
				t.Fatalf("frozen collect envelope changed after drift")
			}
			if after := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo)); !reflect.DeepEqual(inventory, after) {
				t.Fatalf("status changed authority inventory: before=%#v after=%#v", inventory, after)
			}
		})
	}
}

func TestExplicitFrozenReviewingStatusUsesFrozenUntrackedScope(t *testing.T) {
	reviewEnabledHome(t)
	repo, _, record := frozenReviewingStatusFixture(t, reviewtransaction.TargetCurrentChanges, []string{"frozen-untracked.txt"})
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'live drift'\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "live-untracked.txt", "live drift\n", 0o644)

	selectorless := explicitFrozenReviewingStatus(t, repo, "")
	status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
	if selectorless.NextTransition == nil || selectorless.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		t.Fatalf("selectorless mixed workspace status = %#v", selectorless)
	}
	if !reflect.DeepEqual(status.Projection.IntendedUntracked, []string{"frozen-untracked.txt"}) ||
		status.NextTransition == nil || status.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("explicit frozen untracked status = %#v", status)
	}
}

func TestExplicitFrozenReviewingStatusRejectsPartialSlotsAndStaleStartLineages(t *testing.T) {
	t.Run("partial canonical slot fails closed", func(t *testing.T) {
		repo, store, record := frozenReviewingStatusFixture(t, reviewtransaction.TargetCurrentChanges, nil)
		captureFrozenReviewerResults(t, repo, record, 1)
		path := filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir, "00-"+record.State.SelectedLenses[0]+".json.sha256")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'live drift'\n", 0o644)
		status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
		if status.Applicability != reviewtransaction.TargetApplicabilityCorrupted || status.NextTransition == nil ||
			status.NextTransition.Kind == reviewNextTransitionCollect {
			t.Fatalf("partial frozen slot status = %#v", status)
		}
	})

	t.Run("occupied intended-untracked slots finalize through the selected OpenCode lineage", func(t *testing.T) {
		repo, _, record := frozenReviewingStatusFixture(t, reviewtransaction.TargetCurrentChanges, []string{"frozen-untracked.txt"})
		for order, lens := range record.State.SelectedLenses {
			result := admittedReviewerResultForTest(t, repo, record, lens, order)
			if order == 0 {
				result.Findings = []facadeFinding{{
					ID: "R1-001", Location: "service-token.ts:1", Severity: "CRITICAL", Claim: "frozen candidate failure",
					ProofRefs: []string{"service-token.ts:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
					CausalDisposition: reviewtransaction.CausalIntroduced,
				}}
			}
			input := filepath.Join(t.TempDir(), lens+".json")
			writeReviewCLIJSON(t, input, result)
			if err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", record.State.LineageID,
				"--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", strconv.Itoa(order), "--input", input}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
		}
		var output bytes.Buffer
		if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV2, "--agent", "opencode", "--next-transition", "--cwd", repo, "--lineage", record.State.LineageID}, &output); err != nil {
			t.Fatalf("selected OpenCode STATUS: %v\n%s", err, output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
			status.Authority.LineageID != record.State.LineageID || status.Action != reviewtransaction.TargetStatusActionFinalize ||
			status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
			status.NextTransition.ReasonCode != "captured_results_ready" || status.NextTransition.Execute == nil ||
			status.NextTransition.Execute.Operation != "review.finalize" || len(status.NextTransition.Execute.Artifacts) != len(record.State.SelectedLenses) {
			t.Fatalf("occupied frozen STATUS = %#v\n%s", status, output.String())
		}
		var finalized bytes.Buffer
		if err := RunReviewFacadeFinalize([]string{"--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo,
			"--lineage", record.State.LineageID, "--captured-results"}, &finalized); err != nil {
			t.Fatalf("finalize selected OpenCode lineage: %v\n%s", err, finalized.String())
		}
		var result ReviewIntegrationFinalizeResult
		decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, finalized.Bytes()).Result, &result)
		if result.State != reviewtransaction.StateCorrectionRequired {
			t.Fatalf("selected OpenCode finalize state = %q, want correction_required", result.State)
		}
	})

	t.Run("explicit compact lineage ignores stale v1 and v3 siblings", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'frozen'\n", 0o644)
		started := atomicStartV2(t, repo, "frozen-status-atomic")
		store := atomicCompactStartStore(t, repo, started.LineageID)
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		writeAtomicStartCorruptSibling(t, repo, "v1", "stale-v1-sibling")
		writeAtomicStartCorruptSibling(t, repo, "v3", "stale-v3-sibling")
		before := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
		writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'live drift'\n", 0o644)

		status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
		if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
			status.Authority.LineageID != record.State.LineageID || status.Authority.Revision != record.Revision ||
			status.Authority.State != reviewtransaction.StateReviewing || status.NextTransition == nil ||
			status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "reviewer_results_required" {
			t.Fatalf("explicit compact STATUS with stale siblings = %#v", status)
		}
		if after := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo)); !reflect.DeepEqual(before, after) {
			t.Fatalf("explicit compact STATUS mutated or selected a sibling authority: before=%#v after=%#v", before, after)
		}
		loaded, err := store.Load()
		if err != nil || !reflect.DeepEqual(loaded, record) {
			t.Fatalf("explicit compact STATUS changed named compact authority: %#v, %v", loaded, err)
		}
	})
}

func frozenReviewingStatusFixture(t *testing.T, kind reviewtransaction.TargetKind, intended []string) (string, reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	if kind == reviewtransaction.TargetBaseWorkspaceOverlay {
		if len(intended) != 0 {
			t.Fatal("staged frozen fixture does not support intended untracked paths")
		}
		return frozenStagedReviewingStatusFixture(t)
	}
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'frozen'\n", 0o644)
	target := reviewtransaction.Target{Kind: kind, IntendedUntracked: append([]string{}, intended...)}
	for _, path := range intended {
		writeUndeclaredWorkspaceFile(t, repo, path, "frozen intended\n", 0o644)
	}
	switch kind {
	case reviewtransaction.TargetBaseDiff:
		runReviewCLIGit(t, repo, "add", "service-token.ts")
		runReviewCLIGit(t, repo, "commit", "-qm", "frozen base diff")
		target.BaseRef = base
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.BuildStoredSnapshot(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == reviewtransaction.RiskMedium {
		lenses = []string{reviewtransaction.LensReliability}
	} else if risk == reviewtransaction.RiskHigh {
		lenses = []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	}
	if len(lenses) == 0 {
		t.Fatalf("fixture must select reviewer lenses: risk=%q lines=%d", risk, lines)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "frozen-status-" + strings.ReplaceAll(string(kind), "_", "-"), Mode: reviewtransaction.ModeOrdinaryBounded,
		Generation: 1, Snapshot: snapshot, PolicyHash: "sha256:" + strings.Repeat("1", 64), RiskLevel: risk,
		SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.CreateOrReplayAtomicStart(t.Context(), reviewtransaction.CompactAtomicStartRequest{
		State: state,
		Binding: reviewtransaction.CompactAtomicStartBinding{
			LineageID: state.LineageID, WorktreeIdentity: lease.Identity().RepositoryRef,
			TargetIdentity: snapshot.Identity, Selector: target, PolicyHash: state.PolicyHash,
			Tier: state.RiskLevel, SelectedLenses: append([]string(nil), state.SelectedLenses...),
			OriginalChangedLines: state.OriginalChangedLines, CorrectionBudget: state.CorrectionBudget,
			CorrectionBudgetPolicy: state.CorrectionBudgetPolicy,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, store, started.Record
}

func frozenStagedReviewingStatusFixture(t *testing.T) (string, reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "docs/candidate.md", "# Candidate\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed base candidate")
	// This predecessor represents compact authority approved before v3 began
	// burning terminal records. Seed it directly rather than using FINALIZE so
	// the recovery fixture remains historical without resurrecting persistence.
	historical := seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, "frozen-staged-root", reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseDiff, BaseRef: base, Projection: reviewtransaction.ProjectionWorkspace,
	})
	if historical.Record.State.LineageID != "frozen-staged-root" ||
		historical.Record.State.State != reviewtransaction.StateApproved || historical.Record.Revision == "" ||
		historical.Record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff ||
		historical.Record.State.InitialSnapshot.BaseTree == "" || historical.Record.State.InitialSnapshot.CandidateTree == "" ||
		historical.Record.State.InitialSnapshot.PathsDigest == "" || historical.Record.State.InitialSnapshot.Identity == "" ||
		!reflect.DeepEqual(historical.Record.State.InitialSnapshot.Paths, []string{"docs/candidate.md"}) {
		t.Fatalf("historical frozen staged predecessor = %#v", historical.Record)
	}

	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'frozen'\n", 0o644)
	probe := selectorTransitionStatus(t, repo, "--lineage", historical.Record.State.LineageID, "--base-ref", base, "--projection", "staged", "--workspace-overlay")
	if probe.Authority == nil || probe.Authority.LineageID != historical.Record.State.LineageID ||
		probe.Authority.Revision != historical.Record.Revision || probe.TargetIdentity == "" ||
		probe.TargetIdentity == historical.Record.State.InitialSnapshot.Identity {
		t.Fatalf("historical frozen staged recovery probe = %#v, predecessor = %#v", probe, historical.Record)
	}
	const successor, actor, reason = "frozen-staged-reviewing", "maintainer", "include staged token"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + probe.Authority.LineageID + "\npredecessor_revision=" + probe.Authority.Revision +
		"\ntarget_identity=" + probe.TargetIdentity + "\nsuccessor_lineage=" + successor + "\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo, "--lineage", probe.Authority.LineageID, "--base-ref", base, "--projection", "staged", "--workspace-overlay",
		"--recovery-successor-lineage", successor, "--recovery-reason", reason, "--recovery-actor", actor, "--recovery-authorization", authorization)
	executeSelectorTransition(t, repo, status)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, successor)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.LineageID != successor || record.State.State != reviewtransaction.StateReviewing ||
		record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseWorkspaceOverlay ||
		record.State.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged ||
		record.State.InitialSnapshot.Identity != probe.TargetIdentity || record.State.Recovery == nil ||
		record.State.Recovery.PredecessorLineageID != probe.Authority.LineageID ||
		record.State.Recovery.PredecessorRevision != probe.Authority.Revision {
		t.Fatalf("historical frozen staged recovery successor = %#v, probe = %#v", record, probe)
	}
	return repo, store, record
}

func explicitFrozenReviewingStatus(t *testing.T, repo, lineage string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	args := []string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo}
	if lineage != "" {
		args = append(args, "--lineage", lineage)
	}
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("frozen STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}

func frozenCollectEnvelope(t *testing.T, status ReviewTargetStatusResult) []byte {
	t.Helper()
	payload, err := json.Marshal([]any{status.Authority, status.Frozen, status.TargetIdentity,
		status.AuthorityTargetIdentity, status.Projection, status.NextTransition})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func captureFrozenReviewerResults(t *testing.T, repo string, record reviewtransaction.CompactRecord, count int) {
	t.Helper()
	for order, lens := range record.State.SelectedLenses[:count] {
		input := filepath.Join(t.TempDir(), lens+".json")
		if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, lens, order), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", record.State.LineageID,
			"--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", strconv.Itoa(order), "--input", input}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
}
