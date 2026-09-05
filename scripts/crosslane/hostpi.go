package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed pirelay.mts
var piRelayDriver string

const hostPiLane = "host-pi"

func piReviewEnvironment(operatorHome, operatorPath string) ([]string, string) {
	if operatorHome == "" {
		return nil, "operator HOME is empty; refusing to launch the Pi relay without an explicit runtime locator"
	}
	if operatorPath == "" {
		return nil, "operator PATH is empty; refusing to launch the Pi relay without an explicit runtime locator"
	}
	agentDir := os.Getenv("PI_CODING_AGENT_DIR")
	if agentDir == "" {
		agentDir = filepath.Join(operatorHome, ".pi", "agent")
	}
	return []string{"HOME=" + operatorHome, "PATH=" + operatorPath, "PI_CODING_AGENT_DIR=" + agentDir, "GENTLE_PI_REVIEW_RELAY_CONTRACT=gentle-pi.review-relay/v1"}, ""
}

// piPackageDir returns the installed gentle-pi package root.
func piPackageDir() string {
	if override := os.Getenv("CROSSLANE_GENTLE_PI_DIR"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "npm", "node_modules", "gentle-pi")
}

func (b *battery) runHostPiLane() {
	if b.piEnvironmentErr != "" {
		b.fail(hostPiLane, "Pi runtime environment", b.piEnvironmentErr)
		return
	}
	if _, err := exec.LookPath("pi"); err != nil {
		b.skip(hostPiLane, "pi host lane", "pi binary is not on PATH")
		return
	}
	if _, err := exec.LookPath("node"); err != nil {
		b.skip(hostPiLane, "pi host lane", "node is unavailable for the gentle-pi relay driver")
		return
	}
	packageDir := piPackageDir()
	if packageDir == "" || !piRelayClosurePresent(packageDir) {
		b.skip(hostPiLane, "pi host lane", "installed gentle-pi package with lib/review-host-relay.ts not found under "+packageDir)
		return
	}

	b.probePiPrintModeExtensions()

	relayDir, err := b.preparePiRelayDriver(packageDir)
	if err != nil {
		b.fail(hostPiLane, "gentle-pi relay driver setup", err.Error())
		return
	}
	b.piRelayDir = relayDir

	repo, ok := b.hostPiCorrectionCandidate()
	if !ok {
		return
	}
	if !b.hostNegotiatedStart(hostPiLane, repo, "pi", b.piEnvironment, "high") {
		return
	}
	if !b.hostPiCaptureReviewers(repo) {
		return
	}
	if !b.hostPiDriveCorrectionToValidation(repo) {
		return
	}
	b.hostFollowToReceipt(hostPiLane, repo, "pi", b.piEnvironment)
	if _, active := b.lineages[repo]; active {
		b.fail(hostPiLane, "correction to validator closure", "Pi review did not close through review.capture-validation")
		return
	}
	b.pass(hostPiLane, "correction to validator closure", "Pi correction reached validator closure")
}

func (b *battery) hostPiCaptureReviewers(repo string) bool {
	const check = "relay reviewer sequence"
	fail := func(note string) bool { b.fail(hostPiLane, check, note); return false }
	seen := map[string]bool{}
	for status, stderr, code := b.statusEnv(repo, "pi", b.piEnvironment); ; status, stderr, code = b.statusEnv(repo, "pi", b.piEnvironment) {
		reason := getString(status, "next_transition", "reason_code")
		if code != 0 || reason == "" {
			return fail(fmt.Sprintf("status after relay capture exit=%d %s", code, firstLine(stderr)))
		}
		if reason == "correction_plan_required" {
			return true
		}
		if reason != "reviewer_results_required" {
			return fail("unexpected status transition " + reason)
		}
		input, slot := piUnadmittedReviewerInput(status, seen)
		if input == nil || slot == "" || !hasArgument(input, "materialize") || getMap(input, "submission") == nil {
			return fail("STATUS reoffered no unadmitted materialized review.capture-result slot")
		}
		capture, admitted := b.runPiRelaySlot(hostPiLane, repo, input)
		if !admitted {
			return false
		}
		seen[slot] = true
		if operationState(capture) == "correction_required" {
			return b.hostCorrectionReentry(hostPiLane, "lifecycle correction re-entry", repo, b.piEnvironment, capture)
		}
	}
}

func piUnadmittedReviewerInput(status map[string]any, seen map[string]bool) (map[string]any, string) {
	for _, raw := range getSlice(status, "next_transition", "collect", "inputs") {
		input, _ := raw.(map[string]any)
		if input == nil || input["capture_operation"] != "review.capture-result" {
			continue
		}
		args := argumentValues(input)
		slot := strings.Join([]string{args["order"], args["lens"], args["subject-hash"]}, "\x00")
		if args["order"] == "" || args["lens"] == "" || args["subject-hash"] == "" || !seen[slot] {
			return input, slot
		}
	}
	return nil, ""
}

