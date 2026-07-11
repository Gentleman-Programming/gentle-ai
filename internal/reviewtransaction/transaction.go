package reviewtransaction

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const TransactionSchema = "gentle-ai.review-transaction/v1"

type Mode string

const (
	ModeOrdinary4R      Mode = "ordinary_4r"
	ModeOrdinaryBounded Mode = "ordinary_bounded"
	ModeJudgmentDay     Mode = "judgment_day"
)

const (
	LensRisk        = "review-risk"
	LensResilience  = "review-resilience"
	LensReadability = "review-readability"
	LensReliability = "review-reliability"
)

var supportedLenses = []string{LensRisk, LensResilience, LensReadability, LensReliability}

type State string

const (
	StateUnreviewed             State = "unreviewed"
	StateReviewing              State = "reviewing"
	StateJudgesConfirmed        State = "judges_confirmed"
	StateFindingsFrozen         State = "findings_frozen"
	StateEvidenceClassified     State = "evidence_classified"
	StateFixRequired            State = "fix_required"
	StateFixing                 State = "fixing"
	StateFixValidating          State = "fix_validating"
	StateReadyFinalVerification State = "ready_final_verification"
	StateFinalVerifying         State = "final_verifying"
	StateApproved               State = "approved"
	StateEscalated              State = "escalated"
)

type EvidenceClass string

const (
	EvidenceDeterministic EvidenceClass = "deterministic"
	EvidenceInferential   EvidenceClass = "inferential"
	EvidenceInsufficient  EvidenceClass = "insufficient"
)

type EvidenceOutcome string

const (
	OutcomeCorroborated EvidenceOutcome = "corroborated"
	OutcomeRefuted      EvidenceOutcome = "refuted"
	OutcomeInconclusive EvidenceOutcome = "inconclusive"
	OutcomeInfo         EvidenceOutcome = "info"
)

type Counters struct {
	FullReviews           int `json:"full_reviews"`
	RefuterBatches        int `json:"refuter_batches"`
	FixBatches            int `json:"fix_batches"`
	ScopedFixValidations  int `json:"scoped_fix_validations"`
	FinalVerifications    int `json:"final_verifications"`
	FixRounds             int `json:"fix_rounds"`
	ScopedRejudgments     int `json:"scoped_rejudgments"`
	JudgeExecutions       int `json:"judge_executions"`
	RiskExecutions        int `json:"risk_executions,omitempty"`
	ResilienceExecutions  int `json:"resilience_executions,omitempty"`
	ReadabilityExecutions int `json:"readability_executions,omitempty"`
	ReliabilityExecutions int `json:"reliability_executions,omitempty"`
}

type Start struct {
	LineageID      string
	Mode           Mode
	Generation     int
	Snapshot       Snapshot
	PolicyHash     string
	RiskLevel      RiskLevel
	SelectedLenses []string
}

type LensResult struct {
	Lens       string    `json:"lens"`
	Findings   []Finding `json:"findings"`
	Evidence   []string  `json:"evidence"`
	ResultHash string    `json:"result_hash"`
}

type Finding struct {
	ID        string   `json:"id"`
	Lens      string   `json:"lens,omitempty"`
	Location  string   `json:"location,omitempty"`
	Severity  string   `json:"severity,omitempty"`
	Claim     string   `json:"claim,omitempty"`
	ProofRefs []string `json:"proof_refs,omitempty"`
}

type ScopedValidationResult struct {
	LedgerIDs            []string        `json:"ledger_ids"`
	Approved             bool            `json:"approved"`
	FixCausedFindings    []Finding       `json:"fix_caused_findings"`
	OriginalCriteria     ValidationCheck `json:"original_criteria"`
	CorrectionRegression ValidationCheck `json:"correction_regression"`
	FollowUps            []FollowUp      `json:"follow_ups"`
}

type ValidationCheck struct {
	EvidenceHash string `json:"evidence_hash"`
	FixDeltaHash string `json:"fix_delta_hash"`
	Passed       bool   `json:"passed"`
}

type FollowUp struct {
	Observation string   `json:"observation"`
	ProofRefs   []string `json:"proof_refs"`
}

type FindingEvidence struct {
	FindingID string        `json:"finding_id"`
	Class     EvidenceClass `json:"class"`
	Proof     string        `json:"proof"`
}

type RefuterClaim struct {
	FindingID        string `json:"finding_id"`
	SnapshotIdentity string `json:"snapshot_identity"`
	Proof            string `json:"proof"`
}

type JudgeProof struct {
	JudgeID       string `json:"judge_id"`
	ExecutionHash string `json:"execution_hash"`
	ResultHash    string `json:"result_hash"`
	Blind         bool   `json:"blind"`
	Confirmed     bool   `json:"confirmed"`
}

type EvidenceRoute struct {
	RefuterClaims     []RefuterClaim `json:"refuter_claims"`
	AutoFixFindingIDs []string       `json:"auto_fix_finding_ids"`
}

type EvidenceResult struct {
	FindingID string          `json:"finding_id"`
	Outcome   EvidenceOutcome `json:"outcome"`
	Proof     string          `json:"proof"`
}

type Transaction struct {
	Schema                 string                     `json:"schema"`
	LineageID              string                     `json:"lineage_id"`
	Mode                   Mode                       `json:"mode"`
	Generation             int                        `json:"generation"`
	State                  State                      `json:"state"`
	Snapshot               Snapshot                   `json:"snapshot"`
	GenesisPaths           []string                   `json:"genesis_paths,omitempty"`
	BaseTree               string                     `json:"base_tree"`
	PathsDigest            string                     `json:"paths_digest"`
	InitialReviewTree      string                     `json:"initial_review_tree"`
	FinalCandidateTree     string                     `json:"final_candidate_tree"`
	FixDeltaHash           string                     `json:"fix_delta_hash"`
	PolicyHash             string                     `json:"policy_hash"`
	LedgerHash             string                     `json:"ledger_hash"`
	LedgerFindingsHash     string                     `json:"ledger_findings_hash"`
	EvidenceHash           string                     `json:"evidence_hash"`
	JudgeProofHash         string                     `json:"judge_proof_hash,omitempty"`
	JudgeAgreementHash     string                     `json:"judge_agreement_hash,omitempty"`
	JudgeProofs            []JudgeProof               `json:"judge_proofs"`
	Release                *ReleaseEvidence           `json:"release,omitempty"`
	FailedEvidenceRevision string                     `json:"failed_evidence_revision,omitempty"`
	Counters               Counters                   `json:"counters"`
	Findings               []Finding                  `json:"findings"`
	Classifications        map[string]FindingEvidence `json:"classifications"`
	Outcomes               map[string]EvidenceOutcome `json:"outcomes"`
	FixFindingIDs          []string                   `json:"fix_finding_ids"`
	PendingRefuterIDs      []string                   `json:"pending_refuter_ids"`
	FixCausedFindings      []Finding                  `json:"fix_caused_findings"`
	FollowUps              []FollowUp                 `json:"follow_ups"`
	OriginalCriteria       *ValidationCheck           `json:"original_criteria,omitempty"`
	CorrectionRegression   *ValidationCheck           `json:"correction_regression,omitempty"`
	RiskLevel              RiskLevel                  `json:"risk_level,omitempty"`
	SelectedLenses         []string                   `json:"selected_lenses,omitempty"`
	LensResults            []LensResult               `json:"lens_results,omitempty"`
}

