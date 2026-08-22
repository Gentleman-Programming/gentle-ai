package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The targeted validator is the one review role that is expected to inspect an
// immutable candidate itself instead of being handed prompt-carried evidence
// alone. Issue #3380: it was granted the capability and withheld the recipe.
// Its provider prompt named no inspection command, and the one native command
// that can read the frozen corrected tree -- `gentle-ai review
// inspect-candidate --purpose targeted-validation` -- requires an opaque
// `--repository-context` handle that the prompt never carried, so the recipe
// was not merely unnamed but underivable. Two runtimes hit the same wall: one
// submitted `passed: false` on "could not be inspected", consuming the
// lineage's single correction attempt on a non-observation; the other refused
// to submit and stranded its lineage at targeted_validation_required.
//
// These tests pin both halves against the exact bytes every runtime's
// validator receives: the materialized provider invocation. OpenCode delivers
// those bytes behind the materialization header, and the Claude, Codex, and Pi
// adapters deliver them verbatim on stdin.

// reviewValidatorRecipeFlags is the complete closed argument set a targeted
// validation inspection needs. Naming them individually keeps a partial recipe
// (the failure shape that produced the field reports) from passing.
var reviewValidatorRecipeFlags = []string{
	"--purpose", "--lineage", "--expected-revision", "--target",
	"--request-hash", "--repository-context", "--operation", "--path-index", "--side",
}

func TestTargetedValidatorPromptCarriesTheInspectionRecipe(t *testing.T) {
	reviewEnabledHome(t)
	repo, state, revision := driveReviewToOpenTargetedValidation(t)
	prompt := string(targetedValidatorProviderPrompt(t, repo, state, revision))

	for _, want := range []string{
		"gentle-ai review inspect-candidate",
		reviewTargetedValidationPurpose,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("targeted validator prompt does not name %q; the validator cannot inspect what it is never told to run\nprompt:\n%s", want, prompt)
		}
	}
	for _, flag := range reviewValidatorRecipeFlags {
		if !strings.Contains(prompt, flag) {
			t.Errorf("targeted validator prompt does not name inspection flag %q", flag)
		}
	}
}

func TestTargetedValidatorPromptCarriesEveryInspectionArgument(t *testing.T) {
	reviewEnabledHome(t)
	repo, state, revision := driveReviewToOpenTargetedValidation(t)
	request := decodeTargetedValidatorPromptRequest(t, targetedValidatorProviderPrompt(t, repo, state, revision))

	if err := reviewtransaction.ValidateReviewRepositoryContextHandle(request.RepositoryContext); err != nil {
		t.Fatalf("targeted validator request carries no usable repository context (%q): %v; every other inspection argument is derivable, so this one omission makes the whole recipe unrunnable", request.RepositoryContext, err)
	}
	for name, value := range map[string]string{
		"lineage_id":                 request.ValidationRequest.LineageID,
		"expected_revision":          request.ValidationRequest.ExpectedRevision,
		"correction_target_identity": request.ValidationRequest.CorrectionTargetIdentity,
		"request_hash":               request.ValidationRequest.RequestHash,
	} {
		if strings.TrimSpace(value) == "" {
			t.Errorf("targeted validator request field %s is empty; --%s has no source", name, name)
		}
	}
	if len(request.ValidationRequest.CorrectionPaths) == 0 {
		t.Error("targeted validator request carries no correction paths, so --path-index has no source")
	}
}

func TestTargetedValidatorPromptCarriesFrozenPolicyAndCausalFindingEvidence(t *testing.T) {
	reviewEnabledHome(t)
	repo, state, revision := driveReviewToOpenTargetedValidation(t)
	prompt := string(targetedValidatorProviderPrompt(t, repo, state, revision))

	if !strings.Contains(prompt, facadeReviewPolicy) {
		t.Fatal("targeted validator prompt omits the exact policy bound when the review started")
	}
	for _, findingID := range state.FixFindingIDs {
		var finding reviewtransaction.Finding
		for _, candidate := range state.Findings {
			if candidate.ID == findingID {
				finding = candidate
				break
			}
		}
		classification, found := state.Classifications[findingID]
		if finding.ID == "" || !found {
			t.Fatalf("open correction has no frozen causal finding for %q: %#v", findingID, state)
		}
		for _, value := range append([]string{finding.ID, finding.Location, finding.Claim, classification.Proof}, finding.ProofRefs...) {
			if !strings.Contains(prompt, value) {
				t.Fatalf("targeted validator prompt omits frozen causal finding evidence %q", value)
			}
		}
	}
}

