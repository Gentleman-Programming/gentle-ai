package organicruntime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
)

// openCodePoisonedReviewSetup is the shared fixture for the ordinary-session
// adapter conformance proof below: a real negotiated review, walked through
// to its exactly-one reviewer collect input, with the live worktree poisoned
// strictly after the candidate froze.
type openCodePoisonedReviewSetup struct {
	harness       *organicHarness
	lineage       string
	candidatePath string
	binding       map[string]string
	manifestPaths []string
	configRoot    string
	home          string
}

// poisonMarker is planted, after the candidate freezes, into the reviewed
// file, a brand-new secret-shaped file, AND AGENTS.md at the worktree root.
// The reviewer holds no bash and no read tool, and the plugin replaces its
// task prompt wholesale with the provider-injected frozen context, so none
// of these three locations can ever reach the reviewer's own task prompt or
// the OpenCode session transcript, regardless of host isolation
// configuration.
const openCodePoisonMarker = "POISON-MARKER-2417-b6b2c1"

func requireOpenCodeImmutableReviewExecutor(t *testing.T) {
	t.Helper()
	if os.Getenv(realAgentE2EEnvironment) != "1" {
		t.Skip("set GENTLE_AI_REAL_AGENT_E2E=1 to run the real OpenCode reviewer proof")
	}
}

