package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/advisoryreview"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// advisoryLensFindingPrefix mirrors the native per-lens finding ID prefix
// (reviewtransaction's unexported wantPrefix in canonicalReviewerResult) so a
// fixture result can bind to whichever lens native selection actually chose
// for the fixture repository, instead of hardcoding one lens.
var advisoryLensFindingPrefix = map[string]string{
	reviewtransaction.LensRisk: "R1-", reviewtransaction.LensReadability: "R2-",
	reviewtransaction.LensReliability: "R3-", reviewtransaction.LensResilience: "R4-",
}

// advisoryTestSubject independently derives the exact ArtifactSubject the CLI
// itself would derive for this repository, lineage, and lens, so a test can
// build a matching ReviewerResult fixture without going through the advisory
// command first.
func advisoryTestSubject(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string) (reviewtransaction.ArtifactSubject, reviewtransaction.FrozenCandidateContext) {
	t.Helper()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	frozen, err := builder.FrozenCandidateContext(t.Context(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	order := slices.Index(record.State.SelectedLenses, lens)
	if order < 0 {
		t.Fatalf("lens %q is not selected for this fixture review", lens)
	}
	subject, err := reviewtransaction.NewArtifactSubject(record.State, record.Revision, frozen, lens, order, "")
	if err != nil {
		t.Fatal(err)
	}
	return subject, frozen
}

func TestReviewAdvisoryPromptUsesGenericPromptTransportWithoutOpenCodeIsolation(t *testing.T) {
	t.Setenv("OPENCODE_DISABLE_PROJECT_CONFIG", "")
	t.Setenv("OPENCODE_DISABLE_EXTERNAL_SKILLS", "")
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]

	var output bytes.Buffer
	if err := RunReview([]string{"advisory", "prompt", "--repository-context", handle, "--lens", lens, "--runtime", "opencode"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "advisory model output for native Go admission") || strings.Contains(output.String(), "OPENCODE_DISABLE_") {
		t.Fatalf("OpenCode advisory prompt = %q", output.String())
	}
	title, focus, ok := reviewtransaction.LensMandate(lens)
	if !ok {
		t.Fatalf("fixture lens %q has no canonical mandate", lens)
	}
	if !strings.Contains(output.String(), title) || !strings.Contains(output.String(), focus) {
		t.Fatalf("advisory prompt omits the canonical lens mandate %q / %q:\n%s", title, focus, output.String())
	}
}

// TestReviewAdvisoryPromptDerivesRequestFromFrozenAuthority proves the request
// is built server-side from the same two opaque tokens `review lens-context`
// consumes -- never from a caller-authored request file. Two runs against the
// same repository context and lens must derive byte-identical evidence.
func TestReviewAdvisoryPromptDerivesRequestFromFrozenAuthority(t *testing.T) {
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]

	var first, second bytes.Buffer
	if err := RunReview([]string{"advisory", "prompt", "--repository-context", handle, "--lens", lens, "--runtime", "claude-code"}, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"advisory", "prompt", "--repository-context", handle, "--lens", lens, "--runtime", "claude-code"}, &second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatalf("advisory prompt is not deterministic for the same frozen authority:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	if strings.Contains(first.String(), "--request") {
		t.Fatalf("advisory prompt output references a caller-authored request path: %s", first.String())
	}
}

// TestReviewAdvisoryPromptSupportsCodexRuntime proves Codex reached the same
// generic prompt/text seam as Claude Code and OpenCode at the CLI surface,
// once its organic runtime proof
// (TestRealCodexReviewerOrdinarySessionAdmitsRawOutput and its fail-closed
// companions, e2e/organicruntime) passed: the exact same closed form that
// used to return "unavailable for runtime" for codex now returns the
// canonical advisory prompt, byte-identical to the one every other supported
// runtime receives.
func TestReviewAdvisoryPromptSupportsCodexRuntime(t *testing.T) {
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]

	var codex, claude bytes.Buffer
	if err := RunReview([]string{"advisory", "prompt", "--repository-context", handle, "--lens", lens, "--runtime", "codex"}, &codex); err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"advisory", "prompt", "--repository-context", handle, "--lens", lens, "--runtime", "claude-code"}, &claude); err != nil {
		t.Fatal(err)
	}
	if codex.String() != claude.String() {
		t.Fatalf("codex received a runtime-specific prompt:\ncodex:\n%s\nclaude-code:\n%s", codex.String(), claude.String())
	}
	if !strings.Contains(codex.String(), "advisory model output for native Go admission") {
		t.Fatalf("codex advisory prompt = %q", codex.String())
	}
}

// TestReviewAdvisoryRejectsCallerAuthoredRequestForm proves the retired
// caller-JSON request path is gone: an unknown --request flag is refused the
// same way any other unrecognized flag is.
func TestReviewAdvisoryRejectsCallerAuthoredRequestForm(t *testing.T) {
	requestPath := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(requestPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := RunReview([]string{"advisory", "prompt", "--request", requestPath, "--runtime", "opencode"}, &output)
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("caller-authored --request error = %v, want an unknown-flag refusal", err)
	}
	// The flag package's own usage text may still land on stdout for an
	// unrecognized flag (the same as any other closed-form CLI refusal in
	// this product); what must never appear is an actual advisory prompt.
	if strings.Contains(output.String(), "advisory model output for native Go admission") {
		t.Fatalf("rejected caller-authored request still produced an advisory prompt: %s", output.String())
	}
}

