//go:build windows

package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// wantParsedRequest returns the request the Windows worker entry must parse
// back: it resolves the install channel (stable by default, main for
// main-preview SDK sources). The scope and confirmation must still survive
// the round trip unchanged.
func wantParsedRequest(t *testing.T, req shellinstaller.UserInstallRequest, sdkSource string) shellinstaller.UserInstallRequest {
	t.Helper()
	if sdkSource == "main-preview" {
		req.Channel = "main"
	} else {
		req.Channel = "stable"
	}
	return req
}
