package sddstatus

import (
	"encoding/json"
	"fmt"
)

const StatusContractV2 = "gentle-ai.sdd-status/v2"

// StatusV2Projection is the complete public SDD status document. It projects
// only SDD planning, task, verification, action, and relationship truth; native
// runtime bookkeeping remains internal to Resolve.
type StatusV2Projection struct {
	SchemaName        string                       `json:"schemaName"`
	SchemaVersion     int                          `json:"schemaVersion"`
	ChangeName        *string                      `json:"changeName"`
	ArtifactStore     ArtifactStore                `json:"artifactStore"`
	PlanningHome      planningHomeV2               `json:"planningHome"`
	ChangeRoot        *string                      `json:"changeRoot"`
	ArtifactPaths     artifactPathsV2              `json:"artifactPaths"`
	ContextFiles      artifactPathsV2              `json:"contextFiles"`
	Artifacts         map[string]ArtifactState     `json:"artifacts"`
	TaskProgress      taskProgressV2               `json:"taskProgress"`
	Dependencies      dependenciesV2               `json:"dependencies"`
	ApplyState        ApplyState                   `json:"applyState"`
	ActionContext     actionContextV2              `json:"actionContext"`
	Relationships     relationshipsV2              `json:"relationships"`
	RemediationState  remediationStateV2           `json:"remediationState"`
	ReviewOffer       *ReviewOfferBlock            `json:"reviewOffer,omitempty"`
	Consent           *SDDIntegrationConsentResult `json:"consent,omitempty"`
	PhaseInstructions *phaseInstructionsV2         `json:"phaseInstructions,omitempty"`
	NextRecommended   string                       `json:"nextRecommended"`
	BlockedReasons    []string                     `json:"blockedReasons"`
}

type planningHomeV2 struct {
	Mode ActionMode `json:"mode"`
	Path string     `json:"path"`
}

type artifactPathsV2 struct {
	Proposal      []string `json:"proposal"`
	Specs         []string `json:"specs"`
	Design        []string `json:"design"`
	Tasks         []string `json:"tasks"`
	ApplyProgress []string `json:"applyProgress"`
	VerifyReport  []string `json:"verifyReport"`
}

type taskProgressV2 struct {
	Total       int  `json:"total"`
	Completed   int  `json:"completed"`
	Pending     int  `json:"pending"`
	AllComplete bool `json:"allComplete"`
}

type dependenciesV2 struct {
	Proposal DependencyState `json:"proposal"`
	Specs    DependencyState `json:"specs"`
	Design   DependencyState `json:"design"`
	Tasks    DependencyState `json:"tasks"`
	Apply    DependencyState `json:"apply"`
	Verify   DependencyState `json:"verify"`
	Archive  DependencyState `json:"archive"`
}

type actionContextV2 struct {
	Mode             ActionMode `json:"mode"`
	WorkspaceRoot    string     `json:"workspaceRoot"`
	AllowedEditRoots []string   `json:"allowedEditRoots"`
}

type relationshipsV2 struct {
	DependsOn               []string `json:"dependsOn"`
	Supersedes              []string `json:"supersedes"`
	Amends                  []string `json:"amends"`
	ConflictsWith           []string `json:"conflictsWith"`
	SameDomainActiveChanges []string `json:"sameDomainActiveChanges"`
}

type remediationStateV2 struct {
	Required               bool   `json:"required"`
	Complete               bool   `json:"complete"`
	FailedEvidenceRevision string `json:"failedEvidenceRevision"`
	Reason                 string `json:"reason"`
}

type phaseInstructionsV2 struct {
	Apply     []string `json:"apply"`
	Verify    []string `json:"verify"`
	Remediate []string `json:"remediate"`
	Archive   []string `json:"archive"`
}

