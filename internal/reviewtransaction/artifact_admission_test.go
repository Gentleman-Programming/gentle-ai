package reviewtransaction

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func admittedArtifactFixture(t *testing.T) (ArtifactSubject, FrozenCandidateContext, ArtifactAdmissionRequest) {
	t.Helper()
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "internal/a.go", "package internal\n\nconst value = 1\n")
	writeSnapshotFile(t, repo, "internal/secret.go", "package internal\n")
	writeSnapshotFile(t, repo, "secret.go", "package secret\n")
	gitSnapshot(t, repo, "add", "-A", "--")
	gitSnapshot(t, repo, "commit", "-m", "add admission fixtures")
	writeSnapshotFile(t, repo, "internal/a.go", "package internal\n\nconst value = 2\n")
	writeSnapshotFile(t, repo, "internal/b.go", "package internal\n")
	gitSnapshot(t, repo, "add", "-A", "--")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{Kind: TargetExactRevision, Revision: strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))})
	if err != nil {
		t.Fatal(err)
	}
	context, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state := CompactState{LineageID: "review-artifact-subject", SelectedLenses: []string{LensReliability, LensReadability}, InitialSnapshot: snapshot}
	subject, err := NewArtifactSubject(state, "sha256:"+strings.Repeat("2", 64), context, LensReliability, 0, "")
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
	canonical, admission, err := AdmitArtifact(t.Context(), request)
	if err != nil {
		t.Fatalf("AdmitArtifact() error = %v", err)
	}
	if admission.Decision != ArtifactAdmissionCompleted || admission.RawSHA256 == "" ||
		admission.CanonicalSHA256 == "" || admission.ResultHash != canonical.ResultHash {
		t.Fatalf("admission = %#v, canonical = %#v", admission, canonical)
	}
	if err := admission.Validate(request.ExpectedSubject, request.CanonicalPayload); err != nil {
		t.Fatalf("admission.Validate() error = %v", err)
	}

	tests := []struct {
		name     string
		mutate   func(*ArtifactAdmissionRequest)
		decision ArtifactAdmissionDecision
	}{
		{name: "legacy subject omitted", mutate: func(r *ArtifactAdmissionRequest) { r.EchoedSubjectHash = "" }, decision: ArtifactAdmissionIncomplete},
		{name: "binding mismatch", mutate: func(r *ArtifactAdmissionRequest) { r.EchoedSubjectHash = "sha256:" + strings.Repeat("9", 64) }, decision: ArtifactAdmissionBindingMismatch},
		{name: "inspection status unavailable", mutate: func(r *ArtifactAdmissionRequest) {
			r.Inspection.Status = "unavailable"
		}, decision: ArtifactAdmissionIncomplete},
		{name: "partial inspection", mutate: func(r *ArtifactAdmissionRequest) { r.Inspection.Paths = []string{"internal/a.go"} }, decision: ArtifactAdmissionIncomplete},
		{name: "repository authority missing", mutate: func(r *ArtifactAdmissionRequest) {
			r.FrozenContext.repositoryRoot = ""
		}, decision: ArtifactAdmissionBindingMismatch},
		{name: "out of scope finding", mutate: func(r *ArtifactAdmissionRequest) { r.Result.Findings[0].Location = "unrelated/old.go:3" }, decision: ArtifactAdmissionOutOfScope},
		{name: "unknown repository proof", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Findings[0].ProofRefs = []string{"diff: missing/old.go:3"}
		}, decision: ArtifactAdmissionOutOfScope},
		{name: "unknown repository evidence", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Evidence = append(r.Result.Evidence, "diff: missing.go:42")
		}, decision: ArtifactAdmissionOutOfScope},
		{name: "directory repository evidence does not recurse", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Evidence = append(r.Result.Evidence, "directory: `internal:1`")
		}, decision: ArtifactAdmissionOutOfScope},
		{name: "non-canonical repository proof", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Findings[0].ProofRefs = []string{"diff: ./secret.go:42"}
		}, decision: ArtifactAdmissionOutOfScope},
		{name: "non ASCII finding id", mutate: func(r *ArtifactAdmissionRequest) {
			r.Result.Findings[0].ID = "R3-é"
		}, decision: ArtifactAdmissionBindingMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, candidate := admittedArtifactFixture(t)
			tc.mutate(&candidate)
			_, admission, err := AdmitArtifact(t.Context(), candidate)
			if err == nil || admission.Decision != tc.decision {
				t.Fatalf("AdmitArtifact() decision = %q, error = %v; want %q", admission.Decision, err, tc.decision)
			}
		})
	}
}

