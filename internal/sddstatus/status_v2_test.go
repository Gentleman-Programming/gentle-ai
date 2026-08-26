package sddstatus

import (
	"encoding/json"
	"path/filepath"
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

func TestFinalVerificationReason(t *testing.T) {
	tests := []struct {
		name                              string
		seed                              func(t *testing.T, root string)
		wantReason                        string
		wantVerify, wantArchive, wantNext string
	}{
		{
			name: "OpenSpec missing report", seed: func(t *testing.T, root string) {
				seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
			},
			wantReason: "verify_report_missing", wantVerify: "ready", wantArchive: "blocked", wantNext: "verify",
		},
		{
			name: "OpenSpec passing report", seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))
			},
			wantVerify: "all_done", wantArchive: "ready", wantNext: "archive",
		},
		{
			name: "Engram missing report", seed: func(t *testing.T, root string) {
				mkdir(t, filepath.Join(root, ".engram"))
				project := strings.ToLower(filepath.Base(root))
				restore := stubEngramExport(t, []engramObservation{
					{Title: "sdd/thin/proposal", Content: "# Proposal\n", Project: project, Scope: "project"},
					{Title: "sdd/thin/spec", Content: "# Spec\n", Project: project, Scope: "project"},
					{Title: "sdd/thin/design", Content: "# Design\n", Project: project, Scope: "project"},
					{Title: "sdd/thin/tasks", Content: "- [x] 1.1 Work\n", Project: project, Scope: "project"},
				})
				t.Cleanup(restore)
			},
			wantVerify: "ready", wantArchive: "blocked", wantNext: "verify",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.seed(t, root)
			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			if string(status.Dependencies.Verify) != tt.wantVerify || string(status.Dependencies.Archive) != tt.wantArchive || status.NextRecommended != tt.wantNext {
				t.Fatalf("status = verify %q archive %q next %q", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
			}
			projected, err := ProjectStatusV2(status)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(projected)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatal(err)
			}
			reason, present := document["finalVerificationReason"]
			if present != (tt.wantReason != "") || (present && string(reason) != `"`+tt.wantReason+`"`) {
				t.Fatalf("finalVerificationReason = %s, present=%t, want %q", reason, present, tt.wantReason)
			}
		})
	}
}
