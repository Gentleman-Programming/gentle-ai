package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Issues #3942, #2791 and #1867: an in-process reviewer runtime returns free
// text, so one out-of-schema or truncated result used to be a dead end. The
// capture now grants exactly one corrective re-invocation that carries the
// exact admission error, and every rejected payload is preserved outside the
// authority store so a report can quote the bytes.

// reviewerPayloadWithNestedLens injects the #3942 shape: a `lens` field
// inside `inspection`, which the strict decoder rejects as unknown.
func reviewerPayloadWithNestedLens(t *testing.T, valid []byte) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(valid, &object); err != nil {
		t.Fatal(err)
	}
	var inspection map[string]any
	if err := json.Unmarshal(object["inspection"], &inspection); err != nil {
		t.Fatal(err)
	}
	inspection["lens"] = "review-reliability"
	nested, err := json.Marshal(inspection)
	if err != nil {
		t.Fatal(err)
	}
	object["inspection"] = nested
	payload, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// recordingProviderAdapter installs a fake in-process reviewer that answers
// the scripted payloads in order and records every prompt it received.
func recordingProviderAdapter(t *testing.T, answers ...func() ([]byte, error)) *[][]byte {
	t.Helper()
	prompts := &[][]byte{}
	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
			*prompts = append(*prompts, invocation.Prompt())
			if len(*prompts) > len(answers) {
				t.Fatalf("provider reviewer was invoked %d times, want at most %d", len(*prompts), len(answers))
			}
			return answers[len(*prompts)-1]()
		}), nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	return prompts
}

func rejectedResultsDir(t *testing.T, repo, lineage string) string {
	t.Helper()
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(lease.Identity().GitCommonDir, "gentle-ai", reviewRejectedResultDirName, lineage)
}

func readRejectedResults(t *testing.T, dir string) map[string]reviewRejectedResultEnvelope {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rejected results %s: %v", dir, err)
	}
	envelopes := map[string]reviewRejectedResultEnvelope{}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("rejected result %s mode = %o, want 0600", path, info.Mode().Perm())
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var envelope reviewRejectedResultEnvelope
		decodeStrictReviewJSON(t, payload, &envelope)
		envelopes[path] = envelope
	}
	return envelopes
}

func assertRejectedEnvelope(t *testing.T, envelope reviewRejectedResultEnvelope, lineage, lens string, attempt int, raw []byte) {
	t.Helper()
	digest := sha256.Sum256(raw)
	if envelope.Schema != reviewRejectedResultSchema || envelope.LineageID != lineage || envelope.Lens != lens ||
		envelope.Attempt != attempt || envelope.RawSHA256 != hex.EncodeToString(digest[:]) || envelope.Raw != string(raw) ||
		envelope.Reason == "" || envelope.CapturedAt == "" {
		t.Fatalf("rejected result envelope = %#v, want lineage %q lens %q attempt %d", envelope, lineage, lens, attempt)
	}
}

func TestProviderCaptureRetriesOnceWithAdmissionFeedback(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	valid, err := json.Marshal(admittedReviewerResultForTest(t, repo, record, lens, 0))
	if err != nil {
		t.Fatal(err)
	}
	invalid := reviewerPayloadWithNestedLens(t, valid)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return invalid, nil },
		func() ([]byte, error) { return valid, nil },
	)

	var output bytes.Buffer
	if err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode)), &output); err != nil {
		t.Fatalf("capture with one corrective attempt: %v\n%s", err, output.String())
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 2", len(*prompts))
	}
	first, second := (*prompts)[0], (*prompts)[1]
	if !bytes.HasPrefix(second, first) {
		t.Fatal("corrective prompt does not start with the original materialized prompt")
	}
	for _, want := range []string{reviewProviderCorrectiveFeedbackHeader, `unknown field "lens"`, "exactly one JSON object", reviewLensContextResultSchema} {
		if !bytes.Contains(second, []byte(want)) {
			t.Fatalf("corrective prompt lacks %q:\n%s", want, second[len(first):])
		}
	}
	if bytes.Contains(first, []byte(reviewProviderCorrectiveFeedbackHeader)) {
		t.Fatal("first prompt already carries admission feedback")
	}

	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected results = %d, want 1", len(preserved))
	}
	for path, envelope := range preserved {
		if !strings.HasPrefix(filepath.Base(path), lens+"-1-") {
			t.Fatalf("rejected result file name = %q, want <lens>-1-<sha>.json", filepath.Base(path))
		}
		assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, invalid)
	}
}

