package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ArtifactAdmissionSchema = "gentle-ai.review-artifact-admission/v1"

type ArtifactAdmissionDecision string

const (
	ArtifactAdmissionCompleted       ArtifactAdmissionDecision = "completed"
	ArtifactAdmissionIncomplete      ArtifactAdmissionDecision = "incomplete"
	ArtifactAdmissionAmbiguous       ArtifactAdmissionDecision = "ambiguous"
	ArtifactAdmissionOutOfScope      ArtifactAdmissionDecision = "out_of_scope"
	ArtifactAdmissionBindingMismatch ArtifactAdmissionDecision = "binding_mismatch"
)

type ArtifactInspectionStatus string

const ArtifactInspectionCompleted ArtifactInspectionStatus = "completed"

// ArtifactInspection is the reviewer's structured assertion that every path
// in the immutable manifest was actually inspected.
type ArtifactInspection struct {
	Status ArtifactInspectionStatus `json:"status"`
	Paths  []string                 `json:"paths"`
}

// ArtifactAdmissionCausalDowngrade names one severe finding whose self-claimed
// candidate-causal disposition (introduced/behavior-activated/worsened) could
// not be proven by repository-derived changed-line evidence. Admission
// downgrades exactly this finding to CausalUnknown -- the same disposition
// CompactReviewView's replay already assigns an unverified claim -- and
// admits the rest of the artifact, instead of rejecting the whole result for
// one borderline finding.
type ArtifactAdmissionCausalDowngrade struct {
	FindingID string `json:"finding_id"`
	Reason    string `json:"reason"`
}

// ArtifactAdmission records the provider's decision and exact raw/canonical
// payload identities. Only completed records are reviewer results.
type ArtifactAdmission struct {
	Schema                    string                             `json:"schema"`
	Decision                  ArtifactAdmissionDecision          `json:"decision"`
	SubjectHash               string                             `json:"subject_hash"`
	RawSHA256                 string                             `json:"raw_sha256"`
	CanonicalSHA256           string                             `json:"canonical_sha256"`
	ResultHash                string                             `json:"result_hash,omitempty"`
	CandidateCausalFindingIDs []string                           `json:"candidate_causal_finding_ids"`
	DowngradedCausalFindings  []ArtifactAdmissionCausalDowngrade `json:"downgraded_causal_findings,omitempty"`
	Diagnostic                string                             `json:"diagnostic,omitempty"`
}

// EvidenceDerivationStatus reports whether CandidateCausalFindingIDs was
// computed from a complete repository-derived changed-line signal for every
// self-claimed candidate-causal finding it covers, or whether that derivation
// degraded for at least one of them (binary content, an untraceable rename,
// or another condition the changed-line derivation could not resolve). A
// request that never sets this field defaults to EvidenceDerivationComplete,
// matching every caller that replays an already-admitted result rather than
// freshly deriving evidence -- the degraded case can only have been produced
// by a fresh derivation that already gated it before the replay was ever
// persisted.
type EvidenceDerivationStatus string

const (
	EvidenceDerivationComplete EvidenceDerivationStatus = "complete"
	EvidenceDerivationDegraded EvidenceDerivationStatus = "degraded"
)

type ArtifactAdmissionRequest struct {
	ExpectedSubject   ArtifactSubject
	FrozenContext     FrozenCandidateContext
	EchoedSubjectHash string
	Inspection        ArtifactInspection
	Result            LensResult
	// CandidateCausalFindingIDs is the canonical set whose claimed candidate
	// causality the provider verified against repository-derived changed-line
	// evidence before admission.
	CandidateCausalFindingIDs []string
	// EvidenceDerivation and EvidenceDerivationReason report the quality of
	// the changed-line derivation behind CandidateCausalFindingIDs. When
	// EvidenceDerivationDegraded, AdmitArtifact refuses to downgrade any
	// unverified self-claimed finding to CausalUnknown -- a degraded
	// derivation cannot distinguish "proven not caused by the candidate" from
	// "no dependable signal could be derived", and downgrading on that
	// ambiguity would silently neutralize a real blocker.
	EvidenceDerivation       EvidenceDerivationStatus
	EvidenceDerivationReason string
	RawPayload               []byte
	CanonicalPayload         []byte
}

// ArtifactAdmissionError exposes the stable native decision without requiring
// callers to parse diagnostic prose.
type ArtifactAdmissionError struct {
	Admission  ArtifactAdmission
	Diagnostic *ArtifactAdmissionDiagnostic
	cause      error
}

func (err *ArtifactAdmissionError) Error() string {
	message := fmt.Sprintf("reviewer artifact admission %s: %s", err.Admission.Decision, err.Admission.Diagnostic)
	if err.Diagnostic != nil {
		encoded, _ := json.Marshal(err.Diagnostic)
		message += "; admission_diagnostic=" + string(encoded)
	}
	return message
}

func (err *ArtifactAdmissionError) Unwrap() error { return err.cause }

// ArtifactAdmissionDiagnostic contains bounded, non-sensitive recovery fields.
type ArtifactAdmissionDiagnostic struct {
	Code             string `json:"code"`
	FindingID        string `json:"finding_id,omitempty"`
	Location         string `json:"location,omitempty"`
	Reason           string `json:"reason"`
	MissingPathCount int    `json:"missing_path_count,omitempty"`
	ForeignPathCount int    `json:"foreign_path_count,omitempty"`
}

