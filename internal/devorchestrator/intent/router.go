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

// RouteRequest carries an explicit intent to classify. Phase, when
// non-empty, is a structured field supplied by the caller (H-06) and is the
// PRIMARY classification signal -- it takes precedence over intentText's
// substring content entirely. Phase is case-insensitive and recognizes
// "DISCOVERY" and "PROPOSE"; any other value (including empty) falls back to
// the heuristic below.
type RouteRequest struct {
	IntentText string
	SourceID   string
	Phase      string
}

// classifyIntent decides the artifact filename and phase for a routing
// request (H-06). A caller-supplied req.Phase is the primary signal; when
// absent or unrecognized, this falls back to the pre-existing naive
// substring heuristic below, unchanged and still documented as naive -- it
// is known to misclassify inputs like "a proposal to explore options", which
// is exactly why the structured field exists as the preferred signal.
func classifyIntent(req RouteRequest) (artifactName string, phase string) {
	switch strings.ToUpper(strings.TrimSpace(req.Phase)) {
	case "DISCOVERY":
		return "explore.md", "DISCOVERY"
	case "PROPOSE":
		return "proposal.md", "PROPOSE"
	}

	// Fallback: naive heuristic, kept for the sake of architecture
	// fulfillment when no structured field is supplied.
	if strings.Contains(strings.ToLower(req.IntentText), "explore") || strings.Contains(strings.ToLower(req.IntentText), "bug") {
		return "explore.md", "DISCOVERY"
	}
	return "proposal.md", "PROPOSE"
}

// NormalizeChangeID derives the on-disk change identifier from a raw source
// identifier, exactly as RouteIntent applies it internally. Exported so
// callers (e.g. the CLI's Engram-mode write refusal check) can determine
// which change a routing request targets before RouteIntent runs.
func NormalizeChangeID(sourceID string) string {
	rawSource := strings.TrimSpace(sourceID)
	if rawSource == "" {
		return "feature-auto-generated"
	}
	changeID := strings.Split(rawSource, "@")[0] // Use just the prefix for the folder
	changeID = strings.ReplaceAll(changeID, " ", "-")
	return strings.ToLower(changeID)
}

// RouteIntent analyzes the input text, generates an ID (if not provided), and creates
// the first tracking artifact in openspec/changes/<changeID>/. It is a thin
// wrapper over RouteIntentRequest with no structured Phase field, so its
// classification always falls back to the substring heuristic (H-06):
// existing callers keep their exact prior behavior.
func (r *Router) RouteIntent(intentText string, sourceID string) (IntentResult, error) {
	return r.RouteIntentRequest(RouteRequest{IntentText: intentText, SourceID: sourceID})
}

// RouteIntentRequest is RouteIntent's structured-input entry point (H-06):
// req.Phase, when set, is the primary classification signal and takes
// precedence over req.IntentText's substring content.
func (r *Router) RouteIntentRequest(req RouteRequest) (IntentResult, error) {
	// 1. Determine Change ID
	rawSource := strings.TrimSpace(req.SourceID)
	changeID := NormalizeChangeID(req.SourceID)

	// T1: reject a change ID that would escape openspec/changes/<id> before
	// any AssertCanWrite/MkdirAll/WriteFile call. Containment precedes
	// ownership in the refusal precedence ordering (spec, Refusal Precedence
	// Ordering requirement).
	if err := ValidateIdentifier(changeID); err != nil {
		return IntentResult{}, err
	}

	// 2. Determine initial phase/artifact (H-06: structured field first,
	// naive substring heuristic as documented fallback).
	artifactName, phase := classifyIntent(req)

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
	intentLower := strings.ToLower(req.IntentText)
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
	content := frontmatter + "# Intake Request\n\n" + req.IntentText + "\n"

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
