package reviewtransaction

// Wave 5 fix cycle 2, CRITICAL-A (verify-report #10186). Unit-level RED/
// GREEN coverage for the minimal capture primitive, independent of the CLI
// routing (internal/cli's own end-to-end test drives the real verify repro
// through the compiled binary path).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func newLineageCaptureFixtureStore(t *testing.T, lenses []string) (AuthorityStore, NewLineageRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	treeOutput, err := runGit(context.Background(), repo, nil, nil, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	tree := strings.TrimSpace(string(treeOutput))
	store, err := NewLineageAuthorityStore(context.Background(), repo, "capture-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		next.State = NewLineageStateReviewing
		next.CandidateIdentity = CandidateIdentity{
			RepositoryID: "repo-id", BaseTree: tree, CandidateTree: tree, PolicyHash: "sha256:" + hash("policy"),
		}
		next.Tier = RiskMedium
		next.SelectedLenses = lenses
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return store, record
}

// TestCaptureLensResult_HappyPathThenFinalizeSatisfiesLensResults is C-A's
// core proof: a captured result for every selected lens, fed into
// hasCapturedAllSelectedLenses via CapturedLensNames, satisfies the exact
// check ReviewCore.finalize enforces (review_core.go:159) -- the check that
// was permanently unsatisfiable before this fix.
func TestCaptureLensResult_HappyPathThenFinalizeSatisfiesLensResults(t *testing.T) {
	store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
	subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)

	updated, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, nil)
	if err != nil {
		t.Fatalf("CaptureLensResult happy path: %v", err)
	}
	if len(updated.Authority.CapturedResults) != 1 || updated.Authority.CapturedResults[0].Lens != "review-reliability" {
		t.Fatalf("captured results = %#v", updated.Authority.CapturedResults)
	}
	if !hasCapturedAllSelectedLenses(updated.Authority.SelectedLenses, updated.Authority.CapturedLensNames()) {
		t.Fatalf("hasCapturedAllSelectedLenses = false after capturing every selected lens: captured=%v selected=%v",
			updated.Authority.CapturedLensNames(), updated.Authority.SelectedLenses)
	}
}

// TestCaptureLensResult_RejectsLensNotInFrozenSelection proves C-A's
// explicit reject-unknown-lens requirement.
func TestCaptureLensResult_RejectsLensNotInFrozenSelection(t *testing.T) {
	store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
	subject := NewLineageArtifactSubjectHash(record.Authority, "review-risk", 0)

	if _, err := store.CaptureLensResult(context.Background(), record.Revision, "review-risk", 0, subject, nil); err == nil {
		t.Fatal("captured a lens that was never in the frozen SelectedLenses set")
	}
}

// TestCaptureLensResult_RejectsWrongOrder proves the order binding: a real
// selected lens at the WRONG order index must still be refused.
func TestCaptureLensResult_RejectsWrongOrder(t *testing.T) {
	store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability", "review-risk"})
	subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 1)

	if _, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 1, subject, nil); err == nil {
		t.Fatal("captured a real lens at the wrong frozen order index")
	}
}

// TestCaptureLensResult_RejectsSubjectHashMismatch proves the "echo
// subject_hash binding per the reviewer contract" requirement: an
// arbitrary/fabricated subject hash must be refused, not silently accepted.
func TestCaptureLensResult_RejectsSubjectHashMismatch(t *testing.T) {
	store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})

	if _, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, "sha256:fabricated", nil); err == nil {
		t.Fatal("captured a result whose subject hash does not match the provider-owned derivation")
	}
}

// TestCaptureLensResult_OneShotNoReopen proves C-A's explicit scope
// boundary: recapturing the SAME lens with the IDENTICAL subject hash is an
// idempotent no-op (Mutate's own byte-identical replay path), but a
// DIFFERENT subject hash for an already-captured lens is refused -- there is
// no reopen machinery.
func TestCaptureLensResult_OneShotNoReopen(t *testing.T) {
	store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
	subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)

	first, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, nil)
	if err != nil {
		t.Fatalf("first capture: %v", err)
	}

	t.Run("identical resubmission is an idempotent no-op", func(t *testing.T) {
		again, err := store.CaptureLensResult(context.Background(), first.Revision, "review-reliability", 0, subject, nil)
		if err != nil {
			t.Fatalf("identical resubmission: %v", err)
		}
		if again.Revision != first.Revision {
			t.Fatalf("identical resubmission changed the revision: %q -> %q, want an idempotent no-op", first.Revision, again.Revision)
		}
	})

	t.Run("different subject hash for an already-captured lens is refused, not reopened", func(t *testing.T) {
		differentSubject := NewLineageArtifactSubjectHash(first.Authority, "review-reliability-different-domain", 0)
		if _, err := store.CaptureLensResult(context.Background(), first.Revision, "review-reliability", 0, differentSubject, nil); err == nil {
			t.Fatal("recaptured an already-captured lens under a different subject hash; capture must be one-shot, no reopen")
		}
	})
}

