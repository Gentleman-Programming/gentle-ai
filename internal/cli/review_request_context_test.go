package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

const requestContextFixture = "Feature: budget set\n\nS1 `budget set --year <year>` rejects years before 2000 with exit code 2.\nS2 Existing `budget show` output stays unchanged.\n"

// writeRequestContextFile writes the request outside the repository, so the
// file itself never becomes part of the reviewed candidate.
func writeRequestContextFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadRequestContextRecord(t *testing.T, repo, lineage string) reviewtransaction.CompactRecord {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// startRequestContextReview starts one negotiated medium review over a small
// tracked change and returns the lens-context arguments for its only lens.
func startRequestContextReview(t *testing.T, lineage string, extra ...string) (string, []string, ReviewIntegrationStartResult) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	started := runNegotiatedReviewStartWith(t, repo, lineage, extra...)
	if len(started.SelectedLenses) == 0 {
		t.Fatal("fixture selected no lens; it no longer exercises the lens context")
	}
	args := []string{
		"--cwd", repo, "--repository-context", started.RepositoryContext.Handle,
		"--expected-revision", started.RepositoryContext.Revision,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--lens", started.SelectedLenses[0],
	}
	return repo, args, started
}

// TestReviewStartFreezesRequestContext is S10's input: START freezes the
// verbatim request and its hash beside the policy, and a resumed START keeps
// that frozen content instead of rebinding a different request.
func TestReviewStartFreezesRequestContext(t *testing.T) {
	request := writeRequestContextFile(t, requestContextFixture)
	repo, _, started := startRequestContextReview(t, "request-context-freeze", "--request-context", request)

	record := loadRequestContextRecord(t, repo, started.LineageID)
	wantHash := facadePayloadHash([]byte(requestContextFixture))
	if record.State.FrozenRequestContext == nil || *record.State.FrozenRequestContext != requestContextFixture ||
		record.State.RequestContextHash != wantHash {
		t.Fatalf("START did not freeze the request context: hash=%q content=%v", record.State.RequestContextHash, record.State.FrozenRequestContext)
	}
	if record.State.InitialAtomicStart == nil || record.State.InitialAtomicStart.RequestContextHash != wantHash {
		t.Fatalf("START binding does not carry the request context hash: %#v", record.State.InitialAtomicStart)
	}

	// The live file may change after START; the frozen copy is what reviews.
	if err := os.WriteFile(request, []byte("S1 a different request.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", started.LineageID, "--request-context", request,
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "atomic_start_conflict") {
		t.Fatalf("START resumed with a different request context: %v\n%s", err, output.String())
	}
	if after := loadRequestContextRecord(t, repo, started.LineageID); after.Revision != record.Revision {
		t.Fatal("a refused resume rewrote the frozen authority")
	}
}

// TestReviewStartRefusesRepeatedRequestContext keeps the START binding
// allowlist closed: two request files would silently drop one of them.
func TestReviewStartRefusesRepeatedRequestContext(t *testing.T) {
	err := validateReviewStartBinding([]string{"--request-context", "a.md", "--request-context", "b.md"}, false, "", "workspace", "", "", false, false, "", "", "")
	if err == nil || !strings.Contains(err.Error(), "repeats --request-context") {
		t.Fatalf("repeated --request-context error = %v", err)
	}
}

// TestReviewConsentFollowUpKeepsRequestContext proves the relayed consent
// answer reruns START with the same request, never silently without it.
func TestReviewConsentFollowUpKeepsRequestContext(t *testing.T) {
	base := reviewConsentFollowUpBase("/repo", "sha256:target", "", reviewtransaction.ProjectionWorkspace,
		"", "", "", "reliability", "", false, false, ReviewIntegrationContractV2, "", "",
		reviewIntendedUntrackedScope{Intended: []string{}})
	if reviewRequestContextFollowUpArgument("") != "" {
		t.Fatal("absent --request-context changed the consent follow-up")
	}
	command := base + reviewRequestContextFollowUpArgument("/tmp/request one.md")
	if !strings.Contains(command, "--request-context '/tmp/request one.md'") {
		t.Fatalf("consent follow-up dropped --request-context: %s", command)
	}
}

// TestReviewLensContextCarriesRequestContext is S10's output: the lens block
// carries the frozen request as its own section, and the instruction tells the
// lens to judge unmet requirements and unrequested scope against it.
func TestReviewLensContextCarriesRequestContext(t *testing.T) {
	request := writeRequestContextFile(t, requestContextFixture)
	_, args, _ := startRequestContextReview(t, "request-context-lens", "--request-context", request)

	block := lensContextBlock(t, args, args[slices.Index(args, "--lens")+1])
	section, found := lensContextSection(block, "GENTLE_AI_REVIEW_REQUEST_CONTEXT")
	if !found || section != strings.TrimSpace(requestContextFixture) {
		t.Fatalf("lens block does not carry the verbatim request context:\n%s", block)
	}
	instruction, _ := lensContextSection(block, "GENTLE_AI_REVIEW_INSTRUCTION")
	for _, required := range []string{"GENTLE_AI_REVIEW_REQUEST_CONTEXT", "requested requirement the candidate does not meet", "unrequested scope"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("instruction omits %q:\n%s", required, instruction)
		}
	}
	if strings.Contains(instruction, "Verify evidence.") {
		t.Fatalf("instruction claims verify evidence the request does not carry:\n%s", instruction)
	}
	if strings.Index(block, "GENTLE_AI_REVIEW_REQUEST_CONTEXT\n") > strings.Index(block, "GENTLE_AI_REVIEW_NAME_STATUS") {
		t.Fatal("request context appears after the candidate evidence")
	}
}

