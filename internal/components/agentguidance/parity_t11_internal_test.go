package agentguidance

import (
	"strings"
	"testing"
)

// harness-continuity S5/S10: the module pointer for edit surfaces names the
// parent's adjudication first; only human-owned scope is relayed.
func TestEditSurfaceModulePointerAdjudicatesFirst(t *testing.T) {
	bundle := buildClaudeTestModules(t)
	const want = "you prepare a bounded writer or adjudicate its edit-surface request (relay only human-owned scope)"
	if !strings.Contains(bundle.core, want) {
		t.Errorf("Claude module core is missing the adjudication pointer %q", want)
	}
	if strings.Contains(bundle.core, "relay its edit-surface request") {
		t.Error("Claude module core still relays every edit-surface request")
	}
}
