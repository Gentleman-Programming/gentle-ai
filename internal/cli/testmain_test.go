package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	skipOpenCodePluginPackageInstall = true
	_ = os.Setenv("GENTLE_AI_SKIP_OPENCODE_PLUGIN_INSTALL", "1")
	os.Exit(m.Run())
}
