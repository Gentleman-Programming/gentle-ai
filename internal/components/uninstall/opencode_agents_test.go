package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestRemoveOpenCodeOrchestratorOnlyWhenEntireEntryIsManaged(t *testing.T) {
	for _, tc := range []struct {
		name, userPrompt string
		extra            map[string]any
		keep             bool
	}{
		{name: "managed-only"},
		{name: "user prompt", userPrompt: "User instructions\n", keep: true},
		{name: "user option", extra: map[string]any{"temperature": 0.2}, keep: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "opencode.json")
			prompt := filemerge.InjectMarkdownSection(tc.userPrompt, "orchestrator", "Managed instructions")
			prompt = filemerge.InjectMarkdownSection(prompt, "agent-routing", "Managed routing")
			orchestrator := map[string]any{"prompt": prompt, "permission": map[string]any{"task": map[string]any{"jd-judge-b": "allow"}}}
			for key, value := range tc.extra {
				orchestrator[key] = value
			}
			var judge map[string]any
			for _, spec := range opencodeagents.Parity(model.AgentOpenCode) {
				if spec.Name == "jd-judge-b" {
					var entryErr error
					judge, entryErr = opencodeagents.Entry(spec)
					if entryErr != nil {
						t.Fatal(entryErr)
					}
					break
				}
			}
			root := map[string]any{"agent": map[string]any{"gentle-orchestrator": orchestrator, "jd-judge-b": judge}}
			raw, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			op := removeOpenCodeFamilyAgents(path, model.AgentOpenCode)
			changed, _, err := op.apply(path)
			if err != nil || !changed {
				t.Fatalf("rewrite: changed=%v err=%v", changed, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var after map[string]any
			if err := json.Unmarshal(body, &after); err != nil {
				t.Fatal(err)
			}
			agents := after["agent"].(map[string]any)
			entry, exists := agents["gentle-orchestrator"].(map[string]any)
			if exists != tc.keep {
				t.Fatalf("orchestrator existence=%v, want %v: %s", exists, tc.keep, body)
			}
			if exists {
				remainingPrompt := entry["prompt"].(string)
				if strings.Contains(remainingPrompt, "gentle-ai:") || !strings.Contains(remainingPrompt, tc.userPrompt) {
					t.Fatalf("managed prompt not stripped or user text lost: %s", body)
				}
				if _, ok := entry["permission"]; ok {
					t.Fatalf("managed task retained: %s", body)
				}
			}
		})
	}
}
