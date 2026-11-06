package communitytool

import (
	"regexp"
	"strings"
	"testing"
)

// TestPiCodeGraphOverlayHasNoRetiredWorkflowReferences ratchets the text
// Gentle AI appends to Pi child agents. The child body belongs to Gentle Shell,
// so only the Gentle-rendered overlay is scanned, for both overlay shapes.
func TestPiCodeGraphOverlayHasNoRetiredWorkflowReferences(t *testing.T) {
	retired := regexp.MustCompile(`(?i)sdd|openspec`)
	const body = "---\nname: worker\ntools: read, bash\n---\n\nWorker body.\n"
	for _, injectTools := range []bool{true, false} {
		rendered, err := renderPiChild(body, []string{"read", "bash", "mcp"}, injectTools)
		if err != nil {
			t.Fatal(err)
		}
		start := strings.Index(rendered, "\n<!-- gentle-ai:pi-codegraph")
		if start < 0 {
			t.Fatalf("injectTools=%v rendered no CodeGraph overlay:\n%s", injectTools, rendered)
		}
		for index, line := range strings.Split(rendered[start:], "\n") {
			if retired.MatchString(line) {
				t.Errorf("injectTools=%v overlay line %d renders retired workflow reference: %s", injectTools, index+1, line)
			}
		}
	}
}
