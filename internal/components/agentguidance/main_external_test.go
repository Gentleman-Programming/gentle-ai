package agentguidance_test

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// TestMain wires the production review contract source the installer
// registers, so rendered prompts in this package match what users receive.
// It also neutralizes ambient agent runtime-dir overrides (PI_CODING_AGENT_DIR
// and friends): this package's many catalog.AllAgents() loops resolve Pi's
// config path, and a developer shell exporting that var (Gentle Shell does)
// would otherwise redirect these tests into the real ~/.pi.
func TestMain(m *testing.M) {
	testenv.Isolate()
	agentguidance.SetReviewContractSource(reviewassets.ReviewExecutionContractFor)
	os.Exit(m.Run())
}
