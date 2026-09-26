package persona

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// TestMain neutralizes ambient agent runtime-dir overrides (PI_CODING_AGENT_DIR,
// OPENCODE_CONFIG_DIR) before any test runs: this package resolves real Pi and
// OpenCode adapters (agents.NewAdapter(model.AgentPi), opencodeAdapter()) whose
// config-path resolution honors these overrides directly from the environment,
// bypassing whatever t.TempDir() home a test passes in.
func TestMain(m *testing.M) {
	testenv.Isolate()
	os.Exit(m.Run())
}
