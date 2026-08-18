// This file carries the review-lifecycle-hardening batch's per-issue,
// end-to-end traceability: one named subtest per defect, grouped into the
// seven mechanism journeys the design lays out. It is kept separate from
// organic_runtime_test.go so that file stays reviewable on its own.
package organicruntime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// currentChangesTargetIdentity derives the exact frozen target identity for
// the live workspace the same way a real negotiated caller must: by building
// the snapshot through the shipped SnapshotBuilder before naming --target.
func currentChangesTargetIdentity(t *testing.T, repo string) string {
	t.Helper()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, err := builder.DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatalf("discover intended untracked candidate: %v", err)
	}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: intended,
	})
	if err != nil {
		t.Fatalf("build candidate snapshot: %v", err)
	}
	return snapshot.Identity
}

// organicLargeWorkspaceE2EEnvironment gates the deliberately heavy lifecycle
// proof below. The fixture catches regressions from batched Git inventories
// back to per-path subprocess loops across START, capture, and finalize.
const organicLargeWorkspaceE2EEnvironment = "GENTLE_AI_LARGE_WORKSPACE_E2E"

// TestOrganicReviewStartDeadlineHeadroom now covers the #1957 remainder of
// #1778: later capture and finalize operations share the same bounded topology.
func TestOrganicReviewStartDeadlineHeadroom(t *testing.T) {
	t.Run("issue-1778", func(t *testing.T) {
		if os.Getenv(organicLargeWorkspaceE2EEnvironment) != "1" {
			t.Skip("set GENTLE_AI_LARGE_WORKSPACE_E2E=1 to run the large-workspace START/capture/finalize proof")
		}
		harness := newOrganicHarness(t)
		lineage := "organic-large-workspace-lifecycle"

		const headroomFiles = 5000
		files := make(map[string]string, headroomFiles)
		for index := 0; index < headroomFiles; index++ {
			files[fmt.Sprintf("bulk/headroom-%05d.mdx", index)] = "export const value = 1\n"
		}
		harness.writeFiles(files)

		// The negotiated (agent-integration) START contract requires the caller
		// to name the exact frozen target identity up front. This precompute is
		// not itself subject to the facade deadline; only the timed call below is.
		targetIdentity := currentChangesTargetIdentity(t, harness.repo.worktree)

		lifecycleStarted := time.Now()
		startedAt := time.Now()
		stdout, stderr, err := harness.gentleAllowFailure(
			"review", "start", "--cwd", harness.repo.worktree,
			"--lineage", lineage,
			"--contract", "gentle-ai.review-integration/v1",
			"--target", targetIdentity,
			"--projection", "workspace",
		)
		startElapsed := time.Since(startedAt)
		if err != nil {
			t.Fatalf("negotiated review start on a large valid workspace failed after %s: %v\nstdout:\n%s\nstderr:\n%s", startElapsed, err, stdout, stderr)
		}
		if startElapsed >= organicLocalTimeout {
			t.Fatalf("negotiated review start took %s, want less than the harness local timeout %s", startElapsed, organicLocalTimeout)
		}
		var started organicStartResult
		if err := json.Unmarshal([]byte(stdout), &started); err != nil {
			t.Fatalf("decode large-workspace START: %v\n%s", err, stdout)
		}
		if returnedTargetIdentity := started.targetIdentity(); returnedTargetIdentity != targetIdentity {
			t.Fatalf("START target identity = %q, want %q", returnedTargetIdentity, targetIdentity)
		}
		if len(started.SelectedLenses) != 1 {
			t.Fatalf("large active-MDX workspace selected lenses = %v, want one consolidated lens", started.SelectedLenses)
		}
		finalized := harness.approveReview(lineage, started)
		lifecycleElapsed := time.Since(lifecycleStarted)
		if finalized.State != organicStateApproved {
			t.Fatalf("large-workspace lifecycle finalized as %q, want %q", finalized.State, organicStateApproved)
		}
		if lifecycleElapsed >= organicLocalTimeout {
			t.Fatalf("large-workspace lifecycle took %s, want less than the harness local timeout %s", lifecycleElapsed, organicLocalTimeout)
		}
		t.Logf("large-workspace lifecycle completed %d files: START=%s total=%s", headroomFiles, startElapsed, lifecycleElapsed)

		harness.assertSingleReviewLineage(lineage)
		harness.assertNoSDDArtifacts()
	})
}

