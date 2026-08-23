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
// approved terminal result while carrying findings that do not block. That gap is not
// theoretical: in the field a host read a WARNING off an approved review result,
// inferred from the bare severity string that it had to act, "fixed" it, and
// re-ran a whole review on an already-approved candidate.
//
// So this lane freezes exactly that shape -- a medium candidate reviewed into
// one WARNING and one SUGGESTION -- and requires the terminal payload to
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
	if code != 0 || !admittedCapture(capture) {
		b.fail(advisoryLane, "non-blocking reviewer result captured", fmt.Sprintf("exit=%d state=%q %s", code, operationState(capture), firstLine(stderr)))
		return
	}
	if operationState(capture) != "approved" {
		b.fail(advisoryLane, "WARNING + SUGGESTION admitted", fmt.Sprintf("terminal state = %q, want approved", operationState(capture)))
		return
	}
	b.pass(advisoryLane, "WARNING + SUGGESTION admitted", "both non-blocking findings closed on the final capture; no correction or validator route opened")
	b.burnApproved(advisoryLane, "final capture and burned", repo, "claude-code", nil, capture)
}
