package sddstatus

import (
	"encoding/json"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const StatusContractV1 = "gentle-ai.sdd-status/v1"

// StatusV1Projection is the frozen wire shape consumed by legacy Gentle Pi
// clients. Status remains the internal aggregate and may grow independently.
type StatusV1Projection struct {
	SchemaName        string                         `json:"schemaName"`
	SchemaVersion     int                            `json:"schemaVersion"`
	ChangeName        *string                        `json:"changeName"`
	ArtifactStore     ArtifactStore                  `json:"artifactStore"`
	PlanningHome      planningHomeV1                 `json:"planningHome"`
	ChangeRoot        *string                        `json:"changeRoot"`
	ArtifactPaths     artifactPathsV1                `json:"artifactPaths"`
	ContextFiles      artifactPathsV1                `json:"contextFiles"`
	Artifacts         map[string]ArtifactState       `json:"artifacts"`
	TaskProgress      taskProgressV1                 `json:"taskProgress"`
	Dependencies      dependenciesV1                 `json:"dependencies"`
	ApplyState        ApplyState                     `json:"applyState"`
	ActionContext     actionContextV1                `json:"actionContext"`
	Relationships     relationshipsV1                `json:"relationships"`
	RemediationState  remediationStateV1             `json:"remediationState"`
	ReviewGate        *reviewGateStateV1             `json:"reviewGate,omitempty"`
	ReviewTransaction *reviewtransaction.Transaction `json:"reviewTransaction,omitempty"`
	PhaseInstructions *phaseInstructionsV1           `json:"phaseInstructions,omitempty"`
	NextRecommended   string                         `json:"nextRecommended"`
	BlockedReasons    []string                       `json:"blockedReasons"`
}

type planningHomeV1 struct {
	Mode ActionMode `json:"mode"`
	Path string     `json:"path"`
}

type artifactPathsV1 struct {
	Proposal      []string `json:"proposal"`
	Specs         []string `json:"specs"`
	Design        []string `json:"design"`
	Tasks         []string `json:"tasks"`
	ApplyProgress []string `json:"applyProgress"`
	VerifyReport  []string `json:"verifyReport"`
	ReviewPolicy  []string `json:"reviewPolicy"`
	ReviewLedger  []string `json:"reviewLedger"`
	ReviewReceipt []string `json:"reviewReceipt"`
	ReviewBundle  []string `json:"reviewBundle"`
	ReviewContext []string `json:"reviewContext"`
	ReviewState   []string `json:"reviewState"`
}

type taskProgressV1 struct {
	Total       int  `json:"total"`
	Completed   int  `json:"completed"`
	Pending     int  `json:"pending"`
	AllComplete bool `json:"allComplete"`
}

type dependenciesV1 struct {
	Proposal DependencyState `json:"proposal"`
	Specs    DependencyState `json:"specs"`
	Design   DependencyState `json:"design"`
	Tasks    DependencyState `json:"tasks"`
	Apply    DependencyState `json:"apply"`
	Verify   DependencyState `json:"verify"`
	Archive  DependencyState `json:"archive"`
}

type actionContextV1 struct {
	Mode             ActionMode `json:"mode"`
	WorkspaceRoot    string     `json:"workspaceRoot"`
	AllowedEditRoots []string   `json:"allowedEditRoots"`
}

type relationshipsV1 struct {
	DependsOn               []string `json:"dependsOn"`
	Supersedes              []string `json:"supersedes"`
	Amends                  []string `json:"amends"`
	ConflictsWith           []string `json:"conflictsWith"`
	SameDomainActiveChanges []string `json:"sameDomainActiveChanges"`
}

type remediationStateV1 struct {
	Required               bool   `json:"required"`
	Complete               bool   `json:"complete"`
	FailedEvidenceRevision string `json:"failedEvidenceRevision"`
	LineageID              string `json:"lineageId"`
	Generation             int    `json:"generation"`
	FixBatch               int    `json:"fixBatch"`
	Reason                 string `json:"reason"`
}

// reviewGateStateV1 grows one omitempty field over the frozen v1 shape.
// Delivery is empty on every path that could already produce a review gate, so
// the bytes a legacy Gentle Pi client receives for those paths are unchanged;
// it appears only for the disabled/unmanaged disposition, which no legacy path
// could reach.
type reviewGateStateV1 struct {
	Result   reviewtransaction.GateResult  `json:"result"`
	Reason   string                        `json:"reason"`
	Delivery reviewtransaction.RDDDelivery `json:"delivery,omitempty"`
}

type phaseInstructionsV1 struct {
	Apply     []string `json:"apply"`
	Verify    []string `json:"verify"`
	Remediate []string `json:"remediate"`
	Archive   []string `json:"archive"`
}

// ProjectStatusV1 rejects values that the exact legacy decoder cannot
// understand instead of leaking internal-only fields or action tokens.
func ProjectStatusV1(status Status) (StatusV1Projection, error) {
	if status.SchemaName != SchemaName || status.SchemaVersion != SchemaVersion {
		return StatusV1Projection{}, fmt.Errorf("unsupported SDD status identity %q@%d", status.SchemaName, status.SchemaVersion)
	}
	if !legacyArtifactStore(status.ArtifactStore) {
		return StatusV1Projection{}, fmt.Errorf("unsupported SDD v1 artifact store %q", status.ArtifactStore)
	}
	if !legacyApplyState(status.ApplyState) {
		return StatusV1Projection{}, fmt.Errorf("unsupported SDD v1 apply state %q", status.ApplyState)
	}
	if !legacyNextRecommended(status.NextRecommended) {
		return StatusV1Projection{}, fmt.Errorf("unsupported SDD v1 next action %q", status.NextRecommended)
	}

	artifacts, err := projectArtifactsV1(status.ArtifactStore, status.Artifacts)
	if err != nil {
		return StatusV1Projection{}, err
	}

	projected := StatusV1Projection{
		SchemaName:    status.SchemaName,
		SchemaVersion: status.SchemaVersion,
		ChangeName:    status.ChangeName,
		ArtifactStore: status.ArtifactStore,
		PlanningHome:  projectPlanningHomeV1(status.PlanningHome),
		ChangeRoot:    status.ChangeRoot,
		ArtifactPaths: projectArtifactPathsV1(status.ArtifactPaths),
		ContextFiles:  projectArtifactPathsV1(status.ContextFiles),
		Artifacts:     artifacts,
		TaskProgress:  projectTaskProgressV1(status.TaskProgress),
		Dependencies:  projectDependenciesV1(status.Dependencies),
		ApplyState:    status.ApplyState,
		ActionContext: projectActionContextV1(status.ActionContext),
		Relationships: projectRelationshipsV1(status.Relationships),
		RemediationState: remediationStateV1{
			Required:               status.RemediationState.Required,
			Complete:               status.RemediationState.Complete,
			FailedEvidenceRevision: status.RemediationState.FailedEvidenceRevision,
			LineageID:              status.RemediationState.LineageID,
			Generation:             status.RemediationState.Generation,
			FixBatch:               status.RemediationState.FixBatch,
			Reason:                 status.RemediationState.Reason,
		},
		ReviewTransaction: status.ReviewTransaction,
		NextRecommended:   status.NextRecommended,
		BlockedReasons:    status.BlockedReasons,
	}
	if status.ReviewGate != nil {
		projected.ReviewGate = &reviewGateStateV1{
			Result:   status.ReviewGate.Result,
			Reason:   status.ReviewGate.Reason,
			Delivery: status.ReviewGate.Delivery,
		}
	}
	if status.PhaseInstructions != nil {
		projected.PhaseInstructions = &phaseInstructionsV1{
			Apply:     status.PhaseInstructions.Apply,
			Verify:    status.PhaseInstructions.Verify,
			Remediate: status.PhaseInstructions.Remediate,
			Archive:   status.PhaseInstructions.Archive,
		}
	}
	return projected, nil
}

func marshalStatusV1Indent(status Status) ([]byte, error) {
	projected, err := ProjectStatusV1(status)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(projected, "", "  ")
}

func projectArtifactsV1(store ArtifactStore, source map[string]ArtifactState) (map[string]ArtifactState, error) {
	keys := []string{
		"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport",
		"reviewLedger", "reviewReceipt", "reviewBundle", "reviewContext", "reviewState",
	}
	if store == ArtifactStoreEngram {
		keys = append(keys, "reviewPolicy")
	}
	projected := make(map[string]ArtifactState, len(keys))
	for _, key := range keys {
		state, ok := source[key]
		if !ok || !legacyArtifactState(state) {
			return nil, fmt.Errorf("unsupported SDD v1 artifact %q state %q", key, state)
		}
		projected[key] = state
	}
	return projected, nil
}

func projectPlanningHomeV1(value PlanningHome) planningHomeV1 {
	return planningHomeV1{Mode: value.Mode, Path: value.Path}
}

func projectArtifactPathsV1(value ArtifactPaths) artifactPathsV1 {
	return artifactPathsV1{
		Proposal: value.Proposal, Specs: value.Specs, Design: value.Design, Tasks: value.Tasks,
		ApplyProgress: value.ApplyProgress, VerifyReport: value.VerifyReport, ReviewPolicy: value.ReviewPolicy,
		ReviewLedger: value.ReviewLedger, ReviewReceipt: value.ReviewReceipt, ReviewBundle: value.ReviewBundle,
		ReviewContext: value.ReviewContext, ReviewState: value.ReviewState,
	}
}

func projectTaskProgressV1(value TaskProgress) taskProgressV1 {
	return taskProgressV1{
		Total: value.Total, Completed: value.Completed, Pending: value.Pending, AllComplete: value.AllComplete,
	}
}

func projectDependenciesV1(value Dependencies) dependenciesV1 {
	return dependenciesV1{
		Proposal: value.Proposal, Specs: value.Specs, Design: value.Design, Tasks: value.Tasks,
		Apply: value.Apply, Verify: value.Verify, Archive: value.Archive,
	}
}

func projectActionContextV1(value ActionContext) actionContextV1 {
	return actionContextV1{
		Mode: value.Mode, WorkspaceRoot: value.WorkspaceRoot, AllowedEditRoots: value.AllowedEditRoots,
	}
}

func projectRelationshipsV1(value Relationships) relationshipsV1 {
	return relationshipsV1{
		DependsOn: value.DependsOn, Supersedes: value.Supersedes, Amends: value.Amends,
		ConflictsWith: value.ConflictsWith, SameDomainActiveChanges: value.SameDomainActiveChanges,
	}
}

func legacyArtifactStore(value ArtifactStore) bool {
	return value == ArtifactStoreOpenSpec || value == ArtifactStoreEngram || value == ArtifactStoreNone
}

func legacyArtifactState(value ArtifactState) bool {
	return value == ArtifactMissing || value == ArtifactPartial || value == ArtifactDone
}

func legacyApplyState(value ApplyState) bool {
	return value == ApplyBlocked || value == ApplyReady || value == ApplyAllDone
}

func legacyNextRecommended(value string) bool {
	switch value {
	case "apply", "verify", "remediate", "archive", "review", "resolve-review",
		"resolve-blockers", "sdd-new", "select-change", "propose", "spec", "design", "tasks":
		return true
	default:
		return false
	}
}
