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

// hostPiLane drives the REAL Pi host: the lens capture runs through the
// INSTALLED gentle-pi review-host-relay implementation (byte-identical lib
// copy) which spawns a brand-new locked-down print-mode `pi` subprocess, and
// any refuter/validator leg runs through `--execute`, where Go itself spawns
// the real pi role process. Every reviewer verdict in this lane comes out of
// a real pi process over the dev subscription.
const hostPiLane = "host-pi"

// piRelayContractEnv is the exact handshake gentle-ai requires before the pi
// runtime identity becomes eligible for immutable review transport.
var piRelayContractEnv = []string{"GENTLE_PI_REVIEW_RELAY_CONTRACT=gentle-pi.review-relay/v1"}

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

// runHostPiLane probes the real pi surface, documents the print-mode
// extension evidence, and drives one medium candidate review to the receipt.
//
// Print-mode extension note, verified against `pi --help`: pi discovers and
// loads extensions in print mode too (`--no-extensions` exists to turn that
// off, and extension-registered CLI flags appear in the help output). What
// stays out of this battery is driving gentle-pi's ORCHESTRATOR review flow
// through a live print-mode model session: that is an unbounded multi-turn
// agentic run answering consent envelopes, not a boundable lane. The bounded
// real-host coverage is this host-relay reviewer lane: the exact seam where
// gentle-pi's launcher meets the Go facade, with real pi processes at every
// reviewer, refuter, and validator leg.
func (b *battery) runHostPiLane() {
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

	repo, ok := b.hostMediumCandidate(hostPiLane, "host-pi")
	if !ok {
		return
	}
	if !b.hostNegotiatedMediumStart(hostPiLane, repo, "pi", piRelayContractEnv) {
		return
	}
	statusDoc, stderr, _ := b.statusEnv(repo, "pi", piRelayContractEnv)
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" || !hasArgument(input, "materialize") || getMap(input, "submission") == nil {
		b.fail(hostPiLane, "relay capture admitted", "no materialize+submission capture-result collect input; "+firstLine(stderr))
		return
	}
	if !b.runPiRelaySlot(hostPiLane, repo, input) {
		return
	}
	b.hostFollowToReceipt(hostPiLane, repo, "pi", piRelayContractEnv)
}

// probePiPrintModeExtensions records the probe evidence for pi's print-mode
// extension surface from `pi --help` output.
func (b *battery) probePiPrintModeExtensions() {
	output, err := exec.Command("pi", "--help").CombinedOutput()
	if err != nil {
		b.fail(hostPiLane, "print-mode extension probe", fmt.Sprintf("pi --help failed: %v", err))
		return
	}
	help := string(output)
	if strings.Contains(help, "Extension CLI Flags") && strings.Contains(help, "--no-extensions") {
		b.pass(hostPiLane, "print-mode extension probe",
			"pi loads extensions in print mode (extension-registered CLI flags in --help; --no-extensions is the opt-out); the full extension-driven review flow is an unbounded agentic session, so this lane drives the bounded host relay instead")
		return
	}
	b.skip(hostPiLane, "print-mode extension probe",
		"pi --help shows no extension-registered flags; extension loading in print mode unproven, lane drives the host relay")
}

func piRelayClosurePresent(packageDir string) bool {
	_, err := os.Stat(filepath.Join(packageDir, "lib", "review-host-relay.ts"))
	return err == nil
}

// preparePiRelayDriver copies the installed gentle-pi lib/ and scripts/
// sources byte-identical into the work directory (relocated out of
// node_modules only so Node can type-strip them) next to the battery driver.
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

// runPiRelaySlot satisfies one materialize+submission capture-result collect
// input through the installed gentle-pi relay: materialize -> real locked-down
// pi subprocess -> provider-owned submission, all owned by gentle-pi's code.
func (b *battery) runPiRelaySlot(lane, repo string, input map[string]any) bool {
	const check = "relay capture admitted"
	if b.piRelayDir == "" {
		b.fail(lane, check, "gentle-pi relay driver is not prepared")
		return false
	}
	caseConfig := map[string]any{
		"capture_argument_tokens": argumentTokens(input),
		"submission":              input["submission"],
		"gentle_ai_executable":    b.binary,
		"pi_executable":           "pi",
	}
	payload, err := json.Marshal(caseConfig)
	if err != nil {
		b.fail(lane, check, err.Error())
		return false
	}
	casePath := filepath.Join(b.piRelayDir, "case.json")
	if err := os.WriteFile(casePath, payload, 0o644); err != nil {
		b.fail(lane, check, err.Error())
		return false
	}
	b.noteHostCost(lane, "1 real pi print-mode reviewer run (gentle-pi host relay)")
	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "driver.mts", casePath)
	command.Dir = b.piRelayDir
	command.Env = append(os.Environ(), piRelayContractEnv...)
	output, err := command.Output()
	if err != nil {
		detail := ""
		if exit, ok := err.(*exec.ExitError); ok {
			detail = string(exit.Stderr)
		}
		b.fail(lane, check, fmt.Sprintf("gentle-pi relay driver failed: %v %s", err, firstLine(detail)))
		return false
	}
	var result piRelayResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &result); err != nil {
		b.fail(lane, check, fmt.Sprintf("decode relay driver output %q: %v", firstLine(string(output)), err))
		return false
	}
	manifest := b.record("result-artifact", []byte(result.Submission))
	if getString(manifest, "schema") != "gentle-ai.review-result-artifact/v2" {
		b.fail(lane, check, "relay submission did not round-trip a native result artifact")
		return false
	}
	b.pass(lane, check, fmt.Sprintf("installed gentle-pi relay ran a real pi process (%d prompt bytes -> %d result bytes) and the capture was admitted", result.PromptBytes, result.ResultBytes))
	return true
}
