package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

// TestReviewLensContextFocusesOnDesignWhenVerifyEvidenceIsPresent is S13: an
// independent verify's per-spec verdicts ride in the same request file, and
// the lens treats those specs as checked instead of re-checking them.
func TestReviewLensContextFocusesOnDesignWhenVerifyEvidenceIsPresent(t *testing.T) {
	content := requestContextFixture + "\n## Verify\n\nS1 PASS probe: `budget set --year 1999` exits 2.\nS2 PASS probe: `budget show` output matches the base byte for byte.\n"
	request := writeRequestContextFile(t, content)
	_, args, _ := startRequestContextReview(t, "request-context-verify", "--request-context", request)

	block := lensContextBlock(t, args, args[slices.Index(args, "--lens")+1])
	instruction, _ := lensContextSection(block, "GENTLE_AI_REVIEW_INSTRUCTION")
	for _, required := range []string{"Verify evidence.", "already checked", "design, security, and maintainability"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("instruction omits %q:\n%s", required, instruction)
		}
	}
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
