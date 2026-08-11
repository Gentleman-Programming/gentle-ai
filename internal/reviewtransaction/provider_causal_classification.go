package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type ProviderCausalClassification string

const (
	ProviderCandidateCausal    ProviderCausalClassification = "candidate-causal"
	ProviderProvenNonCandidate ProviderCausalClassification = "proven-non-candidate"
	ProviderUnknown            ProviderCausalClassification = "unknown"
)

// ProviderCausalEvidence contains reviewer claims. Classification is ignored;
// the frozen Git trees are the authority used to derive the carrier.
type ProviderCausalEvidence struct {
	FindingID          string            `json:"finding_id"`
	Location           string            `json:"location"`
	ProofRefs          []string          `json:"proof_refs"`
	CausalDisposition  CausalDisposition `json:"causal_disposition,omitempty"`
	BaseProofRefs      []string          `json:"base_proof_refs,omitempty"`
	CandidateProofRefs []string          `json:"candidate_proof_refs,omitempty"`
}

type ProviderCausalFinding struct {
	FindingID          string                       `json:"finding_id"`
	Location           string                       `json:"location"`
	ProofRefs          []string                     `json:"proof_refs"`
	BaseProofRefs      []string                     `json:"base_proof_refs,omitempty"`
	CandidateProofRefs []string                     `json:"candidate_proof_refs,omitempty"`
	Classification     ProviderCausalClassification `json:"classification"`
	EvidenceDigest     string                       `json:"evidence_digest"`
}

type ProviderCausalCarrier struct {
	SubjectHash       string                    `json:"subject_hash"`
	CandidateIdentity CandidateIdentity         `json:"candidate_identity"`
	ArtifactBinding   NewLineageArtifactBinding `json:"artifact_binding"`
	Findings          []ProviderCausalFinding   `json:"findings"`
	AggregateDigest   string                    `json:"aggregate_digest"`
}

type frozenProofComparison struct {
	Comparable bool
	Differs    bool
}

func (carrier ProviderCausalCarrier) Validate() error {
	if !validSHA256(carrier.SubjectHash) || carrier.CandidateIdentity == (CandidateIdentity{}) {
		return errors.New("provider causal carrier binding is incomplete") // refusal:by-design operator-knowledge: malformed provider authority requires fresh capture
	}
	seen := map[string]bool{}
	for i, finding := range carrier.Findings {
		if finding.FindingID == "" || seen[finding.FindingID] || i > 0 && finding.FindingID < carrier.Findings[i-1].FindingID {
			return errors.New("provider causal carrier finding IDs are not canonical") // refusal:by-design operator-knowledge: canonical IDs are required before persistence
		}
		if finding.Location != strings.TrimSpace(finding.Location) || !canonicalProviderProofRefsOrdered(finding.ProofRefs) || !canonicalProviderProofRefsOrdered(finding.BaseProofRefs) || !canonicalProviderProofRefsOrdered(finding.CandidateProofRefs) {
			return errors.New("provider causal carrier proof references are not canonical") // refusal:by-design operator-knowledge: proof order is part of persisted authority
		}
		if len(finding.BaseProofRefs) > 0 && !canonicalProviderProofRefsParseable(finding.BaseProofRefs) || len(finding.CandidateProofRefs) > 0 && !canonicalProviderProofRefsParseable(finding.CandidateProofRefs) {
			return errors.New("provider causal carrier proof references are not parseable") // refusal:by-design operator-knowledge: persisted frozen proof locations must use the native location grammar
		}
		switch finding.Classification {
		case ProviderCandidateCausal:
			if len(finding.ProofRefs) == 0 || !canonicalProviderProofRefsParseable(finding.ProofRefs) {
				return errors.New("candidate-causal provider finding requires proof references") // refusal:by-design operator-knowledge: candidate causality requires affirmative frozen proof
			}
		case ProviderProvenNonCandidate:
			if len(finding.BaseProofRefs) == 0 || len(finding.CandidateProofRefs) == 0 || !canonicalProviderProofRefsParseable(finding.BaseProofRefs) || !canonicalProviderProofRefsParseable(finding.CandidateProofRefs) {
				return errors.New("proven-non-candidate provider finding requires base and candidate proof references") // refusal:by-design operator-knowledge: non-causality requires affirmative frozen proof on both sides
			}
		case ProviderUnknown:
		default:
			return errors.New("provider causal carrier classification is unsupported") // refusal:by-design operator-knowledge: the provider classification domain is closed
		}
		if finding.EvidenceDigest != providerFindingDigest(finding) {
			return errors.New("provider causal finding digest does not match its content") // refusal:by-design operator-knowledge: persisted evidence integrity failed
		}
		seen[finding.FindingID] = true
	}
	if carrier.AggregateDigest != providerAggregateDigest(carrier) {
		return errors.New("provider causal carrier digest does not match its content") // refusal:by-design operator-knowledge: persisted carrier integrity failed
	}
	if carrier.ArtifactBinding.Subject.SubjectHash != carrier.SubjectHash {
		return errors.New("provider causal carrier artifact binding does not match its subject") // refusal:by-design operator-knowledge: capture binding is contradictory
	}
	return nil
}

func canonicalProviderProofRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref != "" && !seen[ref] {
			seen[ref] = true
			result = append(result, ref)
		}
	}
	sort.Strings(result)
	return result
}
func canonicalProviderProofRefsOrdered(refs []string) bool {
	for i, ref := range refs {
		if ref == "" || ref != strings.TrimSpace(ref) || i > 0 && refs[i-1] >= ref {
			return false
		}
	}
	return true
}
func canonicalProviderProofRefsParseable(refs []string) bool {
	for _, ref := range refs {
		if _, err := parseFindingLocation(ref); err != nil {
			return false
		}
	}
	return true
}

func canonicalProviderClaims(claims []ProviderCausalEvidence) ([]ProviderCausalEvidence, error) {
	result := append([]ProviderCausalEvidence(nil), claims...)
	for i := range result {
		result[i].FindingID = strings.TrimSpace(result[i].FindingID)
		result[i].Location = strings.TrimSpace(result[i].Location)
		result[i].ProofRefs = canonicalProviderProofRefs(result[i].ProofRefs)
		result[i].BaseProofRefs = canonicalProviderProofRefs(result[i].BaseProofRefs)
		result[i].CandidateProofRefs = canonicalProviderProofRefs(result[i].CandidateProofRefs)
		if result[i].FindingID == "" {
			return nil, errors.New("provider causal evidence requires a finding ID") // refusal:by-design operator-knowledge: provider evidence needs a stable canonical ID
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FindingID != result[j].FindingID {
			return result[i].FindingID < result[j].FindingID
		}
		return result[i].Location < result[j].Location
	})
	unique := result[:0]
	for _, claim := range result {
		if len(unique) == 0 || unique[len(unique)-1].FindingID != claim.FindingID {
			unique = append(unique, claim)
		}
	}
	return unique, nil
}

