package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// hostLensBindingJSON assembles the binding one-liner exactly the way the
// orchestration contract instructs a live host to: the contract's own field
// order (which differs from Go's marshal order only by host choice), values
// copied verbatim from the collect input -- including `order`, which that
// input delivers as a decimal string argument -- and host-owned whitespace.
func hostLensBindingJSON(lineage, target, lens, order, revision, repositoryContext, subjectHash string) string {
	return fmt.Sprintf(
		`{"lineage": %q, "target": %q, "lens": %q, "order": %s, "revision": %q, "repository_context": %q, "subject_hash": %q}`,
		lineage, target, lens, order, revision, repositoryContext, subjectHash,
	)
}

func hostLensReviewFixture(t *testing.T) (repo string, store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord, lens, contextHandle string, subject reviewtransaction.ArtifactSubject) {
	t.Helper()
	repo, _, store, record = newArtifactReview(t, true)
	lens = record.State.SelectedLenses[0]
	contextHandle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(context.Background(), repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	subject = mustArtifactSubject(t, repo, record, lens, 0)
	// The relay runs in a host worktree, which may differ from the provider-bound
	// repository. The rctx2 handle discovers exactly one common-directory
	// registered target, whose compact authority admits review.
	t.Chdir(repo)
	return repo, store, record, lens, contextHandle, subject
}

// TestOpenCodeReviewTransportAdmitsContractShapedHostLensFrame is the live
// defect reproduction: a real OpenCode host assembles the lens binding from
// the collect input per the orchestration contract -- `order` arrives as the
// input's string argument, `subject_hash` is populated, whitespace and key
// treatment are host-owned, and reviewer instructions follow the binding
// line. That first-contact frame must be admitted by field value, start the
// session, and hand the child only Go-rebuilt canonical bytes.
func TestOpenCodeReviewTransportAdmitsContractShapedHostLensFrame(t *testing.T) {
	reviewEnabledHome(t)
	repo, store, record, lens, contextHandle, subject := hostLensReviewFixture(t)
	const injected = "You are the reliability reviewer. Inspect the frozen candidate trees and return one JSON object."
	binding := hostLensBindingJSON(record.State.LineageID, record.State.InitialSnapshot.Identity, lens, `"0"`,
		record.State.CapturePhaseRevision, contextHandle, subject.SubjectHash)
	relay := startOpenCodeTransportRelay(t, repo, openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "start",
		Prompt: reviewLensContextBindingHeader + " " + binding + "\n" + injected,
	})
	if strings.Contains(relay.prompt.Prompt, injected) {
		t.Fatalf("host-authored prompt bytes reached the provider Task: %q", relay.prompt.Prompt)
	}
	taskPrompt, reintercepted, err := decodeOpenCodeTransportMaterialization(relay.prompt.Prompt)
	if err != nil || !reintercepted {
		t.Fatalf("materialization = %q, reintercepted=%v, err=%v", taskPrompt, reintercepted, err)
	}
	canonical, err := json.Marshal(reviewLensContextBinding{
		Lineage: record.State.LineageID, Target: record.State.InitialSnapshot.Identity, Lens: lens, Order: 0,
		Revision: record.State.CapturePhaseRevision, RepositoryContext: contextHandle, SubjectHash: subject.SubjectHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if taskPrompt != reviewLensContextBindingHeader+" "+string(canonical) {
		t.Fatalf("materialized task prompt = %q, want the Go-rebuilt canonical binding line", taskPrompt)
	}
	raw := admittedReviewerPayloadForTest(t, repo, record, lens, 0)
	hostOutput := string(raw)
	completed, err := relay.complete(openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: relay.prompt.Nonce, Output: &hostOutput,
	})
	if err != nil {
		t.Fatal(err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, []byte(*completed.Output), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
		t.Fatalf("transport artifact = %#v", artifact)
	}
	if _, found, err := store.ResolveAdmittedReviewerResult(context.Background(), record.State.CapturePhaseRevision, record.State.InitialSnapshot.Identity,
		mustFrozenContext(t, repo, record), subject); err != nil || !found {
		t.Fatalf("captured host-frame result found=%v err=%v", found, err)
	}
}

// TestOpenCodeReviewTransportAdmitsHostLensFrameWithNumericOrderAndShuffledKeys
// covers the other legitimate host serialization: numeric order and a key
// order different from both the contract listing and Go's marshal order.
func TestOpenCodeReviewTransportAdmitsHostLensFrameWithNumericOrderAndShuffledKeys(t *testing.T) {
	reviewEnabledHome(t)
	repo, _, record, lens, contextHandle, subject := hostLensReviewFixture(t)
	binding := fmt.Sprintf(
		`{"subject_hash": %q, "repository_context": %q, "revision": %q, "order": 0, "lens": %q, "target": %q, "lineage": %q}`,
		subject.SubjectHash, contextHandle, record.State.CapturePhaseRevision, lens, record.State.InitialSnapshot.Identity, record.State.LineageID,
	)
	relay := startOpenCodeTransportRelay(t, repo, openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "start",
		Prompt: reviewLensContextBindingHeader + " " + binding + "\nReviewer instructions follow.",
	})
	if !strings.HasPrefix(relay.prompt.Prompt, openCodeTransportMaterializationHeader+" ") {
		t.Fatalf("relay prompt = %q", relay.prompt.Prompt)
	}
	if err := relay.closeWithoutCompletion(); err == nil {
		t.Fatal("relay without completion unexpectedly succeeded")
	}
	_ = repo
}

// TestOpenCodeReviewTransportRefusesHostLensFrameValueTampering verifies the
// semantic admission stays fail-closed: every contract field value is checked
// against the Go-resolved authority, unknown keys refuse, and missing
// required fields refuse. No tampered frame may release a provider prompt.
func TestOpenCodeReviewTransportRefusesHostLensFrameValueTampering(t *testing.T) {
	reviewEnabledHome(t)
	repo, store, record, lens, contextHandle, subject := hostLensReviewFixture(t)
	valid := func() map[string]any {
		return map[string]any{
			"lineage": record.State.LineageID, "target": record.State.InitialSnapshot.Identity, "lens": lens,
			"order": 0, "revision": record.State.CapturePhaseRevision, "repository_context": contextHandle, "subject_hash": subject.SubjectHash,
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "unknown key", mutate: func(binding map[string]any) { binding["note"] = "host extension" }},
		{name: "tampered subject hash", mutate: func(binding map[string]any) {
			binding["subject_hash"] = differentOpenCodeTransportTestSHA(subject.SubjectHash)
		}},
		{name: "tampered order", mutate: func(binding map[string]any) { binding["order"] = 3 }},
		{name: "non-numeric order", mutate: func(binding map[string]any) { binding["order"] = "first" }},
		{name: "tampered lens", mutate: func(binding map[string]any) { binding["lens"] = "review-unselected" }},
		{name: "tampered revision", mutate: func(binding map[string]any) {
			binding["revision"] = differentOpenCodeTransportTestSHA(record.State.CapturePhaseRevision)
		}},
		{name: "tampered target", mutate: func(binding map[string]any) {
			binding["target"] = differentOpenCodeTransportTestSHA(record.State.InitialSnapshot.Identity)
		}},
		{name: "tampered lineage", mutate: func(binding map[string]any) { binding["lineage"] = "different-live-lineage" }},
		{name: "missing revision", mutate: func(binding map[string]any) { delete(binding, "revision") }},
		{name: "missing lineage", mutate: func(binding map[string]any) { delete(binding, "lineage") }},
		{name: "missing target", mutate: func(binding map[string]any) { delete(binding, "target") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := discoverOpenCodeRelayRecord(t, repo, record.State.LineageID)
			binding := valid()
			test.mutate(binding)
			encoded, err := json.Marshal(binding)
			if err != nil {
				t.Fatal(err)
			}
			start, err := json.Marshal(openCodeTransportEnvelope{
				Schema: openCodeReviewTransportSchema, Operation: "start",
				Prompt: reviewLensContextBindingHeader + " " + string(encoded) + "\nReviewer instructions follow.",
			})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			transportErr := runReviewOpenCodeTransport(nil, bytes.NewReader(start), &output)
			var bindingErr *openCodeTransportBindingError
			if !errors.As(transportErr, &bindingErr) {
				t.Fatalf("transport error = %v, want typed binding refusal", transportErr)
			}
			if output.Len() != 0 {
				t.Fatalf("tampered host frame released a provider prompt: %q", output.String())
			}
			assertOpenCodeRelayAuthorityUnchanged(t, repo, record.State.LineageID, store, before)
		})
	}
}

// TestOpenCodeReviewTransportAdmitsHostExpandedProviderRoleTask covers the
// role lane a real host drives: the Go-issued provider task binding line
// followed by host-authored task instructions. The frame is admitted by field
// value against the resolved authority, and the child receives only the
// Go-rebuilt materialization -- never the host-authored trailing bytes.
func TestOpenCodeReviewTransportAdmitsHostExpandedProviderRoleTask(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)
	task := openCodeTargetedValidatorTask(t, repo, lineage)
	const injected = "Input:\nmaterialized validator payload"
	relay := startOpenCodeTransportRelay(t, repo, openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: task.Prompt + "\n\n" + injected,
	})
	if strings.Contains(relay.prompt.Prompt, injected) {
		t.Fatalf("host-authored prompt bytes reached the provider Task: %q", relay.prompt.Prompt)
	}
	taskPrompt, reintercepted, err := decodeOpenCodeTransportMaterialization(relay.prompt.Prompt)
	if err != nil || !reintercepted {
		t.Fatalf("materialization = %q, reintercepted=%v, err=%v", taskPrompt, reintercepted, err)
	}
	if taskPrompt != task.Prompt {
		t.Fatalf("materialized task prompt = %q, want the exact Go-issued role binding line", taskPrompt)
	}
	if err := relay.closeWithoutCompletion(); err == nil {
		t.Fatal("relay without completion unexpectedly succeeded")
	}
}

