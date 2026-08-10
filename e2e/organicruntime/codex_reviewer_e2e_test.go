package organicruntime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/advisoryreview"
)

// codexPoisonMarker is planted, after the candidate freezes, into the
// reviewed file and a brand-new secret-shaped file. CodexAdapter launches
// codex in an empty scratch directory it creates and deletes itself, so
// neither location can ever reach the reviewer's own prompt, regardless of
// what the live worktree contains by the time codex actually runs.
const codexPoisonMarker = "POISON-MARKER-2624-c0d3x"

func requireCodexOrganicReviewer(t *testing.T) {
	t.Helper()
	if os.Getenv(realAgentE2EEnvironment) != "1" {
		t.Skip("set GENTLE_AI_REAL_AGENT_E2E=1 to run the real Codex reviewer proof")
	}
	// Codex-specific and deliberately not requireOrganicExecutable (shared
	// with npm and, via requireOrganicExecutableVersion, OpenCode): CI's
	// organic-runtime-e2e job sets GENTLE_AI_REAL_AGENT_E2E=1 but does not
	// install a codex binary or an authenticated account (issue #2662), so
	// an absent binary here is an expected, known gap, never a hard failure
	// -- unlike OpenCode and npm, which CI always provisions and for which a
	// missing binary would be a genuine environment defect. Any other
	// LookPath error still fails loudly.
	if _, err := exec.LookPath("codex"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skipf("codex binary not installed; continuous CI execution tracked in #2662: %v", err)
		}
		t.Fatalf("required executable codex: %v", err)
	}
}

// codexNegotiatedStatus is organicNegotiatedStatus bound to the codex runtime
// identity instead of opencode.
func codexNegotiatedStatus(t *testing.T, harness *organicHarness, lineage string) organicNegotiatedStatusResult {
	t.Helper()
	payload := harness.gentle(
		"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v2",
		"--agent", "codex", "--lineage", lineage, "--next-transition",
		"--base-ref", "origin/main", "--projection", "workspace",
	)
	var status organicNegotiatedStatusResult
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("decode negotiated review status: %v\n%s", err, payload)
	}
	return status
}

// codexPoisonedReviewSetup is the shared fixture for the Codex organic
// proofs below: a real negotiated review, walked through to its exactly-one
// reviewer collect input, with the live worktree poisoned strictly after the
// candidate froze.
type codexPoisonedReviewSetup struct {
	harness       *organicHarness
	lineage       string
	candidatePath string
	binding       map[string]string
	manifestPaths []string
}

