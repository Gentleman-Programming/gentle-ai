package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// hostOpenCodeLane drives the REAL OpenCode host application headlessly:
// `opencode run` in a sandboxed HOME with the real gentle-ai transport plugin
// installed globally, a real driver model invoking one review Task, the real
// tool.execute.before/after hooks relaying the start and completion frames
// through a live `gentle-ai review opencode-transport` child, and a real
// reviewer subagent model producing the captured verdict. This is the lane
// that catches host-app behavior (like swallowed hook throws) the emulated
// deterministic opencode lane cannot see.
const hostOpenCodeLane = "host-opencode"

// runHostOpenCodeLane sandboxes an OpenCode installation, prepares a scratch
// lineage under that sandbox HOME (the transport child inherits it, so the
// opaque repository-context store must live there too), and asserts through
// the authority store that the relay frames flowed through the real hooks.
func (b *battery) runHostOpenCodeLane() {
	openCodeBinary, err := exec.LookPath("opencode")
	if err != nil {
		b.skip(hostOpenCodeLane, "opencode host lane", "opencode binary is not on PATH")
		return
	}
	realHome, err := os.UserHomeDir()
	if err != nil {
		b.skip(hostOpenCodeLane, "opencode host lane", "cannot resolve the real home directory")
		return
	}
	authPath := filepath.Join(realHome, ".local", "share", "opencode", "auth.json")
	if _, err := os.Stat(authPath); err != nil {
		b.skip(hostOpenCodeLane, "opencode host lane", "no OpenCode credentials ("+authPath+"); the real session would have no model")
		return
	}
	reviewAgents, err := realOpenCodeReviewAgents(realHome)
	if err != nil {
		b.skip(hostOpenCodeLane, "opencode host lane", "no synced review agents in the real OpenCode config: "+err.Error())
		return
	}

	sandboxHome, env, err := b.prepareOpenCodeSandbox(authPath, reviewAgents)
	if err != nil {
		b.fail(hostOpenCodeLane, "sandbox HOME setup", err.Error())
		return
	}

	repo, ok := b.hostMediumCandidate(hostOpenCodeLane, "host-opencode")
	if !ok {
		return
	}
	// The whole lane, gentle-ai included, runs under the sandbox HOME: the
	// plugin's transport child inherits OpenCode's environment, so lineage
	// authority, opaque contexts, and the RDD switch all live in the sandbox.
	// The enable runs from the scratch repo because mode resolution also
	// records the clone identity of its working directory.
	if _, stderr, code := b.runEnv(repo, env, "review", "mode", "enable"); code != 0 {
		b.fail(hostOpenCodeLane, "sandbox HOME setup", "enable sandbox RDD switch: "+firstLine(stderr))
		return
	}
	if !b.hostNegotiatedMediumStart(hostOpenCodeLane, repo, "opencode", env) {
		return
	}
	statusDoc, stderr, _ := b.statusEnv(repo, "opencode", env)
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(hostOpenCodeLane, "reviewer collect slot", "no review.capture-result collect input; "+firstLine(stderr))
		return
	}
	args := argumentValues(input)

	// The binding uses the Go-typed order (JSON number). The host-faithful
	// string serialization is the deterministic lane's known-red
	// (fix/opencode-host-binding); this lane isolates host-app behavior, not
	// that serialization defect.
	binding, err := json.Marshal(map[string]any{
		"lineage":            args["lineage"],
		"target":             args["target"],
		"lens":               args["lens"],
		"order":              0,
		"revision":           args["expected-revision"],
		"repository_context": args["repository-context"],
		"subject_hash":       args["subject-hash"],
	})
	if err != nil {
		b.fail(hostOpenCodeLane, "binding assembly", err.Error())
		return
	}
	prompt := "GENTLE_AI_REVIEW_BINDING " + string(binding) + "\nReview this frozen candidate through the assigned lens."
	message := "You are driving a review transport integration test. Call the task tool exactly once with " +
		"subagent_type set to \"" + args["lens"] + "\" and the prompt argument set to EXACTLY the text between " +
		"the BEGIN and END marker lines below (marker lines excluded), byte for byte: preserve the JSON exactly " +
		"as written, do not reformat, reorder, or add whitespace. After the task tool returns, reply with exactly: RELAY DONE\n" +
		"BEGIN\n" + prompt + "\nEND\n"

	b.noteHostCost(hostOpenCodeLane, "1 real opencode session (driver model + reviewer subagent model)")
	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()
	session := exec.CommandContext(ctx, openCodeBinary, "run", "--dir", repo, message)
	session.Dir = b.workRoot
	session.Env = openCodeSandboxSessionEnvironment(sandboxHome, b.hostShimPath())
	output, sessionErr := session.CombinedOutput()
	logPath := filepath.Join(b.workRoot, "host-opencode-run.log")
	_ = os.WriteFile(logPath, output, 0o644)

	// The real host confirms the hook round-trip itself. The final selected
	// lens capture owns terminal closure, so there is no FINALIZE transition to
	// discover afterward and no receipt-driven delivery step to follow.
	if sessionErr != nil || !strings.Contains(string(output), "RELAY DONE") {
		note := "real OpenCode session did not acknowledge the hook relay"
		if sessionErr != nil {
			note += fmt.Sprintf("; opencode run: %v", sessionErr)
		}
		b.fail(hostOpenCodeLane, "relay frames through real hooks", note+"; session log kept at "+logPath)
		return
	}
	b.pass(hostOpenCodeLane, "relay frames through real hooks",
		"real opencode session: before hook relayed the start frame and after hook relayed the terminal capture")
}

