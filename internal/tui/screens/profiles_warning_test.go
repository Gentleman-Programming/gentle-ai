package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

func TestRenderProfiles_IncludesOpenCodeOnlyWarning(t *testing.T) {
	out := screens.RenderProfiles(nil, 0, nil)
	if !strings.Contains(out, "OpenCode-only") {
		t.Fatalf("expected OpenCode-only warning in profiles screen")
	}
}
