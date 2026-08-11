package organicruntime_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/advisoryreview"
)

// claudePoisonMarker is planted, after the candidate freezes, into the
// reviewed file and a brand-new secret-shaped file. The test-local launcher runs
// claude in an empty scratch directory it creates and deletes itself, with
// every built-in tool disabled, so neither location can ever reach the
// reviewer's own prompt, regardless of what the live worktree contains by the
// time claude actually runs.
const claudePoisonMarker = "POISON-MARKER-2692-cl4ud3"

const (
	claudeRuntimeVersion = "2.1.220 (Claude Code)"
	claudeRuntimeSHA256  = "674f61f20ff306f3100cf9200e4c36c4b70278b5bef2884549819b942a89c863"
	claudeRuntimeFakeKey = "gentle-ai-network-none-mock-key"
)

// requireClaudeBinary gates the proofs that only need the real claude binary,
// never an authenticated account: the transport-failure companion kills the
// process before any model call, so it runs continuously wherever the suite
// runs, including CI.
func requireClaudeBinary(t *testing.T) {
	t.Helper()
	if os.Getenv(realAgentE2EEnvironment) != "1" {
		t.Skip("set GENTLE_AI_REAL_AGENT_E2E=1 to run the real Claude reviewer proofs")
	}
	// Claude-specific and deliberately not requireOrganicExecutable (same
	// reasoning as requireCodexOrganicReviewer): a machine without the binary
	// is an expected, known gap, never a hard failure -- unlike OpenCode and
	// npm, which CI always provisions and for which a missing binary would be
	// a genuine environment defect. Any other LookPath error still fails
	// loudly.
	if _, err := exec.LookPath("claude"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skipf("claude binary not installed; continuous CI execution mirrors the codex gap (#2662): %v", err)
		}
		t.Fatalf("required executable claude: %v", err)
	}
}

func requireClaudeNetworkNone(t *testing.T) string {
	t.Helper()
	if testing.Short() || os.Getenv("GENTLE_AI_CLAUDE_RUNTIME_E2E") != "1" {
		t.Skip("claude_network_none_skipped: requires the pinned Docker proof image")
	}
	binary := os.Getenv("GENTLE_AI_CLAUDE_RUNTIME_BINARY")
	payload, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("claude_network_none_unavailable: pinned binary unavailable: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != claudeRuntimeSHA256 {
		t.Fatalf("Claude binary SHA-256 = %s, want %s", got, claudeRuntimeSHA256)
	}
	namespace, err := os.Readlink("/proc/self/ns/net")
	parent := os.Getenv("GENTLE_AI_PARENT_NETNS")
	if err != nil || parent == "" || strings.Trim(namespace, "net:[]") == parent {
		t.Fatalf("Claude proof does not have a distinct network namespace: child=%q parent=%q error=%v", namespace, parent, err)
	}
	interfaces, err := net.Interfaces()
	if err != nil || len(interfaces) != 1 || interfaces[0].Name != "lo" || interfaces[0].Flags&(net.FlagUp|net.FlagLoopback) != net.FlagUp|net.FlagLoopback {
		t.Fatalf("Claude proof requires only lo UP/LOOPBACK: interfaces=%v error=%v", interfaces, err)
	}
	routes, err := os.ReadFile("/proc/net/route")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(routes), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) > 1 && (fields[0] != "lo" || fields[1] == "00000000") {
			t.Fatalf("Claude proof has an external/default route: %q", line)
		}
	}
	connection, err := net.DialTimeout("tcp4", "192.0.2.1:443", time.Second)
	if connection != nil {
		_ = connection.Close()
		t.Fatal("Claude proof reached an external address")
	}
	if !errors.Is(err, syscall.ENETUNREACH) {
		t.Fatalf("external connect error = %v, want ENETUNREACH", err)
	}
	version := exec.Command(binary, "--version")
	version.Env = []string{"HOME=" + t.TempDir(), "LANG=C", "LC_ALL=C"}
	output, err := version.Output()
	if err != nil || strings.TrimSpace(string(output)) != claudeRuntimeVersion {
		t.Fatalf("Claude version = %q, want %q (error %v)", strings.TrimSpace(string(output)), claudeRuntimeVersion, err)
	}
	return binary
}

