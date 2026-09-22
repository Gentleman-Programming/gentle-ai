package kickoff

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"gopkg.in/yaml.v3"
)

// Closed vocabularies for every enum field this package owns. A value
// absent from its set is rejected with a named error; none of these maps
// degrades an unrecognised value to its nearest known member.
var (
	validFlowModes = map[FlowMode]bool{
		FlowODD: true,
		FlowSDD: true,
	}

	validExecutionStyles = map[ExecutionStyle]bool{
		ExecutionContinuous:   true,
		ExecutionCheckpointed: true,
	}

	validHandoffPolicies = map[HandoffPolicy]bool{
		HandoffNone:          true,
		HandoffPerCheckpoint: true,
	}

	validGatePolicies = map[multirole.GatePolicy]bool{
		multirole.PolicyBlocking: true,
		multirole.PolicyDeferred: true,
		multirole.PolicyOptional: true,
	}

	validDeploymentTargets = map[string]bool{
		"local":      true,
		"staging":    true,
		"production": true,
	}

	validEvidenceKinds = map[EvidenceKind]bool{
		EvidencePRMerged:    true,
		EvidenceDeployment:  true,
		EvidenceAttestation: true,
	}

	validGateDecisions = map[GateDecision]bool{
		DecisionApproved: true,
		DecisionRejected: true,
	}

	fixedGateKeys = map[GateKey]bool{
		GateSpec:        true,
		GateDesign:      true,
		GateTasks:       true,
		GateIntegration: true,
	}
)

// ParseKickoff decodes and validates a kickoff.yaml document. It never
// returns a zero-value Kickoff on malformed input: both a decode failure
// and a failed Validate() surface as a named error, never a silent zero
// value that would pass through as "no gates configured" (T-10).
func ParseKickoff(data []byte) (Kickoff, error) {
	var k Kickoff
	if err := yaml.Unmarshal(data, &k); err != nil {
		return Kickoff{}, fmt.Errorf("decodificar kickoff.yaml: %w", err)
	}
	if err := k.Validate(); err != nil {
		return Kickoff{}, err
	}
	return k, nil
}

// Validate checks that every enum field of the sealed document belongs to
// its closed vocabulary declared above. An unrecognised value is always
// rejected, never coerced to the nearest known one.
func (k Kickoff) Validate() error {
	if k.Schema != KickoffSchemaV1 {
		return fmt.Errorf("kickoff: esquema desconocido %q (se esperaba %q)", k.Schema, KickoffSchemaV1)
	}
	if !validFlowModes[k.Config.FlowMode] {
		return fmt.Errorf("kickoff: flow_mode desconocido %q", k.Config.FlowMode)
	}
	if !validExecutionStyles[k.Config.ExecutionStyle] {
		return fmt.Errorf("kickoff: execution_style desconocido %q", k.Config.ExecutionStyle)
	}
	if !validHandoffPolicies[k.Config.HandoffPolicy] {
		return fmt.Errorf("kickoff: handoff_policy desconocido %q", k.Config.HandoffPolicy)
	}
	for _, role := range k.Config.Roles {
		if !validGatePolicies[role.GatePolicy] {
			return fmt.Errorf("kickoff: gate_policy desconocido %q para el rol %q", role.GatePolicy, role.Role)
		}
	}
	if !validDeploymentTargets[k.Lifecycle.DeploymentTarget] {
		return fmt.Errorf("kickoff: deployment_target desconocido %q", k.Lifecycle.DeploymentTarget)
	}
	return nil
}

// ParseGateLedger decodes and validates a gates.yaml document, applying the
// same "never a silent zero value" discipline as ParseKickoff.
func ParseGateLedger(data []byte) (GateLedger, error) {
	var l GateLedger
	if err := yaml.Unmarshal(data, &l); err != nil {
		return GateLedger{}, fmt.Errorf("decodificar gates.yaml: %w", err)
	}
	if err := l.Validate(); err != nil {
		return GateLedger{}, err
	}
	return l, nil
}

// Validate checks the ledger schema and every record it holds.
func (l GateLedger) Validate() error {
	if l.Schema != GateLedgerSchemaV1 {
		return fmt.Errorf("gates: esquema desconocido %q (se esperaba %q)", l.Schema, GateLedgerSchemaV1)
	}
	for i, record := range l.Records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("gates: registro %d invalido: %w", i, err)
		}
	}
	return nil
}

// Validate checks one gate record: its gate key belongs to the closed
// vocabulary (the four fixed keys, or a role-apply:<role> key built by
// RoleApplyGate), its decision is a recognised verdict, and, when present,
// its evidence kind is one of the three declared classes.
func (r GateRecord) Validate() error {
	if r.Gate == "" {
		return fmt.Errorf("gate record: la clave de compuerta esta vacia")
	}
	if !fixedGateKeys[r.Gate] && !strings.HasPrefix(string(r.Gate), roleApplyGatePrefix) {
		return fmt.Errorf("gate record: clave de compuerta desconocida %q", r.Gate)
	}
	if !validGateDecisions[r.Decision] {
		return fmt.Errorf("gate record: decision desconocida %q", r.Decision)
	}
	if r.EvidenceKind != "" && !validEvidenceKinds[r.EvidenceKind] {
		return fmt.Errorf("gate record: evidence_kind desconocido %q", r.EvidenceKind)
	}
	return nil
}
