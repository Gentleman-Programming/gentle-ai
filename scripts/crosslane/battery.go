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

	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
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

// lineageScope is the Go-issued authority binding a lane received at START.
// The lineage is immutable; revision and target advance only from native STATUS.
type lineageScope struct {
	Lineage       string
	Revision      string
	Target        string
	BaseRef       string
	CommittedOnly bool
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
	lineages   map[string]lineageScope
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

// status queries a native transition. Once START has bound a repository, every
// continuation carries that exact lineage and its authority is checked here.
func (b *battery) status(repo, agent string, extra ...string) (map[string]any, string, int) {
	args := b.statusArgs(repo, agent, extra...)
	doc, stderr, code := b.runJSON("status", repo, args...)
	if err := b.admitStatusScope(repo, doc); err != nil {
		return nil, err.Error(), 1
	}
	return doc, stderr, code
}

func (b *battery) statusArgs(repo, agent string, extra ...string) []string {
	args := []string{"review", "status", "--cwd", repo, "--contract", reviewContract, "--agent", agent, "--next-transition"}
	args = append(args, extra...)
	if scope, found := b.lineages[repo]; found {
		args = append(args, "--lineage", scope.Lineage)
		if scope.BaseRef != "" {
			args = append(args, "--base-ref", scope.BaseRef)
		}
		if scope.CommittedOnly {
			args = append(args, "--committed-only")
		}
	}
	return args
}

// statusFromClosure follows the final capture's provider-owned continuation.
// It intentionally consumes operation plus ordered tokens, never a command line
// or the lane's retained lineage state.
func (b *battery) statusFromClosure(repo string, closure map[string]any) (map[string]any, string, int) {
	return b.statusFromClosureEnv(repo, nil, closure)
}

func (b *battery) statusFromClosureEnv(repo string, env []string, closure map[string]any) (map[string]any, string, int) {
	continuation := getMap(closure, "status_continuation")
	if getString(continuation, "operation") != "review.status" {
		return nil, "last-event closure omitted review.status continuation", 1
	}
	if scope, found := b.lineages[repo]; found && operationLineage(closure) != scope.Lineage {
		return nil, "last-event closure lineage does not match the started authority", 1
	}
	doc, stderr, code := b.runTransitionExecution("status", repo, env, continuation)
	if err := b.admitStatusScope(repo, doc); err != nil {
		return nil, err.Error(), 1
	}
	return doc, stderr, code
}

// runTransitionExecution dispatches a public operation and its ordered argument
// tokens. It never reparses the rendered command or rebuilds any selector.
func (b *battery) runTransitionExecution(source, repo string, env []string, execution map[string]any) (map[string]any, string, int) {
	var args []string
	switch getString(execution, "operation") {
	case "review.start":
		args = []string{"review", "start"}
	case "review.status":
		args = []string{"review", "status"}
	case "review.recover":
		args = []string{"review", "recover"}
	case "review.repair":
		args = []string{"review", "repair"}
	case "review.validate":
		args = []string{"review", "validate"}
	default:
		return nil, "unsupported provider transition operation", 1
	}
	for _, argument := range getSlice(execution, "arguments") {
		entry, ok := argument.(map[string]any)
		if !ok {
			return nil, "provider transition argument is not an object", 1
		}
		token, _ := entry["token"].(string)
		if token == "" {
			return nil, "provider transition argument omitted its token", 1
		}
		args = append(args, token)
	}
	return b.runJSONEnv(source, repo, env, args...)
}

func (b *battery) rememberStarted(repo, target string, start map[string]any) error {
	context := getMap(start, "repository_context")
	scope := lineageScope{Lineage: operationLineage(start), Revision: getString(context, "revision"), Target: getString(context, "target_identity")}
	if scope.Lineage == "" || scope.Revision == "" || scope.Target == "" || scope.Target != target {
		return fmt.Errorf("START omitted the exact authority lineage/revision/target")
	}
	b.lineages[repo] = scope
	return nil
}

func (b *battery) admitStatusScope(repo string, doc map[string]any) error {
	scope, active := b.lineages[repo]
	if !active || doc == nil {
		return nil
	}
	authority := getMap(doc, "authority")
	target := getString(doc, "authority_target_identity")
	if target == "" {
		target = getString(doc, "target_identity")
	}
	if authority == nil || getString(authority, "lineage_id") != scope.Lineage || getString(authority, "revision") == "" || target == "" {
		return fmt.Errorf("STATUS no longer matches the started authority lineage/revision/target")
	}
	scope.Revision, scope.Target = getString(authority, "revision"), target
	b.lineages[repo] = scope
	if input := collectInput(doc); input != nil {
		args := argumentValues(input)
		expectedTarget := scope.Target
		if correctionTarget := getString(doc, "validation_request", "correction_target_identity"); correctionTarget != "" {
			expectedTarget = correctionTarget
		}
		if args["lineage"] != scope.Lineage || args["expected-revision"] != scope.Revision || args["target"] != expectedTarget {
			return fmt.Errorf("collect slot does not match the started authority lineage/revision/target")
		}
	}
	return nil
}

// runCommandLine executes a provider-rendered command with the product's
// quoting-aware splitter. Transition closures use runTransitionExecution instead,
// because their operation and ordered argument tokens are already structured.
func (b *battery) runCommandLine(source, dir, command string) (map[string]any, string, int) {
	words, err := cli.SplitPrintedCommandWords(command)
	if err != nil || len(words) < 2 || words[0] != "gentle-ai" {
		return nil, fmt.Sprintf("unexpected provider command %q", command), 1
	}
	return b.runJSON(source, dir, words[1:]...)
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

// committedMediumCandidate creates the exact committed-only review shape: an
// immutable base tree followed by a clean committed candidate. The later
// lifecycle never depends on a mutable workspace diff for its initial target.
func (b *battery) committedMediumCandidate(lane, name, path, base, candidate string) (string, string, bool) {
	repo, err := b.scratchRepo(name)
	if err != nil {
		b.fail(lane, "committed process scratch repository", err.Error())
		return "", "", false
	}
	if err := writeFile(repo, path, base); err != nil {
		b.fail(lane, "committed process base", err.Error())
		return "", "", false
	}
	if err := commitAll(repo, "feat: committed base"); err != nil {
		b.fail(lane, "committed process base", err.Error())
		return "", "", false
	}
	baseTree, err := runGitOutput(repo, "rev-parse", "HEAD^{tree}")
	if err != nil || baseTree == "" {
		b.fail(lane, "committed process base", fmt.Sprintf("resolve immutable base tree: %v", err))
		return "", "", false
	}
	if err := writeFile(repo, path, candidate); err != nil {
		b.fail(lane, "committed process candidate", err.Error())
		return "", "", false
	}
	if err := commitAll(repo, "feat: review candidate"); err != nil {
		b.fail(lane, "committed process candidate", err.Error())
		return "", "", false
	}
	return repo, baseTree, true
}

// startCommittedMedium follows STATUS's structured START operation for an exact
// base tree and committed-only candidate, then runs the consent envelope's
// provider-owned granted invocation through the product's quoting-aware splitter.
func (b *battery) startCommittedMedium(lane, repo, agent, baseTree string) bool {
	statusDoc, stderr, code := b.status(repo, agent, "--base-ref", baseTree, "--committed-only")
	execution := getMap(statusDoc, "next_transition", "execute")
	if code != 0 || getString(execution, "operation") != "review.start" ||
		!transitionCarriesToken(execution, "--base-ref="+baseTree) || !transitionCarriesToken(execution, "--committed-only=true") {
		b.fail(lane, "committed START advertised", fmt.Sprintf("exit=%d operation=%q base-ref=%t committed-only=%t %s",
			code, getString(execution, "operation"), transitionCarriesToken(execution, "--base-ref="+baseTree),
			transitionCarriesToken(execution, "--committed-only=true"), firstLine(stderr)))
		return false
	}
	consent, stderr, code := b.runTransitionExecution("start", repo, nil, execution)
	if code != 0 || getString(consent, "schema") != "gentle-ai.review-integration.consent/v3" || getString(consent, "action") != "consent_required" {
		b.fail(lane, "committed START consent", fmt.Sprintf("exit=%d schema=%q action=%q %s",
			code, getString(consent, "schema"), getString(consent, "action"), firstLine(stderr)))
		return false
	}
	granted := grantedInvocation(consent)
	if granted == "" {
		b.fail(lane, "committed START consent", "no granted choice invocation in envelope")
		return false
	}
	started, stderr, code := b.runCommandLine("start", repo, granted)
	if code != 0 || getString(started, "state") != "reviewing" || getString(started, "risk_level") != "medium" || len(getSlice(started, "selected_lenses")) != 1 {
		b.fail(lane, "committed START consent", fmt.Sprintf("exit=%d state=%q risk=%q lenses=%d %s",
			code, getString(started, "state"), getString(started, "risk_level"), len(getSlice(started, "selected_lenses")), firstLine(stderr)))
		return false
	}
	if err := b.rememberStarted(repo, getString(statusDoc, "target_identity"), started); err != nil {
		b.fail(lane, "committed START consent", err.Error())
		return false
	}
	scope := b.lineages[repo]
	scope.BaseRef, scope.CommittedOnly = baseTree, true
	b.lineages[repo] = scope
	b.pass(lane, "committed START consent", "immutable base tree and committed-only candidate created a reviewing medium lineage")
	return true
}

func transitionCarriesToken(execution map[string]any, want string) bool {
	for _, argument := range getSlice(execution, "arguments") {
		entry, ok := argument.(map[string]any)
		if ok && entry["token"] == want {
			return true
		}
	}
	return false
}

func runGit(dir string, args ...string) error {
	_, err := runGitOutput(dir, args...)
	return err
}

func runGitOutput(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
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

// operationState reads the state from either a negotiated operation envelope
// (result.state) or a direct last-event capture result (state).
func operationState(doc map[string]any) string {
	if state := getString(doc, "result", "state"); state != "" {
		return state
	}
	return getString(doc, "state")
}

// admittedCapture accepts both an intermediate result-artifact acknowledgement
// and the terminal last-event response. The final selected lens or validator
// no longer hands control to FINALIZE: it closes the review itself.
func admittedCapture(doc map[string]any) bool {
	switch getString(doc, "schema") {
	case "gentle-ai.review-result-artifact/v2":
		return getString(doc, "admission_decision") == "completed"
	case "gentle-ai.review-last-event-closure/v1":
		switch operationState(doc) {
		case "approved", "correction_required", "escalated":
			return true
		}
	}
	return false
}

// operationLineage mirrors operationState for the lineage identifier.
func operationLineage(doc map[string]any) string {
	if lineage := getString(doc, "result", "lineage_id"); lineage != "" {
		return lineage
	}
	return getString(doc, "lineage_id")
}

func (b *battery) burnApproved(lane, name, repo, _ string, env []string, finalized map[string]any) bool {
	scope, found := b.lineages[repo]
	result := getMap(finalized, "result")
	if result == nil {
		result = finalized
	}
	if !found || operationState(finalized) != "approved" || getString(result, "lineage_id") != scope.Lineage || !strings.Contains(getString(result, "action"), "burned") {
		b.fail(lane, name, "terminal capture did not report approved+burn for the exact lineage")
		return false
	}
	for _, field := range []string{"receipt", "receipt_path", "authority", "next_transition"} {
		if _, present := result[field]; present {
			b.fail(lane, name, "burned terminal retained "+field)
			return false
		}
	}
	delete(b.lineages, repo)
	for _, gate := range []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"} {
		doc, stderr, code := b.runJSONEnv("gate", repo, env,
			"review", "validate", "--cwd", repo, "--contract", reviewContract, "--gate", gate)
		gateResult := getMap(doc, "result")
		if code != 0 || getString(gateResult, "result") != "invalidated" || getString(gateResult, "delivery") != "unmanaged" ||
			getString(gateResult, "action") != "repository-policy" {
			b.fail(lane, name, fmt.Sprintf("%s exit=%d result=%q delivery=%q action=%q %s", gate, code,
				getString(gateResult, "result"), getString(gateResult, "delivery"), getString(gateResult, "action"), firstLine(stderr)))
			return false
		}
		allowed, _ := gateResult["allowed"].(bool)
		if allowed || len(getMap(gateResult, "context")) != 1 || getString(gateResult, "context", "gate") != gate {
			b.fail(lane, name, gate+" did not return the strict unmanaged gate shape")
			return false
		}
	}
	b.pass(lane, name, "approved authority burned; five delivery gates are invalidated/unmanaged repository policy")
	return true
}
