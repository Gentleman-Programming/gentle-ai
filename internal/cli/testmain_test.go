package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	skipOpenCodePluginPackageInstall = true
	os.Exit(m.Run())
}