const hostPiCorrectedModule = "export function authorize(token) { return token === \"trusted-token\" }\n"

func (b *battery) hostPiCorrectionCandidate() (string, bool) {
	fail := func(name, note string) (string, bool) { b.fail(hostPiLane, name, note); return "", false }
	repo, err := b.scratchRepo("host-pi")
	if err != nil {
		return fail("scratch repository", err.Error())
	}
	if err = writeFile(repo, "auth/authorize.mjs", hostPiCorrectedModule); err == nil {
		err = writeFile(repo, "test/authorize.test.mjs", `import assert from "node:assert/strict"; import { authorize } from "../auth/authorize.mjs"; assert.equal(authorize("trusted-token"), true); assert.equal(authorize("invalid-token"), false);`)
	}
	if err == nil {
		err = commitAll(repo, "feat: add token authorization")
	}
	if err != nil {
		return fail("scratch repository", err.Error())
	}
	if err := runHostPiScratchTest(repo); err != nil {
		return fail("authorization expectations before bypass", err.Error())
	}
	b.pass(hostPiLane, "authorization expectations before bypass", "committed base passed node --test test/authorize.test.mjs with trusted token authorized and invalid token denied")
	if err := writeFile(repo, "auth/authorize.mjs", strings.Replace(hostPiCorrectedModule, `return token === "trusted-token"`, "return true", 1)); err != nil {
		return fail("scratch repository", err.Error())
	}
	if err := runHostPiScratchTest(repo); err == nil {
		return fail("authorization expectations reject bypass", "node --test test/authorize.test.mjs unexpectedly passed after authorize() bypass returned true")
	}
	b.pass(hostPiLane, "authorization expectations reject bypass", "unchanged committed invalid-token denial failed after only authorize() changed from token validation to return true")
	return repo, true
}
func runHostPiScratchTest(repo string) error {
	command := exec.Command("node", "--test", "test/authorize.test.mjs")
	command.Dir = repo
	return command.Run()
}
func (b *battery) hostPiDriveCorrectionToValidation(repo string) bool {
	const check = "Pi correction plan and validator vector"
	fail := func(note string) bool { b.fail(hostPiLane, check, note); return false }
	status, stderr, code := b.statusEnv(repo, "pi", b.piEnvironment)
	input := collectInput(status)
	if code != 0 || getString(status, "next_transition", "reason_code") != "correction_plan_required" || input == nil || input["capture_operation"] != "review.capture-correction-plan" {
		return fail(fmt.Sprintf("expected correction-plan collect input: exit=%d reason=%q operation=%q %s",
			code, getString(status, "next_transition", "reason_code"), getString(input, "capture_operation"), firstLine(stderr)))
	}
	planTokens := substituteTokens(getSlice(input, "submission", "argument_tokens"), map[string]string{"value": "2"})
	plan, stderr, code := b.runJSONEnv("operation", repo, b.piEnvironment,
		append([]string{"review", getString(input, "submission", "operation_token")}, planTokens...)...)
	if code != 0 || operationState(plan) != "correction_required" {
		return fail(fmt.Sprintf("exact correction-plan vector exit=%d state=%q %s", code, operationState(plan), firstLine(stderr)))
	}
	if err := writeFile(repo, "auth/authorize.mjs", hostPiCorrectedModule); err != nil {
		return fail("apply in-scope correction: " + err.Error())
	}
	if err := runHostPiScratchTest(repo); err != nil {
		return fail("corrected authorization expectations: " + err.Error())
	}
	b.pass(hostPiLane, check, "exact correction-plan vector forecast one deletion plus one addition; restoring authorize(token) to trusted-token validation made the unchanged security test pass")
	status, stderr, code = b.statusEnv(repo, "pi", b.piEnvironment)
	input = collectInput(status)
	if code != 0 || getString(status, "next_transition", "reason_code") != "targeted_validation_required" || input == nil || input["capture_operation"] != "review.capture-validation" || !hasArgument(input, "agent") || !hasArgument(input, "execute") {
		return fail(fmt.Sprintf("expected targeted-validator vector: exit=%d reason=%q operation=%q %s",
			code, getString(status, "next_transition", "reason_code"), getString(input, "capture_operation"), firstLine(stderr)))
	}
	vector := argumentTokens(input)
	wrong := append([]string(nil), vector...)
	for index, token := range wrong {
		if strings.HasPrefix(token, "--request-hash=") {
			wrong[index] = "--request-hash=sha256:" + strings.Repeat("0", 64)
			break
		}
	}
	if strings.Join(wrong, "\x00") == strings.Join(vector, "\x00") {
		return fail("targeted-validator vector omitted request-hash")
	}
	if _, rejection, rejected := b.runJSONEnv("validator-unconfirmed", repo, b.piEnvironment,
		append([]string{"review", "capture-validation"}, wrong...)...); rejected == 0 {
		return fail("wrong validator binding unexpectedly succeeded: " + firstLine(rejection))
	}
	reoffered, reofferStderr, reofferCode := b.statusEnv(repo, "pi", b.piEnvironment)
	reofferedInput := collectInput(reoffered)
	if reofferCode != 0 || getString(reoffered, "next_transition", "reason_code") != "targeted_validation_required" ||
		reofferedInput == nil || strings.Join(argumentTokens(reofferedInput), "\x00") != strings.Join(vector, "\x00") {
		return fail(fmt.Sprintf("unconfirmed validator capture lost its exact live slot: exit=%d reason=%q %s",
			reofferCode, getString(reoffered, "next_transition", "reason_code"), firstLine(reofferStderr)))
	}
	b.pass(hostPiLane, check, "a rejected unconfirmed validator vector retained the same lineage and exact reoffered capture; no fresh review was authorized")
	return true
}

