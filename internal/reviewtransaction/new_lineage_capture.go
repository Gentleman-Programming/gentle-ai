package reviewtransaction

// Wave 5 fix cycle 2, CRITICAL-A closure (verify-report #10186). Coordinator
// decision, recorded here and in design.md: a MINIMAL capture primitive, not
// a parallel v2-shaped admission pipeline. v2's ArtifactSubject/
// FrozenCandidateContext/reopen/journal machinery is legacy-era complexity
// already scheduled for Wave 7 deletion; rebuilding it for v3 would violate
// the wave's own simplification mandate. v3 needs exactly the minimum that
// makes ReviewCore.finalize's ErrFinalizeRequiresLensResults refusal
// satisfiable: persist captured lens results onto the v3 authority, bound
// to a provider-owned subject hash, through the existing Mutate/CAS path --
// no reopen, no separate artifact store, no findings/evidence ADMISSION
// pipeline (ArtifactSubject/FrozenCandidateContext/reopen stay v2-only).
//
// Fix cycle 3, CRITICAL-E closure (verify-report #10186 cycle 2, coordinator
// decision "option 2, the channel does what it appears to do"): the capture
// primitive DOES persist the reviewer's validated findings (not just
// SubjectHash) -- cycle 2's own capture-result already required and
// validated `findings`/`evidence` were present before this fix, so silently
// discarding them was a channel that looked like it carried findings and
// did not, a fail-open in an authorization system. Persisted findings feed
// the EXISTING AdmitCandidateCausalFindings at finalize (review_facade.go)
// -- the same admission decision function the --admission-findings channel
// already uses, reused rather than duplicated.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

type NewLineageArtifactSubject struct {
	Schema                    string `json:"schema"`
	SubjectHash               string `json:"subject_hash"`
	LineageID                 string `json:"lineage_id"`
	RepositoryID              string `json:"repository_id"`
	BaseTree                  string `json:"base_tree"`
	CandidateTree             string `json:"candidate_tree"`
	ChangedPathManifestSHA256 string `json:"changed_path_manifest_sha256"`
	Lens                      string `json:"lens"`
	SelectedOrder             int    `json:"selected_order"`
}

type NewLineageArtifactBinding struct {
	Subject            NewLineageArtifactSubject  `json:"subject"`
	FrozenPathManifest []ChangedPathManifestEntry `json:"frozen_path_manifest"`
	Inspection         ArtifactInspection         `json:"inspection"`
}

func (binding NewLineageArtifactBinding) Validate(authority NewLineageAuthority, lens string, order int, subjectHash string) error {
	if binding.Subject.Schema != "gentle-ai.new-lineage-artifact-subject/v1" || binding.Subject.SubjectHash != subjectHash || binding.Subject.SubjectHash != NewLineageArtifactSubjectHash(authority, lens, order) || binding.Subject.LineageID != authority.LineageID || binding.Subject.BaseTree != authority.CandidateIdentity.BaseTree || binding.Subject.CandidateTree != authority.CandidateIdentity.CandidateTree || binding.Subject.Lens != lens || binding.Subject.SelectedOrder != order {
		return errors.New("new-lineage artifact subject does not bind the frozen authority") // refusal:by-design operator-knowledge: capture must use the exact frozen subject
	}
	if binding.Inspection.Status != ArtifactInspectionCompleted {
		return errors.New("new-lineage artifact inspection is not completed") // refusal:by-design operator-knowledge: incomplete provider inspection cannot be admitted
	}
	if err := ValidateChangedPathManifest(binding.FrozenPathManifest); err != nil {
		return errors.New("new-lineage artifact frozen manifest is not canonical") // refusal:by-design operator-knowledge: provider coverage must bind the exact frozen path set
	}
	coverage, coverageErr := validateCompleteInspectionCoverage(binding.Inspection.Paths, binding.FrozenPathManifest)
	if coverageErr != nil || coverage != nil {
		return errors.New("new-lineage artifact inspection does not cover the frozen manifest") // refusal:by-design operator-knowledge: canonical inspection evidence is required
	}
	return nil
}

