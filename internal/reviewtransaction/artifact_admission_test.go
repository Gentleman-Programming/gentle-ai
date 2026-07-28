package reviewtransaction

import (
	"strings"
	"testing"
)

func admittedArtifactFixture(t *testing.T) (ArtifactSubject, FrozenCandidateContext, ArtifactAdmissionRequest) {
	t.Helper()
	state, revision, context := artifactSubjectFixture(t)
	subject, err := NewArtifactSubject(state, revision, context, LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	result := LensResult{
		Lens: LensReliability,
		Findings: []Finding{{
			ID: "R3-001", Lens: "reliability", Location: "internal/a.go:7", Severity: "WARNING",
			Claim: "the candidate loses the retry error", ProofRefs: []string{"diff: internal/a.go:7"},
		}},
		Evidence: []string{"inspection: internal/a.go:7 and internal/b.go:1", "test: go test ./internal/reviewtransaction"},
	}
	request := ArtifactAdmissionRequest{
		ExpectedSubject:   subject,
		FrozenContext:     context,
		EchoedSubjectHash: subject.SubjectHash,
		Inspection:        ArtifactInspection{Status: ArtifactInspectionCompleted, Paths: []string{"internal/a.go", "internal/b.go"}},
		Result:            result,
		RawPayload:        []byte("review complete\n{\"subject_hash\":\"" + subject.SubjectHash + "\"}"),
		CanonicalPayload:  []byte("{\"findings\":[],\"evidence\":[\"inspection\"]}\n"),
	}
	return subject, context, request
}

func TestExtractBoundedSingleJSONObject(t *testing.T) {
	payload := []byte("I inspected the frozen candidate.\n{\"findings\":[],\"evidence\":[\"brace } inside string\"]}\nDone.")
	extracted, decision, err := ExtractBoundedSingleJSONObject(payload, 4096)
	if err != nil || decision != ArtifactAdmissionCompleted {
		t.Fatalf("ExtractBoundedSingleJSONObject() = %q, %q, %v", extracted, decision, err)
	}
	if got := string(extracted); got != `{"findings":[],"evidence":["brace } inside string"]}` {
		t.Fatalf("extracted = %q", got)
	}

	_, decision, err = ExtractBoundedSingleJSONObject([]byte(`before {"findings":[]} between {"evidence":[]} after`), 4096)
	if err == nil || decision != ArtifactAdmissionAmbiguous {
		t.Fatalf("multiple objects = %q, %v; want ambiguous", decision, err)
	}
	_, decision, err = ExtractBoundedSingleJSONObject([]byte("no JSON here"), 4096)
	if err == nil || decision != ArtifactAdmissionIncomplete {
		t.Fatalf("missing object = %q, %v; want incomplete", decision, err)
	}
}

func TestAdmitArtifactRequiresCompletedBoundInScopeInspection(t *testing.T) {
	_, _, request := admittedArtifactFixture(t)
	canonical, admission, err := AdmitArtifact(request)
	if err != nil {
		t.Fatalf("AdmitArtifact() error = %v", err)
	}
	if admission.Decision != ArtifactAdmissionCompleted || admission.RawSHA256 == "" ||
		admission.CanonicalSHA256 == "" || admission.ResultHash != canonical.ResultHash {
		t.Fatalf("admission = %#v, canonical = %#v", admission, canonical)
	}
	if err := admission.Validate(request.ExpectedSubject); err != nil {
		t.Fatalf("admission.Validate() error = %v", err)
	}

	tests := []struct {
		name     string
		mutate   func(*ArtifactAdmissionRequest)
		decision ArtifactAdmissionDecision
	}{
		{name: "legacy subject omitted", mutate: func(r *ArtifactAdmissionRequest) { r.EchoedSubjectHash = "" }, decision: ArtifactAdmissionIncomplete},
		{name: "binding mismatch", mutate: func(r *ArtifactAdmissionRequest) { r.EchoedSubjectHash = "sha256:" + strings.Repeat("9", 64) }, decision: ArtifactAdmissionBindingMismatch},
		{name: "inspection unavailable", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Findings = []Finding{}
			r.Result.Evidence = []string{"Inspection blocked: read access denied; no candidate contents were available."}
		}, decision: ArtifactAdmissionIncomplete},
		{name: "partial inspection", mutate: func(r *ArtifactAdmissionRequest) { r.Inspection.Paths = []string{"internal/a.go"} }, decision: ArtifactAdmissionIncomplete},
		{name: "repository path manifest missing", mutate: func(r *ArtifactAdmissionRequest) {
			r.FrozenContext.repositoryPaths = nil
		}, decision: ArtifactAdmissionBindingMismatch},
		{name: "repository path manifest non-canonical", mutate: func(r *ArtifactAdmissionRequest) {
			r.FrozenContext.repositoryPaths = append([]string{"./not-canonical.go"}, r.FrozenContext.repositoryPaths...)
		}, decision: ArtifactAdmissionBindingMismatch},
		{name: "changed path absent from repository manifest", mutate: func(r *ArtifactAdmissionRequest) {
			r.FrozenContext.repositoryPaths = []string{"internal/a.go"}
		}, decision: ArtifactAdmissionBindingMismatch},
		{name: "out of scope finding", mutate: func(r *ArtifactAdmissionRequest) { r.Result.Findings[0].Location = "unrelated/old.go:3" }, decision: ArtifactAdmissionOutOfScope},
		{name: "out of scope proof", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Findings[0].ProofRefs = []string{"diff: unrelated/old.go:3"}
		}, decision: ArtifactAdmissionOutOfScope},
		{name: "root path evidence outside scope", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Evidence = append(r.Result.Evidence, "diff: secret.go:42")
		}, decision: ArtifactAdmissionOutOfScope},
		{name: "root path proof outside scope", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Findings[0].ProofRefs = []string{"diff: secret.go:42"}
		}, decision: ArtifactAdmissionOutOfScope},
		{name: "non ASCII finding id", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Findings[0].ID = "R3-é"
		}, decision: ArtifactAdmissionBindingMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, candidate := admittedArtifactFixture(t)
			tc.mutate(&candidate)
			_, admission, err := AdmitArtifact(candidate)
			if err == nil || admission.Decision != tc.decision {
				t.Fatalf("AdmitArtifact() decision = %q, error = %v; want %q", admission.Decision, err, tc.decision)
			}
		})
	}
}