// requestContextVerifyPassFixture is a request whose verify section reports
// every spec as passing: evidence a lens may read, never proof that removes a
// requirement from its scope (S13 as reopened by S21).
const requestContextVerifyPassFixture = requestContextFixture + "\n## Verify\n\nS1 PASS probe: `budget set --year 1999` exits 2.\nS2 PASS probe: `budget show` output matches the base byte for byte.\n"

// requireVerifyEvidenceKeepsScope checks the lens block for a frozen request
// carrying Verify PASS verdicts: the evidence stays visible verbatim, the
// lens keeps its own mandate, and the charge neither skips the passing specs
// nor vouches that they were checked on the candidate under review.
func requireVerifyEvidenceKeepsScope(t *testing.T, block, lens string) {
	t.Helper()
	section, found := lensContextSection(block, "GENTLE_AI_REVIEW_REQUEST_CONTEXT")
	if !found || section != strings.TrimSpace(requestContextVerifyPassFixture) {
		t.Fatalf("lens block does not carry the verify evidence verbatim:\n%s", block)
	}
	instruction, _ := lensContextSection(block, "GENTLE_AI_REVIEW_INSTRUCTION")
	for _, granted := range []string{"already checked", "do not re-check", "of this same candidate", "spend your review on"} {
		if strings.Contains(instruction, granted) {
			t.Fatalf("a reported PASS still removes requirements from the lens scope (%q):\n%s", granted, instruction)
		}
	}
	_, focus, _ := reviewtransaction.LensMandate(lens)
	for _, required := range []string{"Verify evidence.", "including specs it reports as passing", "never removes a requirement from your scope", focus} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("instruction omits %q:\n%s", required, instruction)
		}
	}
}

// TestReviewLensContextKeepsVerifyPassSpecsInScope is S13 as reopened by S21:
// a verify's per-spec verdicts ride in the request file and inform the lens,
// but a textual PASS does not remove those specs from its review.
func TestReviewLensContextKeepsVerifyPassSpecsInScope(t *testing.T) {
	request := writeRequestContextFile(t, requestContextVerifyPassFixture)
	_, args, _ := startRequestContextReview(t, "request-context-verify", "--request-context", request)

	lens := args[slices.Index(args, "--lens")+1]
	requireVerifyEvidenceKeepsScope(t, lensContextBlock(t, args, lens), lens)
}

// TestReviewStartCountsRequestContextAgainstLensBudget proves the request is
// part of the reviewer prompt the budget bounds: a request that cannot fit is
// refused by START before any authority exists, never truncated.
func TestReviewStartCountsRequestContextAgainstLensBudget(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	request := writeRequestContextFile(t, strings.Repeat("S1 requirement line\n", reviewLensContextByteBudget/20+1))
	authorityBefore := snapshotAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "request-context-budget", "--request-context", request,
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("over-budget request context START error = %v\n%s", err, output.String())
	}
	if after := snapshotAuthorityTree(t, reviewCLIAuthorityRoot(t, repo)); after != authorityBefore {
		t.Fatal("over-budget request context START persisted authority")
	}
}