func (b *battery) probePiPrintModeExtensions() {
	output, err := exec.Command("pi", "--help").CombinedOutput()
	if err != nil {
		b.fail(hostPiLane, "print-mode extension probe", fmt.Sprintf("pi --help failed: %v", err))
		return
	}
	help := string(output)
	if strings.Contains(help, "Extension CLI Flags") && strings.Contains(help, "--no-extensions") {
		b.pass(hostPiLane, "print-mode extension probe", "pi loads extensions in print mode")
		return
	}
	b.skip(hostPiLane, "print-mode extension probe", "extension loading in print mode unproven")
}

func piRelayClosurePresent(packageDir string) bool {
	_, err := os.Stat(filepath.Join(packageDir, "lib", "review-host-relay.ts"))
	return err == nil
}

func (b *battery) preparePiRelayDriver(packageDir string) (string, error) {
	relayDir := filepath.Join(b.workRoot, "gentle-pi-relay")
	for _, sub := range []string{"lib", "scripts"} {
		if err := os.MkdirAll(filepath.Join(relayDir, sub), 0o755); err != nil {
			return "", err
		}
		entries, err := os.ReadDir(filepath.Join(packageDir, sub))
		if err != nil {
			return "", fmt.Errorf("read installed gentle-pi %s/: %w", sub, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			payload, err := os.ReadFile(filepath.Join(packageDir, sub, entry.Name()))
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(relayDir, sub, entry.Name()), payload, 0o644); err != nil {
				return "", err
			}
		}
	}
	if err := os.WriteFile(filepath.Join(relayDir, "driver.mts"), []byte(piRelayDriver), 0o644); err != nil {
		return "", err
	}
	return relayDir, nil
}

type piRelayResult struct {
	PromptBytes int    `json:"prompt_bytes"`
	ResultBytes int    `json:"result_bytes"`
	Submission  string `json:"submission"`
}

func (b *battery) runPiRelaySlot(lane, repo string, input map[string]any) (map[string]any, bool) {
	const check = "relay capture admitted"
	fail := func(note string) (map[string]any, bool) { b.fail(lane, check, note); return nil, false }
	if b.piRelayDir == "" {
		return fail("gentle-pi relay driver is not prepared")
	}
	caseConfig := map[string]any{
		"capture_argument_tokens": argumentTokens(input),
		"submission":              input["submission"],
		"gentle_ai_executable":    b.binary,
		"pi_executable":           "pi",
		"target_cwd":              repo,
	}
	payload, err := json.Marshal(caseConfig)
	if err != nil {
		return fail(err.Error())
	}
	casePath := filepath.Join(b.piRelayDir, "case.json")
	if err := os.WriteFile(casePath, payload, 0o644); err != nil {
		return fail(err.Error())
	}
	b.noteHostCost(lane, "1 real pi print-mode reviewer run (gentle-pi host relay)")
	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "driver.mts", casePath)
	command.Dir = b.piRelayDir
	command.Env = b.piEnvironment
	output, err := command.Output()
	if err != nil {
		detail := ""
		if exit, ok := err.(*exec.ExitError); ok {
			detail = string(exit.Stderr)
		}
		return fail(fmt.Sprintf("gentle-pi relay driver failed: %v %s", err, strings.TrimSpace(detail)))
	}
	var result piRelayResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &result); err != nil {
		return fail(fmt.Sprintf("decode relay driver output %q: %v", firstLine(string(output)), err))
	}
	capture := b.record("result-artifact", []byte(result.Submission))
	if !admittedCapture(capture) {
		return fail(fmt.Sprintf("relay submission was not an admitted native capture (schema=%q state=%q)", getString(capture, "schema"), operationState(capture)))
	}
	b.pass(lane, check, fmt.Sprintf("installed gentle-pi relay ran a real pi process (%d prompt bytes -> %d result bytes) and the capture was admitted", result.PromptBytes, result.ResultBytes))
	return capture, true
}
