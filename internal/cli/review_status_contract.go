package cli

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

const ReviewIntegrationStatusSchema = "gentle-ai.review-integration.status/v1"
const ReviewIntegrationStatusSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/status.schema.json"
const ReviewIntegrationProjectionSchema = "gentle-ai.review-integration.projection/v1"
const ReviewIntegrationProjectionSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/projection.schema.json"

type ReviewReceiptStatus string

const (
	ReviewReceiptExpectedMissing    ReviewReceiptStatus = "expected_missing"
	ReviewReceiptPresent            ReviewReceiptStatus = "present"
	ReviewReceiptPublicationPending ReviewReceiptStatus = "publication_pending"
	ReviewReceiptNotApplicable      ReviewReceiptStatus = "not_applicable"
)

type ReviewTargetStatusResult struct {
	Schema        string                                `json:"schema"`
	Contract      string                                `json:"contract"`
	Operation     string                                `json:"operation"`
	Applicability reviewtransaction.TargetApplicability `json:"applicability"`
	Authority     *ReviewTargetStatusAuthority          `json:"authority,omitempty"`
	Receipt       ReviewTargetStatusReceipt             `json:"receipt"`
	Action        reviewtransaction.TargetStatusAction  `json:"action"`
	// ActionDisposition names the `review recover --disposition` value the
	// recovery rules accept. It is present exactly when Action is recover.
	ActionDisposition reviewtransaction.RecoveryDisposition `json:"action_disposition,omitempty"`
	Replayability     reviewtransaction.Replayability       `json:"replayability"`
	Frozen            *ReviewTargetStatusFrozen             `json:"frozen,omitempty"`
	TargetIdentity    string                                `json:"target_identity"`
	Projection        ReviewTargetStatusProjection          `json:"projection"`
	Candidates        []string                              `json:"candidates"`
	Reconciliation    *ReviewFinalizeReconciliation         `json:"reconciliation,omitempty"`
}

type ReviewFinalizeReconciliation struct {
	Required bool `json:"required"`
}

type ReviewTargetStatusAuthority struct {
	Version    reviewtransaction.AuthorityVersion `json:"version"`
	LineageID  string                             `json:"lineage_id"`
	State      reviewtransaction.State            `json:"state"`
	Generation int                                `json:"generation"`
	Revision   string                             `json:"revision"`
}

type ReviewTargetStatusReceipt struct {
	Status   ReviewReceiptStatus `json:"status"`
	Identity string              `json:"identity,omitempty"`
}

type ReviewTargetStatusFrozen struct {
	Tier                 reviewtransaction.RiskLevel `json:"tier"`
	OriginalChangedLines int                         `json:"original_changed_lines"`
	CorrectionBudget     int                         `json:"correction_budget"`
}

type ReviewTargetStatusProjection struct {
	Schema                  string                       `json:"schema"`
	Kind                    reviewtransaction.TargetKind `json:"kind"`
	Projection              reviewtransaction.Projection `json:"projection"`
	BaseTree                string                       `json:"base_tree"`
	InitialReviewTree       string                       `json:"initial_review_tree"`
	CurrentCandidateTree    string                       `json:"current_candidate_tree"`
	PathsDigest             string                       `json:"paths_digest"`
	Paths                   []string                     `json:"paths"`
	IntendedUntracked       []string                     `json:"intended_untracked"`
	IntendedUntrackedProof  string                       `json:"intended_untracked_proof"`
	InitialSnapshotIdentity string                       `json:"initial_snapshot_identity"`
	CurrentSnapshotIdentity string                       `json:"current_snapshot_identity"`
}