// TestOrganicReviewLifecycleErrorTyping proves Group A: published-delivery
// pre-push classification routes into the existing scope-changed terminal
// instead of authority_corrupted (1801-adjacent release blocker), candidate-
// causal admission canonicalizes both sides before comparing (1699), and the
// operation_outcome_unknown envelope carries its wrapped native cause instead
// of a fixed placeholder (1666, 1807).
func TestOrganicReviewLifecycleErrorTyping(t *testing.T) {
	t.Run("issue-1801-published-delivery", func(t *testing.T) {
		for _, mode := range []struct {
			name    string
			disable bool
		}{
			{name: "disabled-reports-unmanaged", disable: true},
			{name: "enabled-reports-typed-scope-changed", disable: false},
		} {
			t.Run(mode.name, func(t *testing.T) {
				harness := newOrganicHarness(t)
				lineage := "organic-published-delivery-" + mode.name
				harness.writeFiles(map[string]string{"tracked.txt": "reviewed candidate behavior\n"})
				started, _ := harness.startReview(lineage)
				approved := harness.approveReview(lineage, started)
				if approved.State != organicStateApproved {
					t.Fatalf("published-delivery fixture did not reach approved: %#v", approved)
				}
				harness.git("add", "-A")
				harness.git("commit", "-qm", "reviewed candidate")
				// Revert to the exact original content: this second commit's tree is
				// byte-identical to the receipt's reviewed base tree, so the
				// publication range now contains two commits whose tree equals the
				// reviewed tree — the historical base commit and this revert — which
				// is the published-delivery ambiguous-match shape 1153 exists for.
				harness.writeFiles(map[string]string{"tracked.txt": "organic runtime\n"})
				harness.git("commit", "-qam", "revert to reviewed tree")

				if mode.disable {
					harness.disableReview()
				}

				result := harness.gateAllowFailure(string(reviewtransaction.GatePrePush))
				if mode.disable {
					if result.Delivery != "disabled/unmanaged" {
						t.Fatalf("disabled published-delivery gate = %#v, want delivery disabled/unmanaged", result)
					}
					if result.Allowed || result.Result == string(reviewtransaction.GateAllow) {
						t.Fatalf("disabled published-delivery gate fabricated an approval: %#v", result)
					}
					return
				}
				if result.Result != string(reviewtransaction.GateScopeChanged) {
					t.Fatalf("enabled published-delivery gate result = %q, want %q (not authority_corrupted)", result.Result, reviewtransaction.GateScopeChanged)
				}
				if result.Context.Denial == nil || result.Context.Denial.Code != "delivery-base-ambiguous" {
					t.Fatalf("enabled published-delivery gate denial = %#v, want delivery-base-ambiguous", result.Context.Denial)
				}
			})
		}
	})

	t.Run("issue-1699", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-candidate-causal-canonicalization"
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("candidate causal proof line", 6)})
		started, _ := harness.startReview(lineage)
		if len(started.SelectedLenses) == 0 || started.TargetIdentity == "" {
			t.Fatalf("expected at least one selected lens and a target identity: %#v", started)
		}
		lens := started.SelectedLenses[0]
		prefix, known := map[string]string{
			"review-risk": "R1-", "review-readability": "R2-", "review-reliability": "R3-", "review-resilience": "R4-",
		}[lens]
		if !known {
			t.Fatalf("unrecognized selected lens %q", lens)
		}

		preflightPayload := harness.gentle(
			"review", "capture-result", "--cwd", harness.repo.worktree,
			"--lineage", lineage, "--target", started.TargetIdentity,
			"--lens", lens, "--order", "0", "--preflight",
		)
		var preflight struct {
			ArtifactSubject struct {
				SubjectHash string `json:"subject_hash"`
			} `json:"artifact_subject"`
		}
		if err := json.Unmarshal(preflightPayload, &preflight); err != nil {
			t.Fatalf("decode capture-result preflight: %v\n%s", err, preflightPayload)
		}
		if preflight.ArtifactSubject.SubjectHash == "" {
			t.Fatalf("capture-result preflight omitted the artifact subject hash: %s", preflightPayload)
		}

		// The submitted candidate-causal finding ID carries leading/trailing
		// whitespace an agent transport could plausibly introduce. Before the
		// fix, AdmitArtifact compared the canonicalized verified IDs against
		// this raw, non-canonical submission and rejected it out_of_scope even
		// though it names the same proven finding.
		payload := struct {
			SubjectHash string                   `json:"subject_hash"`
			Inspection  organicCaptureInspection `json:"inspection"`
			Findings    []organicFinding         `json:"findings"`
			Evidence    []string                 `json:"evidence"`
		}{
			SubjectHash: preflight.ArtifactSubject.SubjectHash,
			Inspection:  organicCaptureInspection{Status: "completed", Paths: []string{"tracked.txt"}},
			Findings: []organicFinding{{
				ID: "  " + prefix + "001  ", Location: "tracked.txt:1", Severity: "BLOCKER",
				Claim:             "the candidate introduces an unreviewed causal defect",
				ProofRefs:         []string{"diff: tracked.txt:1"},
				EvidenceClass:     "deterministic",
				CausalDisposition: "introduced",
			}},
			Evidence: []string{"inspected every frozen candidate path for " + lens},
		}
		resultPath := harness.writeJSON("candidate-causal-result.json", payload)

		captured := harness.gentle(
			"review", "capture-result", "--cwd", harness.repo.worktree,
			"--lineage", lineage, "--target", started.TargetIdentity,
			"--lens", lens, "--order", "0", "--input", resultPath,
		)
		var artifact struct {
			AdmissionDecision string `json:"admission_decision"`
			Path              string `json:"path"`
		}
		if err := json.Unmarshal(captured, &artifact); err != nil {
			t.Fatalf("decode capture-result: %v\n%s", err, captured)
		}
		if artifact.AdmissionDecision != "completed" {
			t.Fatalf("candidate-causal admission = %q, want completed (non-canonical id must still admit)", artifact.AdmissionDecision)
		}
		envelope, err := os.ReadFile(artifact.Path)
		if err != nil {
			t.Fatalf("read captured reviewer artifact: %v", err)
		}
		var stored struct {
			Admission struct {
				CandidateCausalFindingIDs []string `json:"candidate_causal_finding_ids"`
			} `json:"admission"`
		}
		if err := json.Unmarshal(envelope, &stored); err != nil {
			t.Fatalf("decode stored reviewer artifact: %v\n%s", err, envelope)
		}
		wantID := prefix + "001"
		if len(stored.Admission.CandidateCausalFindingIDs) != 1 || stored.Admission.CandidateCausalFindingIDs[0] != wantID {
			t.Fatalf("admitted candidate-causal finding ids = %v, want canonical [%s]", stored.Admission.CandidateCausalFindingIDs, wantID)
		}
	})

	t.Run("issue-1699-id-less-candidate-causal-finding", func(t *testing.T) {
		// A community reviewer (ftorga) proved the Group A canonicalization fix
		// above did not cover the reachable shape: a severe candidate-causal
		// finding submitted with NO "id" at all (not merely a non-canonical
		// one). verifiedCandidateCausalFindingIDs was reading the raw,
		// pre-canonicalization result, so the omitted ID could never match the
		// canonical fallback ID (`R#-001`) CanonicalCompactLensResult assigns
		// inside admission, permanently producing out-of-scope/incomplete.
		harness := newOrganicHarness(t)
		lineage := "organic-candidate-causal-id-less"
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("candidate causal proof line", 6)})
		started, _ := harness.startReview(lineage)
		if len(started.SelectedLenses) == 0 || started.TargetIdentity == "" {
			t.Fatalf("expected at least one selected lens and a target identity: %#v", started)
		}
		lens := started.SelectedLenses[0]
		prefix, known := map[string]string{
			"review-risk": "R1-", "review-readability": "R2-", "review-reliability": "R3-", "review-resilience": "R4-",
		}[lens]
		if !known {
			t.Fatalf("unrecognized selected lens %q", lens)
		}

		preflightPayload := harness.gentle(
			"review", "capture-result", "--cwd", harness.repo.worktree,
			"--lineage", lineage, "--target", started.TargetIdentity,
			"--lens", lens, "--order", "0", "--preflight",
		)
		var preflight struct {
			ArtifactSubject struct {
				SubjectHash string `json:"subject_hash"`
			} `json:"artifact_subject"`
		}
		if err := json.Unmarshal(preflightPayload, &preflight); err != nil {
			t.Fatalf("decode capture-result preflight: %v\n%s", err, preflightPayload)
		}

		payload := struct {
			SubjectHash string                   `json:"subject_hash"`
			Inspection  organicCaptureInspection `json:"inspection"`
			Findings    []organicFinding         `json:"findings"`
			Evidence    []string                 `json:"evidence"`
		}{
			SubjectHash: preflight.ArtifactSubject.SubjectHash,
			Inspection:  organicCaptureInspection{Status: "completed", Paths: []string{"tracked.txt"}},
			Findings: []organicFinding{{
				// ID intentionally omitted entirely (json:"id,omitempty").
				Location: "tracked.txt:1", Severity: "BLOCKER",
				Claim:             "the candidate introduces an unreviewed causal defect",
				ProofRefs:         []string{"diff: tracked.txt:1"},
				EvidenceClass:     "deterministic",
				CausalDisposition: "introduced",
			}},
			Evidence: []string{"inspected every frozen candidate path for " + lens},
		}
		resultPath := harness.writeJSON("candidate-causal-id-less-result.json", payload)

		captured := harness.gentle(
			"review", "capture-result", "--cwd", harness.repo.worktree,
			"--lineage", lineage, "--target", started.TargetIdentity,
			"--lens", lens, "--order", "0", "--input", resultPath,
		)
		var artifact struct {
			AdmissionDecision string `json:"admission_decision"`
			Path              string `json:"path"`
		}
		if err := json.Unmarshal(captured, &artifact); err != nil {
			t.Fatalf("decode capture-result: %v\n%s", err, captured)
		}
		if artifact.AdmissionDecision != "completed" {
			t.Fatalf("id-less candidate-causal admission = %q, want completed", artifact.AdmissionDecision)
		}
		envelope, err := os.ReadFile(artifact.Path)
		if err != nil {
			t.Fatalf("read captured reviewer artifact: %v", err)
		}
		var stored struct {
			Admission struct {
				CandidateCausalFindingIDs []string `json:"candidate_causal_finding_ids"`
			} `json:"admission"`
		}
		if err := json.Unmarshal(envelope, &stored); err != nil {
			t.Fatalf("decode stored reviewer artifact: %v\n%s", err, envelope)
		}
		wantID := prefix + "001"
		if len(stored.Admission.CandidateCausalFindingIDs) != 1 || stored.Admission.CandidateCausalFindingIDs[0] != wantID {
			t.Fatalf("admitted candidate-causal finding ids = %v, want canonical fallback [%s]", stored.Admission.CandidateCausalFindingIDs, wantID)
		}
	})

	t.Run("issue-1666", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-operation-outcome-cause-1666"
		harness.writeFiles(map[string]string{"tracked.txt": "cause envelope candidate\n"})
		targetIdentity := currentChangesTargetIdentity(t, harness.repo.worktree)
		missingPolicy := filepath.Join(harness.repo.worktree, "missing-policy.json")

		stdout, stderr, err := harness.gentleAllowFailure(
			"review", "start", "--cwd", harness.repo.worktree,
			"--lineage", lineage,
			"--contract", "gentle-ai.review-integration/v1",
			"--target", targetIdentity,
			"--projection", "workspace",
			"--policy", missingPolicy,
		)
		if err == nil {
			t.Fatalf("negotiated start with an unreadable policy file succeeded\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
		}
		failure := decodeOrganicIntegrationFailure(t, stdout)
		if failure.Code != "operation_outcome_unknown" || failure.Phase != "native_running" || failure.MutationOutcome != "unknown" {
			t.Fatalf("operation_outcome_unknown envelope changed byte-identical fields = %#v", failure)
		}
		if !strings.Contains(failure.Cause, "read facade review policy") {
			t.Fatalf("operation_outcome_unknown envelope did not carry the wrapped native cause: %#v", failure)
		}
	})

	t.Run("issue-1832", func(t *testing.T) {
		// A disposable repository with no remote and no branch upstream: no
		// publication boundary to derive at all. That is not authority
		// damage, and while receipt-driven development is off it is not
		// something pre-push should block on.
		harness := newOrganicHarness(t)
		harness.git("remote", "remove", "origin")
		lineage := "organic-no-upstream-disabled-1832"
		harness.writeFiles(map[string]string{"tracked.txt": "reviewed candidate behavior\n"})
		started, _ := harness.startReview(lineage)
		approved := harness.approveReview(lineage, started)
		if approved.State != organicStateApproved {
			t.Fatalf("no-upstream fixture did not reach approved: %#v", approved)
		}
		harness.git("add", "-A")
		harness.git("commit", "-qm", "reviewed candidate")

		harness.disableReview()

		result := harness.gate(string(reviewtransaction.GatePrePush))
		if result.Delivery != "disabled/unmanaged" {
			t.Fatalf("disabled pre-push with no upstream gate = %#v, want delivery disabled/unmanaged", result)
		}
		if result.Allowed || result.Result == string(reviewtransaction.GateAllow) {
			t.Fatalf("disabled pre-push with no upstream fabricated an approval: %#v", result)
		}
		// Wave 5 Slice 2 (design decision 4): the switch is consulted before
		// any authority read, so the no-upstream boundary is never even
		// derived while disabled -- no discovery-kind detail leaks.
		if result.Context.Denial != nil {
			t.Fatalf("disabled pre-push with no upstream leaked discovery-kind detail: %#v", result.Context.Denial)
		}
	})

	t.Run("issue-1807", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-operation-outcome-cause-1807"
		harness.writeFiles(map[string]string{"tracked.txt": "cause envelope candidate two\n"})
		targetIdentity := currentChangesTargetIdentity(t, harness.repo.worktree)
		// A directory where the policy file is expected is a different concrete
		// native failure than a missing file (issue-1666), reached through the
		// identical unwrapped facadePolicyBytes seam: os.ReadFile on a directory
		// returns "is a directory" instead of "no such file".
		directoryAsPolicy := harness.repo.worktree

		stdout, stderr, err := harness.gentleAllowFailure(
			"review", "start", "--cwd", harness.repo.worktree,
			"--lineage", lineage,
			"--contract", "gentle-ai.review-integration/v1",
			"--target", targetIdentity,
			"--projection", "workspace",
			"--policy", directoryAsPolicy,
		)
		if err == nil {
			t.Fatalf("negotiated start with a directory as the policy file succeeded\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
		}
		failure := decodeOrganicIntegrationFailure(t, stdout)
		if failure.Code != "operation_outcome_unknown" || failure.Phase != "native_running" || failure.MutationOutcome != "unknown" {
			t.Fatalf("operation_outcome_unknown envelope changed byte-identical fields = %#v", failure)
		}
		if !strings.Contains(failure.Cause, "read facade review policy") {
			t.Fatalf("operation_outcome_unknown envelope did not carry the wrapped native cause: %#v", failure)
		}
	})
}