// TestAdmitArtifactIgnoresFreeTextInspectionClaims is issue #4256: admission
// must decide completeness primarily from the typed inspection.status field,
// not by keyword-scanning the reviewer's free-text evidence for a bare word.
// A rationale that merely contains "unavailable" outside the narrow
// read/access-failure phrase family evidenceReportsUnavailableInspection
// recognizes must not flip a completed inspection to incomplete, and a
// genuinely incomplete typed status must still refuse regardless of what the
// evidence text says. A narrow defense-in-depth backstop still catches
// evidence squarely inside that phrase family even when the typed status
// claims "completed" (finding R4-false-completion-backstop-removed): Status
// is produced by a non-deterministic model, so a reviewer that could not
// read the candidate but leaves Status at "completed" while its evidence
// names a read/access failure must still be caught.
func TestAdmitArtifactIgnoresFreeTextInspectionClaims(t *testing.T) {
	t.Run("unavailable wording outside the phrase family with completed status is admitted", func(t *testing.T) {
		_, _, request := admittedArtifactFixture(t)
		request.Result.Evidence = []string{
			"the fixture was unavailable in the previous release; nothing else changed",
		}
		_, admission, err := AdmitArtifact(t.Context(), request)
		if err != nil || admission.Decision != ArtifactAdmissionCompleted {
			t.Fatalf("AdmitArtifact() decision = %q, error = %v; want %q", admission.Decision, err, ArtifactAdmissionCompleted)
		}
	})

	t.Run("typed unavailable status is incomplete regardless of evidence wording", func(t *testing.T) {
		_, _, request := admittedArtifactFixture(t)
		request.Inspection.Status = "unavailable"
		request.Result.Evidence = []string{"inspection: internal/a.go:7 and internal/b.go:1"}
		_, admission, err := AdmitArtifact(t.Context(), request)
		if err == nil || admission.Decision != ArtifactAdmissionIncomplete {
			t.Fatalf("AdmitArtifact() decision = %q, error = %v; want %q", admission.Decision, err, ArtifactAdmissionIncomplete)
		}
	})

	t.Run("read/access-failure evidence with completed status is refused by the backstop", func(t *testing.T) {
		_, _, request := admittedArtifactFixture(t)
		request.Result.Evidence = []string{"inspection blocked: read " + "access " + "denied for the candidate diff"}
		_, admission, err := AdmitArtifact(t.Context(), request)
		if err == nil || admission.Decision != ArtifactAdmissionIncomplete {
			t.Fatalf("AdmitArtifact() decision = %q, error = %v; want %q", admission.Decision, err, ArtifactAdmissionIncomplete)
		}
		if !strings.Contains(admission.Diagnostic, "could not be "+"inspected") ||
			!strings.Contains(admission.Diagnostic, "inspection.status: \"unavailable\"") ||
			!strings.Contains(admission.Diagnostic, "gentle-ai review capture-result") {
			t.Fatalf("backstop diagnostic = %q", admission.Diagnostic)
		}
	})
}

