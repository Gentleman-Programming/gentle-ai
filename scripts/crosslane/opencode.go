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
	Name         string   `json:"name"`
	Subagent     string   `json:"subagent"`
	BindingPairs [][2]any `json:"binding_pairs,omitempty"`
	Body         string   `json:"body,omitempty"`
	Prompt       string   `json:"prompt,omitempty"`
	TaskOutput   string   `json:"task_output"`
	SkipAfter    bool     `json:"skip_after,omitempty"`
}

// runOpenCodeLane drives the real plugin bytes through host-assembled binding
// frames against a live medium-risk lineage: consent round-trip, lens frame
// (host-faithful and Go-typed), correction flow, and validator role frame
// (host-serialized and exact relay).
func (b *battery) runOpenCodeLane() {
	repo, err := b.scratchRepo("opencode-lane")
	if err != nil {
		b.fail(openCodeLane, "scratch repository", err.Error())
		return
	}
	base := "export function greet(name) {\n  return \"hi \" + name;\n}\n"
	err = writeFile(repo, "src/greet.js", base)
	if err == nil {
		err = commitAll(repo, "feat: greet")
	}
	if err != nil {
		b.fail(openCodeLane, "scratch repository", err.Error())
		return
	}
	unsafe := base + "export function shout(name) {\n  return name.toUpperCase() + \"!\";\n}\n"
	if err := writeFile(repo, "src/greet.js", unsafe); err != nil {
		b.fail(openCodeLane, "scratch repository", err.Error())
		return
	}

	// Consent round-trip with the opencode runtime identity.
	statusDoc, stderr, code := b.status(repo, "opencode")
	target := getString(statusDoc, "target_identity")
	if target == "" {
		b.fail(openCodeLane, "negotiated status", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
		return
	}
	consent, stderr, _ := b.runJSON("consent", repo,
		"review", "start", "--contract", reviewContract, "--cwd", repo,
		"--target", target, "--projection", "workspace", "--agent", "opencode", "--consent", "relay")
	if getString(consent, "action") != "consent_required" || getString(consent, "schema") != "gentle-ai.review-integration.consent/v3" {
		b.fail(openCodeLane, "consent envelope surfaced", fmt.Sprintf("schema=%q action=%q %s", getString(consent, "schema"), getString(consent, "action"), firstLine(stderr)))
		return
	}
	granted := grantedInvocation(consent)
	if granted == "" {
		b.fail(openCodeLane, "consent envelope surfaced", "no granted choice invocation in envelope")
		return
	}
	startDoc, stderr, code := b.runCommandLine("start", repo, granted)
	if code != 0 || getString(startDoc, "state") != "reviewing" || getString(startDoc, "risk_level") != "medium" {
		b.fail(openCodeLane, "consent granted round-trip", fmt.Sprintf("exit=%d state=%q risk=%q %s", code, getString(startDoc, "state"), getString(startDoc, "risk_level"), firstLine(stderr)))
		return
	}
	b.pass(openCodeLane, "consent granted round-trip", "consent/v3 surfaced; granted invocation created a reviewing medium lineage")

	// Reviewer collect slot: this is where the host assembles the binding.
	statusDoc, stderr, _ = b.status(repo, "opencode")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(openCodeLane, "reviewer collect slot", fmt.Sprintf("no review.capture-result collect input; %s", firstLine(stderr)))
		return
	}
	args := argumentValues(input)
	node, err := b.prepareHookHarness(repo)
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

	// Host-faithful lens frame: the binding values come verbatim from the
	// collect input arguments, exactly as the orchestration contract tells a
	// host to assemble them. The provider delivers order as the string "0",
	// so a faithful host serializes "order":"0".
	hostPairs := [][2]any{
		{"lineage", args["lineage"]},
		{"target", args["target"]},
		{"lens", args["lens"]},
		{"order", args["order"]},
		{"revision", args["expected-revision"]},
		{"repository_context", args["repository-context"]},
		{"subject_hash", args["subject-hash"]},
	}
	hostCase := harnessCase{
		Name:         "lens-host-faithful",
		Subagent:     args["lens"],
		BindingPairs: hostPairs,
		Body:         "Review this frozen candidate through the assigned lens.",
		TaskOutput:   string(reviewerJSON),
	}
	hostResult, err := b.runHookCase(node, hostCase)
	lensCaptured := false
	switch {
	case err != nil:
		b.fail(openCodeLane, "lens frame: host-assembled", err.Error())
	case hostResult.AfterOK:
		lensCaptured = true
		b.pass(openCodeLane, "lens frame: host-assembled", "host-serialized binding accepted end to end (fix merged)")
		b.skip(openCodeLane, "lens frame: Go-typed control", "host frame already captured the slot; control unnecessary")
	case strings.Contains(hostResult.Error, bindingInvalid):
		b.fail(openCodeLane, "lens frame: host-assembled",
			"known-red pending fix/opencode-host-binding: Go transport rejects the host-serialized binding (order delivered as collect-argument string): "+firstLine(hostResult.Error))
	default:
		b.fail(openCodeLane, "lens frame: host-assembled", "unexpected failure: "+firstLine(hostResult.Error))
	}

	if !lensCaptured {
		// Go-typed control: identical binding but with order as a JSON number.
		// Proves the slot itself is healthy, isolating the failure above to
		// the host serialization.
		goPairs := append([][2]any(nil), hostPairs...)
		goPairs[3] = [2]any{"order", 0}
		controlResult, err := b.runHookCase(node, harnessCase{
			Name:         "lens-go-typed",
			Subagent:     args["lens"],
			BindingPairs: goPairs,
			Body:         "Review this frozen candidate through the assigned lens.",
			TaskOutput:   string(reviewerJSON),
		})
		switch {
		case err != nil:
			b.fail(openCodeLane, "lens frame: Go-typed control", err.Error())
			return
		case !controlResult.AfterOK:
			b.fail(openCodeLane, "lens frame: Go-typed control", firstLine(controlResult.Error))
			return
		case !strings.HasPrefix(controlResult.ChildPrompt, "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION "):
			b.fail(openCodeLane, "lens frame: Go-typed control", "child prompt is not the Go-issued materialization")
			return
		default:
			manifest := b.record("result-artifact", []byte(controlResult.Output))
			if getString(manifest, "schema") != "gentle-ai.review-result-artifact/v2" || getString(manifest, "admission_decision") != "completed" {
				b.fail(openCodeLane, "lens frame: Go-typed control", "completion did not round-trip a completed result artifact")
				return
			}
			b.pass(openCodeLane, "lens frame: Go-typed control", "session started, child received Go-canonical bytes, completion round-tripped")
		}
	}

	// Correction flow to reach a live validator role slot.
	if !b.driveCorrectionToValidation(repo, base) {
		return
	}

	statusDoc, stderr, _ = b.status(repo, "opencode")
	input = collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "external.run_provider_role" {
		b.fail(openCodeLane, "validator role slot", fmt.Sprintf("no provider role collect input; %s", firstLine(stderr)))
		return
	}
	providerPrompt := getString(input, "provider_task", "prompt")
	validationRequest := getMap(statusDoc, "validation_request")
	if providerPrompt == "" || validationRequest == nil {
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
		Subagent:   "review-validator",
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
		Subagent:   "review-validator",
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

	// Host-serialized role frame: same semantic binding, re-serialized by the
	// host (sorted keys). The Go transport currently requires the byte-exact
	// provider-issued prompt.
	hostRolePrompt, err := reserializeBindingLine(providerPrompt)
	if err != nil {
		b.fail(openCodeLane, "validator frame: host-serialized", err.Error())
		return
	}
	validatorCaptured := false
	hostRole, err := b.runHookCase(node, harnessCase{
		Name:       "validator-host-serialized",
		Subagent:   "review-validator",
		Prompt:     hostRolePrompt,
		TaskOutput: string(validatorJSON),
	})
	switch {
	case err != nil:
		b.fail(openCodeLane, "validator frame: host-serialized", err.Error())
	case hostRole.AfterOK:
		validatorCaptured = true
		b.pass(openCodeLane, "validator frame: host-serialized", "host-serialized role binding accepted end to end (fix merged)")
		b.skip(openCodeLane, "validator frame: exact relay control", "host frame already captured the slot; control unnecessary")
	case strings.Contains(hostRole.Error, bindingInvalid):
		b.fail(openCodeLane, "validator frame: host-serialized",
			"known-red pending fix/opencode-host-binding: Go transport requires byte-exact provider prompt; host re-serialization refused: "+firstLine(hostRole.Error))
	default:
		b.fail(openCodeLane, "validator frame: host-serialized", "unexpected failure: "+firstLine(hostRole.Error))
	}

	if !validatorCaptured {
		exact, err := b.runHookCase(node, harnessCase{
			Name:       "validator-exact-relay",
			Subagent:   "review-validator",
			Prompt:     providerPrompt,
			TaskOutput: string(validatorJSON),
		})
		switch {
		case err != nil:
			b.fail(openCodeLane, "validator frame: exact relay control", err.Error())
			return
		case !exact.AfterOK:
			b.fail(openCodeLane, "validator frame: exact relay control", firstLine(exact.Error))
			return
		default:
			roleResult := b.record("provider-role", []byte(exact.Output))
			if captured, _ := roleResult["captured"].(bool); !captured {
				b.fail(openCodeLane, "validator frame: exact relay control", "role completion did not report captured=true")
				return
			}
			b.pass(openCodeLane, "validator frame: exact relay control", "exact Go-issued role prompt round-tripped and captured")
		}
	}

	// Terminal: finalize the captured validation to the approved receipt.
	statusDoc, stderr, _ = b.status(repo, "opencode")
	command := getString(statusDoc, "next_transition", "execute", "command")
	if getString(statusDoc, "next_transition", "execute", "operation") != "review.finalize" || command == "" {
		b.fail(openCodeLane, "correction lifecycle approved", fmt.Sprintf("expected finalize transition, got %s/%s %s",
			getString(statusDoc, "next_transition", "kind"), getString(statusDoc, "next_transition", "reason_code"), firstLine(stderr)))
		return
	}
	finalize, stderr, code := b.runCommandLine("operation", repo, command)
	if code != 0 || operationState(finalize) != "approved" {
		b.fail(openCodeLane, "correction lifecycle approved", fmt.Sprintf("exit=%d state=%q %s", code, operationState(finalize), firstLine(stderr)))
		return
	}
	b.pass(openCodeLane, "correction lifecycle approved", "plan, fix, evidence, and captured validation reached the approved receipt")
}

// driveCorrectionToValidation walks finalize -> correction plan -> fix edit ->
// evidence capture, leaving the lineage waiting on targeted validation.
func (b *battery) driveCorrectionToValidation(repo, fixedBase string) bool {
	statusDoc, stderr, _ := b.status(repo, "opencode")
	command := getString(statusDoc, "next_transition", "execute", "command")
	if getString(statusDoc, "next_transition", "execute", "operation") != "review.finalize" || command == "" {
		b.fail(openCodeLane, "correction: finalize captured results", fmt.Sprintf("expected finalize transition; %s", firstLine(stderr)))
		return false
	}
	finalize, stderr, code := b.runCommandLine("operation", repo, command)
	if code != 0 || operationState(finalize) != "correction_required" {
		b.fail(openCodeLane, "correction: finalize captured results", fmt.Sprintf("exit=%d state=%q %s", code, operationState(finalize), firstLine(stderr)))
		return false
	}

	// Correction plan forecast is submitted BEFORE editing.
	statusDoc, stderr, _ = b.status(repo, "opencode")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "external.plan_correction" {
		b.fail(openCodeLane, "correction: plan forecast", fmt.Sprintf("no plan_correction collect input; %s", firstLine(stderr)))
		return false
	}
	tokens := substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"value": "2"})
	planArgs := append([]string{"review", getString(input, "submission", "operation_token")}, tokens...)
	plan, stderr, code := b.runJSON("operation", b.workRoot, planArgs...)
	if code != 0 || operationState(plan) != "correction_required" {
		b.fail(openCodeLane, "correction: plan forecast", fmt.Sprintf("exit=%d state=%q %s", code, operationState(plan), firstLine(stderr)))
		return false
	}

	// Bounded fix edit.
	fixed := fixedBase + "export function shout(name) {\n  if (name == null) return \"!\";\n  return name.toUpperCase() + \"!\";\n}\n"
	if err := writeFile(repo, "src/greet.js", fixed); err != nil {
		b.fail(openCodeLane, "correction: bounded fix edit", err.Error())
		return false
	}

	// Correction verification evidence.
	statusDoc, stderr, _ = b.status(repo, "opencode")
	input = collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-evidence" {
		b.fail(openCodeLane, "correction: evidence capture", fmt.Sprintf("no capture-evidence collect input; %s", firstLine(stderr)))
		return false
	}
	evidencePath := filepath.Join(b.workRoot, "opencode-evidence.txt")
	evidence := fmt.Sprintf("crosslane battery %s: node --check src/greet.js passed; shout(null) now returns \"!\" instead of throwing\n", timestamp())
	if err := os.WriteFile(evidencePath, []byte(evidence), 0o644); err != nil {
		b.fail(openCodeLane, "correction: evidence capture", err.Error())
		return false
	}
	tokens = substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"outcome": "passed", "input": evidencePath})
	captureArgs := append([]string{"review", getString(input, "submission", "operation_token")}, tokens...)
	record, stderr, code := b.runJSON("verification-evidence", b.workRoot, captureArgs...)
	if code != 0 || getString(record, "outcome") != "passed" {
		b.fail(openCodeLane, "correction: evidence capture", fmt.Sprintf("exit=%d outcome=%q %s", code, getString(record, "outcome"), firstLine(stderr)))
		return false
	}
	b.pass(openCodeLane, "correction: plan, fix, evidence", "forecast before edit, bounded fix, and passed evidence captured")
	return true
}

