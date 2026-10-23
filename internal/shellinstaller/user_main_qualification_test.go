package shellinstaller

import (
	"runtime"
	"testing"
)

// skipDarwinUnqualifiedMain documents why Main/advanced admission tests do not
// run on darwin: product policy intentionally refuses these profiles there
// ("SDK Main + stock provider is qualified on Linux amd64 today; macOS support
// is being completed"). The asserted behavior is correct on Linux; the skip
// records that darwin is not qualified yet instead of expecting refusals.
func skipDarwinUnqualifiedMain(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("SDK Main qualification is Linux amd64 only; macOS support is being completed")
	}
}
