package devorchestrator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/context"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/db"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/intent"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/router"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/skill"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/trace"
	"github.com/gentleman-programming/gentle-ai/v2/internal/repository"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// Orchestrator wraps the core services required to resolve delegation contexts.
type Orchestrator struct {
	WorkspaceRoot string
	IntentRouter  *intent.Router
	SkillResolver *skill.Resolver
	DBRouter      *db.Router
}

// New creates a new instance of the DevOrchestrator.
func New(workspaceRoot string) *Orchestrator {
	return &Orchestrator{
		WorkspaceRoot: workspaceRoot,
		IntentRouter:  intent.New(workspaceRoot),
		SkillResolver: skill.New(workspaceRoot),
		DBRouter:      db.New(),
	}
}

// RouteIntent acts as the front door for SDD. It accepts a raw intent and routes it
// to the appropriate starting phase, initializing the required planning artifacts.
func (o *Orchestrator) RouteIntent(intentText string, sourceID string) (intent.IntentResult, error) {
	return o.IntentRouter.RouteIntent(intentText, sourceID)
}

// ResolveSkills identifies and validates a list of requested skills against the local registry.
func (o *Orchestrator) ResolveSkills(requestedSkills []string) ([]string, error) {
	return o.SkillResolver.Resolve(requestedSkills)
}

// PrepareBatches takes a projected SDD status and splits the pending apply tasks
// into individual ExecutionBatch instances per repository.
func (o *Orchestrator) PrepareBatches(status sddstatus.StatusV1Projection, defaultAgent string) []batch.ExecutionBatch {
	return batch.GenerateExecutionBatches(status, defaultAgent)
}

// GenerateContextForAgent coordinates the Skills Resolver, Repository Resolver, and Trace Resolver
// to produce a complete Context Package for delegation.
func (o *Orchestrator) GenerateContextForAgent(
	executionID string,
	agentName string,
	primaryArtifact string, // e.g. "openspec/changes/multi-repo-status/proposal.md"
	repoNames []string,
	architectureID string,
	requiredSkills []string,
	expectedType string,
	expectedID string,
) (*context.Package, error) {

	// 1. Trace Resolver
	var traceNode trace.Node
	if primaryArtifact != "" {
		absPath := filepath.Join(o.WorkspaceRoot, primaryArtifact)
		if _, err := os.Stat(absPath); err == nil {
			node, err := trace.ParseTraceability(absPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse traceability: %w", err)
			}
			if node != nil {
				traceNode = *node
			}
		}
	}

	// 2. Repository Resolver
	// Ensure repositories are valid. The registry lives at docs/repository-registry.md
	// (see internal/repository/registry.go), keyed by repo-slug.
	regPath := filepath.Join(o.WorkspaceRoot, "docs", "repository-registry.md")
	registry, err := repository.ParseRegistry(regPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to parse repository registry: %w", err)
	}

	var validRepos []string
	if registry != nil {
		for _, r := range repoNames {
			if _, exists := registry[r]; exists {
				validRepos = append(validRepos, r)
			}
		}
	} else {
		// Fallback if no registry exists
		validRepos = repoNames
	}

	// 3. Load Repo Profiles (handled as internal skills)
	// The registry's own Profile column is authoritative for where a repo's
	// profile lives -- it does NOT always match "skills/repo-profiles/<repo-slug>/"
	// (e.g. gp-apps-cross-pagos's profile folder is "payments-api", not its slug).
	var combinedRepoProfile string
	for _, repo := range validRepos {
		entry, ok := registry[repo]
		if !ok || entry.Profile == "" || entry.Profile == "none" {
			continue
		}
		profilePath := filepath.Join(o.WorkspaceRoot, filepath.FromSlash(entry.Profile))
		data, err := os.ReadFile(profilePath)
		if err == nil {
			combinedRepoProfile += fmt.Sprintf("## Profile for %s\n%s\n\n", repo, string(data))
		}
	}

	// 3.5 Load Architecture Profile (Greenfield)
	var architectureProfile string
	if architectureID != "" {
		archPath := filepath.Join(o.WorkspaceRoot, "skills", "architecture", architectureID, "SKILL.md")
		data, err := os.ReadFile(archPath)
		if err == nil {
			architectureProfile = fmt.Sprintf("## Architecture Profile for %s\n%s\n\n", architectureID, string(data))
		}
	}

	// 3.7 Evaluate DB Impact and Resolve Skills
	if primaryArtifact != "" {
		absPath := filepath.Join(o.WorkspaceRoot, primaryArtifact)
		data, err := os.ReadFile(absPath)
		if err == nil {
			impact := o.DBRouter.EvaluateImpact(string(data))
			if impact == db.ImpactSimple && agentName == "backend-implementer" {
				requiredSkills = append(requiredSkills, "database-specialist")
			}
		}
	}

	resolvedSkills, err := o.ResolveSkills(requiredSkills)
	if err != nil {
		// Non-fatal, just log or ignore for now, we pass whatever we could resolve
		// In a real implementation we might fail hard
	}

	// 4. Context Builder
	req := context.BuildRequest{
		ExecutionID:         executionID,
		AgentName:           agentName,
		Trace:               traceNode,
		Repositories:        validRepos,
		ArchitectureID:      architectureID,
		Artifacts:           []string{primaryArtifact},
		Skills:              resolvedSkills,
		RepoProfile:         combinedRepoProfile,
		ArchitectureProfile: architectureProfile,
		ExpectedType:        expectedType,
		ExpectedID:          expectedID,
	}

	pkg := context.Build(req)

	// Adjust permissions based on agent type
	if isExecutionAgent(agentName) {
		pkg.Permissions.Code = "write"
		pkg.Permissions.Git = "write"
	}

	return &pkg, nil
}

func isExecutionAgent(agent string) bool {
	switch agent {
	case "backend-implementer", "frontend-implementer", "database-specialist":
		return true
	}
	return false
}

// GenerateAgentPrompt is a convenience wrapper that generates the context package
// and directly formats it into a prompt string for the agent.
func (o *Orchestrator) GenerateAgentPrompt(
	executionID string,
	agentName string,
	primaryArtifact string,
	repoNames []string,
	architectureID string,
	requiredSkills []string,
	expectedType string,
	expectedID string,
	baseInstruction string,
) (string, error) {
	pkg, err := o.GenerateContextForAgent(
		executionID, agentName, primaryArtifact, repoNames, architectureID, requiredSkills, expectedType, expectedID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate context package: %w", err)
	}

	return router.FormatPromptSignature(baseInstruction, pkg)
}
