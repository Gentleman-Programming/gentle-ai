//go:build linux && amd64

package shellinstaller

import (
	"strings"
	"testing"
)

func TestUserNativePublicationFailure(t *testing.T) {
	failed := NativeInstallerResult{Outcome: "failed", Completed: []string{"setup"},
		Details: []byte(`{"outcome":"failed","failedStep":"install-shell","detail":"setup\nfailed"}`)}
	err := userNativePublicationFailure(failed)
	if err == nil || !strings.Contains(err.Error(), `step="install-shell"`) ||
		!strings.Contains(err.Error(), `detail="setup\nfailed"`) || strings.Contains(err.Error(), "\n") {
		t.Fatal("original step or escaped diagnostic lost", err)
	}
	blocked := NativeInstallerResult{Outcome: "blocked", Reason: "consent-required"}
	if got := userNativePublicationFailure(blocked).Error(); got != "private installation: refused: original installer did not publish: blocked (consent-required)" {
		t.Fatal("existing blocked result changed", got)
	}
}
