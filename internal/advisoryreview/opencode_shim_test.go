package advisoryreview

// Slice-4 RED tests for the native OpenCode shim (tasks 4.1, design #3138).
//
// The shim contract (REQ-RPC-5/6): accept the opaque canonical reviewer task
// prompt and return a provider block to inject, defer while the legacy
// review-result-artifacts.ts review half is still installed (SEN-RPC-17), or
// refuse with the preserved #3049 provenance text (REQ-RPC-6). Raw completed
// task output passes through `review shim-result` unchanged except for the
// runtime's <task_result> envelope, which Go removes (SEN-RPC-5/6).
//
// These tests are RED by construction: they reference OpenCodeShim,
// NewOpenCodeShim, Dispatch, DispatchResult, UnwrapReviewerTaskResult, and
// ShimProvenanceRefusal, none of which exist until task 4.2 creates
// opencode_shim.go.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// shimTestPrompt builds the opaque task prompt the driver's task call carries:
// the provider binding on the first line plus caller prose that provider
// injection must discard.
func shimTestPrompt(t *testing.T, repositoryContext string) string {
	t.Helper()
	return "GENTLE_AI_REVIEW_BINDING {\"lineage\":\"lin_1\",\"target\":\"tgt_1\",\"lens\":\"review-risk\",\"order\":0," +
		"\"revision\":\"rev_1\",\"repository_context\":\"" + repositoryContext + "\",\"subject_hash\":\"subject_1\"}\n" +
		"caller prose that provider injection must discard"
}

// shimTestShim returns an OpenCodeShim with every seam faked so the unit
// tests never touch the filesystem or spawn a process. produceCalls records
// the (repositoryContext, lens) pair the ProduceBlock seam receives.
func shimTestShim(t *testing.T, legacyInstalled bool, produceCalls *[]struct{ repositoryContext, lens string }) *OpenCodeShim {
	t.Helper()
	return &OpenCodeShim{
		ReadPinned: func() (PinnedRuntime, error) {
			return PinnedRuntime{
				Executable: "/opt/gentle-ai/gentle-ai",
				SHA256:     "sha256:" + strings.Repeat("a", 64),
				Version:    "gentle-ai test-version",
			}, nil
		},
		CurrentProcess: func() (PinnedRuntime, error) {
			return PinnedRuntime{
				Executable: "/opt/gentle-ai/gentle-ai",
				SHA256:     "sha256:" + strings.Repeat("a", 64),
				Version:    "gentle-ai test-version",
			}, nil
		},
		LegacyReviewPluginInstalled: func() (bool, error) { return legacyInstalled, nil },
		ProduceBlock: func(_ context.Context, repositoryContext, lens string) ([]byte, error) {
			*produceCalls = append(*produceCalls, struct{ repositoryContext, lens string }{repositoryContext, lens})
			return []byte("GENTLE_AI_REVIEW_CONTEXT block\nGENTLE_AI_REVIEW_CONTEXT_END\n"), nil
		},
	}
}

func TestOpenCodeShimDefersWhileLegacyReviewPluginInstalled(t *testing.T) {
	// SEN-RPC-17: between the shim ship (slice 4) and the plugin removal
	// (slice 6), a user with the old plugin who has not re-synced must receive
	// exactly one injection; the shim defers and the legacy plugin remains the
	// sole injection source. The deferral happens BEFORE provenance admission
	// and BEFORE any block production: the shim does not take over even when
	// the pin is valid.
	var produceCalls []struct{ repositoryContext, lens string }
	shim := shimTestShim(t, true, &produceCalls)

	result, err := shim.Dispatch(context.Background(), shimTestPrompt(t, "rctx_1"), "review-risk")
	if err != nil {
		t.Fatalf("Dispatch() error = %v, want deferral without error", err)
	}
	if !result.Deferred {
		t.Fatal("Dispatch() did not defer while the legacy review plugin is installed")
	}
	if result.Block != nil {
		t.Fatalf("Dispatch() produced a block while deferring: %q", result.Block)
	}
	if len(produceCalls) != 0 {
		t.Fatalf("Dispatch() produced a provider block while deferring (SEN-RPC-17 double injection): %#v", produceCalls)
	}
}