func NewTransaction(start Start) (*Transaction, error) {
	if err := validateLineageID(start.LineageID); err != nil {
		return nil, err
	}
	if start.Mode != ModeOrdinary4R && start.Mode != ModeOrdinaryBounded && start.Mode != ModeJudgmentDay {
		return nil, fmt.Errorf("unsupported review mode %q", start.Mode)
	}
	selectedLenses, err := validateSelectedLenses(start.Mode, start.RiskLevel, start.SelectedLenses)
	if err != nil {
		return nil, err
	}
	if start.Generation < 1 {
		return nil, errors.New("generation must be positive")
	}
	if err := validateSnapshot(start.Snapshot); err != nil {
		return nil, err
	}
	if !validSHA256(start.PolicyHash) {
		return nil, errors.New("policy_hash must be a lowercase SHA-256 identity")
	}
	transaction := &Transaction{
		Schema: TransactionSchema, LineageID: start.LineageID, Mode: start.Mode,
		Generation: start.Generation, State: StateUnreviewed, Snapshot: start.Snapshot,
		GenesisPaths: append([]string(nil), start.Snapshot.Paths...),
		BaseTree:     start.Snapshot.BaseTree, PathsDigest: start.Snapshot.PathsDigest,
		InitialReviewTree: start.Snapshot.CandidateTree, FinalCandidateTree: start.Snapshot.CandidateTree,
		FixDeltaHash: EmptyFixDeltaHash, PolicyHash: start.PolicyHash,
		Findings: []Finding{}, Classifications: map[string]FindingEvidence{},
		Outcomes: map[string]EvidenceOutcome{}, FixFindingIDs: []string{}, PendingRefuterIDs: []string{},
		FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, JudgeProofs: []JudgeProof{},
	}
	if start.Mode == ModeOrdinaryBounded {
		transaction.RiskLevel = start.RiskLevel
		transaction.SelectedLenses = selectedLenses
		transaction.LensResults = []LensResult{}
	}
	return transaction, nil
}

func NewLineage(previousLineageID string, start Start) (*Transaction, error) {
	if validateLineageID(previousLineageID) != nil || start.LineageID == previousLineageID {
		return nil, errors.New("scope change requires an explicit different lineage_id")
	}
	return NewTransaction(start)
}

func (transaction *Transaction) StartReview() error {
	if transaction.State != StateUnreviewed {
		return transaction.invalidTransition("start review")
	}
	if transaction.Mode == ModeOrdinary4R {
		if transaction.Counters.FullReviews >= 1 {
			return transaction.escalateBudget("full review")
		}
		transaction.Counters.FullReviews++
	}
	transaction.State = StateReviewing
	return nil
}

func (transaction *Transaction) RecordLensResult(result LensResult) error {
	if transaction.Mode != ModeOrdinaryBounded || transaction.State != StateReviewing {
		return transaction.invalidTransition("record lens result")
	}
	result.Lens = strings.TrimSpace(result.Lens)
	if !isSupportedLens(result.Lens) {
		return fmt.Errorf("unknown review lens %q", result.Lens)
	}
	selectedIndex := stringIndex(transaction.SelectedLenses, result.Lens)
	if selectedIndex < 0 {
		return fmt.Errorf("review lens %q was not selected", result.Lens)
	}
	if transaction.lensExecutions(result.Lens) != 0 || stringIndexLensResult(transaction.LensResults, result.Lens) >= 0 {
		return fmt.Errorf("review lens %q already has a complete result", result.Lens)
	}
	if selectedIndex != len(transaction.LensResults) {
		return fmt.Errorf("review lens %q is out of canonical execution order", result.Lens)
	}
	validated, err := validateLensResult(result)
	if err != nil {
		return err
	}
	for _, existing := range transaction.LensResults {
		if existing.ResultHash == validated.ResultHash {
			return errors.New("lens result evidence cannot be reused across selected lenses")
		}
	}
	transaction.LensResults = append(transaction.LensResults, validated)
	transaction.setLensExecutions(validated.Lens, 1)
	return nil
}

