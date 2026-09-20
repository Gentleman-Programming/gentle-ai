package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/cli"
)

// hostCommandTimeout bounds every command a --with-host lane runs: real
// reviewer model processes (codex exec, pi print mode, an OpenCode session)
// are spawned underneath these commands, so a stalled provider must surface
// as a bounded lane failure instead of hanging the battery.
const hostCommandTimeout = 12 * time.Minute

// hostStepBudget bounds the negotiated transition follower. A full medium
// lifecycle is start -> capture -> final capture closure; the budget leaves
// room for refuter/validator legs without permitting a loop.
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
	piEnvironment := len(overrides) == 4 && strings.HasPrefix(overrides[0], "HOME=") && strings.HasPrefix(overrides[1], "PATH=") && strings.HasPrefix(overrides[2], "PI_CODING_AGENT_DIR=") && overrides[3] == "GENTLE_PI_REVIEW_RELAY_CONTRACT=gentle-pi.review-relay/v1"
	if b.sandboxHome != "" && !piEnvironment {
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
		if piEnvironment {
			command.Env = overrides
		} else {
			command.Env = mergeEnvironment(overrides)
		}
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

// runCommandLineEnv mirrors runCommandLine over runEnv with the product's
// quoting-aware splitter. Structured closures use runTransitionExecution
// instead. Accepts either the canonical "axiom" name or the preserved
// "gentle-ai" alias a provider might print (D-03/D2.4 dual tolerance on
// read), current name evaluated first.
func (b *battery) runCommandLineEnv(source, dir string, env []string, command string) (map[string]any, string, int) {
	words, err := cli.SplitPrintedCommandWords(command)
	if err != nil || len(words) < 2 || (words[0] != "axiom" && words[0] != "gentle-ai") {
		return nil, fmt.Sprintf("unexpected provider command %q", command), 1
	}
	return b.runJSONEnv(source, dir, env, words[1:]...)
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

// authoredRoleVerdict materializes the exact Go-issued non-lens provider role
// request for a host-relay collect input, then authors a minimal admissible
// verdict from it instead of relaying it through a model (#4611: Go never
// spawns pi for these roles, so there is nothing left for the battery to
// spawn either). The battery's only model relay path (runPiRelaySlot,
// through gentle-pi's installed review-host-relay.ts) is built for the lens
// role only; teaching it the two non-lens roles belongs to gentle-pi's own
// in-process completion work (this feature's sibling PR under
// inprocess-reviewer-completion.md), not this cross-lane harness. The
// refuter and validator verdicts this produces are authored test fixtures,
// not real adversarial or acceptance judgments -- they exist only to prove
// the submission and admission mechanism, the same role this battery's other
// authored fixtures (e.g. hostPiCorrectedModule) already play.
func (b *battery) authoredRoleVerdict(lane, repo string, env []string, input map[string]any) (string, bool) {
	fail := func(note string) (string, bool) { b.fail(lane, "author role verdict", note); return "", false }
	operation := strings.TrimPrefix(getString(input, "capture_operation"), "review.")
	prompt, stderr, code := b.runEnv(repo, env, append([]string{"review", operation}, argumentTokens(input)...)...)
	if code != 0 || strings.TrimSpace(prompt) == "" {
		return fail(fmt.Sprintf("materialize %s request exit=%d %s", operation, code, firstLine(stderr)))
	}
	switch getString(input, "submission", "value", "slot") {
	case "provider_targeted_validator":
		values := argumentValues(input)
		if values["request-hash"] == "" || values["target"] == "" {
			return fail("targeted-validator collect input omitted its binding arguments")
		}
		payload, err := json.Marshal(map[string]any{
			"targeted_validation_request_hash": values["request-hash"],
			"correction_target_identity":       values["target"],
			"original_criteria":                map[string]any{"passed": true, "evidence": []string{"crosslane authored fixture verdict: original acceptance criteria re-ran and passed"}},
			"correction_regression":            map[string]any{"passed": true, "evidence": []string{"crosslane authored fixture verdict: no regression introduced"}},
			"follow_ups":                       []any{},
		})
		if err != nil {
			return fail(err.Error())
		}
		return string(payload), true
	case "provider_refuter":
		request, err := hostPiRoleRequestPayload(prompt)
		if err != nil {
			return fail(err.Error())
		}
		requestHash, _ := request["request_hash"].(string)
		claims, _ := request["claims"].([]any)
		results := make([]map[string]any, 0, len(claims))
		for _, raw := range claims {
			claim, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if findingID, _ := claim["finding_id"].(string); findingID != "" {
				results = append(results, map[string]any{
					"finding_id": findingID, "outcome": "inconclusive",
					"proof_refs": []string{"crosslane authored fixture verdict: no independent reproduction attempted"},
				})
			}
		}
		if requestHash == "" || len(results) == 0 {
			return fail(fmt.Sprintf("materialized refuter request carried no request_hash or claims: %s", firstLine(prompt)))
		}
		payload, err := json.Marshal(map[string]any{"refuter_request_hash": requestHash, "results": results})
		if err != nil {
			return fail(err.Error())
		}
		return string(payload), true
	default:
		return fail(fmt.Sprintf("unsupported provider role submission slot %q", getString(input, "submission", "value", "slot")))
	}
}

// hostPiRoleRequestPayload extracts the JSON "Input:" section a Go-issued
// non-lens role prompt embeds between its instruction and its output schema,
// mirroring the delimiter the product's own materialization uses
// (reviewProviderRoleTaskRequest: "%s\n\nInput:\n%s\n\nOutput schema:\n%s").
func hostPiRoleRequestPayload(prompt string) (map[string]any, error) {
	_, after, found := strings.Cut(prompt, "\n\nInput:\n")
	if !found {
		return nil, fmt.Errorf("materialized role prompt carries no Input section")
	}
	body, _, found := strings.Cut(after, "\n\nOutput schema:\n")
	if !found {
		return nil, fmt.Errorf("materialized role prompt carries no Output schema section")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, fmt.Errorf("decode materialized role request: %w", err)
	}
	return payload, nil
}

// submitRoleVerdict writes an authored role verdict to a scratch file outside
// the reviewed repository (the frozen candidate must never gain an untracked
// file it did not intend) and submits it through the collect input's
// rendered submission descriptor, substituting its {{value}} slot with that
// file's path exactly as a real host-relay runtime would after running its
// own reviewer out of process.
func (b *battery) submitRoleVerdict(lane, repo string, env []string, input map[string]any, verdict string) (map[string]any, string, int) {
	path := filepath.Join(b.workRoot, "host-pi-role-verdict.json")
	if err := os.WriteFile(path, []byte(verdict), 0o644); err != nil {
		return nil, err.Error(), 1
	}
	tokens := substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"value": path})
	return b.runJSONEnv("role-submit", repo, env, append([]string{"review", getString(input, "submission", "operation_token")}, tokens...)...)
}