// setupOpenCodePoisonedReview drives a real negotiated review to its
// reviewer collect point and poisons the worktree. It does not launch
// OpenCode: the caller configures its own environment and fixture.
func setupOpenCodePoisonedReview(t *testing.T, lineage string) openCodePoisonedReviewSetup {
	t.Helper()
	requireOrganicExecutableVersion(t, "opencode", pinnedOpenCodeVersion)

	harness := newOrganicHarness(t)
	const candidatePath = "internal/mechanical/candidate.go"
	// Committed deliberately: an intended-untracked candidate's frozen
	// snapshot is later re-validated against its live content, so a
	// candidate this test is about to poison must instead be a genuinely
	// immutable committed blob the poison can never reach.
	harness.writeFiles(map[string]string{
		candidatePath: "package mechanical\n\nfunc Compute(value int) int {\n\tif value < 0 {\n\t\treturn -value\n\t}\n\treturn value * 2\n}\n",
	})
	harness.git("add", "--", candidatePath)
	harness.git("commit", "-q", "-m", "test: seed the reviewed candidate")

	// Reuses harness.home (not a second, unrelated t.TempDir()): the opaque
	// repository-context handle STATUS/START mint below is itself persisted
	// under a HOME-rooted storage path (reviewRepositoryContextHome), so the
	// OpenCode-launched gentle-ai process that later resolves it must run
	// under the exact same HOME harness.gentle already uses.
	home := harness.home
	configRoot := prepareOpenCodeConfig(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	if _, err := sdd.Inject(home, opencode.NewAdapter(), ""); err != nil {
		t.Fatalf("generate OpenCode reviewer: %v", err)
	}
	pluginPath := filepath.Join(configRoot, "opencode", "plugins", "review-result-artifacts.ts")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("generated OpenCode review plugin is unavailable: %v", err)
	}

	// Follow the negotiated route exactly as review-ledger-contract.md
	// requires: begin from STATUS, never hardcode or substitute START, and
	// run only the exact returned execute command until STATUS itself hands
	// back a collect transition. The first round asks for --focus=risk (set
	// explicitly below since a fresh lineage has no prior focus) and relays
	// the one-time medium-risk consent question; the second round answers it
	// with --consent=granted for this exact candidate.
	status := organicNegotiatedStatus(t, harness, lineage)
	for round := 0; round < 5 && status.NextTransition != nil && status.NextTransition.Kind == "execute"; round++ {
		execute := status.NextTransition.Execute
		if execute == nil {
			t.Fatalf("execute transition with no execute payload: %#v", status.NextTransition)
		}
		arguments := organicCommandArguments(t, execute.Command)
		if execute.Operation == "review.start" {
			if organicArgumentValue(arguments, "--focus") == "" {
				arguments = append(arguments, "--focus=risk")
			}
			if organicArgumentValue(arguments, "--consent") == "relay" {
				arguments = organicReplaceArgument(arguments, "--consent", "granted")
			}
		}
		harness.gentle(arguments...)
		status = organicNegotiatedStatus(t, harness, lineage)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != "collect" || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("expected exactly one reviewer collect input for a single-lens standard review: %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	binding := organicCollectBindingFields(t, input)
	if binding["lens"] != "review-risk" {
		t.Fatalf("--focus risk did not select review-risk: %#v", binding)
	}
	manifestPaths := organicManifestPaths(input)
	if len(manifestPaths) != 1 || manifestPaths[0] != candidatePath {
		t.Fatalf("changed_path_manifest = %v, want exactly [%s]", manifestPaths, candidatePath)
	}

	// Poison the worktree strictly AFTER the candidate froze: the reviewed
	// file, a brand-new secret-shaped file, and AGENTS.md at the worktree
	// root.
	poisonedCandidate := "package mechanical\n\n// " + openCodePoisonMarker + "\nfunc Compute(value int) int {\n\treturn 0\n}\n"
	if err := os.WriteFile(filepath.Join(harness.repo.worktree, candidatePath), []byte(poisonedCandidate), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness.repo.worktree, "SECRET.txt"), []byte(openCodePoisonMarker+"\nsk-live-not-a-real-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	agentsMd := "# Project instructions\n\n" + openCodePoisonMarker + "\n\nPlanted after the candidate froze.\n"
	if err := os.WriteFile(filepath.Join(harness.repo.worktree, "AGENTS.md"), []byte(agentsMd), 0o644); err != nil {
		t.Fatal(err)
	}

	return openCodePoisonedReviewSetup{
		harness: harness, lineage: lineage, candidatePath: candidatePath,
		binding: binding, manifestPaths: manifestPaths, configRoot: configRoot, home: home,
	}
}

// openCodeReviewEnvironment builds the environment for the `opencode run`
// process. It sets no OPENCODE_DISABLE_PROJECT_CONFIG or
// OPENCODE_DISABLE_EXTERNAL_SKILLS: the shared advisory transport
// (rdd-advisory-transport SKILL.md) requires neither, and this is the
// ordinary-session adapter conformance proof.
func (setup openCodePoisonedReviewSetup) openCodeReviewEnvironment(t *testing.T, configJSON string) []string {
	t.Helper()
	return replaceOrganicEnvironment(organicEnvironment(setup.home), map[string]string{
		// The OpenCode plugin's runNative spawns the bare "gentle-ai" name
		// (never the full organicBinary path, since production callers never
		// know it), so the built test binary's directory must resolve first
		// on PATH -- otherwise a real, differently-configured `gentle-ai` on
		// the operator's own PATH answers instead, against a repository
		// state this test never touched.
		"PATH":                                      filepath.Dir(organicBinary) + string(os.PathListSeparator) + os.Getenv("PATH"),
		"XDG_CONFIG_HOME":                           setup.configRoot,
		"XDG_CACHE_HOME":                            t.TempDir(),
		"OPENCODE_CONFIG_DIR":                       filepath.Join(setup.configRoot, "opencode"),
		"OPENCODE_TEST_HOME":                        filepath.Join(setup.home, "opencode"),
		"OPENCODE_CONFIG_CONTENT":                   configJSON,
		"OPENCODE_AUTH_CONTENT":                     "{}",
		"OPENCODE_DISABLE_AUTOUPDATE":               "1",
		"OPENCODE_DISABLE_AUTOCOMPACT":              "1",
		"OPENCODE_DISABLE_CLAUDE_CODE":              "1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS":          "1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD":             "1",
		"OPENCODE_DISABLE_MODELS_FETCH":             "1",
		"OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER": "1",
		"OPENCODE_FAST_BOOT":                        "1",
	})
}

// runOpenCodeReview launches the review-driver against a real `opencode`
// process. It deliberately omits --pure: --pure disables every local
// OpenCode plugin, including review-result-artifacts.ts, so a --pure run
// would prove nothing about the transport under test.
func runOpenCodeReview(t *testing.T, setup openCodePoisonedReviewSetup, environment []string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), organicAgentTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "opencode", "run", "--format", "json", "--agent", "review-driver", "--model", "fixture/fixture", "--dir", setup.harness.repo.worktree, "Delegate the immutable review inspection.")
	command.Dir = setup.harness.repo.worktree
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("opencode run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// TestRealOpenCodeReviewerOrdinarySessionInjectsFrozenContextAndAdmitsRawOutput
// is the ordinary-session adapter conformance proof (rdd-advisory-transport
// SKILL.md, replacing issue #2417's isolation-gated proof): a genuinely
// launched review-risk lens -- holding no bash and no read tool, running in
// an ordinary already-running OpenCode session with no
// OPENCODE_DISABLE_PROJECT_CONFIG or OPENCODE_DISABLE_EXTERNAL_SKILLS set --
// still only ever sees the frozen candidate the OpenCode plugin
// (review-result-artifacts.ts) injected into its prompt before it launched:
// not the reviewed file's live content, not a brand-new secret-shaped file,
// not AGENTS.md, and not the caller-authored prose the driver's task call
// carried before provider injection replaced it.
//
// It also proves the reduced plugin's second half: the completed reviewer
// task's own output is the model's raw final text, not a native capture
// manifest the plugin built itself (the plugin no longer calls
// `review capture-result` or `review preserve-result` at all). Native
// admission is exercised exactly as a real driver would: this test extracts
// that raw text from the OpenCode transcript and routes it through the
// exact native capture operation the negotiated collect step named, then
// finalizes -- proving raw output reaches native admission and creates
// review authority.
func TestRealOpenCodeReviewerOrdinarySessionInjectsFrozenContextAndAdmitsRawOutput(t *testing.T) {
	requireOpenCodeImmutableReviewExecutor(t)
	setup := setupOpenCodePoisonedReview(t, "opencode-poisoned-worktree-ordinary-session")

	fixture := newOpenCodeReviewerFixture(t, setup.binding, setup.manifestPaths)
	defer fixture.Close()

	config := generatedOpenCodeReviewConfig(t, filepath.Join(setup.configRoot, "opencode", "opencode.json"), fixture.URL)
	environment := setup.openCodeReviewEnvironment(t, config)

	transcript := runOpenCodeReview(t, setup, environment)
	if strings.Contains(transcript, openCodePoisonMarker) {
		t.Fatalf("poison marker leaked into the OpenCode transcript:\n%s", transcript)
	}
	assertNoBashOrReadToolUse(t, transcript)
	launches, rawOutput := lastCompletedTaskRawOutput(t, transcript)
	if launches != 1 || strings.TrimSpace(rawOutput) == "" {
		t.Fatalf("expected exactly one completed reviewer task launch with raw output, got launches=%d output=%q:\n%s", launches, rawOutput, transcript)
	}
	// The reduced plugin hands back the model's raw final text: no native
	// capture manifest field can appear in a task's own output, because the
	// plugin never calls `review capture-result` itself anymore.
	if strings.Contains(rawOutput, "admission_decision") {
		t.Fatalf("task output still carries a native capture manifest; the plugin must hand back raw text: %s", rawOutput)
	}

	fixture.mu.Lock()
	received := fixture.receivedContext
	fixture.mu.Unlock()
	if received == "" {
		t.Fatal("the reviewer's own model call never arrived at the fixture")
	}
	if strings.Contains(received, openCodePoisonMarker) {
		t.Fatalf("poison marker (worktree file, secret file, or AGENTS.md) reached the reviewer's own model call:\n%s", received)
	}
	if strings.Contains(received, "caller prose that provider injection must discard") {
		t.Fatalf("the driver's caller-authored prose survived provider injection into the reviewer's own model call:\n%s", received)
	}
	if !strings.Contains(received, "GENTLE_AI_REVIEW_CONTEXT") || !strings.Contains(received, setup.candidatePath) {
		t.Fatalf("reviewer did not receive the provider-injected context block:\n%s", received)
	}

	// Raw-output-to-native-admission: route the exact raw text the plugin
	// handed back through the exact capture operation the negotiated collect
	// step named for this binding, precisely as the launching session -- not
	// the plugin -- is now responsible for doing.
	resultPath := filepath.Join(t.TempDir(), "reviewer-result.json")
	if err := os.WriteFile(resultPath, []byte(rawOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := setup.harness.gentleAllowFailure(
		"review", "capture-result",
		"--repository-context", setup.binding["repository-context"],
		"--expected-revision", setup.binding["expected-revision"],
		"--lineage", setup.binding["lineage"],
		"--target", setup.binding["target"],
		"--lens", setup.binding["lens"],
		"--order", setup.binding["order"],
		"--subject-hash", setup.binding["subject-hash"],
		"--input", resultPath,
	); err != nil {
		t.Fatalf("native admission refused the plugin's raw output: %v", err)
	}

	// The receipt materializes: finalize succeeds against the exact captured
	// lineage and creates durable review authority. STATUS is not re-queried
	// here: STATUS's fresh-target derivation always reflects the live
	// workspace, which this test just deliberately poisoned, so it would
	// derive a new, different target rather than recognizing the one already
	// reviewing -- finalize itself needs no such re-derivation.
	setup.harness.gentle("review", "finalize", "--cwd", setup.harness.repo.worktree, "--lineage", setup.lineage, "--captured-results=true")
	if _, err := os.Stat(filepath.Join(setup.harness.commonDir(), "gentle-ai", "review-transactions", "v2")); err != nil {
		t.Fatalf("captured reviewer result never created review authority: %v", err)
	}
}

// organicCommandArguments splits one negotiated transition's literally
// runnable command string into the argv this test hands to harness.gentle,
// dropping only the leading "gentle-ai" binary token. Every value in these
// commands (hashes, lineage names, opaque handles) is a single
// whitespace-free token, so a plain field split is exact.
func organicCommandArguments(t *testing.T, command string) []string {
	t.Helper()
	fields := strings.Fields(command)
	if len(fields) == 0 || fields[0] != "gentle-ai" {
		t.Fatalf("negotiated transition command does not start with gentle-ai: %q", command)
	}
	return append([]string(nil), fields[1:]...)
}

func organicArgumentValue(arguments []string, flag string) string {
	prefix := flag + "="
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			return strings.TrimPrefix(argument, prefix)
		}
	}
	return ""
}

func organicReplaceArgument(arguments []string, flag, value string) []string {
	prefix := flag + "="
	replaced := append([]string(nil), arguments...)
	for index, argument := range replaced {
		if strings.HasPrefix(argument, prefix) {
			replaced[index] = prefix + value
			return replaced
		}
	}
	return append(replaced, prefix+value)
}

type organicNegotiatedArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Token string `json:"token"`
}

type organicCollectionInput struct {
	Name                string                      `json:"name"`
	CaptureOperation    string                      `json:"capture_operation"`
	Arguments           []organicNegotiatedArgument `json:"arguments"`
	ChangedPathManifest []struct {
		Path string `json:"path"`
	} `json:"changed_path_manifest"`
}

type organicNegotiatedCollection struct {
	Inputs []organicCollectionInput `json:"inputs"`
}

type organicNegotiatedExecute struct {
	Operation string `json:"operation"`
	Command   string `json:"command"`
}

type organicNegotiatedTransition struct {
	Kind    string                       `json:"kind"`
	Execute *organicNegotiatedExecute    `json:"execute"`
	Collect *organicNegotiatedCollection `json:"collect"`
}

type organicNegotiatedStatusResult struct {
	NextTransition *organicNegotiatedTransition `json:"next_transition"`
}

func organicNegotiatedStatus(t *testing.T, harness *organicHarness, lineage string) organicNegotiatedStatusResult {
	t.Helper()
	payload := harness.gentle(
		"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v2",
		"--agent", "opencode", "--lineage", lineage, "--next-transition",
		"--base-ref", "origin/main", "--projection", "workspace",
	)
	var status organicNegotiatedStatusResult
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("decode negotiated review status: %v\n%s", err, payload)
	}
	return status
}

// organicCollectBindingFields flattens one collect input's arguments into a
// name->value map and requires the exact fields this test's own native
// capture-result call must relay -- the same fields the OpenCode plugin used
// to parse out of GENTLE_AI_REVIEW_BINDING before the shared advisory
// transport made that the launching session's job, not the plugin's.
func organicCollectBindingFields(t *testing.T, input organicCollectionInput) map[string]string {
	t.Helper()
	fields := make(map[string]string, len(input.Arguments))
	for _, argument := range input.Arguments {
		fields[argument.Name] = argument.Value
	}
	for _, required := range []string{"lineage", "expected-revision", "target", "repository-context", "lens", "order", "subject-hash"} {
		if fields[required] == "" {
			t.Fatalf("collect input is missing required binding field %q: %#v", required, input)
		}
	}
	return fields
}

func organicManifestPaths(input organicCollectionInput) []string {
	paths := make([]string, len(input.ChangedPathManifest))
	for index, entry := range input.ChangedPathManifest {
		paths[index] = entry.Path
	}
	return paths
}

// openCodeReviewerFixture plays two roles on one fixture model server: the
// primary "review-driver" agent, which launches exactly one real reviewer
// task with a genuine binding, and the launched reviewer's own model call,
// whose received content this test inspects for the poison marker.
type openCodeReviewerFixture struct {
	*httptest.Server
	mu              sync.Mutex
	binding         map[string]string
	manifestPaths   []string
	driverCalls     int
	receivedContext string
}

func newOpenCodeReviewerFixture(t *testing.T, binding map[string]string, manifestPaths []string) *openCodeReviewerFixture {
	t.Helper()
	fixture := &openCodeReviewerFixture{binding: binding, manifestPaths: manifestPaths}
	fixture.Server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	return fixture
}

func (fixture *openCodeReviewerFixture) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method", http.StatusMethodNotAllowed)
		return
	}
	var input openAIRequest
	if err := json.NewDecoder(io.LimitReader(request.Body, 8<<20)).Decode(&input); err != nil {
		http.Error(writer, "decode", http.StatusInternalServerError)
		return
	}
	if len(input.Messages) > 0 {
		last := input.Messages[len(input.Messages)-1]
		text := messageText(last.Content)
		if strings.Contains(text, "GENTLE_AI_REVIEW_CONTEXT") {
			// This is the reviewer's own model call: the plugin already
			// replaced the driver's short prompt with the full
			// provider-injected block. Record exactly what arrived and
			// answer with a genuine completed result -- the plugin hands
			// this raw text straight back to the driver, unmodified.
			fixture.mu.Lock()
			fixture.receivedContext = text
			fixture.mu.Unlock()
			result := map[string]any{
				"subject_hash": fixture.binding["subject-hash"],
				"inspection":   map[string]any{"status": "completed", "paths": fixture.manifestPaths},
				"findings":     []any{},
				"evidence":     []string{"inspected the provider-injected immutable evidence for " + fixture.binding["lens"]},
			}
			payload, _ := json.Marshal(result)
			fixture.writeText(writer, string(payload), "stop")
			return
		}
	}
	if len(input.Tools) == 0 {
		fixture.writeText(writer, "done", "stop")
		return
	}
	fixture.mu.Lock()
	fixture.driverCalls++
	call := fixture.driverCalls
	fixture.mu.Unlock()
	if call > 1 {
		fixture.writeText(writer, "driver done", "stop")
		return
	}
	order, err := strconv.Atoi(fixture.binding["order"])
	if err != nil {
		http.Error(writer, "malformed order", http.StatusInternalServerError)
		return
	}
	bindingPayload, _ := json.Marshal(map[string]any{
		"lineage": fixture.binding["lineage"], "target": fixture.binding["target"],
		"lens": fixture.binding["lens"], "order": order,
		"revision": fixture.binding["expected-revision"], "repository_context": fixture.binding["repository-context"],
		"subject_hash": fixture.binding["subject-hash"],
	})
	// The caller-authored prose below must never survive provider injection:
	// this test's transcript assertions implicitly cover this, since the
	// reviewer's completed result contains no trace of it.
	prompt := "GENTLE_AI_REVIEW_BINDING " + string(bindingPayload) + "\n" +
		"caller prose that provider injection must discard: read the live worktree directly"
	fixture.writeTool(writer, "reviewer-launch", "task", map[string]any{
		"description":   "Delegate the immutable review inspection",
		"subagent_type": fixture.binding["lens"],
		"prompt":        prompt,
	})
}