// claudeNegotiatedStatus is organicNegotiatedStatus bound to the claude-code
// runtime identity instead of opencode.
func claudeNegotiatedStatus(t *testing.T, harness *organicHarness, lineage string) organicNegotiatedStatusResult {
	t.Helper()
	payload := harness.gentle(
		"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v2",
		"--agent", "claude-code", "--lineage", lineage, "--next-transition",
		"--base-ref", "origin/main", "--projection", "workspace",
	)
	var status organicNegotiatedStatusResult
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("decode negotiated review status: %v\n%s", err, payload)
	}
	return status
}

// claudeReviewerCollectStatus queries negotiated STATUS and, when the live
// worktree's post-poison untracked files demand an explicit declaration
// (#2730's intended-untracked contract), answers it with exclude for exactly
// the reported inventory -- precisely what a real session would do, since the
// poisoned SECRET.txt is deliberately not part of the frozen candidate -- so
// the in-flight lineage's pending reviewer collect slot is what comes back.
// A STATUS that instead carried the reviewer collect straight through needs
// no declaration and is returned unchanged.
func claudeReviewerCollectStatus(t *testing.T, harness *organicHarness, lineage string) organicNegotiatedStatusResult {
	t.Helper()
	status := claudeNegotiatedStatus(t, harness, lineage)
	if status.NextTransition == nil || status.NextTransition.Kind != "collect" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Name != "intended_untracked_selection" {
		return status
	}
	digest := ""
	for _, argument := range status.NextTransition.Collect.Inputs[0].Arguments {
		if argument.Name == "expected_untracked_inventory" {
			digest = argument.Value
		}
	}
	if digest == "" {
		t.Fatalf("intended_untracked_selection collect input carries no expected_untracked_inventory: %#v", status.NextTransition)
	}
	payload := harness.gentle(
		"review", "status", "--cwd", harness.repo.worktree, "--contract", "gentle-ai.review-integration/v2",
		"--agent", "claude-code", "--lineage", lineage, "--next-transition",
		"--base-ref", "origin/main", "--projection", "workspace",
		"--untracked-scope=exclude", "--expected-untracked-inventory", digest,
	)
	var declaredStatus organicNegotiatedStatusResult
	if err := json.Unmarshal(payload, &declaredStatus); err != nil {
		t.Fatalf("decode negotiated review status after untracked declaration: %v\n%s", err, payload)
	}
	return declaredStatus
}

// claudePoisonedReviewSetup is the shared fixture for the Claude organic
// proofs below: a real negotiated review, walked through to its exactly-one
// reviewer collect input, with the live worktree poisoned strictly after the
// candidate froze.
type claudePoisonedReviewSetup struct {
	harness       *organicHarness
	lineage       string
	candidatePath string
	binding       map[string]string
	manifestPaths []string
}

func setupClaudePoisonedReview(t *testing.T, lineage string) claudePoisonedReviewSetup {
	t.Helper()
	harness := newOrganicHarness(t)
	const candidatePath = "internal/mechanical/candidate.go"
	// Committed deliberately, mirroring setupCodexPoisonedReview: a genuinely
	// immutable committed blob is what the poison below can never reach.
	harness.writeFiles(map[string]string{
		candidatePath: "package mechanical\n\nfunc Compute(value int) int {\n\tif value < 0 {\n\t\treturn -value\n\t}\n\treturn value * 2\n}\n",
	})
	harness.git("add", "--", candidatePath)
	harness.git("commit", "-q", "-m", "test: seed the reviewed candidate")

	// Follow the negotiated route exactly: begin from STATUS, never hardcode
	// or substitute START, and run only the exact returned execute command
	// until STATUS itself hands back a collect transition. --focus=risk keeps
	// the review to one lens; the one-time medium-risk consent question is
	// answered with --consent=granted for this exact candidate.
	status := claudeNegotiatedStatus(t, harness, lineage)
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
		status = claudeNegotiatedStatus(t, harness, lineage)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != "collect" || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("expected exactly one reviewer collect input for a single-lens standard review: %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "reviewer_result" {
		t.Fatalf("expected the reviewer collect input, got %#v", input)
	}
	binding := organicCollectBindingFields(t, input)
	if binding["lens"] != "review-risk" {
		t.Fatalf("--focus risk did not select review-risk: %#v", binding)
	}
	manifestPaths := organicManifestPaths(input)
	if len(manifestPaths) != 1 || manifestPaths[0] != candidatePath {
		t.Fatalf("changed_path_manifest = %v, want exactly [%s]", manifestPaths, candidatePath)
	}

	// Poison the worktree strictly AFTER the candidate froze: the reviewed
	// file and a brand-new secret-shaped file.
	poisonedCandidate := "package mechanical\n\n// " + claudePoisonMarker + "\nfunc Compute(value int) int {\n\treturn 0\n}\n"
	if err := os.WriteFile(filepath.Join(harness.repo.worktree, candidatePath), []byte(poisonedCandidate), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness.repo.worktree, "SECRET.txt"), []byte(claudePoisonMarker+"\nsk-live-not-a-real-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return claudePoisonedReviewSetup{
		harness: harness, lineage: lineage, candidatePath: candidatePath,
		binding: binding, manifestPaths: manifestPaths,
	}
}