// prepareHookHarness materializes the node harness directory: the REAL plugin
// bytes, the hook emulator, and a PATH shim so the plugin's spawn("gentle-ai")
// resolves to the binary under test.
func (b *battery) prepareHookHarness(repo string) (string, error) {
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
	_ = repo
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
	configPath := filepath.Join(harnessDir, c.Name+".case.json")
	payload, err := json.Marshal(c)
	if err != nil {
		return harnessResult{}, err
	}
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		return harnessResult{}, err
	}
	command := exec.Command("node", "harness.mts", configPath)
	command.Dir = harnessDir
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

// reserializeBindingLine re-serializes the role binding line of a Go-issued
// provider task prompt with the host's own JSON encoding (sorted keys),
// preserving the binding semantics byte-for-byte at the field level.
func reserializeBindingLine(prompt string) (string, error) {
	line, rest, hasRest := strings.Cut(prompt, "\n")
	const header = "GENTLE_AI_REVIEW_PROVIDER_TASK "
	encoded, found := strings.CutPrefix(line, header)
	if !found {
		return "", fmt.Errorf("provider task prompt has no role binding header")
	}
	var binding map[string]any
	if err := json.Unmarshal([]byte(encoded), &binding); err != nil {
		return "", fmt.Errorf("decode role binding: %w", err)
	}
	reserialized, err := json.Marshal(binding) // Go maps marshal with sorted keys
	if err != nil {
		return "", err
	}
	if string(reserialized) == encoded {
		return "", fmt.Errorf("re-serialized role binding is byte-identical; perturbation void")
	}
	out := header + string(reserialized)
	if hasRest {
		out += "\n" + rest
	}
	return out, nil
}
