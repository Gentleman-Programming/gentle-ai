package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
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
	if result.Candidate.Consumed {
		t.Fatalf("fresh passive candidate reported consumed: %#v", result.Candidate)
	}
	if result.ReviewDue || result.ReviewDueReason != "passive" {
		t.Fatalf("passive review assess review_due = %v/%q, want false/passive", result.ReviewDue, result.ReviewDueReason)
	}
	if result.NextTransition != nil {
		t.Fatalf("passive review assess published a next_transition it must not offer: %#v", result.NextTransition)
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
	if result.ChangedLines >= reviewtransaction.LargeChangeLines {
		t.Fatalf("executable-change fixture changed_lines = %d, want under the %d-line slice budget", result.ChangedLines, reviewtransaction.LargeChangeLines)
	}
	if result.ReviewDue || result.ReviewDueReason != "under_budget" {
		t.Fatalf("under-budget medium review assess review_due = %v/%q, want false/under_budget", result.ReviewDue, result.ReviewDueReason)
	}
	if result.NextTransition != nil {
		t.Fatalf("under-budget medium review assess published a next_transition it must not offer: %#v", result.NextTransition)
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
	if !result.ReviewDue || result.ReviewDueReason != "high_risk" {
		t.Fatalf("high-risk review assess review_due = %v/%q, want true/high_risk", result.ReviewDue, result.ReviewDueReason)
	}
	assertReviewAssessNextTransition(t, result.NextTransition, repo, "", "")
}

// assertReviewAssessNextTransition asserts the shared next_transition
// invariant every review_due=true envelope must publish: a literal,
// executable `review status` continuation whose tokens are exactly its
// arguments, in the exact order review assess documents (cwd, contract,
// optional agent, next-transition, optional base-ref/committed-only pair).
// agent and baseRef are empty when the caller omitted --agent/--base-ref.
func assertReviewAssessNextTransition(t *testing.T, transition *ReviewAssessmentNextTransition, repo, agent, baseRef string) {
	t.Helper()
	if transition == nil {
		t.Fatal("review_due=true result published no next_transition")
	}
	if transition.Operation != "review.status" {
		t.Fatalf("next_transition operation = %q, want review.status", transition.Operation)
	}
	wantNames := []string{"cwd", "contract"}
	if agent != "" {
		wantNames = append(wantNames, "agent")
	}
	wantNames = append(wantNames, "next-transition")
	if baseRef != "" {
		wantNames = append(wantNames, "base-ref", "committed-only")
	}
	if len(transition.Arguments) != len(wantNames) {
		t.Fatalf("next_transition arguments = %#v, want names %v", transition.Arguments, wantNames)
	}
	tokens := make([]string, 0, len(transition.Arguments)+2)
	tokens = append(tokens, "review", "status")
	for index, argument := range transition.Arguments {
		if argument.Name != wantNames[index] {
			t.Fatalf("next_transition argument[%d] name = %q, want %q (order = %#v)", index, argument.Name, wantNames[index], transition.Arguments)
		}
		if argument.Token != "--"+argument.Name+"="+argument.Value {
			t.Fatalf("next_transition argument %#v is not its literal --name=value token", argument)
		}
		tokens = append(tokens, reviewTransitionShellWord(argument.Token))
	}
	if want := "gentle-ai " + strings.Join(tokens, " "); transition.Command != want {
		t.Fatalf("next_transition command = %q, want %q", transition.Command, want)
	}
	byName := map[string]string{}
	for _, argument := range transition.Arguments {
		byName[argument.Name] = argument.Value
	}
	if byName["cwd"] != repo {
		t.Fatalf("next_transition cwd = %q, want repository root %q", byName["cwd"], repo)
	}
	if byName["contract"] != ReviewIntegrationContractV2 {
		t.Fatalf("next_transition contract = %q, want %q", byName["contract"], ReviewIntegrationContractV2)
	}
	if byName["next-transition"] != "true" {
		t.Fatalf("next_transition next-transition selector = %q, want true", byName["next-transition"])
	}
	if agent != "" && byName["agent"] != agent {
		t.Fatalf("next_transition agent = %q, want %q", byName["agent"], agent)
	}
	if baseRef != "" {
		if byName["base-ref"] != baseRef {
			t.Fatalf("next_transition base-ref = %q, want the caller's --base-ref verbatim %q", byName["base-ref"], baseRef)
		}
		if byName["committed-only"] != "true" {
			t.Fatalf("next_transition committed-only = %q, want true", byName["committed-only"])
		}
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
	if !strings.Contains(rendered, "review due: no (passive)") {
		t.Fatalf("human-readable review assess output missing the review-due line: %q", rendered)
	}
}

// TestReviewAssessHumanReadableOutputNamesDueTransition proves the
// human-readable line for a review_due=true candidate also prints the exact
// runnable `gentle-ai review status ...` continuation, so an orchestrator
// reading plain text (not --json) still gets the literal command rather than
// having to re-derive it from the ODD prose rule.
func TestReviewAssessHumanReadableOutputNamesDueTransition(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "internal/auth/login.go", "package auth\n", 0o644)

	var output bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo}, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	rendered := output.String()
	wantLine := fmt.Sprintf("review due: yes (high_risk) -> gentle-ai review status %s --contract=%s --next-transition=true", reviewTransitionShellWord("--cwd="+repo), ReviewIntegrationContractV2)
	if !strings.Contains(rendered, wantLine) {
		t.Fatalf("human-readable review assess output = %q, want it to contain %q", rendered, wantLine)
	}
}

