package pi

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// TestMain neutralizes ambient agent runtime-dir overrides before any test
// runs: AgentConfigPath honors an absolute PI_CODING_AGENT_DIR over the home
// directory a test passes in, so an exported override from Gentle Shell would
// otherwise redirect path resolution outside t.TempDir().
func TestMain(m *testing.M) {
	testenv.Isolate()
	os.Exit(m.Run())
}
