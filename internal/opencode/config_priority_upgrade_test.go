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