// TestReviewAssessMediumAtBudgetReviewDueSliceBudgetReached proves the ODD
// slice-budget rule directly: a medium-risk candidate whose changed_lines
// reaches reviewtransaction.LargeChangeLines is review_due=true with reason
// slice_budget_reached, and its next_transition carries the exact tokens
// review status --next-transition needs, with --base-ref echoed verbatim and
// --agent present only when the caller supplied one.
func TestReviewAssessMediumAtBudgetReviewDueSliceBudgetReached(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	baseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	lines := make([]string, reviewtransaction.LargeChangeLines)
	for index := range lines {
		lines[index] = fmt.Sprintf("scratch change line %03d", index+1)
	}
	writeReviewStartCandidate(t, repo, "notes/scratch-large.txt", strings.Join(lines, "\n")+"\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "add large scratch file")

	for _, test := range []struct {
		name  string
		agent string
	}{
		{name: "without agent"},
		{name: "with agent", agent: string(model.AgentClaudeCode)},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"assess", "--cwd", repo, "--base-ref", baseRef, "--committed-only", "--json"}
			if test.agent != "" {
				args = append(args, "--agent", test.agent)
			}
			var output bytes.Buffer
			if err := RunReview(args, &output); err != nil {
				t.Fatalf("review assess: %v\n%s", err, output.String())
			}
			var result ReviewAssessmentResult
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode review assess envelope: %v\n%s", err, output.String())
			}
			if result.Risk != "medium" {
				t.Fatalf("medium-at-budget review assess risk = %q, want medium: %#v", result.Risk, result)
			}
			if result.ChangedLines < reviewtransaction.LargeChangeLines {
				t.Fatalf("medium-at-budget review assess changed_lines = %d, want >= %d", result.ChangedLines, reviewtransaction.LargeChangeLines)
			}
			if !result.ReviewDue || result.ReviewDueReason != "slice_budget_reached" {
				t.Fatalf("medium-at-budget review_due = %v/%q, want true/slice_budget_reached", result.ReviewDue, result.ReviewDueReason)
			}
			if result.Candidate.Consumed {
				t.Fatalf("fresh medium-at-budget candidate reported consumed: %#v", result.Candidate)
			}
			assertReviewAssessNextTransition(t, result.NextTransition, repo, test.agent, baseRef)
		})
	}
}

// TestReviewAssessConsumedCandidateReportsAlreadyReviewed proves consumed
// takes precedence over every risk tier: once a candidate's exact terminal
// review authority is acknowledged, re-assessing the identical scope reports
// review_due=false/already_reviewed and candidate.consumed=true, never the
// tier-derived reason the same content would otherwise carry.
func TestReviewAssessConsumedCandidateReportsAlreadyReviewed(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineage := startLowRiskFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)

	var output bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode review assess envelope: %v\n%s", err, output.String())
	}
	if !result.Candidate.Consumed {
		t.Fatalf("review assess over an acknowledged candidate reported consumed=false: %#v", result.Candidate)
	}
	if result.ReviewDue || result.ReviewDueReason != "already_reviewed" {
		t.Fatalf("consumed candidate review_due = %v/%q, want false/already_reviewed", result.ReviewDue, result.ReviewDueReason)
	}
	if result.NextTransition != nil {
		t.Fatalf("consumed candidate published a next_transition it must not offer: %#v", result.NextTransition)
	}
}

