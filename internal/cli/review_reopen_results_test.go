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
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewReopenResultsQuarantinesLegacyUnadmittedArtifactAndReplacesSlot(t *testing.T) {
	repo, started, store, initial := newArtifactReview(t, false)
	lens := initial.State.SelectedLenses[0]
	legacy := facadeReviewerResult{
		Lens: lens, Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate tree"},
	}
	legacyPath := filepath.Join(t.TempDir(), "legacy-result.json")
	writeReviewCLIJSON(t, legacyPath, legacy)
	if err := finalizeReviewCLIArgs(t, repo, []string{
		"--cwd", repo, "--lineage", started.LineageID, "--result", legacyPath,
	}, io.Discard); err != nil {
		t.Fatalf("finalize historical result: %v", err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if validating.State.State != reviewtransaction.StateValidating {
		t.Fatalf("historical result state = %q, want validating", validating.State.State)
	}
	legacyBytes, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	legacyDigest := writeLegacyReviewerSlot(t, store, lens, 0, legacyBytes)

	const actor = "maintainer@example.com"
	const reason = "historical reviewer result lacks provider admission"
	baseArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID,
		"--expected-revision", validating.Revision, "--target", validating.State.InitialSnapshot.Identity,
		"--reason", reason, "--actor", actor,
	}
	if err := os.WriteFile(store.ReceiptPath(), []byte("historical receipt residue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(append([]string{"reopen-results"}, append(baseArgs, "--prepare")...), io.Discard); err == nil ||
		!strings.Contains(err.Error(), "already has a receipt") {
		t.Fatalf("validating authority with receipt residue was accepted: %v", err)
	}
	if err := os.Remove(store.ReceiptPath()); err != nil {
		t.Fatal(err)
	}
	var prepared bytes.Buffer
	if err := RunReview(append([]string{"reopen-results"}, append(baseArgs, "--prepare")...), &prepared); err != nil {
		t.Fatalf("prepare reopen-results: %v", err)
	}
	var preparation ReviewResultReopenResult
	decodeStrictReviewJSON(t, prepared.Bytes(), &preparation)
	if !preparation.Prepared || preparation.Plan == nil || len(preparation.Plan.Quarantined) != 1 ||
		preparation.Plan.Quarantined[0].ArtifactDigest != legacyDigest || len(preparation.Plan.Retained) != 0 {
		t.Fatalf("unexpected reopen plan: %#v", preparation)
	}
	beforeRefusal := validating
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
		!reflect.DeepEqual(reopened.State.InitialSnapshot, validating.State.InitialSnapshot) ||
		!reflect.DeepEqual(reopened.State.SelectedLenses, validating.State.SelectedLenses) ||
		reopened.State.RiskLevel != validating.State.RiskLevel || reopened.State.CorrectionBudget != validating.State.CorrectionBudget ||
		len(reopened.State.LensResults) != 0 || len(reopened.State.ResultReopens) != 1 {
		t.Fatalf("reopen changed immutable review inputs or retained derived results: %#v", reopened.State)
	}
	if artifacts, err := discoverCapturedReviewerArtifacts(context.Background(), repo, store.Dir, reopened.State, reopened.Revision); err != nil || len(artifacts) != 0 {
		t.Fatalf("quarantined slot remains discoverable: artifacts=%#v err=%v", artifacts, err)
	}

	replacementInput := filepath.Join(t.TempDir(), "replacement.json")
	if err := os.WriteFile(replacementInput, admittedReviewerPayloadForTest(t, repo, reopened, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", reopened.State.InitialSnapshot.Identity,
		"--lens", lens, "--order", "0", "--expected-revision", reopened.Revision, "--input", replacementInput,
	}, &captured); err != nil {
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
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", strings.TrimSpace(captured.String()),
	}, io.Discard); err != nil {
		t.Fatalf("finalize replacement: %v", err)
	}
	refinalized, err := store.Load()
	if err != nil || refinalized.State.State != reviewtransaction.StateValidating {
		t.Fatalf("replacement did not return to validating: state=%q err=%v", refinalized.State.State, err)
	}

	var replay bytes.Buffer
	if err := RunReview(applyArgs, &replay); err != nil {
		t.Fatalf("exact reopen replay: %v", err)
	}
	var replayed ReviewResultReopenResult
	decodeStrictReviewJSON(t, replay.Bytes(), &replayed)
	if replayed.Record == nil || !replayed.Record.Replayed {
		t.Fatalf("exact reopen did not converge: %#v", replayed)
	}
}

func TestReviewReopenResultsRetainsAdmittedSlotsAndRejectsCleanAuthority(t *testing.T) {
	t.Parallel()

	repo, started, store, initial := newArtifactReview(t, true)
	if len(initial.State.SelectedLenses) != 4 {
		t.Fatalf("selected lenses = %v, want 4R", initial.State.SelectedLenses)
	}
	resultPaths := make([]string, len(initial.State.SelectedLenses))
	var retainedManifest reviewResultArtifact
	for order, lens := range initial.State.SelectedLenses {
		payload := admittedReviewerPayloadForTest(t, repo, initial, lens, order)
		path := filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", order))
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		resultPaths[order] = path
		// Every slot is admitted now that finalize only consumes captured
		// results; order 0's manifest is the one this test later re-checks.
		var output bytes.Buffer
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", initial.State.InitialSnapshot.Identity,
			"--lens", lens, "--order", strconv.Itoa(order), "--input", path,
		}, &output); err != nil {
			t.Fatal(err)
		}
		if order == 0 {
			decodeStrictReviewJSON(t, output.Bytes(), &retainedManifest)
		}
	}
	finalizeArgs := []string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results=true"}
	if err := RunReviewFacadeFinalize(finalizeArgs, io.Discard); err != nil {
		t.Fatal(err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	legacyDigests := make([]string, len(initial.State.SelectedLenses))
	for order := 1; order < len(initial.State.SelectedLenses); order++ {
		payload, readErr := os.ReadFile(resultPaths[order])
		if readErr != nil {
			t.Fatal(readErr)
		}
		legacyDigests[order] = writeLegacyReviewerSlot(t, store, initial.State.SelectedLenses[order], order, payload)
	}
	const actor = "maintainer@example.com"
	const reason = "replace historical unadmitted slots"
	baseArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--expected-revision", validating.Revision,
		"--target", validating.State.InitialSnapshot.Identity, "--reason", reason, "--actor", actor,
	}
	if err := RunReview(append([]string{"reopen-results"}, append([]string{}, append(baseArgs, "--expected-revision", initial.Revision, "--prepare")...)...), io.Discard); err == nil {
		t.Fatal("stale validating revision was accepted")
	}
	var prepared bytes.Buffer
	if err := RunReview(append([]string{"reopen-results"}, append(baseArgs, "--prepare")...), &prepared); err != nil {
		t.Fatal(err)
	}
	var preparation ReviewResultReopenResult
	decodeStrictReviewJSON(t, prepared.Bytes(), &preparation)
	if preparation.Plan == nil || len(preparation.Plan.Retained) != 1 || len(preparation.Plan.Quarantined) != 3 ||
		preparation.Plan.Retained[0].ArtifactDigest != retainedManifest.SHA256 ||
		preparation.Plan.Retained[0].SubjectHash != retainedManifest.SubjectHash {
		t.Fatalf("trusted slot classification = %#v", preparation.Plan)
	}
	if err := RunReview(append([]string{"reopen-results"}, append(baseArgs,
		"--maintainer-authorization", preparation.Plan.RequiredMaintainerAuthorization)...), io.Discard); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := discoverCapturedReviewerArtifacts(context.Background(), repo, store.Dir, reopened.State, reopened.Revision)
	if err != nil || len(artifacts) != 1 || artifacts[0].SHA256 != retainedManifest.SHA256 {
		t.Fatalf("retained admitted slot discovery = %#v, err=%v", artifacts, err)
	}
	for order := 1; order < len(reopened.State.SelectedLenses); order++ {
		lens := reopened.State.SelectedLenses[order]
		path := filepath.Join(t.TempDir(), fmt.Sprintf("replacement-%d.json", order))
		if err := os.WriteFile(path, admittedReviewerPayloadForTest(t, repo, reopened, lens, order), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", reopened.State.InitialSnapshot.Identity,
			"--lens", lens, "--order", fmt.Sprint(order), "--expected-revision", reopened.Revision, "--input", path,
		}, io.Discard); err != nil {
			t.Fatalf("capture replacement %d: %v", order, err)
		}
		archivePath, ok := reviewtransaction.ReviewerResultQuarantinePath(store.Dir, reopened.State, order, legacyDigests[order])
		if _, statErr := os.Stat(archivePath); !ok || statErr != nil {
			t.Fatalf("legacy slot %d was not archived: path=%q ok=%v err=%v", order, archivePath, ok, statErr)
		}
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--captured-results",
	}, io.Discard); err != nil {
		t.Fatalf("finalize retained and replacement slots: %v", err)
	}
	refinalized, err := store.Load()
	if err != nil || refinalized.State.State != reviewtransaction.StateValidating {
		t.Fatalf("refinalized state = %q, err=%v", refinalized.State.State, err)
	}

	cleanRepo, cleanStarted, cleanStore, cleanInitial, cleanArtifact := capturedArtifact(t)
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", cleanRepo, "--lineage", cleanStarted.LineageID, "--result-artifact", mustReviewJSON(t, cleanArtifact),
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	cleanValidating, err := cleanStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	cleanRequest := reviewtransaction.CompactResultReopenRequest{
		LineageID: cleanStarted.LineageID, ExpectedRevision: cleanValidating.Revision,
		TargetIdentity: cleanInitial.State.InitialSnapshot.Identity, Reason: "nothing to quarantine", Actor: actor,
	}
	if _, err := reviewtransaction.PrepareCompactResultReopen(context.Background(), cleanRepo, cleanRequest); err == nil ||
		!strings.Contains(err.Error(), "found no unusable") {
		t.Fatalf("clean admitted authority reopen error = %v", err)
	}
}

func TestReviewReopenResultsQuarantinesTamperedAdmittedResultBytes(t *testing.T) {
	repo, started, store, initial, artifact := capturedArtifact(t)
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", mustReviewJSON(t, artifact),
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
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
		LineageID: started.LineageID, ExpectedRevision: validating.Revision,
		TargetIdentity: validating.State.InitialSnapshot.Identity,
		Reason:         "replace tampered provider result", Actor: "maintainer@example.com",
	}
	plan, err := reviewtransaction.PrepareCompactResultReopen(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("tampered admitted result should be quarantinable: %v", err)
	}
	if len(plan.Retained) != 0 || len(plan.Quarantined) != 1 || plan.Quarantined[0].ArtifactDigest != tamperedDigest {
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