// LensResultHash binds a complete native lens result to its lens and content.
func LensResultHash(result LensResult) string {
	payload, _ := json.Marshal(struct {
		Lens     string    `json:"lens"`
		Findings []Finding `json:"findings"`
		Evidence []string  `json:"evidence"`
	}{Lens: result.Lens, Findings: result.Findings, Evidence: result.Evidence})
	sum := sha256.Sum256(append([]byte("gentle-ai.lens-result/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validateLensResult(result LensResult) (LensResult, error) {
	result.Lens = strings.TrimSpace(result.Lens)
	if !isSupportedLens(result.Lens) {
		return LensResult{}, fmt.Errorf("unknown review lens %q", result.Lens)
	}
	if result.Findings == nil || result.Evidence == nil || len(result.Evidence) == 0 {
		return LensResult{}, errors.New("lens result requires explicit findings and concrete evidence")
	}
	wantFindingLens := strings.TrimPrefix(result.Lens, "review-")
	for index, finding := range result.Findings {
		if err := validateStructuredFinding(finding); err != nil {
			return LensResult{}, fmt.Errorf("lens result finding[%d]: %w", index, err)
		}
		if finding.Lens != wantFindingLens {
			return LensResult{}, fmt.Errorf("lens result finding[%d] is not bound to %q", index, result.Lens)
		}
	}
	for index, evidence := range result.Evidence {
		if !isConcreteEvidence(evidence) {
			return LensResult{}, fmt.Errorf("lens result evidence[%d] must be concrete", index)
		}
	}
	findings := make([]Finding, len(result.Findings))
	copy(findings, result.Findings)
	result.Findings = findings
	evidence := make([]string, len(result.Evidence))
	copy(evidence, result.Evidence)
	result.Evidence = evidence
	derived := LensResultHash(result)
	if result.ResultHash != "" && result.ResultHash != derived {
		return LensResult{}, errors.New("lens result_hash does not match canonical structured result content")
	}
	result.ResultHash = derived
	return result, nil
}

func (transaction *Transaction) RecordJudgeProofs(proofs []JudgeProof, agreementHash string) error {
	if transaction.Mode != ModeJudgmentDay || transaction.State != StateReviewing {
		return transaction.invalidTransition("record Judgment Day judge proofs")
	}
	validated, proofHash, err := validateJudgeProofs(proofs, agreementHash)
	if err != nil {
		return err
	}
	transaction.JudgeProofs = validated
	transaction.JudgeProofHash = proofHash
	transaction.JudgeAgreementHash = agreementHash
	transaction.Counters.JudgeExecutions = len(validated)
	transaction.State = StateJudgesConfirmed
	return nil
}

func (transaction *Transaction) FreezeFindings(findings []Finding, ledgerHash string) error {
	expectedState := StateReviewing
	if transaction.Mode == ModeJudgmentDay {
		expectedState = StateJudgesConfirmed
	}
	if transaction.State != expectedState {
		return transaction.invalidTransition("freeze findings")
	}
	if transaction.Mode == ModeOrdinaryBounded {
		if len(transaction.LensResults) != len(transaction.SelectedLenses) {
			return errors.New("cannot freeze findings before every selected lens has one complete result")
		}
		if len(transaction.SelectedLenses) == 0 && findings == nil {
			return errors.New("zero-lens review requires an explicit empty findings ledger")
		}
		if !reflectLensFindings(transaction.LensResults, findings) {
			return errors.New("frozen findings must exactly match the completed native lens results")
		}
	}
	if !validSHA256(ledgerHash) {
		return errors.New("ledger_hash must be a lowercase SHA-256 identity")
	}
	seen := map[string]struct{}{}
	validated := make([]Finding, len(findings))
	infoOutcomes := make(map[string]EvidenceOutcome)
	for index, finding := range findings {
		finding.ID = strings.TrimSpace(finding.ID)
		if finding.ID == "" {
			return errors.New("finding id is required")
		}
		if _, duplicate := seen[finding.ID]; duplicate {
			return fmt.Errorf("duplicate finding id %q", finding.ID)
		}
		seen[finding.ID] = struct{}{}
		if !isSupportedSeverity(finding.Severity) {
			return fmt.Errorf("finding %q has unsupported severity %q", finding.ID, finding.Severity)
		}
		validated[index] = finding
		if !isSevereSeverity(finding.Severity) {
			infoOutcomes[finding.ID] = OutcomeInfo
		}
	}
	transaction.Findings = validated
	for id, outcome := range infoOutcomes {
		transaction.Outcomes[id] = outcome
	}
	transaction.LedgerHash = ledgerHash
	transaction.LedgerFindingsHash = findingsHash(validated)
	transaction.State = StateFindingsFrozen
	return nil
}

func (transaction *Transaction) ClassifyEvidence(evidence []FindingEvidence) (EvidenceRoute, error) {
	if transaction.State != StateFindingsFrozen {
		return EvidenceRoute{}, transaction.invalidTransition("classify evidence")
	}
	severe := make(map[string]Finding, len(transaction.Findings))
	for _, finding := range transaction.Findings {
		if isSevereSeverity(finding.Severity) {
			severe[finding.ID] = finding
		}
	}
	byID := make(map[string]FindingEvidence, len(evidence))
	for _, item := range evidence {
		item.FindingID = strings.TrimSpace(item.FindingID)
		if _, ok := severe[item.FindingID]; !ok {
			return EvidenceRoute{}, fmt.Errorf("finding %q is not a frozen severe finding", item.FindingID)
		}
		if _, duplicate := byID[item.FindingID]; duplicate {
			return EvidenceRoute{}, fmt.Errorf("duplicate evidence for finding %q", item.FindingID)
		}
		byID[item.FindingID] = item
	}
	if len(byID) != len(severe) {
		return EvidenceRoute{}, errors.New("evidence classification must cover every frozen BLOCKER/CRITICAL finding exactly once")
	}

	route := EvidenceRoute{RefuterClaims: []RefuterClaim{}, AutoFixFindingIDs: []string{}}
	escalate := false
	classifications := cloneClassifications(transaction.Classifications)
	outcomes := cloneOutcomes(transaction.Outcomes)
	fixFindingIDs := append([]string{}, transaction.FixFindingIDs...)
	pendingRefuterIDs := append([]string{}, transaction.PendingRefuterIDs...)
	for _, finding := range transaction.Findings {
		if !isSevereSeverity(finding.Severity) {
			continue
		}
		item, ok := byID[finding.ID]
		if !ok || !isConcreteEvidence(item.Proof) {
			return EvidenceRoute{}, fmt.Errorf("finding %q requires concrete evidence", finding.ID)
		}
		classifications[finding.ID] = item
		switch item.Class {
		case EvidenceDeterministic:
			outcomes[finding.ID] = OutcomeCorroborated
			fixFindingIDs = addUniqueSorted(fixFindingIDs, finding.ID)
			route.AutoFixFindingIDs = append(route.AutoFixFindingIDs, finding.ID)
		case EvidenceInferential:
			if transaction.Mode == ModeJudgmentDay {
				outcomes[finding.ID] = OutcomeCorroborated
				fixFindingIDs = addUniqueSorted(fixFindingIDs, finding.ID)
				route.AutoFixFindingIDs = append(route.AutoFixFindingIDs, finding.ID)
				continue
			}
			pendingRefuterIDs = append(pendingRefuterIDs, finding.ID)
			route.RefuterClaims = append(route.RefuterClaims, RefuterClaim{
				FindingID: finding.ID, SnapshotIdentity: transaction.Snapshot.Identity, Proof: item.Proof,
			})
		case EvidenceInsufficient:
			outcomes[finding.ID] = OutcomeInconclusive
			escalate = true
		default:
			return EvidenceRoute{}, fmt.Errorf("unsupported evidence class %q", item.Class)
		}
	}
	sort.Strings(pendingRefuterIDs)
	sort.Strings(route.AutoFixFindingIDs)
	transaction.Classifications = classifications
	transaction.Outcomes = outcomes
	transaction.FixFindingIDs = fixFindingIDs
	transaction.PendingRefuterIDs = pendingRefuterIDs
	transaction.State = StateEvidenceClassified
	if escalate {
		for _, findingID := range transaction.PendingRefuterIDs {
			transaction.Outcomes[findingID] = OutcomeInconclusive
		}
		transaction.PendingRefuterIDs = []string{}
		transaction.State = StateEscalated
	} else if len(transaction.PendingRefuterIDs) == 0 {
		transaction.advanceAfterEvidence()
	}
	return route, nil
}

func (transaction *Transaction) ApplyRefuterOutcomes(results []EvidenceResult) error {
	if !isOrdinaryMode(transaction.Mode) || transaction.State != StateEvidenceClassified || len(transaction.PendingRefuterIDs) == 0 {
		return transaction.invalidTransition("apply refuter outcomes")
	}
	if transaction.Counters.RefuterBatches >= 1 {
		return transaction.escalateBudget("refuter batch")
	}
	byID := make(map[string]EvidenceResult, len(results))
	for _, result := range results {
		if _, duplicate := byID[result.FindingID]; duplicate {
			return transaction.failRefuterBatch(fmt.Errorf("duplicate refuter result for %q", result.FindingID))
		}
		byID[result.FindingID] = result
	}
	if len(byID) != len(transaction.PendingRefuterIDs) {
		return transaction.failRefuterBatch(errors.New("one complete refuter batch must return every inferential finding"))
	}
	outcomes := cloneOutcomes(transaction.Outcomes)
	fixFindingIDs := append([]string{}, transaction.FixFindingIDs...)
	escalate := false
	for _, findingID := range transaction.PendingRefuterIDs {
		result, ok := byID[findingID]
		if !ok || !isConcreteEvidence(result.Proof) {
			return transaction.failRefuterBatch(fmt.Errorf("refuter result %q requires concrete proof", findingID))
		}
		switch result.Outcome {
		case OutcomeCorroborated:
			fixFindingIDs = addUniqueSorted(fixFindingIDs, findingID)
		case OutcomeRefuted:
		case OutcomeInconclusive:
			escalate = true
		default:
			return transaction.failRefuterBatch(fmt.Errorf("unsupported refuter outcome %q", result.Outcome))
		}
		outcomes[findingID] = result.Outcome
	}
	transaction.Outcomes = outcomes
	transaction.FixFindingIDs = fixFindingIDs
	transaction.Counters.RefuterBatches++
	transaction.PendingRefuterIDs = []string{}
	if escalate {
		transaction.State = StateEscalated
	} else {
		transaction.advanceAfterEvidence()
	}
	return nil
}

func (transaction *Transaction) failRefuterBatch(cause error) error {
	for _, findingID := range transaction.PendingRefuterIDs {
		transaction.Outcomes[findingID] = OutcomeInconclusive
	}
	transaction.Counters.RefuterBatches++
	transaction.PendingRefuterIDs = []string{}
	transaction.State = StateEscalated
	return cause
}

func (transaction *Transaction) BeginFix(failedEvidenceRevision string) error {
	if transaction.State != StateFixRequired {
		return transaction.invalidTransition("begin fix")
	}
	if !validSHA256(failedEvidenceRevision) {
		return errors.New("failed evidence revision must be a lowercase SHA-256 identity")
	}
	switch transaction.Mode {
	case ModeOrdinary4R, ModeOrdinaryBounded:
		if transaction.Counters.FixBatches >= 1 {
			return transaction.escalateBudget("fix batch")
		}
		transaction.Counters.FixBatches++
	case ModeJudgmentDay:
		if transaction.Counters.FixRounds >= 2 {
			return transaction.escalateBudget("judgment-day fix round")
		}
		transaction.Counters.FixRounds++
	}
	transaction.FailedEvidenceRevision = failedEvidenceRevision
	transaction.State = StateFixing
	return nil
}

func (transaction *Transaction) CompleteFix(snapshot Snapshot, fixDeltaHash string, ledgerIDs []string) error {
	if transaction.State != StateFixing {
		return transaction.invalidTransition("complete fix")
	}
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	if snapshot.Kind != TargetFixDiff || snapshot.BaseTree != transaction.FinalCandidateTree {
		return errors.New("fix snapshot must be a fix-diff based on the previous final candidate tree")
	}
	ids, err := canonicalStrings(ledgerIDs, "ledger id")
	if err != nil {
		return err
	}
	if !equalStrings(ids, transaction.FixFindingIDs) || !equalStrings(ids, snapshot.LedgerIDs) {
		return errors.New("fix diff must be bound exactly to corroborated frozen ledger IDs")
	}
	genesisPaths := transaction.GenesisPaths
	if genesisPaths == nil {
		genesisPaths = transaction.Snapshot.Paths
	}
	if err := pathsAreSubset(snapshot.Paths, genesisPaths); err != nil {
		return err
	}
	transaction.Snapshot = snapshot
	transaction.FinalCandidateTree = snapshot.CandidateTree
	transaction.FixDeltaHash = FixDeltaHashForSnapshot(snapshot)
	transaction.State = StateFixValidating
	return nil
}

// FixDeltaHashForSnapshot is derived solely from the authoritative fix
// snapshot boundary. Narrative patches and caller-provided artifact hashes are
// not evidence of the correction that changed the candidate tree.
func FixDeltaHashForSnapshot(snapshot Snapshot) string {
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.fix-delta/v1\x00"))
	for _, value := range []string{snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof} {
		writeLengthPrefixed(hash, []byte(value))
	}
	for _, value := range snapshot.LedgerIDs {
		writeLengthPrefixed(hash, []byte(value))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func (transaction *Transaction) ValidateFixDelta(ledgerIDs []string, approved bool) error {
	// Legacy internal callers retain the v1 boolean transition. Public CLI
	// validation uses ValidateFixDeltaResult and persists independent evidence.
	return transaction.ValidateFixDeltaResult(ScopedValidationResult{
		LedgerIDs: ledgerIDs, Approved: approved, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: transaction.FixDeltaHash, FixDeltaHash: transaction.FixDeltaHash, Passed: approved},
		CorrectionRegression: ValidationCheck{EvidenceHash: transaction.FixDeltaHash, FixDeltaHash: transaction.FixDeltaHash, Passed: approved},
	})
}

func (transaction *Transaction) ValidateFixDeltaResult(result ScopedValidationResult) error {
	if transaction.State != StateFixValidating {
		return transaction.invalidTransition("validate fix delta")
	}
	ids, err := canonicalStrings(result.LedgerIDs, "ledger id")
	if err != nil {
		return err
	}
	if !equalStrings(ids, transaction.FixFindingIDs) {
		return errors.New("scoped validation must use the frozen fix ledger IDs")
	}
	if isOrdinaryMode(transaction.Mode) {
		legacy := result.OriginalCriteria.EvidenceHash == transaction.FixDeltaHash &&
			result.CorrectionRegression.EvidenceHash == transaction.FixDeltaHash
		if !legacy {
			if err := validateTargetedValidation(result, transaction.FixDeltaHash); err != nil {
				return err
			}
		}
	}
	if result.FixCausedFindings == nil {
		return errors.New("scoped validation must provide an explicit fix_caused_findings array")
	}
	if isOrdinaryMode(transaction.Mode) && result.FollowUps == nil {
		return errors.New("scoped validation must provide an explicit follow_ups array")
	}
	if isOrdinaryMode(transaction.Mode) && len(result.FixCausedFindings) != 0 {
		return errors.New("ordinary scoped validation records later observations only as non-blocking follow-ups")
	}
	if err := validateFollowUps(result.FollowUps); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(transaction.Findings)+len(result.FixCausedFindings))
	for _, finding := range transaction.Findings {
		seen[finding.ID] = struct{}{}
	}
	validated := make([]Finding, len(result.FixCausedFindings))
	severeFixCaused := 0
	for index, finding := range result.FixCausedFindings {
		if err := validateStructuredFinding(finding); err != nil {
			return fmt.Errorf("fix-caused finding[%d]: %w", index, err)
		}
		if _, exists := seen[finding.ID]; exists {
			return fmt.Errorf("duplicate fix-caused finding id %q", finding.ID)
		}
		seen[finding.ID] = struct{}{}
		validated[index] = finding
		if isSevereSeverity(finding.Severity) {
			severeFixCaused++
		}
	}
	if isOrdinaryMode(transaction.Mode) {
		result.Approved = result.OriginalCriteria.Passed && result.CorrectionRegression.Passed
		legacy := result.OriginalCriteria.EvidenceHash == transaction.FixDeltaHash &&
			result.CorrectionRegression.EvidenceHash == transaction.FixDeltaHash
		if !legacy {
			original, regression := result.OriginalCriteria, result.CorrectionRegression
			transaction.OriginalCriteria = &original
			transaction.CorrectionRegression = &regression
		}
	}
	if result.Approved && severeFixCaused > 0 {
		return errors.New("scoped validation cannot approve while recording severe fix-caused defects")
	}
	if !result.Approved && len(validated) > 0 && severeFixCaused == 0 {
		result.Approved = true
	}
	transaction.FixCausedFindings = append(transaction.FixCausedFindings, validated...)
	transaction.FollowUps = append(transaction.FollowUps, result.FollowUps...)
	if transaction.Mode == ModeJudgmentDay && severeFixCaused > 0 {
		for _, finding := range validated {
			if isSevereSeverity(finding.Severity) {
				transaction.FixFindingIDs = addUniqueSorted(transaction.FixFindingIDs, finding.ID)
			}
		}
	}
	switch transaction.Mode {
	case ModeOrdinary4R, ModeOrdinaryBounded:
		if transaction.Counters.ScopedFixValidations >= 1 {
			return transaction.escalateBudget("scoped fix validation")
		}
		transaction.Counters.ScopedFixValidations++
		if result.Approved {
			transaction.State = StateReadyFinalVerification
		} else {
			transaction.State = StateEscalated
		}
	case ModeJudgmentDay:
		if transaction.Counters.ScopedRejudgments >= 2 {
			return transaction.escalateBudget("scoped re-judgment")
		}
		transaction.Counters.ScopedRejudgments++
		if result.Approved {
			transaction.State = StateReadyFinalVerification
		} else if transaction.Counters.FixRounds >= 2 {
			transaction.State = StateEscalated
		} else {
			transaction.State = StateFixRequired
		}
	}
	return nil
}

func validateTargetedValidation(result ScopedValidationResult, fixDeltaHash string) error {
	for name, check := range map[string]ValidationCheck{"original criteria": result.OriginalCriteria, "correction regression": result.CorrectionRegression} {
		if !validSHA256(check.EvidenceHash) || !validSHA256(check.FixDeltaHash) {
			return fmt.Errorf("%s check requires SHA-256 evidence and fix-delta identities", name)
		}
		if check.FixDeltaHash != fixDeltaHash {
			return fmt.Errorf("%s check is stale for the immutable fix delta", name)
		}
	}
	return nil
}

func validateFollowUps(followUps []FollowUp) error {
	for index, followUp := range followUps {
		if strings.TrimSpace(followUp.Observation) == "" || len(followUp.ProofRefs) == 0 {
			return fmt.Errorf("follow_up[%d] requires observation and proof_refs", index)
		}
	}
	return nil
}

func (transaction *Transaction) BeginFinalVerification() error {
	if transaction.State != StateReadyFinalVerification {
		return transaction.invalidTransition("begin final verification")
	}
	if transaction.Counters.FinalVerifications >= 1 {
		return transaction.escalateBudget("final verification")
	}
	transaction.Counters.FinalVerifications++
	transaction.State = StateFinalVerifying
	return nil
}

func (transaction *Transaction) BindReleaseEvidence(release ReleaseEvidence) error {
	if transaction.State != StateReadyFinalVerification {
		return transaction.invalidTransition("bind release evidence")
	}
	if transaction.Release != nil {
		if *transaction.Release == release {
			return nil
		}
		return errors.New("release evidence is already bound and immutable")
	}
	if err := validateReleaseEvidence(release); err != nil {
		return err
	}
	if release.ReleaseTree != transaction.FinalCandidateTree {
		return errors.New("release tree must exactly match the final reviewed candidate tree")
	}
	copy := release
	transaction.Release = &copy
	return nil
}

func (transaction *Transaction) CompleteFinalVerification(evidenceHash string, approved bool) error {
	if transaction.State != StateFinalVerifying {
		return transaction.invalidTransition("complete final verification")
	}
	if !validSHA256(evidenceHash) {
		return errors.New("evidence_hash must be a lowercase SHA-256 identity")
	}
	transaction.EvidenceHash = evidenceHash
	if approved {
		transaction.State = StateApproved
	} else {
		transaction.State = StateEscalated
	}
	return nil
}

func ParseTransaction(payload []byte) (Transaction, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var transaction Transaction
	if err := decoder.Decode(&transaction); err != nil {
		return Transaction{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Transaction{}, errors.New("multiple JSON values in review transaction")
	}
	if err := transaction.validate(); err != nil {
		return Transaction{}, err
	}
	return transaction, nil
}

func (transaction *Transaction) validate() error {
	// v1 transactions did not persist follow-ups. Their absence is equivalent to
	// the explicit empty array required by the current lifecycle.
	if transaction.FollowUps == nil {
		transaction.FollowUps = []FollowUp{}
	}
	// v1 events written before semantic ledger binding used only ledger_hash.
	// Their immutable findings deterministically supply the missing binding on
	// read; the native gate still compares it to the retained ledger content.
	if transaction.LedgerHash != "" && transaction.LedgerFindingsHash == "" && len(transaction.Findings) > 0 || transaction.LedgerHash != "" && transaction.LedgerFindingsHash == "" && transaction.Findings != nil {
		transaction.LedgerFindingsHash = findingsHash(transaction.Findings)
	}
	if transaction.Schema != TransactionSchema {
		return errors.New("unsupported review transaction schema")
	}
	if err := validateLineageID(transaction.LineageID); err != nil {
		return err
	}
	if transaction.Generation < 1 {
		return errors.New("transaction requires a positive generation")
	}
	if transaction.Mode != ModeOrdinary4R && transaction.Mode != ModeOrdinaryBounded && transaction.Mode != ModeJudgmentDay {
		return errors.New("invalid transaction mode")
	}
	if err := validateSnapshot(transaction.Snapshot); err != nil {
		return err
	}
	if !validGitTree(transaction.BaseTree) || !validGitTree(transaction.InitialReviewTree) || !validGitTree(transaction.FinalCandidateTree) {
		return errors.New("transaction tree identities must be full lowercase Git object IDs")
	}
	for _, identity := range []string{transaction.PathsDigest, transaction.FixDeltaHash, transaction.PolicyHash} {
		if !validSHA256(identity) {
			return errors.New("transaction core hashes must be lowercase SHA-256 identities")
		}
	}
	if transaction.LedgerHash != "" && !validSHA256(transaction.LedgerHash) {
		return errors.New("transaction ledger_hash is invalid")
	}
	if transaction.LedgerFindingsHash != "" && !validSHA256(transaction.LedgerFindingsHash) {
		return errors.New("transaction ledger_findings_hash is invalid")
	}
	if transaction.EvidenceHash != "" && !validSHA256(transaction.EvidenceHash) {
		return errors.New("transaction evidence_hash is invalid")
	}
	if transaction.FailedEvidenceRevision != "" && !validSHA256(transaction.FailedEvidenceRevision) {
		return errors.New("transaction failed_evidence_revision is invalid")
	}
	if transaction.Release != nil {
		if err := validateReleaseEvidence(*transaction.Release); err != nil {
			return err
		}
		if transaction.Release.ReleaseTree != transaction.FinalCandidateTree {
			return errors.New("transaction release tree must match final candidate tree")
		}
	}
	if transaction.Findings == nil || transaction.Classifications == nil || transaction.Outcomes == nil || transaction.FixFindingIDs == nil || transaction.PendingRefuterIDs == nil || transaction.FixCausedFindings == nil || transaction.FollowUps == nil || transaction.JudgeProofs == nil {
		return errors.New("transaction collections must be explicit arrays or objects")
	}
	if transaction.GenesisPaths != nil {
		paths, err := canonicalPaths(transaction.GenesisPaths)
		if err != nil || !equalStrings(paths, transaction.GenesisPaths) {
			return errors.New("genesis paths must be canonical")
		}
		if err := pathsAreSubset(transaction.Snapshot.Paths, transaction.GenesisPaths); err != nil {
			return errors.New("snapshot paths must remain within immutable genesis paths")
		}
	}
	if (transaction.OriginalCriteria == nil) != (transaction.CorrectionRegression == nil) {
		return errors.New("targeted validation checks must be persisted together")
	}
	if transaction.OriginalCriteria != nil {
		result := ScopedValidationResult{OriginalCriteria: *transaction.OriginalCriteria, CorrectionRegression: *transaction.CorrectionRegression}
		if !isOrdinaryMode(transaction.Mode) || transaction.Counters.ScopedFixValidations != 1 || transaction.State == StateFixValidating {
			return errors.New("targeted validation checks require a completed ordinary scoped validation")
		}
		if err := validateTargetedValidation(result, transaction.FixDeltaHash); err != nil {
			return err
		}
	}
	for index, finding := range transaction.FixCausedFindings {
		if err := validateStructuredFinding(finding); err != nil {
			return fmt.Errorf("fix-caused finding[%d]: %w", index, err)
		}
	}
	if err := validateFollowUps(transaction.FollowUps); err != nil {
		return err
	}
	if err := transaction.validateFindingRouting(); err != nil {
		return err
	}
	switch transaction.State {
	case StateUnreviewed, StateReviewing, StateJudgesConfirmed, StateFindingsFrozen, StateEvidenceClassified,
		StateFixRequired, StateFixing, StateFixValidating, StateReadyFinalVerification,
		StateFinalVerifying, StateApproved, StateEscalated:
	default:
		return fmt.Errorf("invalid transaction state %q", transaction.State)
	}
	if err := validateCounters(transaction.Mode, transaction.Counters); err != nil {
		return err
	}
	if err := transaction.validateLensState(); err != nil {
		return err
	}
	if err := transaction.validateJudgeState(); err != nil {
		return err
	}
	if transaction.State == StateFixing || transaction.State == StateFixValidating {
		if !validSHA256(transaction.FailedEvidenceRevision) {
			return errors.New("active fix state requires an exact failed evidence revision")
		}
		if isOrdinaryMode(transaction.Mode) && transaction.Counters.FixBatches != 1 {
			return errors.New("ordinary active fix state requires its single fix batch")
		}
		if transaction.Mode == ModeJudgmentDay && (transaction.Counters.FixRounds < 1 || transaction.Counters.FixRounds > 2) {
			return errors.New("judgment-day active fix state requires a bounded fix round")
		}
	}
	if transaction.State == StateApproved && (!validSHA256(transaction.LedgerHash) || !validSHA256(transaction.EvidenceHash)) {
		return errors.New("approved transaction requires ledger and evidence hashes")
	}
	return nil
}

func (transaction *Transaction) advanceAfterEvidence() {
	if len(transaction.FixFindingIDs) > 0 {
		transaction.State = StateFixRequired
	} else {
		transaction.State = StateReadyFinalVerification
	}
}

func (transaction *Transaction) addFixFinding(id string) {
	for _, existing := range transaction.FixFindingIDs {
		if existing == id {
			return
		}
	}
	transaction.FixFindingIDs = append(transaction.FixFindingIDs, id)
	sort.Strings(transaction.FixFindingIDs)
}

func (transaction *Transaction) invalidTransition(operation string) error {
	return fmt.Errorf("cannot %s from transaction state %q", operation, transaction.State)
}

func (transaction *Transaction) escalateBudget(name string) error {
	transaction.State = StateEscalated
	return fmt.Errorf("%s budget exhausted", name)
}

func validateSnapshot(snapshot Snapshot) error {
	if snapshot.Kind == "" || !validGitTree(snapshot.BaseTree) || !validGitTree(snapshot.CandidateTree) {
		return errors.New("snapshot requires kind, base_tree, and candidate_tree")
	}
	for _, value := range []string{snapshot.PathsDigest, snapshot.IntendedUntrackedProof, snapshot.Identity} {
		if !validSHA256(value) {
			return errors.New("snapshot digests must be lowercase SHA-256 identities")
		}
	}
	if snapshot.IntendedUntracked == nil || snapshot.Paths == nil {
		return errors.New("snapshot path lists must be explicit arrays")
	}
	return nil
}

func validateCounters(mode Mode, counters Counters) error {
	values := []int{counters.FullReviews, counters.RefuterBatches, counters.FixBatches, counters.ScopedFixValidations, counters.FinalVerifications, counters.FixRounds, counters.ScopedRejudgments, counters.JudgeExecutions, counters.RiskExecutions, counters.ResilienceExecutions, counters.ReadabilityExecutions, counters.ReliabilityExecutions}
	for _, value := range values {
		if value < 0 {
			return errors.New("review counters cannot be negative")
		}
	}
	switch mode {
	case ModeOrdinary4R, ModeOrdinaryBounded:
		if counters.FullReviews > 1 || counters.RefuterBatches > 1 || counters.FixBatches > 1 || counters.ScopedFixValidations > 1 || counters.FinalVerifications > 1 || counters.FixRounds != 0 || counters.ScopedRejudgments != 0 || counters.JudgeExecutions != 0 || counters.RiskExecutions > 1 || counters.ResilienceExecutions > 1 || counters.ReadabilityExecutions > 1 || counters.ReliabilityExecutions > 1 {
			return errors.New("ordinary review budget exceeded")
		}
		if mode == ModeOrdinary4R && (counters.RiskExecutions != 0 || counters.ResilienceExecutions != 0 || counters.ReadabilityExecutions != 0 || counters.ReliabilityExecutions != 0) {
			return errors.New("ordinary_4r cannot contain native lens execution counters")
		}
		if mode == ModeOrdinaryBounded && counters.FullReviews != 0 {
			return errors.New("ordinary_bounded cannot consume the legacy full review counter")
		}
	case ModeJudgmentDay:
		if counters.FixRounds > 2 || counters.ScopedRejudgments > 2 || counters.RefuterBatches != 0 || counters.FixBatches != 0 || counters.ScopedFixValidations != 0 || counters.FinalVerifications > 1 || (counters.JudgeExecutions != 0 && counters.JudgeExecutions != 2) || counters.RiskExecutions != 0 || counters.ResilienceExecutions != 0 || counters.ReadabilityExecutions != 0 || counters.ReliabilityExecutions != 0 {
			return errors.New("judgment_day budget exceeded")
		}
	}
	return nil
}

func validateSelectedLenses(mode Mode, riskLevel RiskLevel, lenses []string) ([]string, error) {
	if mode != ModeOrdinaryBounded {
		if riskLevel != "" || len(lenses) != 0 {
			return nil, errors.New("selected lenses require ordinary_bounded mode")
		}
		return nil, nil
	}
	validated := append([]string(nil), lenses...)
	want := -1
	switch riskLevel {
	case RiskLow:
		want = 0
	case RiskMedium:
		want = 1
	case RiskHigh:
		want = len(supportedLenses)
	default:
		return nil, errors.New("ordinary_bounded requires a native low, medium, or high risk classification")
	}
	if len(validated) != want {
		return nil, fmt.Errorf("ordinary_bounded %s risk requires exactly %d selected lenses", riskLevel, want)
	}
	for index, lens := range validated {
		if strings.TrimSpace(lens) != lens || !isSupportedLens(lens) {
			return nil, fmt.Errorf("unknown review lens %q", lens)
		}
		if len(validated) == len(supportedLenses) && lens != supportedLenses[index] {
			return nil, errors.New("the full selected lens set must use canonical 4R order")
		}
	}
	return validated, nil
}

func (transaction *Transaction) validateLensState() error {
	if transaction.Mode != ModeOrdinaryBounded {
		if len(transaction.SelectedLenses) != 0 || len(transaction.LensResults) != 0 {
			return errors.New("native lens state requires ordinary_bounded mode")
		}
		return nil
	}
	selected, err := validateSelectedLenses(transaction.Mode, transaction.RiskLevel, transaction.SelectedLenses)
	if err != nil || !equalStrings(selected, transaction.SelectedLenses) {
		return errors.New("ordinary_bounded selected lenses are invalid")
	}
	if len(transaction.LensResults) > len(selected) {
		return errors.New("ordinary_bounded has more lens results than selected lenses")
	}
	for index, result := range transaction.LensResults {
		validated, validationErr := validateLensResult(result)
		if validationErr != nil || result.Lens != selected[index] || result.ResultHash != validated.ResultHash || transaction.lensExecutions(result.Lens) != 1 {
			return errors.New("ordinary_bounded lens results must be complete, unique, and canonically ordered")
		}
		for previous := 0; previous < index; previous++ {
			if transaction.LensResults[previous].ResultHash == result.ResultHash {
				return errors.New("ordinary_bounded lens result identities must be distinct")
			}
		}
	}
	for _, lens := range supportedLenses {
		want := 0
		if stringIndexLensResult(transaction.LensResults, lens) >= 0 {
			want = 1
		}
		if transaction.lensExecutions(lens) != want {
			return errors.New("ordinary_bounded lens execution counters do not match completed results")
		}
	}
	if transaction.State != StateUnreviewed && transaction.State != StateReviewing {
		if len(transaction.LensResults) != len(selected) {
			return errors.New("ordinary_bounded cannot leave reviewing before every selected lens is complete")
		}
		if !reflectLensFindings(transaction.LensResults, transaction.Findings) {
			return errors.New("ordinary_bounded frozen ledger does not match completed lens results")
		}
	}
	return nil
}

func reflectLensFindings(results []LensResult, findings []Finding) bool {
	merged := make([]Finding, 0)
	for _, result := range results {
		merged = append(merged, result.Findings...)
	}
	left, _ := json.Marshal(merged)
	right, _ := json.Marshal(findings)
	return bytes.Equal(left, right)
}

func isOrdinaryMode(mode Mode) bool {
	return mode == ModeOrdinary4R || mode == ModeOrdinaryBounded
}

func isSupportedLens(lens string) bool {
	return stringIndex(supportedLenses, lens) >= 0
}

func stringIndex(values []string, value string) int {
	for index, candidate := range values {
		if candidate == value {
			return index
		}
	}
	return -1
}

func stringIndexLensResult(results []LensResult, lens string) int {
	for index, result := range results {
		if result.Lens == lens {
			return index
		}
	}
	return -1
}

func (transaction *Transaction) lensExecutions(lens string) int {
	switch lens {
	case LensRisk:
		return transaction.Counters.RiskExecutions
	case LensResilience:
		return transaction.Counters.ResilienceExecutions
	case LensReadability:
		return transaction.Counters.ReadabilityExecutions
	case LensReliability:
		return transaction.Counters.ReliabilityExecutions
	default:
		return 0
	}
}

func (transaction *Transaction) setLensExecutions(lens string, value int) {
	setLensCounter(&transaction.Counters, lens, value)
}

func setLensCounter(counters *Counters, lens string, value int) {
	switch lens {
	case LensRisk:
		counters.RiskExecutions = value
	case LensResilience:
		counters.ResilienceExecutions = value
	case LensReadability:
		counters.ReadabilityExecutions = value
	case LensReliability:
		counters.ReliabilityExecutions = value
	}
}

func validateJudgeProofs(proofs []JudgeProof, agreementHash string) ([]JudgeProof, string, error) {
	if len(proofs) != 2 || !validSHA256(agreementHash) {
		return nil, "", errors.New("Judgment Day requires exactly two blind confirmed judge results and an agreement hash")
	}
	validated := append([]JudgeProof(nil), proofs...)
	sort.Slice(validated, func(i, j int) bool { return validated[i].JudgeID < validated[j].JudgeID })
	seenJudges := map[string]struct{}{}
	seenExecutions := map[string]struct{}{}
	seenResults := map[string]struct{}{}
	hasher := sha256.New()
	hasher.Write([]byte("gentle-ai.judgment-day-proof/v1\x00"))
	for _, proof := range validated {
		proof.JudgeID = strings.TrimSpace(proof.JudgeID)
		if proof.JudgeID == "" || !validSHA256(proof.ExecutionHash) || !validSHA256(proof.ResultHash) || !proof.Blind || !proof.Confirmed {
			return nil, "", errors.New("each Judgment Day judge proof must be distinct, blind, confirmed, and content-hashed")
		}
		if _, exists := seenJudges[proof.JudgeID]; exists {
			return nil, "", errors.New("Judgment Day judge identities must be distinct")
		}
		if _, exists := seenExecutions[proof.ExecutionHash]; exists {
			return nil, "", errors.New("Judgment Day judge execution proofs must be distinct")
		}
		if _, exists := seenResults[proof.ResultHash]; exists {
			return nil, "", errors.New("Judgment Day judge result proofs must be distinct")
		}
		seenJudges[proof.JudgeID] = struct{}{}
		seenExecutions[proof.ExecutionHash] = struct{}{}
		seenResults[proof.ResultHash] = struct{}{}
		for _, value := range []string{proof.JudgeID, proof.ExecutionHash, proof.ResultHash} {
			writeLengthPrefixed(hasher, []byte(value))
		}
	}
	writeLengthPrefixed(hasher, []byte(agreementHash))
	return validated, "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func (transaction *Transaction) validateJudgeState() error {
	if isOrdinaryMode(transaction.Mode) {
		if len(transaction.JudgeProofs) != 0 || transaction.JudgeProofHash != "" || transaction.JudgeAgreementHash != "" || transaction.Counters.JudgeExecutions != 0 || transaction.State == StateJudgesConfirmed {
			return errors.New("ordinary review cannot contain Judgment Day judge proof")
		}
		return nil
	}
	if transaction.State == StateUnreviewed || transaction.State == StateReviewing {
		if len(transaction.JudgeProofs) != 0 || transaction.Counters.JudgeExecutions != 0 || transaction.JudgeProofHash != "" || transaction.JudgeAgreementHash != "" {
			return errors.New("unconfirmed Judgment Day state cannot contain partial judge proof")
		}
		return nil
	}
	proofs, proofHash, err := validateJudgeProofs(transaction.JudgeProofs, transaction.JudgeAgreementHash)
	if err != nil || proofHash != transaction.JudgeProofHash || len(proofs) != transaction.Counters.JudgeExecutions {
		return errors.New("Judgment Day state requires exactly two immutable distinct judge proofs")
	}
	return nil
}

func (transaction *Transaction) validateFindingRouting() error {
	findings := make(map[string]Finding, len(transaction.Findings))
	severe := make(map[string]Finding, len(transaction.Findings))
	for _, finding := range transaction.Findings {
		if strings.TrimSpace(finding.ID) == "" || !isSupportedSeverity(finding.Severity) {
			return errors.New("transaction findings require IDs and supported severities")
		}
		if _, duplicate := findings[finding.ID]; duplicate {
			return fmt.Errorf("duplicate transaction finding %q", finding.ID)
		}
		findings[finding.ID] = finding
		if isSevereSeverity(finding.Severity) {
			severe[finding.ID] = finding
		}
		outcome, hasOutcome := transaction.Outcomes[finding.ID]
		if !isSevereSeverity(finding.Severity) {
			if !hasOutcome || outcome != OutcomeInfo {
				return fmt.Errorf("informational finding %q must remain info", finding.ID)
			}
		} else if hasOutcome {
			switch outcome {
			case OutcomeCorroborated, OutcomeRefuted, OutcomeInconclusive:
			default:
				return fmt.Errorf("severe finding %q has invalid outcome %q", finding.ID, outcome)
			}
		}
	}
	for id, classification := range transaction.Classifications {
		finding, ok := findings[id]
		if !ok || !isSevereSeverity(finding.Severity) || classification.FindingID != id || !isConcreteEvidence(classification.Proof) {
			return fmt.Errorf("evidence classification %q is not bound to a frozen severe finding", id)
		}
		switch classification.Class {
		case EvidenceDeterministic, EvidenceInferential, EvidenceInsufficient:
		default:
			return fmt.Errorf("evidence classification %q has invalid class %q", id, classification.Class)
		}
	}
	for id := range transaction.Outcomes {
		if _, ok := findings[id]; !ok {
			return fmt.Errorf("outcome %q is not bound to a frozen finding", id)
		}
	}
	fixCaused := make(map[string]Finding, len(transaction.FixCausedFindings))
	for _, finding := range transaction.FixCausedFindings {
		fixCaused[finding.ID] = finding
	}
	for _, id := range append(append([]string{}, transaction.FixFindingIDs...), transaction.PendingRefuterIDs...) {
		finding, ok := findings[id]
		if !ok {
			finding, ok = fixCaused[id]
		}
		if !ok || !isSevereSeverity(finding.Severity) {
			return fmt.Errorf("routed finding %q is not BLOCKER or CRITICAL", id)
		}
	}
	fixIDs, err := canonicalStrings(transaction.FixFindingIDs, "fix finding id")
	if err != nil || !equalStrings(fixIDs, transaction.FixFindingIDs) {
		return errors.New("fix finding IDs must be unique and canonical")
	}
	pendingIDs, err := canonicalStrings(transaction.PendingRefuterIDs, "pending refuter id")
	if err != nil || !equalStrings(pendingIDs, transaction.PendingRefuterIDs) {
		return errors.New("pending refuter IDs must be unique and canonical")
	}
	if hasStringIntersection(fixIDs, pendingIDs) {
		return errors.New("a severe finding cannot be both correction-bound and pending refutation")
	}
	if transaction.State != StateUnreviewed && transaction.State != StateReviewing && transaction.State != StateJudgesConfirmed && (!validSHA256(transaction.LedgerHash) || !validSHA256(transaction.LedgerFindingsHash) || transaction.LedgerFindingsHash != findingsHash(transaction.Findings)) {
		return errors.New("frozen review state requires a content-bound ledger hash")
	}
	return transaction.validateFindingState(findings, severe)
}

func findingsHash(findings []Finding) string {
	payload, _ := json.Marshal(findings)
	sum := sha256.Sum256(append([]byte("gentle-ai.review-ledger-findings/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (transaction *Transaction) validateFindingState(findings, severe map[string]Finding) error {
	beforeFreeze := transaction.State == StateUnreviewed || transaction.State == StateReviewing || transaction.State == StateJudgesConfirmed
	if beforeFreeze {
		if len(findings) != 0 || len(transaction.Classifications) != 0 || len(transaction.Outcomes) != 0 || len(transaction.FixFindingIDs) != 0 || len(transaction.PendingRefuterIDs) != 0 {
			return errors.New("pre-freeze transaction cannot contain findings or evidence routing")
		}
		return nil
	}
	if transaction.State == StateFindingsFrozen {
		if len(transaction.Classifications) != 0 || len(transaction.FixFindingIDs) != 0 || len(transaction.PendingRefuterIDs) != 0 {
			return errors.New("findings_frozen state cannot contain classified or routed severe findings")
		}
		for id := range severe {
			if _, exists := transaction.Outcomes[id]; exists {
				return fmt.Errorf("frozen severe finding %q cannot have an outcome before classification", id)
			}
		}
		return nil
	}
	if transaction.State == StateEscalated && transaction.LedgerHash == "" {
		if len(findings) != 0 || len(transaction.Classifications) != 0 || len(transaction.Outcomes) != 0 || len(transaction.FixFindingIDs) != 0 || len(transaction.PendingRefuterIDs) != 0 {
			return errors.New("pre-freeze escalation cannot contain findings or evidence routing")
		}
		return nil
	}

	fixSet := stringSet(transaction.FixFindingIDs)
	pendingSet := stringSet(transaction.PendingRefuterIDs)
	corroborated := make(map[string]struct{})
	hasInsufficient := false
	hasResolvedInferential := false
	for id := range severe {
		classification, classified := transaction.Classifications[id]
		if !classified {
			return fmt.Errorf("severe finding %q must retain one evidence classification", id)
		}
		outcome, hasOutcome := transaction.Outcomes[id]
		_, fix := fixSet[id]
		_, pending := pendingSet[id]
		switch classification.Class {
		case EvidenceDeterministic:
			if !hasOutcome || outcome != OutcomeCorroborated || !fix || pending {
				return fmt.Errorf("deterministic severe finding %q must remain corroborated and correction-bound", id)
			}
			corroborated[id] = struct{}{}
		case EvidenceInferential:
			if transaction.Mode == ModeJudgmentDay {
				if !hasOutcome || outcome != OutcomeCorroborated || !fix || pending {
					return fmt.Errorf("Judgment Day inferential finding %q must remain corroborated and correction-bound", id)
				}
				corroborated[id] = struct{}{}
				continue
			}
			if pending {
				if transaction.State != StateEvidenceClassified || hasOutcome || transaction.Counters.RefuterBatches != 0 {
					return fmt.Errorf("inferential finding %q is pending outside the single unconsumed refuter state", id)
				}
				continue
			}
			if !hasOutcome || (outcome != OutcomeCorroborated && outcome != OutcomeRefuted && outcome != OutcomeInconclusive) {
				return fmt.Errorf("resolved inferential finding %q requires a refuter outcome", id)
			}
			hasResolvedInferential = true
			if outcome == OutcomeCorroborated {
				if !fix {
					return fmt.Errorf("corroborated inferential finding %q must remain correction-bound", id)
				}
				corroborated[id] = struct{}{}
			} else if fix {
				return fmt.Errorf("non-corroborated inferential finding %q cannot enter correction", id)
			}
		case EvidenceInsufficient:
			hasInsufficient = true
			if !hasOutcome || outcome != OutcomeInconclusive || fix || pending || transaction.State != StateEscalated {
				return fmt.Errorf("insufficient severe finding %q must terminally escalate as inconclusive", id)
			}
		}
	}
	if len(transaction.Classifications) != len(severe) {
		return errors.New("evidence classification must cover every frozen severe finding exactly once")
	}
	for _, finding := range transaction.FixCausedFindings {
		if isSevereSeverity(finding.Severity) {
			corroborated[finding.ID] = struct{}{}
		}
	}
	if len(fixSet) != len(corroborated) {
		return errors.New("correction IDs must equal all and only corroborated severe findings")
	}
	if transaction.State == StateEvidenceClassified && len(pendingSet) == 0 {
		return errors.New("evidence_classified state requires pending inferential findings")
	}
	if transaction.State != StateEvidenceClassified && len(pendingSet) != 0 {
		return errors.New("pending refuter findings cannot survive outside evidence_classified")
	}
	if transaction.State != StateEscalated {
		for id, outcome := range transaction.Outcomes {
			if _, severeFinding := severe[id]; severeFinding && outcome == OutcomeInconclusive {
				return fmt.Errorf("inconclusive severe finding %q requires terminal escalation", id)
			}
		}
	}
	if isOrdinaryMode(transaction.Mode) && hasResolvedInferential && !hasInsufficient && transaction.Counters.RefuterBatches != 1 {
		return errors.New("resolved ordinary inferential findings require exactly one consumed refuter batch")
	}
	return transaction.validateResolutionCounters(len(corroborated) > 0)
}

func (transaction *Transaction) validateResolutionCounters(hasCorrections bool) error {
	switch transaction.State {
	case StateFixRequired:
		if isOrdinaryMode(transaction.Mode) && (transaction.Counters.FixBatches != 0 || transaction.Counters.ScopedFixValidations != 0) {
			return errors.New("ordinary fix_required state cannot pre-consume correction or validation")
		}
	case StateFixing, StateFixValidating:
		if !hasCorrections {
			return errors.New("active correction state requires corroborated severe findings")
		}
	case StateReadyFinalVerification, StateFinalVerifying, StateApproved:
		if len(transaction.PendingRefuterIDs) != 0 {
			return errors.New("final verification states cannot contain pending refuter IDs")
		}
		switch transaction.Mode {
		case ModeOrdinary4R, ModeOrdinaryBounded:
			want := 0
			if hasCorrections {
				want = 1
			}
			if transaction.Counters.FixBatches != want || transaction.Counters.ScopedFixValidations != want {
				return errors.New("ordinary final verification readiness requires coherent correction and scoped-validation counters")
			}
		case ModeJudgmentDay:
			if hasCorrections {
				if transaction.Counters.FixRounds < 1 || transaction.Counters.FixRounds != transaction.Counters.ScopedRejudgments {
					return errors.New("Judgment Day final verification readiness requires coherent fix and scoped re-judgment counters")
				}
			} else if transaction.Counters.FixRounds != 0 || transaction.Counters.ScopedRejudgments != 0 {
				return errors.New("uncorrected Judgment Day readiness cannot consume fix counters")
			}
		}
		if hasCorrections {
			wrongBase := isOrdinaryMode(transaction.Mode) && transaction.Snapshot.BaseTree != transaction.InitialReviewTree
			if !validSHA256(transaction.FailedEvidenceRevision) || transaction.Snapshot.Kind != TargetFixDiff || wrongBase || transaction.Snapshot.CandidateTree != transaction.FinalCandidateTree || !equalStrings(transaction.Snapshot.LedgerIDs, transaction.FixFindingIDs) || transaction.FixDeltaHash != FixDeltaHashForSnapshot(transaction.Snapshot) {
				return errors.New("corrected final verification readiness requires a complete ledger-bound fix snapshot")
			}
		}
		if transaction.State == StateReadyFinalVerification && transaction.Counters.FinalVerifications != 0 {
			return errors.New("ready_final_verification cannot pre-consume final verification")
		}
		if (transaction.State == StateFinalVerifying || transaction.State == StateApproved) && transaction.Counters.FinalVerifications != 1 {
			return errors.New("final_verifying and approved require exactly one final verification")
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func hasStringIntersection(left, right []string) bool {
	set := stringSet(left)
	for _, value := range right {
		if _, exists := set[value]; exists {
			return true
		}
	}
	return false
}

func isSupportedSeverity(severity string) bool {
	switch severity {
	case "BLOCKER", "CRITICAL", "WARNING", "SUGGESTION":
		return true
	default:
		return false
	}
}

func isSevereSeverity(severity string) bool {
	return severity == "BLOCKER" || severity == "CRITICAL"
}

func cloneClassifications(source map[string]FindingEvidence) map[string]FindingEvidence {
	cloned := make(map[string]FindingEvidence, len(source))
	for id, value := range source {
		cloned[id] = value
	}
	return cloned
}

func cloneOutcomes(source map[string]EvidenceOutcome) map[string]EvidenceOutcome {
	cloned := make(map[string]EvidenceOutcome, len(source))
	for id, value := range source {
		cloned[id] = value
	}
	return cloned
}

func addUniqueSorted(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	values = append(values, value)
	sort.Strings(values)
	return values
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func isConcreteEvidence(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.ContainsAny(trimmed, "{}<>") {
		return false
	}
	switch strings.ToLower(trimmed) {
	case "n/a", "na", "none", "todo", "tbd", "pass", "passed", "success", "placeholder":
		return false
	}
	return true
}

func validateStructuredFinding(finding Finding) error {
	if strings.TrimSpace(finding.ID) == "" || strings.TrimSpace(finding.Lens) == "" || strings.TrimSpace(finding.Location) == "" || strings.TrimSpace(finding.Severity) == "" || strings.TrimSpace(finding.Claim) == "" {
		return errors.New("id, lens, location, severity, and neutral claim are required")
	}
	if len(finding.ProofRefs) == 0 {
		return errors.New("at least one proof reference is required")
	}
	if !isSupportedSeverity(finding.Severity) {
		return errors.New("severity must be BLOCKER, CRITICAL, WARNING, or SUGGESTION")
	}
	for _, proof := range finding.ProofRefs {
		if !isConcreteEvidence(proof) {
			return errors.New("proof references must be concrete")
		}
	}
	return nil
}