// TestOpenCodeReviewTransportRefusesHostRoleFrameTampering keeps the role
// lane fail-closed under semantic admission: unknown keys and same-line
// trailing JSON still refuse before any provider launch.
func TestOpenCodeReviewTransportRefusesHostRoleFrameTampering(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)
	task := openCodeTargetedValidatorTask(t, repo, lineage)
	line, _, _ := strings.Cut(task.Prompt, "\n")
	encoded, found := strings.CutPrefix(line, reviewProviderTaskBindingHeader+" ")
	if !found {
		t.Fatalf("provider task prompt has no binding: %q", task.Prompt)
	}
	for _, test := range []struct {
		name   string
		prompt string
	}{
		{name: "unknown key", prompt: reviewProviderTaskBindingHeader + " " + strings.TrimSuffix(encoded, "}") + `,"note":"host extension"}`},
		{name: "trailing JSON on the binding line", prompt: reviewProviderTaskBindingHeader + " " + encoded + " {}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			start, err := json.Marshal(openCodeTransportEnvelope{
				Schema: openCodeReviewTransportSchema, Operation: "start", Prompt: test.prompt,
			})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			transportErr := runReviewOpenCodeTransport(nil, bytes.NewReader(start), &output)
			var bindingErr *openCodeTransportBindingError
			if !errors.As(transportErr, &bindingErr) {
				t.Fatalf("transport error = %v, want typed binding refusal", transportErr)
			}
			if output.Len() != 0 {
				t.Fatalf("tampered role frame released a provider prompt: %q", output.String())
			}
		})
	}
	_ = lineage
}

func discoverOpenCodeRelayRecord(t *testing.T, repo, lineage string) reviewtransaction.CompactRecord {
	t.Helper()
	_, record, err := discoverCompactFacadeReview(context.Background(), repo, lineage, false)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