func TestAdmitArtifactCanonicalizesCompleteInspectionCoverage(t *testing.T) {
	tests := []struct {
		name           string
		paths          []string
		location       string
		decision       ArtifactAdmissionDecision
		wantDiagnostic string
	}{
		{name: "unordered complete manifest and contiguous location", paths: []string{"internal/b.go", "internal/a.go"}, location: "internal/a.go:7-7", decision: ArtifactAdmissionCompleted},
		{name: "duplicate path", paths: []string{"internal/a.go", "internal/a.go", "internal/b.go"}, decision: ArtifactAdmissionOutOfScope},
		{name: "missing frozen path", paths: []string{"internal/a.go"}, decision: ArtifactAdmissionIncomplete, wantDiagnostic: `{"code":"inspection_coverage","reason":"missing_frozen_manifest_paths","missing_path_count":1}`},
		{name: "foreign path", paths: []string{"internal/a.go", "outside/private.go", "internal/b.go"}, decision: ArtifactAdmissionOutOfScope, wantDiagnostic: `{"code":"inspection_coverage","reason":"foreign_inspection_paths","foreign_path_count":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, request := admittedArtifactFixture(t)
			request.Inspection.Paths = tt.paths
			if tt.location != "" {
				request.Result.Findings[0].Location = tt.location
			}
			_, admission, err := AdmitArtifact(t.Context(), request)
			if (err != nil) != (tt.decision != ArtifactAdmissionCompleted) || admission.Decision != tt.decision {
				t.Fatalf("AdmitArtifact() = %q, %v; want %q", admission.Decision, err, tt.decision)
			}
			if tt.wantDiagnostic == "" {
				return
			}
			var admissionErr *ArtifactAdmissionError
			if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil {
				t.Fatalf("AdmitArtifact() error = %v; want structured diagnostic", err)
			}
			got, marshalErr := json.Marshal(admissionErr.Diagnostic)
			if marshalErr != nil || string(got) != tt.wantDiagnostic || strings.Contains(err.Error(), "outside/private.go") {
				t.Fatalf("coverage diagnostic = %s, error = %v", got, err)
			}
		})
	}
}

// legacyAdmittedArtifactFixture rebinds the shared admission fixture onto a
// negotiated review-integration/v1 subject, whose identity is the rendered
// candidate diff rather than the frozen Git trees.
func legacyAdmittedArtifactFixture(t *testing.T) ArtifactAdmissionRequest {
	t.Helper()
	native, frozen, request := admittedArtifactFixture(t)
	state := CompactState{
		LineageID: native.LineageID, SelectedLenses: []string{LensReliability, LensReadability},
		InitialSnapshot: Snapshot{
			Identity: native.TargetIdentity, BaseTree: frozen.BaseTree, CandidateTree: frozen.CandidateTree,
			Paths: manifestPaths(frozen.ChangedPathManifest),
		},
	}
	diff, err := NewFrozenCandidateDiff([]byte("diff --git a/internal/a.go b/internal/a.go\n"))
	if err != nil {
		t.Fatal(err)
	}
	frozen.LegacyCandidateDiff = &diff
	subject, err := NewLegacyArtifactSubject(state, native.AuthorityRevision, frozen, LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedSubject, request.FrozenContext = subject, frozen
	request.EchoedSubjectHash = subject.SubjectHash
	return request
}

// A v1 subject carries no trees by construction, so admission has to bind it
// through the candidate diff digest. Comparing the blanked trees against the
// frozen context rejected every legacy capture before it could consume its lens
// slot, leaving the collect loop to re-offer the same slot forever.
func TestAdmitArtifactBindsLegacySubjectByCandidateDiff(t *testing.T) {
	request := legacyAdmittedArtifactFixture(t)
	canonical, admission, err := AdmitArtifact(t.Context(), request)
	if err != nil {
		t.Fatalf("AdmitArtifact() error = %v (decision %q, diagnostic %q)", err, admission.Decision, admission.Diagnostic)
	}
	if admission.Decision != ArtifactAdmissionCompleted || admission.ResultHash != canonical.ResultHash {
		t.Fatalf("admission = %#v, canonical = %#v", admission, canonical)
	}
	if err := admission.Validate(request.ExpectedSubject, request.CanonicalPayload); err != nil {
		t.Fatalf("admission.Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ArtifactAdmissionRequest)
	}{
		{name: "legacy transport absent", mutate: func(r *ArtifactAdmissionRequest) {
			r.FrozenContext.LegacyCandidateDiff = nil
		}},
		{name: "legacy transport rerendered", mutate: func(r *ArtifactAdmissionRequest) {
			other, err := NewFrozenCandidateDiff([]byte("diff --git a/internal/b.go b/internal/b.go\n"))
			if err != nil {
				t.Fatal(err)
			}
			r.FrozenContext.LegacyCandidateDiff = &other
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := legacyAdmittedArtifactFixture(t)
			tc.mutate(&candidate)
			_, admission, err := AdmitArtifact(t.Context(), candidate)
			if err == nil || admission.Decision != ArtifactAdmissionBindingMismatch {
				t.Fatalf("AdmitArtifact() decision = %q, error = %v; want %q",
					admission.Decision, err, ArtifactAdmissionBindingMismatch)
			}
		})
	}
}

func TestAdmitArtifactAllowsSupportingProofOutsideChangedManifest(t *testing.T) {
	_, _, request := admittedArtifactFixture(t)
	gitSnapshot(t, request.FrozenContext.repositoryRoot, "rm", "--", "secret.go")
	writeSnapshotFile(t, request.FrozenContext.repositoryRoot, "live-only.go", "package liveonly\n")
	request.Result.Evidence = append(request.Result.Evidence, "supporting implementation: internal/secret.go:7")
	request.Result.Findings[0].ProofRefs = []string{"repository proof: secret.go:42"}

	canonical, admission, err := AdmitArtifact(t.Context(), request)
	if err != nil || admission.Decision != ArtifactAdmissionCompleted {
		t.Fatalf("AdmitArtifact() = %q, %v; want completed", admission.Decision, err)
	}
	if stringIndex(canonical.Evidence, "supporting implementation: internal/secret.go:7") < 0 ||
		!equalStrings(canonical.Findings[0].ProofRefs, request.Result.Findings[0].ProofRefs) {
		t.Fatalf("supporting repository proof was not preserved: %#v", canonical)
	}

	request.Inspection.Paths = append(request.Inspection.Paths, "internal/secret.go")
	if _, rejected, err := AdmitArtifact(t.Context(), request); err == nil || rejected.Decision != ArtifactAdmissionOutOfScope {
		t.Fatalf("external proof path became an inspectable candidate path: decision=%q error=%v", rejected.Decision, err)
	}
	request.Inspection.Paths = []string{"internal/a.go", "internal/b.go"}
	request.Result.Evidence = []string{"live-only proof: live-only.go:1"}
	if _, rejected, err := AdmitArtifact(t.Context(), request); err == nil || rejected.Decision != ArtifactAdmissionOutOfScope {
		t.Fatalf("live-worktree-only proof was admitted: decision=%q error=%v", rejected.Decision, err)
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

// admittedTwoFindingCandidateCausalArtifactFixture extends the single-finding
// fixture with a second severe finding that also self-claims candidate
// causality, so a test can verify one and leave the other unverified.
func admittedTwoFindingCandidateCausalArtifactFixture(t *testing.T) ArtifactAdmissionRequest {
	t.Helper()
	request := admittedCandidateCausalArtifactFixture(t)
	request.Result.Findings = append(request.Result.Findings, Finding{
		ID: "R3-002", Lens: "reliability", Location: "internal/b.go:1", Severity: "BLOCKER",
		Claim: "the candidate drops the added helper's bounds check", ProofRefs: []string{"diff: internal/b.go:1"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	})
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
		_, admission, err := AdmitArtifact(t.Context(), request)
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
		_, admission, err := AdmitArtifact(t.Context(), request)
		if err == nil || admission.Decision != ArtifactAdmissionIncomplete {
			t.Fatalf("AdmitArtifact() = %q, %v; want incomplete", admission.Decision, err)
		}
		if !strings.Contains(admission.Diagnostic, "R3-001") {
			t.Fatalf("admission.Diagnostic = %q, want the offending id named", admission.Diagnostic)
		}
	})
	// real_set_mismatch_stays_out_of_scope_byte-identical pinned the OLD
	// whole-artifact hard-reject: any mismatch between wantCandidateCausalIDs
	// and verifiedIDs, in either direction, rejected the entire result. Issues
	// #1757/#2782 required a per-finding downgrade for an unverified
	// self-claim (see the two subtests below), so this scenario now exercises
	// only what still is a structural defect: request.CandidateCausalFindingIDs
	// is only ever computed from findings that already self-claim causality
	// (review_artifact.go's verifiedCandidateCausalFindingIDs), so a verified
	// ID that names no self-claiming finding at all -- "R3-999" is not even
	// the fixture's real finding ID -- can never be a proof failure on a real
	// finding, and is deliberately kept as a whole-artifact rejection with a
	// distinct diagnostic code.
	t.Run("real set mismatch stays out of scope byte-identical", func(t *testing.T) {
		request := admittedCandidateCausalArtifactFixture(t)
		request.CandidateCausalFindingIDs = []string{"R3-999"}
		_, admission, err := AdmitArtifact(t.Context(), request)
		wantMessage := "verified candidate-causal finding IDs are not a subset of the reviewer's self-claimed candidate-causal findings"
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope || admission.Diagnostic != wantMessage {
			t.Fatalf("AdmitArtifact() = %q, %q, %v; want out-of-scope %q", admission.Decision, admission.Diagnostic, err, wantMessage)
		}
		var admissionErr *ArtifactAdmissionError
		if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil {
			t.Fatalf("candidate-causal error = %v; want structured diagnostic", err)
		}
		if admissionErr.Diagnostic.Code != "candidate_causality_unclaimed_id" ||
			admissionErr.Diagnostic.FindingID != "R3-999" ||
			admissionErr.Diagnostic.Location != "" ||
			admissionErr.Diagnostic.Reason != "id_not_self_claimed_by_finding" {
			t.Fatalf("candidate-causal diagnostic = %#v", admissionErr.Diagnostic)
		}
		if len(admission.DowngradedCausalFindings) != 0 {
			t.Fatalf("admission.DowngradedCausalFindings = %#v, want none for a rejected artifact", admission.DowngradedCausalFindings)
		}
	})
	t.Run("one unverified claim among verified ones is downgraded and the artifact admitted", func(t *testing.T) {
		request := admittedTwoFindingCandidateCausalArtifactFixture(t)
		request.CandidateCausalFindingIDs = []string{"R3-001"}
		canonical, admission, err := AdmitArtifact(t.Context(), request)
		if err != nil || admission.Decision != ArtifactAdmissionCompleted {
			t.Fatalf("AdmitArtifact() = %q, %v; want completed", admission.Decision, err)
		}
		if len(admission.CandidateCausalFindingIDs) != 1 || admission.CandidateCausalFindingIDs[0] != "R3-001" {
			t.Fatalf("admission.CandidateCausalFindingIDs = %v, want exactly the verified [R3-001]", admission.CandidateCausalFindingIDs)
		}
		if len(admission.DowngradedCausalFindings) != 1 || admission.DowngradedCausalFindings[0] != (ArtifactAdmissionCausalDowngrade{FindingID: "R3-002", Reason: "unverified_location"}) {
			t.Fatalf("admission.DowngradedCausalFindings = %#v, want exactly R3-002 downgraded as unverified_location", admission.DowngradedCausalFindings)
		}
		// The persisted/returned canonical result -- not only the side-channel
		// diagnostic -- must carry the downgrade: the verified finding keeps
		// its self-claimed disposition, and only the unverified one flips to
		// CausalUnknown.
		gotByID := map[string]CausalDisposition{}
		for _, finding := range canonical.Findings {
			gotByID[finding.ID] = finding.CausalDisposition
		}
		if gotByID["R3-001"] != CausalIntroduced || gotByID["R3-002"] != CausalUnknown {
			t.Fatalf("canonical.Findings dispositions = %#v, want R3-001 unchanged and R3-002 downgraded", gotByID)
		}
		if canonical.ResultHash != admission.ResultHash {
			t.Fatalf("canonical.ResultHash = %q, admission.ResultHash = %q; want them to match the same downgraded content", canonical.ResultHash, admission.ResultHash)
		}
	})
	t.Run("all claims unverified admits with all downgraded", func(t *testing.T) {
		request := admittedTwoFindingCandidateCausalArtifactFixture(t)
		request.CandidateCausalFindingIDs = nil
		canonical, admission, err := AdmitArtifact(t.Context(), request)
		if err != nil || admission.Decision != ArtifactAdmissionCompleted {
			t.Fatalf("AdmitArtifact() = %q, %v; want completed", admission.Decision, err)
		}
		if len(admission.CandidateCausalFindingIDs) != 0 {
			t.Fatalf("admission.CandidateCausalFindingIDs = %v, want none verified", admission.CandidateCausalFindingIDs)
		}
		want := []ArtifactAdmissionCausalDowngrade{
			{FindingID: "R3-001", Reason: "unverified_location"},
			{FindingID: "R3-002", Reason: "unverified_location"},
		}
		if len(admission.DowngradedCausalFindings) != len(want) || admission.DowngradedCausalFindings[0] != want[0] || admission.DowngradedCausalFindings[1] != want[1] {
			t.Fatalf("admission.DowngradedCausalFindings = %#v, want both findings downgraded and no blockers", admission.DowngradedCausalFindings)
		}
		// A fully-degraded admission (every self-claimed candidate-causal
		// finding unverified) must still admit -- decision Completed, zero
		// verified candidate-causal IDs, and every returned finding's
		// disposition actually rewritten to CausalUnknown, not merely named in
		// the side-channel diagnostic.
		if len(canonical.Findings) != 2 {
			t.Fatalf("canonical.Findings = %#v, want both fixture findings", canonical.Findings)
		}
		for _, finding := range canonical.Findings {
			if finding.CausalDisposition != CausalUnknown {
				t.Fatalf("canonical.Findings[%s].CausalDisposition = %q, want CausalUnknown for a fully-degraded admission", finding.ID, finding.CausalDisposition)
			}
		}
	})
}

// TestValidateCorrectionPlanRequestRejectsDowngradedCausalUnknownFixTarget
// proves the correction-plan validator itself refuses a fix target whose
// disposition is CausalUnknown -- the exact disposition AdmitArtifact now
// rewrites an unverified self-claim to -- independent of how that request was
// assembled upstream. A control request with a real candidate-causal
// disposition is accepted to prove the rejection is about the disposition,
// not the rest of the shape.
func TestValidateCorrectionPlanRequestRejectsDowngradedCausalUnknownFixTarget(t *testing.T) {
	buildRequest := func(disposition CausalDisposition) CorrectionPlanRequest {
		finding := CorrectionPlanFinding{
			ID: "R3-002", Lens: "reliability", Location: "internal/b.go:1", Severity: "BLOCKER",
			Claim: "the candidate drops the added helper's bounds check", ProofRefs: []string{"diff: internal/b.go:1"},
			Evidence: "diff: internal/b.go:1", EvidenceClass: EvidenceDeterministic, CausalDisposition: disposition,
		}
		request := CorrectionPlanRequest{
			Schema: CorrectionPlanRequestSchema, LineageID: "correction-plan-request-fixture",
			ExpectedRevision: "sha256:" + strings.Repeat("1", 64), TargetIdentity: "sha256:" + strings.Repeat("2", 64),
			CorrectionBudget: 1, FixFindingIDs: []string{finding.ID}, Findings: []CorrectionPlanFinding{finding},
		}
		request.RequestHash = correctionPlanRequestHash(request)
		return request
	}
	if err := ValidateCorrectionPlanRequest(buildRequest(CausalIntroduced)); err != nil {
		t.Fatalf("control request with a real candidate-causal disposition was rejected: %v", err)
	}
	if err := ValidateCorrectionPlanRequest(buildRequest(CausalUnknown)); err == nil {
		t.Fatal("a finding downgraded to CausalUnknown must not be usable as a correction plan fix target")
	}
}

// TestAdmitArtifactRefusesToDowngradeOnDegradedEvidenceDerivation is the
// fail-open proof: an unverified self-claimed finding is only a safe
// candidate for CausalUnknown downgrade when the caller's changed-line
// derivation was complete for it. If that derivation degraded (binary
// content, an untraceable rename, a large candidate, a manifest-less
// capture -- anything CandidateLocationSupportsCausality could not resolve),
// "unverified" cannot be distinguished from "no signal was derived", and
// downgrading would silently neutralize a real blocker. AdmitArtifact must
// keep the whole-artifact rejection in that case instead.
func TestAdmitArtifactRefusesToDowngradeOnDegradedEvidenceDerivation(t *testing.T) {
	request := admittedTwoFindingCandidateCausalArtifactFixture(t)
	request.CandidateCausalFindingIDs = []string{"R3-001"}
	request.EvidenceDerivation = EvidenceDerivationDegraded
	request.EvidenceDerivationReason = "repository-derived changed-line evidence for finding \"R3-002\" could not be resolved (binary content or an untraceable path change)"

	_, admission, err := AdmitArtifact(t.Context(), request)
	if err == nil || admission.Decision != ArtifactAdmissionOutOfScope {
		t.Fatalf("AdmitArtifact() = %q, %v; want out-of-scope rejection when evidence derivation is degraded", admission.Decision, err)
	}
	if len(admission.DowngradedCausalFindings) != 0 {
		t.Fatalf("admission.DowngradedCausalFindings = %#v, want none -- a degraded derivation must never downgrade", admission.DowngradedCausalFindings)
	}
	if !strings.Contains(admission.Diagnostic, request.EvidenceDerivationReason) {
		t.Fatalf("admission.Diagnostic = %q, want the evidence-derivation reason named", admission.Diagnostic)
	}
	if !strings.Contains(admission.Diagnostic, "gentle-ai review capture-result") {
		t.Fatalf("admission.Diagnostic = %q, want a gentle-ai capture-result continuation named", admission.Diagnostic)
	}
	var admissionErr *ArtifactAdmissionError
	if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil ||
		admissionErr.Diagnostic.Code != "candidate_causality_evidence_degraded" ||
		admissionErr.Diagnostic.Reason != "evidence_derivation_degraded" {
		t.Fatalf("candidate-causal evidence-degraded diagnostic = %#v", admissionErr.Diagnostic)
	}

	// Control: the identical request with EvidenceDerivationComplete (the
	// default) still downgrades and admits, proving the rejection above is
	// caused by the degraded signal, not by anything else in the fixture.
	completeRequest := admittedTwoFindingCandidateCausalArtifactFixture(t)
	completeRequest.CandidateCausalFindingIDs = []string{"R3-001"}
	completeRequest.EvidenceDerivation = EvidenceDerivationComplete
	_, completeAdmission, err := AdmitArtifact(t.Context(), completeRequest)
	if err != nil || completeAdmission.Decision != ArtifactAdmissionCompleted || len(completeAdmission.DowngradedCausalFindings) != 1 {
		t.Fatalf("control AdmitArtifact() = %q, %v, downgraded=%#v; want completed with one downgrade", completeAdmission.Decision, err, completeAdmission.DowngradedCausalFindings)
	}
}

func TestAdmitArtifactReturnsStructuredInvalidLocationDiagnostic(t *testing.T) {
	request := admittedCandidateCausalArtifactFixture(t)
	request.Result.Findings[0].Location = "internal/a.go:7-9,10"

	_, admission, err := AdmitArtifact(t.Context(), request)
	var admissionErr *ArtifactAdmissionError
	var locationErr *FindingLocationError
	if admission.Decision != ArtifactAdmissionOutOfScope ||
		!errors.As(err, &admissionErr) || !errors.As(err, &locationErr) {
		t.Fatalf("AdmitArtifact() = %#v, %v; want typed location refusal", admission, err)
	}
	if admissionErr.Diagnostic == nil ||
		admissionErr.Diagnostic.Code != "invalid_finding_location" ||
		admissionErr.Diagnostic.FindingID != "R3-001" ||
		admissionErr.Diagnostic.Location != "internal/a.go:7-9,10" ||
		admissionErr.Diagnostic.Reason != "line_suffix_not_integer" {
		t.Fatalf("structured diagnostic = %#v", admissionErr.Diagnostic)
	}

	for _, unsafe := range []string{
		"internal:../private/a.go:7-9", "internal/a.go:7:/home/private/repo.go",
		"internal/a.go:7:https://private.example/repo", `internal/a.go:7:C:\Users\private\repo.go`,
	} {
		request.Result.Findings[0].Location = unsafe
		_, _, err = AdmitArtifact(t.Context(), request)
		if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil ||
			admissionErr.Diagnostic.Location != "" || strings.Contains(err.Error(), unsafe) {
			t.Fatalf("unsafe location escaped structured diagnostic: %#v, %v", admissionErr.Diagnostic, err)
		}
	}

	err = NewArtifactLocationAdmissionError("R3-001", "internal/a.go:bad", errors.New("untyped cause"))
	if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil || admissionErr.Diagnostic.Reason != "invalid_location" || admissionErr.Diagnostic.Location != "" {
		t.Fatalf("untyped location cause was not handled safely: %v", err)
	}
}

func TestFindingAdmissionDiagnosticRequiresCompatibleLocation(t *testing.T) {
	tests := []struct {
		name, code, location, reason string
		wantLocation                 bool
	}{
		{"invalid comma list", "invalid_finding_location", "internal/a.go:7-9,10", "line_suffix_not_integer", true},
		{"candidate line", "candidate_causality_unproven", "internal/a.go:7", "line_not_changed_by_candidate", true},
		{"candidate range", "candidate_causality_unproven", "internal/a.go:7-9", "line_not_changed_by_candidate", true},
		{"invalid valid line", "invalid_finding_location", "internal/a.go:7", "line_suffix_not_integer", false},
		{"invalid reason mismatch", "invalid_finding_location", "internal/a.go:7-9", "line_must_be_positive", false},
		{"evidence unknown path", "evidence_path_out_of_scope", "bogus.go:7", "unknown_or_malformed_repository_path", true},
		{"proof unknown path", "proof_path_out_of_scope", "bogus.go:7", "unknown_or_malformed_repository_path", true},
		{"evidence reason mismatch", "evidence_path_out_of_scope", "bogus.go:7", "line_not_changed_by_candidate", false},
		{"proof malformed shape", "proof_path_out_of_scope", "/home/private/leak.go:5", "unknown_or_malformed_repository_path", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := findingAdmissionDiagnostic(tt.code, "R3-001", tt.location, tt.reason)
			if (diagnostic.Location != "") != tt.wantLocation {
				t.Fatalf("findingAdmissionDiagnostic() = %#v", diagnostic)
			}
		})
	}
}

// TestAdmitArtifactNamesOutOfScopeCitationToken is the RED-first proof for the
// undiagnosable out_of_scope rejection: a free-text evidence or proof_refs
// token shaped like path:line that names an unknown or malformed repository
// path rejected the entire reviewer result without naming the offending
// token, so the incident could not be diagnosed after the fact.
func TestAdmitArtifactNamesOutOfScopeCitationToken(t *testing.T) {
	t.Run("evidence rejection names the offending token", func(t *testing.T) {
		_, _, request := admittedArtifactFixture(t)
		request.Result.Evidence = append(request.Result.Evidence, "helper moved: bogus.go:7")
		_, admission, err := AdmitArtifact(t.Context(), request)
		wantMessage := "reviewer evidence references a path outside the frozen repository"
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope || admission.Diagnostic != wantMessage {
			t.Fatalf("AdmitArtifact() = %q, %q, %v; want out-of-scope %q", admission.Decision, admission.Diagnostic, err, wantMessage)
		}
		var admissionErr *ArtifactAdmissionError
		if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil {
			t.Fatalf("evidence rejection carries no structured diagnostic: %v", err)
		}
		if admissionErr.Diagnostic.Code != "evidence_path_out_of_scope" ||
			admissionErr.Diagnostic.FindingID != "" ||
			admissionErr.Diagnostic.Location != "bogus.go:7" ||
			admissionErr.Diagnostic.Reason != "unknown_or_malformed_repository_path" {
			t.Fatalf("evidence diagnostic = %#v", admissionErr.Diagnostic)
		}
		if !strings.Contains(err.Error(), "bogus.go:7") {
			t.Fatalf("error surface does not name the offending token: %v", err)
		}
	})
	t.Run("proof rejection names the offending token and finding", func(t *testing.T) {
		_, _, request := admittedArtifactFixture(t)
		request.Result.Findings[0].ProofRefs = []string{"diff: bogus.go:7"}
		_, admission, err := AdmitArtifact(t.Context(), request)
		wantMessage := "reviewer proof references a path outside the frozen repository"
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope || admission.Diagnostic != wantMessage {
			t.Fatalf("AdmitArtifact() = %q, %q, %v; want out-of-scope %q", admission.Decision, admission.Diagnostic, err, wantMessage)
		}
		var admissionErr *ArtifactAdmissionError
		if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil {
			t.Fatalf("proof rejection carries no structured diagnostic: %v", err)
		}
		if admissionErr.Diagnostic.Code != "proof_path_out_of_scope" ||
			admissionErr.Diagnostic.FindingID != "R3-001" ||
			admissionErr.Diagnostic.Location != "bogus.go:7" ||
			admissionErr.Diagnostic.Reason != "unknown_or_malformed_repository_path" {
			t.Fatalf("proof diagnostic = %#v", admissionErr.Diagnostic)
		}
		if !strings.Contains(err.Error(), "bogus.go:7") {
			t.Fatalf("error surface does not name the offending token: %v", err)
		}
	})
	t.Run("unsafe offending token stays out of the error surface", func(t *testing.T) {
		_, _, request := admittedArtifactFixture(t)
		request.Result.Evidence = append(request.Result.Evidence, "read /home/private/leak.go:5")
		_, _, err := AdmitArtifact(t.Context(), request)
		var admissionErr *ArtifactAdmissionError
		if !errors.As(err, &admissionErr) || admissionErr.Diagnostic == nil ||
			admissionErr.Diagnostic.Code != "evidence_path_out_of_scope" ||
			admissionErr.Diagnostic.Location != "" || strings.Contains(err.Error(), "/home/private/leak.go") {
			t.Fatalf("unsafe token escaped the structured diagnostic: %#v, %v", admissionErr.Diagnostic, err)
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
	_, admission, err := AdmitArtifact(t.Context(), request)
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

func TestReferenceOutsideRepositoryRecognizesOnlyCanonicalRepositoryPaths(t *testing.T) {
	repository := map[string]struct{}{}
	for _, logicalPath := range []string{
		"Dockerfile", "Makefile", "docs/naïve guide.md", "docs/秘密 guide.md", "internal/a.go",
		"internal/secret.go", "main.go", "secret.go", "sha256", "status",
	} {
		repository[logicalPath] = struct{}{}
	}
	lookup := func(logicalPath string) (bool, error) {
		_, known := repository[logicalPath]
		return known, nil
	}
	tests := []struct {
		name    string
		value   string
		outside bool
	}{
		{name: "root path in scope", value: "main.go:42"},
		{name: "root supporting path", value: "secret.go:42"},
		{name: "nested path in scope", value: "diff: internal/a.go:7"},
		{name: "nested supporting path", value: "diff: internal/secret.go:7"},
		{name: "quoted unicode and spaces in scope", value: "reviewed \"docs/naïve guide.md:9\""},
		{name: "quoted unicode and spaces supporting", value: "reviewed \"docs/秘密 guide.md:9\""},
		{name: "quoted extensionless root in scope", value: "reviewed `Makefile:12`"},
		{name: "quoted extensionless root supporting", value: "reviewed `Dockerfile:12`"},
		{name: "sha256 digest", value: "sha256:1234567890123456789012345678901234567890123456789012345678901234"},
		{name: "timestamp", value: "2026-07-22T12:34:56Z"},
		{name: "timestamp without zone", value: "2026-07-22T12:34:56"},
		{name: "numeric status", value: "status:500"},
		{name: "filename token absent from repository", value: "not-in-repository.go:42", outside: true},
		{name: "absolute path", value: "/tmp/secret.go:42", outside: true},
		{name: "traversal path", value: "../secret.go:42", outside: true},
		{name: "non-canonical path", value: "./secret.go:42", outside: true},
		{name: "https URL", value: "https://example.test/internal/secret.go:42"},
		{name: "URL port", value: "http://127.0.0.1:8080/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No basename resolution in this unit: it pins the literal token
			// grammar on its own, so a citation that misses stays outside.
			noBasename := func(string) (string, bool, error) { return "", false, nil }
			got, _, err := referenceOutsideRepository(tt.value, lookup, noBasename)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.outside {
				t.Fatalf("referenceOutsideRepository(%q) = %v, want %v", tt.value, got, tt.outside)
			}
		})
	}
}

// TestAdmitArtifactResolvesUnambiguousBasenameCitations covers the citation
// shape a real reviewer produces (#3042): it names a file of the candidate by
// its basename instead of its repository-relative path. The path IS in the
// frozen candidate, so refusing it as "outside the frozen repository" is both
// wrong and unrecoverable -- the reviewer is Go-owned, so no caller can correct
// the citation, and every retry reproduces it.
//
// Ambiguity is the line: one candidate path may answer a basename, two may not,
// because guessing which file a reviewer meant is how a finding gets attached
// to code it never read.
func TestAdmitArtifactResolvesUnambiguousBasenameCitations(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "internal/a.go", "package internal\n\nconst value = 1\n")
	writeSnapshotFile(t, repo, "internal/deep/dup.go", "package deep\n")
	writeSnapshotFile(t, repo, "other/dup.go", "package other\n")
	gitSnapshot(t, repo, "add", "-A", "--")
	gitSnapshot(t, repo, "commit", "-m", "base")
	// Both dup.go files change too, so the ambiguity is real inside the frozen
	// manifest rather than an accident of one of them being unchanged.
	writeSnapshotFile(t, repo, "internal/a.go", "package internal\n\nconst value = 2\n")
	writeSnapshotFile(t, repo, "internal/deep/dup.go", "package deep\n\nconst v = 1\n")
	writeSnapshotFile(t, repo, "other/dup.go", "package other\n\nconst v = 1\n")
	gitSnapshot(t, repo, "add", "-A", "--")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{Kind: TargetExactRevision, Revision: strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state := CompactState{LineageID: "review-basename-citation", SelectedLenses: []string{LensReliability}, InitialSnapshot: snapshot}
	subject, err := NewArtifactSubject(state, "sha256:"+strings.Repeat("3", 64), frozen, LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	newRequest := func(evidence string) ArtifactAdmissionRequest {
		return ArtifactAdmissionRequest{
			ExpectedSubject:   subject,
			FrozenContext:     frozen,
			EchoedSubjectHash: subject.SubjectHash,
			Inspection:        ArtifactInspection{Status: ArtifactInspectionCompleted, Paths: []string{"internal/a.go", "internal/deep/dup.go", "other/dup.go"}},
			Result: LensResult{
				Lens:     LensReliability,
				Findings: []Finding{},
				Evidence: []string{evidence},
			},
			RawPayload:       []byte("review complete\n{\"subject_hash\":\"" + subject.SubjectHash + "\"}"),
			CanonicalPayload: []byte("{\"findings\":[],\"evidence\":[\"inspection\"]}\n"),
		}
	}

	t.Run("unambiguous basename is the candidate path it names", func(t *testing.T) {
		_, admission, err := AdmitArtifact(t.Context(), newRequest("inspection: a.go:3"))
		if err != nil || admission.Decision != ArtifactAdmissionCompleted {
			t.Fatalf("basename citation admission = %q, %v; want completed", admission.Decision, err)
		}
	})
	t.Run("ambiguous basename stays out of scope", func(t *testing.T) {
		_, admission, err := AdmitArtifact(t.Context(), newRequest("inspection: dup.go:1"))
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope {
			t.Fatalf("ambiguous basename admission = %q, %v; want out of scope", admission.Decision, err)
		}
	})
	t.Run("a finding location resolves the same way its evidence does", func(t *testing.T) {
		request := newRequest("inspection: internal/a.go:3")
		request.Result.Findings = []Finding{{
			ID: "R3-001", Lens: "reliability", Location: "a.go:3", Severity: "WARNING",
			Claim: "the candidate loses the retry error", ProofRefs: []string{"diff: a.go:3"},
		}}
		_, admission, err := AdmitArtifact(t.Context(), request)
		if err != nil || admission.Decision != ArtifactAdmissionCompleted {
			t.Fatalf("basename finding location admission = %q, %v; want completed", admission.Decision, err)
		}
	})
	t.Run("an ambiguous finding location stays out of scope", func(t *testing.T) {
		request := newRequest("inspection: internal/a.go:3")
		request.Result.Findings = []Finding{{
			ID: "R3-001", Lens: "reliability", Location: "dup.go:1", Severity: "WARNING",
			Claim: "ambiguous", ProofRefs: []string{"diff: internal/a.go:3"},
		}}
		_, admission, err := AdmitArtifact(t.Context(), request)
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope {
			t.Fatalf("ambiguous finding location admission = %q, %v; want out of scope", admission.Decision, err)
		}
	})
	t.Run("a proof reference resolves the same way evidence does", func(t *testing.T) {
		request := newRequest("inspection: internal/a.go:3")
		request.Result.Findings = []Finding{{
			ID: "R3-001", Lens: "reliability", Location: "internal/a.go:3", Severity: "WARNING",
			Claim: "the candidate loses the retry error", ProofRefs: []string{"diff: a.go:3"},
		}}
		_, admission, err := AdmitArtifact(t.Context(), request)
		if err != nil || admission.Decision != ArtifactAdmissionCompleted {
			t.Fatalf("basename proof admission = %q, %v; want completed", admission.Decision, err)
		}
	})
	t.Run("a citation carrying a directory prefix is not a basename", func(t *testing.T) {
		// It said where it meant. Being wrong about that is not ambiguity to
		// resolve, so it stays refused even though its last segment is unique.
		_, admission, err := AdmitArtifact(t.Context(), newRequest("inspection: wrong/a.go:3"))
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope {
			t.Fatalf("prefixed citation admission = %q, %v; want out of scope", admission.Decision, err)
		}
	})
	t.Run("unknown basename stays out of scope", func(t *testing.T) {
		_, admission, err := AdmitArtifact(t.Context(), newRequest("inspection: nowhere.go:1"))
		if err == nil || admission.Decision != ArtifactAdmissionOutOfScope {
			t.Fatalf("unknown basename admission = %q, %v; want out of scope", admission.Decision, err)
		}
	})
}

// TestExtractBoundedSingleJSONObjectReportsStructuralCensus is #2791: a
// truncated reviewer payload used to be refused with one bare sentence, so a
// report could not say whether an array, an object, or the whole payload was
// left open. The refusal now keeps its prefix and appends a census.
func TestExtractBoundedSingleJSONObjectReportsStructuralCensus(t *testing.T) {
	for _, test := range []struct {
		name, payload, want string
	}{
		{
			name:    "unclosed array inside a finding",
			payload: `{"findings":[{"id":"R3-001","proof_refs":["x"`,
			want:    "reviewer payload contains no complete JSON object: 2 objects opened, 0 closed; 2 arrays opened, 0 closed; scan ended at byte 42",
		},
		{
			name:    "unclosed inspection object",
			payload: `{"subject_hash":"sha256:0","inspection":{"status":"completed","paths":["a.go"]`,
			want:    "reviewer payload contains no complete JSON object: 2 objects opened, 0 closed; 1 array opened, 1 closed; scan ended at byte 78",
		},
		{
			name:    "prose only",
			payload: "I inspected the candidate and found nothing to report.",
			want:    "reviewer payload contains no complete JSON object: no object start was found in 54 bytes",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, decision, err := ExtractBoundedSingleJSONObject([]byte(test.payload), 4096)
			if decision != ArtifactAdmissionIncomplete || err == nil || err.Error() != test.want {
				t.Fatalf("ExtractBoundedSingleJSONObject() = %q, %v; want incomplete with %q", decision, err, test.want)
			}
		})
	}
	extracted, decision, err := ExtractBoundedSingleJSONObject([]byte("done\n{\"findings\":[],\"evidence\":[\"[unclosed in string\"]}\n"), 4096)
	if err != nil || decision != ArtifactAdmissionCompleted || string(extracted) != `{"findings":[],"evidence":["[unclosed in string"]}` {
		t.Fatalf("complete payload = %q, %q, %v", extracted, decision, err)
	}
}

// TestEvidenceReportsUnavailableInspectionPhrases is #1867: the phrases a
// reviewer writes when the immutable candidate could not be read must be
// classified as an unavailable inspection, while a completed inspection that
// merely mentions reading stays a genuine result.
func TestEvidenceReportsUnavailableInspectionPhrases(t *testing.T) {
	for _, test := range []struct {
		evidence string
		want     bool
	}{
		{"Read denied on the immutable candidate tree", true},
		{"the frozen tree was denied by filesystem policy", true},
		{"cannot read diff for the candidate", true},
		{"Cannot read manifest of changed paths", true},
		{"the reviewer could not read the candidate at all", true},
		{"unable to read the candidate tree sha256:9f2c", true},
		{"inspection blocked by the sandbox", true},
		{"could not be inspected with read-only Git access", true},
		{"read the manifest and inspected every path", false},
		{"inspected the tree: the loop still stops one entry short", false},
	} {
		if got := evidenceReportsUnavailableInspection(test.evidence); got != test.want {
			t.Fatalf("evidenceReportsUnavailableInspection(%q) = %v, want %v", test.evidence, got, test.want)
		}
	}
}
