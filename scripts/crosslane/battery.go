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
)

// commandTimeout bounds every non-host command: the --with-model lane drives
// a real reviewer underneath, so a hung provider must fail the lane with a
// diagnostic instead of hanging the battery forever.
const commandTimeout = 20 * time.Minute

const (
	statusPass = "PASS"
	statusFail = "FAIL"
	statusSkip = "SKIP"

	reviewContract = "gentle-ai.review-integration/v2"
)

type check struct {
	Lane   string
	Name   string
	Status string
	Note   string
}

type capturedEnvelope struct {
	Source string
	Schema string
	Body   []byte
}

type battery struct {
	binary    string
	repoRoot  string
	workRoot  string
	withModel bool
	withHost  bool

	// sandboxHome is the battery's own HOME for every deterministic-lane
	// invocation. The lanes used to inherit the operator's real HOME, which
	// silently made their results depend on that machine's global review mode.
	// Receipt-driven development is opt-in, so on a machine nobody configured
	// every lifecycle lane would be refused at start; on the maintainer's own
	// machine it would pass. Owning the HOME makes the battery answer the same
	// way everywhere, and keeps it from ever writing to the operator's state.
	sandboxHome string

	envelopes  []capturedEnvelope
	checks     []check
	hostCosts  []string
	piRelayDir string
}

func (b *battery) pass(lane, name, note string) {
	b.checks = append(b.checks, check{lane, name, statusPass, note})
}
func (b *battery) fail(lane, name, note string) {
	b.checks = append(b.checks, check{lane, name, statusFail, note})
}
func (b *battery) skip(lane, name, note string) {
	b.checks = append(b.checks, check{lane, name, statusSkip, note})
}

// record captures one emitted envelope for the schema conformance lane.
func (b *battery) record(source string, body []byte) map[string]any {
	doc := map[string]any{}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil
	}
	schema, _ := doc["schema"].(string)
	if schema != "" {
		b.envelopes = append(b.envelopes, capturedEnvelope{Source: source, Schema: schema, Body: append([]byte(nil), body...)})
	}
	return doc
}

// run executes the binary under test and returns stdout, stderr, and exit code.
func (b *battery) run(dir string, args ...string) (string, string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, b.binary, args...)
	command.WaitDelay = 30 * time.Second
	command.Dir = dir
	if b.sandboxHome != "" {
		command.Env = mergeEnvironment([]string{"HOME=" + b.sandboxHome, "USERPROFILE=" + b.sandboxHome})
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
			return stdout.String(), fmt.Sprintf("timed out after %s: %s", commandTimeout, stderr.String()), code
		}
	}
	return stdout.String(), stderr.String(), code
}

// runJSON executes the binary and records + decodes its JSON stdout document.
func (b *battery) runJSON(source, dir string, args ...string) (map[string]any, string, int) {
	stdout, stderr, code := b.run(dir, args...)
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, stderr, code
	}
	doc := b.record(source, []byte(trimmed))
	return doc, stderr, code
}

// status queries the negotiated next transition for one repository.
func (b *battery) status(repo, agent string, extra ...string) (map[string]any, string, int) {
	args := []string{"review", "status", "--cwd", repo, "--contract", reviewContract, "--agent", agent, "--next-transition"}
	args = append(args, extra...)
	return b.runJSON("status", repo, args...)
}

// runCommandLine splits a provider-rendered command string and executes it
// verbatim through the binary under test, from the given directory.
func (b *battery) runCommandLine(source, dir, command string) (map[string]any, string, int) {
	fields := strings.Fields(command)
	if len(fields) < 2 || fields[0] != "gentle-ai" {
		return nil, fmt.Sprintf("unexpected provider command %q", command), 1
	}
	return b.runJSON(source, dir, fields[1:]...)
}

// scratchRepo creates one initialized scratch git repository.
func (b *battery) scratchRepo(name string) (string, error) {
	dir := filepath.Join(b.workRoot, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "crosslane@example.com"},
		{"config", "user.name", "Cross Lane Battery"},
		{"commit", "-q", "--allow-empty", "-m", "chore: root"},
	} {
		if err := runGit(dir, args...); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func runGit(dir string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = dir
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

func commitAll(dir, message string) error {
	if err := runGit(dir, "add", "-A"); err != nil {
		return err
	}
	return runGit(dir, "commit", "-q", "-m", message)
}

func writeFile(dir, name, content string) error {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// --- generic JSON navigation helpers ---

func getMap(doc map[string]any, path ...string) map[string]any {
	current := doc
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func getString(doc map[string]any, path ...string) string {
	if len(path) == 0 {
		return ""
	}
	parent := getMap(doc, path[:len(path)-1]...)
	if parent == nil {
		return ""
	}
	value, _ := parent[path[len(path)-1]].(string)
	return value
}

func getSlice(doc map[string]any, path ...string) []any {
	if len(path) == 0 {
		return nil
	}
	parent := getMap(doc, path[:len(path)-1]...)
	if parent == nil {
		return nil
	}
	value, _ := parent[path[len(path)-1]].([]any)
	return value
}

// collectInput returns the first collect input of one status document.
func collectInput(status map[string]any) map[string]any {
	inputs := getSlice(status, "next_transition", "collect", "inputs")
	if len(inputs) == 0 {
		return nil
	}
	input, _ := inputs[0].(map[string]any)
	return input
}

// argumentValues maps a collect input's arguments by name, values verbatim.
func argumentValues(input map[string]any) map[string]string {
	values := map[string]string{}
	arguments, _ := input["arguments"].([]any)
	for _, raw := range arguments {
		argument, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := argument["name"].(string)
		value, _ := argument["value"].(string)
		if name != "" {
			values[name] = value
		}
	}
	return values
}

// substituteTokens replaces {{slot}} markers in submission argument tokens.
func substituteTokens(tokens []any, values map[string]string) []string {
	out := make([]string, 0, len(tokens))
	for _, raw := range tokens {
		token, _ := raw.(string)
		for slot, value := range values {
			token = strings.ReplaceAll(token, "{{"+slot+"}}", value)
		}
		out = append(out, token)
	}
	return out
}

func (b *battery) captureCapabilities() {
	if doc, stderr, code := b.runJSON("capabilities", b.workRoot, "review", "capabilities"); doc == nil || code != 0 {
		b.fail("schema", "capture capabilities v1", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
	}
	if doc, stderr, code := b.runJSON("capabilities", b.workRoot, "review", "capabilities", "--contract", reviewContract); doc == nil || code != 0 {
		b.fail("schema", "capture capabilities v2", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

func timestamp() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// operationState reads the finalize state from either the negotiated
// operation envelope (result.state) or the legacy bare result (state):
// provider-rendered finalize commands without --contract emit the latter.
func operationState(doc map[string]any) string {
	if state := getString(doc, "result", "state"); state != "" {
		return state
	}
	return getString(doc, "state")
}

// operationLineage mirrors operationState for the lineage identifier.
func operationLineage(doc map[string]any) string {
	if lineage := getString(doc, "result", "lineage_id"); lineage != "" {
		return lineage
	}
	return getString(doc, "lineage_id")
}