type organicCaptureInspection struct {
	Status string   `json:"status"`
	Paths  []string `json:"paths"`
}

type organicIntegrationFailure struct {
	Code            string `json:"code"`
	Phase           string `json:"phase"`
	MutationOutcome string `json:"mutation_outcome"`
	Cause           string `json:"cause"`
}

func decodeOrganicIntegrationFailure(t *testing.T, payload string) organicIntegrationFailure {
	t.Helper()
	var failure organicIntegrationFailure
	if err := json.Unmarshal([]byte(payload), &failure); err != nil {
		t.Fatalf("decode negotiated review failure: %v\n%s", err, payload)
	}
	return failure
}

type organicNextTransitionArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Token string `json:"token"`
}

type organicNextTransitionExecute struct {
	Operation string                          `json:"operation"`
	Arguments []organicNextTransitionArgument `json:"arguments"`
}

type organicNextTransition struct {
	Kind       string                        `json:"kind"`
	ReasonCode string                        `json:"reason_code"`
	Execute    *organicNextTransitionExecute `json:"execute"`
}

type organicStatusAuthority struct {
	Revision string `json:"revision"`
}

type organicStatusResult struct {
	TargetIdentity string                 `json:"target_identity"`
	Authority      organicStatusAuthority `json:"authority"`
	NextTransition *organicNextTransition `json:"next_transition"`
}

// harnessCaptureResultSubjectHash preflights one capture-result binding and
// returns the artifact subject hash the real payload must echo.
func harnessCaptureResultSubjectHash(t *testing.T, harness *organicHarness, lineage, target, lens string, order int) string {
	t.Helper()
	payload := harness.gentle(
		"review", "capture-result", "--cwd", harness.repo.worktree,
		"--lineage", lineage, "--target", target, "--lens", lens, "--order", fmt.Sprintf("%d", order), "--preflight",
	)
	var preflight struct {
		ArtifactSubject struct {
			SubjectHash string `json:"subject_hash"`
		} `json:"artifact_subject"`
	}
	if err := json.Unmarshal(payload, &preflight); err != nil {
		t.Fatalf("decode capture-result preflight: %v\n%s", err, payload)
	}
	if preflight.ArtifactSubject.SubjectHash == "" {
		t.Fatalf("capture-result preflight omitted the artifact subject hash: %s", payload)
	}
	return preflight.ArtifactSubject.SubjectHash
}

// harnessCaptureCleanResult captures one clean (no findings) reviewer result
// through the real provider-owned review capture-result command, for every
// selected lens in order, so the candidate reaches natively captured
// artifacts the same way a real reviewer client does.
func harnessCaptureCleanResults(t *testing.T, harness *organicHarness, lineage, target string, lenses []string) {
	t.Helper()
	for index, lens := range lenses {
		subjectHash := harnessCaptureResultSubjectHash(t, harness, lineage, target, lens, index)
		payload := struct {
			SubjectHash string                   `json:"subject_hash"`
			Inspection  organicCaptureInspection `json:"inspection"`
			Findings    []organicFinding         `json:"findings"`
			Evidence    []string                 `json:"evidence"`
		}{
			SubjectHash: subjectHash,
			Inspection:  organicCaptureInspection{Status: "completed", Paths: []string{"tracked.txt"}},
			Findings:    []organicFinding{},
			Evidence:    []string{"inspected every frozen candidate path for " + lens},
		}
		resultPath := harness.writeJSON(fmt.Sprintf("clean-capture-%d.json", index), payload)
		harness.gentle(
			"review", "capture-result", "--cwd", harness.repo.worktree,
			"--lineage", lineage, "--target", target, "--lens", lens, "--order", fmt.Sprintf("%d", index), "--input", resultPath,
		)
	}
}

func harnessStatus(t *testing.T, harness *organicHarness, lineage string, extra ...string) organicStatusResult {
	t.Helper()
	arguments := []string{"review", "status", "--cwd", harness.repo.worktree, "--lineage", lineage, "--contract", "gentle-ai.review-integration/v1"}
	arguments = append(arguments, extra...)
	payload := harness.gentle(arguments...)
	var status organicStatusResult
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("decode review status: %v\n%s", err, payload)
	}
	return status
}

