package testenv_test

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/testenv"
)

// Isolate removes inherited overrides entirely, rather than setting empty
// values. Behavioral path/write coverage lives in the subprocess regressions.
func TestIsolateUnsetsKnownAgentRuntimeDirOverrides(t *testing.T) {
	for _, key := range []string{"PI_CODING_AGENT_DIR", "OPENCODE_CONFIG_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(key, t.TempDir())
	}

	testenv.Isolate()

	for _, key := range []string{"PI_CODING_AGENT_DIR", "OPENCODE_CONFIG_DIR", "XDG_CONFIG_HOME"} {
		if got, ok := os.LookupEnv(key); ok {
			t.Fatalf("Isolate() left %s = %q, want unset", key, got)
		}
	}
}
