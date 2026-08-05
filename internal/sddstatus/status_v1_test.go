package sddstatus

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectStatusV1FreezesExactLegacyShape(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
	status.RuntimeStatus = &RuntimeStatus{Schema: RuntimeStatusSchema, Change: "internal-only"}
	status.RemediationState.CorrectionBudgetRemaining = 7
	status.RemediationState.CorrectionBudgetTotal = 10
	status.Artifacts["futureArtifact"] = ArtifactDone

	projected, err := ProjectStatusV1(status)
	if err != nil {
		t.Fatalf("ProjectStatusV1() error = %v", err)
	}
	payload, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	wantRootKeys := []string{
		"schemaName", "schemaVersion", "changeName", "artifactStore", "planningHome", "changeRoot",
		"artifactPaths", "contextFiles", "artifacts", "taskProgress", "dependencies", "applyState",
		"actionContext", "relationships", "remediationState", "nextRecommended", "blockedReasons",
	}
	assertExactJSONKeys(t, document, wantRootKeys)

	var remediation map[string]json.RawMessage
	if err := json.Unmarshal(document["remediationState"], &remediation); err != nil {
		t.Fatalf("decode remediationState: %v", err)
	}
	assertExactJSONKeys(t, remediation, []string{
		"required", "complete", "failedEvidenceRevision", "lineageId", "generation", "fixBatch", "reason",
	})

	var artifacts map[string]json.RawMessage
	if err := json.Unmarshal(document["artifacts"], &artifacts); err != nil {
		t.Fatalf("decode artifacts: %v", err)
	}
	if _, exists := artifacts["futureArtifact"]; exists {
		t.Fatal("legacy projection leaked futureArtifact")
	}
}

func TestProjectStatusV1RejectsNonLegacyValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Status)
		want   string
	}{
		{
			name: "work-only next action",
			mutate: func(status *Status) {
				status.NextRecommended = "working"
			},
			want: `unsupported SDD v1 next action "working"`,
		},
		{
			name: "future artifact state",
			mutate: func(status *Status) {
				status.Artifacts["proposal"] = "checking"
			},
			want: `unsupported SDD v1 artifact "proposal" state "checking"`,
		},
		{
			name: "future store",
			mutate: func(status *Status) {
				status.ArtifactStore = "workrun"
			},
			want: `unsupported SDD v1 artifact store "workrun"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
			tt.mutate(&status)
			_, err := ProjectStatusV1(status)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ProjectStatusV1() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStatusRenderersEmbedOnlyStatusV1Projection(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
	status.RuntimeStatus = &RuntimeStatus{Schema: RuntimeStatusSchema, Change: "internal-only"}
	status.RemediationState.CorrectionBudgetRemaining = 7
	status.RemediationState.CorrectionBudgetTotal = 10

	rendered := map[string]string{
		"markdown":     RenderMarkdown(status),
		"dispatcher":   RenderDispatcherMarkdown(status),
		"native phase": RenderNativePhasePrompt(status, PhaseApply),
	}
	for name, output := range rendered {
		t.Run(name, func(t *testing.T) {
			for _, forbidden := range []string{"runtimeStatus", "correctionBudget", "internal-only"} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("%s leaked %q:\n%s", name, forbidden, output)
				}
			}
			if !strings.Contains(output, `"schemaName": "gentle-ai.sdd-status"`) {
				t.Fatalf("%s omitted projected SDD status:\n%s", name, output)
			}
		})
	}
}

func TestParseCommandArgsNegotiatesOnlyStatusV1(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "absent preserves legacy", args: []string{"change"}, want: ""},
		{name: "separate recognized value", args: []string{"change", "--contract", StatusContractV1}, want: StatusContractV1},
		{name: "inline recognized value", args: []string{"change", "--contract=" + StatusContractV1}, want: StatusContractV1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCommandArgs(tt.args)
			if err != nil {
				t.Fatalf("ParseCommandArgs() error = %v", err)
			}
			if got.Contract != tt.want {
				t.Fatalf("Contract = %q, want %q", got.Contract, tt.want)
			}
		})
	}
}

func TestParseCommandArgsRejectsUnsupportedContractReadOnly(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "explicit empty", args: []string{"change", "--contract="}, want: `unsupported sdd-status contract ""`},
		{name: "unknown", args: []string{"change", "--contract", "gentle-ai.work-status/v1"}, want: `unsupported sdd-status contract "gentle-ai.work-status/v1"`},
		{name: "missing value", args: []string{"change", "--contract"}, want: "--contract requires a value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCommandArgs(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseCommandArgs() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func assertExactJSONKeys(t *testing.T, document map[string]json.RawMessage, expected []string) {
	t.Helper()
	want := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		want[key] = struct{}{}
	}
	for key := range document {
		if _, ok := want[key]; !ok {
			t.Fatalf("unexpected JSON key %q", key)
		}
		delete(want, key)
	}
	for key := range want {
		t.Fatalf("missing JSON key %q", key)
	}
}
