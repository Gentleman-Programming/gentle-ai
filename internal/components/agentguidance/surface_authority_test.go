package agentguidance_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// harness-continuity S5: the worker never authorizes itself. A necessary route
// inside the objective and repository the human already authorized is
// parent-adjudicated after ownership, preservation, risk, and concurrency
// checks; a real new objective or scope, an unapproved repository, private
// material, a destructive or prohibited operation, or a consent, denial, or
// exhaustion boundary still reaches the human, and a declared surface never
// substitutes for repository authority. A continuation replaces the inherited
// section instead of adding to it, so every still-needed path has to travel in
// the new declaration. These are shipped-policy checks through the public
// render and the installed worker contract: they claim no model adherence and
// no filesystem confinement, because the existing admission already accepts a
// parent-supplied declaration and enforces no sandbox.
const (
	s5WorkerNeverWidens          = "never widens it on its own"
	s5WorkerWaitsForDeclaration  = "waits for the parent-issued declaration"
	s5ParentAddsNecessaryPath    = "Adjudicate a writer's edit-surface request yourself first: add a necessary path to its declaration without asking the human"
	s5SameAuthorizedRoute        = "objective already authorized in the same repository"
	s5OwnershipPreservationCheck = "you have checked ownership, preservation of existing work, risk, and concurrency with other writers and units"
	s5OptionalRefactorIsNotRoute = "An optional or cosmetic refactor is not that necessary path"
	s5OverlapBlocksTheAddition   = "unresolved overlap or ownership conflict blocks the addition"
	s5HumanOwnedScope            = "Only a real new objective or product scope, an unapproved repository, private material, a destructive or otherwise prohibited operation, missing consent, a denied action, or an exhausted limit goes to the human, under the existing protocols"

	// s5AdjudicationDirective opens the writer-request paragraph: the parent
	// adjudicates first, so no human question can precede that step.
	s5AdjudicationDirective = "Adjudicate a writer's edit-surface request yourself first"

	// The retired relay pointed back to the human approve/decline prompt
	// before the parent's own adjudication (verifier mwi607f7-18-0crn, P2).
	retiredRelaySameWay = "Relay a writer's interaction request about edit surfaces the same way"
	s5SurfaceIsNotAuthority      = "A surface entry never substitutes for repository or worktree authority and never authorizes an operation the rules prohibit"
	s5ContinuationIsReplacement  = "A continuation continues the same writer, and the section you pass replaces the inherited one rather than adding to it"
	s5StillNeededPathsTravel     = "carry every still-needed path, including the unchanged ones, and the concise reason for the addition"
	s5DroppingNeededPathDefect   = "Dropping a needed path is the same defect as no surface"
	s5NoRepoWideOrBudgetReset    = "never widens to a repository-wide surface or resets a budget"
	s5TechnicalUncertainty       = "Investigate a technical uncertainty about the route inside the authorized work; it is not a human question by itself"

	// The retired worker-authority clause made every surface change a human
	// instruction, which is the S5 defect: the parent may adjudicate a
	// necessary same-objective route itself.
	retiredHumanPathAuthority = "add or drop paths only on the human's explicit instruction"
)

// allowedEditSurfacesSection returns the Allowed edit surfaces body of a
// rendered orchestrator, up to the next heading of the same or a higher level.
// Scoping every S5 assertion to it keeps the test about this rule instead of
// about wording elsewhere in the prompt.
func allowedEditSurfacesSection(t *testing.T, rendered string) string {
	t.Helper()
	const heading = "### Allowed edit surfaces (MANDATORY)"
	start := strings.Index(rendered, heading)
	if start < 0 {
		return ""
	}
	rest := rendered[start+len(heading):]
	end := len(rest)
	for _, next := range []string{"\n### ", "\n## "} {
		if at := strings.Index(rest, next); at >= 0 && at < end {
			end = at
		}
	}
	return rest[:end]
}

