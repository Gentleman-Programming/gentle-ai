package main

import (
	"strings"
	"testing"
)

func TestRequireReviewedSupersetPrePushAllowObservationFailures(t *testing.T) {
	sandbox := &Sandbox{
		Lineage: reviewedSupersetLineage,
		Scratch: map[string]string{
			"reviewed-superset-tree":           "receipt-tree",
			"reviewed-superset-head-tree":      "local-head-tree",
			"reviewed-superset-outgoing-paths": reviewedSupersetPathB,
		},
	}

	tests := []struct {
		name        string
		observation Observation
		want        []string
		notWant     string
	}{
		{
			name: "typed gate denial preserves receipt binding evidence",
			observation: Observation{
				ExitCode: 1,
				Stdout:   `{"result":"scope-changed","allowed":false,"context":{"lineage_id":"reviewed-superset-pre-push","denial":{"stage":"receipt-binding","code":"candidate-or-paths-mismatch"}}}`,
			},
			want: []string{
				"exit=1",
				`result="scope-changed"`,
				"receipt_tree=receipt-tree",
				"local_head_tree=local-head-tree",
				"receipt_paths={docs/reviewed-a.md,docs/reviewed-b.md}",
				"outgoing_paths={docs/reviewed-b.md}",
				`denial_stage="receipt-binding"`,
				`denial_code="candidate-or-paths-mismatch"`,
			},
		},
		{
			name: "malformed stdout reports command failure",
			observation: Observation{
				ExitCode: 23,
				Stdout:   "not-json",
				Stderr:   "review validate unavailable\nretry later",
			},
			want:    []string{"exited 23: review validate unavailable"},
			notWant: "parse reviewed-superset pre-push result",
		},
		{
			name: "empty stdout reports command failure",
			observation: Observation{
				ExitCode: 24,
				Stderr:   "review validate produced no envelope\nretry later",
			},
			want:    []string{"exited 24: review validate produced no envelope"},
			notWant: "parse reviewed-superset pre-push result",
		},
		{
			name: "malformed successful stdout reports parse failure",
			observation: Observation{
				Stdout: "not-json",
				Stderr: "review validate returned malformed JSON",
			},
			want: []string{"parse reviewed-superset pre-push result", "stderr: review validate returned malformed JSON"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireReviewedSupersetPrePushAllow(sandbox, tt.observation)
			if err == nil {
				t.Fatal("accepted an invalid pre-push observation")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want %q", err, want)
				}
			}
			if tt.notWant != "" && strings.Contains(err.Error(), tt.notWant) {
				t.Errorf("error = %q, must not contain %q", err, tt.notWant)
			}
		})
	}
}
