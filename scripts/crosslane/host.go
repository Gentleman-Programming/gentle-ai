package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// hostCommandTimeout bounds every command a --with-host lane runs: real
// reviewer model processes (codex exec, pi print mode, an OpenCode session)
// are spawned underneath these commands, so a stalled provider must surface
// as a bounded lane failure instead of hanging the battery.
const hostCommandTimeout = 12 * time.Minute

// hostStepBudget bounds the negotiated transition follower. A full medium
// lifecycle is start -> capture -> finalize -> evidence -> finalize; the
// budget leaves room for refuter/validator legs without permitting a loop.
const hostStepBudget = 10

// mergeEnvironment overlays overrides onto the inherited environment,
// replacing any variable both define so no consumer-dependent duplicate-key
// resolution can leak the inherited value through.
func mergeEnvironment(overrides []string) []string {
	overridden := make(map[string]bool, len(overrides))
	for _, entry := range overrides {
		if name, _, found := strings.Cut(entry, "="); found {
			overridden[name] = true
		}
	}
	merged := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		if name, _, found := strings.Cut(entry, "="); found && overridden[name] {
			continue
		}
		merged = append(merged, entry)
	}
	return append(merged, overrides...)
}

// runEnv mirrors run with a bounded timeout and per-lane environment
// overrides replacing their inherited counterparts.
func (b *battery) runEnv(dir string, env []string, args ...string) (string, string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, b.binary, args...)
	// Force Wait to close the stdout/stderr pipes after kill so grandchild
	// processes holding them open cannot block past the context cancel.
	command.WaitDelay = 30 * time.Second
	command.Dir = dir
	overrides := append([]string(nil), env...)
	if b.sandboxHome != "" {
		supplied := make(map[string]bool, len(overrides))
		for _, entry := range overrides {
			if name, _, found := strings.Cut(entry, "="); found {
				supplied[name] = true
			}
		}
		for _, name := range []string{"HOME", "USERPROFILE"} {
			if !supplied[name] {
				overrides = append(overrides, name+"="+b.sandboxHome)
			}
		}
	}
	if len(overrides) > 0 {
		command.Env = mergeEnvironment(overrides)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	code := 0
	if err != nil {
		code = 1
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
		if ctx.Err() != nil {
			return stdout.String(), fmt.Sprintf("timed out after %s: %s", hostCommandTimeout, stderr.String()), code
		}
	}
	return stdout.String(), stderr.String(), code
}

// runJSONEnv mirrors runJSON over runEnv.
func (b *battery) runJSONEnv(source, dir string, env []string, args ...string) (map[string]any, string, int) {
	stdout, stderr, code := b.runEnv(dir, env, args...)
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, stderr, code
	}
	doc := b.record(source, []byte(trimmed))
	return doc, stderr, code
}

// statusEnv mirrors status with the lane's isolated environment.
func (b *battery) statusEnv(repo, agent string, env []string) (map[string]any, string, int) {
	doc, stderr, code := b.runJSONEnv("status", repo, env, b.statusArgs(repo, agent)...)
	if err := b.admitStatusScope(repo, doc); err != nil {
		return nil, err.Error(), 1
	}
	return doc, stderr, code
}

// runCommandLineEnv mirrors runCommandLine over runEnv.
func (b *battery) runCommandLineEnv(source, dir string, env []string, command string) (map[string]any, string, int) {
	fields := strings.Fields(command)
	if len(fields) < 2 || fields[0] != "gentle-ai" {
		return nil, fmt.Sprintf("unexpected provider command %q", command), 1
	}
	return b.runJSONEnv(source, dir, env, fields[1:]...)
}

