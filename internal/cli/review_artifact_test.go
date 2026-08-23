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
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewCaptureResultStrictBindingTerminalCapture(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

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
	var output bytes.Buffer
	if err := RunReviewCaptureResult(validArgs, &output); err != nil {
		t.Fatal(err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Schema != reviewLastEventClosureSchema || terminal.State != reviewtransaction.StateApproved || terminal.Action != reviewApprovedLastEventBurnedAction {
		t.Fatalf("terminal capture result = %#v", terminal)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestReviewCaptureResultAdmitsOneJSONEnvelopeInsideProse(t *testing.T) {
	reviewEnabledHome(t)
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
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, captured.Bytes(), &terminal)
	if terminal.Schema != reviewLastEventClosureSchema || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("prose terminal capture = %#v", terminal)
	}
}

func TestReviewCaptureResultRejectsSemanticAdmissionBeforePublication(t *testing.T) {
	t.Parallel()

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
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "support.go"), []byte("package support\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "support.go")
	runReviewCLIGit(t, repo, "commit", "-m", "add supporting implementation")
	if err := os.WriteFile(filepath.Join(repo, "service-token.ts"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startHighRiskCLIReview(t, repo)
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
		Location: "service-token.ts:1", Severity: "WARNING", Claim: "candidate behavior depends on supporting code",
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
	reviewEnabledHome(t)
	repo, started, _, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "the candidate introduces an unreviewed causal defect",
		ProofRefs: []string{"tracked.txt:1 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	var captured bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &captured); err != nil {
		t.Fatalf("id-less candidate-causal finding capture-result failed: %v", err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, captured.Bytes(), &terminal)
	if terminal.Schema != reviewLastEventClosureSchema || terminal.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("id-less candidate-causal terminal capture = %#v", terminal)
	}
}

func TestReviewCaptureResultCanonicalizesPublishedLensFormsAndRejectsExplicitInvalidValues(t *testing.T) {
	reviewEnabledHome(t)
	t.Run("long form finding lens", func(t *testing.T) {
		repo, started, _, record := newArtifactReview(t, false)
		result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
		result.Lens = "reliability"
		result.Findings = []facadeFinding{{
			Lens: reviewtransaction.LensReliability, Location: "tracked.txt:1", Severity: "WARNING", Claim: "candidate behavior changed",
			ProofRefs: []string{"tracked.txt:1 changed hunk"},
		}}
		input := filepath.Join(t.TempDir(), "result.json")
		writeReviewCLIJSON(t, input, result)
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
			"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
		}, io.Discard); err != nil {
			t.Fatalf("capture long-form lens: %v", err)
		}
	})

	for _, lens := range []string{"", "review-"} {
		t.Run("explicit result lens "+fmt.Sprintf("%q", lens), func(t *testing.T) {
			repo, started, store, record := newArtifactReview(t, false)
			result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
			payload, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatal(err)
			}
			document["lens"] = lens
			payload, err = json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			input := filepath.Join(t.TempDir(), "result.json")
			if err := os.WriteFile(input, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			err = RunReviewCaptureResult([]string{
				"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
				"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
			}, io.Discard)
			if err == nil {
				t.Fatal("capture accepted an explicit invalid lens")
			}
			assertArtifactRevision(t, store, record.Revision)
		})
	}
}

func TestReviewCaptureResultRejectsInvalidLocationWithActionableDiagnostic(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1-2,3", Severity: "CRITICAL", Claim: "candidate failure",
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
		admissionErr.Diagnostic.Location != "tracked.txt:1-2,3" ||
		admissionErr.Diagnostic.Reason != "line_suffix_not_integer" {
		t.Fatalf("capture diagnostic = %#v", admissionErr.Diagnostic)
	}
}

func TestReviewCaptureResultTerminalCapturePreservesCausalClassification(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo, started, store, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	finding := completed.State.LensResults[0].Findings[0]
	classification := completed.State.Classifications[finding.ID]
	if finding.EvidenceClass != reviewtransaction.EvidenceDeterministic || finding.CausalDisposition != reviewtransaction.CausalIntroduced ||
		classification.Class != reviewtransaction.EvidenceDeterministic || classification.Causality != reviewtransaction.CausalIntroduced ||
		completed.State.Outcomes[finding.ID] != reviewtransaction.OutcomeCorroborated ||
		completed.State.State != reviewtransaction.StateCorrectionRequired || !reflect.DeepEqual(completed.State.FixFindingIDs, []string{finding.ID}) {
		t.Fatalf("causal result was not preserved: finding=%#v classification=%#v state=%q outcomes=%#v fixes=%v",
			finding, classification, completed.State.State, completed.State.Outcomes, completed.State.FixFindingIDs)
	}
}

func TestReviewCaptureResultWaitsForMaintenanceBeforePublication(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

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
func TestReviewCaptureResultConcurrentSelectedLenses(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, record := newArtifactReview(t, true)
	outputs := make([]string, len(record.State.SelectedLenses))
	errs := make([]error, len(record.State.SelectedLenses))
	var wg sync.WaitGroup
	for order, lens := range record.State.SelectedLenses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := filepath.Join(t.TempDir(), fmt.Sprintf("%d.json", order))
			_ = os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, lens, order), 0o600)
			var output bytes.Buffer
			errs[order] = RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", fmt.Sprint(order), "--input", input}, &output)
			outputs[order] = strings.TrimSpace(output.String())
		}()
	}
	wg.Wait()

	terminal, acknowledgements := 0, 0
	for order, output := range outputs {
		if errs[order] != nil {
			t.Errorf("capture %s: %v", record.State.SelectedLenses[order], errs[order])
			continue
		}
		var result struct {
			Schema string                  `json:"schema"`
			State  reviewtransaction.State `json:"state"`
			Lens   string                  `json:"lens"`
			Order  int                     `json:"selected_order"`
		}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("decode capture %s: %v\n%s", record.State.SelectedLenses[order], err, output)
		}
		switch result.Schema {
		case reviewLastEventClosureSchema:
			if result.State != reviewtransaction.StateApproved {
				t.Fatalf("terminal capture %s state = %q, want approved", record.State.SelectedLenses[order], result.State)
			}
			terminal++
		case reviewResultArtifactSchema:
			if result.Lens != record.State.SelectedLenses[order] || result.Order != order {
				t.Fatalf("nonterminal acknowledgement = %#v, want captured lens %q at order %d", result, record.State.SelectedLenses[order], order)
			}
			acknowledgements++
		default:
			t.Fatalf("capture %s returned unexpected schema %q: %s", record.State.SelectedLenses[order], result.Schema, output)
		}
	}
	if terminal != 1 || acknowledgements != len(record.State.SelectedLenses)-1 {
		t.Fatalf("concurrent selected-lens captures = %d terminal + %d acknowledgements, want 1 + %d; errors=%v", terminal, acknowledgements, len(record.State.SelectedLenses)-1, errs)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
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
	writeReviewStartCandidate(t, repo, name, "candidate\n", 0o644)
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

func TestReviewStatusClassifiesCapturedReviewerSlots(t *testing.T) {
	reviewEnabledHome(t)
	tests := []struct {
		name, wantKind, wantReason string
		capture, high              bool
		mutate                     func(*testing.T, string)
	}{
		{"clean pending", "collect", "reviewer_results_required", false, false, nil},
		{"complete last event burns", "execute", "fresh_target_ready", true, false, nil},
		{"missing payload", "stop", "captured_artifacts_unverifiable", true, true, func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing digest sidecar", "stop", "captured_artifacts_unverifiable", true, true, func(t *testing.T, path string) {
			if err := os.Remove(path + ".sha256"); err != nil {
				t.Fatal(err)
			}
		}},
		{"alternate names ignored", "collect", "reviewer_results_required", false, false, func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+".alternate", []byte("unrelated\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unrelated entries ignored", "collect", "reviewer_results_required", false, false, func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			for index := 0; index < 33; index++ {
				if err := os.WriteFile(filepath.Join(filepath.Dir(path), fmt.Sprintf("unrelated-%02d", index)), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, started, store, record := newArtifactReview(t, test.high)
			path := filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir, fmt.Sprintf("00-%s.json", record.State.SelectedLenses[0]))
			if test.capture {
				input := filepath.Join(t.TempDir(), "result.json")
				if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := RunReviewCaptureResult([]string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			if test.mutate != nil {
				test.mutate(t, path)
			}
			var output bytes.Buffer
			if err := RunReviewStatus([]string{"--cwd", repo, "--lineage", started.LineageID, "--contract", ReviewIntegrationContractV1, "--next-transition"}, &output); err != nil {
				t.Fatal(err)
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.NextTransition == nil || string(status.NextTransition.Kind) != test.wantKind || status.NextTransition.ReasonCode != test.wantReason {
				t.Fatalf("slot status = %#v", status.NextTransition)
			}
		})
	}
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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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
