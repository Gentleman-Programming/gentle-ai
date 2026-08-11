package advisoryreview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func testRequest(t *testing.T) Request {
	t.Helper()
	manifest := []reviewtransaction.ChangedPathManifestEntry{
		{Path: "internal/example.go", Status: reviewtransaction.CandidatePathModified, OldMode: "100644", NewMode: "100644"},
		{Path: "internal/other.go", Status: reviewtransaction.CandidatePathModified, OldMode: "100644", NewMode: "100644"},
	}
	state := reviewtransaction.CompactState{
		LineageID:      "review-advisory",
		SelectedLenses: []string{reviewtransaction.LensReliability},
		InitialSnapshot: reviewtransaction.Snapshot{
			Identity:      "sha256:" + strings.Repeat("a", 64),
			BaseTree:      strings.Repeat("a", 40),
			CandidateTree: strings.Repeat("b", 40),
			Paths:         []string{"internal/example.go", "internal/other.go"},
		},
	}
	subject, err := reviewtransaction.NewArtifactSubject(state, "sha256:"+strings.Repeat("c", 64), reviewtransaction.FrozenCandidateContext{
		BaseTree: state.InitialSnapshot.BaseTree, CandidateTree: state.InitialSnapshot.CandidateTree, ChangedPathManifest: manifest,
	}, reviewtransaction.LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return Request{
		ArtifactSubject: subject, ChangedPathManifest: manifest,
		Evidence: []Evidence{{Path: "internal/example.go", Content: "package example\n"}, {Path: "internal/other.go", Content: "package other\n"}},
	}
}

func testResult(request Request) reviewtransaction.ReviewerResult {
	return reviewtransaction.ReviewerResult{
		SubjectHash: request.ArtifactSubject.SubjectHash, Lens: request.ArtifactSubject.Lens,
		Inspection: reviewtransaction.ArtifactInspection{Status: reviewtransaction.ArtifactInspectionCompleted, Paths: []string{"internal/other.go", "internal/example.go"}},
		Findings:   []reviewtransaction.Finding{{ID: "R3-001", Lens: "reliability", Location: "internal/example.go:1", Severity: "BLOCKER", Claim: "example", EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced, ProofRefs: []string{"internal/example.go:1"}}},
		Evidence:   []string{"inspected internal/example.go"},
	}
}

func TestPromptForUsesOneCanonicalContractForSupportedRuntimes(t *testing.T) {
	request := testRequest(t)
	title, focus, ok := reviewtransaction.LensMandate(request.ArtifactSubject.Lens)
	if !ok {
		t.Fatal("test request lens has no canonical mandate")
	}
	var canonical string
	for _, runtime := range []Runtime{RuntimeClaudeCode, RuntimeOpenCode, RuntimeCodex} {
		t.Run(string(runtime), func(t *testing.T) {
			prompt, err := PromptFor(runtime, request)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, OutputSchema) || !strings.Contains(prompt, "advisory model output for native Go admission") {
				t.Fatalf("prompt does not carry the canonical advisory contract: %q", prompt)
			}
			if !strings.Contains(prompt, title) || !strings.Contains(prompt, focus) {
				t.Fatalf("prompt omits the canonical lens mandate %q / %q:\n%s", title, focus, prompt)
			}
			if canonical == "" {
				canonical = prompt
			} else if prompt != canonical {
				t.Fatalf("%s received a runtime-specific prompt", runtime)
			}
		})
	}
	if got, want := strings.Join(RuntimeNames(), ","), "claude-code,codex,opencode"; got != want {
		t.Fatalf("runtime projection = %q, want %q", got, want)
	}
}

// TestPromptForSupportsCodexRuntimeAfterOrganicProof is the activation-backed
// successor to the former TestPromptForRefusesUnadvertisedCodexRuntime: once
// TestRealCodexReviewerOrdinarySessionAdmitsRawOutput and its fail-closed
// companions (e2e/organicruntime) proved a real codex exec process could
// carry the canonical prompt and reach native admission, Codex stopped being
// refused here and became a genuinely advertised runtime like Claude and
// OpenCode.
func TestPromptForSupportsCodexRuntimeAfterOrganicProof(t *testing.T) {
	if !RuntimeCodex.Supports() {
		t.Fatal("Codex should be advertised once its organic runtime proof passed")
	}
	if got := RuntimeNames(); !slicesContain(got, string(RuntimeCodex)) {
		t.Fatalf("RuntimeNames() does not advertise codex: %v", got)
	}
	prompt, err := PromptFor(RuntimeCodex, testRequest(t))
	if err != nil {
		t.Fatalf("PromptFor(codex) = %q, %v, want the canonical prompt", prompt, err)
	}
	if !strings.Contains(prompt, OutputSchema) {
		t.Fatalf("PromptFor(codex) omits the published output schema: %q", prompt)
	}
}