// TestOrganicReviewExecutableTransitionContract proves Group B: emitted
// transition arguments are literally executable (1745), review schema
// publishes the verification-evidence contract (1775), a lineage-only
// finalize consumes canonical captured evidence instead of ignoring it
// (1663), repeating it after results were consumed is a typed rejection
// instead of a silent success (1788), and next-transition surfaces the
// escalate-to-recover continuation (1800).
func TestOrganicReviewExecutableTransitionContract(t *testing.T) {
	t.Run("issue-1745", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-executable-token-1745"
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("executable token candidate", 6)})
		started, _ := harness.startReview(lineage)
		if len(started.SelectedLenses) == 0 || started.TargetIdentity == "" {
			t.Fatalf("expected at least one selected lens and a target identity: %#v", started)
		}
		harnessCaptureCleanResults(t, harness, lineage, started.TargetIdentity, started.SelectedLenses)

		status := harnessStatus(t, harness, lineage, "--next-transition")
		if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.finalize" {
			t.Fatalf("expected an executable review.finalize transition with captured artifacts: %#v", status.NextTransition)
		}
		var capturedResultsToken string
		for _, argument := range status.NextTransition.Execute.Arguments {
			if argument.Name == "captured_results" {
				capturedResultsToken = argument.Token
			}
		}
		if capturedResultsToken != "--captured-results=true" {
			t.Fatalf("captured_results token = %q, want the exact executable --captured-results=true", capturedResultsToken)
		}

		// Run the emitted token literally, exactly as printed, without any
		// reformatting (no underscore-to-hyphen translation, no space-joining a
		// detached boolean).
		stdout, stderr, err := harness.gentleAllowFailure(
			"review", "finalize", "--cwd", harness.repo.worktree, "--lineage", lineage, capturedResultsToken,
		)
		if err != nil {
			t.Fatalf("running the printed token literally failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		var finalized organicFinalizeResult
		if err := json.Unmarshal([]byte(stdout), &finalized); err != nil {
			t.Fatalf("decode literal-token finalize: %v\n%s", err, stdout)
		}
		if finalized.State != organicStateValidating {
			t.Fatalf("literal-token finalize state = %q, want %q", finalized.State, organicStateValidating)
		}
	})

	t.Run("issue-1775", func(t *testing.T) {
		harness := newOrganicHarness(t)
		payload := harness.gentle("review", "schema", "verification-evidence")
		var schema struct {
			ID        string `json:"$id"`
			Type      string `json:"type"`
			MinLength int    `json:"minLength"`
		}
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatalf("decode verification-evidence schema: %v\n%s", err, payload)
		}
		if schema.ID != "https://gentle-ai.dev/schema/review/verification-evidence/v1" || schema.Type != "string" || schema.MinLength != 1 {
			t.Fatalf("verification-evidence schema = %#v", schema)
		}

		_, stderr, err := harness.gentleAllowFailure("review", "schema")
		if err == nil || !strings.Contains(stderr, "verification-evidence") {
			t.Fatalf("review schema usage error = %v, stderr %q, want it to name verification-evidence", err, stderr)
		}
	})

	t.Run("issue-1663-and-1788", func(t *testing.T) {
		reach := func(t *testing.T, suffix string) (*organicHarness, string) {
			t.Helper()
			harness := newOrganicHarness(t)
			lineage := "organic-finalize-no-op-" + suffix
			harness.writeFiles(map[string]string{"tracked.txt": organicLines("finalize no-op candidate", 6)})
			started, _ := harness.startReview(lineage)
			if len(started.SelectedLenses) == 0 {
				t.Fatal("expected at least one selected lens")
			}
			harnessCaptureCleanResults(t, harness, lineage, started.TargetIdentity, started.SelectedLenses)
			finalized := harness.finalize(lineage, "--captured-results")
			if finalized.State != organicStateValidating {
				t.Fatalf("fixture did not reach validating: %#v", finalized)
			}
			return harness, lineage
		}

		t.Run("issue-1663", func(t *testing.T) {
			harness, lineage := reach(t, "1663")
			status := harnessStatus(t, harness, lineage)
			evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
			if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			harness.gentle(
				"review", "capture-evidence", "--cwd", harness.repo.worktree, "--lineage", lineage,
				"--target", status.TargetIdentity, "--expected-revision", status.Authority.Revision, "--outcome", "passed", "--input", evidencePath,
			)
			stdout, stderr, err := harness.gentleAllowFailure("review", "finalize", "--cwd", harness.repo.worktree, "--lineage", lineage)
			if err != nil {
				t.Fatalf("lineage-only finalize with canonical captured evidence failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			var finalized organicFinalizeResult
			if err := json.Unmarshal([]byte(stdout), &finalized); err != nil {
				t.Fatalf("decode finalize: %v\n%s", err, stdout)
			}
			if finalized.State == organicStateValidating {
				t.Fatalf("lineage-only finalize did not consume canonical captured evidence, stayed validating: %#v", finalized)
			}
		})

		t.Run("issue-1788", func(t *testing.T) {
			harness, lineage := reach(t, "1788")
			stdout, stderr, err := harness.gentleAllowFailure("review", "finalize", "--cwd", harness.repo.worktree, "--lineage", lineage)
			if err == nil {
				t.Fatalf("lineage-only finalize with no evidence anywhere silently succeeded\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
			if !strings.Contains(stderr, "gentle-ai review capture-evidence") || !strings.Contains(stderr, "gentle-ai review finalize --lineage "+lineage+" --captured-evidence") {
				t.Fatalf("no-transition rejection did not name the escape verbatim: stderr=%q", stderr)
			}
		})
	})

	t.Run("issue-1800", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-escalate-to-recover-1800"
		// A committed, unchanged production path plus a genuinely new candidate
		// file: the reviewer will claim the unchanged path is "introduced",
		// which candidate-causal verification proves false, escalating the
		// authority directly from reviewing.
		harness.writeFiles(map[string]string{"legacy.txt": "unchanged production content\n"})
		harness.git("add", "-A")
		harness.git("commit", "-qm", "add unchanged production path")
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("genuine candidate content", 4)})

		started, _ := harness.startReview(lineage)
		// The claim is refused at admission now, by the repository-derived
		// changed-line evidence finalize used to consult after the fact. Refusing
		// an unprovable candidate-causal claim before it enters authority is the
		// stronger outcome, so this asserts the refusal rather than the escalation
		// that route no longer reaches.
		_, stderr, err := harness.captureReviewerResult(lineage, started, 0, organicReviewerResult{
			Lens: started.SelectedLenses[0],
			Findings: []organicFinding{{
				Location: "legacy.txt:1", Severity: "CRITICAL", Claim: "an unreviewed defect in the unchanged production path",
				ProofRefs:     []string{"the frozen candidate deterministically reproduces the defect"},
				EvidenceClass: "deterministic", CausalDisposition: "introduced",
			}},
			Evidence: []string{"reproduced the defect without candidate-causality evidence"},
		})
		if err == nil {
			t.Fatal("a false-introduced finding on an unchanged path was admitted")
		}
		if !strings.Contains(stderr, "out_of_scope") {
			t.Fatalf("false-introduced admission refusal was not typed out of scope: %s", stderr)
		}

		// A refused admission leaves the lens slot unconsumed, so the same lineage
		// still admits a provable blocker. Escalation is then reached the way it
		// remains reachable: a correction forecast past the frozen budget.
		harness.captureReviewerResultOrFail(lineage, started, 0, organicReviewerResult{
			Lens: started.SelectedLenses[0],
			Findings: []organicFinding{{
				Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "the candidate content is wrong",
				ProofRefs:     []string{"a differential test passes on base and fails on the candidate"},
				EvidenceClass: "deterministic", CausalDisposition: "introduced",
			}},
			Evidence: []string{"the focused differential test failed on the candidate"},
		})
		if escalated := harness.finalize(lineage, "--captured-results=true", "--correction-lines", "1000"); escalated.State != "escalated" {
			t.Fatalf("over-budget correction forecast did not escalate: %#v", escalated)
		}

		// Change the live target so escalated recovery has a real changed
		// target to recover onto (validateCompactRecoveryEdge requires it).
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("genuine candidate content, revised", 5)})

		unauthorized := harnessStatus(t, harness, lineage, "--next-transition")
		liveTarget := unauthorized.TargetIdentity
		successor, actor, reason := "organic-escalate-to-recover-1800-successor", "maintainer", "authorized escalated recovery"
		// A status call always carries a (possibly default) selector, so the
		// authorization binding must include the successor lineage line —
		// matching reviewTransitionRecoveryAuthorization's contract, not the
		// selector-free shape.
		authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + lineage +
			"\npredecessor_revision=" + unauthorized.Authority.Revision + "\ntarget_identity=" + liveTarget +
			"\nsuccessor_lineage=" + successor + "\nactor=" + actor + "\nreason=" + reason

		status := harnessStatus(t, harness, lineage, "--next-transition",
			"--recovery-successor-lineage", successor,
			"--recovery-reason", reason, "--recovery-actor", actor, "--recovery-authorization", authorization,
		)
		if status.NextTransition == nil || status.NextTransition.Kind != "execute" || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.recover" {
			t.Fatalf("escalated next-transition = %#v, want an executable review.recover continuation", status.NextTransition)
		}
		var disposition string
		for _, argument := range status.NextTransition.Execute.Arguments {
			if argument.Name == "disposition" {
				disposition = argument.Value
			}
		}
		if disposition != "escalated" {
			t.Fatalf("escalated recovery disposition = %q, want escalated", disposition)
		}

		harness.assertNoSDDArtifacts()
	})
}

