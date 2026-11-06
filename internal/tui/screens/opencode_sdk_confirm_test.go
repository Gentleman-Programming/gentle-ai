package screens

import (
	"strings"
	"testing"
)

func TestOpenCodeSDKConfirmNamesMutationAndRollbackBoundary(t *testing.T) {
	view := RenderOpenCodeSDKConfirm("@opencode/plugin@2.0.4", "npm", "/home/test/.config/opencode", 1)
	for _, text := range []string{"@opencode/plugin@2.0.4", "npm", "/home/test/.config/opencode", "fresh config", "network", "package.json", "lockfile", "node_modules", "registry.npmjs.org", "without your package-manager credentials", "ignores install scripts", "NOT covered", "No / Back"} {
		if !strings.Contains(view, text) {
			t.Errorf("confirmation missing %q", text)
		}
	}
}
