package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewAssessPassiveDocumentationCandidate proves review assess reports
// the same zero-lens tier START would select for a candidate whose only
// authored bytes are ordinary prose, without creating any review authority.
// It deliberately does not assert an empty reasons array: the shared
// assessor's own fallback reason (non_executable_only) is exactly what
// explains the "passive" tier, and review assess reuses that assessor rather
// than forking it, so this envelope carries the same evidence START does.
func TestReviewAssessPassiveDocumentationCandidate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.md", "ordinary documentation prose.\n", 0o644)

	var output bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode review assess envelope: %v\n%s", err, output.String())
	}
	if result.Schema != ReviewAssessmentSchema || result.Risk != "passive" || result.ChangedPaths != 1 ||
		result.Candidate.Kind != string(reviewtransaction.TargetCurrentChanges) || result.Candidate.BaseRef != "" {
		t.Fatalf("passive review assess = %#v\n%s", result, output.String())
	}
	if len(result.Reasons) != 1 || result.Reasons[0].Code != string(reviewtransaction.RiskReasonNonExecutableOnly) {
		t.Fatalf("passive review assess reasons = %#v", result.Reasons)
	}
}

// TestReviewAssessExecutableChangeCandidate proves an ordinary but
// non-passive change (a plain-text file, which no passive-document extension
// admits) is reported as medium with the executable-change reason, exactly
// as the shared assessor's fallback classifies it for START.
func TestReviewAssessExecutableChangeCandidate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "notes/scratch.txt", "some plain scratch content\n", 0o644)

	var output bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode review assess envelope: %v\n%s", err, output.String())
	}
	if result.Risk != "medium" {
		t.Fatalf("executable-change review assess risk = %q, want medium: %#v", result.Risk, result)
	}
	if len(result.Reasons) != 1 || result.Reasons[0].Code != string(reviewtransaction.RiskReasonExecutableChange) ||
		result.Reasons[0].Path != "notes/scratch.txt" {
		t.Fatalf("executable-change review assess reasons = %#v", result.Reasons)
	}
}

// TestReviewAssessProcessBoundaryCandidate proves a change touching an
// authentication-pattern path is reported as high with a matching reason
// code, exactly as START's own hot-path classification would.
func TestReviewAssessProcessBoundaryCandidate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "internal/auth/login.go", "package auth\n", 0o644)

	var output bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode review assess envelope: %v\n%s", err, output.String())
	}
	if result.Risk != "high" {
		t.Fatalf("auth-path review assess risk = %q, want high: %#v", result.Risk, result)
	}
	found := false
	for _, reason := range result.Reasons {
		if reason.Code == string(reviewtransaction.RiskReasonHotPath) && strings.Contains(reason.Detail, "signal: auth") {
			found = true
		}
	}
	if !found {
		t.Fatalf("auth-path review assess reasons missing hot_path/auth evidence: %#v", result.Reasons)
	}
}

// TestReviewAssessCommittedOnlyBaseDiffMatchesStart proves a --base-ref
// candidate is classified by review assess exactly the way a negotiated
// review start records it in its own authority state, for the identical
// fixture. review assess must never disagree with the classifier START uses
// to select lenses.
func TestReviewAssessCommittedOnlyBaseDiffMatchesStart(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	baseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "add deploy script")

	var assessOutput bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo, "--base-ref", baseRef, "--committed-only", "--json"}, &assessOutput); err != nil {
		t.Fatalf("review assess --base-ref: %v\n%s", err, assessOutput.String())
	}
	var assessed ReviewAssessmentResult
	if err := json.Unmarshal(assessOutput.Bytes(), &assessed); err != nil {
		t.Fatalf("decode review assess envelope: %v\n%s", err, assessOutput.String())
	}
	if assessed.Candidate.Kind != string(reviewtransaction.TargetBaseDiff) || assessed.Candidate.BaseRef != baseRef {
		t.Fatalf("base-diff review assess candidate = %#v", assessed.Candidate)
	}

	started := runNegotiatedReviewStartWith(t, repo, "review-assess-equivalence", "--base-ref", baseRef)
	wantRisk, err := reviewAssessPublicRisk(started.RiskLevel)
	if err != nil {
		t.Fatal(err)
	}
	if assessed.Risk != wantRisk || assessed.ChangedLines != started.ChangedLines {
		t.Fatalf("review assess (%q, %d changed lines) does not match review start's own classification (%q, %d changed lines)",
			assessed.Risk, assessed.ChangedLines, wantRisk, started.ChangedLines)
	}
}

// TestReviewAssessUnbuildableCandidateNamesResolution proves an unbuildable
// candidate (an unresolvable --base-ref) fails closed with a message naming a
// literal `gentle-ai review assess ...` resolution, per the refusal ratchet
// and issue #4295's fail-closed requirement. Hosts that cannot resolve the
// named continuation are documented to treat this exactly like a "high"
// result.
func TestReviewAssessUnbuildableCandidateNamesResolution(t *testing.T) {
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	err := RunReview([]string{"assess", "--cwd", repo, "--base-ref", "not-a-real-ref-4295"}, &output)
	if err == nil {
		t.Fatalf("review assess with an unresolvable --base-ref unexpectedly succeeded: %s", output.String())
	}
	if !strings.Contains(err.Error(), "gentle-ai review assess") {
		t.Fatalf("unbuildable review assess error does not name a gentle-ai review assess resolution: %v", err)
	}
}

// TestReviewAssessJSONEnvelopeValidatesAgainstPublishedSchema proves the
// --json envelope validates against the published
// gentle-ai.review-assessment/v1 schema for a representative candidate of
// each public risk word.
func TestReviewAssessJSONEnvelopeValidatesAgainstPublishedSchema(t *testing.T) {
	schema := compileWholePublishedReviewSchema(t, "v2", "assess.schema.json")

	for _, fixture := range []struct {
		name string
		path string
		body string
	}{
		{name: "passive", path: "docs/guide.md", body: "ordinary documentation prose.\n"},
		{name: "medium", path: "notes/scratch.txt", body: "some plain scratch content\n"},
		{name: "high", path: "internal/auth/login.go", body: "package auth\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, fixture.path, fixture.body, 0o644)

			var output bytes.Buffer
			if err := RunReview([]string{"assess", "--cwd", repo, "--json"}, &output); err != nil {
				t.Fatalf("review assess: %v\n%s", err, output.String())
			}
			validatePublishedReviewSchema(t, schema, output.Bytes())
		})
	}
}

// TestReviewAssessHumanReadableOutputOmitsJSON proves the default (no --json)
// output is human-readable text, not the machine envelope, and still reports
// the risk and candidate shape.
func TestReviewAssessHumanReadableOutputOmitsJSON(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.md", "ordinary documentation prose.\n", 0o644)

	var output bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo}, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	rendered := output.String()
	if strings.Contains(rendered, "{") || !strings.Contains(rendered, "Risk: passive") || !strings.Contains(rendered, "Candidate: current-changes") {
		t.Fatalf("human-readable review assess output = %q", rendered)
	}
}

// TestReviewAssessDispatchedFromReviewCommand proves the verb is reachable
// through the plain review facade dispatch, the same route
// docs/review-integration.md's guard requires.
func TestReviewAssessDispatchedFromReviewCommand(t *testing.T) {
	source, err := os.ReadFile("review_facade.go")
	if err != nil {
		t.Fatal(err)
	}
	if !reviewCommandDispatchVerbsFromSource(t, source)["assess"] {
		t.Fatal("review assess is not dispatched by review_facade.go")
	}
}