func TestOpenCodeShimDispatchRefusesProvenanceAbsence(t *testing.T) {
	// REQ-RPC-6 (#3049): an absent opencode_runtime_provenance record refuses
	// with the exact refusal text the managed plugin authors, never a weaker
	// or reworded message.
	shim := shimTestShim(t, false, &[]struct{ repositoryContext, lens string }{})
	shim.ReadPinned = func() (PinnedRuntime, error) {
		return PinnedRuntime{}, errors.New("no opencode_runtime_provenance recorded")
	}

	_, err := shim.Dispatch(context.Background(), shimTestPrompt(t, "rctx_1"), "review-risk")
	if err == nil {
		t.Fatal("Dispatch() admitted a review without any provenance record")
	}
	if err.Error() != ShimProvenanceRefusal {
		t.Fatalf("provenance-absence refusal = %q, want the exact preserved text %q", err.Error(), ShimProvenanceRefusal)
	}
}

func TestOpenCodeShimDispatchRefusesProvenanceMismatch(t *testing.T) {
	// REQ-RPC-6 + SEN-RPC-8: a synced runtime whose recorded identity no
	// longer matches the running gentle-ai binary refuses with the preserved
	// refusal text in every mismatch class the old plugin's pinnedRuntime()
	// checked: executable path, sha256 digest, and version output.
	pinned := PinnedRuntime{
		Executable: "/opt/gentle-ai/gentle-ai",
		SHA256:     "sha256:" + strings.Repeat("a", 64),
		Version:    "gentle-ai test-version",
	}
	for _, scenario := range []struct {
		name   string
		mutate func(*PinnedRuntime)
	}{
		{name: "replaced executable", mutate: func(r *PinnedRuntime) { r.Executable = "/usr/local/bin/gentle-ai" }},
		{name: "different digest", mutate: func(r *PinnedRuntime) { r.SHA256 = "sha256:" + strings.Repeat("b", 64) }},
		{name: "drifted version", mutate: func(r *PinnedRuntime) { r.Version = "gentle-ai other-version" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			current := pinned
			scenario.mutate(&current)
			shim := shimTestShim(t, false, &[]struct{ repositoryContext, lens string }{})
			shim.CurrentProcess = func() (PinnedRuntime, error) { return current, nil }

			_, err := shim.Dispatch(context.Background(), shimTestPrompt(t, "rctx_1"), "review-risk")
			if err == nil {
				t.Fatal("Dispatch() admitted a review with a mismatched runtime identity")
			}
			if err.Error() != ShimProvenanceRefusal {
				t.Fatalf("provenance-mismatch refusal = %q, want the exact preserved text %q", err.Error(), ShimProvenanceRefusal)
			}
		})
	}
}

func TestOpenCodeShimDispatchRefusesWhenCurrentProcessUndeterminable(t *testing.T) {
	// REQ-RPC-6: if the running process's own identity cannot be derived, the
	// shim refuses rather than admit a review it cannot bind to the synced
	// binary. A refusal is a transport outcome, never a verdict (SEN-RPC-20):
	// no block, no ValidatedResult.
	shim := shimTestShim(t, false, &[]struct{ repositoryContext, lens string }{})
	shim.CurrentProcess = func() (PinnedRuntime, error) {
		return PinnedRuntime{}, errors.New("resolve current gentle-ai executable: test failure")
	}

	result, err := shim.Dispatch(context.Background(), shimTestPrompt(t, "rctx_1"), "review-risk")
	if err == nil || err.Error() != ShimProvenanceRefusal {
		t.Fatalf("Dispatch() = (%#v, %v), want refusal with the preserved text", result, err)
	}
	if result.Block != nil {
		t.Fatalf("provenance refusal still produced a block: %q", result.Block)
	}
}