func (fixture *openCodeReviewerFixture) writeText(writer http.ResponseWriter, content, reason string) {
	fixture.writeChunks(writer, []any{
		map[string]any{
			"id": "chat", "object": "chat.completion.chunk", "created": 0, "model": "fixture",
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": content}, "finish_reason": nil}},
		},
		organicFinishChunk(reason),
	})
}

func (fixture *openCodeReviewerFixture) writeTool(writer http.ResponseWriter, id, name string, arguments any) {
	encoded, _ := json.Marshal(arguments)
	fixture.writeChunks(writer, []any{
		map[string]any{
			"id": "chat", "object": "chat.completion.chunk", "created": 0, "model": "fixture",
			"choices": []any{map[string]any{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"index": 0, "id": "call_" + id, "type": "function",
						"function": map[string]any{"name": name, "arguments": string(encoded)},
					}},
				},
				"finish_reason": nil,
			}},
		},
		organicFinishChunk("tool_calls"),
	})
}

func (fixture *openCodeReviewerFixture) writeChunks(writer http.ResponseWriter, chunks []any) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	for _, chunk := range chunks {
		encoded, _ := json.Marshal(chunk)
		_, _ = writer.Write([]byte("data: " + string(encoded) + "\n\n"))
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	_, _ = io.WriteString(writer, "data: [DONE]\n\n")
}