// claudeAdvisoryPrompt fetches the exact canonical prompt for the claude-code
// runtime through the CLI's own generic prompt/text seam
// (`review advisory prompt`), the same command a real Claude-orchestrated
// session would run from the collect input's own repository-context and lens.
// It never builds a caller-authored request.
func (setup claudePoisonedReviewSetup) claudeAdvisoryPrompt(t *testing.T) string {
	t.Helper()
	payload := setup.harness.gentle(
		"review", "advisory", "prompt",
		"--repository-context", setup.binding["repository-context"],
		"--lens", setup.binding["lens"],
		"--runtime", "claude-code",
	)
	return strings.TrimRight(string(payload), "\n")
}

func writeClaudeReviewSSE(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "text/event-stream")
	text, _ := json.Marshal(result)
	_, _ = fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-5\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":10}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", text)
}

func requireCanonicalClaudeRequest(t *testing.T, setup claudePoisonedReviewSetup, prompt string) advisoryreview.Request {
	t.Helper()
	_, input, found := strings.Cut(prompt, "Input:\n")
	if !found {
		t.Fatalf("canonical advisory prompt omitted its Input block:\n%s", prompt)
	}
	payload, _, found := strings.Cut(input, "\n\nOutput schema:\n")
	if !found {
		t.Fatalf("canonical advisory prompt omitted its Output schema boundary:\n%s", prompt)
	}
	var request advisoryreview.Request
	if err := json.Unmarshal([]byte(payload), &request); err != nil {
		t.Fatalf("decode canonical advisory request: %v\n%s", err, payload)
	}
	subject := request.ArtifactSubject
	if subject.SubjectHash != setup.binding["subject-hash"] || subject.LineageID != setup.binding["lineage"] ||
		subject.AuthorityRevision != setup.binding["expected-revision"] || subject.TargetIdentity != setup.binding["target"] ||
		subject.Lens != setup.binding["lens"] || subject.BaseTree == "" || subject.CandidateTree == "" {
		t.Fatalf("canonical advisory subject does not match the negotiated binding: subject=%+v binding=%v", subject, setup.binding)
	}
	if len(request.ChangedPathManifest) != len(setup.manifestPaths) || len(request.Evidence) != len(setup.manifestPaths) {
		t.Fatalf("canonical advisory scope is incomplete: manifest=%+v evidence=%+v want paths=%v", request.ChangedPathManifest, request.Evidence, setup.manifestPaths)
	}
	for index, path := range setup.manifestPaths {
		if request.ChangedPathManifest[index].Path != path || request.Evidence[index].Path != path || strings.TrimSpace(request.Evidence[index].Content) == "" {
			t.Fatalf("canonical advisory path %d is not bound to complete provider evidence: manifest=%+v evidence=%+v", index, request.ChangedPathManifest[index], request.Evidence[index])
		}
	}
	if !strings.Contains(request.Evidence[0].Content, "func Compute(value int) int") ||
		!strings.Contains(request.Evidence[0].Content, "return value * 2") || strings.Contains(request.Evidence[0].Content, claudePoisonMarker) {
		t.Fatalf("canonical advisory evidence is not the frozen candidate content: %q", request.Evidence[0].Content)
	}
	return request
}

