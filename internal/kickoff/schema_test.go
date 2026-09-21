package kickoff

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
)

func validKickoffYAML() string {
	return `
schema: axiom.sdd-kickoff/v1
change: inc-99-example
sealed_at: 2026-09-21T15:25:00Z
sealed_by: cli
kickoff:
  flow_mode: sdd
  execution_style: checkpointed
  execution_style_source: session_pace
  handoff_policy: per_checkpoint
  roles:
    - role: fullstack
      gate_policy: blocking
      tasks_file: tasks.md
      verify_file: verify-report.md
lifecycle:
  deployment_target: staging
  post_archive_policy: bug_only
`
}

func TestParseKickoffValidDocument(t *testing.T) {
	k, err := ParseKickoff([]byte(validKickoffYAML()))
	if err != nil {
		t.Fatalf("ParseKickoff() error inesperado = %v", err)
	}
	if k.Config.FlowMode != FlowSDD {
		t.Errorf("FlowMode = %q, se esperaba %q", k.Config.FlowMode, FlowSDD)
	}
	if len(k.Config.Roles) != 1 || k.Config.Roles[0].TasksFile != "tasks.md" {
		t.Errorf("Roles = %+v, se esperaba un rol fullstack con tasks.md", k.Config.Roles)
	}
	if k.Config.Roles[0].VerifyFile != "verify-report.md" {
		t.Errorf("VerifyFile = %q, se esperaba verify-report.md", k.Config.Roles[0].VerifyFile)
	}
}

func TestKickoffValidateEnums(t *testing.T) {
	base := func(mutate func(*Kickoff)) Kickoff {
		k := Kickoff{
			Schema:   KickoffSchemaV1,
			Change:   "inc-99-example",
			SealedBy: "cli",
			Config: FlowConfig{
				FlowMode:             FlowSDD,
				ExecutionStyle:       ExecutionCheckpointed,
				ExecutionStyleSource: "session_pace",
				HandoffPolicy:        HandoffPerCheckpoint,
				Roles: []KickoffRole{
					{Role: "fullstack", GatePolicy: multirole.PolicyBlocking, TasksFile: "tasks.md", VerifyFile: "verify-report.md"},
				},
			},
			Lifecycle: Lifecycle{DeploymentTarget: "staging", PostArchivePolicy: "bug_only"},
		}
		mutate(&k)
		return k
	}

	tests := []struct {
		name    string
		kickoff Kickoff
		wantErr bool
		errText string
	}{
		{
			name:    "documento valido con todos los enums reconocidos",
			kickoff: base(func(*Kickoff) {}),
			wantErr: false,
		},
		{
			name:    "esquema desconocido se rechaza",
			kickoff: base(func(k *Kickoff) { k.Schema = "axiom.sdd-kickoff/v99" }),
			wantErr: true,
			errText: "esquema desconocido",
		},
		{
			name:    "flow_mode desconocido se rechaza",
			kickoff: base(func(k *Kickoff) { k.Config.FlowMode = FlowMode("waterfall") }),
			wantErr: true,
			errText: "flow_mode desconocido",
		},
		{
			name:    "execution_style desconocido se rechaza",
			kickoff: base(func(k *Kickoff) { k.Config.ExecutionStyle = ExecutionStyle("sometimes") }),
			wantErr: true,
			errText: "execution_style desconocido",
		},
		{
			name:    "handoff_policy desconocida se rechaza",
			kickoff: base(func(k *Kickoff) { k.Config.HandoffPolicy = HandoffPolicy("always") }),
			wantErr: true,
			errText: "handoff_policy desconocido",
		},
		{
			name: "gate_policy desconocida en un rol se rechaza",
			kickoff: base(func(k *Kickoff) {
				k.Config.Roles[0].GatePolicy = multirole.GatePolicy("urgent")
			}),
			wantErr: true,
			errText: "gate_policy desconocido",
		},
		{
			name:    "deployment_target desconocido se rechaza",
			kickoff: base(func(k *Kickoff) { k.Lifecycle.DeploymentTarget = "moon" }),
			wantErr: true,
			errText: "deployment_target desconocido",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.kickoff.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errText) {
				t.Errorf("Validate() error = %q, se esperaba que contuviera %q", err.Error(), tt.errText)
			}
		})
	}
}

func TestParseKickoffMalformedYAML(t *testing.T) {
	_, err := ParseKickoff([]byte("kickoff: [este, no, es, un, mapeo\n  roto: si"))
	if err == nil {
		t.Fatal("ParseKickoff() con YAML malformado debia devolver error, no un Kickoff vacio silencioso")
	}
}

func TestGateLedgerValidate(t *testing.T) {
	base := func(mutate func(*GateRecord)) GateRecord {
		r := GateRecord{
			Gate:     GateSpec,
			Decision: DecisionApproved,
			Reason:   "cubre el caso borde",
			Actor:    "maintainer",
		}
		mutate(&r)
		return r
	}

	tests := []struct {
		name    string
		record  GateRecord
		wantErr bool
		errText string
	}{
		{
			name:    "registro valido para una compuerta fija",
			record:  base(func(*GateRecord) {}),
			wantErr: false,
		},
		{
			name:    "clave role-apply reconocida por su prefijo",
			record:  base(func(r *GateRecord) { r.Gate = RoleApplyGate("fullstack") }),
			wantErr: false,
		},
		{
			name:    "clave de compuerta desconocida se rechaza",
			record:  base(func(r *GateRecord) { r.Gate = GateKey("bogus") }),
			wantErr: true,
			errText: "clave de compuerta desconocida",
		},
		{
			name:    "decision desconocida se rechaza",
			record:  base(func(r *GateRecord) { r.Decision = GateDecision("maybe") }),
			wantErr: true,
			errText: "decision desconocida",
		},
		{
			name:    "evidence_kind reconocido se acepta",
			record:  base(func(r *GateRecord) { r.EvidenceKind = EvidencePRMerged }),
			wantErr: false,
		},
		{
			name:    "evidence_kind desconocido se rechaza",
			record:  base(func(r *GateRecord) { r.EvidenceKind = EvidenceKind("guess") }),
			wantErr: true,
			errText: "evidence_kind desconocido",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.record.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errText) {
				t.Errorf("Validate() error = %q, se esperaba que contuviera %q", err.Error(), tt.errText)
			}
		})
	}
}

func TestParseGateLedgerUnknownSchema(t *testing.T) {
	_, err := ParseGateLedger([]byte("schema: axiom.sdd-gate-ledger/v99\nchange: inc-99\nrecords: []\n"))
	if err == nil || !strings.Contains(err.Error(), "esquema desconocido") {
		t.Fatalf("ParseGateLedger() error = %v, se esperaba rechazo por esquema desconocido", err)
	}
}