// admittedCandidateCausalArtifactFixture builds a request whose single
// finding is severe and candidate-causal, so AdmitArtifact populates
// wantCandidateCausalIDs and the request.CandidateCausalFindingIDs
// comparison in AdmitArtifact actually exercises the 1699 canonicalization
// seam instead of comparing two empty slices.
func admittedCandidateCausalArtifactFixture(t *testing.T) ArtifactAdmissionRequest {
	t.Helper()
	_, _, request := admittedArtifactFixture(t)
	request.Result.Findings[0].Severity = "BLOCKER"
	request.Result.Findings[0].EvidenceClass = EvidenceDeterministic
	request.Result.Findings[0].CausalDisposition = CausalIntroduced
	return request
}

// TestArtifactAdmissionCandidateCausalCanonicalization is the RED-first proof
// for 1699: the predicate used to compare canonicalized verifiedIDs against
// the raw submitted CandidateCausalFindingIDs, so a semantically identical but
// differently formatted submission was misclassified out_of_scope.
func TestArtifactAdmissionCandidateCausalCanonicalization(t *testing.T) {
	t.Run("non-canonical submitted order still admits", func(t *testing.T) {
		request := admittedCandidateCausalArtifactFixture(t)
		request.CandidateCausalFindingIDs = []string{" R3-001 "}
		_, admission, err := AdmitArtifact(request)
		if err != nil || admission.Decision != ArtifactAdmissionCompleted {
			t.Fatalf("AdmitArtifact() = %q, %v; want completed", admission.Decision, err)
		}
		if len(admission.CandidateCausalFindingIDs) != 1 || admission.CandidateCausalFindingIDs[0] != "R3-001" {
			t.Fatalf("admission.CandidateCausalFindingIDs = %v, want canonical [R3-001]", admission.CandidateCausalFindingIDs)
		}
	})
	t.Run("canonicalization error becomes incomplete and names the offending id", func(t *testing.T) {
		request := admittedCandidateCausalArtifactFixture(t)
		request.CandidateCausalFindingIDs = []string{"R3-001", "R3-001"}
		_, admission, err := AdmitArtifact(request)
		if err == nil || admission.Decision != ArtifactAdmissionIncomplete {
			t.Fatalf("AdmitArtifact() = %q, %v; want incomplete", admission.Decision, err)
		}
		if !strings.Contains(admission.Diagnostic, "R3-001") {
			t.Fatalf("admission.Diagnostic = %q, want the offending id named", admission.Diagnostic)
		}
	})
	t.Run("real set mismatch stays out of scope byte-identical", func(t *testing.T) {
		request := admittedCandidateCausalArtifactFixture(t)
		request.CandidateCausalFindingIDs = []string{"R3-999"}
		_, admission, err := AdmitArtifact(request)
		wantMessage := "candidate-causal findings are not proven by repository-derived changed-line evidence"
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope || admission.Diagnostic != wantMessage {
			t.Fatalf("AdmitArtifact() = %q, %q, %v; want out-of-scope %q", admission.Decision, admission.Diagnostic, err, wantMessage)
		}
	})
}