func setupCodexPoisonedReview(t *testing.T, lineage string) codexPoisonedReviewSetup {
	t.Helper()
	harness := newOrganicHarness(t)
	const candidatePath = "internal/mechanical/candidate.go"
	// Committed deliberately, mirroring setupOpenCodePoisonedReview: a
	// genuinely immutable committed blob is what the poison below can never
	// reach.
	harness.writeFiles(map[string]string{
		candidatePath: "package mechanical\n\nfunc Compute(value int) int {\n\tif value < 0 {\n\t\treturn -value\n\t}\n\treturn value * 2\n}\n",
	})
	harness.git("add", "--", candidatePath)
	harness.git("commit", "-q", "-m", "test: seed the reviewed candidate")

	// Follow the negotiated route exactly: begin from STATUS, never hardcode
	// or substitute START, and run only the exact returned execute command
	// until STATUS itself hands back a collect transition. --focus=risk keeps
	// the review to one lens; the one-time medium-risk consent question is
	// answered with --consent=granted for this exact candidate.
	status := codexNegotiatedStatus(t, harness, lineage)
	for round := 0; round < 5 && status.NextTransition != nil && status.NextTransition.Kind == "execute"; round++ {
		execute := status.NextTransition.Execute
		if execute == nil {
			t.Fatalf("execute transition with no execute payload: %#v", status.NextTransition)
		}
		arguments := organicCommandArguments(t, execute.Command)
		if execute.Operation == "review.start" {
			if organicArgumentValue(arguments, "--focus") == "" {
				arguments = append(arguments, "--focus=risk")
			}
			if organicArgumentValue(arguments, "--consent") == "relay" {
				arguments = organicReplaceArgument(arguments, "--consent", "granted")
			}
		}
		harness.gentle(arguments...)
		status = codexNegotiatedStatus(t, harness, lineage)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != "collect" || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("expected exactly one reviewer collect input for a single-lens standard review: %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	binding := organicCollectBindingFields(t, input)
	if binding["lens"] != "review-risk" {
		t.Fatalf("--focus risk did not select review-risk: %#v", binding)
	}
	manifestPaths := organicManifestPaths(input)
	if len(manifestPaths) != 1 || manifestPaths[0] != candidatePath {
		t.Fatalf("changed_path_manifest = %v, want exactly [%s]", manifestPaths, candidatePath)
	}

	// Poison the worktree strictly AFTER the candidate froze: the reviewed
	// file and a brand-new secret-shaped file.
	poisonedCandidate := "package mechanical\n\n// " + codexPoisonMarker + "\nfunc Compute(value int) int {\n\treturn 0\n}\n"
	if err := os.WriteFile(filepath.Join(harness.repo.worktree, candidatePath), []byte(poisonedCandidate), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness.repo.worktree, "SECRET.txt"), []byte(codexPoisonMarker+"\nsk-live-not-a-real-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return codexPoisonedReviewSetup{
		harness: harness, lineage: lineage, candidatePath: candidatePath,
		binding: binding, manifestPaths: manifestPaths,
	}
}

// codexAdvisoryPrompt fetches the exact canonical prompt for the codex
// runtime through the CLI's own generic prompt/text seam
// (`review advisory prompt`), the same command a real Codex-orchestrated
// session would run from the collect input's own repository-context and
// lens. It never builds a caller-authored request.
func (setup codexPoisonedReviewSetup) codexAdvisoryPrompt(t *testing.T) string {
	t.Helper()
	payload := setup.harness.gentle(
		"review", "advisory", "prompt",
		"--repository-context", setup.binding["repository-context"],
		"--lens", setup.binding["lens"],
		"--runtime", "codex",
	)
	return strings.TrimRight(string(payload), "\n")
}

// TestRealCodexReviewerOrdinarySessionAdmitsRawOutput is the Codex organic
// runtime proof the shared advisory transport (rdd-advisory-transport
// SKILL.md) requires before Codex may be advertised: a genuinely launched
// review-risk lens, run by a real `codex exec` process through
// advisoryreview.CodexAdapter in an empty scratch directory it creates and
// deletes itself, still only ever sees the frozen candidate the provider
// rendered into its prompt -- not the reviewed file's live poisoned content
// and not a brand-new secret-shaped file. It also proves the raw-output-to-
// native-admission boundary end to end: the real codex process's raw final
// text is routed through the exact native capture operation the negotiated
// collect step named, then finalized through to a terminal receipt and one
// delivery gate.
//
// The three fail-closed companions below share this same setup and codex
// call, exercising native admission's malformed-input and subject-mismatch
// refusals against the identical genuine raw bytes before finally admitting
// them, plus a dedicated transport-failure case using a deliberately expired
// context.
func TestRealCodexReviewerOrdinarySessionAdmitsRawOutput(t *testing.T) {
	requireCodexOrganicReviewer(t)
	setup := setupCodexPoisonedReview(t, "codex-poisoned-worktree-ordinary-session")

	prompt := setup.codexAdvisoryPrompt(t)
	if strings.Contains(prompt, codexPoisonMarker) {
		t.Fatalf("poison marker leaked into the provider-rendered advisory prompt:\n%s", prompt)
	}

	adapter := advisoryreview.NewCodexAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), organicAgentTimeout)
	defer cancel()
	raw, err := adapter.Review(ctx, prompt)
	if err != nil {
		t.Fatalf("real codex reviewer call failed: %v", err)
	}
	if strings.Contains(string(raw), codexPoisonMarker) {
		t.Fatalf("poison marker (worktree file or secret file) reached the reviewer's raw output:\n%s", raw)
	}
	t.Logf("real codex raw output: %s", raw)

	// --- Negative: malformed output refused, no authority mutation. ---
	// Truncated mid-object, never a second trailing object: native admission
	// extracts one complete JSON object from surrounding prose but rejects an
	// unterminated one (docs/review-integration.md), so this must genuinely
	// fail to parse rather than merely carry ignorable trailing garbage.
	closingBrace := bytes.LastIndexByte(raw, '}')
	if closingBrace < 0 {
		t.Fatalf("real codex raw output has no closing brace to truncate before: %q", raw)
	}
	// Cut strictly before the final '}' rather than a fixed byte count: a
	// fixed-width truncation only stays unterminated by coincidence, since
	// trailing prose length varies run to run and could leave the cut point
	// past the object's own close, producing an accidentally well-formed
	// negative case. Cutting at the last '}' keeps the JSON object itself
	// genuinely unterminated regardless of how much trailing prose codex adds.
	malformed := raw[:closingBrace]
	malformedPath := setup.harness.writeJSON("codex-malformed.json", json.RawMessage("null"))
	if err := os.WriteFile(malformedPath, malformed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := setup.captureResult(malformedPath); err == nil {
		t.Fatal("native admission accepted malformed codex output")
	}
	if status := codexNegotiatedStatus(t, setup.harness, setup.lineage); status.NextTransition == nil || status.NextTransition.Kind != "collect" {
		t.Fatalf("malformed capture attempt mutated the negotiated transition: %#v", status.NextTransition)
	}

	// --- Negative: subject/lens mismatch refused, no authority mutation. ---
	wrongSubject := strings.Replace(string(raw), setup.binding["subject-hash"], "sha256:"+strings.Repeat("d", 64), 1)
	if wrongSubject == string(raw) {
		t.Fatalf("raw codex output does not contain the expected subject_hash %q to mutate:\n%s", setup.binding["subject-hash"], raw)
	}
	mismatchedPath := setup.harness.writeJSON("codex-mismatched.json", json.RawMessage("null"))
	if err := os.WriteFile(mismatchedPath, []byte(wrongSubject), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := setup.captureResult(mismatchedPath); err == nil {
		t.Fatal("native admission accepted a subject-mismatched codex output")
	}
	if status := codexNegotiatedStatus(t, setup.harness, setup.lineage); status.NextTransition == nil || status.NextTransition.Kind != "collect" {
		t.Fatalf("mismatched capture attempt mutated the negotiated transition: %#v", status.NextTransition)
	}

	// --- Positive: the genuine raw output is admitted and reaches a receipt. ---
	resultPath := setup.harness.writeJSON("codex-result.json", json.RawMessage("null"))
	if err := os.WriteFile(resultPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := setup.captureResult(resultPath); err != nil {
		t.Fatalf("native admission refused genuine codex output: %v\n%s", err, stderr)
	}

	// A single-lens medium-risk review only reaches "validating" once its
	// reviewer results are captured; final verification evidence is a
	// distinct, required second step before the review can reach a terminal,
	// gate-eligible state (mirrors organicHarness.approveReview).
	if result := setup.harness.finalize(setup.lineage, "--captured-results=true"); result.State != organicStateValidating {
		t.Fatalf("captured codex result did not reach validation: %#v", result)
	}
	setup.harness.finalize(setup.lineage, "--evidence", setup.harness.writeEvidence())
	if _, err := os.Stat(filepath.Join(setup.harness.commonDir(), "gentle-ai", "review-transactions", "v2")); err != nil {
		t.Fatalf("captured reviewer result never created review authority: %v", err)
	}

	gate := setup.harness.gate("pre-push")
	if gate.Result != organicGateAllow {
		t.Fatalf("pre-push gate result = %q, want %q: %#v", gate.Result, organicGateAllow, gate)
	}
}

// captureResult runs the exact `review capture-result` invocation the
// negotiated collect step named for this binding, with the caller-supplied
// input file.
func (setup codexPoisonedReviewSetup) captureResult(inputPath string) (string, string, error) {
	return setup.harness.gentleAllowFailure(
		"review", "capture-result",
		"--repository-context", setup.binding["repository-context"],
		"--expected-revision", setup.binding["expected-revision"],
		"--lineage", setup.binding["lineage"],
		"--target", setup.binding["target"],
		"--lens", setup.binding["lens"],
		"--order", setup.binding["order"],
		"--subject-hash", setup.binding["subject-hash"],
		"--input", inputPath,
	)
}

// TestRealCodexReviewerTransportFailureStrandsNothing is the timeout/
// transport-failure fail-closed companion: an already-expired context makes
// the real codex binary a genuine transport failure (the process is killed
// before it can answer), CodexAdapter reports it as a typed transport error,
// and the negotiated review is untouched -- no capture, no consumed
// correction budget, no stranded lineage. STATUS still offers the exact same
// collect slot afterward.
func TestRealCodexReviewerTransportFailureStrandsNothing(t *testing.T) {
	requireCodexOrganicReviewer(t)
	setup := setupCodexPoisonedReview(t, "codex-transport-failure")
	prompt := setup.codexAdvisoryPrompt(t)

	before := codexNegotiatedStatus(t, setup.harness, setup.lineage)
	if before.NextTransition == nil || before.NextTransition.Kind != "collect" {
		t.Fatalf("fixture did not reach a collect transition: %#v", before.NextTransition)
	}

	adapter := advisoryreview.NewCodexAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	raw, err := adapter.Review(ctx, prompt)
	if err == nil {
		t.Fatalf("Review() with an already-expired context = %q, nil, want a transport failure", raw)
	}
	t.Logf("codex transport failure (expected): %v", err)

	after := codexNegotiatedStatus(t, setup.harness, setup.lineage)
	if after.NextTransition == nil || after.NextTransition.Kind != "collect" {
		t.Fatalf("transport failure mutated the negotiated transition: before=%#v after=%#v", before.NextTransition, after.NextTransition)
	}
	if after.NextTransition.Collect == nil || len(after.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("collect transition carried no collect inputs to index: %#v", after.NextTransition)
	}
	afterBinding := organicCollectBindingFields(t, after.NextTransition.Collect.Inputs[0])
	beforeBinding := organicCollectBindingFields(t, before.NextTransition.Collect.Inputs[0])
	if afterBinding["lineage"] != beforeBinding["lineage"] ||
		afterBinding["target"] != beforeBinding["target"] ||
		afterBinding["expected-revision"] != beforeBinding["expected-revision"] ||
		afterBinding["order"] != beforeBinding["order"] ||
		afterBinding["subject-hash"] != beforeBinding["subject-hash"] {
		t.Fatalf("transport failure changed the pending collect binding: before=%#v after=%#v", beforeBinding, afterBinding)
	}
}
