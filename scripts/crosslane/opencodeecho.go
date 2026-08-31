package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The step name is deliberately about the host behaviour, not the Go symbol:
// this is the round trip a real OpenCode host performs when its first review
// Task never executed.
const openCodeEchoStep = "lens frame: host-echoed materialization"

// runOpenCodeHostEchoScenario drives the exact two-call round trip that reached
// the field and that every other case in this battery skipped.
//
// A real OpenCode host persists the plugin-mutated Task argument into its own
// transcript. When the first Task never runs, the driver model's next attempt
// re-types that transcript entry -- the whole Go-issued materialization --
// instead of the short contract binding frame, and the copy always drifts.
// Every other case here fires the before hook once with a freshly assembled
// binding, so none of them can see what a second call carrying a paraphrased
// materialization does.
//
// The scenario owns a private lineage: phase one leaves an uncaptured slot on
// purpose, and the assertion is that phase two still reaches durable capture
// through Go-canonical bytes.
func (b *battery) runOpenCodeHostEchoScenario(node string) {
	repo, err := b.scratchRepo("opencode-echo")
	if err != nil {
		b.fail(openCodeLane, openCodeEchoStep, "scratch repository: "+err.Error())
		return
	}
	base := "export function trim(value) {\n  return value.trim();\n}\n"
	if err := writeFile(repo, "src/trim.js", base); err == nil {
		err = commitAll(repo, "feat: trim")
	}
	if err != nil {
		b.fail(openCodeLane, openCodeEchoStep, "scratch repository: "+err.Error())
		return
	}
	if err := writeFile(repo, "src/trim.js", base+"export function head(value) {\n  return value.split(\"\\n\")[0].trim();\n}\n"); err != nil {
		b.fail(openCodeLane, openCodeEchoStep, "scratch candidate: "+err.Error())
		return
	}

	args, ok := b.openCodeMediumLensSlot(repo)
	if !ok {
		return
	}
	reviewerJSON, err := json.Marshal(map[string]any{
		"subject_hash": args["subject-hash"],
		"inspection":   map[string]any{"status": "completed", "paths": []string{"src/trim.js"}},
		"evidence":     []string{"head splits on a literal newline before trimming; introduced by the candidate hunk"},
		"findings":     []any{},
	})
	if err != nil {
		b.fail(openCodeLane, openCodeEchoStep, "reviewer manifest: "+err.Error())
		return
	}

	// Phase one: the contract-shaped host frame. The Task never executes, so
	// the relay is disposed and the slot stays open, exactly as in the field.
	issue, err := b.runHookCase(node, harnessCase{
		Name:     "echo-issue",
		Subagent: args["lens"],
		BindingPairs: [][2]any{
			{"lineage", args["lineage"]},
			{"target", args["target"]},
			{"lens", args["lens"]},
			{"order", args["order"]},
			{"revision", args["expected-revision"]},
			{"repository_context", args["repository-context"]},
			{"subject_hash", args["subject-hash"]},
		},
		Body:       "Review this frozen candidate through the assigned lens.",
		TaskOutput: string(reviewerJSON),
		SkipAfter:  true,
	})
	if err != nil {
		b.fail(openCodeLane, openCodeEchoStep, "materialize the first Task: "+err.Error())
		return
	}
	if !issue.BeforeOK || !strings.HasPrefix(issue.ChildPrompt, "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION ") {
		b.fail(openCodeLane, openCodeEchoStep, "first Task did not receive a Go materialization: "+firstLine(issue.Error))
		return
	}

	// Phase two: the host echoes that materialization back with the drift a
	// re-typed copy always carries.
	echoed, ok := hostEchoedMaterialization(issue.ChildPrompt)
	if !ok {
		b.fail(openCodeLane, openCodeEchoStep, "could not derive a drifted host echo from the Go materialization")
		return
	}
	replay, err := b.runHookCase(node, harnessCase{
		Name: "echo-replay", Subagent: args["lens"], Prompt: echoed, TaskOutput: string(reviewerJSON),
	})
	switch {
	case err != nil:
		b.fail(openCodeLane, openCodeEchoStep, err.Error())
		return
	case !replay.BeforeOK && strings.Contains(replay.Error, bindingInvalid):
		b.fail(openCodeLane, openCodeEchoStep,
			"host-echoed materialization refused before launch, wedging the reviewer slot: "+firstLine(replay.Error))
		return
	case !replay.AfterOK:
		b.fail(openCodeLane, openCodeEchoStep, "unexpected failure: "+firstLine(replay.Error))
		return
	case replay.ChildPrompt != issue.ChildPrompt:
		b.fail(openCodeLane, openCodeEchoStep, fmt.Sprintf(
			"child received %d bytes, want the %d-byte Go materialization", len(replay.ChildPrompt), len(issue.ChildPrompt)))
		return
	}
	manifest := b.record("result-artifact", []byte(replay.Output))
	if !admittedCapture(manifest) {
		b.fail(openCodeLane, openCodeEchoStep, "host echo did not round-trip a completed result artifact")
		return
	}
	b.pass(openCodeLane, openCodeEchoStep,
		"drifted host echo was rebuilt into Go-canonical bytes and captured; only the exact bytes stay a pass-through")
	if operationState(manifest) != "approved" {
		b.fail(openCodeLane, "host-echo lifecycle acknowledged and burned", fmt.Sprintf("terminal state = %q, want approved", operationState(manifest)))
		return
	}
	b.acknowledgeApproved(openCodeLane, "host-echo lifecycle acknowledged and burned", repo, "opencode", nil, manifest)
}