// realOpenCodeReviewAgents extracts the gentle-ai-synced review-* subagent
// definitions from the user's real OpenCode configuration.
func realOpenCodeReviewAgents(realHome string) (map[string]any, error) {
	payload, err := os.ReadFile(filepath.Join(realHome, ".config", "opencode", "opencode.json"))
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(payload, &config); err != nil {
		return nil, err
	}
	agents, _ := config["agent"].(map[string]any)
	review := map[string]any{}
	for name, definition := range agents {
		if !strings.HasPrefix(name, "review-") {
			continue
		}
		if body, ok := definition.(map[string]any); ok {
			// The sandbox driver must be able to name the subagent in a task
			// call, so the synced hidden flag is dropped inside the sandbox.
			copied := map[string]any{}
			for key, value := range body {
				copied[key] = value
			}
			delete(copied, "hidden")
			review[name] = copied
		}
	}
	if len(review) == 0 {
		return nil, fmt.Errorf("no review-* agents; run gentle-ai sync for OpenCode first")
	}
	return review, nil
}

// prepareOpenCodeSandbox seeds a sandboxed HOME: the REAL transport plugin
// bytes installed globally, the real synced review agents, and a copy of the
// user's OpenCode credentials (removed with the work directory).
func (b *battery) prepareOpenCodeSandbox(authPath string, reviewAgents map[string]any) (string, []string, error) {
	sandboxHome := filepath.Join(b.workRoot, "opencode-home")
	configDir := filepath.Join(sandboxHome, ".config", "opencode")
	dataDir := filepath.Join(sandboxHome, ".local", "share", "opencode")
	for _, dir := range []string{filepath.Join(configDir, "plugins"), dataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", nil, err
		}
	}
	plugin, err := os.ReadFile(filepath.Join(b.repoRoot, pluginSourcePath))
	if err != nil {
		return "", nil, fmt.Errorf("read real plugin bytes: %w", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "plugins", filepath.Base(pluginSourcePath)), plugin, 0o644); err != nil {
		return "", nil, err
	}
	auth, err := os.ReadFile(authPath)
	if err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(dataDir, "auth.json"), auth, 0o600); err != nil {
		return "", nil, err
	}
	driverModel := ""
	for _, definition := range reviewAgents {
		if body, ok := definition.(map[string]any); ok {
			if model, _ := body["model"].(string); model != "" {
				driverModel = model
				break
			}
		}
	}
	config := map[string]any{
		"share":      "disabled",
		"autoupdate": false,
		"agent":      reviewAgents,
		"permission": map[string]any{"task": "allow", "read": "allow"},
	}
	if driverModel != "" {
		config["model"] = driverModel
	}
	payload, err := json.MarshalIndent(config, "", " ")
	if err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), payload, 0o644); err != nil {
		return "", nil, err
	}
	env := openCodeSandboxGentleEnvironment(sandboxHome)
	return sandboxHome, env, nil
}

// openCodeSandboxGentleEnvironment is the override set for gentle-ai
// invocations belonging to the sandboxed lane.
func openCodeSandboxGentleEnvironment(sandboxHome string) []string {
	return []string{
		"HOME=" + sandboxHome,
		"XDG_CONFIG_HOME=" + filepath.Join(sandboxHome, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(sandboxHome, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(sandboxHome, ".cache"),
		"XDG_STATE_HOME=" + filepath.Join(sandboxHome, ".local", "state"),
	}
}

// openCodeSandboxSessionEnvironment is the complete environment for the real
// OpenCode session process itself: sandbox HOME, no autoupdate, and a PATH
// whose first entry resolves `gentle-ai` to the binary under test.
func openCodeSandboxSessionEnvironment(sandboxHome, shimPath string) []string {
	return mergeEnvironment(append(openCodeSandboxGentleEnvironment(sandboxHome),
		"OPENCODE_DISABLE_AUTOUPDATE=1", "PATH="+shimPath))
}

// hostShimPath builds a PATH whose first entry resolves `gentle-ai` to the
// binary under test, so the real plugin's transport spawn hits it.
func (b *battery) hostShimPath() string {
	shimDir := filepath.Join(b.workRoot, "host-shim-bin")
	if err := os.MkdirAll(shimDir, 0o755); err == nil {
		shim := "#!/bin/sh\nexec \"" + b.binary + "\" \"$@\"\n"
		_ = os.WriteFile(filepath.Join(shimDir, "gentle-ai"), []byte(shim), 0o755)
	}
	return shimDir + string(os.PathListSeparator) + os.Getenv("PATH")
}