func newClaudeReviewFixture(t *testing.T, setup claudePoisonedReviewSetup, prompt string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	canonicalRequest := requireCanonicalClaudeRequest(t, setup, prompt)
	result, err := json.Marshal(map[string]any{
		"subject_hash": setup.binding["subject-hash"],
		"inspection":   map[string]any{"status": "completed", "paths": setup.manifestPaths},
		"findings":     []any{},
		"evidence":     []string{"inspected every frozen candidate path from the provider-bound manifest"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := listener.Addr().String()
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Host != host {
			w.WriteHeader(http.StatusMisdirectedRequest)
			return
		}
		if request.Method == http.MethodHead && request.URL.Path == "/api/hello" {
			return
		}
		body, _ := io.ReadAll(request.Body)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/messages" || request.Header.Get("x-api-key") != claudeRuntimeFakeKey {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		calls.Add(1)
		for _, required := range []string{canonicalRequest.ArtifactSubject.SubjectHash, canonicalRequest.ArtifactSubject.Lens, setup.candidatePath, "func Compute(value int) int", "return value * 2"} {
			if !bytes.Contains(body, []byte(required)) {
				t.Errorf("Claude request omitted provider-bound value %q", required)
			}
		}
		if bytes.Contains(body, []byte(claudePoisonMarker)) || bytes.Contains(body, []byte(`"tools":[{`)) {
			t.Errorf("Claude request leaked live state or exposed a tool")
		}
		writeClaudeReviewSSE(w, string(result))
	})}}
	server.Start()
	t.Cleanup(server.Close)
	return server, &calls
}

type claudeReviewEnvelope struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
}

// runClaudeReview is test-local process wiring for the pinned runtime proof.
// Production Claude transport remains session prompt-carried; this launcher
// exists only to prove that the genuine CLI preserves that boundary.
func runClaudeReview(ctx context.Context, binary, prompt string, environment []string) ([]byte, error) {
	scratch, err := os.MkdirTemp("", "gentle-ai-claude-advisory-*")
	if err != nil {
		return nil, fmt.Errorf("claude advisory transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	command := exec.CommandContext(ctx, binary,
		"--bare",
		"--print",
		"--output-format", "json",
		"--tools", "",
		"--permission-mode", "dontAsk",
		"--setting-sources=",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--no-chrome",
		"--no-session-persistence",
		"--prompt-suggestions=false",
	)
	command.Dir = scratch
	command.Env = environment
	command.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		var envelope claudeReviewEnvelope
		if jsonErr := json.Unmarshal(stdout.Bytes(), &envelope); jsonErr == nil && strings.TrimSpace(envelope.Result) != "" {
			return nil, fmt.Errorf("claude advisory transport failed: %w: %s", err, strings.TrimSpace(envelope.Result))
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("claude advisory transport failed: %w: %s", err, detail)
	}

	var envelope claudeReviewEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		return nil, fmt.Errorf("claude advisory transport produced no decodable result envelope: %w: %s", err, stdout.String())
	}
	if envelope.IsError {
		return nil, fmt.Errorf("claude advisory transport failed: %s", envelope.Result)
	}
	return []byte(envelope.Result), nil
}