func TestOpenCodeShimDispatchProducesBlockUntouched(t *testing.T) {
	// SEN-RPC-6 (unit level): with a valid pin and an ordinary session, the
	// shim routes the opaque binding to the block seam with the parsed
	// repository-context handle and the exact selected lens, and returns the
	// provider block byte-for-byte.
	var produceCalls []struct{ repositoryContext, lens string }
	shim := shimTestShim(t, false, &produceCalls)

	result, err := shim.Dispatch(context.Background(), shimTestPrompt(t, "rctx_1"), "review-risk")
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Deferred {
		t.Fatal("Dispatch() deferred without the legacy plugin installed")
	}
	if len(produceCalls) != 1 {
		t.Fatalf("ProduceBlock calls = %d, want exactly 1", len(produceCalls))
	}
	if produceCalls[0].repositoryContext != "rctx_1" || produceCalls[0].lens != "review-risk" {
		t.Fatalf("ProduceBlock routed (%q, %q), want (\"rctx_1\", \"review-risk\")", produceCalls[0].repositoryContext, produceCalls[0].lens)
	}
	if string(result.Block) != "GENTLE_AI_REVIEW_CONTEXT block\nGENTLE_AI_REVIEW_CONTEXT_END\n" {
		t.Fatalf("Dispatch() altered the provider block: %q", result.Block)
	}
}

func TestOpenCodeShimDispatchRefusesMissingBindingRoute(t *testing.T) {
	// The binding is opaque provider data the shim only routes on: without the
	// one repository-context handle it needs, the reviewer must never launch.
	var produceCalls []struct{ repositoryContext, lens string }
	shim := shimTestShim(t, false, &produceCalls)

	result, err := shim.Dispatch(context.Background(), "caller prose with no binding at all", "review-risk")
	if err == nil || !strings.Contains(err.Error(), "requires a repository-context binding") {
		t.Fatalf("Dispatch() = (%#v, %v), want repository-context binding refusal", result, err)
	}
	if len(produceCalls) != 0 {
		t.Fatalf("ProduceBlock called without a repository-context handle: %#v", produceCalls)
	}
	if result.Block != nil {
		t.Fatalf("missing-binding refusal still produced a block: %q", result.Block)
	}
}

func TestOpenCodeShimDispatchFailsClosedOnUnwiredSeams(t *testing.T) {
	// Fail-closed wiring: an OpenCodeShim whose seams were never wired must
	// return a transport error, never panic and never admit anything.
	shim := NewOpenCodeShim()
	shim.ProduceBlock = func(context.Context, string, string) ([]byte, error) {
		t.Fatal("unwired shim called ProduceBlock")
		return nil, nil
	}
	shim.LegacyReviewPluginInstalled = func() (bool, error) {
		t.Fatal("unwired shim scanned the legacy plugin")
		return false, nil
	}
	shim.ReadPinned = func() (PinnedRuntime, error) {
		t.Fatal("unwired shim read the provenance record")
		return PinnedRuntime{}, nil
	}
	shim.CurrentProcess = func() (PinnedRuntime, error) {
		t.Fatal("unwired shim derived the current process identity")
		return PinnedRuntime{}, nil
	}

	shim.ReadPinned = nil
	result, err := shim.Dispatch(context.Background(), shimTestPrompt(t, "rctx_1"), "review-risk")
	if err == nil || !strings.Contains(err.Error(), "not wired") {
		t.Fatalf("Dispatch() with unwired ReadPinned = (%#v, %v), want fail-closed transport error", result, err)
	}
}

