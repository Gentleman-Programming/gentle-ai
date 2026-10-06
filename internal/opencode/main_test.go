package opencode

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/testenv"
)

// TestMain neutralizes ambient agent runtime-dir overrides before any test
// runs: config resolution prepends an absolute OPENCODE_CONFIG_DIR override,
// so an inherited developer setting would otherwise make tests read the real
// OpenCode config. Tests that exercise the override still set it explicitly.
// The login shell selects which profile activation writes, so it is cleared
// too; profile tests pass ActivationOptions.Shell or set SHELL explicitly.
func TestMain(m *testing.M) {
	testenv.Isolate()
	_ = os.Unsetenv("SHELL")
	_ = os.Unsetenv("ZDOTDIR")
	os.Exit(m.Run())
}
