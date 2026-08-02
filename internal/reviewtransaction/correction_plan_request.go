package reviewtransaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

const (
	CorrectionPlanRequestSchema   = "gentle-ai.review-correction-plan-request/v1"
	CorrectionPlanRequestSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/correction-plan-request.schema.json"
)

// CorrectionPlanRequest is the immutable provider-owned input for planning one
// bounded correction. Its findings are the exact causal findings accepted by
// compact authority, not raw reviewer output.
type CorrectionPlanRequest struct {
	Schema           string                  `json:"schema"`
	RequestHash      string                  `json:"request_hash"`
	LineageID        string                  `json:"lineage_id"`
	ExpectedRevision string                  `json:"expected_revision"`
	TargetIdentity   string                  `json:"target_identity"`
	CorrectionBudget int                     `json:"correction_budget"`
	FixFindingIDs    []string                `json:"fix_finding_ids"`
	Findings         []CorrectionPlanFinding `json:"findings"`
}

type CorrectionPlanFinding struct {
	ID                string            `json:"id"`
	Lens              string            `json:"lens"`
	Location          string            `json:"location"`
	Severity          string            `json:"severity"`
	Claim             string            `json:"claim"`
	ProofRefs         []string          `json:"proof_refs"`
	Evidence          string            `json:"evidence"`
	EvidenceClass     EvidenceClass     `json:"evidence_class"`
	CausalDisposition CausalDisposition `json:"causal_disposition"`
}

func BuildCorrectionPlanRequest(state CompactState, revision string) (CorrectionPlanRequest, error) {
	if err := state.Validate(); err != nil {
		return CorrectionPlanRequest{}, err
	}
	wantRevision, err := CompactRevisionForState(state)
	if err != nil || revision != wantRevision {
		return CorrectionPlanRequest{}, errors.New("correction plan request requires the exact compact authority revision") // refusal:by-design world-action: provider code must rebuild the request from current validated authority
	}
	if state.State != StateCorrectionRequired || state.CorrectionAttemptConsumed() || len(state.FixFindingIDs) == 0 {
		return CorrectionPlanRequest{}, errors.New("correction plan request requires an available compact correction") // refusal:by-design world-action: only current provider authority can grant correction scope
	}
	byID := make(map[string]Finding, len(state.Findings))
	for _, finding := range state.Findings {
		byID[finding.ID] = finding
	}
	findings := make([]CorrectionPlanFinding, len(state.FixFindingIDs))
	for index, id := range state.FixFindingIDs {
		finding, ok := byID[id]
		classification, classified := state.Classifications[id]
		if !ok || !classified || state.Outcomes[id] != OutcomeCorroborated {
			return CorrectionPlanRequest{}, errors.New("correction plan request finding is not accepted by compact authority") // refusal:by-design world-action: provider authority must be repaired before projecting findings
		}
		findings[index] = CorrectionPlanFinding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location, Severity: finding.Severity,
			Claim: finding.Claim, ProofRefs: append([]string(nil), finding.ProofRefs...), Evidence: classification.Proof,
			EvidenceClass: classification.Class, CausalDisposition: classification.Causality,
		}
	}
	request := CorrectionPlanRequest{
		Schema: CorrectionPlanRequestSchema, LineageID: state.LineageID, ExpectedRevision: revision,
		TargetIdentity: state.CurrentSnapshot.Identity, CorrectionBudget: state.CorrectionBudget,
		FixFindingIDs: append([]string(nil), state.FixFindingIDs...), Findings: findings,
	}
	request.RequestHash = correctionPlanRequestHash(request)
	if err := ValidateCorrectionPlanRequest(request); err != nil {
		return CorrectionPlanRequest{}, err
	}
	return request, nil
}

func ValidateCorrectionPlanRequest(request CorrectionPlanRequest) error {
	if request.Schema != CorrectionPlanRequestSchema || validateLineageID(request.LineageID) != nil ||
		!validSHA256(request.RequestHash) || !validSHA256(request.ExpectedRevision) || !validSHA256(request.TargetIdentity) {
		return errors.New("correction plan request identity is incomplete") // refusal:by-design world-action: a malformed provider request cannot authorize correction planning
	}
	if request.CorrectionBudget <= 0 || request.CorrectionBudget > MaxCorrectionChangedLines {
		return errors.New("correction plan request budget is invalid") // refusal:by-design world-action: provider code must preserve the frozen authority budget
	}
	ids, err := canonicalStrings(request.FixFindingIDs, "fix finding id")
	if err != nil || !equalStrings(ids, request.FixFindingIDs) || len(ids) == 0 || len(ids) != len(request.Findings) {
		return errors.New("correction plan request finding IDs are not canonical") // refusal:by-design world-action: provider code must emit the exact accepted finding set
	}
	for index, finding := range request.Findings {
		structured := Finding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location, Severity: finding.Severity,
			Claim: finding.Claim, ProofRefs: finding.ProofRefs,
		}
		if finding.ID != ids[index] || validateStructuredFinding(structured) != nil || !isConcreteEvidence(finding.Evidence) ||
			!isSupportedEvidenceClass(finding.EvidenceClass) ||
			(finding.CausalDisposition != CausalIntroduced && finding.CausalDisposition != CausalBehaviorActivated && finding.CausalDisposition != CausalWorsened) {
			return errors.New("correction plan request contains a non-actionable finding") // refusal:by-design world-action: provider code must project complete accepted finding evidence
		}
	}
	if correctionPlanRequestHash(request) != request.RequestHash {
		return errors.New("correction plan request hash does not match its binding") // refusal:by-design world-action: tampered provider-owned planning input must not be used
	}
	return nil
}

func correctionPlanRequestHash(request CorrectionPlanRequest) string {
	preimage := struct {
		Schema           string                  `json:"schema"`
		LineageID        string                  `json:"lineage_id"`
		ExpectedRevision string                  `json:"expected_revision"`
		TargetIdentity   string                  `json:"target_identity"`
		CorrectionBudget int                     `json:"correction_budget"`
		FixFindingIDs    []string                `json:"fix_finding_ids"`
		Findings         []CorrectionPlanFinding `json:"findings"`
	}{
		Schema: request.Schema, LineageID: request.LineageID, ExpectedRevision: request.ExpectedRevision,
		TargetIdentity: request.TargetIdentity, CorrectionBudget: request.CorrectionBudget,
		FixFindingIDs: request.FixFindingIDs, Findings: request.Findings,
	}
	payload, _ := json.Marshal(preimage)
	sum := sha256.Sum256(append([]byte(CorrectionPlanRequestSchema+"\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}
