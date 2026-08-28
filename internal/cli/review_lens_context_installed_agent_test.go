package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
)

// gentleAIMarkerToken matches every envelope marker either half of the
// reviewer transport can name, so the guard below compares the two halves by
// what they actually contain rather than by a name restated in the test.
var gentleAIMarkerToken = regexp.MustCompile(`GENTLE_AI_[A-Z_]+`)

// TestLensContextBlockCarriesEveryMarkerInstalledClaudeLensAgentsRequire is the
// regression guard for issue #2777: the renderer a Claude parent relays and the
// installed Claude lens agent definition are two halves of one shipped
// mechanism, and a marker the agent requires but the renderer never emits makes
// every rendered prompt inadmissible. The reviewer has no execution tools, so
// the prompt is its only channel and no caller can route around the mismatch.
//
// The assertion is deliberately containment rather than equality: the block may
// carry more structure than the agent names, but never less.
func TestLensContextBlockCarriesEveryMarkerInstalledClaudeLensAgentsRequire(t *testing.T) {
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]

	block := lensContextBlock(t, handle, lens)

	installHome := t.TempDir()
	if _, err := sdd.Inject(installHome, claude.NewAdapter(), ""); err != nil {
		t.Fatalf("install Claude agent definitions: %v", err)
	}
	definition, err := os.ReadFile(filepath.Join(installHome, ".claude", "agents", lens+".md"))
	if err != nil {
		t.Fatalf("read installed %s definition: %v", lens, err)
	}

	required := gentleAIMarkerToken.FindAllString(string(definition), -1)
	if len(required) == 0 {
		t.Fatalf("installed %s definition names no envelope marker at all:\n%s", lens, definition)
	}
	sort.Strings(required)
	required = slices.Compact(required)

	var missing []string
	for _, marker := range required {
		if !strings.Contains(block, marker) {
			missing = append(missing, marker)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("installed %s definition requires markers the rendered lens context never emits: %v\nemitted: %v",
			lens, missing, slices.Compact(gentleAIMarkerToken.FindAllString(block, -1)))
	}
}