func NewLineageArtifactBindingForAuthority(ctx context.Context, repo string, authority NewLineageAuthority, lens string, order int, inspection ArtifactInspection) (NewLineageArtifactBinding, error) {
	if order < 0 || order >= len(authority.SelectedLenses) || authority.SelectedLenses[order] != lens {
		return NewLineageArtifactBinding{}, ErrNewLineageCaptureLensNotSelected
	}
	manifest, err := frozenNewLineagePathManifest(ctx, repo, authority.CandidateIdentity)
	if err != nil {
		return NewLineageArtifactBinding{}, err
	}
	binding := NewLineageArtifactBinding{Subject: NewLineageArtifactSubject{Schema: "gentle-ai.new-lineage-artifact-subject/v1", SubjectHash: NewLineageArtifactSubjectHash(authority, lens, order), LineageID: authority.LineageID, RepositoryID: authority.CandidateIdentity.RepositoryID, BaseTree: authority.CandidateIdentity.BaseTree, CandidateTree: authority.CandidateIdentity.CandidateTree, ChangedPathManifestSHA256: authority.CandidateIdentity.ChangedPathsModesDigest, Lens: lens, SelectedOrder: order}, FrozenPathManifest: manifest, Inspection: inspection}
	return binding, binding.Validate(authority, lens, order, binding.Subject.SubjectHash)
}

func frozenNewLineagePathManifest(ctx context.Context, repo string, candidate CandidateIdentity) ([]ChangedPathManifestEntry, error) {
	raw, err := runGit(ctx, repo, nil, nil, "diff", "--raw", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", candidate.BaseTree, candidate.CandidateTree, "--")
	if err != nil {
		return nil, err
	}
	modes, err := parseRawDiffModes(raw)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(modes))
	for path := range modes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	manifest := make([]ChangedPathManifestEntry, 0, len(paths))
	for _, path := range paths {
		mode := modes[path]
		entry := ChangedPathManifestEntry{Path: path, Status: mode.status, OldMode: mode.oldMode, NewMode: mode.newMode, Deleted: mode.status == CandidatePathDeleted, TypeChanged: mode.status == CandidatePathTypeChanged, ModeOnly: mode.status == CandidatePathModified && mode.oldObject == mode.newObject && mode.oldMode != mode.newMode}
		if err := validateChangedPathManifestEntry(entry); err != nil {
			return nil, err
		}
		manifest = append(manifest, entry)
	}
	return manifest, nil
}

