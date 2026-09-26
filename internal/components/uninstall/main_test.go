package uninstall

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// TestMain neutralizes ambient agent runtime-dir overrides (PI_CODING_AGENT_DIR,
// OPENCODE_CONFIG_DIR) before any test runs: this package constructs a real Pi
// adapter (pi.NewAdapter()) and resolves its config paths, which honor these
// overrides directly from the environment, bypassing whatever t.TempDir() home
// a test passes in.
func TestMain(m *testing.M) {
	testenv.Isolate()
	os.Exit(m.Run())
}
