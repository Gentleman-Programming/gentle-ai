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
	"sort"
	"strconv"
	"strings"
)

// ProviderCausalClassification is derived from the frozen base/candidate
// trees. The classification field on ProviderCausalEvidence is intentionally
// only a reviewer claim and is never consulted by the provider.
type ProviderCausalClassification string

const (
	ProviderCandidateCausal    ProviderCausalClassification = "candidate-causal"
	ProviderProvenNonCandidate ProviderCausalClassification = "proven-non-candidate"
	ProviderUnknown            ProviderCausalClassification = "unknown"
)

// ProviderCausalEvidence is the reviewer-facing claim accepted at native
// capture. Location and proof references are claims; Classification is not an
// authority input. The provider derives the persisted classification.
type ProviderCausalEvidence struct {
	FindingID      string                       `json:"finding_id"`
	Location       string                       `json:"location"`
	ProofRefs      []string                     `json:"proof_refs"`
	Classification ProviderCausalClassification `json:"classification,omitempty"`
}

type ProviderCausalFinding struct {
	FindingID      string                       `json:"finding_id"`
	Location       string                       `json:"location"`
	ProofRefs      []string                     `json:"proof_refs"`
	Classification ProviderCausalClassification `json:"classification"`
	EvidenceDigest string                       `json:"evidence_digest"`
}

type ProviderCausalCarrier struct {
	SubjectHash       string                  `json:"subject_hash"`
	CandidateIdentity CandidateIdentity       `json:"candidate_identity"`
	Findings          []ProviderCausalFinding `json:"findings"`
	AggregateDigest   string                  `json:"aggregate_digest"`
}

var (
	ErrProviderCausalCarrierMissing  = errors.New("new-lineage provider causal carrier is missing")                     // refusal:by-design operator-knowledge: a missing persisted carrier requires fresh capture and cannot be safely inferred at finalize
	ErrProviderCausalCarrierConflict = errors.New("new-lineage provider causal classifications conflict across lenses") // refusal:by-design operator-knowledge: conflicting provider authority requires fresh capture and cannot be resolved by finalize
)

func (carrier ProviderCausalCarrier) Validate() error {
	if carrier.SubjectHash == "" || !validSHA256(carrier.SubjectHash) {
		return errors.New("provider causal carrier requires a canonical subject hash") // refusal:by-design operator-knowledge: malformed persisted provider authority is an in-process integrity failure, not an operator-repairable state
	}
	seen := make(map[string]bool, len(carrier.Findings))
	for _, finding := range carrier.Findings {
		if finding.FindingID == "" || seen[finding.FindingID] {
			return errors.New("provider causal carrier finding ids must be non-empty and unique") // refusal:by-design operator-knowledge: provider findings are canonicalized before persistence and malformed authority cannot be repaired safely in place
		}
		seen[finding.FindingID] = true
		switch finding.Classification {
		case ProviderCandidateCausal, ProviderProvenNonCandidate, ProviderUnknown:
		default:
			return errors.New("provider causal carrier classification is unsupported") // refusal:by-design operator-knowledge: the provider classification domain is closed and unsupported persisted values indicate in-process corruption
		}
		if finding.EvidenceDigest != providerFindingDigest(finding) {
			return errors.New("provider causal carrier finding digest does not match its content") // refusal:by-design operator-knowledge: a mismatched digest proves persisted authority corruption and must not be repaired by an operator
		}
	}
	if carrier.AggregateDigest != providerAggregateDigest(carrier) {
		return errors.New("provider causal carrier aggregate digest does not match its findings") // refusal:by-design operator-knowledge: aggregate integrity failure requires a fresh provider capture, not in-place operator repair
	}
	return nil
}

