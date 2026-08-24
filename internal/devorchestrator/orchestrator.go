package devorchestrator

import (
	stdctx "context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/changeowner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/agent"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/context"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/db"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/executor"
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
	// TraceManager validates phase-transition traceability (Implements /
	// OriginatesFrom) between a source artifact and the artifact being
	// generated for. See GenerateContextForAgent's sourceArtifact parameter.
	TraceManager trace.Manager
	// agentRegistry is loaded once from the canonical claude/agents/*.md files
	// embedded in internal/assets.FS. It is the single source of truth for
	// agent permissions — no static map to drift.
	agentRegistry map[string]agent.Contract
}

// New creates a new instance of the DevOrchestrator.
// It eagerly loads the agent registry from the embedded canonical agent
// definitions so that GenerateContextForAgent never touches the filesystem
// for permission checks.
func New(workspaceRoot string) *Orchestrator {
	registry, err := agent.LoadRegistryFromFS(assets.FS, "claude/agents")
	if err != nil {
		// Fallback to empty registry — orchestrator will reject unknown agents
		// gracefully at call time.
		registry = map[string]agent.Contract{}
	}
	return &Orchestrator{
		WorkspaceRoot: workspaceRoot,
		IntentRouter:  intent.New(workspaceRoot),
		SkillResolver: skill.New(workspaceRoot),
		DBRouter:      db.New(),
		TraceManager:  trace.NewManager(),
		agentRegistry: registry,
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

// ExecuteBatches distributes the given execution batches to a ConcurrentEngine.
func (o *Orchestrator) ExecuteBatches(ctx stdctx.Context, batches []batch.ExecutionBatch, prompts map[string]string, runner executor.AgentRunner, maxWorkers int) map[string]error {
	engine := executor.New(runner, maxWorkers)
	return engine.ExecuteBatches(ctx, batches, prompts)
}

// GenerateContextForAgent coordinates the Skills Resolver, Repository Resolver, and Trace Resolver
// to produce a complete Context Package for delegation.
//
// sourceArtifact, when non-empty, is the workspace-relative path to the
// artifact this phase transition originates from (e.g. the proposal.md a
// spec.md must implement or originate from). When set, the Traceability
// Manager (per the documented Notion architecture flow, "Arquitectura de
// nuestros agentes") validates that primaryArtifact declares
// Implements/OriginatesFrom the source artifact's ID before the context
// package is built, failing the same way the strict-enforcement checks
// below it do. An empty sourceArtifact skips the check entirely — the first
// artifact of a change has no predecessor to trace to.
func (o *Orchestrator) GenerateContextForAgent(
	executionID string,
	agentName string,
	primaryArtifact string, // e.g. "openspec/changes/multi-repo-status/proposal.md"
	repoNames []string,
	architectureID string,
	requiredSkills []string,
	expectedType string,
	expectedID string,
	sourceArtifact string,
) (*context.Package, error) {

	// 0. T1 path containment. Must run before any filepath.Join/os.Stat on a
	// caller-supplied path and before ownership (refusal precedence #1: this
	// is what makes the reachable route/context/dispatch CLI verbs (H-01)
	// safe against a traversal escaping the intended change tree).
	if primaryArtifact != "" {
		if err := intent.ValidateContainedPath(o.WorkspaceRoot, primaryArtifact); err != nil {
			return nil, err
		}
	}
	if sourceArtifact != "" {
		if err := intent.ValidateContainedPath(o.WorkspaceRoot, sourceArtifact); err != nil {
			return nil, err
		}
	}

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

	// 1.5 Traceability Manager (phase-transition validation)
	if sourceArtifact != "" && primaryArtifact != "" {
		absSource := filepath.Join(o.WorkspaceRoot, sourceArtifact)
		absDest := filepath.Join(o.WorkspaceRoot, primaryArtifact)
		if err := o.TraceManager.ValidatePhaseTransition(absSource, absDest); err != nil {
			return nil, fmt.Errorf("strict enforcement: %w", err)
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

	// 3.7 Evaluate DB Impact and Resolve Skills. dbImpactSkills below (design
	// decision D6) replaces the single hardcoded
	// `impact == ImpactSimple && agentName == "backend-implementer"` check
	// with a declarative (impact, agentName) matrix, so db.ImpactHighRisk is
	// consumed distinctly instead of being silently ignored, and
	// frontend-implementer gets a schema-impact-aware branch when impact is
	// high risk.
	var dbImpact db.Impact
	if primaryArtifact != "" {
		absPath := filepath.Join(o.WorkspaceRoot, primaryArtifact)
		data, err := os.ReadFile(absPath)
		if err == nil {
			dbImpact = o.DBRouter.EvaluateImpact(string(data))
			requiredSkills = append(requiredSkills, dbImpactSkills(dbImpact, agentName)...)
		}
	}

	resolvedSkills, err := o.ResolveSkills(requiredSkills)
	if err != nil {
		return nil, fmt.Errorf("resolve skills: %w", err)
	}

	// 3.9 Validate Agent Contract — derived from canonical claude/agents/*.md
	contract, exists := o.agentRegistry[agentName]
	if !exists {
		return nil, fmt.Errorf("strict enforcement: agent '%s' is not registered in the agent contract registry (derived from internal/assets/claude/agents/)", agentName)
	}

	// 3.95 Validate Change Ownership (SPEC-007, defense in depth). This rides
	// the exact chokepoint and error style the be3909ab strict-registry check
	// above already established: every agent invocation passes through here,
	// so the ownership check runs on every phase advance, not only at
	// RouteIntent creation time.
	//
	// changeowner.ResolveMarked (not Resolve) is used deliberately: an
	// unmarked change proceeds unchanged (marked == false), matching the
	// design's Data Flow/Testing Strategy ("own/unmarked-default" proceeds)
	// and the pre-existing TestGenerateContextForAgent fixture, which writes
	// an unmarked proposal.md and expects success. Only an explicit,
	// recognized marker naming a different engine is refused -- RouteIntent
	// always stamps its own changes, so a legitimately dev-owned change is
	// always marked by the time it reaches this check.
	if primaryArtifact != "" {
		changeRoot := filepath.Dir(filepath.Join(o.WorkspaceRoot, primaryArtifact))
		owner, marked, err := changeowner.ResolveMarked(changeRoot)
		if err != nil {
			return nil, fmt.Errorf("strict enforcement: %w", err)
		}
		if marked && owner != changeowner.EngineDev {
			return nil, fmt.Errorf("strict enforcement: %s", changeowner.RefusalMessage(filepath.Base(changeRoot), owner, changeowner.EngineDev))
		}
	}

	// In a real implementation, we would filter `Artifacts` based on `contract.Inputs.AllowedArtifactTypes`
	// For now, we ensure the orchestrator assigns the precise permissions mandated by the contract.

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
		DBImpact:            string(dbImpact),
	}

	pkg := context.Build(req)

	// Apply permissions from the strict contract
	pkg.Permissions.Code = contract.Permissions.Code
	pkg.Permissions.Git = contract.Permissions.Git

	return &pkg, nil
}

// dbImpactSkills is the declarative (impact, agentName) matrix from design
// decision D6, corrected after review: it replaces the single hardcoded
// `impact == ImpactSimple && agentName == "backend-implementer"` check that
// used to leave db.ImpactHighRisk entirely unhandled -- the highest-risk DB
// changes were exactly the ones that never got the specialist.
//
// database-specialist is injected only for db.ImpactHighRisk, not
// db.ImpactSimple: skills/agents/database-specialist/SKILL.md describes
// itself as handling "complex database migrations, schema changes, and
// high-risk DB tasks", not simple ones. The original single-`if` check had
// it backwards -- wired to the low-risk case while the genuinely high-risk
// path got nothing. This also satisfies spec H-05's first scenario: the
// resolved skill set for backend-implementer now differs measurably between
// db.ImpactSimple (nil) and db.ImpactHighRisk ({database-specialist}).
//
// frontend-implementer's schema-impact-aware branch (spec H-05's second
// scenario) resolves to database-specialist as well -- a real skill, not an
// invented "frontend-schema-impact" name with no corresponding
// skills/**/SKILL.md content anywhere in the workspace. The
// schema-impact-specific signal itself is the rendered `db_impact: <value>`
// field in the prompt (see context.Package.DBImpact / promptTemplate),
// matching design decision D6's own rationale for that field.
//
// It returns the additional required skills to inject, or nil when nothing
// applies. TestDBImpactSkillsResolveOnDisk guards every skill name this
// function can ever emit against the real workspace skills/ directory, so a
// future dangling reference fails the build instead of surfacing as a
// runtime hard-fail after CLI wiring.
func dbImpactSkills(impact db.Impact, agentName string) []string {
	if impact != db.ImpactHighRisk {
		return nil
	}
	switch agentName {
	case "backend-implementer", "frontend-implementer":
		return []string{"database-specialist"}
	}
	return nil
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
	sourceArtifact string,
	baseInstruction string,
) (string, error) {
	pkg, err := o.GenerateContextForAgent(
		executionID, agentName, primaryArtifact, repoNames, architectureID, requiredSkills, expectedType, expectedID, sourceArtifact,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate context package: %w", err)
	}

	return router.FormatPromptSignature(baseInstruction, pkg)
}
