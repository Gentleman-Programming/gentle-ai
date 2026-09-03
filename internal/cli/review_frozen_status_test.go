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
			wantSubject, err := reviewtransaction.NewArtifactSubject(record.State, record.State.CapturePhaseRevision, frozen, record.State.SelectedLenses[0], 0, "")
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

func TestExplicitFrozenTerminalStatusRestoresUntrackedScope(t *testing.T) {
	repo, store, record := frozenReviewingStatusFixture(t, reviewtransaction.TargetCurrentChanges, []string{"frozen-untracked.txt"})
	captureFrozenReviewerResults(t, repo, record, len(record.State.SelectedLenses))
	pending, err := store.Load()
	if err != nil {
		t.Fatalf("load approved pending authority: %v", err)
	}
	if pending.State.State != reviewtransaction.StateApproved {
		t.Fatalf("terminal authority state = %q, want approved", pending.State.State)
	}

	status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
	if status.TargetIdentity != record.State.InitialSnapshot.Identity ||
		status.Projection.Kind != reviewtransaction.TargetCurrentChanges ||
		status.Projection.Projection != reviewtransaction.ProjectionWorkspace ||
		!reflect.DeepEqual(status.Projection.IntendedUntracked, record.State.InitialSnapshot.IntendedUntracked) ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("explicit terminal STATUS = %#v", status)
	}
	assertApprovedAcknowledgementTransition(t, status.NextTransition.Execute, repo, record.State.LineageID,
		record.State.InitialSnapshot.Identity, pending.Revision)

	writeUndeclaredWorkspaceFile(t, repo, "frozen-untracked.txt", "changed selected bytes\n", 0o644)
	changed := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
	live := explicitFrozenReviewingStatus(t, repo, "")
	if changed.TargetIdentity == record.State.InitialSnapshot.Identity ||
		changed.TargetIdentity != live.TargetIdentity ||
		changed.NextTransition == nil || changed.NextTransition.ReasonCode == "approved_acknowledgement_required" {
		t.Fatalf("changed selected bytes reused terminal scope: changed=%#v live=%#v", changed, live)
	}
}

func TestExplicitFrozenReviewingStatusRejectsPartialSlotsAndStaleStartLineages(t *testing.T) {
	t.Run("partial canonical record entry fails closed", func(t *testing.T) {
		repo, store, record := frozenReviewingStatusFixture(t, reviewtransaction.TargetCurrentChanges, nil)
		captureFrozenReviewerResults(t, repo, record, 1)
		captured, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if captured.State.CapturePhaseRevision == "" || len(captured.State.AdmittedRoleResults) != 1 {
			t.Fatalf("captured compact fixture lost its phase-bound record tuple: %#v", captured.State)
		}
		captured.State.AdmittedRoleResults[0].ArtifactDigest = ""
		captured.Revision, err = reviewtransaction.CompactRevisionForState(captured.State)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.MarshalIndent(captured, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.StatePath(), append(payload, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'live drift'\n", 0o644)
		status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
		if status.Applicability != reviewtransaction.TargetApplicabilityCorrupted || status.NextTransition == nil ||
			status.NextTransition.Kind == reviewNextTransitionCollect {
			t.Fatalf("partial frozen record entry status = %#v", status)
		}
	})

	t.Run("occupied intended-untracked slots close on the final current capture", func(t *testing.T) {
		repo, store, record := frozenReviewingStatusFixture(t, reviewtransaction.TargetCurrentChanges, []string{"frozen-untracked.txt"})
		for order, lens := range record.State.SelectedLenses[:len(record.State.SelectedLenses)-1] {
			input := filepath.Join(t.TempDir(), lens+".json")
			writeReviewCLIJSON(t, input, admittedReviewerResultForTest(t, repo, record, lens, order))
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
			status.Authority.LineageID != record.State.LineageID || status.NextTransition == nil ||
			status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "reviewer_results_required" ||
			status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 ||
			status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureResultCaptureOperation {
			t.Fatalf("occupied frozen STATUS = %#v\n%s", status, output.String())
		}

		order := len(record.State.SelectedLenses) - 1
		lens := record.State.SelectedLenses[order]
		input := filepath.Join(t.TempDir(), lens+".json")
		writeReviewCLIJSON(t, input, admittedReviewerResultForTest(t, repo, record, lens, order))
		var terminal bytes.Buffer
		if err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", record.State.LineageID,
			"--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", strconv.Itoa(order), "--input", input}, &terminal); err != nil {
			t.Fatalf("capture final frozen lens: %v\n%s", err, terminal.String())
		}
		var result reviewLastEventClosureResult
		decodeStrictReviewJSON(t, terminal.Bytes(), &result)
		if result.Operation != "review/capture-result" || result.LineageID != record.State.LineageID || result.State != reviewtransaction.StateApproved {
			t.Fatalf("final frozen capture = %#v", result)
		}
		assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
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
	store, record := createFrozenReviewingStatusRecord(t, repo,
		"frozen-status-"+strings.ReplaceAll(string(kind), "_", "-"), target)
	return repo, store, record
}

func frozenStagedReviewingStatusFixture(t *testing.T) (string, reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)

	// Keep a legal, unrelated open lineage in the authority inventory. The
	// explicit frozen selector below must resume only its own pending review.
	writeReviewStartCandidate(t, repo, "unrelated-token.ts", "export const unrelated = 'open'\n", 0o644)
	_, unrelated := createFrozenReviewingStatusRecord(t, repo, "frozen-staged-unrelated", reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if unrelated.State.State != reviewtransaction.StateReviewing || len(unrelated.State.SelectedLenses) == 0 {
		t.Fatalf("unrelated open frozen authority = %#v", unrelated)
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "commit unrelated authority subject")

	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'frozen'\n", 0o644)
	store, record := createFrozenReviewingStatusRecord(t, repo, "frozen-staged-reviewing", reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: base, Projection: reviewtransaction.ProjectionStaged,
		IntendedUntracked: []string{}, LedgerIDs: []string{},
	})
	if record.State.State != reviewtransaction.StateReviewing || len(record.State.SelectedLenses) == 0 ||
		record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseWorkspaceOverlay ||
		record.State.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged ||
		!reflect.DeepEqual(record.State.InitialSnapshot.Paths, []string{"service-token.ts"}) || record.State.Recovery != nil {
		t.Fatalf("open frozen staged authority = %#v", record)
	}
	return repo, store, record
}

func createFrozenReviewingStatusRecord(t *testing.T, repo, lineage string, target reviewtransaction.Target) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
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
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: "sha256:" + strings.Repeat("1", 64), RiskLevel: risk,
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
	return store, started.Record
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
