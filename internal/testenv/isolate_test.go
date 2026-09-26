package testenv_test

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// TestIsolateUnsetsKnownAgentRuntimeDirOverrides pins the exact set of
// environment variables Isolate must neutralize: PI_CODING_AGENT_DIR
// (internal/agents/pi.AgentConfigPath honors it as an absolute override,
// bypassing whatever homeDir a test passes in) and OPENCODE_CONFIG_DIR
// (internal/opencode.ResolveRuntimeConfigForHome does the same). Both are
// read unconditionally when absolute, unlike XDG_CONFIG_HOME/APPDATA in the
// agent adapters, which only apply when homeDir equals the real
// os.UserHomeDir() — already guarded against an arbitrary t.TempDir() home.
func TestIsolateUnsetsKnownAgentRuntimeDirOverrides(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", "/sentinel/pi-agent-dir")
	t.Setenv("OPENCODE_CONFIG_DIR", "/sentinel/opencode-config-dir")

	testenv.Isolate()

	for _, key := range []string{"PI_CODING_AGENT_DIR", "OPENCODE_CONFIG_DIR"} {
		if got, ok := os.LookupEnv(key); ok {
			t.Fatalf("Isolate() left %s = %q, want unset", key, got)
		}
	}
}