// argumentTokens returns a collect input's provider-rendered argument tokens
// verbatim ("--name=value"), the exact argv the negotiated contract binds.
func argumentTokens(input map[string]any) []string {
	tokens := make([]string, 0)
	arguments, _ := input["arguments"].([]any)
	for _, raw := range arguments {
		argument, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if token, _ := argument["token"].(string); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func hasArgument(input map[string]any, name string) bool {
	arguments, _ := input["arguments"].([]any)
	for _, raw := range arguments {
		if argument, ok := raw.(map[string]any); ok && argument["name"] == name {
			return true
		}
	}
	return false
}

// hostNegotiatedMediumStart drives status -> provider-rendered review.start
// -> consent/v3 granted round-trip for one host runtime, asserting the frozen
// medium tier. Returns false after recording the failing check.
func (b *battery) hostNegotiatedMediumStart(lane, repo, agent string, env []string) bool {
	statusDoc, stderr, code := b.statusEnv(repo, agent, env)
	command := getString(statusDoc, "next_transition", "execute", "command")
	if getString(statusDoc, "next_transition", "execute", "operation") != "review.start" || command == "" {
		b.fail(lane, "negotiated start", fmt.Sprintf("exit=%d %s/%s %s", code,
			getString(statusDoc, "next_transition", "kind"), getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
		return false
	}
	consent, stderr, _ := b.runCommandLineEnv("consent", repo, env, command)
	if getString(consent, "schema") != "gentle-ai.review-integration.consent/v3" || getString(consent, "action") != "consent_required" {
		b.fail(lane, "consent envelope surfaced", fmt.Sprintf("schema=%q action=%q %s", getString(consent, "schema"), getString(consent, "action"), firstLine(stderr)))
		return false
	}
	granted := grantedInvocation(consent)
	if granted == "" {
		b.fail(lane, "consent envelope surfaced", "no granted choice invocation in envelope")
		return false
	}
	startDoc, stderr, code := b.runCommandLineEnv("start", repo, env, granted)
	if code != 0 || getString(startDoc, "state") != "reviewing" || getString(startDoc, "risk_level") != "medium" {
		b.fail(lane, "consent granted round-trip", fmt.Sprintf("exit=%d state=%q risk=%q %s",
			code, getString(startDoc, "state"), getString(startDoc, "risk_level"), firstLine(stderr)))
		return false
	}
	if err := b.rememberStarted(repo, getString(statusDoc, "target_identity"), startDoc); err != nil {
		b.fail(lane, "consent granted round-trip", err.Error())
		return false
	}
	b.pass(lane, "consent granted round-trip", "consent/v3 surfaced; granted invocation created a reviewing medium lineage")
	return true
}

// hostMediumCandidate writes the shared clean medium-risk candidate: a small
// committed module plus one uncommitted additive function. Real reviewers
// usually approve it, keeping the with-host lanes deterministic in practice
// while a genuine finding still exercises the same transport honestly.
func (b *battery) hostMediumCandidate(lane, name string) (string, bool) {
	repo, err := b.scratchRepo(name)
	if err != nil {
		b.fail(lane, "scratch repository", err.Error())
		return "", false
	}
	base := "export function add(a, b) {\n  return a + b;\n}\n"
	err = writeFile(repo, "src/add.js", base)
	if err == nil {
		err = commitAll(repo, "feat: add")
	}
	if err != nil {
		b.fail(lane, "scratch repository", err.Error())
		return "", false
	}
	extended := base + "export function double(a) {\n  return add(a, a);\n}\n"
	if err := writeFile(repo, "src/add.js", extended); err != nil {
		b.fail(lane, "scratch repository", err.Error())
		return "", false
	}
	return repo, true
}

// hostCaptureLens satisfies one review.capture-result collect input for a
// host lane. Compiled runtimes (codex) run the provider-rendered argv, which
// makes Go spawn the real reviewer process; the pi host relay routes through
// the installed gentle-pi relay implementation instead.
func (b *battery) hostCaptureLens(lane, repo string, env []string, input map[string]any) bool {
	if hasArgument(input, "materialize") {
		return b.runPiRelaySlot(lane, repo, input)
	}
	b.noteHostCost(lane, "1 compiled reviewer subprocess run (capture-result --agent)")
	capture, stderr, code := b.runJSONEnv("result-artifact", repo, env,
		append([]string{"review", "capture-result"}, argumentTokens(input)...)...)
	if code != 0 || getString(capture, "schema") != "gentle-ai.review-result-artifact/v2" || getString(capture, "admission_decision") != "completed" {
		b.fail(lane, "reviewer capture admitted", fmt.Sprintf("exit=%d schema=%q admission=%q %s",
			code, getString(capture, "schema"), getString(capture, "admission_decision"), firstLine(stderr)))
		return false
	}
	b.pass(lane, "reviewer capture admitted", "real reviewer process produced a native result artifact")
	return true
}

// hostFollowToReceipt follows negotiated transitions to the terminal burn.
// A correction_required terminal is a legitimate real-model outcome and is
// reported as such, mirroring the claude --with-model lane.
func (b *battery) hostFollowToReceipt(lane, repo, agent string, env []string) {
	const check = "lifecycle burned"
	evidencePath := filepath.Join(b.workRoot, lane+"-evidence.txt")
	evidence := fmt.Sprintf("crosslane battery %s: node --check passed on the frozen candidate\n", timestamp())
	if err := os.WriteFile(evidencePath, []byte(evidence), 0o644); err != nil {
		b.fail(lane, check, err.Error())
		return
	}
	for step := 0; step < hostStepBudget; step++ {
		statusDoc, statusStderr, _ := b.statusEnv(repo, agent, env)
		kind := getString(statusDoc, "next_transition", "kind")
		switch kind {
		case "execute":
			operation := getString(statusDoc, "next_transition", "execute", "operation")
			doc, execStderr, code := b.runCommandLineEnv("operation", repo, env, getString(statusDoc, "next_transition", "execute", "command"))
			if code != 0 {
				b.fail(lane, check, fmt.Sprintf("%s exit=%d %s", operation, code, firstLine(execStderr)))
				return
			}
			switch operationState(doc) {
			case "approved":
				b.burnApproved(lane, check, repo, agent, env, doc)
				return
			case "correction_required":
				// A real model finding against scratch code is a legitimate
				// outcome; transport, admission, and lifecycle are proven, and
				// the bounded correction flow is not automated in this tier.
				b.pass(lane, check, "real reviewer reported candidate-causal findings (correction_required); transport and admission proven")
				return
			}
		case "collect":
			input := collectInput(statusDoc)
			operation, _ := input["capture_operation"].(string)
			switch operation {
			case "review.capture-result":
				if !b.hostCaptureLens(lane, repo, env, input) {
					return
				}
			case "review.capture-evidence":
				tokens := substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"outcome": "passed", "input": evidencePath})
				captureArgs := append([]string{"review", getString(input, "submission", "operation_token")}, tokens...)
				record, captureStderr, code := b.runJSONEnv("verification-evidence", repo, env, captureArgs...)
				if code != 0 || getString(record, "outcome") != "passed" {
					b.fail(lane, check, fmt.Sprintf("evidence capture exit=%d %s", code, firstLine(captureStderr)))
					return
				}
			case "review.capture-refuter", "review.capture-validation":
				// The provider-rendered argv carries --execute: Go itself spawns
				// the real locked-down pi role process on the materialized request.
				b.noteHostCost(lane, "1 Go-owned pi role process run ("+operation+")")
				roleDoc, roleStderr, code := b.runJSONEnv("provider-role", repo, env,
					append([]string{"review", strings.TrimPrefix(operation, "review.")}, argumentTokens(input)...)...)
				captured, _ := roleDoc["captured"].(bool)
				if code != 0 || !captured {
					b.fail(lane, check, fmt.Sprintf("%s exit=%d captured=%v %s", operation, code, captured, firstLine(roleStderr)))
					return
				}
			default:
				b.fail(lane, check, fmt.Sprintf("unsupported collect input %q for the with-host tier", operation))
				return
			}
		default:
			b.fail(lane, check, fmt.Sprintf("unexpected transition %s/%s %s", kind, getString(statusDoc, "next_transition", "reason_code"), firstLine(statusStderr)))
			return
		}
	}
	b.fail(lane, check, "did not reach a terminal state within the step budget")
}

// noteHostCost records one real model-spend event for the summary.
func (b *battery) noteHostCost(lane, note string) {
	b.hostCosts = append(b.hostCosts, lane+": "+note)
}

// runHostLanes drives the three real host applications end to end.
func (b *battery) runHostLanes() {
	if !b.withHost {
		b.skip(hostCodexLane, "real codex host tier", "pass --with-host to spawn real host applications (dev subscription)")
		b.skip(hostPiLane, "typed SKIP: separate gentle-pi dev-binary evidence contract", "default battery does not fake/copy the relay; --with-host is independent live-host proof")
		b.skip(hostOpenCodeLane, "real opencode host tier", "pass --with-host to spawn real host applications (dev subscription)")
		return
	}
	b.runHostCodexLane()
	b.runHostPiLane()
	b.runHostOpenCodeLane()
}