// TestReviewStartBudgetRefusalNamesOversizedRequestContext is A1: when the
// candidate alone fits and the request is what overflows, the refusal names
// --request-context and says to shorten or omit it instead of asking for a
// split that cannot help.
func TestReviewStartBudgetRefusalNamesOversizedRequestContext(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	request := writeRequestContextFile(t, strings.Repeat("S1 requirement line\n", reviewLensContextByteBudget/20+1))

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "request-context-remedy", "--request-context", request,
	}), &output)
	if err == nil {
		t.Fatal("over-budget request context START succeeded")
	}
	refusal := err.Error() + output.String()
	for _, required := range []string{"--request-context", "shorten", "omit"} {
		if !strings.Contains(refusal, required) {
			t.Fatalf("budget refusal omits %q:\n%s", required, refusal)
		}
	}
	if strings.Contains(refusal, "split") || strings.Contains(refusal, "smaller candidates") {
		t.Fatalf("budget refusal asks to split a candidate that fits alone:\n%s", refusal)
	}
}

// TestReviewWithoutRequestContextIsUnchanged is the PRESERVE contract: no
// flag means no frozen fields, no section, and no request paragraph.
func TestReviewWithoutRequestContextIsUnchanged(t *testing.T) {
	repo, args, started := startRequestContextReview(t, "request-context-absent")

	record := loadRequestContextRecord(t, repo, started.LineageID)
	if record.State.RequestContextHash != "" || record.State.FrozenRequestContext != nil ||
		record.State.InitialAtomicStart == nil || record.State.InitialAtomicStart.RequestContextHash != "" {
		t.Fatalf("absent --request-context froze request fields: %#v", record.State)
	}
	block := lensContextBlock(t, args, args[slices.Index(args, "--lens")+1])
	if strings.Contains(block, "GENTLE_AI_REVIEW_REQUEST_CONTEXT") || strings.Contains(block, "unrequested scope") {
		t.Fatalf("absent --request-context changed the lens block:\n%s", block)
	}
}

// TestReviewRecoverInheritsFrozenRequestContext proves a recovered successor
// keeps judging against the request its predecessor froze.
func TestReviewRecoverInheritsFrozenRequestContext(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := writeRequestContextFile(t, requestContextFixture)
	started := runNegotiatedReviewStartWith(t, repo, "request-context-recover", "--request-context", request)
	escalateReviewForRecovery(t, repo, ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, SelectedLenses: started.SelectedLenses,
	})
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\nmore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	predecessor := loadRequestContextRecord(t, repo, started.LineageID)
	var output bytes.Buffer
	if err := RunReview([]string{
		"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "request-context-successor",
		"--disposition", string(reviewtransaction.RecoveryEscalated),
	}, &output); err != nil {
		t.Fatalf("recover: %v\n%s", err, output.String())
	}
	successor := loadRequestContextRecord(t, repo, "request-context-successor")
	if successor.State.FrozenRequestContext == nil || *successor.State.FrozenRequestContext != requestContextFixture ||
		successor.State.RequestContextHash != predecessor.State.RequestContextHash {
		t.Fatalf("recovered successor lost the frozen request context: hash=%q content=%v", successor.State.RequestContextHash, successor.State.FrozenRequestContext)
	}
}