// workerSection returns one "## " section body of the installed worker
// contract, up to the next "## " heading.
func workerSection(t *testing.T, body, heading string) string {
	t.Helper()
	start := strings.Index(body, heading)
	if start < 0 {
		return ""
	}
	rest := body[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// TestInstalledAllowedEditSurfacesLetTheParentAdjudicateSameObjectiveRoutes
// covers S5's positive and negative routes on every orchestrator that installs
// the shared section: a needed test or helper path inside the authorized
// objective is the parent's to approve after its checks, while an optional
// refactor, another repository, private material, a destructive or prohibited
// operation, a consent, denial, or exhaustion boundary, and an unresolved
// ownership conflict are not.
func TestInstalledAllowedEditSurfacesLetTheParentAdjudicateSameObjectiveRoutes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		required []string
	}{
		{
			name:     "S5 the worker never authorizes itself",
			required: []string{s5WorkerNeverWidens, s5WorkerWaitsForDeclaration},
		},
		{
			name: "S5 a necessary same-objective route with a needed test or helper file is parent-adjudicated",
			required: []string{
				s5ParentAddsNecessaryPath,
				s5SameAuthorizedRoute,
				s5OwnershipPreservationCheck,
			},
		},
		{
			name:     "S5 refactor-only scope is not that necessary route",
			required: []string{s5OptionalRefactorIsNotRoute},
		},
		{
			name:     "S5 an unresolved overlap or ownership conflict blocks the addition",
			required: []string{s5OverlapBlocksTheAddition},
		},
		{
			name: "S5 actual new scope, another repository, privacy, and prohibited operations stay with the human",
			required: []string{
				s5HumanOwnedScope,
				s5SurfaceIsNotAuthority,
			},
		},
		{
			name:     "S5 a surface never substitutes for repository or worktree authority",
			required: []string{s5SurfaceIsNotAuthority},
		},
		{
			name:     "S5 continuation replaces rather than unions and keeps still-needed paths",
			required: []string{s5ContinuationIsReplacement, s5StillNeededPathsTravel, s5DroppingNeededPathDefect},
		},
		{
			name:     "S5 an extension widens no repository-wide surface and resets no budget",
			required: []string{s5NoRepoWideOrBudgetReset},
		},
		{
			name:     "S5 technical uncertainty is investigated instead of becoming a human question",
			required: []string{s5TechnicalUncertainty},
		},
	}

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi || agent.ID == model.AgentConductor {
			// Pi's prompt is owned by Gentle Shell; Conductor is catalog-only.
			continue
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderOrchestratorWithSource(agent.ID, reviewassets.ReviewExecutionContractFor, "")
			if err != nil {
				t.Fatalf("RenderOrchestratorWithSource(%q) error = %v", agent.ID, err)
			}
			section := allowedEditSurfacesSection(t, rendered)
			if section == "" {
				t.Fatalf("RenderOrchestratorWithSource(%q) has no Allowed edit surfaces section", agent.ID)
			}
			for _, tt := range cases {
				for _, want := range tt.required {
					if !strings.Contains(section, want) {
						t.Errorf("%s: installed %q Allowed edit surfaces section is missing %q", tt.name, agent.ID, want)
					}
				}
			}
			for _, retired := range []string{retiredHumanPathAuthority, retiredRelaySameWay} {
				if strings.Contains(section, retired) {
					t.Errorf("installed %q Allowed edit surfaces section keeps retired %q", agent.ID, retired)
				}
			}
			// The writer-request paragraph opens with the parent's adjudication;
			// any route to the human comes only after it.
			paragraph := ""
			for _, p := range strings.Split(section, "\n\n") {
				if strings.Contains(p, "edit-surface request") {
					paragraph = strings.TrimSpace(p)
				}
			}
			switch {
			case paragraph == "":
				t.Errorf("installed %q Allowed edit surfaces section has no writer edit-surface request paragraph", agent.ID)
			case !strings.HasPrefix(paragraph, s5AdjudicationDirective):
				t.Errorf("installed %q writer-request paragraph does not open with the parent's adjudication: %q", agent.ID, paragraph)
			case strings.Index(paragraph, "human") < len(s5AdjudicationDirective):
				t.Errorf("installed %q writer-request paragraph routes to the human before adjudicating: %q", agent.ID, paragraph)
			}
		})
	}
}

