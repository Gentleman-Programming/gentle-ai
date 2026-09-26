package opencode

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// TestMain neutralizes ambient agent runtime-dir overrides before any test
// runs: config resolution prepends an absolute OPENCODE_CONFIG_DIR override,
// so an inherited developer setting would otherwise make tests read the real
// OpenCode config. Tests that exercise the override still set it explicitly.
func TestMain(m *testing.M) {
	testenv.Isolate()
	os.Exit(m.Run())
}