// TestCaptureLensResult_RejectsNonReviewableStates proves the state gate:
// only reviewing/validating may capture -- approved, escalated, and
// correcting must all be refused.
func TestCaptureLensResult_RejectsNonReviewableStates(t *testing.T) {
	for _, state := range []NewLineageState{NewLineageStateApproved, NewLineageStateEscalated, NewLineageStateCorrecting} {
		t.Run(string(state), func(t *testing.T) {
			store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
			revision, err := store.Mutate(context.Background(), record.Revision, func(next *NewLineageAuthority) error {
				next.State = state
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			authority, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			subject := NewLineageArtifactSubjectHash(authority.Authority, "review-reliability", 0)
			if _, err := store.CaptureLensResult(context.Background(), revision, "review-reliability", 0, subject, nil); err == nil {
				t.Fatalf("captured against a %q authority, want refused", state)
			}
		})
	}
}

// TestCaptureLensResult_PersistsFindingsForAdmission is C-E's unit-level
// proof (Wave 5 fix cycle 3, verify-report #10186 cycle 2): findings
// submitted with a capture are PERSISTED, not discarded, and
// CapturedFindingEvidence flattens them into the exact shape
// AdmitCandidateCausalFindings consumes -- reused, not duplicated.
func TestCaptureLensResult_PersistsFindingsForAdmission(t *testing.T) {
	store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
	subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)
	findings := []FindingEvidence{
		{FindingID: "R3-boom-div-zero", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "lib/boom.go:1"},
	}

	updated, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, findings)
	if err != nil {
		t.Fatalf("CaptureLensResult with findings: %v", err)
	}
	if len(updated.Authority.CapturedResults) != 1 || len(updated.Authority.CapturedResults[0].Findings) != 1 ||
		updated.Authority.CapturedResults[0].Findings[0].FindingID != "R3-boom-div-zero" {
		t.Fatalf("captured findings = %#v, want the submitted finding persisted verbatim", updated.Authority.CapturedResults)
	}
	admitted, followUp, _ := AdmitCandidateCausalFindings(updated.Authority.CapturedFindingEvidence())
	if len(admitted) != 1 || admitted[0] != "R3-boom-div-zero" || len(followUp) != 0 {
		t.Fatalf("AdmitCandidateCausalFindings(CapturedFindingEvidence()) = admitted=%v followUp=%v, want admitted=[R3-boom-div-zero] followUp=[]", admitted, followUp)
	}
}

// TestCaptureLensResult_DifferentFindingsForSameLensIsConflictNotReopen
// extends the one-shot guard (C-A) to findings (C-E): an already-captured
// lens resubmitted with the SAME subject hash but DIFFERENT findings must
// still refuse -- one-shot means the whole captured result is immutable,
// not just the subject hash.
func TestCaptureLensResult_DifferentFindingsForSameLensIsConflictNotReopen(t *testing.T) {
	store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
	subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)

	first, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, nil)
	if err != nil {
		t.Fatalf("first capture (zero findings): %v", err)
	}
	differentFindings := []FindingEvidence{{FindingID: "late-finding", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "proof"}}
	if _, err := store.CaptureLensResult(context.Background(), first.Revision, "review-reliability", 0, subject, differentFindings); err == nil {
		t.Fatal("recaptured the same lens/subject-hash with different findings; capture must be one-shot for the whole result, not just the subject hash")
	}
}

