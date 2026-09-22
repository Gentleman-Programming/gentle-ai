package sddstatus

import (
	"encoding/json"
	"fmt"
	"time"
)

const StatusContractV2 = "gentle-ai.sdd-status/v2"

// StatusV2Projection is the complete public SDD status document. It projects
// only SDD planning, task, verification, action, and relationship truth; native
// runtime bookkeeping remains internal to Resolve.
type StatusV2Projection struct {
	SchemaName    string                       `json:"schemaName"`
	SchemaVersion int                          `json:"schemaVersion"`
	ChangeName    *string                      `json:"changeName"`
	ArtifactStore ArtifactStore                `json:"artifactStore"`
	PlanningHome  planningHomeV2               `json:"planningHome"`
	ChangeRoot    *string                      `json:"changeRoot"`
	ArtifactPaths artifactPathsV2              `json:"artifactPaths"`
	ContextFiles  artifactPathsV2              `json:"contextFiles"`
	Artifacts     map[string]ArtifactState     `json:"artifacts"`
	TaskProgress  taskProgressV2               `json:"taskProgress"`
	Dependencies  dependenciesV2               `json:"dependencies"`
	ApplyState    ApplyState                   `json:"applyState"`
	ActionContext actionContextV2              `json:"actionContext"`
	Relationships relationshipsV2              `json:"relationships"`
	Consent       *SDDIntegrationConsentResult `json:"consent,omitempty"`
	Archived      *ArchivedProjection          `json:"archived,omitempty"`
	// Governance is INC-21's per-change block-review view, present exactly
	// when Status.Governance is (design.md S4.3, D-05): a sealed,
	// non-continuous kickoff. This is an additive v2 field; no existing
	// value or field changes (task 14.3).
	Governance *governanceV2 `json:"governance,omitempty"`
	// GateQuestion projects Status.GateQuestion unchanged: like Consent, the
	// wire type already matches the public contract 1:1, so no *V2
	// translation type is needed (design.md S5.5 defines the wire shape
	// directly).
	GateQuestion      *SDDGovernanceGateResult `json:"gateQuestion,omitempty"`
	PhaseInstructions *phaseInstructionsV2     `json:"phaseInstructions,omitempty"`
	NextRecommended   string                   `json:"nextRecommended"`
	BlockedReasons    []string                 `json:"blockedReasons"`
	Notes             []string                 `json:"notes"`
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

type phaseInstructionsV2 struct {
	Apply   []string `json:"apply"`
	Verify  []string `json:"verify"`
	Archive []string `json:"archive"`
}

// governanceV2 is INC-21's public shape for one change's sealed kickoff and
// block-review gate ledger (design.md S5.4). It intentionally does not
// reuse the internal kickoff.Kickoff/kickoff.GateState types directly: the
// wire contract stays independent of that domain package's own evolution,
// the same discipline every other *V2 type in this file already applies to
// its internal counterpart.
type governanceV2 struct {
	Kickoff kickoffV2 `json:"kickoff"`
	Gates   []gateV2  `json:"gates"`
	Roster  rosterV2  `json:"roster"`
}

type kickoffV2 struct {
	Schema         string `json:"schema"`
	FlowMode       string `json:"flowMode"`
	ExecutionStyle string `json:"executionStyle"`
	HandoffPolicy  string `json:"handoffPolicy"`
	SealedAt       string `json:"sealedAt"`
	SealedBy       string `json:"sealedBy"`
}

type gateV2 struct {
	Key      string `json:"key"`
	Status   string `json:"status"`
	Blocks   string `json:"blocks"`
	Reason   string `json:"reason,omitempty"`
	Reopened bool   `json:"reopened,omitempty"`
}

// rosterV2 projects GovernanceRoster. Conflict stays nil (omitempty) in
// this slice: D-06's multirole.ResolveRoster stratification, the only
// producer of a genuine kickoff/design conflict, is Phase 16's job. The
// field exists now so Phase 16 only has to populate it, not add it.
type rosterV2 struct {
	Source   string   `json:"source"`
	Roles    []string `json:"roles"`
	Conflict *string  `json:"conflict,omitempty"`
}

// projectGovernanceV2 translates the internal Governance view into its
// public wire shape, or reports nil when governance is nil -- exactly the
// D-05 structural absence Status.Governance and StatusV2Projection.Governance
// both document.
func projectGovernanceV2(governance *Governance) *governanceV2 {
	if governance == nil {
		return nil
	}
	gates := make([]gateV2, 0, len(governance.Gates))
	for _, gate := range governance.Gates {
		gates = append(gates, gateV2{
			Key: string(gate.Key), Status: gate.Status, Blocks: gate.Blocks,
			Reason: gate.Reason, Reopened: gate.Reopened,
		})
	}
	sealedAt := ""
	if !governance.Kickoff.SealedAt.IsZero() {
		sealedAt = governance.Kickoff.SealedAt.Format(time.RFC3339)
	}
	return &governanceV2{
		Kickoff: kickoffV2{
			Schema:         governance.Kickoff.Schema,
			FlowMode:       string(governance.Kickoff.Config.FlowMode),
			ExecutionStyle: string(governance.Kickoff.Config.ExecutionStyle),
			HandoffPolicy:  string(governance.Kickoff.Config.HandoffPolicy),
			SealedAt:       sealedAt,
			SealedBy:       governance.Kickoff.SealedBy,
		},
		Gates:  gates,
		Roster: rosterV2{Source: governance.Roster.Source, Roles: governance.Roster.Roles},
	}
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
		SchemaName:      status.SchemaName,
		SchemaVersion:   status.SchemaVersion,
		ChangeName:      status.ChangeName,
		ArtifactStore:   status.ArtifactStore,
		PlanningHome:    projectPlanningHomeV2(status.PlanningHome),
		ChangeRoot:      status.ChangeRoot,
		ArtifactPaths:   projectArtifactPathsV2(status.ArtifactPaths),
		ContextFiles:    projectArtifactPathsV2(status.ContextFiles),
		Artifacts:       artifacts,
		TaskProgress:    projectTaskProgressV2(status.TaskProgress),
		Dependencies:    projectDependenciesV2(status.Dependencies),
		ApplyState:      status.ApplyState,
		ActionContext:   projectActionContextV2(status.ActionContext),
		Relationships:   projectRelationshipsV2(status.Relationships),
		Consent:         status.Consent,
		Archived:        status.Archived,
		Governance:      projectGovernanceV2(status.Governance),
		GateQuestion:    status.GateQuestion,
		NextRecommended: status.NextRecommended,
		BlockedReasons:  status.BlockedReasons,
		Notes:           status.Notes,
	}
	if status.PhaseInstructions != nil {
		projected.PhaseInstructions = &phaseInstructionsV2{
			Apply: status.PhaseInstructions.Apply, Verify: status.PhaseInstructions.Verify,
			Archive: status.PhaseInstructions.Archive,
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
	// #3636 asked for hybrid to reach the public document: a workspace that
	// declares it was reported as openspec, so its file writes were invisible
	// to a consumer that read the status as pure Engram. This is an additive
	// v2 enum value; no existing value or field changes.
	return value == ArtifactStoreOpenSpec || value == ArtifactStoreEngram ||
		value == ArtifactStoreHybrid || value == ArtifactStoreNone
}

func statusV2ArtifactState(value ArtifactState) bool {
	return value == ArtifactMissing || value == ArtifactPartial || value == ArtifactDone
}

func statusV2ApplyState(value ApplyState) bool {
	return value == ApplyBlocked || value == ApplyReady || value == ApplyAllDone
}

func statusV2NextRecommended(value string) bool {
	// "archived" is #4002's positive terminal route: the change is closed, no
	// phase remains, and the archived block carries the location fact. This is
	// an additive v2 enum value; no existing value or field changes.
	// "await-gate" is INC-21's own additive v2 enum value (D-09): a block
	// review gate is pending a decision. Same discipline: no existing value
	// or field changes (task 14.3).
	switch value {
	case "apply", "verify", "archive", "archived", "resolve-blockers", "sdd-new", "select-change", "propose", "spec", "design", "tasks", "await-gate":
		return true
	default:
		return false
	}
}