// TestReviewRecoveredLensContextKeepsInheritedVerifyPassInScope is the S21
// regression: a correction changes the candidate, recovery inherits the frozen
// request with the predecessor's Verify PASS verdicts, and the successor's
// lens still sees that evidence but must judge the changed candidate against
// every requirement instead of treating the inherited PASS as checked.
func TestReviewRecoveredLensContextKeepsInheritedVerifyPassInScope(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := writeRequestContextFile(t, requestContextVerifyPassFixture)
	started := runNegotiatedReviewStartWith(t, repo, "request-context-verify-recover", "--request-context", request)
	escalateReviewForRecovery(t, repo, ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, SelectedLenses: started.SelectedLenses,
	})
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\nmore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	predecessor := loadRequestContextRecord(t, repo, started.LineageID)
	var output bytes.Buffer
	if err := RunReview([]string{
		"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "request-context-verify-successor",
		"--disposition", string(reviewtransaction.RecoveryEscalated),
	}, &output); err != nil {
		t.Fatalf("recover: %v\n%s", err, output.String())
	}

	// The successor's lens context is reached through the collect transition
	// STATUS publishes, exactly as a runtime would reach it.
	output.Reset()
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
		"--lineage", "request-context-verify-successor", "--next-transition",
	}, &output); err != nil {
		t.Fatalf("successor STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("successor next transition = %#v", status.NextTransition)
	}
	args, lens, target := []string{"--cwd", repo}, "", ""
	for _, argument := range status.NextTransition.Collect.Inputs[0].Arguments {
		switch argument.Name {
		case "lens":
			lens = argument.Value
		case "target":
			target = argument.Value
		}
		args = append(args, "--"+argument.Name, argument.Value)
	}
	if lens == "" || target == "" || target == started.RepositoryContext.TargetIdentity {
		t.Fatalf("successor collect does not review a changed candidate: lens=%q target=%q predecessor=%q", lens, target, started.RepositoryContext.TargetIdentity)
	}
	requireVerifyEvidenceKeepsScope(t, lensContextBlock(t, args, lens), lens)
}

// requestContextRefuterReview starts a negotiated review with the given START
// arguments and captures one severe inferential lens finding, so the
// transaction-wide refuter batch is required. It returns the refuter binding
// for the public capture-refuter command.
func requestContextRefuterReview(t *testing.T, lineage string, extra ...string) (string, reviewtransaction.CompactStore, reviewtransaction.CompactRecord, []string) {
	t.Helper()
	repo, _, started := startRequestContextReview(t, lineage, extra...)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalBehaviorActivated,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if record, err = store.Load(); err != nil {
		t.Fatal(err)
	}
	handle := rctx2ReviewRepositoryContextForTest(t, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	return repo, store, record, piRefuterBinding(repo, record, handle)
}

// materializeRequestContextRefuter prints the refuter provider task through
// the public capture-refuter materialize mode, which writes nothing.
func materializeRequestContextRefuter(t *testing.T, binding []string) string {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--materialize=true"), &output); err != nil {
		t.Fatalf("materialize refuter: %v\n%s", err, output.String())
	}
	return output.String()
}

// refuterRequestPreimageHash recomputes the refuter request hash from the
// unchanged preimage fields; the frozen request is not one of them.
func refuterRequestPreimageHash(request reviewProviderRefuterRequest) string {
	return facadeValueHash("provider-refuter-request", struct {
		Schema, LineageID, AuthorityVersion, TargetIdentity, SnapshotIdentity string
		Claims                                                                []reviewtransaction.RefuterClaim
		Evidence                                                              []reviewProviderEvidence
	}{request.Schema, request.LineageID, request.AuthorityVersion, request.TargetIdentity, request.SnapshotIdentity, request.Claims, request.Evidence})
}