// TestRealClaudeReviewerOrdinarySessionAdmitsRawOutput is the Claude Code
// organic runtime proof the shared advisory transport
// (rdd-advisory-transport SKILL.md) requires -- the evidence gap issues #2692
// and #2566 name: TestClaudeReviewerTransportInNetworkNone already proves the
// prompt-carried transport against a fake local model, but no real Claude
// Code run had completed a review to a terminal receipt. This is that run, in
// the exact shape the Codex proof uses: a genuinely launched review-risk
// lens, run by a real `claude --print` process through the test-local launcher
// in an empty scratch directory it creates and
// deletes itself, with every built-in tool disabled, still only ever sees the
// frozen candidate the provider rendered into its prompt -- not the reviewed
// file's live poisoned content and not a brand-new secret-shaped file. The
// reviewer needs no shell and never calls `inspect-candidate`: the provider
// request already carries the frozen evidence. It also proves the
// raw-output-to-native-admission boundary end to end: the real claude
// process's raw final text is routed through the exact native capture
// operation the negotiated collect step named, then finalized through to a
// terminal receipt and one delivery gate.
//
// The fail-closed companions below share this same setup and claude call,
// exercising native admission's malformed-input and subject-mismatch refusals
// against the identical genuine raw bytes before finally admitting them, plus
// a dedicated transport-failure case using a deliberately expired context.
func TestRealClaudeReviewerOrdinarySessionAdmitsRawOutput(t *testing.T) {
	claude := requireClaudeNetworkNone(t)
	setup := setupClaudePoisonedReview(t, "claude-poisoned-worktree-ordinary-session")

	prompt := setup.claudeAdvisoryPrompt(t)
	if strings.Contains(prompt, claudePoisonMarker) {
		t.Fatalf("poison marker leaked into the provider-rendered advisory prompt:\n%s", prompt)
	}

	server, calls := newClaudeReviewFixture(t, setup, prompt)
	home := t.TempDir()
	environment := []string{
		"HOME=" + home,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"ANTHROPIC_API_KEY=" + claudeRuntimeFakeKey,
		"ANTHROPIC_BASE_URL=" + server.URL,
		"NO_PROXY=127.0.0.1,localhost",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"LANG=C", "LC_ALL=C",
	}
	ctx, cancel := context.WithTimeout(context.Background(), organicAgentTimeout)
	defer cancel()
	raw, err := runClaudeReview(ctx, claude, prompt, environment)
	if err != nil {
		t.Fatalf("real claude reviewer call failed: %v", err)
	}
	if strings.Contains(string(raw), claudePoisonMarker) {
		t.Fatalf("poison marker (worktree file or secret file) reached the reviewer's raw output:\n%s", raw)
	}
	if calls.Load() != 1 {
		t.Fatalf("Claude model requests = %d, want exactly one", calls.Load())
	}
	t.Logf("real claude raw output: %s", raw)

	// --- Negative: malformed output refused, no authority mutation. ---
	// Truncated mid-object, never a second trailing object: native admission
	// extracts one complete JSON object from surrounding prose but rejects an
	// unterminated one (docs/review-integration.md), so this must genuinely
	// fail to parse rather than merely carry ignorable trailing garbage.
	closingBrace := bytes.LastIndexByte(raw, '}')
	if closingBrace < 0 {
		t.Fatalf("real claude raw output has no closing brace to truncate before: %q", raw)
	}
	// Cut strictly before the final '}' rather than a fixed byte count: a
	// fixed-width truncation only stays unterminated by coincidence, since
	// trailing prose length varies run to run and could leave the cut point
	// past the object's own close, producing an accidentally well-formed
	// negative case. Cutting at the last '}' keeps the JSON object itself
	// genuinely unterminated regardless of how much trailing prose claude
	// adds.
	malformed := raw[:closingBrace]
	malformedPath := setup.harness.writeRawReviewerResult("claude-malformed.json", malformed)
	if _, _, err := setup.captureResult(malformedPath); err == nil {
		t.Fatal("native admission accepted malformed claude output")
	}
	if status := claudeReviewerCollectStatus(t, setup.harness, setup.lineage); status.NextTransition == nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		t.Fatalf("malformed capture attempt mutated the negotiated transition: %#v", status.NextTransition)
	}

	// --- Negative: subject/lens mismatch refused, no authority mutation. ---
	wrongSubject := strings.Replace(string(raw), setup.binding["subject-hash"], "sha256:"+strings.Repeat("d", 64), 1)
	if wrongSubject == string(raw) {
		t.Fatalf("raw claude output does not contain the expected subject_hash %q to mutate:\n%s", setup.binding["subject-hash"], raw)
	}
	mismatchedPath := setup.harness.writeRawReviewerResult("claude-mismatched.json", []byte(wrongSubject))
	if _, _, err := setup.captureResult(mismatchedPath); err == nil {
		t.Fatal("native admission accepted a subject-mismatched claude output")
	}
	if status := claudeReviewerCollectStatus(t, setup.harness, setup.lineage); status.NextTransition == nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		t.Fatalf("mismatched capture attempt mutated the negotiated transition: %#v", status.NextTransition)
	}

	// --- Positive: the genuine raw output is admitted and reaches a receipt. ---
	resultPath := setup.harness.writeRawReviewerResult("claude-result.json", raw)
	if _, stderr, err := setup.captureResult(resultPath); err != nil {
		t.Fatalf("native admission refused genuine claude output: %v\n%s", err, stderr)
	}

	// A single-lens medium-risk review only reaches "validating" once its
	// reviewer results are captured; final verification evidence is a
	// distinct, required second step before the review can reach a terminal,
	// gate-eligible state (mirrors organicHarness.approveReview).
	if result := setup.harness.finalize(setup.lineage, "--captured-results=true"); result.State != organicStateValidating {
		t.Fatalf("captured claude result did not reach validation: %#v", result)
	}
	terminal := setup.harness.finalize(setup.lineage, "--evidence", setup.harness.writeEvidence())
	if terminal.State != organicStateApproved || terminal.ReceiptPath == "" {
		t.Fatalf("Claude review did not produce a terminal receipt: %#v", terminal)
	}
	if _, err := os.Stat(filepath.Join(setup.harness.commonDir(), "gentle-ai", "review-transactions", "v2")); err != nil {
		t.Fatalf("captured reviewer result never created review authority: %v", err)
	}

	gate := setup.harness.gate("pre-push")
	if gate.Result != organicGateAllow {
		t.Fatalf("pre-push gate result = %q, want %q: %#v", gate.Result, organicGateAllow, gate)
	}
}