// initOrganicUnbornRepository builds a fresh worktree whose HEAD has never
// been committed, without the seeded commit and remote push newOrganicHarness
// always performs. Group G's 1771 and 1641 subtests need a genuinely unborn
// HEAD, which the shared harness fixture cannot represent.
func initOrganicUnbornRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "--quiet", "--initial-branch=main", "."},
		{"config", "user.name", "Organic E2E"},
		{"config", "user.email", "organic-e2e@example.invalid"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := organicGitOutput(context.Background(), repo, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

// TestOrganicReviewTargetShapeRefusals proves Group G: staged base-diff input
// is canonicalized to its committed-only route, so an empty candidate names
// that real continuation rather than the retired staged/base-ref ambiguity;
// a selector-free status call on an unborn HEAD resolves to the empty-tree
// projection instead of surfacing a raw Git command failure (1771); and a
// first publication attempted from an empty-base receipt is refused, naming
// the proven in-product escape verbatim (1641).
func TestOrganicReviewTargetShapeRefusals(t *testing.T) {
	t.Run("issue-1812", func(t *testing.T) {
		harness := newOrganicHarness(t)
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("staged candidate", 4)})
		harness.git("add", "--", "tracked.txt")
		base := strings.TrimSpace(harness.git("rev-parse", "HEAD"))

		_, stderr, err := harness.gentleAllowFailure(
			"review", "start", "--cwd", harness.repo.worktree,
			"--projection", "staged", "--base-ref", base, "--committed-only",
		)
		if err == nil {
			t.Fatal("staged projection + base-ref start unexpectedly succeeded")
		}
		if strings.Contains(stderr, "intent is ambiguous") || !strings.Contains(stderr, "candidate has no pending changes") ||
			!strings.Contains(stderr, "--base-ref <commit>") {
			t.Fatalf("staged base-diff empty-candidate continuation = %q", stderr)
		}
		harness.assertNoSDDArtifacts()
	})

	t.Run("issue-1771", func(t *testing.T) {
		harness := newOrganicHarnessForWorktree(t, initOrganicUnbornRepository(t))
		harness.writeFiles(map[string]string{"candidate.txt": organicLines("unborn selector-free candidate", 4)})
		harness.git("add", "--", "candidate.txt")

		stdout, stderr, err := harness.gentleAllowFailure(
			"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v1",
		)
		if err != nil {
			t.Fatalf("selector-free status on unborn HEAD failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		var status organicStatusResult
		if err := json.Unmarshal([]byte(stdout), &status); err != nil {
			t.Fatalf("decode selector-free unborn status: %v\n%s", err, stdout)
		}
		if status.TargetIdentity == "" {
			t.Fatalf("selector-free unborn status = %#v, want a resolved target identity instead of a raw Git failure", status)
		}

		// Community-confirmed repro (reporter lu149e): the plain direct-start
		// path (default workspace projection, no --projection flag) hits the
		// exact same resolveCurrentChangesBase seam and used to fail with
		// `build facade review target: git rev-parse --verify HEAD^{tree}
		// failed with exit code 128: fatal: Needed a single revision`.
		startStdout, startStderr, startErr := harness.gentleAllowFailure(
			"review", "start", "--cwd", harness.repo.worktree,
		)
		if startErr != nil {
			t.Fatalf("direct-start on unborn HEAD (default projection) failed: %v\nstdout:\n%s\nstderr:\n%s", startErr, startStdout, startStderr)
		}
		var started organicStartResult
		if err := json.Unmarshal([]byte(startStdout), &started); err != nil {
			t.Fatalf("decode direct-start on unborn HEAD: %v\n%s", err, startStdout)
		}
		if started.TargetIdentity == "" {
			t.Fatalf("direct-start on unborn HEAD = %#v, want a resolved target identity instead of a raw Git failure", started)
		}
	})

	t.Run("issue-1641", func(t *testing.T) {
		harness := newOrganicHarnessForWorktree(t, initOrganicUnbornRepository(t))
		harness.writeFiles(map[string]string{"candidate.txt": organicLines("empty-base candidate", 4)})
		harness.git("add", "--", "candidate.txt")

		lineage := "organic-empty-base-1641"
		started, _ := harness.startReview(lineage, "--projection", "staged")
		finalized := harness.approveReview(lineage, started)
		if finalized.State != organicStateApproved {
			t.Fatalf("empty-base candidate did not reach approved: %#v", finalized)
		}
		harness.git("commit", "-qm", "deliver empty-base candidate")

		// The remote must not already carry the delivery, or the push under
		// test transfers nothing and there is no first publication to refuse.
		// An empty remote is that first publication, and it advertises no
		// branch, so pre-push derives its bootstrap boundary without a
		// --base-ref.
		bare := filepath.Join(t.TempDir(), "origin.git")
		harness.git("init", "--bare", "--quiet", bare)
		harness.git("remote", "add", "origin", bare)
		harness.git("config", "branch.main.remote", "origin")
		harness.git("config", "branch.main.merge", "refs/heads/main")

		want := "commit an authorized empty root, then run gentle-ai review start --committed-only with --base-ref set to that commit's SHA"
		result := harness.gateAllowFailure("pre-push", "--lineage", lineage)
		if result.Allowed || !strings.Contains(result.Reason, want) {
			t.Fatalf("pre-push from an empty-base receipt = %#v, want typed refusal naming %q verbatim (1641)", result, want)
		}

		// pre-PR asks whether the receipt may open a pull request against an
		// advertised boundary, not whether a commit moves, so publishing the
		// delivery does not retire its refusal.
		harness.git("push", "--quiet", "origin", "HEAD:refs/heads/main")
		result = harness.gateAllowFailure("pre-pr", "--lineage", lineage, "--base-ref", "origin/main")
		if result.Allowed || !strings.Contains(result.Reason, want) {
			t.Fatalf("pre-pr from an empty-base receipt = %#v, want typed refusal naming %q verbatim (1641)", result, want)
		}
	})
}

// TestOrganicReviewPlatformFailSafes proves Group F: the secure-open lock
// walk anchors at the repository's Git common directory instead of the
// filesystem root and a pre-lock failure reports not-started (1781), and
// immutable publication falls back to exclusive-create-then-copy when the
// platform rename syscall reports it is unsupported (1804). Neither real
// platform shape reproduces on Linux CI: 1781 needs a managed macOS
// configuration profile that restricts filesystem-root traversal, and 1804
// needs the Git common directory hosted on an ExFAT volume. Both subtests
// are skipped here with a named, explicit reason.
//
// The two fallbacks differ in how much proof runs on Linux, and the
// difference matters when reading this file:
//   - 1781's lock-anchor logic is proven here, because
//     TestSecureOpenLockParentAnchor and
//     TestNegotiatedStoreLockPreAcquisitionFailureIsNotStarted are ordinary
//     Linux tests.
//   - 1804's ENOTSUP fallback has NO proof that executes on Linux.
//     TestPublishNoReplaceDarwinENOTSUPFallsBackToCopy and its siblings live
//     behind a `//go:build darwin` constraint, so Linux CI compiles neither
//     the fallback nor its tests. Its only executed proof is a darwin run.
//
// The community must verify, because CI cannot:
//   - issue-1781: `review start`/`finalize` under a macOS managed
//     configuration profile that restricts `/` traversal still succeeds,
//     since the lock walk now anchors at the Git common directory instead
//     of the filesystem root.
//   - issue-1804: the same flows succeed with the Git common directory
//     hosted on an ExFAT volume, where the exclusive-rename syscall and
//     hard links are both unavailable, exercising the real copy fallback.
func TestOrganicReviewPlatformFailSafes(t *testing.T) {
	t.Run("issue-1781", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("issue-1781 needs a managed macOS configuration profile restricting filesystem-root traversal; unverifiable on Linux CI. See TestSecureOpenLockParentAnchor for the mocked-root unit proof that runs here.")
		}
		harness := newOrganicHarness(t)
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("platform fail-safe candidate", 4)})
		started, _ := harness.startReview("organic-platform-lock-anchor-1781")
		harness.approveReview("organic-platform-lock-anchor-1781", started)
		harness.assertNoSDDArtifacts()
	})

	t.Run("issue-1804", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("issue-1804 needs the Git common directory hosted on an ExFAT volume, where the exclusive-rename syscall and hard links are both unavailable; unverifiable on Linux CI. Its mocked-syscall unit proof, TestPublishNoReplaceDarwinENOTSUPFallsBackToCopy, is darwin-only and does NOT run here: `GOOS=darwin go build` typechecks it, but only a darwin runner executes it.")
		}
		harness := newOrganicHarness(t)
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("platform fail-safe candidate", 4)})
		started, _ := harness.startReview("organic-platform-publish-fallback-1804")
		finalized := harness.approveReview("organic-platform-publish-fallback-1804", started)
		if finalized.State != organicStateApproved {
			t.Fatalf("platform fail-safe candidate did not reach approved: %#v", finalized)
		}
		harness.assertNoSDDArtifacts()
	})
}

