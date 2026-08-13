// freeze_test.go pins the CURRENT advisory provider-contract behavior before the typed extraction (change #3138, slice 1): approval/freeze tests that pass immediately and stay green through the byte-identical extraction (REQ-RPC-2, SEN-RPC-3). Exercised via the public Prompt/PromptFor/Validate surfaces; budgets pinned (SEN-RPC-1/2). No production file touched.
package advisoryreview

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func freezeRequestWithEntries(t *testing.T, n int) Request {
	t.Helper()
	manifest := make([]reviewtransaction.ChangedPathManifestEntry, n)
	paths := make([]string, n)
	evidence := make([]Evidence, n)
	for index := 0; index < n; index++ {
		path := fmt.Sprintf("internal/gen%02d.go", index) // zero-padded: generation order == canonical lexical order
		manifest[index] = reviewtransaction.ChangedPathManifestEntry{Path: path, Status: reviewtransaction.CandidatePathModified, OldMode: "100644", NewMode: "100644"}
		paths[index] = path
		evidence[index] = Evidence{Path: path, Content: "package gen\n"}
	}
	state := reviewtransaction.CompactState{LineageID: "review-advisory-freeze", SelectedLenses: []string{reviewtransaction.LensReliability}, InitialSnapshot: reviewtransaction.Snapshot{Identity: "sha256:" + strings.Repeat("a", 64), BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("b", 40), Paths: paths}}
	subject, err := reviewtransaction.NewArtifactSubject(state, "sha256:"+strings.Repeat("c", 64), reviewtransaction.FrozenCandidateContext{BaseTree: state.InitialSnapshot.BaseTree, CandidateTree: state.InitialSnapshot.CandidateTree, ChangedPathManifest: manifest}, reviewtransaction.LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return Request{ArtifactSubject: subject, ChangedPathManifest: manifest, Evidence: evidence}
}

// TestFreezePromptBytesAreByteStable pins the SEN-RPC-3 pre-extraction baseline: Prompt renders identical bytes for equal input, every supported runtime's PromptFor returns exactly those bytes, and the bound request identity is embedded.
func TestFreezePromptBytesAreByteStable(t *testing.T) {
	request := testRequest(t)
	first, err := Prompt(request)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Prompt(request); err != nil || second != first {
		t.Fatalf("Prompt() is not byte-stable for equal input: %v", err)
	}
	if !strings.Contains(first, request.ArtifactSubject.SubjectHash) {
		t.Fatal("Prompt() omits the bound request identity from its bytes")
	}
	for _, runtime := range SupportedRuntimes() {
		rendered, err := PromptFor(runtime, request)
		if err != nil || rendered != first {
			t.Fatalf("PromptFor(%s) diverges from Prompt(): %v", runtime, err)
		}
	}
}

// TestFreezeValidateAdmissionMirrorsNativeValidation pins SEN-RPC-1: Validate admits exactly the bytes native reviewtransaction.ValidateReviewerResult admits, no more, no less.
func TestFreezeValidateAdmissionMirrorsNativeValidation(t *testing.T) {
	request := testRequest(t)
	raw, err := json.Marshal(testResult(request))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewtransaction.ValidateReviewerResult(raw, request.ArtifactSubject, request.ChangedPathManifest); err != nil {
		t.Fatalf("freeze fixture no longer admitted natively: %v", err)
	}
	validated, err := Validate(raw, request)
	if err != nil {
		t.Fatalf("Validate() refused natively admitted bytes: %v", err)
	}
	if !validated.TransportValidated || validated.FindingCount != 1 || validated.Result.SubjectHash != request.ArtifactSubject.SubjectHash {
		t.Fatalf("Validate() = %#v, want a transport-validated binding to the request subject", validated)
	}
	rejected := strings.Replace(string(raw), request.ArtifactSubject.SubjectHash, "sha256:"+strings.Repeat("d", 64), 1)
	if _, err := reviewtransaction.ValidateReviewerResult([]byte(rejected), request.ArtifactSubject, request.ChangedPathManifest); err == nil {
		t.Fatal("mismatched-subject fixture should fail native validation")
	}
	if _, err := Validate([]byte(rejected), request); err == nil {
		t.Fatal("Validate() admitted bytes native validation rejects")
	}
}

// TestFreezeBudgetsPinnedAndEnforcedBeforeInvocation pins the budgets (MaxEvidenceEntries=32, maxResultBytes=64KiB, maxEvidenceBytes==MaxFrozenCandidateDiffBytes) and proves SEN-RPC-1/2: the pre-invocation gates refuse at the boundary, never truncating.
func TestFreezeBudgetsPinnedAndEnforcedBeforeInvocation(t *testing.T) {
	if MaxEvidenceEntries != 32 {
		t.Fatalf("MaxEvidenceEntries = %d, want 32", MaxEvidenceEntries)
	}
	if maxResultBytes != 64<<10 {
		t.Fatalf("maxResultBytes = %d, want 64KiB", maxResultBytes)
	}
	if maxEvidenceBytes != reviewtransaction.MaxFrozenCandidateDiffBytes {
		t.Fatalf("maxEvidenceBytes = %d, want MaxFrozenCandidateDiffBytes (%d)", maxEvidenceBytes, reviewtransaction.MaxFrozenCandidateDiffBytes)
	}
	// SEN-RPC-1: the count budget admits exactly MaxEvidenceEntries entries.
	if _, err := Prompt(freezeRequestWithEntries(t, MaxEvidenceEntries)); err != nil {
		t.Fatalf("request at MaxEvidenceEntries was refused: %v", err)
	}
	overCount := freezeRequestWithEntries(t, MaxEvidenceEntries+1)
	if _, err := Validate([]byte("{}"), overCount); err == nil {
		t.Fatal("Validate() admitted evidence past MaxEvidenceEntries")
	}
	if prompt, err := Prompt(overCount); err == nil {
		t.Fatalf("Prompt() rendered for evidence past MaxEvidenceEntries: %q", prompt)
	}
	// SEN-RPC-2: one byte past maxEvidenceBytes refuses on full size; exactly-at-limit still renders, so nothing is truncated.
	big := freezeRequestWithEntries(t, 1)
	limit := int(reviewtransaction.MaxFrozenCandidateDiffBytes)
	big.Evidence[0].Content = strings.Repeat("x", limit-len(big.Evidence[0].Path))
	if _, err := Prompt(big); err != nil {
		t.Fatalf("evidence at exactly maxEvidenceBytes was refused: %v", err)
	}
	big.Evidence[0].Content += "x"
	if _, err := Validate([]byte("{}"), big); err == nil || !strings.Contains(err.Error(), "exceeds the") {
		t.Fatalf("evidence past maxEvidenceBytes not refused on full size: %v", err)
	}
	// SEN-RPC-2 boundary for model output: one byte past maxResultBytes is refused before any schema validation.
	if _, err := Validate([]byte(strings.Repeat("x", maxResultBytes+1)), testRequest(t)); err == nil || !strings.Contains(err.Error(), "between 1 and") {
		t.Fatalf("raw past maxResultBytes not refused before validation: %v", err)
	}
}