func newReviewTargetStatusResult(native reviewtransaction.TargetStatusResult) ReviewTargetStatusResult {
	result := ReviewTargetStatusResult{
		Schema: ReviewIntegrationStatusSchema, Contract: ReviewIntegrationContractV1, Operation: "review.status",
		Applicability: native.Applicability, Action: native.Action, ActionDisposition: native.ActionDisposition,
		Replayability:  native.Replayability,
		TargetIdentity: native.TargetIdentity, Candidates: append([]string{}, native.CandidateLineageIDs...),
		Projection: ReviewTargetStatusProjection{
			Schema: ReviewIntegrationProjectionSchema, Kind: native.Projection.Kind, Projection: facadeProjection(native.Projection.Projection),
			BaseTree: native.Projection.BaseTree, InitialReviewTree: native.Projection.InitialReviewTree,
			CurrentCandidateTree: native.Projection.CurrentCandidateTree, PathsDigest: native.Projection.PathsDigest,
			Paths: append([]string{}, native.Projection.Paths...), IntendedUntracked: append([]string{}, native.Projection.IntendedUntracked...),
			IntendedUntrackedProof:  native.Projection.IntendedUntrackedProof,
			InitialSnapshotIdentity: native.Projection.InitialSnapshotIdentity, CurrentSnapshotIdentity: native.Projection.CurrentSnapshotIdentity,
		},
		Receipt: ReviewTargetStatusReceipt{Status: ReviewReceiptNotApplicable},
	}
	if native.Applicability != reviewtransaction.TargetApplicabilityCurrent {
		return result
	}
	if native.Action == reviewtransaction.TargetStatusActionReconcileFinalize {
		result.Reconciliation = &ReviewFinalizeReconciliation{Required: true}
	}
	result.Authority = &ReviewTargetStatusAuthority{
		Version: native.AuthorityVersion, LineageID: native.LineageID, State: native.State,
		Generation: native.Generation, Revision: native.Revision,
	}
	if native.AuthorityVersion == reviewtransaction.AuthorityVersionCompact {
		result.Frozen = &ReviewTargetStatusFrozen{
			Tier: native.Tier, OriginalChangedLines: native.OriginalChangedLines, CorrectionBudget: native.CorrectionBudget,
		}
	}
	if native.ReceiptIdentity != "" {
		result.Receipt = ReviewTargetStatusReceipt{Status: ReviewReceiptPresent, Identity: native.ReceiptIdentity}
	} else if native.AuthorityVersion == reviewtransaction.AuthorityVersionCompact &&
		(native.State == reviewtransaction.StateApproved || native.State == reviewtransaction.StateEscalated) {
		result.Receipt = ReviewTargetStatusReceipt{Status: ReviewReceiptPublicationPending}
	} else {
		result.Receipt = ReviewTargetStatusReceipt{Status: ReviewReceiptExpectedMissing}
	}
	return result
}

func (result ReviewTargetStatusResult) Validate() error {
	if result.Schema != ReviewIntegrationStatusSchema || result.Contract != ReviewIntegrationContractV1 || result.Operation != "review.status" {
		return errors.New("invalid negotiated review status identity")
	}
	if !validReviewCapabilitySHA256(result.TargetIdentity) || result.Candidates == nil {
		return errors.New("invalid negotiated review target identity")
	}
	if err := result.Projection.Validate(); err != nil {
		return err
	}
	switch result.Applicability {
	case reviewtransaction.TargetApplicabilityCurrent:
		if result.Authority == nil || result.Authority.Generation < 1 ||
			!validReviewCapabilitySHA256(result.Authority.Revision) || strings.TrimSpace(result.Authority.LineageID) == "" || len(result.Candidates) != 0 {
			return errors.New("current-target status authority is incomplete")
		}
		switch result.Authority.Version {
		case reviewtransaction.AuthorityVersionCompact:
			if result.Frozen == nil {
				return errors.New("compact current-target status requires frozen inputs")
			}
			if result.Frozen.Tier != reviewtransaction.RiskLow && result.Frozen.Tier != reviewtransaction.RiskMedium && result.Frozen.Tier != reviewtransaction.RiskHigh {
				return errors.New("current-target frozen tier is invalid")
			}
			budget, err := reviewtransaction.CorrectionBudget(result.Frozen.OriginalChangedLines)
			if err != nil || budget != result.Frozen.CorrectionBudget {
				return errors.New("current-target frozen budget is invalid")
			}
		case reviewtransaction.AuthorityVersionLegacy:
			if result.Frozen != nil {
				return errors.New("legacy current-target status cannot contain compact frozen inputs")
			}
			if result.Receipt.Status == ReviewReceiptPublicationPending {
				return errors.New("legacy current-target status cannot use compact publication semantics")
			}
			if result.Authority.State == reviewtransaction.StateApproved && result.Receipt.Status != ReviewReceiptPresent {
				return errors.New("approved legacy current-target status requires a present receipt")
			}
		default:
			return errors.New("current-target authority version is unsupported")
		}
		if result.Receipt.Status == ReviewReceiptPresent && !validReviewCapabilitySHA256(result.Receipt.Identity) ||
			result.Receipt.Status != ReviewReceiptPresent && result.Receipt.Identity != "" {
			return errors.New("current-target receipt identity is invalid")
		}
		if result.Receipt.Status != ReviewReceiptPresent && result.Receipt.Status != ReviewReceiptExpectedMissing && result.Receipt.Status != ReviewReceiptPublicationPending {
			return errors.New("current-target receipt status is invalid")
		}
	case reviewtransaction.TargetApplicabilityUnrelated:
		if result.Authority != nil || result.Frozen != nil || result.Receipt.Status != ReviewReceiptNotApplicable || result.Action != reviewtransaction.TargetStatusActionStart || len(result.Candidates) != 0 {
			return errors.New("unrelated target status is inconsistent")
		}
	case reviewtransaction.TargetApplicabilityAmbiguous:
		if result.Authority != nil || result.Frozen != nil || result.Receipt.Status != ReviewReceiptNotApplicable || result.Action != reviewtransaction.TargetStatusActionSelectLineage || len(result.Candidates) < 2 {
			return errors.New("ambiguous target status is inconsistent")
		}
	case reviewtransaction.TargetApplicabilityCorrupted:
		if result.Authority != nil || result.Frozen != nil || result.Receipt.Status != ReviewReceiptNotApplicable || result.Action != reviewtransaction.TargetStatusActionRepairAuthority {
			return errors.New("corrupted target status is inconsistent")
		}
	default:
		return errors.New("unsupported target applicability")
	}
	if strings.TrimSpace(string(result.Action)) == "" {
		return errors.New("negotiated review status requires exactly one action")
	}
	if result.Action == reviewtransaction.TargetStatusActionReconcileFinalize {
		if result.Applicability != reviewtransaction.TargetApplicabilityCurrent || result.Reconciliation == nil || !result.Reconciliation.Required || result.Replayability != reviewtransaction.ReplayabilityStatusRequired {
			return errors.New("pending finalize status requires current-target reconciliation")
		}
	} else if result.Reconciliation != nil {
		return errors.New("only pending finalize status may contain reconciliation")
	}
	switch result.Replayability {
	case reviewtransaction.ReplayabilityNotReplayable, reviewtransaction.ReplayabilityExactReplaySafe,
		reviewtransaction.ReplayabilityStatusRequired, reviewtransaction.ReplayabilityManualActionRequired:
	default:
		return errors.New("unsupported review status replayability")
	}
	switch result.ActionDisposition {
	case "":
		if result.Action == reviewtransaction.TargetStatusActionRecover {
			return errors.New("recover status requires the recovery disposition recovery accepts")
		}
	case reviewtransaction.RecoveryScopeChanged, reviewtransaction.RecoveryInvalidated, reviewtransaction.RecoveryEscalated:
		if result.Action != reviewtransaction.TargetStatusActionRecover {
			return errors.New("only recover status may carry a recovery disposition")
		}
	default:
		return errors.New("unsupported review status recovery disposition")
	}
	return nil
}

