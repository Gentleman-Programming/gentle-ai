package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewReopenResultsQuarantinesLegacyUnadmittedArtifactAndReplacesSlot(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, initial := newArtifactReview(t, true)
	if len(initial.State.SelectedLenses) != 4 {
		t.Fatalf("selected lenses = %v, want 4R", initial.State.SelectedLenses)
	}
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == len(started.SelectedLenses)-1 {
			findings = []facadeFinding{{
				Location: "service-token.ts:1", Severity: "CRITICAL", Claim: "candidate requires a bounded correction",
				ProofRefs: []string{"service-token.ts:1 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	correction, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if correction.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("severe final capture state = %q, want correction_required", correction.State.State)
	}
	lens := initial.State.SelectedLenses[0]
	legacy := facadeReviewerResult{
		Lens: lens, Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate tree"},
	}
	legacyBytes, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyBytes = append(legacyBytes, '\n')
	legacyDigest := writeLegacyReviewerSlot(t, store, lens, 0, legacyBytes)

	const actor = "maintainer@example.com"
	const reason = "historical reviewer result lacks provider admission"
	baseArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID,
		"--expected-revision", correction.Revision, "--target", correction.State.InitialSnapshot.Identity,
		"--reason", reason, "--actor", actor,
	}
	var prepared bytes.Buffer
	if err := RunReview(append([]string{"reopen-results"}, append(baseArgs, "--prepare")...), &prepared); err != nil {
		t.Fatalf("prepare reopen-results: %v", err)
	}
	var preparation ReviewResultReopenResult
	decodeStrictReviewJSON(t, prepared.Bytes(), &preparation)
	if !preparation.Prepared || preparation.Plan == nil || len(preparation.Plan.Quarantined) != 1 ||
		preparation.Plan.Quarantined[0].ArtifactDigest != legacyDigest || len(preparation.Plan.Retained) != 3 {
		t.Fatalf("unexpected reopen plan: %#v", preparation)
	}
	beforeRefusal := correction
	if err := RunReview(append([]string{"reopen-results"}, append(baseArgs, "--maintainer-authorization", "wrong")...), io.Discard); err == nil {
		t.Fatal("inexact reopen authorization was accepted")
	}
	afterRefusal, err := store.Load()
	if err != nil || !reflect.DeepEqual(afterRefusal, beforeRefusal) {
		t.Fatalf("refused reopen mutated authority: err=%v before=%#v after=%#v", err, beforeRefusal, afterRefusal)
	}

	applyArgs := append([]string{"reopen-results"}, append(baseArgs, "--maintainer-authorization", preparation.Plan.RequiredMaintainerAuthorization)...)
	var applied bytes.Buffer
	if err := RunReview(applyArgs, &applied); err != nil {
		t.Fatalf("apply reopen-results: %v", err)
	}
	var result ReviewResultReopenResult
	decodeStrictReviewJSON(t, applied.Bytes(), &result)
	if result.Record == nil || result.Record.State != reviewtransaction.StateReviewing || result.Record.Replayed {
		t.Fatalf("unexpected reopen result: %#v", result)
	}
	reopened, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State.State != reviewtransaction.StateReviewing ||
		!reflect.DeepEqual(reopened.State.InitialSnapshot, correction.State.InitialSnapshot) ||
		!reflect.DeepEqual(reopened.State.SelectedLenses, correction.State.SelectedLenses) ||
		reopened.State.RiskLevel != correction.State.RiskLevel || reopened.State.CorrectionBudget != correction.State.CorrectionBudget ||
		len(reopened.State.LensResults) != 0 || len(reopened.State.ResultReopens) != 1 {
		t.Fatalf("reopen changed immutable review inputs or retained derived results: %#v", reopened.State)
	}
	if artifacts, err := discoverCapturedReviewerArtifacts(context.Background(), repo, store.Dir, reopened.State, reopened.Revision); err != nil || len(artifacts) != 3 {
		t.Fatalf("retained slots were not discoverable after reopen: artifacts=%#v err=%v", artifacts, err)
	}

	replacementInput := filepath.Join(t.TempDir(), "replacement.json")
	if err := os.WriteFile(replacementInput, admittedReviewerPayloadForTest(t, repo, reopened, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", reopened.State.InitialSnapshot.Identity,
		"--lens", lens, "--order", "0", "--expected-revision", reopened.Revision, "--input", replacementInput,
	}, io.Discard); err != nil {
		t.Fatalf("capture replacement: %v", err)
	}
	archivePath, ok := reviewtransaction.ReviewerResultQuarantinePath(store.Dir, reopened.State, 0, legacyDigest)
	if !ok {
		t.Fatal("reopened authority lost quarantine destination")
	}
	archived, err := os.ReadFile(archivePath)
	if err != nil || !bytes.Equal(archived, legacyBytes) {
		t.Fatalf("legacy reviewer bytes were not preserved: err=%v got=%q", err, archived)
	}
	if digest, err := os.ReadFile(archivePath + ".sha256"); err != nil || strings.TrimSpace(string(digest)) != legacyDigest {
		t.Fatalf("legacy reviewer digest was not preserved: err=%v digest=%q", err, digest)
	}
	afterReplacement, err := store.Load()
	if err != nil || afterReplacement.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("replacement did not reopen the bounded correction: state=%q err=%v", afterReplacement.State.State, err)
	}
}

func TestReviewReopenResultsQuarantinesTamperedAdmittedResultBytes(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, initial := newArtifactReview(t, true)
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == len(started.SelectedLenses)-1 {
			findings = []facadeFinding{{
				Location: "service-token.ts:1", Severity: "CRITICAL", Claim: "candidate requires a bounded correction",
				ProofRefs: []string{"service-token.ts:1 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	correction, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if correction.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("severe final capture state = %q, want correction_required", correction.State.State)
	}
	path := filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", 0, initial.State.SelectedLenses[0]))
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope admittedReviewerResult
	decodeStrictReviewJSON(t, payload, &envelope)
	envelope.Result.Evidence = []string{"tampered after provider admission"}
	payload, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	tamperedDigest := facadePayloadHash(payload)
	if err := os.WriteFile(path+".sha256", []byte(tamperedDigest+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := reviewtransaction.CompactResultReopenRequest{
		LineageID: started.LineageID, ExpectedRevision: correction.Revision,
		TargetIdentity: correction.State.InitialSnapshot.Identity,
		Reason:         "replace tampered provider result", Actor: "maintainer@example.com",
	}
	plan, err := reviewtransaction.PrepareCompactResultReopen(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("tampered admitted result should be quarantinable: %v", err)
	}
	if len(plan.Retained) != len(initial.State.SelectedLenses)-1 || len(plan.Quarantined) != 1 || plan.Quarantined[0].ArtifactDigest != tamperedDigest {
		t.Fatalf("tampered result plan = %#v", plan)
	}
}

func mustReviewJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func writeLegacyReviewerSlot(t *testing.T, store reviewtransaction.CompactStore, lens string, order int, payload []byte) string {
	t.Helper()
	dir := filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir)
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%02d-%s.json", order, lens))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := facadePayloadHash(payload)
	if err := os.WriteFile(path+".sha256", []byte(digest+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return digest
}