// TestAdmitArtifactOmittedSubjectDiagnosticNamesContinuation pins the
// discoverability contract for the community-reported dead end (PR #1801): a
// rejected admission never consumes the lens slot, so the incomplete
// diagnostic must tell the operator that the lens can be re-run and captured
// again with the required top-level subject_hash and inspection envelope. The
// machine-readable decision stays "incomplete"; only the prose is extended.
func TestAdmitArtifactOmittedSubjectDiagnosticNamesContinuation(t *testing.T) {
	_, _, request := admittedArtifactFixture(t)
	request.EchoedSubjectHash = ""
	_, admission, err := AdmitArtifact(request)
	if err == nil || admission.Decision != ArtifactAdmissionIncomplete {
		t.Fatalf("AdmitArtifact() decision = %q, error = %v; want incomplete", admission.Decision, err)
	}
	for _, want := range []string{
		"omitted the provider-owned artifact subject",
		"subject_hash",
		"inspection",
		"re-run",
	} {
		if !strings.Contains(admission.Diagnostic, want) {
			t.Fatalf("incomplete diagnostic %q does not name %q", admission.Diagnostic, want)
		}
	}
}

func TestReferenceOutsideScopeRecognizesOnlyStructuredRepositoryPaths(t *testing.T) {
	allowed := map[string]struct{}{
		"docs/naïve guide.md": {},
		"internal/a.go":       {},
		"main.go":             {},
		"Makefile":            {},
	}
	repository := map[string]struct{}{}
	for _, logicalPath := range []string{
		"Dockerfile", "Makefile", "docs/naïve guide.md", "docs/秘密 guide.md", "internal/a.go",
		"internal/secret.go", "main.go", "secret.go", "sha256", "status",
	} {
		repository[logicalPath] = struct{}{}
	}
	tests := []struct {
		name    string
		value   string
		outside bool
	}{
		{name: "root path in scope", value: "main.go:42"},
		{name: "root path outside scope", value: "secret.go:42", outside: true},
		{name: "nested path in scope", value: "diff: internal/a.go:7"},
		{name: "nested path outside scope", value: "diff: internal/secret.go:7", outside: true},
		{name: "quoted unicode and spaces in scope", value: "reviewed \"docs/naïve guide.md:9\""},
		{name: "quoted unicode and spaces outside scope", value: "reviewed \"docs/秘密 guide.md:9\"", outside: true},
		{name: "quoted extensionless root in scope", value: "reviewed `Makefile:12`"},
		{name: "quoted extensionless root outside scope", value: "reviewed `Dockerfile:12`", outside: true},
		{name: "sha256 digest", value: "sha256:1234567890123456789012345678901234567890123456789012345678901234"},
		{name: "timestamp", value: "2026-07-22T12:34:56Z"},
		{name: "timestamp without zone", value: "2026-07-22T12:34:56"},
		{name: "numeric status", value: "status:500"},
		{name: "filename token absent from repository", value: "not-in-repository.go:42"},
		{name: "https URL", value: "https://example.test/internal/secret.go:42"},
		{name: "URL port", value: "http://127.0.0.1:8080/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := referenceOutsideScope(tt.value, allowed, repository); got != tt.outside {
				t.Fatalf("referenceOutsideScope(%q) = %v, want %v", tt.value, got, tt.outside)
			}
		})
	}
}
