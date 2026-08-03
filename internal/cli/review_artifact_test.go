package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewCaptureResultStrictBindingReplayAndFinalize(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	args := func(lineage, target, lens, order string) []string {
		return []string{"--cwd", repo, "--lineage", lineage, "--target", target, "--lens", lens, "--order", order, "--input", input}
	}
	validArgs := args(started.LineageID, record.State.InitialSnapshot.Identity, record.State.SelectedLenses[0], "0")
	for _, payload := range []string{"prose", `{}`, `{"findings":[],"evidence":[]} {}`, `{"findings":[],"evidence":[],"unknown":true}`} {
		if err := os.WriteFile(input, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult(validArgs, io.Discard); err == nil {
			t.Fatalf("invalid payload accepted: %s", payload)
		}
	}
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{
		args("wrong-lineage", record.State.InitialSnapshot.Identity, record.State.SelectedLenses[0], "0"),
		args(started.LineageID, "sha256:"+strings.Repeat("0", 64), record.State.SelectedLenses[0], "0"),
		args(started.LineageID, record.State.InitialSnapshot.Identity, "review-risk", "0"),
		args(started.LineageID, record.State.InitialSnapshot.Identity, record.State.SelectedLenses[0], "1"),
	} {
		if err := RunReviewCaptureResult(bad, io.Discard); err == nil {
			t.Fatal("wrong capture binding accepted")
		}
	}
	var first, replay bytes.Buffer
	if err := RunReviewCaptureResult(validArgs, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult(validArgs, &replay); err != nil || first.String() != replay.String() {
		t.Fatalf("exact replay changed: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, first.Bytes(), &artifact)
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0, "inspection: different evidence over every frozen candidate path"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult(validArgs, io.Discard); err == nil {
		t.Fatal("mismatched replay accepted")
	} else {
		for _, want := range []string{"review dispose-result", "review preserve-result"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("reviewer result byte-conflict = %q, want it to name %q", err.Error(), want)
			}
		}
	}
	manifest := strings.TrimSpace(first.String())
	for _, finalize := range [][]string{
		{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest, "--result", input},
		{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest, "--result-artifact", manifest},
	} {
		if err := RunReviewFacadeFinalize(finalize, io.Discard); err == nil {
			t.Fatal("mixed or duplicate artifact accepted")
		}
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", manifest}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestReviewCaptureResultAdmitsOneJSONEnvelopeInsideProse(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	payload := admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	input := filepath.Join(t.TempDir(), "result.txt")
	if err := os.WriteFile(input, append(append([]byte("Review complete.\n"), payload...), []byte("\nEnd of review.\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &captured); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", strings.TrimSpace(captured.String()),
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestReviewCaptureResultRejectsSemanticAdmissionBeforePublication(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*facadeReviewerResult)
	}{
		{name: "subject mismatch", mutate: func(result *facadeReviewerResult) {
			result.SubjectHash = "sha256:" + strings.Repeat("9", 64)
		}},
		{name: "inspection denied", mutate: func(result *facadeReviewerResult) {
			result.Inspection.Status = "blocked"
			result.Evidence = []string{"Access denied; candidate was not inspected."}
		}},
		{name: "out of scope inspection path", mutate: func(result *facadeReviewerResult) {
			result.Inspection.Paths = append(result.Inspection.Paths, "unrelated/old.go")
		}},
		{name: "out of scope finding", mutate: func(result *facadeReviewerResult) {
			result.Findings = []facadeFinding{{
				ID: "R3-001", Location: "unrelated/old.go:3", Severity: "CRITICAL", Claim: "unrelated defect",
				ProofRefs: []string{"unrelated/old.go:3"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalPreExisting,
			}}
		}},
		{name: "unsupported causal fields", mutate: func(result *facadeReviewerResult) {
			result.Findings = []facadeFinding{{
				ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
				ProofRefs: []string{"tracked.txt:1 changed hunk"}, EvidenceClass: "changed-hunk+before-after-proof",
				CausalDisposition: "introduced and behavior-activated",
			}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record := newArtifactReview(t, false)
			result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
			tt.mutate(&result)
			input := filepath.Join(t.TempDir(), "result.json")
			writeReviewCLIJSON(t, input, result)
			err := RunReviewCaptureResult([]string{
				"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
				"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
			}, io.Discard)
			if err == nil {
				t.Fatal("semantically invalid reviewer result was captured")
			}
			if _, statErr := os.Stat(filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir)); !os.IsNotExist(statErr) {
				t.Fatalf("semantic rejection consumed the immutable result slot: %v", statErr)
			}
			assertArtifactRevision(t, store, record.Revision)
		})
	}
}

func TestReviewCaptureResultPublishesExternalRepositoryProofExactlyOnce(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "support.go"), []byte("package support\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "support.go")
	runReviewCLIGit(t, repo, "commit", "-m", "add supporting implementation")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Evidence = []string{"supporting proof: support.go:1"}
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "WARNING", Claim: "candidate behavior depends on supporting code",
		ProofRefs: []string{"repository proof: support.go:1"},
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	args := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}
	var first bytes.Buffer
	if err := RunReviewCaptureResult(args, &first); err != nil {
		t.Fatalf("capture external repository proof: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, first.Bytes(), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
		t.Fatalf("external proof admission = %q", artifact.AdmissionDecision)
	}
	payload, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	var published admittedReviewerResult
	decodeStrictReviewJSON(t, payload, &published)
	if strings.Count(strings.Join(published.Result.Evidence, "\n"), "support.go:1") != 1 ||
		strings.Count(strings.Join(published.Result.Findings[0].ProofRefs, "\n"), "support.go:1") != 1 {
		t.Fatalf("external repository proof was not published exactly once: %#v", published.Result)
	}
	var replay bytes.Buffer
	if err := RunReviewCaptureResult(args, &replay); err != nil || replay.String() != first.String() {
		t.Fatalf("exact external-proof replay changed: %v\nfirst=%s\nreplay=%s", err, first.String(), replay.String())
	}
	entries, err := os.ReadDir(filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir))
	if err != nil {
		t.Fatal(err)
	}
	jsonFiles := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			jsonFiles++
		}
	}
	if jsonFiles != 1 {
		t.Fatalf("captured reviewer JSON files = %d, want exactly one", jsonFiles)
	}
}

// TestReviewCaptureResultIDLessCandidateCausalFinding proves issue-1699 is not
// actually fixed by the Group A canonicalization change: a severe
// introduced/behavior-activated/worsened finding submitted with no "id" field
// must still admit once verifiedCandidateCausalFindingIDs and AdmitArtifact's
// canonical fallback-ID assignment agree on the same (canonicalized) finding
// IDs. Before the ordering fix, review_artifact.go:309 called
// verifiedCandidateCausalFindingIDs with the RAW, pre-canonicalization
// nativeResult, so the omitted ID produced a verified-ID slice that could
// never match the canonical fallback ID (`R#-001`) that AdmitArtifact assigns
// internally.
func TestReviewCaptureResultIDLessCandidateCausalFinding(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		// ID intentionally omitted (empty): the reachable shape ftorga proved.
		Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "the candidate introduces an unreviewed causal defect",
		ProofRefs: []string{"tracked.txt:1 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	var captured bytes.Buffer
	err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &captured)
	if err != nil {
		t.Fatalf("id-less candidate-causal finding capture-result failed: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
		t.Fatalf("id-less candidate-causal admission = %q, want completed", artifact.AdmissionDecision)
	}
}

func TestReviewCaptureResultRejectsInvalidLocationWithActionableDiagnostic(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1-2", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)

	err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, io.Discard)
	var admissionErr *reviewtransaction.ArtifactAdmissionError
	var locationErr *reviewtransaction.FindingLocationError
	if !errors.As(err, &admissionErr) || !errors.As(err, &locationErr) {
		t.Fatalf("capture-result error = %v; want typed admission and location errors", err)
	}
	if admissionErr.Diagnostic == nil || admissionErr.Diagnostic.FindingID != "R3-001" ||
		admissionErr.Diagnostic.Location != "tracked.txt:1-2" ||
		admissionErr.Diagnostic.Reason != "line_suffix_not_integer" {
		t.Fatalf("capture diagnostic = %#v", admissionErr.Diagnostic)
	}
}

func TestReviewCaptureResultFinalizePreservesCausalClassification(t *testing.T) {
	tests := []struct {
		name        string
		class       reviewtransaction.EvidenceClass
		causality   reviewtransaction.CausalDisposition
		withRefuter bool
	}{
		{name: "deterministic introduced", class: reviewtransaction.EvidenceDeterministic, causality: reviewtransaction.CausalIntroduced},
		{name: "inferential behavior activated", class: reviewtransaction.EvidenceInferential, causality: reviewtransaction.CausalBehaviorActivated, withRefuter: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record := newArtifactReview(t, false)
			result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
			result.Findings = []facadeFinding{{
				ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
				ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: tt.class, CausalDisposition: tt.causality,
			}}
			input := filepath.Join(t.TempDir(), "result.json")
			writeReviewCLIJSON(t, input, result)
			var captured bytes.Buffer
			if err := RunReviewCaptureResult([]string{
				"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
				"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
			}, &captured); err != nil {
				t.Fatal(err)
			}
			args := []string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", strings.TrimSpace(captured.String())}
			if tt.withRefuter {
				refuter := filepath.Join(t.TempDir(), "refuter.json")
				writeReviewCLIJSON(t, refuter, facadeRefuterResult{Results: []facadeRefuterOutcome{{
					FindingID: "R3-001", Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction"},
				}}})
				args = append(args, "--refuter", refuter)
			}
			if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
				t.Fatal(err)
			}
			finalized, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			finding := finalized.State.LensResults[0].Findings[0]
			classification := finalized.State.Classifications[finding.ID]
			if finding.EvidenceClass != tt.class || finding.CausalDisposition != tt.causality ||
				classification.Class != tt.class || classification.Causality != tt.causality ||
				finalized.State.Outcomes[finding.ID] != reviewtransaction.OutcomeCorroborated ||
				finalized.State.State != reviewtransaction.StateCorrectionRequired || !reflect.DeepEqual(finalized.State.FixFindingIDs, []string{finding.ID}) {
				t.Fatalf("causal result was not preserved: finding=%#v classification=%#v state=%q outcomes=%#v fixes=%v",
					finding, classification, finalized.State.State, finalized.State.Outcomes, finalized.State.FixFindingIDs)
			}
		})
	}
}

func TestReviewFinalizeArtifactFiles(t *testing.T) {
	for _, tt := range []struct {
		name     string
		stdin    bool
		fileName string
		prefix   []byte
	}{
		{name: "file"},
		{name: "file with spaces and Unicode", fileName: "manifest café 文件.json"},
		{name: "file with UTF-8 BOM", prefix: []byte("\xef\xbb\xbf")},
		{name: "stdin", stdin: true},
		{name: "stdin with UTF-8 BOM", stdin: true, prefix: []byte("\xef\xbb\xbf")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, _, _, artifacts := capturedArtifacts(t, false)
			before, err := os.ReadFile(artifacts[0].Path)
			if err != nil {
				t.Fatal(err)
			}
			payload := append(tt.prefix, artifactManifestJSON(t, artifacts[0])...)
			path := "-"
			if tt.stdin {
				withFacadeStdin(t, payload)
			} else {
				fileName := tt.fileName
				if fileName == "" {
					fileName = "manifest.json"
				}
				path = filepath.Join(t.TempDir(), fileName)
				if err := os.WriteFile(path, payload, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", path}, io.Discard); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(artifacts[0].Path)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("canonical reviewer-result bytes changed: %v", err)
			}
		})
	}

	t.Run("repeated files preserve selected-lens order", func(t *testing.T) {
		repo, started, _, _, artifacts := capturedArtifacts(t, true)
		args := []string{"--cwd", repo, "--lineage", started.LineageID}
		for _, artifact := range artifacts {
			args = append(args, "--result-artifact-file", writeArtifactManifest(t, artifact))
		}
		if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
			t.Fatal(err)
		}
	})

}

func TestReviewFinalizeArtifactFileSizeLimit(t *testing.T) {
	t.Run("exact limit", func(t *testing.T) {
		repo, started, _, _, artifacts := capturedArtifacts(t, false)
		path := writeSizedArtifactManifest(t, artifacts[0], reviewResultArtifactLimit)
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", path}, io.Discard); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("one byte over limit", func(t *testing.T) {
		repo, started, store, record, artifacts := capturedArtifacts(t, false)
		path := writeSizedArtifactManifest(t, artifacts[0], reviewResultArtifactLimit+1)
		err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", path}, io.Discard)
		if err == nil || err.Error() != "read reviewer artifact manifest 1: artifact exceeds the native result size limit" {
			t.Fatalf("over-limit manifest error = %v", err)
		}
		assertArtifactRevision(t, store, record.Revision)
	})
}

func TestReviewFinalizeArtifactFileCancellationAndErrorPrivacy(t *testing.T) {
	t.Run("active stdin read", func(t *testing.T) {
		repo, started, store, record, artifacts := capturedArtifacts(t, false)
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		oldStdin := os.Stdin
		os.Stdin = reader
		t.Cleanup(func() {
			os.Stdin = oldStdin
			_ = writer.Close()
			_ = reader.Close()
		})
		payload := append(artifactManifestJSON(t, artifacts[0]), bytes.Repeat([]byte(" "), 1<<20)...)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() {
			result <- runReviewFacadeFinalize(ctx, []string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", "-"}, io.Discard)
		}()
		written := make(chan error, 1)
		go func() {
			_, writeErr := writer.Write(payload)
			written <- writeErr
		}()
		select {
		case err := <-written:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("finalize did not begin active stdin read")
		}
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("active stdin cancellation error = %v", err)
			}
		case <-time.After(time.Second):
			_ = writer.Close()
			<-result
			t.Fatal("active stdin read remained blocked after cancellation")
		}
		if _, err := reader.Stat(); err != nil {
			t.Fatalf("cancellation closed the caller-owned input: %v", err)
		}
		assertArtifactRevision(t, store, record.Revision)
	})

	t.Run("cancelled context", func(t *testing.T) {
		repo, started, store, record, artifacts := capturedArtifacts(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := runReviewFacadeFinalize(ctx, []string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", writeArtifactManifest(t, artifacts[0])}, io.Discard)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled finalize error = %v", err)
		}
		assertArtifactRevision(t, store, record.Revision)
	})

	t.Run("negotiated file error is path-free", func(t *testing.T) {
		repo, started, store, record, _ := capturedArtifacts(t, false)
		secret := filepath.Join(t.TempDir(), "token=private-manifest.json")
		var output bytes.Buffer
		err := RunReview([]string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", secret}, &output)
		if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(output.String(), secret) {
			t.Fatalf("negotiated manifest error leaked private path: output=%s error=%v", output.String(), err)
		}
		if failure := decodeReviewIntegrationFailure(t, output.Bytes()); failure.Code != "invalid_request" || failure.MutationOutcome != ReviewMutationNotStarted {
			t.Fatalf("negotiated manifest failure = %#v", failure)
		}
		assertArtifactRevision(t, store, record.Revision)
	})
}

func TestReviewFinalizeRejectsArtifactFileSourceMixing(t *testing.T) {
	result := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(result, []byte(`{"findings":[],"evidence":["checked"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		args func(string, string) []string
	}{
		{"inline manifest", func(path, manifest string) []string {
			return []string{"--result-artifact-file", path, "--result-artifact", manifest}
		}},
		{"legacy result", func(path, _ string) []string { return []string{"--result-artifact-file", path, "--result", result} }},
		{"captured results", func(path, _ string) []string { return []string{"--result-artifact-file", path, "--captured-results"} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record, artifacts := capturedArtifacts(t, false)
			manifest := string(artifactManifestJSON(t, artifacts[0]))
			args := append([]string{"--cwd", repo, "--lineage", started.LineageID}, tt.args(writeArtifactManifest(t, artifacts[0]), manifest)...)
			if err := RunReviewFacadeFinalize(args, io.Discard); err == nil {
				t.Fatal("mixed reviewer-result sources accepted")
			}
			assertArtifactRevision(t, store, record.Revision)
		})
	}
}

func TestReviewFinalizeArtifactFileStdinAccounting(t *testing.T) {
	repo, started, _, _, _ := capturedArtifacts(t, false)
	for _, args := range [][]string{
		{"--result-artifact-file", "-", "--result-artifact-file", "-"},
		{"--result-artifact-file", "-", "--result", "-"},
	} {
		finalize := append([]string{"--cwd", repo, "--lineage", started.LineageID}, args...)
		if err := RunReviewFacadeFinalize(finalize, io.Discard); err == nil || !strings.Contains(err.Error(), "stdin for only one input") {
			t.Fatalf("multiple stdin inputs = %v", err)
		}
	}
}

func TestReviewFinalizeArtifactFilePreservesStrictManifestValidation(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		repo, started, store, record := newArtifactReview(t, false)
		path := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(path, []byte(`{"schema":`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", path}, io.Discard); err == nil {
			t.Fatal("malformed manifest accepted")
		}
		assertArtifactRevision(t, store, record.Revision)
	})

	t.Run("two leading UTF-8 BOMs", func(t *testing.T) {
		repo, started, store, record, artifacts := capturedArtifacts(t, false)
		payload := append([]byte("\xef\xbb\xbf\xef\xbb\xbf"), artifactManifestJSON(t, artifacts[0])...)
		path := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", path}, io.Discard); err == nil {
			t.Fatal("manifest with two leading UTF-8 BOMs accepted")
		}
		assertArtifactRevision(t, store, record.Revision)
	})

	for _, tt := range []struct {
		name   string
		mutate func(*reviewResultArtifact)
	}{
		{"ownership", func(artifact *reviewResultArtifact) { artifact.Path = filepath.Join(t.TempDir(), "result.json") }},
		{"hash", func(artifact *reviewResultArtifact) { artifact.SHA256 = "sha256:" + strings.Repeat("0", 64) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record, artifacts := capturedArtifacts(t, false)
			tt.mutate(&artifacts[0])
			path := writeArtifactManifest(t, artifacts[0])
			if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact-file", path}, io.Discard); err == nil {
				t.Fatal("substituted artifact accepted")
			}
			assertArtifactRevision(t, store, record.Revision)
		})
	}
}

func TestReviewCaptureResultWaitsForMaintenanceBeforePublication(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := reviewtransaction.AcquireReviewMaintenanceExclusive(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}
	if err := RunReviewCaptureResult(args, io.Discard); !errors.Is(err, reviewtransaction.ErrAuthorityLockTimeout) {
		t.Fatalf("capture while maintenance held = %v", err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("authority changed while capture blocked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir)); !os.IsNotExist(err) {
		t.Fatalf("capture published while maintenance held: %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult(args, io.Discard); err != nil {
		t.Fatalf("capture after maintenance release: %v", err)
	}
}
func TestReviewArtifactSubstitutionFailsBeforeMutation(t *testing.T) {
	mutations := []struct {
		name string
		run  func(*reviewResultArtifact)
	}{
		{"lineage", func(a *reviewResultArtifact) { a.LineageID = "wrong" }},
		{"target", func(a *reviewResultArtifact) { a.TargetIdentity = "sha256:" + strings.Repeat("0", 64) }},
		{"lens", func(a *reviewResultArtifact) { a.Lens = "review-risk" }},
		{"order", func(a *reviewResultArtifact) { a.SelectedOrder = 1 }},
		{"hash", func(a *reviewResultArtifact) { a.SHA256 = "sha256:" + strings.Repeat("0", 64) }},
		{"path", func(a *reviewResultArtifact) { a.Path = filepath.Join(t.TempDir(), "result.json") }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			repo, started, store, record, artifact := capturedArtifact(t)
			tt.run(&artifact)
			assertArtifactFinalizeUnchanged(t, repo, started.LineageID, store, record.Revision, artifact)
		})
	}
	for _, kind := range []string{"directory", "symlink", "mode", "bytes", "race"} {
		t.Run(kind, func(t *testing.T) {
			repo, started, store, record, artifact := capturedArtifact(t)
			original, _ := os.ReadFile(artifact.Path)
			replacement := filepath.Join(t.TempDir(), "replacement.json")
			if err := os.WriteFile(replacement, original, 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "directory":
				_ = os.Remove(artifact.Path)
				_ = os.Mkdir(artifact.Path, 0o700)
			case "symlink":
				_ = os.Remove(artifact.Path)
				_ = os.Symlink(filepath.Join(t.TempDir(), "missing"), artifact.Path)
			case "mode":
				if runtime.GOOS == "windows" {
					t.Skip("Windows ACLs do not map to Unix mode bits")
				}
				_ = os.Chmod(artifact.Path, 0o644)
			case "bytes":
				_ = os.WriteFile(artifact.Path, []byte("replacement"), 0o600)
			case "race":
				reviewArtifactAfterLstat = func() {
					reviewArtifactAfterLstat = func() {}
					if err := os.Rename(artifact.Path, artifact.Path+".old"); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, artifact.Path); err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { reviewArtifactAfterLstat = func() {} })
			}
			assertArtifactFinalizeUnchanged(t, repo, started.LineageID, store, record.Revision, artifact)
		})
	}
	if !reviewArtifactModeSafeForOS(0o666, false, "windows") || reviewArtifactModeSafeForOS(0o666, false, "linux") {
		t.Fatal("platform permission semantics changed")
	}
}
func TestReviewCaptureResultConcurrentSelectedLenses(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, true)
	manifests := make([]string, len(record.State.SelectedLenses))
	var wg sync.WaitGroup
	for order, lens := range record.State.SelectedLenses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := filepath.Join(t.TempDir(), fmt.Sprintf("%d.json", order))
			_ = os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, lens, order), 0o600)
			var output bytes.Buffer
			err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", fmt.Sprint(order), "--input", input}, &output)
			if err != nil {
				t.Errorf("capture %s: %v", lens, err)
				return
			}
			manifests[order] = strings.TrimSpace(output.String())
		}()
	}
	wg.Wait()
	if _, err := readFacadeReviewerArtifacts(context.Background(), repo, manifests, store.Dir, record.State, record.Revision); err != nil {
		t.Fatal(err)
	}
}
func TestReviewerArtifactDirectorySyncCompatibility(t *testing.T) {
	originalGOOS, originalSync := reviewArtifactRuntimeGOOS, syncReviewerArtifactDirectory
	t.Cleanup(func() { reviewArtifactRuntimeGOOS, syncReviewerArtifactDirectory = originalGOOS, originalSync })
	cases := []struct {
		name, goos string
		err        error
		wantOK     bool
	}{
		{"fatal", "linux", errors.New("disk sync failed"), false},
		{"invalid", "linux", syscall.EINVAL, true},
		{"unsupported", "linux", errors.ErrUnsupported, true},
		{"windows permission", "windows", os.ErrPermission, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			reviewArtifactRuntimeGOOS = func() string { return tt.goos }
			syncReviewerArtifactDirectory = func(string) error { return tt.err }
			err := syncReviewerArtifactDirectoryCompatible(t.TempDir())
			if (err == nil) != tt.wantOK {
				t.Fatalf("sync compatibility error = %v, want success %v", err, tt.wantOK)
			}
		})
	}
}
func newArtifactReview(t *testing.T, high bool) (string, ReviewFacadeStartResult, reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	name := "tracked.txt"
	if high {
		name = "service-token.ts"
	}
	if err := os.WriteFile(filepath.Join(repo, name), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return repo, started, store, record
}

func admittedReviewerPayloadForTest(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string, order int, evidence ...string) []byte {
	t.Helper()
	result := admittedReviewerResultForTest(t, repo, record, lens, order)
	if len(evidence) == 0 {
		evidence = []string{"inspection: reviewed every frozen candidate path"}
	}
	result.Evidence = evidence
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func admittedReviewerResultForTest(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string, order int) facadeReviewerResult {
	t.Helper()
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := reviewtransaction.NewArtifactSubject(record.State, record.Revision, frozen, lens, order, "")
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(frozen.ChangedPathManifest))
	for index, entry := range frozen.ChangedPathManifest {
		paths[index] = entry.Path
	}
	return facadeReviewerResult{
		SubjectHash: subject.SubjectHash,
		Inspection:  reviewtransaction.ArtifactInspection{Status: reviewtransaction.ArtifactInspectionCompleted, Paths: paths},
		Findings:    []facadeFinding{}, Evidence: []string{"inspection: reviewed every frozen candidate path"},
	}
}

func capturedArtifact(t *testing.T) (string, ReviewFacadeStartResult, reviewtransaction.CompactStore, reviewtransaction.CompactRecord, reviewResultArtifact) {
	t.Helper()
	repo, started, store, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	_ = os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600)
	var output bytes.Buffer
	if err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}, &output); err != nil {
		t.Fatal(err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, output.Bytes(), &artifact)
	return repo, started, store, record, artifact
}

func capturedArtifacts(t *testing.T, high bool) (string, ReviewFacadeStartResult, reviewtransaction.CompactStore, reviewtransaction.CompactRecord, []reviewResultArtifact) {
	t.Helper()
	repo, started, store, record := newArtifactReview(t, high)
	artifacts := make([]reviewResultArtifact, len(record.State.SelectedLenses))
	for order, lens := range record.State.SelectedLenses {
		input := filepath.Join(t.TempDir(), fmt.Sprintf("%d.json", order))
		if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, lens, order), 0o600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", fmt.Sprint(order), "--input", input}, &output); err != nil {
			t.Fatal(err)
		}
		decodeStrictReviewJSON(t, output.Bytes(), &artifacts[order])
	}
	return repo, started, store, record, artifacts
}

func writeArtifactManifest(t *testing.T, artifact reviewResultArtifact) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, artifactManifestJSON(t, artifact), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSizedArtifactManifest(t *testing.T, artifact reviewResultArtifact, size int) string {
	t.Helper()
	payload := artifactManifestJSON(t, artifact)
	if len(payload) > size {
		t.Fatalf("manifest size = %d, exceeds requested %d", len(payload), size)
	}
	payload = append(payload, bytes.Repeat([]byte(" "), size-len(payload))...)
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func artifactManifestJSON(t *testing.T, artifact reviewResultArtifact) []byte {
	t.Helper()
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func withFacadeStdin(t *testing.T, payload []byte) {
	t.Helper()
	old := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = old
		_ = reader.Close()
	})
}

func assertArtifactRevision(t *testing.T, store reviewtransaction.CompactStore, revision string) {
	t.Helper()
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != revision {
		t.Fatal("artifact input failure mutated authority")
	}
}
func assertArtifactFinalizeUnchanged(t *testing.T, repo, lineage string, store reviewtransaction.CompactStore, revision string, artifact reviewResultArtifact) {
	t.Helper()
	payload, _ := json.Marshal(artifact)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage, "--result-artifact", string(payload)}, io.Discard); err == nil {
		t.Fatal("substituted artifact accepted")
	}
	after, _ := store.Load()
	if after.Revision != revision {
		t.Fatal("artifact mismatch mutated authority")
	}
}

func TestValidateReviewerResultPayloadEmptyResult(t *testing.T) {
	for _, payload := range [][]byte{
		nil,
		{},
		[]byte("   "),
		[]byte("\n\t\r\n"),
	} {
		err := validateReviewerResultPayload(payload)
		if err == nil {
			t.Fatalf("expected error for empty payload %q, got nil", payload)
		}
		var payloadErr *ReviewerResultPayloadError
		if !errors.As(err, &payloadErr) {
			t.Fatalf("expected ReviewerResultPayloadError, got %T: %v", err, err)
		}
		if payloadErr.Code != "empty_result" {
			t.Fatalf("expected code empty_result, got %q", payloadErr.Code)
		}
	}
}

func TestValidateReviewerResultPayloadNestedEnvelope(t *testing.T) {
	for _, payload := range [][]byte{
		[]byte("<task_result>\n{\"findings\":[]}\n</task_result>"),
		[]byte("some prefix <task_result> content </task_result> suffix"),
		[]byte("</task_result>"),
		[]byte("<task_result>"),
	} {
		err := validateReviewerResultPayload(payload)
		if err == nil {
			t.Fatalf("expected error for nested envelope payload %q, got nil", payload)
		}
		var payloadErr *ReviewerResultPayloadError
		if !errors.As(err, &payloadErr) {
			t.Fatalf("expected ReviewerResultPayloadError, got %T: %v", err, err)
		}
		if payloadErr.Code != "nested_envelope" {
			t.Fatalf("expected code nested_envelope, got %q", payloadErr.Code)
		}
	}
}

func TestValidateReviewerResultPayloadDistinguishesErrorCodes(t *testing.T) {
	// Empty and nested envelope must produce distinct, non-overlapping codes.
	emptyErr := validateReviewerResultPayload([]byte(""))
	nestedErr := validateReviewerResultPayload([]byte("<task_result>{}</task_result>"))
	validErr := validateReviewerResultPayload([]byte(`{"findings":[],"evidence":["ok"]}`))
	validWithTagsErr := validateReviewerResultPayload([]byte(`{"findings":[],"evidence":["<task_result>"]}`))

	if emptyErr == nil || nestedErr == nil {
		t.Fatal("both failure modes must return errors")
	}
	if validErr != nil || validWithTagsErr != nil {
		t.Fatalf("valid payloads must not error, got: %v, %v", validErr, validWithTagsErr)
	}
	var emptyPayloadErr, nestedPayloadErr *ReviewerResultPayloadError
	if !errors.As(emptyErr, &emptyPayloadErr) || !errors.As(nestedErr, &nestedPayloadErr) {
		t.Fatal("both errors must be ReviewerResultPayloadError")
	}
	if emptyPayloadErr.Code == nestedPayloadErr.Code {
		t.Fatalf("error codes must differ: both are %q", emptyPayloadErr.Code)
	}
}

func TestReviewCaptureResultRejectsEmptyPayload(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	validArgs := []string{"--cwd", repo, "--lineage", started.LineageID, "--target",
		record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}

	for _, payload := range []string{"", "   ", "\n\t"} {
		if err := os.WriteFile(input, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		err := RunReviewCaptureResult(validArgs, io.Discard)
		if err == nil {
			t.Fatalf("empty payload %q must be rejected", payload)
		}
		if !strings.Contains(err.Error(), "empty_result") && !strings.Contains(err.Error(), "empty") {
			t.Fatalf("expected empty_result in error, got: %v", err)
		}
	}
}

func TestReviewCaptureResultRejectsNestedEnvelope(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	validArgs := []string{"--cwd", repo, "--lineage", started.LineageID, "--target",
		record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}

	payload := "<task_result>\n{\"findings\":[],\"evidence\":[\"checked\"]}\n</task_result>"
	if err := os.WriteFile(input, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	err := RunReviewCaptureResult(validArgs, io.Discard)
	if err == nil {
		t.Fatal("nested envelope payload must be rejected")
	}
	if !strings.Contains(err.Error(), "nested_envelope") && !strings.Contains(err.Error(), "envelope") {
		t.Fatalf("expected nested_envelope in error, got: %v", err)
	}
}
