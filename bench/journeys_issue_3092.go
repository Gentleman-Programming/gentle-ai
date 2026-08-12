package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const issue3092Lineage = "issue-3092-fork-pre-push"

var issue3092StartCapability = &Capability{
	Verb:  []string{"review", "start"},
	Flags: []string{"--cwd", "--lineage", "--base-ref", "--committed-only"},
}

func issue3092Journeys() []Journey {
	return []Journey{{
		ID:     "j103-pre-push-symbolic-fork-selector",
		Title:  "Fork pre-push validation preserves an upstream review base separate from the origin tracking branch",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3092",
		Steps: []Step{
			{Name: "fixture: fork remotes, tracking branch, and committed candidate", Fixture: issue3092ForkFixture},
			{Name: "start committed-only review against upstream/main", Requires: issue3092StartCapability,
				Args: productArgs("review", "start", "--lineage", issue3092Lineage, "--base-ref", "upstream/main", "--committed-only")},
			{Name: "finalize the approved candidate", Requires: finalizeCapability,
				Args: productArgs("review", "finalize", "--lineage", issue3092Lineage)},
			{Name: "fixture: upstream advances after approval", Fixture: issue3092AdvanceUpstream},
			{Name: "STATUS emits symbolic pre-push validation and preserves authority", Requires: statusCapability,
				Composite: issue3092ValidateSymbolicSelector},
			{Name: "invalid selectors remain denied without mutation", Requires: validateBaseRefCapability,
				Composite: issue3092RejectInvalidSelectors},
		},
	}}
}

func issue3092ForkFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	base, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	origin := filepath.Join(sandbox.Home, "origin.git")
	upstream := filepath.Join(sandbox.Home, "upstream.git")
	for _, remote := range []string{origin, upstream} {
		if err := sandbox.git(sandbox.Home, "init", "--bare", "-q", remote); err != nil {
			return err
		}
	}
	for name, remote := range map[string]string{"origin": origin, "upstream": upstream} {
		if err := sandbox.git(sandbox.Repo, "remote", "add", name, remote); err != nil {
			return err
		}
	}
	if err := sandbox.git(sandbox.Repo, "push", "-q", "upstream", "HEAD:refs/heads/main"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "--git-dir", upstream, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		return err
	}
	parent := filepath.Join(sandbox.Root, "upstream-work")
	if err := sandbox.git(sandbox.Root, "clone", "-q", upstream, parent); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "-b", "feature", "upstream/main"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "feature.md"), "# Feature\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "docs/feature.md"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "add feature"); err != nil {
		return err
	}
	featureCommit, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sandbox.Scratch["issue3092-feature-commit"] = featureCommit
	for _, branch := range []string{"main", "feature"} {
		if err := sandbox.git(sandbox.Repo, "push", "-q", "origin", base+":refs/heads/"+branch); err != nil {
			return err
		}
	}
	for _, setting := range [][2]string{{"branch.feature.remote", "origin"}, {"branch.feature.merge", "refs/heads/feature"}, {"branch.feature.pushRemote", "origin"}} {
		if err := sandbox.git(sandbox.Repo, "config", setting[0], setting[1]); err != nil {
			return err
		}
	}
	if err := sandbox.git(sandbox.Repo, "branch", "local-only", "HEAD"); err != nil {
		return err
	}
	sandbox.Scratch["issue3092-upstream-work"] = parent
	return nil
}

func issue3092AdvanceUpstream(sandbox *Sandbox) error {
	parent := sandbox.Scratch["issue3092-upstream-work"]
	for _, setting := range [][2]string{{"user.email", "bench@example.invalid"}, {"user.name", "Bench"}} {
		if err := sandbox.git(parent, "config", setting[0], setting[1]); err != nil {
			return err
		}
	}
	if err := sandbox.write(filepath.Join(parent, "docs", "upstream.md"), "# Upstream\n"); err != nil {
		return err
	}
	if err := sandbox.git(parent, "add", "docs/upstream.md"); err != nil {
		return err
	}
	if err := sandbox.git(parent, "commit", "-qm", "advance upstream base"); err != nil {
		return err
	}
	if err := sandbox.git(parent, "push", "-q", "origin", "HEAD:refs/heads/main"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "fetch", "-q", "upstream", "main"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "reset", "-q", "--hard", "upstream/main"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "merge", "--no-ff", "--no-edit", sandbox.Scratch["issue3092-feature-commit"]); err != nil {
		return err
	}
	raw, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "upstream/main")
	if err != nil {
		return err
	}
	sandbox.Scratch["issue3092-raw-base"] = raw
	return nil
}