func safeAdmissionLocation(code, value, reason string) string {
	value = strings.TrimSpace(value)
	if len(value) > 256 || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.Count(value, ":") != 1 {
		return ""
	}
	separator := strings.IndexByte(value, ':')
	logicalPath, suffix := value[:separator], value[separator+1:]
	if _, err := normalizeLogicalPath(logicalPath); err != nil {
		return ""
	}
	if !artifactAdmissionLocationSuffix.MatchString(suffix) {
		return ""
	}
	_, locationErr := parseFindingLocation(value)
	if code == "candidate_causality_unproven" {
		if reason != "line_not_changed_by_candidate" || locationErr != nil {
			return ""
		}
		return value
	}
	if code == "evidence_path_out_of_scope" || code == "proof_path_out_of_scope" {
		// The offending citation names an unknown repository path, so the token
		// itself must still parse as a clean path:line shape before it may be
		// echoed; malformed tokens (absolute paths, traversal, punctuation) stay
		// scrubbed and are named only by the generic reason.
		if reason != "unknown_or_malformed_repository_path" || locationErr != nil {
			return ""
		}
		return value
	}
	var typedLocationErr *FindingLocationError
	if code != "invalid_finding_location" || !errors.As(locationErr, &typedLocationErr) ||
		typedLocationErr == nil || reason != string(typedLocationErr.Reason) {
		return ""
	}
	return value
}

// InspectionCoverageError gives advisory and direct callers scrubbed coverage counts.
type InspectionCoverageError struct {
	MissingPathCount int
	ForeignPathCount int
}

func (err *InspectionCoverageError) reason() string {
	switch {
	case err.MissingPathCount > 0 && err.ForeignPathCount > 0:
		return "missing_and_foreign_inspection_paths"
	case err.MissingPathCount > 0:
		return "missing_frozen_manifest_paths"
	default:
		return "foreign_inspection_paths"
	}
}

func (err *InspectionCoverageError) Error() string {
	count := err.ForeignPathCount
	if err.MissingPathCount > 0 {
		count = err.MissingPathCount
	}
	return fmt.Sprintf("reviewer inspection coverage: %s=%d", err.reason(), count)
}

func inspectionCoverageDiagnostic(coverage *InspectionCoverageError) *ArtifactAdmissionDiagnostic {
	return &ArtifactAdmissionDiagnostic{
		Code:             "inspection_coverage",
		Reason:           coverage.reason(),
		MissingPathCount: coverage.MissingPathCount,
		ForeignPathCount: coverage.ForeignPathCount,
	}
}

