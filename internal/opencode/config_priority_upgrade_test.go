package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedConfigPriorityAfterOpenCodeAgentMigration(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          int
	}{
		{"legacy marker", `{"agent":{"sdd-apply":{"__managed_by":"gentle-ai/sdd"}}}`, 2},
		{"migrated parity only", `{"agent":{"gentle-ai-worker":{"hidden":true,"prompt":"worker","permission":{"task":"deny"}}}}`, 1},
		{"visible odd subagent", `{"agent":{"gentle-ai-worker":{"hidden":false,"prompt":"worker","permission":{"task":"deny"}}}}`, 1},
		{"fresh review lens", `{"agent":{"review-risk":{"hidden":true,"prompt":"risk","permission":{"write":"deny"}}}}`, 1},
		{"unmanaged", `{"agent":{"general":{"hidden":true,"prompt":"custom","permission":{}}}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "opencode.json")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			if got := managedConfigPriority(path); got != tc.want {
				t.Fatalf("priority = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestReadConfigSnapshotMarksVisibleODDSubagentWithoutModelAsManaged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	content := `{"agent":{"gentle-ai-explore":{"mode":"subagent","hidden":false,"prompt":"explore","permission":{"task":"deny"}},"gentle-orchestrator":{"mode":"primary","hidden":false,"prompt":"custom","permission":{"task":"allow"}}}}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadConfigSnapshot(path)
	if err != nil {
		t.Fatalf("ReadConfigSnapshot() error = %v", err)
	}
	got := snapshot.Assignments["gentle-ai-explore"]
	if !got.Present || !got.Cleared || !got.Managed {
		t.Fatalf("gentle-ai-explore assignment = %+v, want Present=true Cleared=true Managed=true", got)
	}
	orch := snapshot.Assignments["gentle-orchestrator"]
	if !orch.Present || !orch.Cleared || orch.Managed {
		t.Fatalf("gentle-orchestrator assignment = %+v, want Present=true Cleared=true Managed=false", orch)
	}
}