// TestReviewAssessInvalidAgentRefusesTyped proves an unsupported --agent
// value fails the same typed classification review status already uses
// (reviewImmutableTransportUnsupportedReason), rather than a bare generic
// error, so a caller can branch on it exactly as it would branch on the
// equivalent review status refusal.
func TestReviewAssessInvalidAgentRefusesTyped(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.md", "ordinary documentation prose.\n", 0o644)

	var output bytes.Buffer
	err := RunReview([]string{"assess", "--cwd", repo, "--agent", "unknown-runtime"}, &output)
	if err == nil {
		t.Fatalf("review assess with an unsupported --agent unexpectedly succeeded: %s", output.String())
	}
	var typed *reviewIntegrationPreflightError
	if !errors.As(err, &typed) {
		t.Fatalf("review assess invalid --agent error = %#v (%T), want a *reviewIntegrationPreflightError", err, err)
	}
	if classification := typed.classification(); classification.Code != reviewImmutableTransportUnsupportedCode {
		t.Fatalf("review assess invalid --agent classification = %#v, want code %q", classification, reviewImmutableTransportUnsupportedCode)
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

// TestReviewAssessJSONFailsClosedOnCandidateErrors proves that when --json is
// requested and an unbuildable or empty candidate causes an error, review assess
// emits a schema-compliant fail-closed envelope with risk "high" instead of
// returning empty output (#4332).
func TestReviewAssessJSONFailsClosedOnCandidateErrors(t *testing.T) {
	schema := compileWholePublishedReviewSchema(t, "v2", "assess.schema.json")
	repo := initReviewCLIRepo(t)

	// Test 1: Empty candidate with --json emits fail-closed JSON and returns error
	var output bytes.Buffer
	err := RunReview([]string{"assess", "--cwd", repo, "--json"}, &output)
	if err == nil {
		t.Fatal("review assess on empty candidate unexpectedly succeeded")
	}
	if output.Len() == 0 {
		t.Fatal("review assess --json returned empty output on error")
	}
	validatePublishedReviewSchema(t, schema, output.Bytes())
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode fail-closed JSON: %v", err)
	}
	if result.Risk != "high" || result.Candidate.Kind != string(reviewtransaction.TargetCurrentChanges) {
		t.Fatalf("unexpected fail-closed assessment result: %#v", result)
	}
	if len(result.Reasons) == 0 || result.Reasons[0].Code != "unassessable" {
		t.Fatalf("expected unassessable reason, got: %#v", result.Reasons)
	}

	// Test 2: Unbuildable base-ref with --json emits fail-closed JSON with base-diff candidate
	output.Reset()
	err = RunReview([]string{"assess", "--cwd", repo, "--base-ref", "nonexistent-ref", "--json"}, &output)
	if err == nil {
		t.Fatal("review assess on unbuildable ref unexpectedly succeeded")
	}
	if output.Len() == 0 {
		t.Fatal("review assess --base-ref ... --json returned empty output on error")
	}
	validatePublishedReviewSchema(t, schema, output.Bytes())
	result = ReviewAssessmentResult{}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode fail-closed JSON: %v", err)
	}
	if result.Risk != "high" || result.Candidate.Kind != string(reviewtransaction.TargetBaseDiff) || result.Candidate.BaseRef != "nonexistent-ref" {
		t.Fatalf("unexpected fail-closed assessment result: %#v", result)
	}

	// Test 3: --json=false does not emit JSON on error
	output.Reset()
	err = RunReview([]string{"assess", "--cwd", repo, "--json=false", "unexpected"}, &output)
	if err == nil {
		t.Fatal("review assess on unexpected argument unexpectedly succeeded")
	}
	if output.Len() != 0 {
		t.Fatalf("review assess with --json=false emitted output: %q", output.String())
	}
}
