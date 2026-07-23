package reviewtransaction

import (
	"bytes"
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

// ArtifactAdmission records the provider's decision and exact raw/canonical
// payload identities. Only completed records are reviewer results.
type ArtifactAdmission struct {
	Schema                    string                    `json:"schema"`
	Decision                  ArtifactAdmissionDecision `json:"decision"`
	SubjectHash               string                    `json:"subject_hash"`
	RawSHA256                 string                    `json:"raw_sha256"`
	CanonicalSHA256           string                    `json:"canonical_sha256"`
	ResultHash                string                    `json:"result_hash,omitempty"`
	CandidateCausalFindingIDs []string                  `json:"candidate_causal_finding_ids"`
	Diagnostic                string                    `json:"diagnostic,omitempty"`
}

type ArtifactAdmissionRequest struct {
	ExpectedSubject   ArtifactSubject
	FrozenContext     FrozenCandidateContext
	EchoedSubjectHash string
	Inspection        ArtifactInspection
	Result            LensResult
	CanonicalResult   *CanonicalArtifactLensResult
	// CandidateCausalFindingIDs is the canonical set whose claimed candidate
	// causality the provider verified against repository-derived changed-line
	// evidence before admission.
	CandidateCausalFindingIDs []string
	RawPayload                []byte
	CanonicalPayload          []byte
}

// CanonicalArtifactLensResult carries one provider-canonicalized result across
// repository causal verification and artifact admission without regenerating
// finding identities at either boundary.
type CanonicalArtifactLensResult struct {
	result LensResult
	valid  bool
}

func NewCanonicalArtifactLensResult(result LensResult) (CanonicalArtifactLensResult, error) {
	canonical, err := CanonicalCompactLensResult(result)
	if err != nil {
		return CanonicalArtifactLensResult{}, err
	}
	if err := validateCanonicalArtifactFindingIDs(canonical); err != nil {
		return CanonicalArtifactLensResult{}, err
	}
	return CanonicalArtifactLensResult{result: cloneArtifactLensResult(canonical), valid: true}, nil
}

func (canonical CanonicalArtifactLensResult) Result() LensResult {
	return cloneArtifactLensResult(canonical.result)
}

// ArtifactAdmissionError exposes the stable native decision without requiring
// callers to parse diagnostic prose.
type ArtifactAdmissionError struct {
	Admission ArtifactAdmission
}

func (err *ArtifactAdmissionError) Error() string {
	return fmt.Sprintf("reviewer artifact admission %s: %s", err.Admission.Decision, err.Admission.Diagnostic)
}

func (admission ArtifactAdmission) Validate(subject ArtifactSubject) error {
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
	return ValidateArtifactSubject(subject)
}

// AdmitArtifact performs the single provider-owned admission decision. It
// validates subject echo, completed full-manifest inspection, result shape,
// and candidate scope before returning a canonical lens result.
func AdmitArtifact(request ArtifactAdmissionRequest) (LensResult, ArtifactAdmission, error) {
	admission := ArtifactAdmission{
		Schema: ArtifactAdmissionSchema, SubjectHash: request.ExpectedSubject.SubjectHash,
		RawSHA256: payloadSHA256(request.RawPayload), CanonicalSHA256: payloadSHA256(request.CanonicalPayload),
	}
	fail := func(decision ArtifactAdmissionDecision, diagnostic string) (LensResult, ArtifactAdmission, error) {
		admission.Decision, admission.Diagnostic = decision, diagnostic
		return LensResult{}, admission, &ArtifactAdmissionError{Admission: admission}
	}
	if err := ValidateArtifactSubject(request.ExpectedSubject); err != nil {
		return fail(ArtifactAdmissionBindingMismatch, err.Error())
	}
	if len(request.RawPayload) == 0 || len(request.CanonicalPayload) == 0 {
		return fail(ArtifactAdmissionIncomplete, "raw and canonical reviewer payloads are required")
	}
	if request.EchoedSubjectHash == "" {
		return fail(ArtifactAdmissionIncomplete, "reviewer result omitted the provider-owned artifact subject")
	}
	if request.EchoedSubjectHash != request.ExpectedSubject.SubjectHash {
		return fail(ArtifactAdmissionBindingMismatch, "reviewer result echoed a different artifact subject")
	}
	if _, err := request.FrozenContext.CandidateDiff.Bytes(); err != nil || request.FrozenContext.CandidateDiff.SHA256 != request.ExpectedSubject.CandidateDiffSHA256 {
		return fail(ArtifactAdmissionBindingMismatch, "frozen candidate diff does not match the artifact subject")
	}
	manifestDigest, err := ChangedPathManifestDigest(request.FrozenContext.ChangedPathManifest)
	if err != nil || manifestDigest != request.ExpectedSubject.ChangedPathManifestSHA256 {
		return fail(ArtifactAdmissionBindingMismatch, "frozen changed-path manifest does not match the artifact subject")
	}
	wantPaths := make([]string, len(request.FrozenContext.ChangedPathManifest))
	allowed := make(map[string]struct{}, len(wantPaths))
	for index, entry := range request.FrozenContext.ChangedPathManifest {
		wantPaths[index] = entry.Path
		allowed[entry.Path] = struct{}{}
	}
	repositoryPaths, err := canonicalPaths(request.FrozenContext.repositoryPaths)
	if err != nil || request.FrozenContext.repositoryPaths == nil || !equalStrings(repositoryPaths, request.FrozenContext.repositoryPaths) {
		return fail(ArtifactAdmissionBindingMismatch, "frozen repository path manifest is missing or non-canonical")
	}
	repository := make(map[string]struct{}, len(repositoryPaths))
	for _, logicalPath := range repositoryPaths {
		repository[logicalPath] = struct{}{}
	}
	for _, logicalPath := range wantPaths {
		if _, ok := repository[logicalPath]; !ok {
			return fail(ArtifactAdmissionBindingMismatch, "frozen changed path is absent from the repository path manifest")
		}
	}
	if request.Inspection.Status != ArtifactInspectionCompleted {
		return fail(ArtifactAdmissionIncomplete, "reviewer did not report completed candidate inspection")
	}
	inspectionPaths, err := canonicalPaths(request.Inspection.Paths)
	if err != nil || !equalStrings(inspectionPaths, request.Inspection.Paths) {
		return fail(ArtifactAdmissionOutOfScope, "reviewer inspection paths are not canonical candidate paths")
	}
	for _, path := range inspectionPaths {
		if _, ok := allowed[path]; !ok {
			return fail(ArtifactAdmissionOutOfScope, "reviewer inspection includes a path outside the frozen candidate")
		}
	}
	if !equalStrings(inspectionPaths, wantPaths) {
		return fail(ArtifactAdmissionIncomplete, "reviewer inspection did not cover the complete frozen path manifest")
	}
	canonical := LensResult{}
	if request.CanonicalResult != nil {
		if !request.CanonicalResult.valid {
			return fail(ArtifactAdmissionIncomplete, "canonical reviewer result is invalid")
		}
		canonical = request.CanonicalResult.Result()
	} else {
		canonical, err = CanonicalCompactLensResult(request.Result)
		if err != nil {
			return fail(ArtifactAdmissionIncomplete, err.Error())
		}
	}
	wantPrefix := map[string]string{LensRisk: "R1-", LensReadability: "R2-", LensReliability: "R3-", LensResilience: "R4-"}[canonical.Lens]
	seenFindingIDs := make(map[string]struct{}, len(canonical.Findings))
	wantCandidateCausalIDs := make([]string, 0)
	for _, evidence := range canonical.Evidence {
		if evidenceReportsUnavailableInspection(evidence) {
			return fail(ArtifactAdmissionIncomplete, "reviewer evidence reports that candidate inspection was unavailable")
		}
		if referenceOutsideScope(evidence, allowed, repository) {
			return fail(ArtifactAdmissionOutOfScope, "reviewer evidence references a path outside the frozen candidate")
		}
	}
	for _, finding := range canonical.Findings {
		if !artifactFindingID.MatchString(finding.ID) {
			return fail(ArtifactAdmissionBindingMismatch, "reviewer finding ID does not match the native ASCII schema")
		}
		if !strings.HasPrefix(finding.ID, wantPrefix) {
			return fail(ArtifactAdmissionBindingMismatch, "reviewer finding ID is not bound to the selected lens")
		}
		if _, duplicate := seenFindingIDs[finding.ID]; duplicate {
			return fail(ArtifactAdmissionAmbiguous, "reviewer result repeats a finding ID")
		}
		seenFindingIDs[finding.ID] = struct{}{}
		if !findingLocationInGenesis(finding.Location, wantPaths) {
			return fail(ArtifactAdmissionOutOfScope, "reviewer finding location is outside the frozen candidate")
		}
		for _, proof := range finding.ProofRefs {
			if referenceOutsideScope(proof, allowed, repository) {
				return fail(ArtifactAdmissionOutOfScope, "reviewer proof references a path outside the frozen candidate")
			}
		}
		if !isSevereSeverity(finding.Severity) {
			continue
		}
		if !isSupportedEvidenceClass(finding.EvidenceClass) || !isSupportedCausalDisposition(finding.CausalDisposition) {
			return fail(ArtifactAdmissionIncomplete, "severe reviewer finding requires supported evidence_class and causal_disposition")
		}
		switch finding.CausalDisposition {
		case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
			wantCandidateCausalIDs = append(wantCandidateCausalIDs, finding.ID)
		}
	}
	wantCandidateCausalIDs, wantErr := canonicalStrings(wantCandidateCausalIDs, "candidate-causal finding id")
	verifiedIDs, err := canonicalStrings(request.CandidateCausalFindingIDs, "candidate-causal finding id")
	if wantErr != nil || err != nil || !equalStrings(verifiedIDs, request.CandidateCausalFindingIDs) || !equalStrings(verifiedIDs, wantCandidateCausalIDs) {
		return fail(ArtifactAdmissionOutOfScope, "candidate-causal findings are not proven by repository-derived changed-line evidence")
	}
	admission.Decision, admission.ResultHash = ArtifactAdmissionCompleted, canonical.ResultHash
	admission.CandidateCausalFindingIDs = verifiedIDs
	return canonical, admission, nil
}

func cloneArtifactLensResult(result LensResult) LensResult {
	clone := result
	clone.Findings = append([]Finding(nil), result.Findings...)
	for index := range clone.Findings {
		clone.Findings[index].ProofRefs = append([]string(nil), result.Findings[index].ProofRefs...)
	}
	clone.Evidence = append([]string(nil), result.Evidence...)
	return clone
}

func validateCanonicalArtifactFindingIDs(result LensResult) error {
	wantPrefix := map[string]string{LensRisk: "R1-", LensReadability: "R2-", LensReliability: "R3-", LensResilience: "R4-"}[result.Lens]
	seen := make(map[string]struct{}, len(result.Findings))
	for index, finding := range result.Findings {
		if !artifactFindingID.MatchString(finding.ID) {
			return fmt.Errorf("lens result finding[%d] ID does not match the native ASCII schema", index)
		}
		if !strings.HasPrefix(finding.ID, wantPrefix) {
			return fmt.Errorf("lens result finding[%d] ID is not bound to %q", index, result.Lens)
		}
		if _, duplicate := seen[finding.ID]; duplicate {
			return fmt.Errorf("lens result finding[%d] repeats finding ID %q", index, finding.ID)
		}
		seen[finding.ID] = struct{}{}
	}
	return nil
}

func payloadSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
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
	for index, value := range payload {
		if depth == 0 {
			if value == '{' {
				start, depth, inString, escaped = index, 1, false, false
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
		case '{':
			depth++
		case '}':
			depth--
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
		return nil, ArtifactAdmissionIncomplete, errors.New("reviewer payload contains no complete JSON object")
	}
	if len(candidates) != 1 {
		return nil, ArtifactAdmissionAmbiguous, errors.New("reviewer payload contains multiple JSON objects")
	}
	match := candidates[0]
	return append([]byte(nil), bytes.TrimSpace(payload[match.start:match.end])...), ArtifactAdmissionCompleted, nil
}

var artifactFindingID = regexp.MustCompile(`^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$`)

type artifactReferenceToken struct {
	value  string
	quoted bool
}

// referenceOutsideScope recognizes only canonical path:positive-line tokens
// that name an immutable base/candidate repository path. Bare root names need
// a dot; extensionless root paths remain available through quoting. This keeps
// status:500 and digest/timestamp labels out of the path grammar while still
// supporting nested, Unicode, and quoted-space Git paths.
func referenceOutsideScope(value string, allowed, repository map[string]struct{}) bool {
	for _, token := range artifactReferenceTokens(value) {
		path, known, malformed := artifactRepositoryPathReference(token, repository)
		if malformed {
			return true
		}
		if !known {
			continue
		}
		if _, ok := allowed[path]; !ok {
			return true
		}
	}
	return false
}

func artifactRepositoryPathReference(token artifactReferenceToken, repository map[string]struct{}) (string, bool, bool) {
	value := token.value
	if !token.quoted {
		value = strings.TrimLeft(value, "([{<")
		value = strings.TrimRight(value, ")]}>.,;!?")
	}
	separator := strings.LastIndexByte(value, ':')
	if separator <= 0 || separator == len(value)-1 {
		return "", false, false
	}
	line := value[separator+1:]
	nonzero := false
	for index := range line {
		if line[index] < '0' || line[index] > '9' {
			return "", false, false
		}
		nonzero = nonzero || line[index] != '0'
	}
	if !nonzero {
		return "", false, false
	}
	logicalPath := value[:separator]
	if strings.Contains(logicalPath, "://") {
		return "", false, false
	}
	pathLike := token.quoted || strings.Contains(logicalPath, "/") || strings.Contains(logicalPath, ".")
	if !pathLike {
		return "", false, false
	}
	canonical, err := normalizeLogicalPath(logicalPath)
	if err != nil || canonical != logicalPath {
		return "", false, true
	}
	_, known := repository[canonical]
	return canonical, known, false
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

func evidenceReportsUnavailableInspection(value string) bool {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	for _, phrase := range []string{
		"inspection blocked", "inspection was blocked", "access denied", "permission denied",
		"candidate unavailable", "candidate was unavailable", "immutable candidate unavailable",
		"could not inspect", "unable to inspect", "was not inspected", "not inspected",
		"no candidate contents were available", "no candidate content was available",
	} {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}
