package intent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteIntent(t *testing.T) {
	tempDir := t.TempDir()
	router := New(tempDir)

	t.Run("Bug routes to explore", func(t *testing.T) {
		intentText := "Fix the critical bug in the login screen."
		sourceID := "issue-456"

		res, err := router.RouteIntent(intentText, sourceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.ChangeID != "issue-456" {
			t.Errorf("expected issue-456, got %s", res.ChangeID)
		}
		if res.Phase != "DISCOVERY" {
			t.Errorf("expected DISCOVERY phase, got %s", res.Phase)
		}
		if !strings.HasSuffix(res.ArtifactPath, "explore.md") {
			t.Errorf("expected explore.md, got %s", res.ArtifactPath)
		}

		// Verify file exists and has content
		content, err := os.ReadFile(filepath.Join(tempDir, res.ArtifactPath))
		if err != nil {
			t.Fatalf("failed to read created artifact: %v", err)
		}
		if !strings.Contains(string(content), "id: issue-456") {
			t.Errorf("expected frontmatter ID, got \n%s", content)
		}
		if !strings.Contains(string(content), "Fix the critical bug") {
			t.Errorf("expected intent text in body, got \n%s", content)
		}
	})

	t.Run("Greenfield feature routes to explore and writes the reported path", func(t *testing.T) {
		intentText := "Set up a new project for the payments dashboard."
		sourceID := "greenfield-payments"

		res, err := router.RouteIntent(intentText, sourceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Phase != "DISCOVERY" {
			t.Errorf("expected DISCOVERY phase, got %s", res.Phase)
		}
		if !strings.HasSuffix(res.ArtifactPath, "explore.md") {
			t.Errorf("expected explore.md, got %s", res.ArtifactPath)
		}

		// The file must actually exist at the reported ArtifactPath, not at
		// whatever path was computed before the greenfield reassignment.
		content, err := os.ReadFile(filepath.Join(tempDir, res.ArtifactPath))
		if err != nil {
			t.Fatalf("expected artifact at reported ArtifactPath %s: %v", res.ArtifactPath, err)
		}
		if !strings.Contains(string(content), "type: greenfield") {
			t.Errorf("expected greenfield frontmatter, got \n%s", content)
		}

		if _, err := os.Stat(filepath.Join(tempDir, "openspec", "changes", res.ChangeID, "proposal.md")); !os.IsNotExist(err) {
			t.Errorf("greenfield intent must not also write proposal.md, stat err = %v", err)
		}
	})

	t.Run("Standard feature routes to proposal", func(t *testing.T) {
		intentText := "Add a new button to the dashboard."
		sourceID := "feature 123" // Should be normalized

		res, err := router.RouteIntent(intentText, sourceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.ChangeID != "feature-123" {
			t.Errorf("expected feature-123, got %s", res.ChangeID)
		}
		if res.Phase != "PROPOSE" {
			t.Errorf("expected PROPOSE phase, got %s", res.Phase)
		}
		if !strings.HasSuffix(res.ArtifactPath, "proposal.md") {
			t.Errorf("expected proposal.md, got %s", res.ArtifactPath)
		}
	})
}
