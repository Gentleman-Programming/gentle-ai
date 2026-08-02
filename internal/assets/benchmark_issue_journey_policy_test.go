package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueClosingWorkflowRequiresReplayableBenchmarkJourney(t *testing.T) {
	const sourceMarker = "Journey.Source"
	const boundaryMarker = "public/runtime"

	rootFiles := []string{
		"CONTRIBUTING.md",
		filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"),
		filepath.Join("bench", "README.md"),
		filepath.Join("skills", "branch-pr", "SKILL.md"),
		filepath.Join("skills", "work-unit-commits", "SKILL.md"),
		filepath.Join("skills", "gentle-ai-collab-perfect", "SKILL.md"),
	}
	for _, path := range rootFiles {
		t.Run(path, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", path))
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", path, err)
			}
			assertBenchmarkJourneyPolicy(t, string(content), sourceMarker, boundaryMarker)
		})
	}

	for _, path := range []string{
		"skills/branch-pr/SKILL.md",
		"skills/work-unit-commits/SKILL.md",
	} {
		t.Run("embedded/"+path, func(t *testing.T) {
			assertBenchmarkJourneyPolicy(t, MustRead(path), sourceMarker, boundaryMarker)
		})
	}
}

func assertBenchmarkJourneyPolicy(t *testing.T, content string, markers ...string) {
	t.Helper()
	for _, marker := range markers {
		if !strings.Contains(content, marker) {
			t.Errorf("workflow source missing benchmark journey marker %q", marker)
		}
	}
}
