package assets

import (
	"strings"
	"testing"
)

func TestWindsurfSDDNewWorkflowConsumesInstalledAuthority(t *testing.T) {
	workflow := MustRead("windsurf/workflows/sdd-new.md")
	if strings.Count(workflow, "{{GENTLE_AI_SDD_SESSION_PREFLIGHT_AUTHORITY}}") != 1 {
		t.Fatal("thin entrypoint must consume exactly one rendered authority reference")
	}
	for _, stale := range []string{"### 1.", "Pace:", "Interactive or Automatic", "400", "Initialize OpenSpec and Engram lifecycle", ".sdd/"} {
		if strings.Contains(workflow, stale) {
			t.Errorf("thin entrypoint duplicates policy or legacy artifacts: %q", stale)
		}
	}
}
