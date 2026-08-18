package intent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IntentResult represents the outcome of an intent routing operation.
type IntentResult struct {
	ChangeID      string
	WorkspacePath string
	ArtifactPath  string
	Phase         string
}

// Router interprets a raw request (e.g. from an issue or user prompt) and bootstraps
// the SDD lifecycle by creating the initial artifact (explore or proposal) for a change.
type Router struct {
	WorkspaceRoot string
}

// New creates a new Intent Router.
func New(workspaceRoot string) *Router {
	return &Router{
		WorkspaceRoot: workspaceRoot,
	}
}

// RouteIntent analyzes the input text, generates an ID (if not provided), and creates
// the first tracking artifact in openspec/changes/<changeID>/.
func (r *Router) RouteIntent(intentText string, sourceID string) (IntentResult, error) {
	// 1. Determine Change ID
	rawSource := strings.TrimSpace(sourceID)
	changeID := rawSource
	if changeID == "" {
		// A real implementation might hash the intentText or ask an LLM to generate a slug
		changeID = "feature-auto-generated"
	} else {
		// Normalize ID for filesystem, especially if it contains a timestamp (e.g., BS-42@2026-...)
		changeID = strings.Split(changeID, "@")[0] // Use just the prefix for the folder
		changeID = strings.ReplaceAll(changeID, " ", "-")
		changeID = strings.ToLower(changeID)
	}

	// 2. Determine initial phase/artifact
	// If it's a bug or complex feature -> explore.md
	// If it's simple -> proposal.md directly
	// We do a naive heuristic here for the sake of architecture fulfillment
	artifactName := "proposal.md"
	phase := "PROPOSE"
	if strings.Contains(strings.ToLower(intentText), "explore") || strings.Contains(strings.ToLower(intentText), "bug") {
		artifactName = "explore.md"
		phase = "DISCOVERY"
	}

	// 3. Create the directory and artifact
	changesDir := filepath.Join(r.WorkspaceRoot, "openspec", "changes", changeID)
	err := os.MkdirAll(changesDir, 0755)
	if err != nil {
		return IntentResult{}, fmt.Errorf("failed to create changes directory: %w", err)
	}

	// 2.5 Greenfield detection
	intentLower := strings.ToLower(intentText)
	isGreenfield := strings.Contains(intentLower, "greenfield") || strings.Contains(intentLower, "new project") || strings.Contains(intentLower, "nuevo proyecto")

	frontmatterType := ""
	if isGreenfield {
		frontmatterType = "type: greenfield\n"
		// If it's greenfield, we MUST start at Explore (or Blueprint, but SDD starts at explore to trigger solution-architect)
		artifactName = "explore.md"
		phase = "DISCOVERY"
	}

	artifactPath := filepath.Join(changesDir, artifactName)

	// Create YAML frontmatter
	// "originates-from" matches trace.Node's yaml tag exactly (internal/devorchestrator/trace/resolver.go),
	// so a freshly routed intent's provenance survives into GenerateContextForAgent's Trace Resolver step.
	frontmatter := fmt.Sprintf("---\nid: %s\n%soriginates-from:\n  - %s\n---\n", changeID, frontmatterType, rawSource)
	content := frontmatter + "# Intake Request\n\n" + intentText + "\n"

	err = os.WriteFile(artifactPath, []byte(content), 0644)
	if err != nil {
		return IntentResult{}, fmt.Errorf("failed to write initial artifact: %w", err)
	}

	return IntentResult{
		ChangeID:      changeID,
		WorkspacePath: changesDir,
		ArtifactPath:  filepath.Join("openspec", "changes", changeID, artifactName),
		Phase:         phase,
	}, nil
}
