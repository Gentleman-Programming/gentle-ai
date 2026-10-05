package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed harness.mts
var hookHarness string

const openCodeLane = "opencode"

// pluginSourcePath is the real transport plugin the battery drives. The bytes
// are read from the repository at run time so the battery always exercises
// the current plugin, never an embedded copy.
const pluginSourcePath = "internal/assets/opencode/plugins/opencode-review-transport.ts"

const bindingInvalid = "opencode_review_transport_binding_invalid"

type harnessResult struct {
	Name        string `json:"name"`
	BeforeOK    bool   `json:"before_ok"`
	AfterOK     bool   `json:"after_ok"`
	ChildPrompt string `json:"child_prompt"`
	Output      string `json:"output"`
	Error       string `json:"error"`
}

type harnessCase struct {
	Name       string `json:"name"`
	Directory  string `json:"-"`
	Subagent   string `json:"subagent"`
	Prompt     string `json:"prompt"`
	TaskOutput string `json:"task_output"`
	SkipAfter  bool   `json:"skip_after,omitempty"`
}

// runOpenCodeLane drives the real plugin bytes through provider-owned Task
// frames against the committed-only issue scenario: immutable base tree,
// committed medium candidate, terminal correction closure, and its exact
// status_continuation before the bounded correction/validator path continues.
func (b *battery) runOpenCodeLane() {
	base := "export function greet(name) {\n  return \"hi \" + name;\n}\n"
	unsafe := base + "export function shout(name) {\n  return name.toUpperCase() + \"!\";\n}\n"
	repo, baseTree, ok := b.committedMediumCandidate(openCodeLane, "opencode-committed", "src/greet.js", base, unsafe)
	if !ok || !b.startCommittedMedium(openCodeLane, repo, "opencode", baseTree) {
		return
	}

	// Reviewer collect slot: the host relays the provider-owned Task unchanged.
	statusDoc, stderr, _ := b.status(repo, "opencode")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(openCodeLane, "reviewer collect slot", fmt.Sprintf("no review.capture-result collect input; %s", firstLine(stderr)))
		return
	}
	args := argumentValues(input)
	lensAgent, lensPrompt, ok := providerTask(input)
	if !ok {
		b.fail(openCodeLane, "reviewer collect slot", "review.capture-result input omitted its provider-owned task")
		return
	}
	node, err := b.prepareHookHarness()
	if err != nil {
		b.fail(openCodeLane, "hook harness setup", err.Error())
		return
	}

	// Owns a private lineage, so it neither consumes nor depends on this
	// lineage's reviewer slot.
	b.runOpenCodeHostEchoScenario(node)

	reviewer := map[string]any{
		"subject_hash": args["subject-hash"],
		"inspection":   map[string]any{"status": "completed", "paths": []string{"src/greet.js"}},
		"evidence":     []string{"shout calls toUpperCase without a nullish guard; introduced by the candidate hunk"},
		"findings": []map[string]any{{
			"claim":              "shout calls toUpperCase on its argument without a null/undefined guard",
			"severity":           "BLOCKER",
			"evidence_class":     "deterministic",
			"causal_disposition": "introduced",
			"lens":               "review-reliability",
			"location":           "src/greet.js:5",
			"proof_refs":         []string{"src/greet.js:4-6 calls name.toUpperCase() with no nullish guard in the candidate tree"},
		}},
	}
	reviewerJSON, err := json.Marshal(reviewer)
	if err != nil {
		b.fail(openCodeLane, "reviewer manifest", err.Error())
		return
	}

	result, err := b.runHookCase(node, harnessCase{
		Name: "lens-provider-owned", Directory: repo, Subagent: lensAgent, Prompt: lensPrompt, TaskOutput: string(reviewerJSON),
	})
	var lensClosure map[string]any
	switch {
	case err != nil:
		b.fail(openCodeLane, "lens frame: provider-owned", err.Error())
		return
	case !result.AfterOK:
		b.fail(openCodeLane, "lens frame: provider-owned", firstLine(result.Error))
		return
	case !strings.HasPrefix(result.ChildPrompt, "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION "):
		b.fail(openCodeLane, "lens frame: provider-owned", "child prompt is not the Go-issued materialization")
		return
	}
	lensClosure = b.record("result-artifact", []byte(result.Output))
	if !admittedCapture(lensClosure) {
		b.fail(openCodeLane, "lens frame: provider-owned", "completion did not round-trip an admitted terminal capture")
		return
	}
	b.pass(openCodeLane, "lens frame: provider-owned", "exact provider task materialized and captured end to end")

	// Correction flow to reach a live validator role slot.
	if !b.driveCorrectionToValidation(repo, base, lensClosure) {
		return
	}

	statusDoc, stderr, _ = b.status(repo, "opencode")
	input = collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "external.run_provider_role" {
		b.fail(openCodeLane, "validator role slot", fmt.Sprintf("no provider role collect input; %s", firstLine(stderr)))
		return
	}
	providerAgent, providerPrompt, providerTaskOK := providerTask(input)
	validationRequest := getMap(statusDoc, "validation_request")
	if !providerTaskOK || validationRequest == nil {
		b.fail(openCodeLane, "validator role slot", "provider task prompt or validation request missing from status")
		return
	}
	validator := map[string]any{
		"targeted_validation_request_hash": validationRequest["request_hash"],
		"correction_target_identity":       validationRequest["correction_target_identity"],
		"original_criteria": map[string]any{
			"passed":   true,
			"evidence": []string{"frozen correction tree guards name == null before toUpperCase per the embedded diff"},
		},
		"correction_regression": map[string]any{
			"passed":   true,
			"evidence": []string{"greet() is untouched by the correction diff; only shout gained the guard"},
		},
		"follow_ups": []any{},
	}
	validatorJSON, err := json.Marshal(validator)
	if err != nil {
		b.fail(openCodeLane, "validator manifest", err.Error())
		return
	}

	// #3380: the targeted validator is the one review role expected to inspect
	// the immutable corrected candidate itself. Prove it can, using only the
	// bytes the real plugin hands its child. The probe returns a deliberate
	// non-result so the relay refuses the completion and the validator slot
	// stays open for the frames below.
	probe, err := b.runHookCase(node, harnessCase{
		Name:       "validator-inspection-recipe",
		Directory:  repo,
		Subagent:   providerAgent,
		Prompt:     providerPrompt,
		TaskOutput: "probe: no verdict submitted",
	})
	switch {
	case err != nil:
		b.fail(openCodeLane, "validator inspection recipe", err.Error())
	case !probe.BeforeOK || probe.ChildPrompt == "":
		b.fail(openCodeLane, "validator inspection recipe", "relay materialized no child prompt: "+firstLine(probe.Error))
	default:
		b.checkValidatorInspectionRecipe(repo, probe.ChildPrompt)
	}

	// An inconclusive validator result is a non-verdict: it must never occupy
	// the immutable slot, because admitting it as failed would spend the one
	// correction attempt on an observation that was never made (issue #3378).
	// This is the deterministic half of that fix; the routing half needs a
	// slot published by a pre-fix build, which no public command can create
	// once the detector refuses these bytes.
	inconclusive := map[string]any{
		"targeted_validation_request_hash": validationRequest["request_hash"],
		"correction_target_identity":       validationRequest["correction_target_identity"],
		"original_criteria": map[string]any{
			"passed":   false,
			"evidence": []string{"Immutable correction candidate tree could not be inspected with read-only Git access, so no verdict was produced"},
		},
		"correction_regression": map[string]any{
			"passed":   false,
			"evidence": []string{"Immutable correction candidate tree could not be inspected with read-only Git access, so no verdict was produced"},
		},
		"follow_ups": []any{},
	}
	inconclusiveJSON, err := json.Marshal(inconclusive)
	if err != nil {
		b.fail(openCodeLane, "validator frame: inconclusive refused", err.Error())
		return
	}
	switch refused, err := b.runHookCase(node, harnessCase{
		Name:       "validator-inconclusive",
		Directory:  repo,
		Subagent:   providerAgent,
		Prompt:     providerPrompt,
		TaskOutput: string(inconclusiveJSON),
	}); {
	case err != nil:
		b.fail(openCodeLane, "validator frame: inconclusive refused", err.Error())
		return
	case refused.AfterOK:
		b.fail(openCodeLane, "validator frame: inconclusive refused",
			"an uninspected-candidate validator result was admitted; it would spend the single correction attempt on a non-observation")
		return
	default:
		statusDoc, stderr, _ = b.status(repo, "opencode")
		retry := collectInput(statusDoc)
		if retry == nil || retry["capture_operation"] != "external.run_provider_role" {
			b.fail(openCodeLane, "validator frame: inconclusive refused",
				fmt.Sprintf("refused inconclusive result did not leave the validator slot retryable; %s %s",
					getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
			return
		}
		b.pass(openCodeLane, "validator frame: inconclusive refused", "uninspected-candidate verdict refused and the validation stayed retryable")
	}

	exact, err := b.runHookCase(node, harnessCase{
		Name: "validator-provider-owned", Directory: repo, Subagent: providerAgent, Prompt: providerPrompt, TaskOutput: string(validatorJSON),
	})
	if err != nil {
		b.fail(openCodeLane, "validator frame: provider-owned", err.Error())
		return
	}
	if !exact.AfterOK {
		b.fail(openCodeLane, "validator frame: provider-owned", firstLine(exact.Error))
		return
	}
	validationClosure := b.record("provider-role", []byte(exact.Output))
	if !admittedCapture(validationClosure) {
		b.fail(openCodeLane, "validator frame: provider-owned", "role completion did not report an admitted validator capture")
		return
	}
	b.pass(openCodeLane, "validator frame: provider-owned", "exact provider task round-tripped and captured")

	if operationState(validationClosure) != "approved" {
		b.fail(openCodeLane, "correction lifecycle approved", fmt.Sprintf("terminal state = %q, want approved", operationState(validationClosure)))
		return
	}
	b.acknowledgeApproved(openCodeLane, "correction lifecycle acknowledged and burned", repo, "opencode", nil, validationClosure)
}

// driveCorrectionToValidation follows the final reviewer capture directly to
// the correction plan, then captures targeted validator evidence after editing.
func (b *battery) driveCorrectionToValidation(repo, fixedBase string, closure map[string]any) bool {
	// The final reviewer capture already produced correction_required; its exact
	// continuation now offers the narrow correction-plan capture before any edit.
	statusDoc, stderr, code := b.statusFromClosure(repo, closure)
	if code != 0 || getString(statusDoc, "authority", "lineage_id") != operationLineage(closure) ||
		getString(statusDoc, "next_transition", "reason_code") != "correction_plan_required" {
		b.fail(openCodeLane, "committed OpenCode correction re-entry", fmt.Sprintf("exit=%d lineage=%q reason=%q %s",
			code, getString(statusDoc, "authority", "lineage_id"), getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
		return false
	}
	b.pass(openCodeLane, "committed OpenCode correction re-entry", "fresh Node/plugin process followed closure operation plus ordered tokens to correction_plan_required")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-correction-plan" {
		b.fail(openCodeLane, "correction: plan forecast", fmt.Sprintf("no capture-correction-plan collect input; %s", firstLine(stderr)))
		return false
	}
	tokens := substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"value": "2"})
	planArgs := append([]string{"review", getString(input, "submission", "operation_token")}, tokens...)
	plan, stderr, code := b.runJSON("operation", repo, planArgs...)
	if code != 0 || operationState(plan) != "correction_required" {
		b.fail(openCodeLane, "correction: plan forecast", fmt.Sprintf("exit=%d state=%q %s", code, operationState(plan), firstLine(stderr)))
		return false
	}

	// Bounded fix edit.
	fixed := fixedBase + "export function shout(name) {\n  if (name == null) return \"!\";\n  return name.toUpperCase() + \"!\";\n}\n"
	err := writeFile(repo, "src/greet.js", fixed)
	if err == nil {
		err = commitAll(repo, "fix: guarded correction candidate")
	}
	if err != nil {
		b.fail(openCodeLane, "correction: bounded fix edit", err.Error())
		return false
	}

	b.pass(openCodeLane, "correction: plan and fix", "forecast captured before the bounded committed fix; STATUS now owns the targeted-validator role route")
	return true
}