// TestTargetedValidatorInspectionRecipeExecutes is the honest half: it does not
// assert that the prompt mentions a command, it executes the command the
// prompt describes, using only values parsed out of that prompt. A recipe that
// reads well and does not run is the same defect in nicer prose.
func TestTargetedValidatorInspectionRecipeExecutes(t *testing.T) {
	reviewEnabledHome(t)
	repo, state, revision := driveReviewToOpenTargetedValidation(t)
	request := decodeTargetedValidatorPromptRequest(t, targetedValidatorProviderPrompt(t, repo, state, revision))

	binding := []string{
		"--purpose", reviewTargetedValidationPurpose,
		"--lineage", request.ValidationRequest.LineageID,
		"--expected-revision", request.ValidationRequest.ExpectedRevision,
		"--target", request.ValidationRequest.CorrectionTargetIdentity,
		"--request-hash", request.ValidationRequest.RequestHash,
		"--repository-context", request.RepositoryContext,
	}
	for _, operation := range [][]string{
		{"--operation", "name-status"},
		{"--operation", "numstat"},
		{"--operation", "stat", "--path-index", "0"},
		{"--operation", "patch", "--path-index", "0"},
		{"--operation", "object", "--path-index", "0", "--side", "candidate"},
	} {
		var out bytes.Buffer
		if err := RunReviewInspectCandidate(append(append([]string(nil), binding...), operation...), &out); err != nil {
			t.Fatalf("inspect-candidate %v derived from the validator prompt alone failed: %v\n%s", operation, err, out.String())
		}
		if out.Len() == 0 {
			t.Errorf("inspect-candidate %v returned no bytes; the validator cannot reach the frozen corrected tree", operation)
		}
	}

	// The frozen corrected content, not the live worktree: the correction
	// rewrote alpha.go to return 10, and only the immutable tree can prove it.
	var patch bytes.Buffer
	if err := RunReviewInspectCandidate(append(append([]string(nil), binding...), "--operation", "patch", "--path-index", correctionPathIndex(t, request, "alpha.go")), &patch); err != nil {
		t.Fatalf("inspect-candidate patch: %v\n%s", err, patch.String())
	}
	if !strings.Contains(patch.String(), "return 10") {
		t.Errorf("frozen corrected patch does not carry the correction:\n%s", patch.String())
	}
}

func correctionPathIndex(t *testing.T, request targetedValidatorPromptRequest, path string) string {
	t.Helper()
	for index, candidate := range request.ValidationRequest.CorrectionPaths {
		if candidate == path {
			return fmt.Sprint(index)
		}
	}
	t.Fatalf("correction paths %v do not contain %q", request.ValidationRequest.CorrectionPaths, path)
	return ""
}

// targetedValidatorProviderPrompt returns the exact opaque bytes the validator
// receives on every runtime: OpenCode's transport materializes them behind its
// header, and the Claude, Codex, and Pi adapters write them straight to a
// locked-down subprocess's stdin.
func targetedValidatorProviderPrompt(t *testing.T, repo string, state reviewtransaction.CompactState, revision string) []byte {
	t.Helper()
	ctx := context.Background()
	correction, err := reviewProviderTargetedValidatorCorrection(ctx, repo, state)
	if err != nil {
		t.Fatalf("resolve open correction: %v", err)
	}
	request, err := reviewProviderNewTargetedValidatorRequest(ctx, repo, state, revision, correction)
	if err != nil {
		t.Fatalf("build targeted validator request: %v", err)
	}
	return request.Invocation.Prompt()
}

// targetedValidatorPromptRequest is declared here rather than reused from the
// provider package on purpose: it models what a validator can actually see,
// which is JSON text with no Go types behind it.
type targetedValidatorPromptRequest struct {
	RepositoryContext string `json:"repository_context"`
	ValidationRequest struct {
		RequestHash              string   `json:"request_hash"`
		LineageID                string   `json:"lineage_id"`
		ExpectedRevision         string   `json:"expected_revision"`
		CorrectionTargetIdentity string   `json:"correction_target_identity"`
		CorrectionPaths          []string `json:"correction_paths"`
	} `json:"validation_request"`
}

