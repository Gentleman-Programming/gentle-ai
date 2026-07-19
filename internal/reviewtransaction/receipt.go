package reviewtransaction

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const ReceiptSchema = "gentle-ai.review-receipt/v1"

var EmptyFixDeltaHash = func() string {
	sum := sha256.Sum256(nil)
	return "sha256:" + hex.EncodeToString(sum[:])
}()

type TerminalState string

const (
	TerminalApproved  TerminalState = "approved"
	TerminalEscalated TerminalState = "escalated"
)

type Receipt struct {
	Schema             string           `json:"schema"`
	LineageID          string           `json:"lineage_id"`
	Mode               Mode             `json:"mode"`
	Generation         int              `json:"generation"`
	BaseTree           string           `json:"base_tree"`
	InitialReviewTree  string           `json:"initial_review_tree"`
	FinalCandidateTree string           `json:"final_candidate_tree"`
	PathsDigest        string           `json:"paths_digest"`
	FixDeltaHash       string           `json:"fix_delta_hash"`
	PolicyHash         string           `json:"policy_hash"`
	LedgerHash         string           `json:"ledger_hash"`
	EvidenceHash       string           `json:"evidence_hash"`
	JudgeProofHash     string           `json:"judge_proof_hash,omitempty"`
	Release            *ReleaseEvidence `json:"release,omitempty"`
	Counters           Counters         `json:"counters"`
	RiskLevel          RiskLevel        `json:"risk_level,omitempty"`
	SelectedLenses     []string         `json:"selected_lenses,omitempty"`
	LensResults        []LensResult     `json:"lens_results,omitempty"`
	TerminalState      TerminalState    `json:"terminal_state"`
}

type PublicationState string

const PublicationStateSealed PublicationState = "sealed"

type EvidenceFreshnessState string

const EvidenceFreshnessCurrent EvidenceFreshnessState = "current"

type ReleaseEvidence struct {
	ReleaseTree             string                 `json:"release_tree"`
	ConfigurationHash       string                 `json:"configuration_hash"`
	GeneratedArtifactHash   string                 `json:"generated_artifact_hash"`
	ProvenanceHash          string                 `json:"provenance_hash"`
	PublicationBoundaryHash string                 `json:"publication_boundary_hash"`
	PublicationState        PublicationState       `json:"publication_state"`
	EvidenceFreshnessHash   string                 `json:"evidence_freshness_hash"`
	EvidenceFreshnessState  EvidenceFreshnessState `json:"evidence_freshness_state"`
}