func TestOpenCodeShimProvenanceRefusalPreservesOriginalText(t *testing.T) {
	// Protective pin on the #3049 refusal contract: the Go constant must keep
	// the typed refusal code and the exact recovery instruction the managed
	// plugin authors so the migration window never shows users two different
	// explanations.
	if !strings.HasPrefix(ShimProvenanceRefusal, "opencode_runtime_provenance_invalid: ") {
		t.Fatalf("ShimProvenanceRefusal lost the typed refusal code: %q", ShimProvenanceRefusal)
	}
	if !strings.Contains(ShimProvenanceRefusal, "Run `gentle-ai sync` from the intended installation, then relaunch the reviewer.") {
		t.Fatalf("ShimProvenanceRefusal lost the sync recovery instruction: %q", ShimProvenanceRefusal)
	}
}

func TestUnwrapReviewerTaskResultPassesThroughRawTextWhenNoEnvelope(t *testing.T) {
	// The old plugin's reviewerResult passed through raw text when the runtime
	// did not wrap it in a <task_result> envelope; the Go unwrap must behave
	// identically (SEN-RPC-5: raw bytes out, no fabrication).
	raw := "{\"subject_hash\":\"subject_1\",\"inspection\":{\"status\":\"completed\"},\"findings\":[],\"evidence\":[\"inspected\"]}"
	got, err := UnwrapReviewerTaskResult(raw)
	if err != nil {
		t.Fatalf("UnwrapReviewerTaskResult() error = %v", err)
	}
	if got != raw {
		t.Fatalf("UnwrapReviewerTaskResult() = %q, want the raw text unchanged", got)
	}
}

func TestUnwrapReviewerTaskResultUnwrapsCompletedTaskEnvelope(t *testing.T) {
	raw := "{\"subject_hash\":\"subject_1\",\"findings\":[],\"evidence\":[\"inspected\"]}"
	wrapped := "<task id=\"reviewer_1\" state=\"completed\">\n<task_result>\n" + raw + "\n</task_result>\n</task>"
	got, err := UnwrapReviewerTaskResult(wrapped)
	if err != nil {
		t.Fatalf("UnwrapReviewerTaskResult() error = %v", err)
	}
	if got != raw {
		t.Fatalf("UnwrapReviewerTaskResult() = %q, want envelope unwrapped to %q", got, raw)
	}
}

func TestUnwrapReviewerTaskResultRefusesEmptyOutput(t *testing.T) {
	for _, value := range []string{"", "   \n\t "} {
		if _, err := UnwrapReviewerTaskResult(value); err == nil || err.Error() != "reviewer output must not be empty" {
			t.Fatalf("UnwrapReviewerTaskResult(%q) = %v, want empty-output refusal", value, err)
		}
	}
}

func TestUnwrapReviewerTaskResultRefusesMalformedEnvelope(t *testing.T) {
	mangled := "<task id=\"reviewer_1\" state=\"failed\">\n<task_result>\n{}\n</task_result>\n</task>"
	if _, err := UnwrapReviewerTaskResult(mangled); err == nil || err.Error() != "reviewer output contains a malformed task result envelope" {
		t.Fatalf("UnwrapReviewerTaskResult(malformed) = %v, want malformed-envelope refusal", err)
	}
}

func TestUnwrapReviewerTaskResultRefusesEmptyEnvelopeBody(t *testing.T) {
	empty := "<task id=\"reviewer_1\" state=\"completed\">\n<task_result>\n\n</task_result>\n</task>"
	if _, err := UnwrapReviewerTaskResult(empty); err == nil || err.Error() != "reviewer task result is empty" {
		t.Fatalf("UnwrapReviewerTaskResult(empty envelope) = %v, want empty-result refusal", err)
	}
}

func TestUnwrapReviewerTaskResultRefusesNestedEnvelope(t *testing.T) {
	nested := "<task id=\"reviewer_1\" state=\"completed\">\n<task_result>\n<task id=\"inner\" state=\"completed\">\n<task_result>\n{}\n</task_result>\n</task>\n</task_result>\n</task>"
	if _, err := UnwrapReviewerTaskResult(nested); err == nil || err.Error() != "reviewer task result contains a nested task envelope" {
		t.Fatalf("UnwrapReviewerTaskResult(nested) = %v, want nested-envelope refusal", err)
	}
}