// decodeTargetedValidatorPromptRequest parses the request exactly the way a
// validator must: out of the single `Input:` block of the prompt it was given,
// with no repository access and no other source of truth.
func decodeTargetedValidatorPromptRequest(t *testing.T, prompt []byte) targetedValidatorPromptRequest {
	t.Helper()
	_, rest, found := strings.Cut(string(prompt), "\n\nInput:\n")
	if !found {
		t.Fatalf("validator prompt has no Input block:\n%s", prompt)
	}
	payload, _, found := strings.Cut(rest, "\n\nOutput schema:\n")
	if !found {
		t.Fatalf("validator prompt has no Output schema block:\n%s", prompt)
	}
	var request targetedValidatorPromptRequest
	if err := json.Unmarshal([]byte(payload), &request); err != nil {
		t.Fatalf("decode validator prompt input: %v\n%s", err, payload)
	}
	return request
}

// driveReviewToOpenTargetedValidation runs a faithful staged correction cycle
// to the exact state the two field reports were stuck in: one open, unconsumed
// correction whose corrected candidate has passed repository verification and
// is waiting on a targeted validation verdict.
func driveReviewToOpenTargetedValidation(t *testing.T) (repo string, state reviewtransaction.CompactState, revision string) {
	t.Helper()
	repo = initReviewCLIRepo(t)
	for name, content := range map[string]string{
		"alpha.go": "package candidate\n\nfunc Alpha() int { return 1 }\n",
		"beta.go":  "package candidate\n\nfunc Beta() int { return 2 }\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runReviewCLIGit(t, repo, "add", "alpha.go", "beta.go")

	var startOut bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "validator-recipe-3380", "--projection", "staged",
	}), &startOut); err != nil {
		t.Fatalf("start: %v\n%s", err, startOut.String())
	}
	started := decodeNegotiatedReviewStart(t, startOut.Bytes())

	finalizeArgs := []string{"--cwd", repo, "--lineage", started.LineageID}
	for index, lens := range started.SelectedLenses {
		resultPath := filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", index))
		findings := []facadeFinding{}
		if index == 0 {
			findings = []facadeFinding{{
				Location: "alpha.go:3", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
				ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
				EvidenceClass: "deterministic", CausalDisposition: "introduced",
			}}
		}
		writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
			Lens: lens, Findings: findings, Evidence: []string{"inspected the exact frozen candidate"},
		})
		finalizeArgs = append(finalizeArgs, "--result", resultPath)
	}
	if err := finalizeReviewCLIArgs(t, repo, finalizeArgs, &bytes.Buffer{}); err != nil {
		t.Fatalf("finalize results: %v", err)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "2",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("forecast: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "alpha.go"), []byte("package candidate\n\nfunc Alpha() int { return 10 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "alpha.go")

	for hop := 1; hop <= 8; hop++ {
		var raw bytes.Buffer
		if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
			"--agent", "claude-code", "--next-transition", "--lineage", started.LineageID, "--projection", "staged"}, &raw); err != nil {
			t.Fatalf("status hop %d: %v", hop, err)
		}
		var parsed ReviewTargetStatusResult
		decodeStrictReviewJSON(t, raw.Bytes(), &parsed)
		transition := parsed.NextTransition
		if transition == nil {
			t.Fatalf("hop %d: no transition", hop)
		}
		switch transition.ReasonCode {
		case "correction_repository_verification_required":
			passed := filepath.Join(t.TempDir(), "passed.txt")
			if err := os.WriteFile(passed, []byte("full repository verification passed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
				"--target", transitionArgumentValue(t, transition, "target"),
				"--expected-revision", transitionArgumentValue(t, transition, "expected-revision"),
				"--outcome", "passed", "--input", passed}, &bytes.Buffer{}); err != nil {
				t.Fatalf("capture passed evidence: %v", err)
			}
		case "targeted_validation_required":
			_, record, err := discoverCompactFacadeReview(context.Background(), repo, started.LineageID, false)
			if err != nil {
				t.Fatalf("discover compact authority: %v", err)
			}
			return repo, record.State, record.Revision
		default:
			t.Fatalf("hop %d reached %s/%s instead of targeted_validation_required", hop, transition.Kind, transition.ReasonCode)
		}
	}
	t.Fatal("never reached targeted_validation_required")
	return "", reviewtransaction.CompactState{}, ""
}