func TestProviderCaptureRefusesAfterTwoRejectedResultsAndPreservesBoth(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	valid, err := json.Marshal(admittedReviewerResultForTest(t, repo, record, lens, 0))
	if err != nil {
		t.Fatal(err)
	}
	invalid := reviewerPayloadWithNestedLens(t, valid)
	truncated := []byte(`{"subject_hash":"sha256:0","inspection":{"status":"completed","paths":["tracked.txt"]},"findings":[{"id":"R3-001","proof_refs":["x"`)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return invalid, nil },
		func() ([]byte, error) { return truncated, nil },
	)

	var output bytes.Buffer
	err = RunReview(append(append([]string{"capture-result"}, binding...), "--agent", string(model.AgentClaudeCode)), &output)
	failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
	if failure.Code != reviewIntegrationInvalidRequestCode || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("second rejection envelope = %#v", failure)
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 2", len(*prompts))
	}

	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 2 {
		t.Fatalf("preserved rejected results = %d, want 2", len(preserved))
	}
	for path, envelope := range preserved {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("refusal does not name preserved payload %s: %v", path, err)
		}
		switch envelope.Attempt {
		case 1:
			assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, invalid)
		case 2:
			assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 2, truncated)
		default:
			t.Fatalf("unexpected preserved attempt %#v", envelope)
		}
	}
	for _, want := range []string{`unknown field "lens"`, "no complete JSON object", "gentle-ai review status"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal lacks %q: %v", want, err)
		}
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != reviewtransaction.StateReviewing || !reflect.DeepEqual(after.State, record.State) {
		t.Fatalf("two rejected results mutated reviewing authority: before=%#v after=%#v", record.State, after.State)
	}
}

func TestProviderCaptureTransportErrorSkipsCorrectionAndPreservation(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return nil, errors.New("reviewer subprocess exited 137") },
		func() ([]byte, error) { t.Fatal("transport failure must not be retried"); return nil, nil },
	)
	err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode)), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invoke provider reviewer") || !strings.Contains(err.Error(), "exited 137") {
		t.Fatalf("transport failure = %v", err)
	}
	if len(*prompts) != 1 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 1", len(*prompts))
	}
	if _, statErr := os.Stat(rejectedResultsDir(t, repo, record.State.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("transport failure preserved a rejected result: %v", statErr)
	}
}

// R4-budget-skip-unclassified: an over-budget corrective prompt must still
// classify as *reviewProviderCaptureRefusedError, carry the preserved clause,
// and skip the second invocation.
func TestProviderCaptureSkipsCorrectionOverBudgetAndClassifiesAsRefused(t *testing.T) {
	invocation := reviewerprovider.NewInvocation([]byte("original prompt"))
	admit := func(_ context.Context, _ []byte) (string, error) {
		return "", errors.New(strings.Repeat("x", reviewLensContextByteBudget))
	}
	preserve := func(_ context.Context, _ int, _ error, _ []byte) string { return "preserved-clause" }
	continuation := func() string { return "gentle-ai review status --next-transition" }
	reviewCalls := 0
	adapter := providerTestAdapterFunc(func(_ context.Context, _ reviewerprovider.Invocation) ([]byte, error) {
		reviewCalls++
		return []byte("raw reviewer output"), nil
	})

	_, _, err := reviewProviderCaptureRetry(context.Background(), adapter, invocation, admit, preserve, continuation, nil)
	var refused *reviewProviderCaptureRefusedError
	if !errors.As(err, &refused) || !strings.Contains(err.Error(), "preserved-clause") {
		t.Fatalf("over-budget error is not a classified refusal carrying the preserved clause: %v", err)
	}
	if reviewCalls != 1 {
		t.Fatalf("adapter invocations = %d, want exactly 1 (no corrective re-invocation)", reviewCalls)
	}
}

func TestRawInputCapturePreservesRejectedResultWithoutInvokingProvider(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	valid, err := json.Marshal(admittedReviewerResultForTest(t, repo, record, lens, 0))
	if err != nil {
		t.Fatal(err)
	}
	invalid := reviewerPayloadWithNestedLens(t, valid)
	prompts := recordingProviderAdapter(t)
	input := filepath.Join(t.TempDir(), "rejected.json")
	if err := os.WriteFile(input, invalid, 0o600); err != nil {
		t.Fatal(err)
	}

	err = RunReviewCaptureResult(append(binding, "--input", input), io.Discard)
	if err == nil || !strings.Contains(err.Error(), `unknown field "lens"`) {
		t.Fatalf("raw input rejection = %v", err)
	}
	if len(*prompts) != 0 {
		t.Fatalf("raw input capture invoked the provider reviewer %d times", len(*prompts))
	}
	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected results = %d, want 1", len(preserved))
	}
	for path, envelope := range preserved {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("refusal does not name preserved payload %s: %v", path, err)
		}
		assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, invalid)
	}
}