// ProjectStatusV2 rejects unsupported internal values rather than exposing
// internal runtime state or silently broadening the public document.
func ProjectStatusV2(status Status) (StatusV2Projection, error) {
	if status.SchemaName != SchemaName || status.SchemaVersion != SchemaVersion {
		return StatusV2Projection{}, fmt.Errorf("unsupported SDD status identity %q@%d", status.SchemaName, status.SchemaVersion)
	}
	if !statusV2ArtifactStore(status.ArtifactStore) {
		return StatusV2Projection{}, fmt.Errorf("unsupported SDD v2 artifact store %q", status.ArtifactStore) // refusal:by-design operator-knowledge: ProjectStatusV2 receives an internal aggregate, so the producer must use a supported store.
	}
	if !statusV2ApplyState(status.ApplyState) {
		return StatusV2Projection{}, fmt.Errorf("unsupported SDD v2 apply state %q", status.ApplyState) // refusal:by-design operator-knowledge: ProjectStatusV2 receives an internal aggregate, so the producer must use a supported apply state.
	}
	if !statusV2NextRecommended(status.NextRecommended) {
		return StatusV2Projection{}, fmt.Errorf("unsupported SDD v2 next action %q", status.NextRecommended) // refusal:by-design operator-knowledge: ProjectStatusV2 receives an internal aggregate, so the producer must use a supported route token.
	}

	artifacts, err := projectArtifactsV2(status.ArtifactStore, status.Artifacts)
	if err != nil {
		return StatusV2Projection{}, err
	}

	projected := StatusV2Projection{
		SchemaName:    status.SchemaName,
		SchemaVersion: status.SchemaVersion,
		ChangeName:    status.ChangeName,
		ArtifactStore: status.ArtifactStore,
		PlanningHome:  projectPlanningHomeV2(status.PlanningHome),
		ChangeRoot:    status.ChangeRoot,
		ArtifactPaths: projectArtifactPathsV2(status.ArtifactPaths),
		ContextFiles:  projectArtifactPathsV2(status.ContextFiles),
		Artifacts:     artifacts,
		TaskProgress:  projectTaskProgressV2(status.TaskProgress),
		Dependencies:  projectDependenciesV2(status.Dependencies),
		ApplyState:    status.ApplyState,
		ActionContext: projectActionContextV2(status.ActionContext),
		Relationships: projectRelationshipsV2(status.Relationships),
		RemediationState: remediationStateV2{
			Required:               status.RemediationState.Required,
			Complete:               status.RemediationState.Complete,
			FailedEvidenceRevision: status.RemediationState.FailedEvidenceRevision,
			Reason:                 status.RemediationState.Reason,
		},
		ReviewOffer:     status.ReviewOffer,
		Consent:         status.Consent,
		NextRecommended: status.NextRecommended,
		BlockedReasons:  status.BlockedReasons,
	}
	if status.PhaseInstructions != nil {
		projected.PhaseInstructions = &phaseInstructionsV2{
			Apply: status.PhaseInstructions.Apply, Verify: status.PhaseInstructions.Verify,
			Remediate: status.PhaseInstructions.Remediate, Archive: status.PhaseInstructions.Archive,
		}
	}
	return projected, nil
}

func marshalStatusV2Indent(status Status) ([]byte, error) {
	projected, err := ProjectStatusV2(status)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(projected, "", "  ")
}

func projectArtifactsV2(store ArtifactStore, source map[string]ArtifactState) (map[string]ArtifactState, error) {
	keys := artifactStateKeys(store)
	projected := make(map[string]ArtifactState, len(keys))
	for _, key := range keys {
		state, ok := source[key]
		if !ok || !statusV2ArtifactState(state) {
			return nil, fmt.Errorf("unsupported SDD v2 artifact %q state %q", key, state) // refusal:by-design operator-knowledge: ProjectStatusV2 receives an internal aggregate, so the producer must use a supported artifact state.
		}
		projected[key] = state
	}
	return projected, nil
}

func projectPlanningHomeV2(value PlanningHome) planningHomeV2 {
	return planningHomeV2{Mode: value.Mode, Path: value.Path}
}

func projectArtifactPathsV2(value ArtifactPaths) artifactPathsV2 {
	return artifactPathsV2{
		Proposal: value.Proposal, Specs: value.Specs, Design: value.Design, Tasks: value.Tasks,
		ApplyProgress: value.ApplyProgress, VerifyReport: value.VerifyReport,
	}
}

func projectTaskProgressV2(value TaskProgress) taskProgressV2 {
	return taskProgressV2{Total: value.Total, Completed: value.Completed, Pending: value.Pending, AllComplete: value.AllComplete}
}

func projectDependenciesV2(value Dependencies) dependenciesV2 {
	return dependenciesV2{
		Proposal: value.Proposal, Specs: value.Specs, Design: value.Design, Tasks: value.Tasks,
		Apply: value.Apply, Verify: value.Verify, Archive: value.Archive,
	}
}

func projectActionContextV2(value ActionContext) actionContextV2 {
	return actionContextV2{Mode: value.Mode, WorkspaceRoot: value.WorkspaceRoot, AllowedEditRoots: value.AllowedEditRoots}
}

func projectRelationshipsV2(value Relationships) relationshipsV2 {
	return relationshipsV2{
		DependsOn: value.DependsOn, Supersedes: value.Supersedes, Amends: value.Amends,
		ConflictsWith: value.ConflictsWith, SameDomainActiveChanges: value.SameDomainActiveChanges,
	}
}

func statusV2ArtifactStore(value ArtifactStore) bool {
	return value == ArtifactStoreOpenSpec || value == ArtifactStoreEngram || value == ArtifactStoreNone
}

func statusV2ArtifactState(value ArtifactState) bool {
	return value == ArtifactMissing || value == ArtifactPartial || value == ArtifactDone
}

func statusV2ApplyState(value ApplyState) bool {
	return value == ApplyBlocked || value == ApplyReady || value == ApplyAllDone
}

func statusV2NextRecommended(value string) bool {
	switch value {
	case "apply", "verify", "remediate", "archive", "resolve-blockers", "sdd-new", "select-change", "propose", "spec", "design", "tasks":
		return true
	default:
		return false
	}
}