// TestFindingEvidenceSeverityFieldRoundTrips is W-10's structural
// precondition (Wave 7 S1, design decision 3a): FindingEvidence today drops
// severity entirely (newLineageCapturedFindings, review_artifact.go:940-942
// pre-fix), which is the reason a severe finding with no evidence_class/
// causal_disposition can be captured with nothing to refuse it on. This
// proves the field exists, round-trips through JSON, and is omitted (never
// "severity":"") when unset -- matching every other omitempty field on this
// struct.
func TestFindingEvidenceSeverityFieldRoundTrips(t *testing.T) {
	severe := FindingEvidence{FindingID: "R3-boom", Severity: "BLOCKER", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "lib/boom.go:1"}
	payload, err := json.Marshal(severe)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(payload), `"severity":"BLOCKER"`) {
		t.Fatalf("marshaled FindingEvidence = %s, want a \"severity\":\"BLOCKER\" field", payload)
	}
	var decoded FindingEvidence
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Severity != "BLOCKER" {
		t.Fatalf("decoded Severity = %q, want %q", decoded.Severity, "BLOCKER")
	}

	unset := FindingEvidence{FindingID: "R3-quiet", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "lib/quiet.go:1"}
	unsetPayload, err := json.Marshal(unset)
	if err != nil {
		t.Fatalf("marshal unset: %v", err)
	}
	if strings.Contains(string(unsetPayload), "severity") {
		t.Fatalf("marshaled unset-severity FindingEvidence = %s, want no \"severity\" key (omitempty)", unsetPayload)
	}
}

// TestCaptureLensResult_RefusesSevereFindingMissingEvidenceOrCausality is
// W-10's behavioral proof (design decision 3a): a BLOCKER/CRITICAL finding
// captured without a supported evidence_class or causal_disposition must be
// refused at capture time -- mirroring artifact_admission.go's identical
// --admission-findings-channel refusal ("severe reviewer finding requires
// supported evidence_class and causal_disposition") verbatim, so v3 fails
// the same way v2 already does for the same operator mistake. A WARNING
// finding with the identical gaps is NOT refused -- only BLOCKER/CRITICAL
// (isSevereSeverity) triggers the requirement.
func TestCaptureLensResult_RefusesSevereFindingMissingEvidenceOrCausality(t *testing.T) {
	t.Run("severe finding missing evidence_class and causal_disposition is refused", func(t *testing.T) {
		store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
		subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)
		findings := []FindingEvidence{{FindingID: "R3-incomplete", Severity: "BLOCKER", Proof: "lib/boom.go:1"}}

		_, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, findings)
		if err == nil {
			t.Fatal("captured a BLOCKER finding with no evidence_class/causal_disposition, want refused")
		}
		if !strings.Contains(err.Error(), "severe reviewer finding requires supported evidence_class and causal_disposition") {
			t.Fatalf("error = %q, want the v2-verbatim severe-finding refusal message", err.Error())
		}
	})

	t.Run("severe finding with only evidence_class missing causal_disposition is refused", func(t *testing.T) {
		store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
		subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)
		findings := []FindingEvidence{{FindingID: "R3-half", Severity: "CRITICAL", Class: EvidenceDeterministic, Proof: "lib/boom.go:1"}}

		if _, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, findings); err == nil {
			t.Fatal("captured a CRITICAL finding with no causal_disposition, want refused")
		}
	})

	t.Run("WARNING finding with the identical gaps is not refused", func(t *testing.T) {
		store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
		subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)
		findings := []FindingEvidence{{FindingID: "R3-warning", Severity: "WARNING", Proof: "lib/boom.go:1"}}

		if _, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, findings); err != nil {
			t.Fatalf("WARNING finding with no evidence_class/causal_disposition was refused: %v, want accepted (only severe findings require these)", err)
		}
	})

	t.Run("severe finding with supported evidence_class and causal_disposition is accepted", func(t *testing.T) {
		store, record := newLineageCaptureFixtureStore(t, []string{"review-reliability"})
		subject := NewLineageArtifactSubjectHash(record.Authority, "review-reliability", 0)
		findings := []FindingEvidence{{FindingID: "R3-complete", Severity: "BLOCKER", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "lib/boom.go:1"}}

		if _, err := store.CaptureLensResult(context.Background(), record.Revision, "review-reliability", 0, subject, findings); err != nil {
			t.Fatalf("complete severe finding was refused: %v, want accepted", err)
		}
	})
}