// TestInstalledAllowedEditSurfacesKeepDerivationFirst is the PRESERVE check for
// the rules the extension leaves untouched: the parent derives exact, narrow,
// repository-relative surfaces before launch, lists untracked targets and the
// directories where new files are authorized, never asks the human to author
// paths, and rejects a surface wider than the task.
func TestInstalledAllowedEditSurfacesKeepDerivationFirst(t *testing.T) {
	t.Parallel()

	cases := []string{
		"A bounded writer refuses to write outside the exact allowed edit surfaces and stops for interaction when they are missing. The parent owns that input.",
		"Every new writer launch carries the section (a continuation inherits it); after a rejected launch, repair what the rejection names once, never relaunch the same prompt unchanged.",
		"If the surface genuinely cannot be derived, do not launch the writer, and do not ask the human to author paths.",
		"present that enumerated list as an approve/decline choice under the Lossless Blocking Prompts rules.",
		"exact repository-relative paths or narrow globs, one per line; never `.`, a bare repository root, or an absolute path",
		"pre-existing untracked targets the writer may write, listed explicitly",
		"the directories where new files are authorized, when the task requires new files",
		"nothing beyond the delegated task — a surface wider than the task is the same defect as no surface at all",
	}
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi || agent.ID == model.AgentConductor {
			continue
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderOrchestratorWithSource(agent.ID, reviewassets.ReviewExecutionContractFor, "")
			if err != nil {
				t.Fatalf("RenderOrchestratorWithSource(%q) error = %v", agent.ID, err)
			}
			section := allowedEditSurfacesSection(t, rendered)
			for _, want := range cases {
				if !strings.Contains(section, want) {
					t.Errorf("installed %q Allowed edit surfaces section lost %q", agent.ID, want)
				}
			}
		})
	}
}

// TestWorkerNeverAuthorizesItsOwnSurfaces covers S5 from the worker's side: the
// installed worker contract reports the exact candidate paths and reasons and
// waits for the parent's declaration instead of expanding itself, and its
// interaction contract asks the parent, which routes the request to the human
// only when the route is a real human choice.
func TestWorkerNeverAuthorizesItsOwnSurfaces(t *testing.T) {
	t.Parallel()

	worker := assets.MustRead("opencode/agents/gentle-ai-worker.md")
	context := workerSection(t, worker, "## Context contract")
	interaction := workerSection(t, worker, "## Interaction contract")
	rules := workerSection(t, worker, "## Implementation rules")
	if context == "" || interaction == "" || rules == "" {
		t.Fatal("installed worker contract lost a section this S5 rule lives in")
	}

	if !strings.Contains(interaction, "as a closed set the decision owner can approve, decline, or select from") {
		t.Error("worker Interaction contract still presupposes the human as the only decision owner")
	}
	if !strings.Contains(interaction, "never ask anyone to author paths, globs, identifiers, or commands as free text") {
		t.Error("worker Interaction contract no longer forbids free-text path authoring")
	}

	for _, want := range []string{
		"The parent supplies the edit surfaces under `## Allowed edit surfaces` in the parent task; treat that section as the authoritative list.",
		"Never widen it yourself",
		"reported to the parent with the exact candidate paths and reasons",
		"only a parent-issued replacement of that section authorizes it",
		"carrying every still-needed path because the replacement is not a union",
		"That declaration never substitutes for repository or worktree authority and never authorizes an operation the existing rules prohibit.",
		"a derived candidate set the parent can approve, narrow, or route to the human",
		// PRESERVE: the ambiguity stop itself stays a stop.
		"stop with `status: interaction_required`; do not guess",
	} {
		if !strings.Contains(context, want) {
			t.Errorf("worker Context contract is missing %q", want)
		}
	}
	if !strings.Contains(interaction, "ask the parent to approve that list, narrow it, or route it to the human") {
		t.Error("worker Interaction contract does not let the parent adjudicate the derived surface list")
	}
	if strings.Contains(worker, "ask the human to approve that list") || strings.Contains(worker, "the human can approve, decline, or select from") {
		t.Error("worker Interaction contract still sends every edit-surface request to the human")
	}
	for _, want := range []string{
		"the parent decides whether it is a technical extension inside the same authorized objective, which it may approve itself, or a real human choice",
		// PRESERVE: blocked stays reserved for non-human technical blockers.
		"Use `blocked` only for a non-human technical blocker",
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("worker Implementation rules are missing %q", want)
		}
	}
	// PRESERVE: the worker still derives its own candidate answer instead of
	// asking anyone to author paths or globs.
	if !strings.Contains(interaction, "never as an open question about which paths or globs to authorize") {
		t.Error("worker Interaction contract no longer forbids an open path/glob question")
	}
}