// TestPromptForRefusesGenuinelyUnknownRuntime keeps the typed-unavailable
// refusal path covered with a runtime name that has never been, and will
// never legitimately become, a recognized identity -- distinct from Codex,
// which is now a proven and advertised runtime.
func TestPromptForRefusesGenuinelyUnknownRuntime(t *testing.T) {
	const unknown Runtime = "unknown-runtime"
	if unknown.Supports() {
		t.Fatal("an unrecognized runtime name must never be advertised")
	}
	if got := RuntimeNames(); slicesContain(got, string(unknown)) {
		t.Fatalf("RuntimeNames() advertises the unknown runtime: %v", got)
	}
	if prompt, err := PromptFor(unknown, testRequest(t)); err == nil {
		t.Fatalf("PromptFor(%q) = %q, nil, want a typed unavailable refusal", unknown, prompt)
	}
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPromptForDoesNotRequireOpenCodeIsolationEnvironment(t *testing.T) {
	t.Setenv("OPENCODE_DISABLE_PROJECT_CONFIG", "")
	t.Setenv("OPENCODE_DISABLE_EXTERNAL_SKILLS", "")
	if _, err := PromptFor(RuntimeOpenCode, testRequest(t)); err != nil {
		t.Fatalf("normal OpenCode session was refused: %v", err)
	}
}

func TestValidateRejectsMalformedOrMismatchedResults(t *testing.T) {
	request := testRequest(t)
	valid, err := json.Marshal(testResult(request))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "not JSON", raw: "not-json"},
		{name: "unknown pass field", raw: strings.TrimSuffix(string(valid), "}") + `,"pass":true}`},
		{name: "subject mismatch", raw: strings.Replace(string(valid), request.ArtifactSubject.SubjectHash, "sha256:"+strings.Repeat("d", 64), 1)},
		{name: "lens mismatch", raw: strings.Replace(string(valid), request.ArtifactSubject.Lens, "review-risk", 1)},
		{name: "multiple objects", raw: string(valid) + "\n" + string(valid)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Validate([]byte(test.raw), request); err == nil {
				t.Fatal("Validate() = nil, want refusal")
			}
		})
	}
}

func TestValidateMatchesNativeReviewerResultRules(t *testing.T) {
	request := testRequest(t)
	tests := []struct {
		name    string
		mutate  func(*reviewtransaction.ReviewerResult)
		wantErr bool
	}{
		{name: "finding lens is accepted", mutate: func(result *reviewtransaction.ReviewerResult) { result.Findings[0].Lens = "reliability" }},
		{name: "omitted lens binding canonicalizes", mutate: func(result *reviewtransaction.ReviewerResult) { result.Lens = "" }},
		{name: "finding ID must use native schema", mutate: func(result *reviewtransaction.ReviewerResult) { result.Findings[0].ID = "bad-id" }, wantErr: true},
		{name: "proof reference must be concrete", mutate: func(result *reviewtransaction.ReviewerResult) { result.Findings[0].ProofRefs = []string{"TODO"} }, wantErr: true},
		{name: "evidence must be concrete", mutate: func(result *reviewtransaction.ReviewerResult) { result.Evidence = []string{"passed"} }, wantErr: true},
		{name: "classification must use native enum", mutate: func(result *reviewtransaction.ReviewerResult) {
			result.Findings[0].Severity = "WARNING"
			result.Findings[0].EvidenceClass = "unknown"
		}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := testResult(request)
			test.mutate(&result)
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Validate(raw, request)
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidatedResultPreservesBlockingFindingsForNativeAdmission(t *testing.T) {
	request := testRequest(t)
	raw, err := json.Marshal(testResult(request))
	if err != nil {
		t.Fatal(err)
	}
	validated, err := Validate(raw, request)
	if err != nil {
		t.Fatal(err)
	}
	if !validated.TransportValidated || validated.RawSHA256 == "" || validated.FindingCount != 1 || validated.Result.Findings[0].Severity != "BLOCKER" {
		t.Fatalf("validated advisory result = %#v", validated)
	}
	payload, err := json.Marshal(validated)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"receipt", "delivery", "pass", "approved", "authorized"} {
		if strings.Contains(strings.ToLower(string(payload)), forbidden) {
			t.Fatalf("advisory result leaked authority field %q: %s", forbidden, payload)
		}
	}
}
