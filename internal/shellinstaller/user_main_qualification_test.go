package shellinstaller

import (
	"runtime"
	"testing"
)

// skipDarwinUnqualifiedMain documents why these Linux admission tests do not
// run on darwin: they assert the Linux legacy authenticated worker and its
// GGA/stock PNPM publication refusals, which have no darwin producer. Main
// preview on macOS arm64 is covered by the unix Main and overlay tests.
func skipDarwinUnqualifiedMain(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("Linux legacy worker admission; darwin Main is covered by the unix Main tests")
	}
}
