package devorchestrator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/context"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/router"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/trace"
	"github.com/gentleman-programming/gentle-ai/v2/internal/repository"
)

// Orchestrator wraps the core services required to resolve delegation contexts.
type Orchestrator struct {
	WorkspaceRoot string
}

// New creates a new instance of the DevOrchestrator.
func New(workspaceRoot string) *Orchestrator {
	return &Orchestrator{
		WorkspaceRoot: workspaceRoot,
	}
}

// GenerateContextForAgent coordinates the Skills Resolver, Repository Resolver, and Trace Resolver
// to produce a complete Context Package for delegation.
func (o *Orchestrator) GenerateContextForAgent(
	executionID string,
	agentName string,
	primaryArtifact string, // e.g. "openspec/changes/multi-repo-status/proposal.md"
	repoNames []string,
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
	// Ensure repositories are valid
	regPath := filepath.Join(o.WorkspaceRoot, "repository-registry.md")
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
	var combinedRepoProfile string
	for _, repo := range validRepos {
		// We expect internal repository profiles to be in `skills/repo-profiles/<repo-slug>/SKILL.md`
		profilePath := filepath.Join(o.WorkspaceRoot, "skills", "repo-profiles", repo, "SKILL.md")
		data, err := os.ReadFile(profilePath)
		if err == nil {
			combinedRepoProfile += fmt.Sprintf("## Profile for %s\n%s\n\n", repo, string(data))
		}
	}

	// 4. Context Builder
	req := context.BuildRequest{
		ExecutionID:  executionID,
		AgentName:    agentName,
		Trace:        traceNode,
		Repositories: validRepos,
		Artifacts:    []string{primaryArtifact},
		Skills:       requiredSkills,
		RepoProfile:  combinedRepoProfile,
		ExpectedType: expectedType,
		ExpectedID:   expectedID,
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
	requiredSkills []string,
	expectedType string,
	expectedID string,
	baseInstruction string,
) (string, error) {
	pkg, err := o.GenerateContextForAgent(
		executionID, agentName, primaryArtifact, repoNames, requiredSkills, expectedType, expectedID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate context package: %w", err)
	}

	return router.FormatPromptSignature(baseInstruction, pkg)
}