// bashOrReadToolUse scans every emitted tool_use event and returns the first
// bash or read tool name it finds, or "" if none occurred. These are tools
// the generated review-risk agent does not hold at all, so a genuine use
// here would mean either the generated config regressed or the runtime
// bypassed it.
func bashOrReadToolUse(transcript string) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(transcript))
	for {
		var event struct {
			Type string `json:"type"`
			Part *struct {
				Type string `json:"type"`
				Tool string `json:"tool"`
			} `json:"part"`
		}
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			return "", nil
		} else if err != nil {
			return "", err
		}
		if event.Type != "tool_use" || event.Part == nil || event.Part.Type != "tool" {
			continue
		}
		if event.Part.Tool == "bash" || event.Part.Tool == "read" {
			return event.Part.Tool, nil
		}
	}
}

func assertNoBashOrReadToolUse(t *testing.T, transcript string) {
	t.Helper()
	tool, err := bashOrReadToolUse(transcript)
	if err != nil {
		t.Fatalf("decode OpenCode JSON event: %v", err)
	}
	if tool != "" {
		t.Fatalf("reviewer session used tool %q, which the generated review-risk agent must never hold", tool)
	}
}

// lastCompletedTaskRawOutput returns how many "task" tool_use events
// occurred and the last completed one's raw output. The reduced plugin
// hands back the model's raw final text (SKILL.md: adapters return raw
// bytes plus a transport error, never a captured artifact), so this is the
// exact text a real launching session would route through native admission
// next -- there is no more capture manifest for a mutation-proof to inspect.
func lastCompletedTaskRawOutput(t *testing.T, transcript string) (launches int, rawOutput string) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(transcript))
	for {
		var event struct {
			Type string `json:"type"`
			Part *struct {
				Type  string `json:"type"`
				Tool  string `json:"tool"`
				State struct {
					Status string `json:"status"`
					Output string `json:"output"`
				} `json:"state"`
			} `json:"part"`
		}
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			return launches, rawOutput
		} else if err != nil {
			t.Fatalf("decode OpenCode JSON event: %v", err)
		}
		if event.Type != "tool_use" || event.Part == nil || event.Part.Tool != "task" {
			continue
		}
		launches++
		if event.Part.State.Status == "completed" {
			rawOutput = event.Part.State.Output
		}
	}
}

