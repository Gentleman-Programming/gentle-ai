package intent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/changeowner"
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

	// changeowner.AssertCanWrite runs BEFORE any filesystem mutation
	// (SPEC-003): a foreign-owned or unrecognized-marker changeRoot must be
	// refused with zero side effects -- no MkdirAll, no WriteFile. A
	// non-existent changeRoot (brand-new change) and a changeRoot already
	// owned by dev-orchestrator (same-engine re-route, SPEC-003's "Same-engine
	// write proceeds normally" scenario) both return nil here.
	if err := changeowner.AssertCanWrite(changesDir, changeowner.EngineDev); err != nil {
		return IntentResult{}, err
	}

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
	//
	// The `engine: dev-orchestrator` line is stamped via changeowner.Stamp
	// (not hand-formatted) so the ownership marker's exact grammar can never
	// drift from the one changeowner.Parse/Resolve read back later (SPEC-001).
	// Stamp is idempotent, so re-routing an already-stamped, same-engine
	// change (the AssertCanWrite same-engine pass-through above) does not
	// duplicate or corrupt the marker line.
	frontmatterBody := fmt.Sprintf("id: %s\n%soriginates-from:\n  - %s\n", changeID, frontmatterType, rawSource)
	frontmatterBody = changeowner.Stamp(frontmatterBody, changeowner.EngineDev)
	frontmatter := "---\n" + frontmatterBody + "---\n"
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
