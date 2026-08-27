//go:build real_agent_e2e

package organicruntime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/permissions"
)

func TestOpenCodeSensitivePathPermissions(t *testing.T) {
	if os.Getenv(realAgentE2EEnvironment) != "1" {
		t.Skip("set GENTLE_AI_REAL_AGENT_E2E=1 to run the pinned OpenCode sensitive-path journey")
	}
	requireOrganicExecutableVersion(t, "opencode", "1.18.21")
	workspace := t.TempDir()
	const secret, safe = "SECRET_CANARY_787", "SAFE_CANARY_787"
	if err := os.Mkdir(filepath.Join(workspace, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{".env": secret, "config/.env": secret, "safe.txt": safe} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(".env", filepath.Join(workspace, "env-alias.txt")); err != nil {
		t.Fatal(err)
	}
	dependencies, home := prepareOpenCodeConfig(t), t.TempDir()
	pluginRoot := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(pluginRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dependencies, "opencode", "node_modules"), filepath.Join(pluginRoot, "node_modules")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(pluginRoot))
	settingsPath := filepath.Join(pluginRoot, "opencode.json")

	for _, tt := range []struct {
		name, tool, canary string
		arguments          map[string]any
		denied             bool
	}{
		{"read .env denied", "read", secret, map[string]any{"filePath": ".env"}, true},
		{"grep .env denied", "grep", secret, map[string]any{"pattern": "SECRET_", "path": ".env"}, true},
		{"grep nested .env denied", "grep", secret, map[string]any{"pattern": "SECRET_", "path": "nested/../.env"}, true},
		{"grep .env.local denied", "grep", secret, map[string]any{"pattern": "SECRET_", "path": ".env.local"}, true},
		{"grep *.env include denied", "grep", secret, map[string]any{"pattern": "SECRET_", "path": "safe.txt", "include": "*.env"}, true},
		{"grep symlink to .env denied", "grep", secret, map[string]any{"pattern": "SECRET_", "path": "env-alias.txt"}, true},
		{"grep pathless denied", "grep", secret, map[string]any{"pattern": "SECRET_"}, true},
		{"grep broad scan denied", "grep", secret, map[string]any{"pattern": "SECRET_", "path": "."}, true},
		{"grep named directory denied", "grep", secret, map[string]any{"pattern": "SECRET_", "path": "config"}, true},
		{"bash quoted .env piped denied", "bash", secret, map[string]any{"command": "grep SECRET_ '.env' | cat"}, true},
		{"grep safe file allowed", "grep", safe, map[string]any{"pattern": safe, "path": "safe.txt"}, false},
		{"bash grep safe file allowed", "bash", safe, map[string]any{"command": "grep " + safe + " safe.txt"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newOpenCodeFixtureServer(t, []openCodeTurn{{tool: tt.tool, arguments: tt.arguments}}, "")
			defer fixture.Close()
			if err := os.WriteFile(settingsPath, []byte(sensitivePathOpenCodeConfig(t, fixture.URL)), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := permissions.Inject(home, opencode.NewAdapter()); err != nil {
				t.Fatal(err)
			}
			output := runOpenCodeSensitivePathTool(t, workspace, home)
			fixture.assertComplete(t, false)
			if tt.denied {
				if strings.Contains(output, `"status":"completed"`) || !strings.Contains(output, `"status":"error"`) || strings.Contains(output, tt.canary) {
					t.Fatalf("%s did not fail closed without leakage:\n%s", tt.name, output)
				}
			} else if !strings.Contains(output, `"status":"completed"`) || !strings.Contains(output, tt.canary) {
				t.Fatalf("%s did not disclose safe canary:\n%s", tt.name, output)
			}
		})
	}
}

func sensitivePathOpenCodeConfig(t *testing.T, serverURL string) string {
	t.Helper()
	config := map[string]any{
		"provider": map[string]any{"fixture": map[string]any{"npm": "@ai-sdk/openai-compatible", "name": "Sensitive-path fixture", "options": map[string]any{"baseURL": serverURL + "/v1", "apiKey": "fixture"}, "models": map[string]any{"fixture": map[string]any{"name": "Fixture"}}}},
		"agent":    map[string]any{"organic": map[string]any{"description": "Sensitive-path permission E2E", "mode": "primary", "model": "fixture/fixture"}}, "compaction": map[string]any{"auto": false},
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func runOpenCodeSensitivePathTool(t *testing.T, workspace, home string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), organicAgentTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "opencode", "run", "--format", "json", "--agent", "organic", "--model", "fixture/fixture", "--dir", workspace, "run the requested tool")
	command.Dir = workspace
	command.Env = append(os.Environ(), "OPENCODE_TEST_HOME="+filepath.Join(home, "opencode"), "OPENCODE_AUTH_CONTENT={}", "OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_AUTOUPDATE=1", "OPENCODE_DISABLE_AUTOCOMPACT=1", "OPENCODE_DISABLE_CLAUDE_CODE=1", "OPENCODE_DISABLE_DEFAULT_PLUGINS=1", "OPENCODE_DISABLE_EXTERNAL_SKILLS=1", "OPENCODE_DISABLE_LSP_DOWNLOAD=1", "OPENCODE_DISABLE_MODELS_FETCH=1", "OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER=1", "OPENCODE_FAST_BOOT=1")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("opencode run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return stdout.String() + stderr.String()
}