// TestBashOrReadToolUseDetectsRegression is a fast, non-gated proof of the
// detection helper itself: mutation-proofs (a)/(b) target the generated
// config (see TestOpenCodeOverlaysRenderBoundedReadOnlyReviewRoles in
// internal/components/sdd), but if a regression ever let a reviewer session
// actually call bash or read, this is what would catch it in a real
// transcript.
func TestBashOrReadToolUseDetectsRegression(t *testing.T) {
	tests := []struct {
		name       string
		transcript string
		want       string
		wantErr    bool
	}{
		{name: "clean transcript", transcript: `{"type":"tool_use","part":{"type":"tool","tool":"task"}}`},
		{name: "bash tool_use", transcript: `{"type":"tool_use","part":{"type":"tool","tool":"bash"}}`, want: "bash"},
		{name: "read tool_use", transcript: `{"type":"tool_use","part":{"type":"tool","tool":"read"}}`, want: "read"},
		{name: "unrelated event", transcript: `{"type":"text"}`},
		{name: "malformed JSON", transcript: `{`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := bashOrReadToolUse(test.transcript)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("bashOrReadToolUse() = (%q, %v), want (%q, error=%t)", got, err, test.want, test.wantErr)
			}
		})
	}
}

func generatedOpenCodeReviewConfig(t *testing.T, settingsPath, serverURL string, instructions ...string) string {
	t.Helper()
	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatal(err)
	}
	config["provider"] = map[string]any{"fixture": map[string]any{
		"npm": "@ai-sdk/openai-compatible", "name": "OpenCode Reviewer E2E Fixture",
		"options": map[string]any{"baseURL": serverURL + "/v1", "apiKey": "fixture"},
		"models":  map[string]any{"fixture": map[string]any{"name": "Fixture"}},
	}}
	agents := config["agent"].(map[string]any)
	agents["review-driver"] = map[string]any{
		"description": "Attempts the generated immutable reviewer", "mode": "primary", "model": "fixture/fixture",
		"permission": map[string]any{"bash": "deny", "task": "allow", "edit": "deny"},
	}
	if len(instructions) > 0 {
		config["instructions"] = instructions
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func replaceOrganicEnvironment(environment []string, values map[string]string) []string {
	for name, value := range values {
		prefix := name + "="
		replaced := false
		for index, entry := range environment {
			if strings.HasPrefix(entry, prefix) {
				environment[index], replaced = prefix+value, true
			}
		}
		if !replaced {
			environment = append(environment, prefix+value)
		}
	}
	return environment
}