func issue3092Status(r *journeyRun, selectors ...string) (statusEnvelope, string, error) {
	args := append([]string{"review", "status", "--contract", reviewContract, "--next-transition"}, selectors...)
	observation := r.run(productArgsFor(r, args...), false)
	if observation.ExitCode != 0 {
		return statusEnvelope{}, "", fmt.Errorf("STATUS exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	payload := strings.TrimSpace(observation.Stdout)
	var status statusEnvelope
	if err := json.Unmarshal([]byte(payload), &status); err != nil {
		return status, "", fmt.Errorf("parse STATUS transition: %w", err)
	}
	return status, payload, nil
}

func issue3092Candidate(sandbox *Sandbox) (string, error) {
	values := make([]string, 0, 3)
	for _, args := range [][]string{{"rev-parse", "HEAD"}, {"status", "--porcelain"}, {"hash-object", filepath.Join(sandbox.Repo, "docs", "feature.md")}} {
		value, err := gitOut(sandbox, sandbox.Repo, args...)
		if err != nil {
			return "", err
		}
		values = append(values, value)
	}
	return strings.Join(values, "\x00"), nil
}

func issue3092AssertUnchanged(r *journeyRun, authority, candidate string) error {
	_, afterAuthority, err := issue3092Status(r, "--lineage", issue3092Lineage, "--gate", "pre-push", "--base-ref", "upstream/main", "--committed-only")
	if err != nil {
		return err
	}
	afterCandidate, err := issue3092Candidate(r.sandbox)
	if err != nil {
		return err
	}
	if afterAuthority != authority || afterCandidate != candidate {
		return fmt.Errorf("pre-push validation mutated authority or candidate: authority=%+v/%+v candidate=%+v/%+v", authority, afterAuthority, candidate, afterCandidate)
	}
	return nil
}

func issue3092ValidateSymbolicSelector(r *journeyRun) error {
	selectors := []string{"--lineage", issue3092Lineage, "--gate", "pre-push", "--base-ref", "upstream/main", "--committed-only"}
	status, authority, err := issue3092Status(r, selectors...)
	if err != nil {
		return err
	}
	if status.Authority.LineageID != issue3092Lineage || status.Authority.State != "approved" ||
		status.NextTransition.Kind != "execute" || status.NextTransition.Execute.Operation != "review.validate" ||
		status.executeArgument("gate") != "pre-push" || status.executeArgument("base-ref") != "upstream/main" {
		return fmt.Errorf("symbolic pre-push STATUS = authority=%+v transition=%+v", authority, status.NextTransition)
	}
	candidate, err := issue3092Candidate(r.sandbox)
	if err != nil {
		return err
	}
	observation, err := runPrintedTransition(r, status)
	if err != nil {
		return err
	}
	if err := requireGateForLineage(observation, issue3092Lineage, true); err != nil {
		return err
	}
	return issue3092AssertUnchanged(r, authority, candidate)
}

func issue3092RejectInvalidSelectors(r *journeyRun) error {
	_, authority, err := issue3092Status(r, "--lineage", issue3092Lineage, "--gate", "pre-push", "--base-ref", "upstream/main", "--committed-only")
	if err != nil {
		return err
	}
	candidate, err := issue3092Candidate(r.sandbox)
	if err != nil {
		return err
	}
	for _, selector := range []string{r.sandbox.Scratch["issue3092-raw-base"], "main", "upstream/missing", "local-only"} {
		observation := r.run(productArgsFor(r, "review", "validate", "--lineage", issue3092Lineage, "--gate", "pre-push", "--base-ref", selector), false)
		var gate waveGateResult
		if observation.ExitCode == 0 || json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate) != nil || gate.Allowed || gate.Result == "allow" {
			return fmt.Errorf("invalid selector %q was not denied: exit=%d stdout=%s stderr=%s", selector, observation.ExitCode, observation.Stdout, observation.Stderr)
		}
		if err := issue3092AssertUnchanged(r, authority, candidate); err != nil {
			return fmt.Errorf("selector %q: %w", selector, err)
		}
	}
	return nil
}