// TestOrganicReviewRecoveryGraph proves Group C: a recovery successor whose
// predecessor kind is compatible reaches semantic admission instead of being
// rejected by the CLI's flag-shape pre-gate alone (1744), release-scope
// recovery accepts an ordinary one-parent delivery commit as HEAD (1816),
// and pre-PR chain selection does not report a convergence when a historical
// approved receipt merely ends at the current publication base (1782).
//
// Maintainer-verified correction (see internal/reviewtransaction spec delta
// "Compact recovery edge admission"): re-targeting across target kinds during
// recovery is a deliberate degree of freedom, not a gap to close with a kind
// constraint. The real protections — identity-bound maintainer authorization,
// substantive scope change, and the base-diff predecessor's frozen base-tree
// pin — are proven never to have weakened by issue-1744's own subtest.
func TestOrganicReviewRecoveryGraph(t *testing.T) {
	t.Run("issue-1744", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-recovery-kind-1744"
		baseRef := strings.TrimSpace(harness.git("rev-parse", "HEAD"))
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("escalated candidate line", 5)})
		started, _ := harness.startReview(lineage)
		if started.TargetIdentity == "" || len(started.SelectedLenses) == 0 {
			t.Fatalf("expected a current-changes candidate with at least one selected lens: %#v", started)
		}
		lens := started.SelectedLenses[0]
		harness.captureReviewerResultOrFail(lineage, started, 0, organicReviewerResult{
			Lens: lens,
			Findings: []organicFinding{{
				Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate regression",
				ProofRefs: []string{"differential test fails only on candidate"}, EvidenceClass: "deterministic", CausalDisposition: "introduced",
			}},
			Evidence: []string{"focused differential test failed"},
		})
		finalized := harness.finalize(lineage, "--captured-results=true", "--correction-lines", "1000")
		if finalized.State != "escalated" {
			t.Fatalf("over-budget forecast did not escalate the current-changes predecessor: %#v", finalized)
		}

		status := harnessStatus(t, harness, lineage, "--base-ref", baseRef)
		if status.TargetIdentity == "" {
			t.Fatalf("could not compute the requested base-diff successor identity: %#v", status)
		}
		authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + lineage +
			"\npredecessor_revision=" + finalized.StoreRevision + "\ntarget_identity=" + status.TargetIdentity +
			"\nactor=maintainer\nreason=recover escalated candidate into base-diff scope"

		recovered := harness.gentle(
			"review", "recover", "--cwd", harness.repo.worktree,
			"--predecessor-lineage", lineage, "--expected-predecessor-revision", finalized.StoreRevision,
			"--successor-lineage", lineage+"-recovered", "--disposition", "escalated",
			"--reason", "recover escalated candidate into base-diff scope", "--actor", "maintainer",
			"--base-ref", baseRef, "--committed-only", "--maintainer-authorization", authorization,
		)
		var result struct {
			LineageID string `json:"lineage_id"`
		}
		if err := json.Unmarshal(recovered, &result); err != nil {
			t.Fatalf("decode review recover: %v\n%s", err, recovered)
		}
		if result.LineageID != lineage+"-recovered" {
			t.Fatalf("recovered successor lineage = %q, want %q", result.LineageID, lineage+"-recovered")
		}

		// Regression, same journey: an incompatible flag shape (mismatched
		// --base-ref/--committed-only) is still rejected by the surviving
		// argv-coherence clause.
		_, stderr, err := harness.gentleAllowFailure(
			"review", "recover", "--cwd", harness.repo.worktree,
			"--predecessor-lineage", lineage, "--expected-predecessor-revision", finalized.StoreRevision,
			"--successor-lineage", lineage+"-argv-mismatch", "--disposition", "escalated",
			"--reason", "argv mismatch", "--actor", "maintainer", "--base-ref", baseRef,
		)
		if err == nil || !strings.Contains(stderr, "base-diff recovery requires matching --base-ref and --committed-only") {
			t.Fatalf("mismatched argv shape recovery = err %v, stderr %s, want the surviving argv-coherence rejection", err, stderr)
		}
		harness.assertNoSDDArtifacts()
	})

	t.Run("issue-1816", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-release-scope-one-parent-1816"
		mainBranch := strings.TrimSpace(harness.git("branch", "--show-current"))
		harness.git("checkout", "-qb", "release-candidate")
		harness.writeFiles(map[string]string{"release-notes.txt": "prepared release\n"})
		harness.git("add", "--", "release-notes.txt")
		harness.git("commit", "-qm", "release notes")
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("release-scope candidate line", 4)})

		started, _ := harness.startReview(lineage)
		finalized := harness.approveReview(lineage, started)
		if finalized.State != organicStateApproved {
			t.Fatalf("release-scope predecessor did not reach approved: %#v", finalized)
		}
		harness.git("add", "--", "tracked.txt")
		harness.git("commit", "-qm", "reviewed candidate")

		// Squash-merge the branch onto main: an ordinary one-parent delivery
		// commit (not a two-parent merge commit) whose tree matches the exact
		// reviewed candidate tree, and whose diff against its single parent
		// spans both the reviewed file and the release-notes.txt file that was
		// already on the branch outside the reviewed genesis scope. This is
		// exactly the HEAD shape 1816 requires release-scope recovery to
		// accept.
		harness.git("checkout", "-q", mainBranch)
		harness.git("merge", "-q", "--squash", "release-candidate")
		harness.git("commit", "-qm", "squash-merged delivery")
		parents := strings.Fields(harness.git("rev-list", "--parents", "-n", "1", "HEAD"))
		if len(parents) != 2 {
			t.Fatalf("delivery commit is not an ordinary one-parent commit: %v", parents)
		}

		recovered := harness.gentle(
			"review", "recover", "--cwd", harness.repo.worktree,
			"--predecessor-lineage", lineage, "--expected-predecessor-revision", finalized.StoreRevision,
			"--successor-lineage", lineage+"-release", "--disposition", "scope_changed",
			"--reason", "prepare complete release scope", "--actor", "maintainer", "--release-scope",
		)
		var result struct {
			LineageID string `json:"lineage_id"`
		}
		if err := json.Unmarshal(recovered, &result); err != nil {
			t.Fatalf("decode review recover: %v\n%s", err, recovered)
		}
		if result.LineageID != lineage+"-release" {
			t.Fatalf("release-scope successor lineage = %q, want %q", result.LineageID, lineage+"-release")
		}
		harness.assertNoSDDArtifacts()
	})

	// issue-1782 is DELETED, not superseded (Wave 5 Slice 5, pre-PR chain
	// composition deletion): its regression -- a linear A->B->C delivery
	// chain misreported as a "chain convergence" when the published
	// tracker sat at the chain's own mid-chain root -- was a bug inside
	// EvaluateCompactPrePRChain's own composition graph and convergence
	// detection. That graph no longer exists
	// (TestPrePRComposition_ZeroCallers, internal/cli, proves it by
	// call-absence), so there is no analogous "after" behavior to pin: a
	// three-segment linear chain like this fixture built now denies
	// receipt_ambiguous at pre-PR regardless of where the tracker sits,
	// exactly like TestUnqualifiedPrePRDiscoveryDeniesSequentialCompactReceiptsWithoutComposition
	// (internal/cli) and j61-pre-pr-multi-segment-delivery-denies-without-composition
	// (bench/journeys_wave5.go) already prove for the general multi-segment
	// case -- this fixture's specific tracker-placement detail is no longer
	// a distinguishing variable once composition itself is gone.
}

