package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type issue2891Status struct {
	ApplyState      string   `json:"applyState"`
	NextRecommended string   `json:"nextRecommended"`
	BlockedReasons  []string `json:"blockedReasons"`
	ActionContext   struct {
		AllowedEditRoots []string `json:"allowedEditRoots"`
	} `json:"actionContext"`
	Consent *struct {
		Schema       string   `json:"schema"`
		MissingRoots []string `json:"missing_roots"`
		Choices      []struct {
			Answer     string `json:"answer"`
			Invocation string `json:"invocation"`
		} `json:"choices"`
	} `json:"consent"`
}

func issue2891SameParentRepository(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	parent := sandbox.Repo
	planning := filepath.Join(parent, "subproject")
	service := filepath.Join(parent, "service-dir")
	root := filepath.Join(planning, "openspec", "changes", "same-repo-rollout")
	for path, content := range map[string]string{
		filepath.Join(service, "README.md"):            "# service\n",
		filepath.Join(root, "proposal.md"):             "# Proposal\n",
		filepath.Join(root, "specs", "api", "spec.md"): "### Requirement: API\n#### Scenario: Header accepted\n",
		filepath.Join(root, "design.md"):               "# Design\n",
		filepath.Join(root, "tasks.md"):                "- [ ] 1.1 Update `../service-dir/internal/api/handler.go` to accept the new header\n",
	} {
		if err := sandbox.write(path, content); err != nil {
			return err
		}
	}
	if err := sandbox.git(parent, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(parent, "commit", "-qm", "same-parent SDD fixture"); err != nil {
		return err
	}
	actualRoot, err := gitOut(sandbox, planning, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if actualRoot != parent {
		return fmt.Errorf("fixture planning workspace resolves to Git root %q, want parent %q", actualRoot, parent)
	}
	sandbox.Scratch["issue-2891-service"] = service
	sandbox.Repo = planning
	return nil
}
func issue2891SameParentStatus(sandbox *Sandbox, observation Observation) error {
	var status issue2891Status
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return fmt.Errorf("parse same-parent sdd-status: %w", err)
	}
	wantService, err := filepath.EvalSymlinks(sandbox.Scratch["issue-2891-service"])
	if err != nil {
		return err
	}
	if status.ApplyState != "blocked" || status.NextRecommended == "apply" {
		return fmt.Errorf("applyState=%q nextRecommended=%q, want blocked and not apply", status.ApplyState, status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "blocked(edit_authority_missing)") {
		return fmt.Errorf("blocked reasons lack edit_authority_missing: %v", status.BlockedReasons)
	}
	if len(status.ActionContext.AllowedEditRoots) != 1 || status.ActionContext.AllowedEditRoots[0] != sandbox.Repo {
		return fmt.Errorf("allowedEditRoots=%v, want only nested planning workspace %s", status.ActionContext.AllowedEditRoots, sandbox.Repo)
	}
	if status.Consent == nil || status.Consent.Schema != "gentle-ai.sdd-integration.consent/v1" ||
		len(status.Consent.MissingRoots) != 1 || status.Consent.MissingRoots[0] != wantService {
		return fmt.Errorf("consent missing_roots=%v, want [%s]", status.Consent, wantService)
	}
	grantInvocation := ""
	for _, choice := range status.Consent.Choices {
		if choice.Answer == "granted" {
			grantInvocation = choice.Invocation
			break
		}
	}
	if grantInvocation == "" {
		return fmt.Errorf("consent choices lack a granted invocation: %v", status.Consent.Choices)
	}
	sandbox.Scratch["issue-2891-grant-invocation"] = grantInvocation
	return nil
}
func issue2891GrantArgs(sandbox *Sandbox) ([]string, error) {
	return printedCommandArguments(sandbox.Scratch["issue-2891-grant-invocation"])
}
func issue2891SameParentGrantedStatus(sandbox *Sandbox, observation Observation) error {
	var status issue2891Status
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return fmt.Errorf("parse granted same-parent sdd-status: %w", err)
	}
	wantService := sandbox.Scratch["issue-2891-service"]
	roots := status.ActionContext.AllowedEditRoots
	if status.ApplyState != "ready" || status.NextRecommended != "apply" || status.Consent != nil ||
		len(roots) != 2 || roots[0] != sandbox.Repo || roots[1] != wantService {
		return fmt.Errorf("post-grant status=%q/%q roots=%v reasons=%v, want ready/apply with %v", status.ApplyState, status.NextRecommended, roots, status.BlockedReasons, []string{sandbox.Repo, wantService})
	}
	return nil
}
func issue2891Journeys() []Journey {
	return []Journey{{
		ID:     "j96-sdd-same-parent-repository-edit-authority",
		Review: reviewOptedIn,
		Title:  "Nested planning workspace blocks a sibling directory in the same Git repository",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2891",
		Steps: []Step{
			{Name: "fixture: nested planning workspace and sibling service share one Git root", Fixture: issue2891SameParentRepository},
			{Name: "sdd-status blocks the unauthorized same-parent target", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", "same-repo-rollout", "--json"), After: issue2891SameParentStatus},
			{Name: "sdd-attempt grant executes the emitted consent invocation", Args: issue2891GrantArgs},
			{Name: "sdd-status re-entry projects the granted sibling edit root", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", "same-repo-rollout", "--json"), After: issue2891SameParentGrantedStatus},
		},
	}}
}
