// Package kickoff owns the per-change SDD governance domain introduced by
// INC-21: the sealed kickoff configuration (kickoff.yaml), the append-only
// gate ledger (gates.yaml), and the pure gate state machine that projects
// them. It is a leaf package: it depends only on internal/multirole (for
// RoleAssignment/GatePolicy), internal/reviewtransaction (atomic publish and
// locking primitives), the standard library, and gopkg.in/yaml.v3.
package kickoff

import (
	"fmt"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
)

// FlowMode selects the execution lane sealed for the change: the agile ODD
// lane (no gates, no roles) or the formal SDD lane (sealed pre-flight,
// block review gates).
type FlowMode string

// ExecutionStyle determines whether an SDD change advances without pausing
// (continuous) or stops at every block review gate (checkpointed).
type ExecutionStyle string

// HandoffPolicy determines whether the change produces intermediate
// handoff.md documents in addition to the final integration handoff.
type HandoffPolicy string

// GateKey identifies one block review gate. The four fixed keys (spec,
// design, tasks, integration) are declared as constants below; the
// per-role role-apply:<role> keys are built exclusively by RoleApplyGate.
type GateKey string

// GateDecision is the human verdict recorded against a gate.
type GateDecision string

// EvidenceKind classifies the integration/deployment evidence attached to
// an approval of the integration gate.
type EvidenceKind string

const (
	// KickoffSchemaV1 is the only schema identifier ParseKickoff accepts.
	KickoffSchemaV1 = "axiom.sdd-kickoff/v1"
	// GateLedgerSchemaV1 is the only schema identifier ParseGateLedger accepts.
	GateLedgerSchemaV1 = "axiom.sdd-gate-ledger/v1"
)

const (
	FlowODD FlowMode = "odd"
	FlowSDD FlowMode = "sdd"

	ExecutionContinuous   ExecutionStyle = "continuous"
	ExecutionCheckpointed ExecutionStyle = "checkpointed"

	HandoffNone          HandoffPolicy = "none"
	HandoffPerCheckpoint HandoffPolicy = "per_checkpoint"

	GateSpec        GateKey = "spec"
	GateDesign      GateKey = "design"
	GateTasks       GateKey = "tasks"
	GateIntegration GateKey = "integration"

	DecisionApproved GateDecision = "approved"
	DecisionRejected GateDecision = "rejected"

	EvidencePRMerged    EvidenceKind = "pr_merged"
	EvidenceDeployment  EvidenceKind = "deployment"
	EvidenceAttestation EvidenceKind = "attestation"
)

// roleApplyGatePrefix is the single literal used to build AND to recognise
// a role-apply gate key. RoleApplyGate is the only constructor; schema
// validation reuses this same prefix instead of a second literal, so the
// two can never drift apart (D-08).
const roleApplyGatePrefix = "role-apply:"

// RoleApplyGate builds the gate key for one role's apply gate. It is the
// ONLY way to construct this key: a "role-apply:"+role literal scattered
// across call sites is exactly how two writers of the same identifier
// diverge.
func RoleApplyGate(role string) GateKey {
	return GateKey(roleApplyGatePrefix + strings.ToLower(role))
}

// KickoffRole is one role entry sealed under the "kickoff:" block of the
// document. It carries the role identity, its per-role gate policy
// (reusing multirole.GatePolicy so the two vocabularies never diverge),
// and the two artifact paths both the role-apply gate and the existing
// multi-role barrier need.
type KickoffRole struct {
	Role       string               `yaml:"role"`
	GatePolicy multirole.GatePolicy `yaml:"gate_policy"`
	TasksFile  string               `yaml:"tasks_file"`
	VerifyFile string               `yaml:"verify_file"`
}