// TestOrganicReviewStoreRobustness proves Group D (1813): one invalid
// TERMINAL lineage among several healthy ones is quarantined out of
// selector-free store enumeration alone, with a structured diagnostic that
// names it and never flips store-wide completeness, while every other
// healthy lineage stays fully operable and the quarantined lineage itself
// still fails closed when operated on directly.
func TestOrganicReviewStoreRobustness(t *testing.T) {
	t.Run("issue-1813", func(t *testing.T) {
		harness := newOrganicHarness(t)

		healthyA := "organic-store-robust-healthy-a"
		harness.writeFiles(map[string]string{"segment-healthy-a.txt": "healthy candidate a\n"})
		startedA, _ := harness.startReview(healthyA)
		if approved := harness.approveReview(healthyA, startedA); approved.State != organicStateApproved {
			t.Fatalf("healthy lineage a did not approve: %#v", approved)
		}

		quarantined := "organic-store-robust-invalid"
		harness.writeFiles(map[string]string{"segment-invalid.txt": "quarantined candidate\n"})
		startedQ, _ := harness.startReview(quarantined)
		if approved := harness.approveReview(quarantined, startedQ); approved.State != organicStateApproved {
			t.Fatalf("quarantine-target lineage did not approve: %#v", approved)
		}

		healthyB := "organic-store-robust-healthy-b"
		harness.writeFiles(map[string]string{"segment-healthy-b.txt": "healthy candidate b\n"})
		startedB, _ := harness.startReview(healthyB)
		if approved := harness.approveReview(healthyB, startedB); approved.State != organicStateApproved {
			t.Fatalf("healthy lineage b did not approve: %#v", approved)
		}

		// Corrupt the quarantine-target lineage's persisted state semantically,
		// on disk, exactly like a real interrupted/tampered TERMINAL authority:
		// mark it invalidated without the invalidation provenance
		// CompactState.Validate() requires.
		statePath := filepath.Join(harness.commonDir(), "gentle-ai", "review-transactions", "v2", quarantined, "review-state.json")
		payload, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		corrupted := strings.Replace(string(payload), `"state": "approved",`, `"state": "invalidated",`, 1)
		if corrupted == string(payload) {
			t.Fatalf("quarantine fixture did not contain the expected approved state marker")
		}
		if err := os.WriteFile(statePath, []byte(corrupted), 0o644); err != nil {
			t.Fatal(err)
		}

		status := harness.gentle("review", "status", "--cwd", harness.repo.worktree)
		var report struct {
			Complete      bool `json:"complete"`
			Authoritative bool `json:"authoritative"`
			Entries       []struct {
				LineageID string `json:"lineage_id"`
				Status    string `json:"status"`
			} `json:"entries"`
			Diagnostics []struct {
				Path    string `json:"path"`
				Problem string `json:"problem"`
			} `json:"diagnostics"`
		}
		if err := json.Unmarshal(status, &report); err != nil {
			t.Fatalf("decode review status: %v\n%s", err, status)
		}
		if !report.Complete || !report.Authoritative {
			t.Fatalf("quarantined terminal lineage flipped store-wide completeness: %#v", report)
		}
		foundDiagnostic := false
		for _, diagnostic := range report.Diagnostics {
			if strings.Contains(diagnostic.Path, quarantined) && strings.HasPrefix(diagnostic.Problem, "quarantined-terminal-lineage:") {
				foundDiagnostic = true
			}
		}
		if !foundDiagnostic {
			t.Fatalf("no structured diagnostic named the quarantined lineage: %#v", report.Diagnostics)
		}
		for _, entry := range report.Entries {
			if entry.LineageID == quarantined {
				t.Fatalf("quarantined lineage still enumerated as a healthy entry: %#v", entry)
			}
		}
		foundHealthyA, foundHealthyB := false, false
		for _, entry := range report.Entries {
			if entry.LineageID == healthyA && entry.Status == "approved" {
				foundHealthyA = true
			}
			if entry.LineageID == healthyB && entry.Status == "approved" {
				foundHealthyB = true
			}
		}
		if !foundHealthyA || !foundHealthyB {
			t.Fatalf("healthy lineages missing or not approved in inventory: %#v", report.Entries)
		}

		// Every other healthy lineage stays fully operable: a fresh review
		// start on unrelated content still works with the quarantined lineage
		// present in the same store.
		harness.writeFiles(map[string]string{"segment-healthy-c.txt": "healthy candidate c\n"})
		startedC, _ := harness.startReview("organic-store-robust-healthy-c")
		if approved := harness.approveReview("organic-store-robust-healthy-c", startedC); approved.State != organicStateApproved {
			t.Fatalf("a fresh healthy lineage did not remain operable alongside the quarantined one: %#v", approved)
		}

		// The quarantined lineage itself still fails closed when operated on
		// directly by name: an explicit selector never silently reports it
		// healthy, even though selector-free enumeration now quarantines it.
		explicit := harness.gentle(
			"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v1",
			"--lineage", quarantined,
		)
		var explicitResult struct {
			Applicability string `json:"applicability"`
		}
		if err := json.Unmarshal(explicit, &explicitResult); err != nil {
			t.Fatalf("decode explicit-selector status: %v\n%s", err, explicit)
		}
		if explicitResult.Applicability != "corrupted" {
			t.Fatalf("explicit status selector on the quarantined lineage did not fail closed: %#v", explicitResult)
		}
		harness.assertNoSDDArtifacts()
	})
}