// ProviderCausalAdmission consumes only persisted provider classifications.
// It returns candidate-causal IDs in canonical order and escalates unknown
// classifications; proven-non-candidate findings are deliberately ignored.
func (authority NewLineageAuthority) ProviderCausalAdmission() (admittedIDs []string, unknown bool, err error) {
	byLens := make(map[string]ProviderCausalCarrier, len(authority.CapturedResults))
	selected := make(map[string]struct{}, len(authority.SelectedLenses))
	for _, lens := range authority.SelectedLenses {
		selected[lens] = struct{}{}
	}
	for _, captured := range authority.CapturedResults {
		if _, ok := selected[captured.Lens]; !ok {
			return nil, false, fmt.Errorf("%w for unselected lens %q", ErrProviderCausalCarrierConflict, captured.Lens)
		}
		if _, exists := byLens[captured.Lens]; exists {
			return nil, false, fmt.Errorf("%w for duplicate lens %q", ErrProviderCausalCarrierConflict, captured.Lens)
		}
		if captured.Provider.SubjectHash == "" {
			return nil, false, fmt.Errorf("%w for lens %q", ErrProviderCausalCarrierMissing, captured.Lens)
		}
		if err := captured.Provider.Validate(); err != nil {
			return nil, false, fmt.Errorf("invalid provider causal carrier for lens %q: %w", captured.Lens, err)
		}
		if captured.Provider.SubjectHash != captured.SubjectHash || captured.Provider.CandidateIdentity != authority.CandidateIdentity {
			return nil, false, fmt.Errorf("invalid provider causal carrier binding for lens %q", captured.Lens) // refusal:by-design operator-knowledge: a binding mismatch proves persisted authority corruption
		}
		byLens[captured.Lens] = captured.Provider
	}
	classifications := make(map[string]ProviderCausalClassification)
	for _, lens := range authority.SelectedLenses {
		carrier, ok := byLens[lens]
		if !ok {
			return nil, false, fmt.Errorf("%w for lens %q", ErrProviderCausalCarrierMissing, lens)
		}
		for _, finding := range carrier.Findings {
			if previous, exists := classifications[finding.FindingID]; exists && previous != finding.Classification {
				return nil, false, fmt.Errorf("%w for finding %q", ErrProviderCausalCarrierConflict, finding.FindingID)
			}
			classifications[finding.FindingID] = finding.Classification
			switch finding.Classification {
			case ProviderCandidateCausal:
				admittedIDs = append(admittedIDs, finding.FindingID)
			case ProviderUnknown:
				unknown = true
			case ProviderProvenNonCandidate:
			default:
				return nil, false, fmt.Errorf("%w: unsupported classification for finding %q", ErrProviderCausalCarrierConflict, finding.FindingID)
			}
		}
	}
	seen := make(map[string]struct{}, len(admittedIDs))
	result := admittedIDs[:0]
	for _, id := range admittedIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	return result, unknown, nil
}