// captureResult runs the exact `review capture-result` invocation the
// negotiated collect step named for this binding, with the caller-supplied
// input file.
func (setup claudePoisonedReviewSetup) captureResult(inputPath string) (string, string, error) {
	return setup.harness.gentleAllowFailure(
		"review", "capture-result",
		"--repository-context", setup.binding["repository-context"],
		"--expected-revision", setup.binding["expected-revision"],
		"--lineage", setup.binding["lineage"],
		"--target", setup.binding["target"],
		"--lens", setup.binding["lens"],
		"--order", setup.binding["order"],
		"--subject-hash", setup.binding["subject-hash"],
		"--input", inputPath,
	)
}

// TestRealClaudeReviewerTransportFailureStrandsNothing is the timeout/
// transport-failure fail-closed companion: an already-expired context makes
// the real claude binary a genuine transport failure (the process is killed
// before it can answer), the launcher reports it as a transport error,
// and the negotiated review is untouched -- no capture, no consumed
// correction budget, no stranded lineage. STATUS still offers the exact same
// collect slot afterward. This is precisely the dead-end #2692 and #2566
// reported, resolved the other way: a transport failure reoffers the slot
// instead of permanently completing it with an incomplete inspection. It
// needs the real binary but never a model call, so it requires no
// authenticated account and runs continuously wherever the suite runs.
func TestRealClaudeReviewerTransportFailureStrandsNothing(t *testing.T) {
	requireClaudeBinary(t)
	setup := setupClaudePoisonedReview(t, "claude-transport-failure")
	prompt := setup.claudeAdvisoryPrompt(t)

	before := claudeReviewerCollectStatus(t, setup.harness, setup.lineage)
	if before.NextTransition == nil || before.NextTransition.Kind != "collect" ||
		before.NextTransition.Collect == nil || len(before.NextTransition.Collect.Inputs) != 1 || before.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		t.Fatalf("fixture did not reach the reviewer collect transition: %#v", before.NextTransition)
	}

	claude, err := exec.LookPath("claude")
	if err != nil {
		t.Fatalf("resolve claude binary: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	raw, err := runClaudeReview(ctx, claude, prompt, []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")})
	if err == nil {
		t.Fatalf("Review() with an already-expired context = %q, nil, want a transport failure", raw)
	}
	t.Logf("claude transport failure (expected): %v", err)

	after := claudeReviewerCollectStatus(t, setup.harness, setup.lineage)
	if after.NextTransition == nil || after.NextTransition.Kind != "collect" ||
		after.NextTransition.Collect == nil || len(after.NextTransition.Collect.Inputs) != 1 || after.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		t.Fatalf("transport failure mutated the negotiated transition: before=%#v after=%#v", before.NextTransition, after.NextTransition)
	}
	afterBinding := organicCollectBindingFields(t, after.NextTransition.Collect.Inputs[0])
	beforeBinding := organicCollectBindingFields(t, before.NextTransition.Collect.Inputs[0])
	if afterBinding["lineage"] != beforeBinding["lineage"] ||
		afterBinding["target"] != beforeBinding["target"] ||
		afterBinding["expected-revision"] != beforeBinding["expected-revision"] ||
		afterBinding["order"] != beforeBinding["order"] ||
		afterBinding["subject-hash"] != beforeBinding["subject-hash"] {
		t.Fatalf("transport failure changed the pending collect binding: before=%#v after=%#v", beforeBinding, afterBinding)
	}
}
