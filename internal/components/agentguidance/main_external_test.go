package agentguidance_test

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/testenv"
)

// TestMain wires the production review contract source the installer
// registers, so rendered prompts in this package match what users receive.
// It also neutralizes ambient agent runtime-dir overrides (PI_CODING_AGENT_DIR
// and friends): this package's many catalog.AllAgents() loops resolve Pi's
// config path, and a developer shell exporting that var (Gentle Shell does)
// would otherwise redirect these tests into the real ~/.pi. Finally, it pins a
// supported Codex runtime version, because the Codex GPT-5.6 check shells out
// to `codex --version` and would otherwise let the host binary decide results.
func TestMain(m *testing.M) {
	testenv.Isolate()
	_ = codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil)
	agentguidance.SetReviewContractSource(reviewassets.ReviewExecutionContractFor)
	os.Exit(m.Run())
}