func TestReviewAdvisoryValidatePreservesBlockingEvidenceForNativeAdmission(t *testing.T) {
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]
	subject, _ := advisoryTestSubject(t, repo, record, lens)

	resultPath := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, resultPath, reviewtransaction.ReviewerResult{
		SubjectHash: subject.SubjectHash, Lens: lens,
		Inspection: reviewtransaction.ArtifactInspection{Status: reviewtransaction.ArtifactInspectionCompleted, Paths: record.State.InitialSnapshot.Paths},
		Findings: []reviewtransaction.Finding{{
			ID: advisoryLensFindingPrefix[lens] + "001", Lens: strings.TrimPrefix(lens, "review-"), Location: record.State.InitialSnapshot.Paths[0] + ":1",
			Severity: "BLOCKER", Claim: "example", EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
			ProofRefs: []string{record.State.InitialSnapshot.Paths[0] + ":1"},
		}},
		Evidence: []string{"inspected " + record.State.InitialSnapshot.Paths[0]},
	})

	var output bytes.Buffer
	if err := RunReview([]string{"advisory", "validate", "--repository-context", handle, "--lens", lens, "--input", resultPath}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(output.String()), "receipt") || strings.Contains(strings.ToLower(output.String()), "delivery") || strings.Contains(strings.ToLower(output.String()), "pass") {
		t.Fatalf("advisory validation claimed authority: %s", output.String())
	}
	var validated advisoryreview.ValidatedResult
	if err := json.Unmarshal(output.Bytes(), &validated); err != nil || !validated.TransportValidated || validated.FindingCount != 1 || validated.Result.Findings[0].Severity != "BLOCKER" {
		t.Fatalf("advisory validation = %#v, %v", validated, err)
	}
}

// TestReviewAdvisoryValidateRebuildsRequestRatherThanTrustingCallerJSON proves
// the validate subcommand binds raw model output to a request it rebuilds
// itself from the repository context and lens, not to anything a caller
// could hand it: a result stamped with a fabricated subject hash is refused
// exactly like any other subject mismatch, with no caller-supplied request to
// blame it on.
func TestReviewAdvisoryValidateRebuildsRequestRatherThanTrustingCallerJSON(t *testing.T) {
	_, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]

	resultPath := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, resultPath, reviewtransaction.ReviewerResult{
		SubjectHash: "sha256:" + strings.Repeat("0", 64), Lens: lens,
		Inspection: reviewtransaction.ArtifactInspection{Status: reviewtransaction.ArtifactInspectionCompleted, Paths: record.State.InitialSnapshot.Paths},
		Findings:   []reviewtransaction.Finding{},
		Evidence:   []string{"reviewed the complete candidate scope"},
	})

	var output bytes.Buffer
	err := RunReview([]string{"advisory", "validate", "--repository-context", handle, "--lens", lens, "--input", resultPath}, &output)
	if err == nil || !strings.Contains(err.Error(), "does not match the requested subject") {
		t.Fatalf("fabricated subject hash error = %v, want a subject mismatch refusal", err)
	}
	if output.Len() != 0 {
		t.Fatalf("refused advisory validation emitted %d bytes", output.Len())
	}
}

// TestReviewAdvisoryPromptRefusesOverBudgetCandidateWithoutTruncating proves
// advisory evidence assembly refuses on the exact same aggregate budget
// `review lens-context` enforces (reviewLensContextByteBudget), rather than
// inventing a second, independently sized cap, and never truncates evidence
// to fit.
func TestReviewAdvisoryPromptRefusesOverBudgetCandidateWithoutTruncating(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := initReviewCLIRepo(t)
	body := strings.Repeat("oversized evidence line\n", 100_000)
	for _, name := range []string{"bulk-one.txt", "bulk-two.txt"} {
		writeReviewStartCandidate(t, repo, name, body, 0o644)
	}
	started := runNegotiatedReviewStart(t, repo, "advisory-budget")
	var output bytes.Buffer
	err := RunReview([]string{
		"advisory", "prompt", "--repository-context", started.RepositoryContext.Handle,
		"--lens", started.SelectedLenses[0], "--runtime", "claude-code",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("over-budget error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("over-budget refusal emitted %d bytes of truncated evidence", output.Len())
	}
}

// TestReviewAdvisoryRefusesUnboundInput mirrors the closed-form refusals
// `review lens-context` already proves: the two required tokens are the only
// accepted input, and no partial output ever reaches stdout on refusal.
func TestReviewAdvisoryRefusesUnboundInput(t *testing.T) {
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "missing lens", argv: []string{"advisory", "prompt", "--repository-context", handle, "--runtime", "opencode"}, want: "requires the exact provider-issued"},
		{name: "missing context", argv: []string{"advisory", "prompt", "--lens", lens, "--runtime", "opencode"}, want: "requires the exact provider-issued"},
		{name: "missing runtime", argv: []string{"advisory", "prompt", "--repository-context", handle, "--lens", lens}, want: "requires the exact provider-issued"},
		{name: "unselected lens", argv: []string{"advisory", "prompt", "--repository-context", handle, "--lens", "review-nonexistent", "--runtime", "opencode"}, want: "lens_context_lens_not_selected"},
		{name: "genuinely unknown runtime", argv: []string{"advisory", "prompt", "--repository-context", handle, "--lens", lens, "--runtime", "unknown-runtime"}, want: "unavailable for runtime"},
		{name: "unknown advisory command", argv: []string{"advisory", "unknown"}, want: "unknown review advisory command"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(test.argv, &output)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if output.Len() != 0 {
				t.Fatalf("refusal emitted %d bytes to stdout", output.Len())
			}
		})
	}
}
