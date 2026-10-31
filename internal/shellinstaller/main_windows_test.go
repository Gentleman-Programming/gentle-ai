//go:build windows

package shellinstaller

import (
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/testsupport/windowsrunner"
)

// The elevated GitHub Windows runner creates objects owned by
// BUILTIN\Administrators and exposes an 8.3 temp alias; windowsrunner makes
// this process behave like an unelevated account. Product code is unchanged.
func TestMain(m *testing.M) {
	windowsrunner.Prepare("shellinstaller")
	os.Exit(m.Run())
}
