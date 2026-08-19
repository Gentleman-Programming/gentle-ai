package intent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/changeowner"
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

	t.Run("New change is stamped engine: dev-orchestrator", func(t *testing.T) {
		intentText := "Add a new export button."
		sourceID := "stamped-change"

		res, err := router.RouteIntent(intentText, sourceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(filepath.Join(tempDir, res.ArtifactPath))
		if err != nil {
			t.Fatalf("failed to read created artifact: %v", err)
		}
		engine, found, err := changeowner.Parse(string(content))
		if err != nil {
			t.Fatalf("Parse(written artifact) error = %v", err)
		}
		if !found || engine != changeowner.EngineDev {
			t.Fatalf("expected engine: dev-orchestrator marker, found=%v engine=%q\n%s", found, engine, content)
		}
	})
}

// TestRouteIntentRefusesForeignOwnedChange covers SPEC-003: RouteIntent
// (dev-orchestrator's write path) must refuse to write into a change already
// owned by gentle-orchestrator, and must leave the existing artifact's bytes
// and mtime byte-for-byte unchanged -- no MkdirAll, no WriteFile on refusal.
func TestRouteIntentRefusesForeignOwnedChange(t *testing.T) {
	tempDir := t.TempDir()
	router := New(tempDir)

	changeDir := filepath.Join(tempDir, "openspec", "changes", "gentle-owned")
	if err := os.MkdirAll(changeDir, 0755); err != nil {
		t.Fatalf("failed to seed change dir: %v", err)
	}
	proposalPath := filepath.Join(changeDir, "proposal.md")
	original := "---\nid: gentle-owned\nengine: gentle-orchestrator\n---\n# Proposal\n"
	if err := os.WriteFile(proposalPath, []byte(original), 0644); err != nil {
		t.Fatalf("failed to seed proposal.md: %v", err)
	}
	// Force a distinguishable mtime so any rewrite is detectable even on
	// filesystems with coarse mtime resolution.
	pastMtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(proposalPath, pastMtime, pastMtime); err != nil {
		t.Fatalf("failed to set mtime: %v", err)
	}
	originalInfo, err := os.Stat(proposalPath)
	if err != nil {
		t.Fatalf("failed to stat seeded proposal.md: %v", err)
	}

	_, err = router.RouteIntent("Add a feature to the gentle-owned change.", "gentle-owned")
	if !errors.Is(err, changeowner.ErrForeignEngine) {
		t.Fatalf("RouteIntent() error = %v, want ErrForeignEngine", err)
	}

	rewritten, readErr := os.ReadFile(proposalPath)
	if readErr != nil {
		t.Fatalf("failed to re-read proposal.md after refused write: %v", readErr)
	}
	if string(rewritten) != original {
		t.Fatalf("proposal.md content changed after refused write:\nwant: %q\ngot:  %q", original, string(rewritten))
	}

	newInfo, statErr := os.Stat(proposalPath)
	if statErr != nil {
		t.Fatalf("failed to re-stat proposal.md: %v", statErr)
	}
	if !newInfo.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatalf("proposal.md mtime changed after refused write: want %v, got %v", originalInfo.ModTime(), newInfo.ModTime())
	}
}

// TestRouteIntentSameEngineProceeds covers SPEC-003's "Same-engine write
// proceeds normally" scenario: re-routing an already dev-orchestrator-owned
// change must succeed exactly as before this feature existed.
func TestRouteIntentSameEngineProceeds(t *testing.T) {
	tempDir := t.TempDir()
	router := New(tempDir)

	changeDir := filepath.Join(tempDir, "openspec", "changes", "dev-owned")
	if err := os.MkdirAll(changeDir, 0755); err != nil {
		t.Fatalf("failed to seed change dir: %v", err)
	}
	proposalPath := filepath.Join(changeDir, "proposal.md")
	if err := os.WriteFile(proposalPath, []byte("---\nid: dev-owned\nengine: dev-orchestrator\n---\n# Proposal\n"), 0644); err != nil {
		t.Fatalf("failed to seed proposal.md: %v", err)
	}

	// RouteIntent always writes artifactName == "proposal.md" for non-bug,
	// non-greenfield intent text, which overwrites the seeded file in place
	// -- exactly the pre-existing, expected re-route behavior.
	_, err := router.RouteIntent("Add another field to the dev-owned change.", "dev-owned")
	if err != nil {
		t.Fatalf("RouteIntent() error = %v, want nil for same-engine re-route", err)
	}
}