func (projection ReviewTargetStatusProjection) Validate() error {
	if projection.Schema != ReviewIntegrationProjectionSchema || projection.Paths == nil || projection.IntendedUntracked == nil {
		return errors.New("restart projection is incomplete")
	}
	for _, identity := range []string{projection.PathsDigest, projection.IntendedUntrackedProof, projection.InitialSnapshotIdentity, projection.CurrentSnapshotIdentity} {
		if !validReviewCapabilitySHA256(identity) {
			return errors.New("restart projection contains an invalid content identity")
		}
	}
	for _, tree := range []string{projection.BaseTree, projection.InitialReviewTree, projection.CurrentCandidateTree} {
		if !validReviewGitTree(tree) {
			return errors.New("restart projection contains an invalid Git tree")
		}
	}
	for _, paths := range [][]string{projection.Paths, projection.IntendedUntracked} {
		for _, value := range paths {
			if value == "" || strings.Contains(value, `\`) || strings.HasPrefix(value, "/") || len(value) >= 2 && value[1] == ':' || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
				return fmt.Errorf("restart projection path %q is not repository-relative", value)
			}
		}
	}
	if projection.Projection != reviewtransaction.ProjectionWorkspace && projection.Projection != reviewtransaction.ProjectionStaged {
		return errors.New("restart projection kind is invalid")
	}
	if !reflect.DeepEqual(sortedReviewStatusStrings(projection.Paths), projection.Paths) || !reflect.DeepEqual(sortedReviewStatusStrings(projection.IntendedUntracked), projection.IntendedUntracked) {
		return errors.New("restart projection paths are not canonical")
	}
	return nil
}

func sortedReviewStatusStrings(values []string) []string {
	copy := append([]string{}, values...)
	for index := 1; index < len(copy); index++ {
		for cursor := index; cursor > 0 && copy[cursor] < copy[cursor-1]; cursor-- {
			copy[cursor], copy[cursor-1] = copy[cursor-1], copy[cursor]
		}
	}
	return copy
}

func validReviewGitTree(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' && char < 'a' || char > 'f' {
			return false
		}
	}
	return true
}