func providerFindingDigest(f ProviderCausalFinding) string {
	payload, _ := json.Marshal(struct {
		ID, Location   string
		Proof          []string
		BaseProof      []string
		CandidateProof []string
		Class          ProviderCausalClassification
	}{f.FindingID, f.Location, f.ProofRefs, f.BaseProofRefs, f.CandidateProofRefs, f.Classification})
	sum := sha256.Sum256(append([]byte("gentle-ai.provider-causal-finding/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func providerAggregateDigest(c ProviderCausalCarrier) string {
	parts := make([]string, len(c.Findings))
	for i, f := range c.Findings {
		parts[i] = f.EvidenceDigest
	}
	payload, _ := json.Marshal(struct {
		Subject   string
		Candidate CandidateIdentity
		Binding   NewLineageArtifactBinding
		Findings  []string
	}{c.SubjectHash, c.CandidateIdentity, c.ArtifactBinding, parts})
	sum := sha256.Sum256(append([]byte("gentle-ai.provider-causal-carrier/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DeriveProviderCausalCarrier(ctx context.Context, repo, subjectHash string, candidate CandidateIdentity, claims []ProviderCausalEvidence) (ProviderCausalCarrier, error) {
	if !validSHA256(subjectHash) || !validGitTree(candidate.BaseTree) || !validGitTree(candidate.CandidateTree) {
		return ProviderCausalCarrier{}, errors.New("provider causal derivation requires a valid frozen candidate") // refusal:by-design operator-knowledge: capture cannot proceed without frozen trees
	}
	canonical, err := canonicalProviderClaims(claims)
	if err != nil {
		return ProviderCausalCarrier{}, err
	}
	carrier := ProviderCausalCarrier{SubjectHash: subjectHash, CandidateIdentity: candidate, Findings: make([]ProviderCausalFinding, 0, len(canonical))}
	for _, claim := range canonical {
		classification := ProviderUnknown
		changed := false
		if claim.Location != "" {
			changed, err = frozenCandidateLineChanged(ctx, repo, candidate, claim.Location)
			if err != nil {
				return ProviderCausalCarrier{}, err
			}
		}
		candidateProof := canonicalProviderProofRefsHaveFrozenEvidence(ctx, repo, candidate.CandidateTree, claim.CandidateProofRefs)
		baseProof := canonicalProviderProofRefsHaveFrozenEvidence(ctx, repo, candidate.BaseTree, claim.BaseProofRefs)
		comparison := frozenDifferentialProof(ctx, repo, candidate, claim.BaseProofRefs, claim.CandidateProofRefs)
		switch claim.CausalDisposition {
		case CausalBehaviorActivated:
			// A changed location alone does not establish an activated behavior.
			if changed && len(claim.ProofRefs) > 0 && canonicalProviderProofRefsHaveChangedFrozenEvidence(ctx, repo, candidate, claim.ProofRefs) && comparison.Comparable && comparison.Differs {
				classification = ProviderCandidateCausal
			}
		case CausalPreExisting:
			if baseProof && candidateProof && comparison.Comparable && !comparison.Differs {
				classification = ProviderProvenNonCandidate
			}
		case CausalBaseOnly:
			if baseProof && candidateProof && comparison.Comparable && comparison.Differs {
				classification = ProviderProvenNonCandidate
			}
		default:
			if changed && len(claim.ProofRefs) > 0 && canonicalProviderProofRefsHaveChangedFrozenEvidence(ctx, repo, candidate, claim.ProofRefs) {
				classification = ProviderCandidateCausal
			}
		}
		finding := ProviderCausalFinding{FindingID: claim.FindingID, Location: claim.Location, ProofRefs: claim.ProofRefs, BaseProofRefs: claim.BaseProofRefs, CandidateProofRefs: claim.CandidateProofRefs, Classification: classification}
		finding.EvidenceDigest = providerFindingDigest(finding)
		carrier.Findings = append(carrier.Findings, finding)
	}
	carrier.AggregateDigest = providerAggregateDigest(carrier)
	return carrier, nil
}

// BindProviderCausalCarrier attaches the exact provider artifact binding and
// refreshes the content digests before persistence.
func BindProviderCausalCarrier(carrier ProviderCausalCarrier, binding NewLineageArtifactBinding) (ProviderCausalCarrier, error) {
	if carrier.SubjectHash == "" || binding.Subject.SubjectHash != carrier.SubjectHash {
		return ProviderCausalCarrier{}, errors.New("provider causal carrier binding does not match its subject")
	}
	carrier.ArtifactBinding = binding
	for index := range carrier.Findings {
		carrier.Findings[index].EvidenceDigest = providerFindingDigest(carrier.Findings[index])
	}
	carrier.AggregateDigest = providerAggregateDigest(carrier)
	if err := carrier.Validate(); err != nil {
		return ProviderCausalCarrier{}, err
	}
	return carrier, nil
}

func canonicalProviderProofRefsHaveChangedFrozenEvidence(ctx context.Context, repo string, candidate CandidateIdentity, refs []string) bool {
	if !canonicalProviderProofRefsParseable(refs) {
		return false
	}
	for _, ref := range refs {
		changed, err := frozenCandidateLineChanged(ctx, repo, candidate, ref)
		if err != nil || !changed {
			return false
		}
	}
	return true
}

func canonicalProviderProofRefsHaveFrozenEvidence(ctx context.Context, repo, tree string, refs []string) bool {
	if !canonicalProviderProofRefsParseable(refs) {
		return false
	}
	for _, ref := range refs {
		if !frozenTreeLocationEvidence(ctx, repo, tree, ref) {
			return false
		}
	}
	return true
}

func frozenTreeLocationEvidence(ctx context.Context, repo, tree, location string) bool {
	parsed, err := parseFindingLocation(location)
	if err != nil {
		return false
	}
	content, err := runGit(ctx, repo, nil, nil, "cat-file", "blob", tree+":"+parsed.Path)
	if err != nil {
		return false
	}
	return len(strings.Split(string(content), "\n")) >= parsed.EndLine
}

func frozenDifferentialProof(ctx context.Context, repo string, candidate CandidateIdentity, baseRefs, candidateRefs []string) frozenProofComparison {
	if len(baseRefs) == 0 || len(candidateRefs) == 0 || len(baseRefs) != len(candidateRefs) {
		return frozenProofComparison{}
	}
	comparison := frozenProofComparison{Comparable: true}
	for index, baseRef := range baseRefs {
		candidateRef := candidateRefs[index]
		base, baseErr := parseFindingLocation(baseRef)
		current, candidateErr := parseFindingLocation(candidateRef)
		if baseErr != nil || candidateErr != nil || base.Path != current.Path || base.StartLine != current.StartLine || base.EndLine != current.EndLine {
			return frozenProofComparison{}
		}
		if !frozenTreeLocationEvidence(ctx, repo, candidate.BaseTree, baseRef) || !frozenTreeLocationEvidence(ctx, repo, candidate.CandidateTree, candidateRef) {
			return frozenProofComparison{}
		}
		baseContent, baseErr := runGit(ctx, repo, nil, nil, "cat-file", "blob", candidate.BaseTree+":"+base.Path)
		candidateContent, candidateErr := runGit(ctx, repo, nil, nil, "cat-file", "blob", candidate.CandidateTree+":"+current.Path)
		if baseErr != nil || candidateErr != nil {
			return frozenProofComparison{}
		}
		if string(baseContent) != string(candidateContent) {
			comparison.Differs = true
		}
	}
	return comparison
}

var providerHunk = regexp.MustCompile(`(?m)^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func frozenCandidateLineChanged(ctx context.Context, repo string, candidate CandidateIdentity, location string) (bool, error) {
	parsed, err := parseFindingLocation(location)
	if err != nil {
		return false, err
	}
	path := parsed.Path
	output, err := runGit(ctx, repo, nil, nil, "diff", "--unified=0", "--no-renames", "--no-ext-diff", "--no-textconv", candidate.BaseTree, candidate.CandidateTree, "--", literalPathspec(path))
	if err != nil {
		return false, err
	}
	coveredThrough := parsed.StartLine
	for _, match := range providerHunk.FindAllSubmatch(output, -1) {
		start, _ := strconv.Atoi(string(match[1]))
		count := 1
		if len(match[2]) > 0 {
			count, _ = strconv.Atoi(string(match[2]))
		}
		if count > 0 && start <= coveredThrough && start+count > coveredThrough {
			coveredThrough = start + count
			if coveredThrough > parsed.EndLine {
				return true, nil
			}
		}
	}
	return false, nil
}