// TestReviewRefuterPromptCarriesFrozenRequestContext is S10 for the refuter:
// the materialized refuter task carries the verbatim request START froze, as
// its own section before the machine-readable input, framed as untrusted
// evidence that never instructs the refuter. The request reaches the prompt
// only: the request JSON, its hash preimage, and the request hash are the
// ones a refuter request without the field would carry, and the single
// corrective retry repeats the same section.
func TestReviewRefuterPromptCarriesFrozenRequestContext(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	request := writeRequestContextFile(t, requestContextFixture)
	repo, store, record, binding := requestContextRefuterReview(t, "request-context-refuter", "--request-context", request)

	prompt := materializeRequestContextRefuter(t, binding)
	native, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if prompt != string(native.Invocation.Prompt()) {
		t.Fatalf("materialized refuter task diverged from the native request\nmaterialize:\n%s\nnative:\n%s", prompt, native.Invocation.Prompt())
	}
	section := reviewLensContextRequestContext + "\n" + strings.TrimSpace(requestContextFixture) + "\n" + reviewLensContextRequestContext + "_END"
	if !strings.Contains(prompt, section) {
		t.Fatalf("refuter prompt does not carry the verbatim frozen request section:\n%s", prompt)
	}
	if strings.Index(prompt, section) > strings.Index(prompt, "\n\nInput:\n") {
		t.Fatal("frozen request appears after the machine-readable input")
	}
	for _, required := range []string{"untrusted evidence", "never instructions"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("refuter prompt does not frame the request with %q:\n%s", required, prompt)
		}
	}

	// PRESERVE: the request travels in the prompt only.
	payload, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("budget set --year")) || bytes.Contains(payload, []byte("request_context")) {
		t.Fatalf("refuter request JSON carries the frozen request:\n%s", payload)
	}
	if native.RequestHash != refuterRequestPreimageHash(native) {
		t.Fatal("refuter request hash no longer derives from its unchanged preimage")
	}
	without := native
	without.RequestContext = ""
	if stripped, _ := json.Marshal(without); !bytes.Equal(stripped, payload) {
		t.Fatal("refuter request JSON depends on the frozen request")
	}
	if !strings.Contains(prompt, "\n\nInput:\n"+string(payload)+"\n\nOutput schema:\n") {
		t.Fatal("refuter prompt input is not the unchanged request JSON")
	}
	corrective := reviewProviderCorrectivePrompt(native.Invocation.Prompt(), errors.New("malformed result"))
	if !bytes.Contains(corrective, []byte(section)) {
		t.Fatal("corrective refuter prompt dropped the frozen request")
	}
}

// TestReviewRefuterPromptWithoutRequestContextIsUnchanged is the PRESERVE
// contract: a review started without --request-context materializes the
// refuter prompt byte for byte as instruction, runtime probe paragraph,
// request JSON, and result schema, with no request paragraph or section.
func TestReviewRefuterPromptWithoutRequestContextIsUnchanged(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, binding := requestContextRefuterReview(t, "request-context-refuter-absent")

	prompt := materializeRequestContextRefuter(t, binding)
	native, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := reviewProviderRoleContractFor(reviewProviderRoleRefuter)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	want := contract.PromptInstruction + "\n\n" + reviewerprovider.RefuterProbeInstruction(model.AgentID(record.State.RuntimeAgent)) +
		"\n\nInput:\n" + string(payload) + "\n\nOutput schema:\n" + string(contract.ResultSchema)
	if prompt != want {
		t.Fatalf("refuter prompt without a request changed\ngot:\n%s\nwant:\n%s", prompt, want)
	}
	if native.RequestHash != refuterRequestPreimageHash(native) {
		t.Fatal("refuter request hash no longer derives from its unchanged preimage")
	}
}

// TestReviewStartCountsRequestContextAgainstRefuterEnvelope proves the
// frozen request is charged to the refuter envelope START measures, not only
// to the lens block. The candidate is quote-dense, so its JSON-escaped
// refuter evidence is about twice its raw lens evidence: with the request,
// the lens block still fits the runtime cap while the refuter prompt cannot.
// START must refuse before any authority exists instead of freezing a
// lineage whose refuter would later fail its budget deterministically.
func TestReviewStartCountsRequestContextAgainstRefuterEnvelope(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", strings.Repeat(strings.Repeat(`"`, 40)+"\n", 1_700), 0o644)
	request := writeRequestContextFile(t, strings.Repeat("S1 requirement line\n", 4_000))
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	authorityBefore := snapshotAuthorityTree(t, authorityRoot)
	homeBefore := readLegacyAuthorityTree(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "request-context-refuter-budget", "--request-context", request,
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("START admitted a request its refuter prompt cannot carry: %v\n%s", err, output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) || failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.Phase != "preflight" {
		t.Fatalf("refuter-envelope budget refusal does not report a refusal that wrote nothing: %#v", err)
	}
	// The candidate alone fits every role, so the remedy names the request.
	if refusal := err.Error() + output.String(); !strings.Contains(refusal, "shorten") || strings.Contains(refusal, "split") {
		t.Fatalf("refuter-envelope budget refusal does not name the oversized request:\n%s", refusal)
	}
	if after := snapshotAuthorityTree(t, authorityRoot); after != authorityBefore {
		t.Fatal("refuter-envelope budget refusal persisted authority")
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("refuter-envelope budget refusal persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}