// hostEchoedMaterialization reproduces the drift a re-typed materialization
// carries: the short marker line and inner binding survive, the long provider
// body loses insignificant whitespace. The field occurrence dropped exactly
// this space inside the embedded result schema.
func hostEchoedMaterialization(materialized string) (string, bool) {
	marker, body, found := strings.Cut(materialized, "\n")
	if !found {
		return "", false
	}
	drifted := strings.Replace(body, `"findings": [], "evidence"`, `"findings": [],"evidence"`, 1)
	if drifted == body {
		return "", false
	}
	return marker + "\n" + drifted, true
}

// openCodeMediumLensSlot walks consent, start, and status for one scratch
// repository and returns the reviewer collect arguments.
func (b *battery) openCodeMediumLensSlot(repo string) (map[string]string, bool) {
	statusDoc, stderr, code := b.status(repo, "opencode")
	target := getString(statusDoc, "target_identity")
	if target == "" {
		b.fail(openCodeLane, openCodeEchoStep, fmt.Sprintf("negotiated status exit=%d %s", code, firstLine(stderr)))
		return nil, false
	}
	consent, stderr, _ := b.runJSON("consent", repo,
		"review", "start", "--contract", reviewContract, "--cwd", repo,
		"--target", target, "--projection", "workspace", "--agent", "opencode", "--consent", "relay")
	granted := grantedInvocation(consent)
	if granted == "" {
		b.fail(openCodeLane, openCodeEchoStep, "no granted choice invocation in envelope; "+firstLine(stderr))
		return nil, false
	}
	startDoc, stderr, code := b.runCommandLine("start", repo, granted)
	if code != 0 || getString(startDoc, "state") != "reviewing" {
		b.fail(openCodeLane, openCodeEchoStep, fmt.Sprintf("consent granted start exit=%d state=%q %s",
			code, getString(startDoc, "state"), firstLine(stderr)))
		return nil, false
	}
	if err := b.rememberStarted(repo, target, startDoc); err != nil {
		b.fail(openCodeLane, openCodeEchoStep, err.Error())
		return nil, false
	}
	statusDoc, stderr, _ = b.status(repo, "opencode")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(openCodeLane, openCodeEchoStep, "no review.capture-result collect input; "+firstLine(stderr))
		return nil, false
	}
	return argumentValues(input), true
}