// NewLineageArtifactSubjectHash is C-A's minimal provider-owned binding: it
// proves a reviewer result was computed against THIS exact frozen authority
// (lineage, candidate identity), lens, and order -- the same "the reviewer
// inspected the exact frozen candidate" guarantee v2's ArtifactSubject.
// SubjectHash gives, derived only from data already persisted on
// NewLineageAuthority, with no additional Git diff/manifest work (that
// richer FrozenCandidateContext derivation is v2-only).
//
// Deliberately excludes the authority's own revision (the CAS token): the
// revision changes on every Mutate, including the very capture this hash
// authorizes, so binding to it would relabel the "correct" subject hash on
// every resubmission -- the exact frozen-anchor lesson Phase 9 already
// applied to the archive re-verify gate (see review_reverify.go). LineageID
// and CandidateIdentity are frozen once at start and never mutate again for
// the authority's whole lifetime, so binding to them instead keeps this hash
// stable across every capture, including the idempotent-resubmission case.
func NewLineageArtifactSubjectHash(authority NewLineageAuthority, lens string, order int) string {
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.new-lineage-artifact-subject/v1\x00"))
	for _, value := range []string{
		authority.LineageID, authority.CandidateIdentity.RepositoryID,
		authority.CandidateIdentity.BaseTree, authority.CandidateIdentity.CandidateTree,
		authority.CandidateIdentity.ChangedPathsModesDigest, authority.CandidateIdentity.PolicyHash,
		lens,
	} {
		writeLengthPrefixed(hash, []byte(value))
	}
	writeLengthPrefixed(hash, []byte(strconv.Itoa(order)))
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

var (
	// ErrNewLineageCaptureNotReviewable is refused when capture-result is
	// attempted against an authority that is not reviewing or validating
	// (approved/escalated are terminal; correcting has no lens-capture
	// concept at all -- captures happen before a correction, not during one).
	ErrNewLineageCaptureNotReviewable = errors.New("new-lineage capture-result requires a reviewing or validating authority") // refusal:by-design operator-knowledge: the caller must capture against the exact reviewing/validating window a fresh review status reports; there is no operator command that reopens a terminal or correcting authority for capture
	// ErrNewLineageCaptureLensNotSelected is refused when the named lens/order
	// does not match the frozen SelectedLenses set at that exact index --
	// C-A's "reject unknown/missing lenses against the frozen selection".
	ErrNewLineageCaptureLensNotSelected = errors.New("capture binding does not match the frozen selected-lens order") // refusal:by-design operator-knowledge: SelectedLenses is frozen at start and never renegotiated; the caller must capture only a lens genuinely selected, at its genuine order
	// ErrNewLineageCaptureSubjectMismatch is refused when the submitted
	// subject hash does not match NewLineageArtifactSubjectHash's own
	// derivation for this exact authority/revision/lens/order.
	ErrNewLineageCaptureSubjectMismatch = errors.New("captured reviewer result does not bind the provider-owned subject hash") // refusal:by-design operator-knowledge: the reviewer must echo the exact subject hash a fresh capture-result --preflight derives; there is no operator command that fabricates a matching binding
	// ErrNewLineageCaptureConflict is refused when a lens already has a
	// captured result under a DIFFERENT subject hash -- one-shot per lens, no
	// reopen machinery (C-A's explicit scope boundary).
	ErrNewLineageCaptureConflict = errors.New("lens already captured with a different binding; capture is one-shot per lens") // refusal:by-design world-action: v3 has no reviewer-artifact reopen concept (that is v2-only, Wave 7 deletion scope); the fix is a fresh review start for a corrected candidate, not an operator command that reopens this capture
	// ErrNewLineageCaptureIncompleteSeverity is refused when a severe
	// (BLOCKER/CRITICAL) finding is captured without a supported
	// evidence_class and causal_disposition -- W-10 (Wave 7 S1, design
	// decision 3a). The message is verbatim identical to
	// artifact_admission.go's own --admission-findings-channel refusal for
	// the same shape, so v3 fails the same way v2 already does for the same
	// operator mistake.
	ErrNewLineageCaptureIncompleteSeverity = errors.New("severe reviewer finding requires supported evidence_class and causal_disposition") // refusal:by-design operator-knowledge: the reviewer must submit a supported evidence_class and causal_disposition for every BLOCKER/CRITICAL finding; there is no operator command that fabricates that classification on the reviewer's behalf
)

// CaptureLensResult persists one captured reviewer result onto a v3
// authority through the existing Mutate/CAS path. Semantics (C-A, extended
// by C-E):
//   - only a reviewing or validating authority may capture;
//   - lens/order must match the frozen SelectedLenses set exactly;
//   - subjectHash must match NewLineageArtifactSubjectHash's own derivation;
//   - findings ARE persisted verbatim (C-E) -- fed into
//     AdmitCandidateCausalFindings at finalize, never discarded;
//   - one-shot per lens: capturing the SAME lens again with the IDENTICAL
//     subject hash AND identical findings is an idempotent replace (Mutate's
//     own byte-identical no-op path); anything else for an already-captured
//     lens is refused (ErrNewLineageCaptureConflict) -- there is no reopen.
func (store AuthorityStore) CaptureLensResult(ctx context.Context, expectedRevision, lens string, order int, subjectHash string, findings []FindingEvidence) (NewLineageRecord, error) {
	if _, err := store.Mutate(ctx, expectedRevision, func(next *NewLineageAuthority) error {
		normalizedFindings := append([]FindingEvidence(nil), findings...)
		if err := ValidateNewLineageLensResult(*next, lens, order, subjectHash, normalizedFindings); err != nil {
			return err
		}
		for _, existing := range next.CapturedResults {
			if existing.Lens != lens {
				continue
			}
			return nil // validation permits only the idempotent existing result
		}
		next.CapturedResults = append(append([]NewLineageCapturedResult(nil), next.CapturedResults...), NewLineageCapturedResult{
			Lens: lens, Order: order, SubjectHash: subjectHash, Findings: normalizedFindings,
		})
		return nil
	}); err != nil {
		return NewLineageRecord{}, err
	}
	return store.Load()
}

func (store AuthorityStore) CaptureLensResultWithProviderEvidenceAndInspection(ctx context.Context, expectedRevision, lens string, order int, subjectHash string, inspection ArtifactInspection, claims []ProviderCausalEvidence) (NewLineageRecord, error) {
	record, err := store.Load()
	if err != nil {
		return NewLineageRecord{}, err
	}
	binding, err := NewLineageArtifactBindingForAuthority(ctx, store.repo, record.Authority, lens, order, inspection)
	if err != nil {
		return NewLineageRecord{}, err
	}
	carrier, err := DeriveProviderCausalCarrier(ctx, store.repo, subjectHash, record.Authority.CandidateIdentity, claims)
	if err != nil {
		return NewLineageRecord{}, err
	}
	if err := binding.Validate(record.Authority, lens, order, subjectHash); err != nil {
		return NewLineageRecord{}, err
	}
	carrier.ArtifactBinding = binding
	carrier.AggregateDigest = providerAggregateDigest(carrier)
	if _, err := store.Mutate(ctx, expectedRevision, func(next *NewLineageAuthority) error {
		if next.State != NewLineageStateReviewing && next.State != NewLineageStateValidating {
			return fmt.Errorf("%w: lineage %q is %q", ErrNewLineageCaptureNotReviewable, next.LineageID, next.State)
		}
		if order < 0 || order >= len(next.SelectedLenses) || next.SelectedLenses[order] != lens || subjectHash != NewLineageArtifactSubjectHash(*next, lens, order) {
			return ErrNewLineageCaptureSubjectMismatch
		}
		for _, existing := range next.CapturedResults {
			if existing.Lens == lens {
				if existing.SubjectHash == subjectHash && reflect.DeepEqual(existing.Provider, carrier) {
					return nil
				}
				return fmt.Errorf("%w: lineage %q lens %q", ErrNewLineageCaptureConflict, next.LineageID, lens)
			}
		}
		next.CapturedResults = append(next.CapturedResults, NewLineageCapturedResult{Lens: lens, Order: order, SubjectHash: subjectHash, Provider: carrier})
		return nil
	}); err != nil {
		return NewLineageRecord{}, err
	}
	return store.Load()
}

// ValidateNewLineageLensResult applies the native admission checks without
// mutating authority, allowing capture and input-backed preflight to share them.
func ValidateNewLineageLensResult(authority NewLineageAuthority, lens string, order int, subjectHash string, findings []FindingEvidence) error {
	if authority.State != NewLineageStateReviewing && authority.State != NewLineageStateValidating {
		return fmt.Errorf("%w: lineage %q is %q", ErrNewLineageCaptureNotReviewable, authority.LineageID, authority.State)
	}
	if order < 0 || order >= len(authority.SelectedLenses) || authority.SelectedLenses[order] != lens {
		return fmt.Errorf("%w: lineage %q lens %q order %d", ErrNewLineageCaptureLensNotSelected, authority.LineageID, lens, order)
	}
	if subjectHash != NewLineageArtifactSubjectHash(authority, lens, order) {
		return fmt.Errorf("%w: lineage %q lens %q", ErrNewLineageCaptureSubjectMismatch, authority.LineageID, lens)
	}
	for _, finding := range findings {
		if isSevereSeverity(finding.Severity) && (!isSupportedEvidenceClass(finding.Class) || !isSupportedCausalDisposition(finding.Causality)) {
			return fmt.Errorf("%w: lineage %q lens %q finding %q", ErrNewLineageCaptureIncompleteSeverity, authority.LineageID, lens, finding.FindingID)
		}
	}
	for _, existing := range authority.CapturedResults {
		if existing.Lens == lens && (existing.SubjectHash != subjectHash || !reflect.DeepEqual(existing.Findings, findings)) {
			return fmt.Errorf("%w: lineage %q lens %q", ErrNewLineageCaptureConflict, authority.LineageID, lens)
		}
	}
	return nil
}