// ProviderCausalCarrierDigest binds every persisted carrier, including its
// lens binding, into a deterministic value that does not depend on the
// content-addressed authority revision. Keeping this primitive separate avoids
// the circular dependency that would result from hashing the revision into the
// authority whose revision is being computed.
func (authority NewLineageAuthority) ProviderCausalCarrierDigest() (string, error) {
	type entry struct {
		Lens, SubjectHash, AggregateDigest string
	}
	entries := make([]entry, 0, len(authority.CapturedResults))
	for _, captured := range authority.CapturedResults {
		if err := captured.Provider.Validate(); err != nil {
			return "", err
		}
		if captured.Provider.SubjectHash != captured.SubjectHash || captured.Provider.CandidateIdentity != authority.CandidateIdentity {
			return "", errors.New("provider causal authority aggregate has an invalid carrier binding") // refusal:by-design operator-knowledge: replay must use the exact persisted provider binding
		}
		entries = append(entries, entry{captured.Lens, captured.SubjectHash, captured.Provider.AggregateDigest})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Lens < entries[j].Lens })
	payload, _ := json.Marshal(struct {
		Candidate CandidateIdentity `json:"candidate_identity"`
		Carriers  []entry           `json:"carriers"`
	}{authority.CandidateIdentity, entries})
	sum := sha256.Sum256(append([]byte("gentle-ai.provider-causal-authority/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ProviderCausalAggregateDigest is the receipt/replay identity. It binds the
// non-circular carrier digest to the exact authority revision that authorized
// it. The revision is an input here, never a field in the authority preimage.
func (authority NewLineageAuthority) ProviderCausalAggregateDigest(authorityRevision string) (string, error) {
	if !validSHA256(authorityRevision) {
		return "", errors.New("provider causal aggregate requires a valid authority revision") // refusal:by-design operator-knowledge: an invalid authority revision cannot safely authorize a replay aggregate and requires fresh authority capture
	}
	carrierDigest, err := authority.ProviderCausalCarrierDigest()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		AuthorityRevision string `json:"authority_revision"`
		CarrierDigest     string `json:"provider_carrier_digest"`
	}{authorityRevision, carrierDigest})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("gentle-ai.provider-causal-aggregate/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func canonicalProviderProofRefs(refs []string) []string {
	seen := make(map[string]struct{}, len(refs))
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		result = append(result, ref)
	}
	sort.Strings(result)
	return result
}

func canonicalProviderClaims(claims []ProviderCausalEvidence) ([]ProviderCausalEvidence, error) {
	result := append([]ProviderCausalEvidence(nil), claims...)
	for i := range result {
		result[i].FindingID = strings.TrimSpace(result[i].FindingID)
		result[i].Location = strings.TrimSpace(result[i].Location)
		result[i].ProofRefs = canonicalProviderProofRefs(result[i].ProofRefs)
		if result[i].FindingID == "" {
			return nil, errors.New("provider causal evidence requires a finding id") // refusal:by-design operator-knowledge: a reviewer payload without a stable finding identity cannot be admitted or repaired by the provider
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FindingID != result[j].FindingID {
			return result[i].FindingID < result[j].FindingID
		}
		return result[i].Location < result[j].Location
	})
	deduped := make([]ProviderCausalEvidence, 0, len(result))
	for _, claim := range result {
		if len(deduped) == 0 || deduped[len(deduped)-1].FindingID != claim.FindingID {
			deduped = append(deduped, claim)
			continue
		}
		previous := deduped[len(deduped)-1]
		if previous.Location != claim.Location || !equalStrings(previous.ProofRefs, claim.ProofRefs) {
			return nil, fmt.Errorf("provider causal evidence contains conflicting duplicate finding id %q", claim.FindingID) // refusal:by-design operator-knowledge: conflicting claims require reviewer correction and cannot be safely resolved by capture
		}
	}
	return deduped, nil
}

func providerFindingDigest(finding ProviderCausalFinding) string {
	payload, _ := json.Marshal(struct {
		FindingID, Location string
		ProofRefs           []string
		Classification      ProviderCausalClassification
	}{finding.FindingID, finding.Location, finding.ProofRefs, finding.Classification})
	sum := sha256.Sum256(append([]byte("gentle-ai.provider-causal-finding/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func providerAggregateDigest(carrier ProviderCausalCarrier) string {
	parts := make([]string, len(carrier.Findings))
	for i, finding := range carrier.Findings {
		parts[i] = finding.EvidenceDigest
	}
	payload, _ := json.Marshal(struct {
		SubjectHash string
		Candidate   CandidateIdentity
		Findings    []string
	}{carrier.SubjectHash, carrier.CandidateIdentity, parts})
	sum := sha256.Sum256(append([]byte("gentle-ai.provider-causal-aggregate/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// DeriveProviderCausalCarrier is the sole classification authority for native
// new-lineage capture. It uses the frozen Git trees, never the reviewer's
// Classification claim. A changed target line is candidate-causal; an equal
// base/candidate line is proven non-candidate. All other cases are unknown.
func DeriveProviderCausalCarrier(ctx context.Context, repo, subjectHash string, candidate CandidateIdentity, claims []ProviderCausalEvidence) (ProviderCausalCarrier, error) {
	if !validSHA256(subjectHash) || !validGitTree(candidate.BaseTree) || !validGitTree(candidate.CandidateTree) {
		return ProviderCausalCarrier{}, errors.New("provider causal derivation requires a valid subject and frozen candidate trees") // refusal:by-design operator-knowledge: capture must restart from a fresh provider-issued frozen binding
	}
	canonical, err := canonicalProviderClaims(claims)
	if err != nil {
		return ProviderCausalCarrier{}, err
	}
	result := ProviderCausalCarrier{SubjectHash: subjectHash, CandidateIdentity: candidate, Findings: make([]ProviderCausalFinding, 0, len(canonical))}
	for _, claim := range canonical {
		classification := ProviderUnknown
		if claim.Location != "" {
			proofValid := frozenProofRefsValid(ctx, repo, candidate, claim.ProofRefs)
			if changed, checkErr := frozenCandidateLineChanged(ctx, repo, candidate, claim.Location); checkErr == nil && changed && proofValid {
				classification = ProviderCandidateCausal
			} else if equal, checkErr := frozenWholePathEqual(ctx, repo, candidate, claim.Location); checkErr == nil && equal {
				classification = ProviderProvenNonCandidate
			}
		}
		finding := ProviderCausalFinding{FindingID: claim.FindingID, Location: claim.Location, ProofRefs: claim.ProofRefs, Classification: classification}
		finding.EvidenceDigest = providerFindingDigest(finding)
		result.Findings = append(result.Findings, finding)
	}
	result.AggregateDigest = providerAggregateDigest(result)
	return result, nil
}

func frozenProofRefsValid(ctx context.Context, repo string, candidate CandidateIdentity, refs []string) bool {
	for _, ref := range refs {
		path, line, err := parseFindingLocation(ref)
		if err != nil {
			continue // Free-form refs remain claims; location claims are checked below.
		}
		if line <= 0 {
			return false
		}
		for _, tree := range []string{candidate.BaseTree, candidate.CandidateTree} {
			blob, showErr := runGit(ctx, repo, nil, nil, "show", tree+":"+path)
			if showErr == nil && line <= len(strings.Split(strings.TrimSuffix(string(blob), "\n"), "\n")) {
				goto nextRef
			}
		}
		return false
	nextRef:
	}
	return true
}

func frozenCandidateLineChanged(ctx context.Context, repo string, candidate CandidateIdentity, location string) (bool, error) {
	path, line, err := parseFindingLocation(location)
	if err != nil {
		return false, err
	}
	output, err := runGit(ctx, repo, nil, nil, "diff", "--unified=0", "--no-renames", "--no-ext-diff", "--no-textconv", candidate.BaseTree, candidate.CandidateTree, "--", literalPathspec(path))
	if err != nil {
		return false, err
	}
	for _, match := range regexp.MustCompile(`(?m)^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`).FindAllSubmatch(output, -1) {
		start, _ := strconv.Atoi(string(match[1]))
		count := 1
		if len(match[2]) > 0 {
			count, _ = strconv.Atoi(string(match[2]))
		}
		if count > 0 && line >= start && line < start+count {
			return true, nil
		}
	}
	return false, nil
}

func frozenWholePathEqual(ctx context.Context, repo string, candidate CandidateIdentity, location string) (bool, error) {
	path, line, err := parseFindingLocation(location)
	if err != nil {
		return false, err
	}
	base, err := runGit(ctx, repo, nil, nil, "show", candidate.BaseTree+":"+path)
	if err != nil {
		return false, nil
	}
	next, err := runGit(ctx, repo, nil, nil, "show", candidate.CandidateTree+":"+path)
	if err != nil {
		return false, nil
	}
	baseLines := strings.Split(string(base), "\n")
	nextLines := strings.Split(string(next), "\n")
	if len(baseLines) > 0 && baseLines[len(baseLines)-1] == "" {
		baseLines = baseLines[:len(baseLines)-1]
	}
	if len(nextLines) > 0 && nextLines[len(nextLines)-1] == "" {
		nextLines = nextLines[:len(nextLines)-1]
	}
	return line > 0 && line <= len(baseLines) && line <= len(nextLines) && bytes.Equal(base, next), nil
}
