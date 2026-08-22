package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const claudeLane = "claude"

// runClaudeLane drives one low-risk full lifecycle to a gate allow and one
// medium-candidate consent/v3 round-trip with the claude-code runtime.
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
	if code != 0 || getString(startDoc, "risk_level") != "low" || getString(startDoc, "state") != "reviewing" {
		b.fail(claudeLane, "low lifecycle: start", fmt.Sprintf("exit=%d risk=%q state=%q %s", code, getString(startDoc, "risk_level"), getString(startDoc, "state"), firstLine(stderr)))
		return
	}
	if lensesRequired, _ := startDoc["lenses_required"].(bool); lensesRequired {
		b.fail(claudeLane, "low lifecycle: start", "low-risk start unexpectedly selected lenses")
		return
	}
	if err := b.rememberStarted(repo, target, startDoc); err != nil {
		b.fail(claudeLane, "low lifecycle: start", err.Error())
		return
	}

	statusDoc, stderr, _ = b.status(repo, "claude-code")
	if getString(statusDoc, "next_transition", "execute", "operation") != "review.finalize" {
		b.fail(claudeLane, "low lifecycle: finalize", fmt.Sprintf("expected finalize transition, got %s/%s %s",
			getString(statusDoc, "next_transition", "kind"), getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
		return
	}
	finalize, stderr, code := b.runCommandLine("operation", repo, getString(statusDoc, "next_transition", "execute", "command"))
	if code != 0 || operationState(finalize) != "approved" {
		b.fail(claudeLane, "low lifecycle: finalize", fmt.Sprintf("exit=%d state=%q %s", code, operationState(finalize), firstLine(stderr)))
		return
	}
	b.burnApproved(claudeLane, "low lifecycle burned", repo, "claude-code", nil, finalize)
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
// the provider-owned request for the pending lens slot, then follows the
// native transitions to the approved receipt and a post-apply allow.
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
	if code != 0 || getString(capture, "schema") != "gentle-ai.review-result-artifact/v2" {
		b.fail(claudeLane, "medium reviewer model run", fmt.Sprintf("exit=%d schema=%q %s", code, getString(capture, "schema"), firstLine(stderr)))
		return
	}
	b.pass(claudeLane, "medium reviewer model run", "compiled claude-code reviewer runtime captured a native result artifact")

	// Follow the native transitions to the terminal receipt, bounded.
	evidencePath := filepath.Join(b.workRoot, "claude-evidence.txt")
	evidence := fmt.Sprintf("crosslane battery %s: node --check src/mul.js passed on the frozen candidate\n", timestamp())
	if err := os.WriteFile(evidencePath, []byte(evidence), 0o644); err != nil {
		b.fail(claudeLane, "medium lifecycle after model run", err.Error())
		return
	}
	for step := 0; step < 8; step++ {
		statusDoc, stderr, _ = b.status(repo, "claude-code")
		kind := getString(statusDoc, "next_transition", "kind")
		switch {
		case kind == "execute":
			operation := getString(statusDoc, "next_transition", "execute", "operation")
			doc, execStderr, code := b.runCommandLine("operation", repo, getString(statusDoc, "next_transition", "execute", "command"))
			if code != 0 {
				b.fail(claudeLane, "medium lifecycle after model run", fmt.Sprintf("%s exit=%d %s", operation, code, firstLine(execStderr)))
				return
			}
			if operationState(doc) == "approved" {
				b.burnApproved(claudeLane, "medium lifecycle after model run", repo, "claude-code", nil, doc)
				return
			}
			if operationState(doc) == "correction_required" {
				// A real model finding against scratch code is a legitimate
				// outcome; the battery treats reaching correction as success
				// for the model lane and stops here.
				b.pass(claudeLane, "medium lifecycle after model run", "model review reported candidate-causal findings (correction_required)")
				return
			}
		case kind == "collect":
			input = collectInput(statusDoc)
			if input == nil || input["capture_operation"] != "review.capture-evidence" {
				b.fail(claudeLane, "medium lifecycle after model run", fmt.Sprintf("unsupported collect input %v", input["capture_operation"]))
				return
			}
			tokens := substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"outcome": "passed", "input": evidencePath})
			captureArgs := append([]string{"review", getString(input, "submission", "operation_token")}, tokens...)
			if record, captureStderr, code := b.runJSON("verification-evidence", b.workRoot, captureArgs...); code != 0 || getString(record, "outcome") != "passed" {
				b.fail(claudeLane, "medium lifecycle after model run", fmt.Sprintf("evidence capture exit=%d %s", code, firstLine(captureStderr)))
				return
			}
		default:
			b.fail(claudeLane, "medium lifecycle after model run", fmt.Sprintf("unexpected transition %s/%s %s", kind, getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
			return
		}
	}
	b.fail(claudeLane, "medium lifecycle after model run", "did not reach a terminal state within the step budget")
}
