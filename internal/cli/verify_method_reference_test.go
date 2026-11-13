package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// S2: resolve the method from its installed inline-settings carrier, not the source asset directory.
func TestPublicOpenCodeMethodReference(t *testing.T) {
	for _, args := range [][]string{
		{"--agent", "opencode", "--component", "persona"},
		{"--agent", "opencode", "--component", "skills", "--preset", "custom"},
		{"--agent", "opencode", "--preset", "full-gentleman"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			home := installTestHome(t)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Chdir(home)
			if _, err := RunInstall(args, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Agent map[string]map[string]any `json:"agent"`
			}
			if err := json.Unmarshal(raw, &config); err != nil {
				t.Fatal(err)
			}
			entry := config.Agent["gentle-ai-verify"]
			if !reflect.DeepEqual(entry["permission"], map[string]any{"write": "deny", "edit": "deny", "task": "deny"}) {
				t.Fatal("verifier permission boundary changed")
			}
			prompt, _ := entry["prompt"].(string)
			for _, clause := range []string{"Read-only means no edits to the repository or its git state", "Do not delegate to child agents, commit, or push.", "RDD review remains independent and parent-owned."} {
				if !strings.Contains(prompt, clause) {
					t.Fatalf("permission envelope lost %q", clause)
				}
			}
			want := assets.MustRead("skills/_shared/verify-method.md")
			if strings.Count(prompt, want) != 1 {
				t.Fatal("inline carrier must contain the exact canonical method once")
			}
			if strings.Contains(prompt, "[the shared verification method]") || strings.Contains(prompt, "{{GENTLE_AI_VERIFY_METHOD}}") {
				t.Fatal("unresolved mandatory method reference")
			}
			installed := filepath.Join(home, ".config", "opencode", "skills", "_shared", "verify-method.md")
			got, err := os.ReadFile(installed)
			if args[len(args)-1] == "full-gentleman" {
				if err != nil || string(got) != want {
					t.Fatalf("installed canonical method mismatch: %v", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("no-skills install unexpectedly wrote a shared method: %v", err)
			}
		})
	}
}
