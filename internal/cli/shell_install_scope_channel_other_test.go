//go:build !windows

package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// wantParsedRequest returns the request the non-Windows worker entry must
// parse back: the entry keeps the confirmed request verbatim, channel
// included (absent on this platform).
func wantParsedRequest(t *testing.T, req shellinstaller.UserInstallRequest, _ string) shellinstaller.UserInstallRequest {
	t.Helper()
	return req
}
