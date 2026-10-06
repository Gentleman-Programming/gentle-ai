package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// TestOpenCodeFamilyInstallHonorsUserQuestionDeny: install grants the
// orchestrator question "allow" (#4816) only where the user has no deny policy
// that covers it. An agent rule outranks the global one, so writing "allow"
// over a global or agent-wide deny would silently override the user.
func TestOpenCodeFamilyInstallHonorsUserQuestionDeny(t *testing.T) {
	for _, target := range []struct {
		agent string
		args  []string
		path  func(home string) string
	}{
		{"opencode", []string{"--agent", "opencode", "--component", "persona"}, func(home string) string {
			return filepath.Join(home, ".config", "opencode", "opencode.json")
		}},
		{"kilocode", []string{"--agent", "kilocode", "--preset", "full-gentleman"}, kiloSettingsPath},
	} {
		for _, policy := range []struct {
			name     string
			settings string
		}{
			{"global deny", `{"permission":"deny"}`},
			{"global wildcard deny", `{"permission":{"*":"deny"}}`},
			{"global question deny", `{"permission":{"question":"deny"}}`},
			{"agent deny", `{"agent":{"gentle-orchestrator":{"permission":"deny"}}}`},
			{"agent wildcard deny", `{"agent":{"gentle-orchestrator":{"permission":{"*":"deny"}}}}`},
		} {
			for _, version := range openCodeRuntimeVersions {
				t.Run(target.agent+"/"+policy.name+"/"+version, func(t *testing.T) {
					home := installTestHome(t)
					stubOpenCodeRuntimeVersion(t, home, version)
					path := target.path(home)
					if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(policy.settings), 0o600); err != nil {
						t.Fatal(err)
					}
					if _, err := RunInstall(target.args, system.DetectionResult{}); err != nil {
						t.Fatal(err)
					}
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					root, err := filemerge.UnmarshalJSONObject(raw)
					if err != nil {
						t.Fatal(err)
					}
					agents, _ := root["agent"].(map[string]any)
					orchestrator, _ := agents["gentle-orchestrator"].(map[string]any)
					if permission, ok := orchestrator["permission"].(map[string]any); ok {
						if _, written := permission["question"]; written {
							t.Fatalf("question rule written over the user's deny policy: %s", raw)
						}
					}
				})
			}
		}
	}
}