// Issue #4027: the in-process reviewer sometimes echoed the transaction's
// target_identity in the subject_hash field instead of the lens binding's own
// subject_hash. Admission correctly refused it (the fail-closed default), but
// that made the lens slot permanently unfillable whenever the model repeated
// the same mistake. The fix corrects this one specific, explainable
// confusion -- the model reading an adjacent field of the same prompt --
// before admission, deterministically and without a second invocation.
func TestProviderCaptureCorrectsSubjectHashEchoedAsTargetIdentity(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	correctSubjectHash := result.SubjectHash
	if correctSubjectHash == record.State.InitialSnapshot.Identity {
		t.Fatal("test fixture's subject hash coincides with target identity; cannot exercise the confusion")
	}
	result.SubjectHash = record.State.InitialSnapshot.Identity // the exact #4027 confusion
	misbound, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	prompts := recordingProviderAdapter(t, func() ([]byte, error) { return misbound, nil })

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode)), &output); err != nil {
		t.Fatalf("capture with a target-identity-echoed subject hash: %v\n%s", err, output.String())
	}
	if len(*prompts) != 1 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 1 (no correction round needed)", len(*prompts))
	}
	// This lineage selects exactly one lens, so capturing it closes the review:
	// terminal approval -- reached only via a completed admitted lens result --
	// is the strongest available proof that the corrected subject hash, not the
	// echoed target identity, is what admission actually bound.
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Schema != reviewLastEventClosureSchema || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("terminal capture after subject hash correction = %#v", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
}

// A wrong subject_hash that is NOT explainable as the adjacent target_identity
// field must still be refused exactly as before: the correction is narrowly
// scoped to the one confusion #4027 reported, not a general "trust the
// binding always" relaxation.
func TestProviderCaptureStillRefusesAnUnexplainedWrongSubjectHash(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	result.SubjectHash = "sha256:" + strings.Repeat("b", 64)
	wrong, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return wrong, nil },
		func() ([]byte, error) { return wrong, nil },
	)

	err = RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode)), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "binding_mismatch") {
		t.Fatalf("unexplained wrong subject hash = %v, want a binding_mismatch refusal", err)
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 2 (first attempt plus the one corrective retry)", len(*prompts))
	}
}

// reviewProviderCorrectSubjectHashEcho itself, in isolation: the pure
// substitution logic the two capture-level tests above exercise end to end.
func TestReviewProviderCorrectSubjectHashEcho(t *testing.T) {
	const target = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	const expected = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	tests := []struct {
		name string
		raw  []byte
		want string // "" means the input must come back byte-identical
	}{
		{"rewrites an echoed target identity", []byte(`{"subject_hash":"` + target + `","findings":[]}`), expected},
		{"leaves an unrelated wrong value untouched", []byte(`{"subject_hash":"sha256:` + strings.Repeat("9", 64) + `"}`), ""},
		{"leaves a correct echo untouched", []byte(`{"subject_hash":"` + expected + `"}`), ""},
		{"no-ops on malformed payloads instead of guessing", []byte("not json"), ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := reviewProviderCorrectSubjectHashEcho(test.raw, target, expected)
			if test.want == "" {
				if !bytes.Equal(got, test.raw) {
					t.Fatalf("correction touched an input it should not have: %q", got)
				}
				return
			}
			var fields map[string]json.RawMessage
			var echoed string
			if err := json.Unmarshal(got, &fields); err != nil || json.Unmarshal(fields["subject_hash"], &echoed) != nil || echoed != test.want {
				t.Fatalf("corrected subject_hash = %q (err=%v), want %q", got, err, test.want)
			}
			if _, present := fields["findings"]; !present {
				t.Fatal("correction dropped an unrelated field")
			}
		})
	}
}

func TestProviderCaptureRefusesPreflightBeforeInvocationOrPreservation(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) {
			t.Fatal("--preflight must refuse before the provider is invoked")
			return nil, nil
		},
	)
	err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode), "--preflight"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--agent cannot be combined with --preflight") {
		t.Fatalf("preflight with a provider runtime = %v", err)
	}
	if len(*prompts) != 0 {
		t.Fatalf("provider reviewer invocations = %d, want none", len(*prompts))
	}
	if _, statErr := os.Stat(rejectedResultsDir(t, repo, record.State.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("preflight preserved a rejected result: %v", statErr)
	}
}
