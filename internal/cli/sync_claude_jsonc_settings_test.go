package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestRunSyncAddsClaudeHooksToJSONCSettings(t *testing.T) {
	home := installTestHome(t)
	restoreBackupHome := backup.UserHomeDirFn
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() { backup.UserHomeDirFn = restoreBackupHome })
	if err := state.Write(home, state.InstallState{
		InstalledAgents:     []string{string(model.AgentClaudeCode)},
		SelectionConfigured: true,
		Persona:             "neutral",
	}); err != nil {
		t.Fatalf("state.Write() error = %v", err)
	}
	const userText = "{\n  // Personal settings.\n  \"model\": \"opus\", /* chosen by hand */\n"
	settings := filepath.Join(home, ".claude", "settings.json")
	mustWriteFile(t, settings, []byte(userText+"}\n"))

	if _, err := RunSync([]string{"--agents", "claude-code"}); err != nil {
		t.Fatalf("RunSync() over JSONC Claude settings error = %v", err)
	}

	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), userText) {
		t.Fatalf("sync did not preserve the JSONC settings text:\n%s", data)
	}
	for _, command := range []string{"gentle-ai review stop-hook --agent claude-code", "gentle-ai telemetry runtime claude --json", "gentle-ai skill-registry refresh"} {
		if !strings.Contains(string(data), command) {
			t.Fatalf("sync did not install hook %q:\n%s", command, data)
		}
	}
}