func validateCompleteInspectionCoverage(paths []string, manifest []ChangedPathManifestEntry) (*InspectionCoverageError, error) {
	inspected, err := canonicalPaths(paths)
	if err != nil {
		return nil, err
	}
	wantPaths := make([]string, len(manifest))
	for index, entry := range manifest {
		wantPaths[index] = entry.Path
	}
	want, err := canonicalPaths(wantPaths)
	if err != nil {
		return nil, errors.New("frozen changed-path manifest is not canonical") // refusal:by-design world-action: a frozen manifest that does not canonicalize cannot safely bind reviewer coverage
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, path := range want {
		wantSet[path] = struct{}{}
	}
	coverage := &InspectionCoverageError{}
	for _, path := range inspected {
		if _, ok := wantSet[path]; !ok {
			coverage.ForeignPathCount++
		}
	}
	inspectedSet := make(map[string]struct{}, len(inspected))
	for _, path := range inspected {
		inspectedSet[path] = struct{}{}
	}
	for _, path := range want {
		if _, ok := inspectedSet[path]; !ok {
			coverage.MissingPathCount++
		}
	}
	if coverage.MissingPathCount == 0 && coverage.ForeignPathCount == 0 {
		return nil, nil
	}
	return coverage, coverage
}

func findingAdmissionDiagnostic(code, findingID, location, reason string) *ArtifactAdmissionDiagnostic {
	findingID = strings.TrimSpace(findingID)
	if len(findingID) > 128 || !artifactFindingID.MatchString(findingID) {
		findingID = ""
	}
	return &ArtifactAdmissionDiagnostic{
		Code: code, FindingID: findingID, Location: safeAdmissionLocation(code, location, reason), Reason: reason,
	}
}

// NewArtifactLocationAdmissionError preserves a typed location cause while
// exposing the stable admission decision and bounded recovery details.
func NewArtifactLocationAdmissionError(findingID, location string, cause error) error {
	var locationErr *FindingLocationError
	reason := "invalid_location"
	if errors.As(cause, &locationErr) {
		reason = string(locationErr.Reason)
	}
	admission := ArtifactAdmission{Decision: ArtifactAdmissionOutOfScope, Diagnostic: "reviewer finding location is invalid"}
	return &ArtifactAdmissionError{
		Admission:  admission,
		Diagnostic: findingAdmissionDiagnostic("invalid_finding_location", findingID, location, reason),
		cause:      cause,
	}
}

// Validate checks admission is a self-consistent completed binding for
// subject. canonicalPayload is the exact bytes the caller stores or intends
// to store as the admitted result's envelope Result (see
// CanonicalReviewerResultPayload) -- it is only compared against
// CanonicalSHA256 when DowngradedCausalFindings is non-empty, since that is
// the one case where CanonicalSHA256 is derived from a rewritten (not
// caller-submitted) canonical form and a caller-local re-serialization could
// silently drift from what AdmitArtifact actually hashed.
func (admission ArtifactAdmission) Validate(subject ArtifactSubject, canonicalPayload []byte) error {
	if admission.Schema != ArtifactAdmissionSchema || admission.Decision != ArtifactAdmissionCompleted ||
		admission.SubjectHash != subject.SubjectHash || !validSHA256(admission.RawSHA256) ||
		!validSHA256(admission.CanonicalSHA256) || !validSHA256(admission.ResultHash) ||
		admission.CandidateCausalFindingIDs == nil || strings.TrimSpace(admission.Diagnostic) != "" {
		return errors.New("artifact admission is not a completed binding")
	}
	ids, err := canonicalStrings(admission.CandidateCausalFindingIDs, "candidate-causal finding id")
	if err != nil || !equalStrings(ids, admission.CandidateCausalFindingIDs) {
		return errors.New("artifact admission candidate-causal finding IDs are not canonical")
	}
	for _, id := range ids {
		if !artifactFindingID.MatchString(id) {
			return errors.New("artifact admission candidate-causal finding ID is invalid")
		}
	}
	downgradedIDs := make([]string, len(admission.DowngradedCausalFindings))
	for index, downgrade := range admission.DowngradedCausalFindings {
		if !artifactFindingID.MatchString(downgrade.FindingID) || downgrade.Reason != "unverified_location" || stringIndex(ids, downgrade.FindingID) >= 0 {
			return errors.New("artifact admission downgraded causal finding is invalid") // refusal:by-design world-action: only AdmitArtifact constructs this record, so a malformed entry is a provider code defect no operator command can repair
		}
		downgradedIDs[index] = downgrade.FindingID
	}
	canonicalDowngradedIDs, err := canonicalStrings(downgradedIDs, "downgraded causal finding id")
	if err != nil || !equalStrings(canonicalDowngradedIDs, downgradedIDs) {
		return errors.New("artifact admission downgraded causal finding IDs are not canonical") // refusal:by-design world-action: only AdmitArtifact constructs this record, so non-canonical IDs are a provider code defect no operator command can repair
	}
	if len(admission.DowngradedCausalFindings) > 0 && admission.CanonicalSHA256 != payloadSHA256(canonicalPayload) {
		return errors.New("artifact admission canonical digest does not match the downgraded canonical payload") // refusal:by-design world-action: only AdmitArtifact and its exact stored bytes can satisfy this; a mismatch requires storage inspection, not an operator command
	}
	return ValidateArtifactSubject(subject)
}

// artifactRecaptureContinuation is the continuation shared by the two subject
// echo rejections. Both are raised before AdmitArtifact returns any canonical
// lens result, and the capture command appends to the store only after
// admission succeeds, so a rejected admission leaves the immutable lens slot
// unconsumed and the store revision unmoved. Stating that once, in one place,
// is what keeps the two messages from drifting into promising different
// recoveries for the same recoverable state.
const artifactRecaptureContinuation = "the rejected admission did not consume the lens slot, " +
	"so re-run the lens and invoke gentle-ai review capture-result again on the same lineage " +
	"with a result that echoes the binding's top-level subject_hash"

// AdmitArtifact performs the single provider-owned admission decision. It
// validates subject echo, completed full-manifest inspection, result shape,
// and candidate scope before returning a canonical lens result.
func AdmitArtifact(ctx context.Context, request ArtifactAdmissionRequest) (LensResult, ArtifactAdmission, error) {
	admission := ArtifactAdmission{
		Schema: ArtifactAdmissionSchema, SubjectHash: request.ExpectedSubject.SubjectHash,
		RawSHA256: payloadSHA256(request.RawPayload), CanonicalSHA256: payloadSHA256(request.CanonicalPayload),
	}
	fail := func(decision ArtifactAdmissionDecision, diagnostic string) (LensResult, ArtifactAdmission, error) {
		admission.Decision, admission.Diagnostic = decision, diagnostic
		return LensResult{}, admission, &ArtifactAdmissionError{Admission: admission}
	}
	failFinding := func(decision ArtifactAdmissionDecision, diagnostic string, detail *ArtifactAdmissionDiagnostic, cause error) (LensResult, ArtifactAdmission, error) {
		admission.Decision, admission.Diagnostic = decision, diagnostic
		return LensResult{}, admission, &ArtifactAdmissionError{Admission: admission, Diagnostic: detail, cause: cause}
	}
	if err := ValidateArtifactSubject(request.ExpectedSubject); err != nil {
		return fail(ArtifactAdmissionBindingMismatch, err.Error())
	}
	if len(request.RawPayload) == 0 || len(request.CanonicalPayload) == 0 {
		return fail(ArtifactAdmissionIncomplete, "raw and canonical reviewer payloads are required")
	}
	if request.EchoedSubjectHash == "" {
		// Name the continuation explicitly: a rejected admission never consumes
		// the immutable lens slot, so without this guidance an operator holding
		// only the preserved incident payload has no discoverable way forward
		// (community report, PR #1801). The decision stays "incomplete" so the
		// machine-readable shape is extended, never reshaped.
		return fail(ArtifactAdmissionIncomplete,
			"reviewer result omitted the provider-owned artifact subject: "+artifactRecaptureContinuation+
				" and a completed inspection envelope")
	}
	if request.EchoedSubjectHash != request.ExpectedSubject.SubjectHash {
		// The omitted-subject sibling immediately above is the SAME block with
		// the same way out. Both are raised here, before any store append, so
		// neither consumes the immutable slot -- yet only the omission said so,
		// leaving an echoed-but-wrong subject naming the fault and no way
		// forward. The expected subject hash is handed back because it is the
		// one value the reviewer cannot derive from the failure alone; every
		// other binding_mismatch below is a frozen-context or finding-shape
		// fault whose fix is not "echo this hash", so none of them borrows this
		// wording.
		return fail(ArtifactAdmissionBindingMismatch,
			"reviewer result echoed a different artifact subject: "+artifactRecaptureContinuation+
				", which is "+request.ExpectedSubject.SubjectHash)
	}
	// Bind the candidate the way the negotiated subject schema binds it.
	// ValidateArtifactSubject above already rejected every other schema, and it
	// enforces the two shapes as mutually exclusive: a v1 subject carries a
	// candidate diff digest and blank trees, a v2 subject carries trees and no
	// digest. Comparing trees unconditionally therefore failed EVERY v1 capture,
	// because NewLegacyArtifactSubject blanks those fields on purpose to keep the
	// published v1 preimage stable. The rejection lands before any store append,
	// so the lens slot was never consumed and the collect loop re-offered the
	// same slot until it gave up.
	switch request.ExpectedSubject.Schema {
	case ArtifactSubjectSchemaV1:
		if request.FrozenContext.LegacyCandidateDiff == nil ||
			request.FrozenContext.LegacyCandidateDiff.SHA256 != request.ExpectedSubject.CandidateDiffSHA256 {
			return fail(ArtifactAdmissionBindingMismatch, "frozen candidate diff does not match the artifact subject")
		}
	default:
		if request.FrozenContext.BaseTree != request.ExpectedSubject.BaseTree ||
			request.FrozenContext.CandidateTree != request.ExpectedSubject.CandidateTree {
			return fail(ArtifactAdmissionBindingMismatch, "frozen candidate trees do not match the artifact subject")
		}
	}
	manifestDigest, err := ChangedPathManifestDigest(request.FrozenContext.ChangedPathManifest)
	if err != nil || manifestDigest != request.ExpectedSubject.ChangedPathManifestSHA256 {
		return fail(ArtifactAdmissionBindingMismatch, "frozen changed-path manifest does not match the artifact subject")
	}
	wantPaths := make([]string, len(request.FrozenContext.ChangedPathManifest))
	for index, entry := range request.FrozenContext.ChangedPathManifest {
		wantPaths[index] = entry.Path
	}
	if request.Inspection.Status != ArtifactInspectionCompleted {
		return fail(ArtifactAdmissionIncomplete, "reviewer did not report completed candidate inspection")
	}
	coverage, coverageErr := validateCompleteInspectionCoverage(request.Inspection.Paths, request.FrozenContext.ChangedPathManifest)
	if coverageErr != nil {
		if coverage == nil {
			return fail(ArtifactAdmissionOutOfScope, "reviewer inspection paths are not canonical candidate paths")
		}
		decision := ArtifactAdmissionIncomplete
		diagnostic := "reviewer inspection did not cover the complete frozen path manifest"
		if coverage.ForeignPathCount > 0 {
			decision = ArtifactAdmissionOutOfScope
			diagnostic = "reviewer inspection includes paths outside the frozen candidate"
		}
		return failFinding(decision, diagnostic, inspectionCoverageDiagnostic(coverage), coverageErr)
	}
	canonical, err := canonicalReviewerResult(request.Result, request.ExpectedSubject.Lens)
	if err != nil {
		var shapeErr *reviewerResultShapeError
		if errors.As(err, &shapeErr) {
			return fail(shapeErr.decision, shapeErr.message)
		}
		return fail(ArtifactAdmissionIncomplete, err.Error())
	}
	repository, cleanup, err := newFrozenRepositoryPathLookup(ctx, request.FrozenContext)
	if err != nil {
		return fail(ArtifactAdmissionBindingMismatch, "frozen repository path lookup is unavailable")
	}
	defer cleanup()
	resolveBasename := candidateBasenameResolver(request.FrozenContext.ChangedPathManifest)
	seenFindingIDs := make(map[string]struct{}, len(canonical.Findings))
	wantCandidateCausalIDs := make([]string, 0)
	for _, evidence := range canonical.Evidence {
		if evidenceReportsUnavailableInspection(evidence) {
			return fail(ArtifactAdmissionIncomplete, "reviewer evidence reports that candidate inspection was unavailable")
		}
		outside, offender, lookupErr := referenceOutsideRepository(evidence, repository.contains, resolveBasename)
		if lookupErr != nil {
			return fail(ArtifactAdmissionBindingMismatch, "frozen repository path lookup failed")
		}
		if outside {
			detail := findingAdmissionDiagnostic("evidence_path_out_of_scope", "", offender, "unknown_or_malformed_repository_path")
			return failFinding(ArtifactAdmissionOutOfScope, "reviewer evidence references a path outside the frozen repository",
				detail, outOfScopeCitationCause(detail.Location))
		}
	}
	for _, finding := range canonical.Findings {
		if _, duplicate := seenFindingIDs[finding.ID]; duplicate {
			return fail(ArtifactAdmissionAmbiguous, "reviewer result repeats a finding ID")
		}
		seenFindingIDs[finding.ID] = struct{}{}
		location, locationErr := parseFindingLocation(finding.Location)
		if locationErr != nil {
			var typedLocationErr *FindingLocationError
			reason := "invalid_location"
			if errors.As(locationErr, &typedLocationErr) && typedLocationErr != nil {
				reason = string(typedLocationErr.Reason)
			}
			return failFinding(ArtifactAdmissionOutOfScope, "reviewer finding location is invalid",
				findingAdmissionDiagnostic("invalid_finding_location", finding.ID, finding.Location, reason), locationErr)
		}
		if stringIndex(wantPaths, location.Path) < 0 {
			// The same citation shape reaches a finding's own location, and
			// resolving it in evidence and proofs but not here would refuse the
			// exact artifact the rest of this admission just accepted.
			resolved, unique, resolveErr := resolveBasename(location.Path)
			if resolveErr != nil {
				return fail(ArtifactAdmissionBindingMismatch, "frozen repository path lookup failed")
			}
			if !unique || stringIndex(wantPaths, resolved) < 0 {
				return fail(ArtifactAdmissionOutOfScope, "reviewer finding location is outside the frozen candidate")
			}
			// The citation is left as the reviewer wrote it. Normalizing it here
			// would rewrite the reviewer's own text for no consumer: nothing
			// downstream reads this location, and an unobserved rewrite is a
			// claim no test can hold.
		}
		for _, proof := range finding.ProofRefs {
			outside, offender, lookupErr := referenceOutsideRepository(proof, repository.contains, resolveBasename)
			if lookupErr != nil {
				return fail(ArtifactAdmissionBindingMismatch, "frozen repository path lookup failed")
			}
			if outside {
				detail := findingAdmissionDiagnostic("proof_path_out_of_scope", finding.ID, offender, "unknown_or_malformed_repository_path")
				return failFinding(ArtifactAdmissionOutOfScope, "reviewer proof references a path outside the frozen repository",
					detail, outOfScopeCitationCause(detail.Location))
			}
		}
		if !isSevereSeverity(finding.Severity) {
			continue
		}
		switch finding.CausalDisposition {
		case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
			wantCandidateCausalIDs = append(wantCandidateCausalIDs, finding.ID)
		}
	}
	wantCandidateCausalIDs, wantErr := canonicalStrings(wantCandidateCausalIDs, "candidate-causal finding id")
	if wantErr != nil {
		return fail(ArtifactAdmissionIncomplete, wantErr.Error())
	}
	verifiedIDs, err := canonicalStrings(request.CandidateCausalFindingIDs, "candidate-causal finding id")
	if err != nil {
		return fail(ArtifactAdmissionIncomplete, err.Error())
	}
	// Both sides are canonicalized before comparing: a submission that names
	// the same candidate-causal findings in a different order or with
	// non-canonical formatting must still admit, since admission persists the
	// canonical form below rather than the caller's raw bytes.
	//
	// verifiedIDs is only ever computed (review_artifact.go's
	// verifiedCandidateCausalFindingIDs) from findings that already self-claim
	// candidate causality, so it can never legitimately name an ID outside
	// wantCandidateCausalIDs. A caller that names one anyway disagrees with the
	// canonical findings about which findings even exist -- a structural
	// defect distinct from an unproven claim on a real finding -- and still
	// rejects the whole artifact.
	for _, id := range verifiedIDs {
		if stringIndex(wantCandidateCausalIDs, id) < 0 {
			return failFinding(ArtifactAdmissionOutOfScope,
				"verified candidate-causal finding IDs are not a subset of the reviewer's self-claimed candidate-causal findings",
				findingAdmissionDiagnostic("candidate_causality_unclaimed_id", id, "", "id_not_self_claimed_by_finding"), nil)
		}
	}
	// A self-claimed candidate-causal finding the caller could not verify
	// against repository-derived changed-line evidence is downgraded to
	// CausalUnknown here -- the disposition CompactReviewView's replay already
	// assigns an unverified claim (compact.go) -- rather than rejecting the
	// whole artifact for one borderline finding (#1757, #2782). The finding
	// still admits with every other finding; its outcome becomes the
	// non-blocking inconclusive follow-up path; and it can never become a
	// correction target, since ValidateCorrectionPlanRequest only accepts a
	// CausalIntroduced, CausalBehaviorActivated, or CausalWorsened
	// disposition.
	downgraded := make([]ArtifactAdmissionCausalDowngrade, 0, len(wantCandidateCausalIDs))
	downgradedIDs := make(map[string]bool, len(wantCandidateCausalIDs))
	for _, id := range wantCandidateCausalIDs {
		if stringIndex(verifiedIDs, id) < 0 {
			downgraded = append(downgraded, ArtifactAdmissionCausalDowngrade{FindingID: id, Reason: "unverified_location"})
			downgradedIDs[id] = true
		}
	}
	if len(downgraded) > 0 && request.EvidenceDerivation == EvidenceDerivationDegraded {
		// A degraded changed-line derivation cannot tell "this finding is
		// proven not caused by the candidate" from "no dependable signal could
		// be derived at all" (binary content, an untraceable rename, or
		// another gap named in the reason). Downgrading on that ambiguity
		// would silently neutralize a real blocker the exact same way the
		// old whole-artifact hard-reject did for a genuinely unproven claim --
		// so this keeps the whole-artifact rejection instead of downgrading.
		reason := strings.TrimSpace(request.EvidenceDerivationReason)
		if reason == "" {
			reason = "repository-derived changed-line evidence could not be fully resolved for this candidate"
		}
		return failFinding(ArtifactAdmissionOutOfScope,
			"candidate-causal findings cannot be downgraded because their changed-line evidence derivation is degraded: "+reason+"; "+artifactRecaptureContinuation,
			findingAdmissionDiagnostic("candidate_causality_evidence_degraded", downgraded[0].FindingID, "", "evidence_derivation_degraded"), nil)
	}
	if len(downgraded) > 0 {
		// canonical.Findings is a freshly built slice of value copies (see
		// canonicalLensResult), so rewriting a disposition here never aliases
		// the caller's original request.Result.Findings. The rewrite happens
		// before canonical.ResultHash and CanonicalSHA256 are (re)derived, so
		// the hashes -- and every persisted artifact keyed off them -- reflect
		// the downgraded disposition instead of the reviewer's unproven
		// self-claim.
		for index := range canonical.Findings {
			if downgradedIDs[canonical.Findings[index].ID] {
				canonical.Findings[index].CausalDisposition = CausalUnknown
			}
		}
		canonical.ResultHash = LensResultHash(canonical)
		canonicalPayload, payloadErr := CanonicalReviewerResultPayload(request.EchoedSubjectHash, request.Inspection, canonical)
		if payloadErr != nil {
			return fail(ArtifactAdmissionIncomplete, payloadErr.Error())
		}
		admission.CanonicalSHA256 = payloadSHA256(canonicalPayload)
	}
	admission.Decision, admission.ResultHash = ArtifactAdmissionCompleted, canonical.ResultHash
	admission.CandidateCausalFindingIDs = verifiedIDs
	if len(downgraded) > 0 {
		admission.DowngradedCausalFindings = downgraded
	}
	return canonical, admission, nil
}

func payloadSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// CanonicalReviewerResultPayload deterministically serializes one admitted
// reviewer result into the exact envelope-payload bytes (including the
// trailing newline) whose SHA-256 becomes an ArtifactAdmission's
// CanonicalSHA256. AdmitArtifact calls this to derive CanonicalSHA256 from
// the findings it actually admits -- including any it just rewrote to
// CausalUnknown -- and every consumer that needs to persist or re-verify that
// same payload (the compact store's capture path, Validate's own
// re-derivation) calls this identical function, so the bytes a constructor
// hashes and the bytes a caller persists can never independently drift.
func CanonicalReviewerResultPayload(subjectHash string, inspection ArtifactInspection, result LensResult) ([]byte, error) {
	paths, err := canonicalPaths(inspection.Paths)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(compactProviderReviewerResult{
		SubjectHash: subjectHash,
		Inspection:  ArtifactInspection{Status: inspection.Status, Paths: paths},
		Lens:        result.Lens, Findings: result.Findings, Evidence: result.Evidence,
	})
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

// ExtractBoundedSingleJSONObject accepts transport prose around exactly one
// unambiguous JSON object. Multiple objects, an unterminated object, or a
// payload outside the caller's bound fail closed with a classified decision.
func ExtractBoundedSingleJSONObject(payload []byte, limit int) ([]byte, ArtifactAdmissionDecision, error) {
	if limit <= 0 || len(payload) == 0 || len(payload) > limit {
		return nil, ArtifactAdmissionIncomplete, errors.New("reviewer payload is empty or exceeds the native bound")
	}
	type candidate struct{ start, end int }
	candidates := []candidate{}
	start, depth := -1, 0
	inString, escaped := false, false
	// The census below is what a truncated payload reports (issue #2791): a
	// bare "no complete object" could not say whether an array, a nested
	// object, or the whole payload was left open, nor where the scan stopped.
	var census jsonStructuralCensus
	for index, value := range payload {
		if depth == 0 {
			if value == '{' {
				start, depth, inString, escaped = index, 1, false, false
				census.count(index, &census.objectsOpened)
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch value {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch value {
		case '"':
			inString = true
		case '[':
			census.count(index, &census.arraysOpened)
		case ']':
			census.count(index, &census.arraysClosed)
		case '{':
			depth++
			census.count(index, &census.objectsOpened)
		case '}':
			depth--
			census.count(index, &census.objectsClosed)
			if depth == 0 {
				var object map[string]json.RawMessage
				fragment := bytes.TrimSpace(payload[start : index+1])
				if json.Unmarshal(fragment, &object) == nil && object != nil {
					candidates = append(candidates, candidate{start: start, end: index + 1})
				}
				start = -1
			}
		}
	}
	if depth != 0 || len(candidates) == 0 {
		return nil, ArtifactAdmissionIncomplete, fmt.Errorf("reviewer payload contains no complete JSON object: %s", census.describe(len(payload))) // refusal:by-design operator-knowledge: only the reviewer runtime can return one complete JSON object; the census names what it left open
	}
	if len(candidates) != 1 {
		return nil, ArtifactAdmissionAmbiguous, errors.New("reviewer payload contains multiple JSON objects")
	}
	match := candidates[0]
	return append([]byte(nil), bytes.TrimSpace(payload[match.start:match.end])...), ArtifactAdmissionCompleted, nil
}

// jsonStructuralCensus counts the structural tokens ExtractBoundedSingleJSONObject
// consumed outside strings and remembers how far the scan got, so an
// incomplete payload is refused with a diagnosis instead of a verdict.
type jsonStructuralCensus struct {
	objectsOpened, objectsClosed, arraysOpened, arraysClosed int
	lastStructuralEnd                                        int
}

func (census *jsonStructuralCensus) count(index int, counter *int) {
	*counter++
	census.lastStructuralEnd = index + 1
}

func (census jsonStructuralCensus) describe(length int) string {
	if census.objectsOpened == 0 {
		return fmt.Sprintf("no object start was found in %d bytes", length)
	}
	return fmt.Sprintf("%s opened, %d closed; %s opened, %d closed; scan ended at byte %d",
		pluralCount(census.objectsOpened, "object"), census.objectsClosed,
		pluralCount(census.arraysOpened, "array"), census.arraysClosed, census.lastStructuralEnd)
}

func pluralCount(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

var artifactFindingID = regexp.MustCompile(`^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$`)
var artifactAdmissionLocationSuffix = regexp.MustCompile(`^[A-Za-z0-9+,.-]*$`)

type artifactReferenceToken struct {
	value  string
	quoted bool
}

// frozenRepositoryPathListingLimit bounds the one full path listing the
// basename index needs. The frozen trees are a reviewed candidate, not an
// arbitrary repository, and an unbounded listing here would be a way to make
// admission allocate on a caller's schedule.
type frozenRepositoryPathLookup struct {
	ctx       context.Context
	repo      string
	isolation []string
	trees     []string
	cache     map[string]bool
}

// candidateBasenameResolver answers the one citation shape a Go-owned reviewer
// produces that a literal lookup cannot satisfy (#3042): a file of the
// candidate named by its basename rather than its repository-relative path. The
// path IS in the frozen candidate, so refusing it as outside is both wrong and
// unrecoverable -- the reviewer is locked down, so no caller can correct the
// citation and every retry reproduces it.
//
// It resolves against the frozen changed-path manifest rather than the tree:
// that is the exact set the reviewer was shown, it is bounded by construction,
// and it costs no Git call on a path that is deliberately bounded.
//
// Exactly one match resolves. Two or more do not, because choosing between them
// would attach a finding to a file the reviewer never read, which is worse than
// the refusal. A citation that already carries a directory prefix said where it
// meant, so it is not eligible: being wrong about that is not ambiguity.
func candidateBasenameResolver(manifest []ChangedPathManifestEntry) func(string) (string, bool, error) {
	index := make(map[string]string, len(manifest))
	for _, entry := range manifest {
		base := entry.Path[strings.LastIndex(entry.Path, "/")+1:]
		if base == "" || base == entry.Path {
			continue
		}
		if existing, seen := index[base]; seen && existing != entry.Path {
			index[base] = ""
			continue
		}
		index[base] = entry.Path
	}
	return func(name string) (string, bool, error) {
		if name == "" || strings.ContainsRune(name, '/') {
			return "", false, nil
		}
		resolved := index[name]
		return resolved, resolved != "", nil
	}
}

func newFrozenRepositoryPathLookup(ctx context.Context, frozen FrozenCandidateContext) (*frozenRepositoryPathLookup, func(), error) {
	if ctx == nil || frozen.repositoryRoot == "" || !validGitTree(frozen.BaseTree) || !validGitTree(frozen.CandidateTree) {
		return nil, func() {}, errors.New("frozen repository identity is incomplete") // refusal:by-design world-action: provider-owned immutable context is incomplete and must be reconstructed from authority
	}
	if err := ctx.Err(); err != nil {
		return nil, func() {}, err
	}
	isolation, cleanup, err := isolatedImmutableTreeGit(ctx, frozen.repositoryRoot)
	if err != nil {
		return nil, func() {}, err
	}
	trees := []string{frozen.BaseTree}
	if frozen.CandidateTree != frozen.BaseTree {
		trees = append(trees, frozen.CandidateTree)
	}
	return &frozenRepositoryPathLookup{
		ctx: ctx, repo: frozen.repositoryRoot, isolation: isolation, trees: trees, cache: make(map[string]bool),
	}, func() { _ = cleanup() }, nil
}

func (lookup *frozenRepositoryPathLookup) contains(logicalPath string) (bool, error) {
	if known, ok := lookup.cache[logicalPath]; ok {
		return known, nil
	}
	want := []byte(logicalPath + "\x00")
	for _, tree := range lookup.trees {
		output, err := runGitLimited(lookup.ctx, lookup.repo, lookup.isolation, nil, len(logicalPath)+len("160000 commit ")+64+2,
			"ls-tree", "-z", "--full-tree", tree, "--", ":(literal)"+logicalPath)
		if err != nil {
			return false, err
		}
		if len(output) == 0 {
			continue
		}
		header, path, found := bytes.Cut(output, []byte{'\t'})
		fields := bytes.Split(header, []byte{' '})
		if !found || !bytes.Equal(path, want) || len(fields) != 3 || !validGitTree(string(fields[2])) {
			return false, errors.New("literal repository path lookup returned a non-exact result") // refusal:by-design world-action: contradictory Git plumbing output cannot establish immutable path authority
		}
		kind := string(fields[0]) + " " + string(fields[1])
		if kind == "040000 tree" {
			continue
		}
		if kind != "100644 blob" && kind != "100755 blob" && kind != "120000 blob" && kind != "160000 commit" {
			return false, errors.New("literal repository path lookup returned an invalid file entry") // refusal:by-design world-action: contradictory Git mode and type cannot establish immutable path authority
		}
		lookup.cache[logicalPath] = true
		return true, nil
	}
	lookup.cache[logicalPath] = false
	return false, nil
}

// referenceOutsideRepository recognizes canonical path:positive-line tokens
// and requires each one to exist in the immutable base/candidate repository
// universe. Bare root names need a dot; extensionless root paths remain
// available through quoting. This keeps status:500 and digest/timestamp labels
// out of the path grammar while rejecting malformed or unknown path claims.
// The offender return names the first malformed or unknown token verbatim so
// a rejection is diagnosable after the fact; detection semantics are
// unchanged.
func referenceOutsideRepository(value string, lookup func(string) (bool, error), resolveBasename func(string) (string, bool, error)) (outside bool, offender string, err error) {
	for _, token := range artifactReferenceTokens(value) {
		path, malformed := artifactRepositoryPathReference(token)
		if malformed {
			return true, token.value, nil
		}
		if path == "" {
			continue
		}
		known, err := lookup(path)
		if err != nil {
			return false, "", err
		}
		if known {
			continue
		}
		// A literal miss is not yet proof the citation left the candidate: it
		// may be a bare basename that exactly one candidate path answers.
		resolved, unique, err := resolveBasename(path)
		if err != nil {
			return false, "", err
		}
		if !unique {
			return true, token.value, nil
		}
		known, err = lookup(resolved)
		if err != nil {
			return false, "", err
		}
		if !known {
			return true, token.value, nil
		}
	}
	return false, "", nil
}

// outOfScopeCitationCause names the already-scrubbed offending citation in the
// admission error chain. It receives the bounded safeAdmissionLocation output,
// never the raw token, so an unsafe token degrades to the generic message.
func outOfScopeCitationCause(safeToken string) error {
	if safeToken == "" {
		return errors.New("reviewer citation names an unknown or malformed repository path") // refusal:by-design world-action: the reviewer's free text cited a path the frozen repository does not contain; only a re-run lens with corrected citations can continue
	}
	return fmt.Errorf("reviewer citation %q names an unknown or malformed repository path", safeToken) // refusal:by-design world-action: the reviewer's free text cited a path the frozen repository does not contain; only a re-run lens with corrected citations can continue
}

func artifactRepositoryPathReference(token artifactReferenceToken) (string, bool) {
	value := token.value
	if !token.quoted {
		value = strings.TrimLeft(value, "([{<")
		value = strings.TrimRight(value, ")]}>.,;!?")
	}
	separator := strings.LastIndexByte(value, ':')
	if separator <= 0 || separator == len(value)-1 {
		return "", false
	}
	line := value[separator+1:]
	nonzero := false
	for index := range line {
		if line[index] < '0' || line[index] > '9' {
			return "", false
		}
		nonzero = nonzero || line[index] != '0'
	}
	if !nonzero {
		return "", false
	}
	logicalPath := value[:separator]
	if strings.Contains(logicalPath, "://") {
		return "", false
	}
	pathLike := token.quoted || strings.Contains(logicalPath, "/") || strings.Contains(logicalPath, ".")
	if !pathLike {
		return "", false
	}
	canonical, err := normalizeLogicalPath(logicalPath)
	if err != nil || canonical != logicalPath {
		return "", true
	}
	return canonical, false
}

func artifactReferenceTokens(value string) []artifactReferenceToken {
	tokens := make([]artifactReferenceToken, 0)
	start := -1
	flush := func(end int) {
		if start >= 0 && start < end {
			tokens = append(tokens, artifactReferenceToken{value: value[start:end]})
		}
		start = -1
	}
	for index := 0; index < len(value); {
		r, size := utf8.DecodeRuneInString(value[index:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if r == '\'' || r == '"' || r == '`' {
			flush(index)
			closing := strings.IndexRune(value[index+size:], r)
			if closing < 0 {
				index += size
				continue
			}
			begin := index + size
			end := begin + closing
			if begin < end {
				tokens = append(tokens, artifactReferenceToken{value: value[begin:end], quoted: true})
			}
			index = end + size
			continue
		}
		if unicode.IsSpace(r) {
			flush(index)
			index += size
			continue
		}
		if start < 0 {
			start = index
		}
		index += size
	}
	flush(len(value))
	return tokens
}

// InconclusiveValidationEvidence reports whether a scoped-fix validation
// check's evidence claims the immutable candidate could not be inspected.
// Such a check carries no verdict in either direction: admitting it as
// failed would consume the single correction attempt on a non-observation,
// and admitting it as passed would approve without inspection.
func InconclusiveValidationEvidence(evidence []string) bool {
	for _, line := range evidence {
		if evidenceReportsUnavailableInspection(line) {
			return true
		}
	}
	return false
}

func evidenceReportsUnavailableInspection(value string) bool {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	for _, phrase := range []string{
		"inspection blocked", "inspection was blocked", "access denied", "permission denied",
		"candidate unavailable", "candidate was unavailable", "immutable candidate unavailable",
		"could not inspect", "unable to inspect", "was not inspected", "not inspected",
		// Passive constructions (issue #3378). "not inspected" cannot cover
		// these: every one of them puts an auxiliary verb between the two
		// words. They are still narrow enough that a completed inspection
		// reporting a real defect ("Inspected the tree: the loop still stops
		// one entry short") never matches.
		"could not be inspected", "cannot be inspected", "can not be inspected",
		"was not able to be inspected", "were not able to be inspected",
		"no candidate contents were available", "no candidate content was available",
		// Read failures reported as such (issue #1867). "read the manifest"
		// alone would match a completed inspection, so every phrase here
		// carries its own negation.
		"read denied", "denied by filesystem", "cannot read diff", "cannot read manifest",
		"could not read the candidate", "unable to read the candidate",
	} {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}
