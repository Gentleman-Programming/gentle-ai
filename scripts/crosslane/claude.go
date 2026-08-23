package main

import "fmt"

const claudeLane = "claude"

// runClaudeLane drives one low-risk terminal capture and one medium-candidate
// consent/v3 round-trip with the claude-code runtime.
// With --with-model it additionally runs the real compiled claude-code
// reviewer runtime over the medium lineage (dev subscription).
func (b *battery) runClaudeLane() {
	b.runClaudeLowLifecycle()
	b.runClaudeMediumConsent()
}

func (b *battery) runClaudeLowLifecycle() {
	repo, err := b.scratchRepo("claude-low")
	if err != nil {
		b.fail(claudeLane, "low lifecycle scratch repository", err.Error())
		return
	}
	err = writeFile(repo, "docs/ordinary-guide.md", "# Ordinary guide\n\nline one\n")
	if err == nil {
		err = commitAll(repo, "docs: guide")
	}
	if err != nil {
		b.fail(claudeLane, "low lifecycle scratch repository", err.Error())
		return
	}
	if err := writeFile(repo, "docs/ordinary-guide.md", "# Ordinary guide\n\nline one\nline two, purely passive documentation\n"); err != nil {
		b.fail(claudeLane, "low lifecycle scratch repository", err.Error())
		return
	}

	statusDoc, stderr, code := b.status(repo, "claude-code")
	target := getString(statusDoc, "target_identity")
	if target == "" || getString(statusDoc, "next_transition", "execute", "operation") != "review.start" {
		b.fail(claudeLane, "low lifecycle: negotiated start", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
		return
	}
	startDoc, stderr, code := b.runCommandLine("start", repo, getString(statusDoc, "next_transition", "execute", "command"))
	if code != 0 || getString(startDoc, "risk_level") != "low" || getString(startDoc, "state") != "approved" {
		b.fail(claudeLane, "low lifecycle: start", fmt.Sprintf("exit=%d risk=%q state=%q %s", code, getString(startDoc, "risk_level"), getString(startDoc, "state"), firstLine(stderr)))
		return
	}
	if lensesRequired, _ := startDoc["lenses_required"].(bool); lensesRequired {
		b.fail(claudeLane, "low lifecycle: start", "low-risk start unexpectedly selected lenses")
		return
	}
	if getString(startDoc, "action") != "closed" {
		b.fail(claudeLane, "low lifecycle: start", fmt.Sprintf("action = %q, want closed", getString(startDoc, "action")))
		return
	}
	b.pass(claudeLane, "low lifecycle burned", "zero-lens START closed and burned its review without FINALIZE")
}

func (b *battery) runClaudeMediumConsent() {
	repo, err := b.scratchRepo("claude-medium")
	if err != nil {
		b.fail(claudeLane, "medium consent scratch repository", err.Error())
		return
	}
	base := "export function mul(a, b) {\n  return a * b;\n}\n"
	err = writeFile(repo, "src/mul.js", base)
	if err == nil {
		err = commitAll(repo, "feat: mul")
	}
	if err != nil {
		b.fail(claudeLane, "medium consent scratch repository", err.Error())
		return
	}
	if err := writeFile(repo, "src/mul.js", base+"export function twice(a) {\n  return a + a;\n}\n"); err != nil {
		b.fail(claudeLane, "medium consent scratch repository", err.Error())
		return
	}

	statusDoc, stderr, code := b.status(repo, "claude-code")
	target := getString(statusDoc, "target_identity")
	if target == "" {
		b.fail(claudeLane, "medium consent: negotiated status", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
		return
	}
	consent, stderr, _ := b.runJSON("consent", repo,
		"review", "start", "--contract", reviewContract, "--cwd", repo,
		"--target", target, "--projection", "workspace", "--agent", "claude-code", "--consent", "relay")
	if getString(consent, "schema") != "gentle-ai.review-integration.consent/v3" || getString(consent, "action") != "consent_required" {
		b.fail(claudeLane, "medium consent: envelope surfaced", fmt.Sprintf("schema=%q action=%q %s", getString(consent, "schema"), getString(consent, "action"), firstLine(stderr)))
		return
	}
	granted := grantedInvocation(consent)
	if granted == "" {
		b.fail(claudeLane, "medium consent: envelope surfaced", "no granted choice invocation in envelope")
		return
	}
	startDoc, stderr, code := b.runCommandLine("start", repo, granted)
	lenses := getSlice(startDoc, "selected_lenses")
	if code != 0 || getString(startDoc, "state") != "reviewing" || getString(startDoc, "risk_level") != "medium" || len(lenses) != 1 {
		b.fail(claudeLane, "medium consent: granted round-trip", fmt.Sprintf("exit=%d state=%q risk=%q lenses=%d %s",
			code, getString(startDoc, "state"), getString(startDoc, "risk_level"), len(lenses), firstLine(stderr)))
		return
	}
	if err := b.rememberStarted(repo, target, startDoc); err != nil {
		b.fail(claudeLane, "medium consent: granted round-trip", err.Error())
		return
	}
	b.pass(claudeLane, "medium consent: granted round-trip", "consent/v3 surfaced; granted invocation created a reviewing medium lineage with one lens")

	if !b.withModel {
		b.skip(claudeLane, "medium reviewer model run", "pass --with-model to run the real claude-code reviewer (dev subscription)")
		return
	}
	b.runClaudeModelReview(repo)
}

// runClaudeModelReview lets the compiled claude-code reviewer runtime execute
// the provider-owned request for the pending lens slot; its final capture
// closes the lifecycle without a receipt or delivery gate.
func (b *battery) runClaudeModelReview(repo string) {
	statusDoc, stderr, _ := b.status(repo, "claude-code")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(claudeLane, "medium reviewer model run", fmt.Sprintf("no capture-result collect input; %s", firstLine(stderr)))
		return
	}
	args := argumentValues(input)
	capture, stderr, code := b.runJSON("result-artifact", repo,
		"review", "capture-result",
		"--lineage", args["lineage"],
		"--expected-revision", args["expected-revision"],
		"--target", args["target"],
		"--repository-context", args["repository-context"],
		"--lens", args["lens"],
		"--order", args["order"],
		"--subject-hash", args["subject-hash"],
		"--agent", "claude-code")
	if code != 0 || !admittedCapture(capture) {
		b.fail(claudeLane, "medium reviewer model run", fmt.Sprintf("exit=%d schema=%q state=%q %s", code, getString(capture, "schema"), operationState(capture), firstLine(stderr)))
		return
	}
	b.pass(claudeLane, "medium reviewer model run", "compiled claude-code reviewer runtime captured a native result")
	switch operationState(capture) {
	case "approved":
		b.burnApproved(claudeLane, "medium lifecycle after model run", repo, "claude-code", nil, capture)
	case "correction_required":
		// A real model finding against scratch code is a legitimate outcome;
		// transport and terminal capture are proven without a synthetic repair.
		b.pass(claudeLane, "medium lifecycle after model run", "model review reported candidate-causal findings (correction_required)")
	default:
		b.fail(claudeLane, "medium lifecycle after model run", fmt.Sprintf("capture remained nonterminal at state %q", operationState(capture)))
	}
}