// TestOrganicReviewNarrationPairedRecoverableVersusTerminal keeps the paired
// scenario from organic-dx Phase 4 -- one underlying machinery condition (no
// existing review record matches this target,
// reviewtransaction.TargetApplicabilityUnrelated) with two caller-selector
// shapes -- under the negotiated-silence contract: a successful negotiated
// (--contract) STATUS is machine-readable end to end, so BOTH shapes keep
// stdout as the byte-for-byte JSON envelope and write zero bytes to stderr.
// The terminal shape's decision stays structural in
// next_transition.reason_code (staged_workspace_overlay_recovery_unavailable)
// instead of being narrated: gentle-pi's adapter fails closed
// (UNEXPECTED_STDERR) on any stderr a successful native process writes, and
// the registered Tier C statement remains in internal/cli/review_narration.go
// as vocabulary only.
func TestOrganicReviewNarrationPairedRecoverableVersusTerminal(t *testing.T) {
	t.Run("issue-organic-dx-phase4-paired-narration", func(t *testing.T) {
		harness := newOrganicHarness(t)
		harness.writeFiles(map[string]string{"tracked.txt": "narration paired scenario\n"})
		harness.git("add", "-A")
		harness.git("commit", "-qm", "narration paired scenario base")
		harness.writeFiles(map[string]string{"tracked.txt": "narration paired scenario, uncommitted\n"})

		t.Run("recoverable", func(t *testing.T) {
			stdout, stderr, err := harness.gentleAllowFailure(
				"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v1", "--next-transition",
			)
			if err != nil {
				t.Fatalf("ordinary next-transition on a fresh target failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			var status organicStatusResult
			if err := json.Unmarshal([]byte(stdout), &status); err != nil {
				t.Fatalf("decode status: %v\n%s", err, stdout)
			}
			if status.NextTransition == nil || status.NextTransition.Kind != "execute" || status.NextTransition.ReasonCode != "fresh_target_ready" {
				t.Fatalf("ordinary next-transition on a fresh target = %#v, want an executable fresh_target_ready transition", status.NextTransition)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("recoverable shape leaked output on the human surface, want zero Tier-B/Tier-C output: stderr=%q", stderr)
			}
		})

		t.Run("terminal", func(t *testing.T) {
			stdout, stderr, err := harness.gentleAllowFailure(
				"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v1", "--next-transition",
				"--workspace-overlay", "--projection", "staged", "--base-ref", "HEAD",
			)
			if err != nil {
				t.Fatalf("terminal next-transition on the same fresh target failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			var status organicStatusResult
			if err := json.Unmarshal([]byte(stdout), &status); err != nil {
				t.Fatalf("decode status: %v\n%s", err, stdout)
			}
			if status.NextTransition == nil || status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "staged_workspace_overlay_recovery_unavailable" {
				t.Fatalf("terminal next-transition on the same fresh target = %#v, want the staged_workspace_overlay_recovery_unavailable stop", status.NextTransition)
			}
			if stderr != "" {
				t.Fatalf("stop-shaped negotiated STATUS wrote stderr, want zero bytes: %q", stderr)
			}
		})
	})
}

// reviewDefectReportDirEntries lists the files under this repository's
// <GitCommonDir>/gentle-ai/defect-reports/, or nil if the directory does not
// exist yet -- exactly the storage location organic-dx tasks.md Phase 5
// documents (never inside the working tree, never committed).
func reviewDefectReportDirEntries(t *testing.T, harness *organicHarness) []string {
	t.Helper()
	dir := filepath.Join(harness.commonDir(), "gentle-ai", "defect-reports")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// TestOrganicReviewDefectReportToolFaultVersusUserDecision proves organic-dx
// Phase 5: a genuine tool-fault terminal (the confirmed captured-final-
// EVIDENCE byte conflict, tasks.md 3b.9) generates a template-shaped,
// privacy-clean defect report named by its Tier C statement, while a
// legitimate occupied reviewer-result slot generates none and directs the
// caller to the authoritative STATUS continuation.
func TestOrganicReviewDefectReportToolFaultVersusUserDecision(t *testing.T) {
	t.Run("issue-organic-dx-phase5-tool-fault-report", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-defect-report-tool-fault"
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("defect report tool-fault candidate", 6)})
		started, _ := harness.startReview(lineage)
		if len(started.SelectedLenses) == 0 {
			t.Fatal("expected at least one selected lens")
		}
		harnessCaptureCleanResults(t, harness, lineage, started.TargetIdentity, started.SelectedLenses)
		finalized := harness.finalize(lineage, "--captured-results")
		if finalized.State != organicStateValidating {
			t.Fatalf("fixture did not reach validating: %#v", finalized)
		}
		status := harnessStatus(t, harness, lineage)

		if len(reviewDefectReportDirEntries(t, harness)) != 0 {
			t.Fatal("defect-reports directory is not empty before the fault is triggered")
		}

		firstEvidence := filepath.Join(t.TempDir(), "evidence-one.txt")
		if err := os.WriteFile(firstEvidence, []byte("first captured evidence\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		harness.gentle(
			"review", "capture-evidence", "--cwd", harness.repo.worktree, "--lineage", lineage,
			"--target", status.TargetIdentity, "--expected-revision", status.Authority.Revision, "--outcome", "passed", "--input", firstEvidence,
		)

		secondEvidence := filepath.Join(t.TempDir(), "evidence-two.txt")
		if err := os.WriteFile(secondEvidence, []byte("second captured evidence, different bytes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, err := harness.gentleAllowFailure(
			"review", "capture-evidence", "--cwd", harness.repo.worktree, "--lineage", lineage,
			"--target", status.TargetIdentity, "--expected-revision", status.Authority.Revision, "--outcome", "passed", "--input", secondEvidence,
		)
		if err == nil {
			t.Fatal("conflicting captured final evidence unexpectedly succeeded")
		}

		entries := reviewDefectReportDirEntries(t, harness)
		if len(entries) != 1 {
			t.Fatalf("defect-reports directory = %v, want exactly one report", entries)
		}
		reportPath := filepath.Join(harness.commonDir(), "gentle-ai", "defect-reports", entries[0])
		if !strings.Contains(stderr, reportPath) {
			t.Fatalf("stderr did not name the report path %q: %q", reportPath, stderr)
		}
		if !strings.Contains(stderr, "https://github.com/Gentleman-Programming/gentle-ai/issues/new/choose") {
			t.Fatalf("stderr did not name the issues URL: %q", stderr)
		}

		reportBytes, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		report := string(reportBytes)
		for _, header := range []string{
			"# Bug Description", "## Steps to Reproduce", "## Expected Behavior", "## Actual Behavior",
			"## Gentle AI Version", "## Operating System", "## AI Agent / Client", "## Affected Area", "## Logs / Error Output",
		} {
			if !strings.Contains(report, header) {
				t.Fatalf("generated report missing template-shaped header %q:\n%s", header, report)
			}
		}
		// Privacy: the report must never carry the local absolute path
		// (under the isolated HOME this harness sets), the worktree path, or
		// any raw evidence file content.
		for _, poison := range []string{harness.home, harness.repo.worktree, "first captured evidence", "second captured evidence, different bytes"} {
			if strings.Contains(report, poison) {
				t.Fatalf("generated report leaked poisoned content %q:\n%s", poison, report)
			}
		}
	})

	t.Run("issue-organic-dx-phase5-user-decision-no-report", func(t *testing.T) {
		harness := newOrganicHarness(t)
		lineage := "organic-defect-report-user-decision"
		harness.writeFiles(map[string]string{"tracked.txt": organicLines("defect report user-decision candidate", 6)})
		started, _ := harness.startReview(lineage)
		if len(started.SelectedLenses) == 0 {
			t.Fatal("expected at least one selected lens")
		}
		lens := started.SelectedLenses[0]
		subjectHash := harnessCaptureResultSubjectHash(t, harness, lineage, started.TargetIdentity, lens, 0)

		reviewerResult := struct {
			SubjectHash string                   `json:"subject_hash"`
			Inspection  organicCaptureInspection `json:"inspection"`
			Findings    []organicFinding         `json:"findings"`
			Evidence    []string                 `json:"evidence"`
		}{
			SubjectHash: subjectHash,
			Inspection:  organicCaptureInspection{Status: "completed", Paths: []string{"tracked.txt"}},
			Findings:    []organicFinding{},
		}
		reviewerResult.Evidence = []string{"first pass"}
		firstResult := harness.writeJSON("first-result.json", reviewerResult)
		harness.gentle(
			"review", "capture-result", "--cwd", harness.repo.worktree, "--lineage", lineage,
			"--target", started.TargetIdentity, "--lens", lens, "--order", "0", "--input", firstResult,
		)

		reviewerResult.Evidence = []string{"second pass, different content"}
		secondResult := harness.writeJSON("second-result.json", reviewerResult)
		_, stderr, err := harness.gentleAllowFailure(
			"review", "capture-result", "--cwd", harness.repo.worktree, "--lineage", lineage,
			"--target", started.TargetIdentity, "--lens", lens, "--order", "0", "--input", secondResult,
		)
		if err == nil {
			t.Fatal("conflicting reviewer result capture unexpectedly succeeded")
		}
		for _, want := range []string{"reviewer_result_slot_occupied", "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition", "authoritative continuation"} {
			if !strings.Contains(stderr, want) {
				t.Fatalf("occupied-slot terminal did not name %q: %q", want, stderr)
			}
		}
		for _, forbidden := range []string{"review dispose-result", "review preserve-result", "retry capture-result"} {
			if strings.Contains(stderr, forbidden) {
				t.Fatalf("occupied-slot terminal advertised %q: %q", forbidden, stderr)
			}
		}
		if entries := reviewDefectReportDirEntries(t, harness); len(entries) != 0 {
			t.Fatalf("user-decision terminal wrote a defect report, want none: %v", entries)
		}
	})
}