func (transaction *Transaction) Receipt() (Receipt, error) {
	var terminal TerminalState
	switch transaction.State {
	case StateApproved:
		terminal = TerminalApproved
	case StateEscalated:
		terminal = TerminalEscalated
	default:
		return Receipt{}, errors.New("receipt requires a terminal approved or escalated transaction")
	}
	ledgerHash := transaction.LedgerHash
	if ledgerHash == "" {
		ledgerHash = EmptyFixDeltaHash
	}
	evidenceHash := transaction.EvidenceHash
	if evidenceHash == "" {
		evidenceHash = EmptyFixDeltaHash
	}
	receipt := Receipt{
		Schema: ReceiptSchema, LineageID: transaction.LineageID, Mode: transaction.Mode,
		Generation: transaction.Generation, BaseTree: transaction.BaseTree,
		InitialReviewTree: transaction.InitialReviewTree, FinalCandidateTree: transaction.FinalCandidateTree,
		PathsDigest: transaction.PathsDigest, FixDeltaHash: transaction.FixDeltaHash,
		PolicyHash: transaction.PolicyHash, LedgerHash: ledgerHash, EvidenceHash: evidenceHash,
		JudgeProofHash: transaction.JudgeProofHash, Release: cloneReleaseEvidence(transaction.Release),
		Counters: transaction.Counters, RiskLevel: transaction.RiskLevel, SelectedLenses: append([]string(nil), transaction.SelectedLenses...),
		LensResults: append([]LensResult(nil), transaction.LensResults...), TerminalState: terminal,
	}
	if err := validateReceiptStructure(receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func ParseReceipt(payload []byte) (Receipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var receipt Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Receipt{}, errors.New("multiple JSON values in review receipt")
	}
	if err := validateReceiptStructure(receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

type GateResult string

const (
	GateAllow        GateResult = "allow"
	GateScopeChanged GateResult = "scope-changed"
	GateInvalidated  GateResult = "invalidated"
	GateEscalated    GateResult = "escalated"
)

type GateKind string

const (
	GatePostApply GateKind = "post-apply"
	GatePreCommit GateKind = "pre-commit"
	GatePrePush   GateKind = "pre-push"
	GatePrePR     GateKind = "pre-pr"
	GateRelease   GateKind = "release"
)

type ExternalEvidenceDisposition string

const (
	ExternalEvidenceNone         ExternalEvidenceDisposition = ""
	ExternalEvidenceInvalidating ExternalEvidenceDisposition = "invalidating"
	ExternalEvidenceEscalating   ExternalEvidenceDisposition = "escalating"
)

type GateContext struct {
	Gate                  GateKind                    `json:"gate"`
	LineageID             string                      `json:"lineage_id"`
	Generation            int                         `json:"generation"`
	StoreRevision         string                      `json:"store_revision,omitempty"`
	GenesisRevision       string                      `json:"genesis_revision,omitempty"`
	ChainIdentity         string                      `json:"chain_identity,omitempty"`
	BundleDigest          string                      `json:"bundle_digest,omitempty"`
	BaseTree              string                      `json:"base_tree"`
	CandidateTree         string                      `json:"candidate_tree"`
	PathsDigest           string                      `json:"paths_digest"`
	FixDeltaHash          string                      `json:"fix_delta_hash"`
	PolicyHash            string                      `json:"policy_hash"`
	LedgerHash            string                      `json:"ledger_hash"`
	EvidenceHash          string                      `json:"evidence_hash"`
	BaseRelationshipValid bool                        `json:"base_relationship_valid"`
	ExternalEvidence      ExternalEvidenceDisposition `json:"external_evidence,omitempty"`
	BaseAdvance           *BaseAdvanceCompatibility   `json:"base_advanced_compatible,omitempty"`
	Release               *ReleaseEvidence            `json:"release,omitempty"`
	PrePRBoundary         *PrePRBoundarySelection     `json:"pre_pr_boundary,omitempty"`
	Denial                *GateDenial                 `json:"denial,omitempty"`
	ScopeChange           *GateScopeChangeDiagnostics `json:"scope_change,omitempty"`
}

// GateDenial identifies the non-authorizing validation stage that rejected a
// gate request. Its presence never changes the gate result.
type GateDenial struct {
	Stage string `json:"stage"`
	Code  string `json:"code"`
}

type GateTargetEvidence struct {
	BaseTree      string   `json:"base_tree"`
	CandidateTree string   `json:"candidate_tree"`
	PathsDigest   string   `json:"paths_digest"`
	Paths         []string `json:"paths"`
}

type GateScopeChangeDiagnostics struct {
	Expected               GateTargetEvidence `json:"expected"`
	Actual                 GateTargetEvidence `json:"actual"`
	DifferingPaths         []string           `json:"differing_paths"`
	DifferingPathCount     int                `json:"differing_path_count"`
	DifferingPathsDigest   string             `json:"differing_paths_digest"`
	PredecessorLineageID   string             `json:"predecessor_lineage_id"`
	PredecessorRevision    string             `json:"predecessor_revision"`
	RecoveryOperation      string             `json:"recovery_operation"`
	RecoveryRequiredInputs []string           `json:"recovery_required_inputs"`
}

func validateDerivedGate(receipt Receipt, context GateContext) GateResult {
	if err := validateReceiptStructure(receipt); err != nil {
		return GateInvalidated
	}
	if receipt.TerminalState == TerminalEscalated || context.ExternalEvidence == ExternalEvidenceEscalating {
		return GateEscalated
	}
	if receipt.TerminalState != TerminalApproved {
		return GateInvalidated
	}
	compatibleAdvance := context.Gate == GatePrePR && context.BaseAdvance != nil && context.BaseAdvance.Compatible
	if receipt.LineageID != context.LineageID || receipt.Generation != context.Generation {
		return GateScopeChanged
	}
	if (receipt.FinalCandidateTree != context.CandidateTree || receipt.PathsDigest != context.PathsDigest) && !compatibleAdvance {
		return GateScopeChanged
	}
	if context.ExternalEvidence == ExternalEvidenceInvalidating {
		return GateInvalidated
	}
	if (receipt.BaseTree != context.BaseTree && !compatibleAdvance) || receipt.FixDeltaHash != context.FixDeltaHash ||
		receipt.PolicyHash != context.PolicyHash || receipt.LedgerHash != context.LedgerHash ||
		receipt.EvidenceHash != context.EvidenceHash {
		return GateInvalidated
	}
	if (context.Gate == GatePrePR || context.Gate == GateRelease) && !context.BaseRelationshipValid && !compatibleAdvance {
		return GateInvalidated
	}
	if context.Gate == GateRelease {
		if receipt.Release == nil || context.Release == nil || *receipt.Release != *context.Release {
			return GateInvalidated
		}
		if err := validateReleaseEvidence(*context.Release); err != nil || context.Release.ReleaseTree != context.CandidateTree {
			return GateInvalidated
		}
	}
	switch context.Gate {
	case GatePostApply, GatePreCommit, GatePrePush, GatePrePR, GateRelease:
		return GateAllow
	default:
		return GateInvalidated
	}
}

func ParseGateContext(payload []byte) (GateContext, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var context GateContext
	if err := decoder.Decode(&context); err != nil {
		return GateContext{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return GateContext{}, errors.New("multiple JSON values in review gate context")
	}
	if validateLineageID(context.LineageID) != nil || context.Generation < 1 || !validGitTree(context.BaseTree) || !validGitTree(context.CandidateTree) {
		return GateContext{}, errors.New("gate context requires lineage, generation, base tree, and candidate tree")
	}
	for _, identity := range []string{context.StoreRevision, context.GenesisRevision, context.ChainIdentity, context.BundleDigest, context.PathsDigest, context.FixDeltaHash, context.PolicyHash, context.LedgerHash, context.EvidenceHash} {
		if !validSHA256(identity) {
			return GateContext{}, errors.New("gate context hashes must be lowercase SHA-256 identities")
		}
	}
	if context.Release != nil {
		if err := validateReleaseEvidence(*context.Release); err != nil {
			return GateContext{}, err
		}
	}
	if context.BaseAdvance != nil {
		if context.Gate != GatePrePR || !context.BaseAdvance.valid() {
			return GateContext{}, errors.New("gate context contains invalid compatible base advance evidence")
		}
	}
	if context.PrePRBoundary != nil {
		boundary := context.PrePRBoundary
		unavailable := boundary.Commit == "" && context.Denial != nil && context.Denial.Stage == "boundary-selection" && context.Denial.Code == "unavailable"
		if context.Gate != GatePrePR || (!validGitTree(boundary.Commit) && !unavailable) || strings.TrimSpace(boundary.Selector) == "" ||
			(boundary.Source != PrePRBoundaryExplicit && boundary.Source != PrePRBoundaryPublicationDefault) {
			return GateContext{}, errors.New("gate context contains invalid pre-PR boundary evidence")
		}
	}
	if context.Denial != nil && (strings.TrimSpace(context.Denial.Stage) == "" || strings.TrimSpace(context.Denial.Code) == "") {
		return GateContext{}, errors.New("gate context denial requires stage and code")
	}
	switch context.ExternalEvidence {
	case ExternalEvidenceNone, ExternalEvidenceInvalidating, ExternalEvidenceEscalating:
	default:
		return GateContext{}, fmt.Errorf("invalid external evidence disposition %q", context.ExternalEvidence)
	}
	return context, nil
}

var gitTreePattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func validateReceiptStructure(receipt Receipt) error {
	if receipt.Schema != ReceiptSchema {
		return errors.New("unsupported review receipt schema")
	}
	if err := validateLineageID(receipt.LineageID); err != nil {
		return err
	}
	if receipt.Generation < 1 {
		return errors.New("receipt requires a positive generation")
	}
	if receipt.Mode != ModeOrdinary4R && receipt.Mode != ModeOrdinaryBounded && receipt.Mode != ModeJudgmentDay {
		return fmt.Errorf("invalid receipt mode %q", receipt.Mode)
	}
	for _, tree := range []string{receipt.BaseTree, receipt.InitialReviewTree, receipt.FinalCandidateTree} {
		if !validGitTree(tree) {
			return errors.New("receipt tree identities must be full lowercase Git object IDs")
		}
	}
	for _, identity := range []string{receipt.PathsDigest, receipt.FixDeltaHash, receipt.PolicyHash, receipt.LedgerHash, receipt.EvidenceHash} {
		if !validSHA256(identity) {
			return errors.New("receipt hashes must be lowercase SHA-256 identities")
		}
	}
	if err := validateCounters(receipt.Mode, receipt.Counters); err != nil {
		return err
	}
	lensState := Transaction{Mode: receipt.Mode, State: StateApproved, RiskLevel: receipt.RiskLevel, SelectedLenses: receipt.SelectedLenses, LensResults: receipt.LensResults, Counters: receipt.Counters, Findings: mergedLensFindings(receipt.LensResults)}
	if err := lensState.validateLensState(); err != nil {
		return err
	}
	switch receipt.TerminalState {
	case TerminalApproved:
		if receipt.Counters.FinalVerifications != 1 {
			return errors.New("approved receipt requires exactly one independent final verification")
		}
		switch receipt.Mode {
		case ModeOrdinary4R:
			if receipt.Counters.FullReviews != 1 || receipt.Counters.FixBatches != receipt.Counters.ScopedFixValidations {
				return errors.New("approved ordinary receipt has incoherent review/fix counters")
			}
		case ModeOrdinaryBounded:
			if len(receipt.LensResults) != len(receipt.SelectedLenses) || receipt.Counters.FixBatches != receipt.Counters.ScopedFixValidations {
				return errors.New("approved ordinary bounded receipt has incomplete lenses or incoherent fix counters")
			}
		case ModeJudgmentDay:
			if receipt.Counters.FixRounds != receipt.Counters.ScopedRejudgments {
				return errors.New("approved judgment-day receipt has incoherent fix/re-judgment counters")
			}
			if receipt.Counters.JudgeExecutions != 2 || !validSHA256(receipt.JudgeProofHash) {
				return errors.New("approved judgment-day receipt requires two immutable judge proofs")
			}
		}
	case TerminalEscalated:
	default:
		return fmt.Errorf("invalid terminal_state %q", receipt.TerminalState)
	}
	if isOrdinaryMode(receipt.Mode) && receipt.JudgeProofHash != "" {
		return errors.New("ordinary receipt cannot contain Judgment Day proof")
	}
	if receipt.Release != nil {
		if err := validateReleaseEvidence(*receipt.Release); err != nil {
			return err
		}
		if receipt.Release.ReleaseTree != receipt.FinalCandidateTree {
			return errors.New("receipt release tree must match final candidate tree")
		}
	}
	return nil
}

func mergedLensFindings(results []LensResult) []Finding {
	findings := make([]Finding, 0)
	for _, result := range results {
		findings = append(findings, result.Findings...)
	}
	return findings
}

func validateReleaseEvidence(release ReleaseEvidence) error {
	if !validGitTree(release.ReleaseTree) {
		return errors.New("release evidence requires an immutable release tree")
	}
	for _, identity := range []string{
		release.ConfigurationHash,
		release.GeneratedArtifactHash,
		release.ProvenanceHash,
		release.PublicationBoundaryHash,
		release.EvidenceFreshnessHash,
	} {
		if !validSHA256(identity) {
			return errors.New("release evidence hashes must be lowercase SHA-256 identities")
		}
	}
	if release.PublicationState != PublicationStateSealed {
		return errors.New("release publication boundary must be sealed")
	}
	if release.EvidenceFreshnessState != EvidenceFreshnessCurrent {
		return errors.New("release evidence must be current")
	}
	return nil
}

func cloneReleaseEvidence(release *ReleaseEvidence) *ReleaseEvidence {
	if release == nil {
		return nil
	}
	copy := *release
	return &copy
}

func validSHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && strings.ToLower(value) == value
}

func validGitTree(value string) bool {
	// Git reserves the all-zero OID as the null object; it never names a real
	// tree or commit, so it is rejected even though it is syntactically hex.
	return gitTreePattern.MatchString(value) && strings.Trim(value, "0") != ""
}
