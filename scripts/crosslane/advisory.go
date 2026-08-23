package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const advisoryLane = "advisory"

// runAdvisoryLane drives the MIDDLE path the other lanes never reach.
//
// The opencode lane covers the blocker path (candidate-causal severe finding
// -> correction -> approved) and the claude lane covers the clean path (no
// findings at all -> approved). Neither covers a review that reaches an
// approved receipt WHILE carrying findings that do not block. That gap is not
// theoretical: in the field a host read a WARNING off an approved receipt,
// inferred from the bare severity string that it had to act, "fixed" it, and
// re-ran a whole review on an already-approved candidate.
//
// So this lane freezes exactly that shape -- a medium candidate reviewed into
// one WARNING and one SUGGESTION -- and requires the approved payload to
// declare both non-blocking out loud and to offer no way back into review.
// It is fully deterministic: the reviewer result is supplied through
// `review capture-result --input`, so the lane costs no model or host spend
// and runs in the default battery tier.
func (b *battery) runAdvisoryLane() {
	repo, err := b.scratchRepo("advisory-lane")
	if err != nil {
		b.fail(advisoryLane, "scratch repository", err.Error())
		return
	}
	base := "export function mul(a, b) {\n  return a * b;\n}\n"
	if err = writeFile(repo, "src/mul.js", base); err == nil {
		err = commitAll(repo, "feat: mul")
	}
	if err != nil {
		b.fail(advisoryLane, "scratch repository", err.Error())
		return
	}
	if err := writeFile(repo, "src/mul.js", base+"export function twice(a) {\n  return a + a;\n}\n"); err != nil {
		b.fail(advisoryLane, "scratch repository", err.Error())
		return
	}

	statusDoc, stderr, code := b.status(repo, "claude-code")
	target := getString(statusDoc, "target_identity")
	if target == "" {
		b.fail(advisoryLane, "negotiated status", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
		return
	}
	consent, stderr, _ := b.runJSON("consent", repo,
		"review", "start", "--contract", reviewContract, "--cwd", repo,
		"--target", target, "--projection", "workspace", "--agent", "claude-code", "--consent", "relay")
	granted := grantedInvocation(consent)
	if granted == "" {
		b.fail(advisoryLane, "consent granted round-trip", fmt.Sprintf("no granted invocation in envelope; %s", firstLine(stderr)))
		return
	}
	startDoc, stderr, code := b.runCommandLine("start", repo, granted)
	if code != 0 || getString(startDoc, "state") != "reviewing" || len(getSlice(startDoc, "selected_lenses")) != 1 {
		b.fail(advisoryLane, "consent granted round-trip", fmt.Sprintf("exit=%d state=%q %s", code, getString(startDoc, "state"), firstLine(stderr)))
		return
	}
	if err := b.rememberStarted(repo, target, startDoc); err != nil {
		b.fail(advisoryLane, "consent granted round-trip", err.Error())
		return
	}

	// Reviewer result: non-blocking only. No evidence_class and no
	// causal_disposition, because a non-severe finding never enters causal
	// classification in the first place.
	statusDoc, stderr, _ = b.status(repo, "claude-code")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(advisoryLane, "reviewer collect slot", fmt.Sprintf("no capture-result collect input; %s", firstLine(stderr)))
		return
	}
	args := argumentValues(input)
	reviewer := map[string]any{
		"subject_hash": args["subject-hash"],
		"inspection":   map[string]any{"status": "completed", "paths": []string{"src/mul.js"}},
		"evidence":     []string{"twice() is added by the candidate hunk and no test in the candidate tree covers it"},
		"findings": []map[string]any{
			{
				"lens": args["lens"], "location": "src/mul.js:4", "severity": "WARNING",
				"claim":      "twice() has no covering test in the candidate tree, so its behaviour is unproved.",
				"proof_refs": []string{"src/mul.js:4-6 adds twice() and the candidate adds no test file"},
			},
			{
				"lens": args["lens"], "location": "src/mul.js:5", "severity": "SUGGESTION",
				"claim":      "twice() could reuse mul(a, 2) instead of duplicating the doubling arithmetic.",
				"proof_refs": []string{"src/mul.js:2 already implements the multiplication twice() open-codes"},
			},
		},
	}
	reviewerPath := filepath.Join(b.workRoot, "advisory-reviewer.json")
	payload, err := json.MarshalIndent(reviewer, "", " ")
	if err == nil {
		err = os.WriteFile(reviewerPath, payload, 0o644)
	}
	if err != nil {
		b.fail(advisoryLane, "reviewer manifest", err.Error())
		return
	}
	capture, stderr, code := b.runJSON("result-artifact", repo,
		"review", "capture-result",
		"--lineage", args["lineage"], "--expected-revision", args["expected-revision"],
		"--target", args["target"], "--repository-context", args["repository-context"],
		"--lens", args["lens"], "--order", args["order"], "--subject-hash", args["subject-hash"],
		"--input", reviewerPath)
	if code != 0 || getString(capture, "admission_decision") != "completed" {
		b.fail(advisoryLane, "non-blocking reviewer result captured", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
		return
	}

	// Native admission must retain these non-blocking findings without opening a
	// correction, refuter, or validator route; the final evidence then burns.
	statusDoc, stderr, _ = b.status(repo, "claude-code")
	if getString(statusDoc, "next_transition", "execute", "operation") != "review.finalize" {
		b.fail(advisoryLane, "WARNING + SUGGESTION admitted", fmt.Sprintf("next route = %s/%s %s",
			getString(statusDoc, "next_transition", "kind"), getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
		return
	}
	b.pass(advisoryLane, "WARNING + SUGGESTION admitted", "both non-blocking findings completed admission; no correction or validator route opened")
	b.driveAdvisoryToApproval(repo)
}

// driveAdvisoryToApproval follows the native transitions from a captured
// non-blocking reviewer result through final evidence to the terminal burn.
func (b *battery) driveAdvisoryToApproval(repo string) map[string]any {
	evidencePath := filepath.Join(b.workRoot, "advisory-evidence.txt")
	evidence := fmt.Sprintf("crosslane battery %s: node --check src/mul.js passed on the frozen candidate\n", timestamp())
	if err := os.WriteFile(evidencePath, []byte(evidence), 0o644); err != nil {
		b.fail(advisoryLane, "lifecycle to approved receipt", err.Error())
		return nil
	}
	for step := 0; step < 8; step++ {
		statusDoc, stderr, _ := b.status(repo, "claude-code")
		switch getString(statusDoc, "next_transition", "kind") {
		case "execute":
			doc, execStderr, code := b.runCommandLine("operation", repo, getString(statusDoc, "next_transition", "execute", "command"))
			if code != 0 {
				b.fail(advisoryLane, "lifecycle to approved receipt",
					fmt.Sprintf("%s exit=%d %s", getString(statusDoc, "next_transition", "execute", "operation"), code, firstLine(execStderr)))
				return nil
			}
			switch operationState(doc) {
			case "approved":
				b.burnApproved(advisoryLane, "final evidence and burned", repo, "claude-code", nil, doc)
				return doc
			case "correction_required", "escalated":
				b.fail(advisoryLane, "lifecycle to approved receipt",
					fmt.Sprintf("non-blocking findings changed the outcome to %q; only candidate-caused severe findings may block", operationState(doc)))
				return nil
			}
		case "collect":
			input := collectInput(statusDoc)
			if input == nil || input["capture_operation"] != "review.capture-evidence" {
				b.fail(advisoryLane, "lifecycle to approved receipt",
					fmt.Sprintf("unsupported collect input %v", input["capture_operation"]))
				return nil
			}
			tokens := substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"outcome": "passed", "input": evidencePath})
			captureArgs := append([]string{"review", getString(input, "submission", "operation_token")}, tokens...)
			if record, captureStderr, code := b.runJSON("verification-evidence", b.workRoot, captureArgs...); code != 0 || getString(record, "outcome") != "passed" {
				b.fail(advisoryLane, "lifecycle to approved receipt",
					fmt.Sprintf("evidence capture exit=%d %s", code, firstLine(captureStderr)))
				return nil
			}
		default:
			b.fail(advisoryLane, "lifecycle to approved receipt",
				fmt.Sprintf("unexpected transition %s/%s %s", getString(statusDoc, "next_transition", "kind"),
					getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
			return nil
		}
	}
	b.fail(advisoryLane, "lifecycle to approved receipt", "did not reach a terminal state within the step budget")
	return nil
}