// FlowConfig is the "kickoff:" block of the sealed document: the flow
// decisions made at the pre-flight questionnaire, before proposal.md
// exists.
type FlowConfig struct {
	FlowMode             FlowMode       `yaml:"flow_mode"`
	ExecutionStyle       ExecutionStyle `yaml:"execution_style"`
	ExecutionStyleSource string         `yaml:"execution_style_source"`
	HandoffPolicy        HandoffPolicy  `yaml:"handoff_policy"`
	Roles                []KickoffRole  `yaml:"roles"`
}

// Lifecycle is the "lifecycle:" block of the sealed document.
type Lifecycle struct {
	DeploymentTarget  string `yaml:"deployment_target"`
	PostArchivePolicy string `yaml:"post_archive_policy"`
}

// Kickoff is the full per-change governance document sealed at
// openspec/changes/<change>/kickoff.yaml. Once written by Seal, its
// "kickoff" and "lifecycle" blocks never change for the life of the change
// instance (D-01, D-02).
type Kickoff struct {
	Schema    string     `yaml:"schema"`
	Change    string     `yaml:"change"`
	SealedAt  time.Time  `yaml:"sealed_at"`
	SealedBy  string     `yaml:"sealed_by"`
	Config    FlowConfig `yaml:"kickoff"`
	Lifecycle Lifecycle  `yaml:"lifecycle"`
}

// GateRecord is one append-only entry in gates.yaml: a single human
// decision about one gate, the digest of the artifact judged, and, for the
// integration gate, the evidence supporting it.
type GateRecord struct {
	Gate            GateKey      `yaml:"gate"`
	Decision        GateDecision `yaml:"decision"`
	Reason          string       `yaml:"reason"`
	ArtifactDigest  string       `yaml:"artifact_digest,omitempty"`
	EvidenceKind    EvidenceKind `yaml:"evidence_kind,omitempty"`
	EvidenceRef     string       `yaml:"evidence_ref,omitempty"`
	EvidenceBaseRef string       `yaml:"evidence_base_ref,omitempty"`
	Verified        bool         `yaml:"verified,omitempty"`
	Actor           string       `yaml:"actor"`
	RecordedAt      time.Time    `yaml:"recorded_at"`
}

// GateLedger is the full append-only content of gates.yaml: every decision
// ever recorded for the change, in the order they were appended. No verb
// ever removes or edits a record (REQ-21.12): rejection history is
// evidence.
type GateLedger struct {
	Schema  string       `yaml:"schema"`
	Change  string       `yaml:"change"`
	Records []GateRecord `yaml:"records"`
}

// Gate is the static definition of one gate key: when the machine
// evaluates it, in the fixed order, and, in prose, what phase or role it
// blocks until approved. It carries no per-change state; GateState carries
// the computed view for one evaluation (D-08).
type Gate struct {
	Key    GateKey
	Blocks string
}

// GateState is the computed view of a gate after applying the digest
// reopening rule of D-08: its current decision, why, what it blocks, and
// whether a prior rejection was reopened by a changed artifact digest.
type GateState struct {
	Key      GateKey
	Status   string // "pending" | "approved" | "rejected"
	Reason   string
	Blocks   string
	Reopened bool
}

// defaultRoleArtifactFiles derives the tasks/verify file names a role uses
// under the one naming convention this package applies everywhere a
// KickoffRole is built: the single "fullstack" role keeps the plain
// tasks.md/verify-report.md pair every existing verb already looks for; any
// other role gets the tasks.<role>.md/verify-report.<role>.md pair the
// multi-role barrier (multirole.EvaluateBarrier) already tries first. Both
// InferKickoff's retro-seal (infer.go) and an explicit `axiom sdd kickoff
// seal` (args.go, SealArgs.ToKickoff) build their KickoffRole entries
// through this single function, so the two paths can never silently
// diverge on which file a role's gate is judged against.
func defaultRoleArtifactFiles(role string) (tasksFile, verifyFile string) {
	if role == "fullstack" {
		return "tasks.md", "verify-report.md"
	}
	return fmt.Sprintf("tasks.%s.md", role), fmt.Sprintf("verify-report.%s.md", role)
}
