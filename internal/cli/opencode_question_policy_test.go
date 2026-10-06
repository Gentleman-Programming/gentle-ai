package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// TestOpenCodeFamilyInstallHonorsUserQuestionDeny: install grants the
// orchestrator question "allow" (#4816) only where the user has no deny policy
// that covers it. Agent rules are evaluated after global ones and the last
// match wins, so writing "allow" over a global or agent-wide deny would
// silently override the user. OpenCode 2.x also reads the native
// `permissions` rule list and the legacy root `tools` map; 1.x turns an
// agent's `tools` into permissions its `permission` map then overrides.
func TestOpenCodeFamilyInstallHonorsUserQuestionDeny(t *testing.T) {
	v2Only := []string{"2.0.23"}
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
			versions []string
		}{
			{"global deny", `{"permission":"deny"}`, openCodeRuntimeVersions},
			{"global wildcard deny", `{"permission":{"*":"deny"}}`, openCodeRuntimeVersions},
			{"global question deny", `{"permission":{"question":"deny"}}`, openCodeRuntimeVersions},
			{"global question pattern deny", `{"permission":{"question":{"*":"deny"}}}`, openCodeRuntimeVersions},
			{"global tools question off", `{"tools":{"question":false}}`, openCodeRuntimeVersions},
			{"global native wildcard deny", `{"permissions":[{"action":"*","resource":"*","effect":"deny"}]}`, v2Only},
			{"global native question deny", `{"permissions":[{"action":"question","effect":"deny"}]}`, v2Only},
			{"agent deny", `{"agent":{"gentle-orchestrator":{"permission":"deny"}}}`, openCodeRuntimeVersions},
			{"agent wildcard deny", `{"agent":{"gentle-orchestrator":{"permission":{"*":"deny"}}}}`, openCodeRuntimeVersions},
			{"agent question pattern deny", `{"agent":{"gentle-orchestrator":{"permission":{"question":{"*":"deny"}}}}}`, openCodeRuntimeVersions},
			{"agent tools question off", `{"agent":{"gentle-orchestrator":{"tools":{"question":false}}}}`, openCodeRuntimeVersions},
			{"agent native wildcard deny", `{"agent":{"gentle-orchestrator":{"permissions":[{"action":"*","resource":"*","effect":"deny"}]}}}`, v2Only},
			{"native agent question deny", `{"agents":{"gentle-orchestrator":{"permissions":[{"action":"question","resource":"*","effect":"deny"}]}}}`, v2Only},
		} {
			for _, version := range policy.versions {
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
						if permission["question"] == "allow" {
							t.Fatalf("question rule written over the user's deny policy: %s", raw)
						}
					}
					assertUserPolicyKept(t, []byte(policy.settings), root)
				})
			}
		}
	}
}

// TestOpenCodeInstallAllowsQuestionBesideUncoveredPolicy: a user policy that
// does not deny question everywhere still gets the orchestrator grant.
func TestOpenCodeInstallAllowsQuestionBesideUncoveredPolicy(t *testing.T) {
	for _, settings := range []string{
		`{"tools":{"question":true}}`,
		`{"permission":{"bash":"deny"}}`,
		`{"permissions":[{"action":"question","resource":"docs/*","effect":"deny"},{"action":"shell","resource":"*","effect":"deny"}]}`,
	} {
		t.Run(settings, func(t *testing.T) {
			home := installTestHome(t)
			stubOpenCodeRuntimeVersion(t, home, "2.0.23")
			path := filepath.Join(home, ".config", "opencode", "opencode.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
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
			assertOpenCodeOrchestratorPermissions(t, agents)
			assertUserPolicyKept(t, []byte(settings), root)
		})
	}
}

// assertUserPolicyKept checks that every policy rule the user wrote, global or
// on the orchestrator, is still encoded the same after install. Install may add
// its own rules beside them (task grants, the permissions component), never
// change or drop one.
func assertUserPolicyKept(t *testing.T, settings []byte, after map[string]any) {
	t.Helper()
	before, err := filemerge.UnmarshalJSONObject(settings)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	kept := func(label string, user, got any) {
		rules, ok := user.(map[string]any)
		if !ok {
			if encode(got) != encode(user) {
				t.Fatalf("user %s changed: before %s, after %s", label, encode(user), encode(got))
			}
			return
		}
		gotRules, _ := got.(map[string]any)
		for name, rule := range rules {
			if encode(gotRules[name]) != encode(rule) {
				t.Fatalf("user %s.%s changed: before %s, after %s", label, name, encode(rule), encode(gotRules[name]))
			}
		}
	}
	for key, value := range before {
		if !slices.Contains([]string{"agent", "agents"}, key) {
			kept(key, value, after[key])
			continue
		}
		userEntry, _ := value.(map[string]any)["gentle-orchestrator"].(map[string]any)
		agents, _ := after[key].(map[string]any)
		entry, _ := agents["gentle-orchestrator"].(map[string]any)
		for field, rule := range userEntry {
			kept(key+".gentle-orchestrator."+field, rule, entry[field])
		}
	}
}