func (b *battery) hostNegotiatedStart(lane, repo, agent string, env []string, risk string) bool {
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
	if code != 0 || getString(startDoc, "state") != "reviewing" || getString(startDoc, "risk_level") != risk || risk == "high" && fmt.Sprint(getSlice(startDoc, "selected_lenses")) != "[review-risk review-resilience review-readability review-reliability]" {
		b.fail(lane, "consent granted round-trip", fmt.Sprintf("exit=%d state=%q risk=%q %s",
			code, getString(startDoc, "state"), getString(startDoc, "risk_level"), firstLine(stderr)))
		return false
	}
	if err := b.rememberStarted(repo, getString(statusDoc, "target_identity"), startDoc); err != nil {
		b.fail(lane, "consent granted round-trip", err.Error())
		return false
	}
	b.pass(lane, "consent granted round-trip", "consent/v3 surfaced; granted invocation created a reviewing "+risk+" lineage with its canonical lenses")
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
func (b *battery) hostCaptureLens(lane, repo, agent string, env []string, input map[string]any) bool {
	if hasArgument(input, "materialize") {
		_, admitted := b.runPiRelaySlot(lane, repo, input)
		return admitted
	}
	b.noteHostCost(lane, "1 compiled reviewer subprocess run (capture-result --agent)")
	capture, stderr, code := b.runJSONEnv("result-artifact", repo, env,
		append([]string{"review", "capture-result"}, argumentTokens(input)...)...)
	if code != 0 || !admittedCapture(capture) {
		b.fail(lane, "reviewer capture admitted", fmt.Sprintf("exit=%d schema=%q admission=%q %s",
			code, getString(capture, "schema"), getString(capture, "admission_decision"), firstLine(stderr)))
		return false
	}
	b.pass(lane, "reviewer capture admitted", "real reviewer process produced an admitted native capture")
	switch operationState(capture) {
	case "approved":
		b.acknowledgeApproved(lane, "lifecycle acknowledged and burned", repo, agent, env, capture)
		return false
	case "correction_required":
		b.hostCorrectionReentry(lane, "lifecycle correction re-entry", repo, env, capture)
		return false
	}
	return true
}

// hostCorrectionReentry follows the provider-owned status continuation from a
// final correction_required capture. Hosts must never rebuild its selectors.
func (b *battery) hostCorrectionReentry(lane, check, repo string, env []string, closure map[string]any) bool {
	status, stderr, code := b.statusFromClosureEnv(repo, env, closure)
	if code != 0 || getString(status, "next_transition", "reason_code") != "correction_plan_required" {
		b.fail(lane, check, fmt.Sprintf("closure continuation exit=%d reason=%q %s", code, getString(status, "next_transition", "reason_code"), firstLine(stderr)))
		return false
	}
	b.pass(lane, check, "provider closure status_continuation re-entered correction planning without host selector reconstruction")
	return true
}

// hostFollowToReceipt follows negotiated transitions to the terminal burn.
func (b *battery) hostFollowToReceipt(lane, repo, agent string, env []string) {
	const check = "lifecycle acknowledged and burned"
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
				b.acknowledgeApproved(lane, check, repo, agent, env, doc)
				return
			case "correction_required":
				b.hostCorrectionReentry(lane, "lifecycle correction re-entry", repo, env, doc)
				return
			}
		case "collect":
			input := collectInput(statusDoc)
			operation, _ := input["capture_operation"].(string)
			switch operation {
			case "review.capture-result":
				if !b.hostCaptureLens(lane, repo, agent, env, input) {
					return
				}
			case "review.capture-refuter", "review.capture-validation":
				var roleDoc map[string]any
				var roleStderr string
				var code int
				if hasArgument(input, "materialize") && getMap(input, "submission") != nil {
					// pi is host-mediated: Go never spawns a process for this
					// role (#4611). The battery materializes the exact
					// Go-issued request itself and submits an authored
					// fixture verdict through --input (authoredRoleVerdict).
					verdict, ok := b.authoredRoleVerdict(lane, repo, env, input)
					if !ok {
						return
					}
					b.noteHostCost(lane, "0 model runs (authored fixture verdict, "+operation+", pi host relay #4611)")
					roleDoc, roleStderr, code = b.submitRoleVerdict(lane, repo, env, input, verdict)
				} else {
					// The provider-rendered argv carries --execute: Go itself
					// spawns the real locked-down compiled adapter process on
					// the materialized request.
					b.noteHostCost(lane, "1 Go-owned "+agent+" role process run ("+operation+")")
					roleDoc, roleStderr, code = b.runJSONEnv("provider-role", repo, env,
						append([]string{"review", strings.TrimPrefix(operation, "review.")}, argumentTokens(input)...)...)
				}
				if code != 0 || !admittedCapture(roleDoc) {
					b.fail(lane, check, fmt.Sprintf("%s exit=%d state=%q %s", operation, code, operationState(roleDoc), firstLine(roleStderr)))
					return
				}
				switch operationState(roleDoc) {
				case "approved":
					b.acknowledgeApproved(lane, check, repo, agent, env, roleDoc)
					return
				case "correction_required":
					b.hostCorrectionReentry(lane, "lifecycle correction re-entry", repo, env, roleDoc)
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
