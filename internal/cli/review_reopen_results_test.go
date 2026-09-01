package cli

import (
	"bytes"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewReopenResultsRemovesSelectedCanonicalEntryWithoutSidecar(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, initial := newArtifactReview(t, true)
	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCLIReviewerResultWithFindings(t, repo, started, order, []facadeFinding{}, &bytes.Buffer{})
	}
	partial, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureAdmittedRefuterResult(t.Context(), reviewtransaction.CompactAdmittedRefuterResultRequest{ExpectedRevision: partial.State.CapturePhaseRevision, TargetIdentity: partial.State.InitialSnapshot.Identity, RequestHash: facadePayloadHash([]byte("reopen refuter")), Payload: []byte(`{"results":[]}`)}); err != nil {
		t.Fatal(err)
	}
	last := len(started.SelectedLenses) - 1
	captureCLIReviewerResultWithFindings(t, repo, started, last, []facadeFinding{{Location: "service-token.ts:1", Severity: "CRITICAL", Claim: "candidate requires a bounded correction", ProofRefs: []string{"service-token.ts:1 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced}}, &bytes.Buffer{})
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if before.State.State != reviewtransaction.StateCorrectionRequired || len(before.State.AdmittedRoleResults) != 5 {
		t.Fatalf("seed authority = %#v", before.State)
	}
	beforeLensPhases := make(map[string]string, len(before.State.SelectedLenses))
	for _, entry := range before.State.AdmittedRoleResults {
		if entry.Role == reviewtransaction.CompactRoleLens {
			beforeLensPhases[entry.Lens] = entry.CapturePhaseRevision
		}
	}
	lenses := []string{initial.State.SelectedLenses[1], initial.State.SelectedLenses[0]}
	base := []string{"--cwd", repo, "--lineage", started.LineageID, "--expected-revision", before.Revision, "--target", before.State.InitialSnapshot.Identity, "--reason", "reviewer input was wrong", "--actor", "maintainer"}
	quarantine := []string{"--quarantine-lens", lenses[0], "--quarantine-lens", lenses[1]}
	var prepared, repeated bytes.Buffer
	if err := RunReview(append([]string{"reopen-results"}, append(append(base, quarantine...), "--prepare")...), &prepared); err != nil || RunReview(append([]string{"reopen-results"}, append(append(base, quarantine...), "--prepare")...), &repeated) != nil {
		t.Fatalf("prepare reopen-results: %v", err)
	}
	if !bytes.Equal(prepared.Bytes(), repeated.Bytes()) {
		t.Fatal("reopen prepare JSON was not canonical")
	}
	var preparation ReviewResultReopenResult
	decodeStrictReviewJSON(t, prepared.Bytes(), &preparation)
	if !preparation.Prepared || preparation.Plan == nil || len(preparation.Plan.QuarantineLenses) != 2 || preparation.Plan.QuarantineLenses[0] != lenses[1] || preparation.Plan.QuarantineLenses[1] != lenses[0] || len(preparation.Plan.Removed) != 3 {
		t.Fatalf("record-only reopen plan = %#v", preparation)
	}
	if err := RunReview(append([]string{"reopen-results"}, append(append(base, quarantine...), "--maintainer-authorization", "wrong")...), &bytes.Buffer{}); err == nil {
		t.Fatal("inexact reopen authorization was accepted")
	}
	wrongSet := []string{"--quarantine-lens", lenses[1], "--quarantine-lens", initial.State.SelectedLenses[2], "--maintainer-authorization", preparation.Plan.RequiredMaintainerAuthorization}
	if err := RunReview(append([]string{"reopen-results"}, append(base, wrongSet...)...), &bytes.Buffer{}); err == nil {
		t.Fatal("authorization for another quarantine set was accepted")
	}
	if unchanged, err := store.Load(); err != nil || unchanged.Revision != before.Revision {
		t.Fatalf("reopen refusal mutated authority: %#v, %v", unchanged, err)
	}
	var applied bytes.Buffer
	if err := RunReview(append([]string{"reopen-results"}, append(append(base, quarantine...), "--maintainer-authorization", preparation.Plan.RequiredMaintainerAuthorization)...), &applied); err != nil {
		t.Fatalf("apply reopen-results: %v", err)
	}
	var result ReviewResultReopenResult
	decodeStrictReviewJSON(t, applied.Bytes(), &result)
	if result.Schema != ReviewResultReopenSchema || result.Record == nil || result.Record.State != reviewtransaction.StateReviewing || len(result.Record.Removed) != 3 {
		t.Fatalf("record-only reopen result JSON = %#v", result)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision == before.Revision || after.State.CapturePhaseRevision == before.State.CapturePhaseRevision || len(after.State.AdmittedRoleResults) != 2 {
		t.Fatalf("reopen did not atomically retain only the other canonical values: %#v", after.State)
	}
	for _, entry := range after.State.AdmittedRoleResults {
		if entry.Role == reviewtransaction.CompactRoleLens && entry.CapturePhaseRevision != beforeLensPhases[entry.Lens] {
			t.Fatalf("retained lens %q phase = %q, want stored phase %q", entry.Lens, entry.CapturePhaseRevision, beforeLensPhases[entry.Lens])
		}
	}
	var statusOutput bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--lineage", started.LineageID, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Collect == nil {
		t.Fatalf("STATUS did not advertise recapture inputs: %#v", status.NextTransition)
	}
	inputs := status.NextTransition.Collect.Inputs
	if len(inputs) != 2 || inputs[0].ArtifactSubject == nil || inputs[1].ArtifactSubject == nil || inputs[0].ArtifactSubject.Lens != lenses[1] || inputs[1].ArtifactSubject.Lens != lenses[0] {
		t.Fatalf("STATUS recapture inputs = %#v", status.NextTransition)
	}
}
