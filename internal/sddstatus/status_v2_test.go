package sddstatus

import (
	"strings"
	"testing"
)

func TestProjectStatusV2RejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Status)
		want   string
	}{
		{
			name: "unknown next action",
			mutate: func(status *Status) {
				status.NextRecommended = "working"
			},
			want: `unsupported SDD v2 next action "working"`,
		},
		{
			name: "unknown artifact state",
			mutate: func(status *Status) {
				status.Artifacts["proposal"] = "checking"
			},
			want: `unsupported SDD v2 artifact "proposal" state "checking"`,
		},
		{
			name: "unknown artifact store",
			mutate: func(status *Status) {
				status.ArtifactStore = "workrun"
			},
			want: `unsupported SDD v2 artifact store "workrun"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
			tt.mutate(&status)
			_, err := ProjectStatusV2(status)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ProjectStatusV2() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStatusRenderersEmbedOnlyStatusV2Projection(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
	status.RuntimeStatus = &RuntimeStatus{Schema: RuntimeStatusSchema, Change: "internal-only"}

	rendered := map[string]string{
		"markdown":     RenderMarkdown(status),
		"dispatcher":   RenderDispatcherMarkdown(status),
		"native phase": RenderNativePhasePrompt(status, PhaseApply),
	}
	for name, output := range rendered {
		t.Run(name, func(t *testing.T) {
			for _, forbidden := range []string{"runtimeStatus", "internal-only", "reviewGate", "reviewTransaction", "reVerify"} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("%s leaked %q:\n%s", name, forbidden, output)
				}
			}
			if !strings.Contains(output, `"schemaVersion": 2`) {
				t.Fatalf("%s omitted v2 projected SDD status:\n%s", name, output)
			}
		})
	}
}
