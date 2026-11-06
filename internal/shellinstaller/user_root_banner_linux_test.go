//go:build linux

package shellinstaller

import (
	"bytes"
	"strings"
	"testing"
)

func TestUserRootRiskBannerUID(t *testing.T) {
	var out bytes.Buffer
	userRootRiskBannerUID(&out, 1000)
	if out.Len() != 0 {
		t.Fatalf("non-root launches must stay silent, got %q", out.String())
	}
	userRootRiskBannerUID(&out, 0)
	if !strings.Contains(out.String(), "WARNING: running as root") || !strings.Contains(out.String(), "disabled") {
		t.Fatalf("root launches must print the sandbox-disabled warning, got %q", out.String())
	}
	userRootRiskBannerUID(nil, 0) // Must not panic without a writer.
}