// prepareHookHarness materializes the node harness directory: the REAL plugin
// bytes, the hook emulator, and a PATH shim so the plugin's spawn("gentle-ai")
// resolves to the binary under test.
func (b *battery) prepareHookHarness() (string, error) {
	if _, err := exec.LookPath("node"); err != nil {
		return "", fmt.Errorf("node is unavailable: %w", err)
	}
	plugin, err := os.ReadFile(filepath.Join(b.repoRoot, pluginSourcePath))
	if err != nil {
		return "", fmt.Errorf("read real plugin bytes: %w", err)
	}
	dir := filepath.Join(b.workRoot, "hook-harness")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.mts"), plugin, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "harness.mts"), []byte(hookHarness), 0o644); err != nil {
		return "", err
	}
	shim := "#!/bin/sh\nexec \"" + b.binary + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "bin", "gentle-ai"), []byte(shim), 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// runHookCase executes one hook case in a fresh node process so every case
// gets an isolated relay registry, exactly like a fresh host session.
// checkValidatorInspectionRecipe answers the question the two #3380 field
// reports could not: can a targeted validator reach the frozen corrected tree
// from what it was handed, and nothing else? It parses the child prompt the
// real plugin delivered, assembles the inspection command from that JSON alone,
// and runs it against the binary under test. Deterministic end to end: no model
// spend, no host application. The residual gap it does NOT cover is whether a
// live reviewer model chooses to run the command it is now given -- that needs
// --with-model, and no assertion here should be read as covering it.
func (b *battery) checkValidatorInspectionRecipe(repo, childPrompt string) {
	const name = "validator inspection recipe"
	_, rest, found := strings.Cut(childPrompt, "\n\nInput:\n")
	if !found {
		b.fail(openCodeLane, name, "child prompt carries no provider Input block")
		return
	}
	payload, _, found := strings.Cut(rest, "\n\nOutput schema:\n")
	if !found {
		b.fail(openCodeLane, name, "child prompt carries no provider Output schema block")
		return
	}
	var request struct {
		RepositoryContext string `json:"repository_context"`
		ValidationRequest struct {
			RequestHash              string   `json:"request_hash"`
			LineageID                string   `json:"lineage_id"`
			ExpectedRevision         string   `json:"expected_revision"`
			CorrectionTargetIdentity string   `json:"correction_target_identity"`
			CorrectionPaths          []string `json:"correction_paths"`
		} `json:"validation_request"`
	}
	if err := json.Unmarshal([]byte(payload), &request); err != nil {
		b.fail(openCodeLane, name, "child prompt Input is not decodable JSON: "+err.Error())
		return
	}
	if !strings.Contains(childPrompt, "gentle-ai review inspect-candidate") {
		b.fail(openCodeLane, name, "child prompt never names the immutable inspection command")
		return
	}
	if request.RepositoryContext == "" || len(request.ValidationRequest.CorrectionPaths) == 0 {
		b.fail(openCodeLane, name, "child prompt omits the repository context or the correction paths, so the recipe is underivable")
		return
	}
	binding := []string{
		"review", "inspect-candidate", "--purpose", "targeted-validation",
		"--lineage", request.ValidationRequest.LineageID,
		"--expected-revision", request.ValidationRequest.ExpectedRevision,
		"--target", request.ValidationRequest.CorrectionTargetIdentity,
		"--request-hash", request.ValidationRequest.RequestHash,
		"--repository-context", request.RepositoryContext,
	}
	for _, operation := range [][]string{
		{"--operation", "name-status"},
		{"--operation", "numstat"},
		{"--operation", "stat", "--path-index", "0"},
		{"--operation", "patch", "--path-index", "0"},
		{"--operation", "object", "--path-index", "0", "--side", "candidate"},
	} {
		stdout, stderr, code := b.run(repo, append(append([]string(nil), binding...), operation...)...)
		if code != 0 || strings.TrimSpace(stdout) == "" {
			b.fail(openCodeLane, name, fmt.Sprintf("inspect-candidate %v derived from the child prompt alone failed: exit=%d %s",
				operation, code, firstLine(stderr)))
			return
		}
	}
	b.pass(openCodeLane, name, "the relayed child prompt alone reaches the frozen corrected tree through every inspection operation")
}

