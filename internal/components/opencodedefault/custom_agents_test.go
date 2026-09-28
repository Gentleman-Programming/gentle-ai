package opencodedefault

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverCustomAgents(t *testing.T) {
	tests := []struct {
		name, config string
		want         []string
	}{
		{"JSON and reserved identities", `{"agent":{"z-user":{},"a-user":{},"build":{},"plan":{},"general":{},"explore":{},"gentle-reviewer":{},"gentle-worker":{},"gentle-orchestrator":{},"sdd-orchestrator":{},"sdd-apply":{},"jd-judge-a":{},"review-risk":{}}}`, []string{"a-user", "z-user"}},
		{"JSONC", `{"agent":{"personal":{}}, // comment
 "default_agent":"personal",}`, []string{"personal"}},
		{"malformed JSON", `{"agent":{`, nil},
		{"malformed agent section", `{"agent":[]}`, nil},
		{"malformed entries", `{"agent":{"bad":null,"also-bad":42,"valid":{}}}`, []string{"valid"}},
		{"missing agent section", `{}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "opencode.json")
			if err := os.WriteFile(path, []byte(tt.config), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := DiscoverCustomAgents(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("agents = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		got, err := DiscoverCustomAgents(filepath.Join(t.TempDir(), "absent.json"))
		if err != nil || got != nil {
			t.Fatalf("agents = %v, err = %v", got, err)
		}
	})
}