func (b *battery) runHookCase(harnessDir string, c harnessCase) (harnessResult, error) {
	if c.Directory == "" {
		return harnessResult{}, fmt.Errorf("hook harness case %q has no provider-bound working directory", c.Name)
	}
	configPath := filepath.Join(harnessDir, c.Name+".case.json")
	payload, err := json.Marshal(c)
	if err != nil {
		return harnessResult{}, err
	}
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		return harnessResult{}, err
	}
	command := exec.Command("node", filepath.Join(harnessDir, "harness.mts"), configPath)
	command.Dir = c.Directory
	// The harness spawns gentle-ai itself, so it has to inherit the battery's
	// sandbox HOME too. Without it the transport child resolves the operator's
	// own review mode instead of the battery's, and on any machine that never
	// opted in the lane fails as an unavailable materialization rather than
	// telling the truth: reviews were off.
	command.Env = mergeEnvironment([]string{
		"HOME=" + b.sandboxHome,
		"USERPROFILE=" + b.sandboxHome,
		"PATH=" + filepath.Join(harnessDir, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	output, err := command.Output()
	if err != nil {
		detail := ""
		if exit, ok := err.(*exec.ExitError); ok {
			detail = string(exit.Stderr)
		}
		return harnessResult{}, fmt.Errorf("hook harness crashed: %v %s", err, firstLine(detail))
	}
	var result harnessResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &result); err != nil {
		return harnessResult{}, fmt.Errorf("decode hook harness output %q: %w", firstLine(string(output)), err)
	}
	return result, nil
}

// grantedInvocation extracts the provider-owned granted choice invocation.
func grantedInvocation(consent map[string]any) string {
	for _, raw := range getSlice(consent, "choices") {
		choice, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if choice["answer"] == "granted" {
			invocation, _ := choice["invocation"].(string)
			return invocation
		}
	}
	return ""
}

// providerTask returns the exact opaque Task fields Go published on one
// collect input. The cross-lane host must not infer either field from arguments
// or artifact metadata.
func providerTask(input map[string]any) (agent, prompt string, ok bool) {
	task := getMap(input, "provider_task")
	if task == nil {
		return "", "", false
	}
	agent, _ = task["agent"].(string)
	prompt, _ = task["prompt"].(string)
	return agent, prompt, agent != "" && prompt != ""
}
