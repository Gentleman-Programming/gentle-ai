package sdd

import (
	"bytes"
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
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	claudeRuntimeVersion = "2.1.220 (Claude Code)"
	claudeRuntimeSHA256  = "674f61f20ff306f3100cf9200e4c36c4b70278b5bef2884549819b942a89c863"
	claudeRuntimeFakeKey = "gentle-ai-network-none-mock-key"
)

type claudeRuntimeResult struct {
	SubjectHash string                               `json:"subject_hash"`
	Inspection  reviewtransaction.ArtifactInspection `json:"inspection"`
	Findings    []reviewtransaction.Finding          `json:"findings"`
	Evidence    []string                             `json:"evidence"`
}

func boundClaudeRuntime(t *testing.T) string {
	t.Helper()
	if testing.Short() || os.Getenv("GENTLE_AI_CLAUDE_RUNTIME_E2E") != "1" {
		t.Skip("claude_network_none_skipped: requires explicit opt-in and the prebuilt Docker image")
	}
	binary := os.Getenv("GENTLE_AI_CLAUDE_RUNTIME_BINARY")
	payload, err := os.ReadFile(binary)
	if err != nil {
		if os.Getenv("GENTLE_AI_CLAUDE_RUNTIME_REQUIRED") == "1" {
			t.Fatalf("claude_network_none_unavailable: pinned binary unavailable: %v", err)
		}
		t.Skipf("claude_network_none_unavailable: prebuilt Docker image unavailable: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != claudeRuntimeSHA256 {
		t.Fatalf("Claude binary SHA-256 = %s, want %s", got, claudeRuntimeSHA256)
	}
	return binary
}
func requireNetworkNone(t *testing.T) {
	t.Helper()
	namespace, err := os.Readlink("/proc/self/ns/net")
	parent := os.Getenv("GENTLE_AI_PARENT_NETNS")
	if err != nil || parent == "" || strings.Trim(namespace, "net:[]") == parent {
		t.Fatalf("Claude proof does not have a distinct parent/container network namespace: child=%q parent=%q error=%v", namespace, parent, err)
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
}
func decodeClaudeRuntimeResult(raw string) (claudeRuntimeResult, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result claudeRuntimeResult
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return result, errors.New("reviewer result must contain exactly one JSON object")
	}
	if result.SubjectHash == "" || result.Inspection.Paths == nil || result.Findings == nil || len(result.Evidence) == 0 {
		return result, errors.New("reviewer result omits required fields")
	}
	return result, nil
}
func claudePromptComplete(payload []byte) bool {
	zero, one := bytes.Index(payload, []byte("path_index: 0")), bytes.Index(payload, []byte("path_index: 1"))
	return zero >= 0 && one > zero && bytes.Contains(payload, []byte("ErrMalformedPort")) && bytes.Contains(payload, []byte("return nil")) &&
		bytes.Count(payload, []byte("parser.go")) >= 2 && bytes.Count(payload, []byte("parser_test.go")) >= 2 &&
		!bytes.Contains(payload, []byte("other_test.go")) && !bytes.Contains(payload, []byte("patch: unavailable"))
}
func claudeMockResult(subject string, completed, unknown bool) string {
	result := claudeRuntimeResult{SubjectHash: subject, Inspection: reviewtransaction.ArtifactInspection{Status: "incomplete", Paths: []string{}}, Findings: []reviewtransaction.Finding{}, Evidence: []string{"immutable evidence was missing, partial, reordered, mismatched, or unavailable"}}
	if completed {
		result.Inspection = reviewtransaction.ArtifactInspection{Status: reviewtransaction.ArtifactInspectionCompleted, Paths: []string{"parser.go", "parser_test.go"}}
		result.Findings = []reviewtransaction.Finding{{Location: "parser.go:2", Severity: "CRITICAL", Claim: "Malformed ports are accepted", EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced, ProofRefs: []string{"changed patch replaces ErrMalformedPort with return nil"}}}
		result.Evidence = []string{"parser.go changes ErrMalformedPort to return nil, so malformed input succeeds"}
	}
	payload, _ := json.Marshal(result)
	if unknown {
		payload = append(payload[:len(payload)-1], []byte(`,"summary":"forbidden"}`)...)
	}
	return string(payload)
}
func writeClaudeSSE(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "text/event-stream")
	text, _ := json.Marshal(result)
	_, _ = fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-5\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":10}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", text)
}
func TestClaudeReviewerTransportInNetworkNone(t *testing.T) {
	claude := boundClaudeRuntime(t)
	requireNetworkNone(t)
	defer requireNetworkNone(t)
	version := exec.Command(claude, "--version")
	version.Env = []string{"HOME=" + t.TempDir(), "LANG=C", "LC_ALL=C"}
	if output, err := version.Output(); err != nil || strings.TrimSpace(string(output)) != claudeRuntimeVersion {
		t.Fatalf("Claude version = %q, want %q (error %v)", strings.TrimSpace(string(output)), claudeRuntimeVersion, err)
	}
	installHome := t.TempDir()
	if _, err := Inject(installHome, claudeAdapter(), ""); err != nil {
		t.Fatal(err)
	}
	installed, _ := os.ReadFile(filepath.Join(installHome, ".claude", "agents", "review-reliability.md"))
	parts := strings.SplitN(string(installed), "---", 3)
	if len(parts) != 3 {
		t.Fatal("installed Claude reviewer has no frontmatter")
	}
	agents, _ := json.Marshal(map[string]any{"gentle-ai-review-transport-e2e": map[string]any{"description": "Exercises the generated Claude reviewer transport.", "prompt": strings.TrimSpace(parts[2]), "model": "sonnet", "tools": []string{"Read", "Grep", "Glob"}}})
	subject := "sha256:" + strings.Repeat("a", 64)
	const evidenceA = "- path_index: 0\n  path: parser.go\n  patch: |\n    -    return ErrMalformedPort\n    +    return nil"
	const evidenceB = "- path_index: 1\n  path: parser_test.go\n  patch: |\n    -    require.Error(t, err)\n    +    require.NoError(t, err)"
	prompt := `GENTLE_AI_REVIEW_BINDING {"lineage":"claude-runtime-e2e","target":"fixture","lens":"review-reliability","order":0,"revision":"1","repository_context":"opaque-fixture","subject_hash":"` + subject + `"}
GENTLE_AI_CLAUDE_REVIEW_CONTEXT
artifact_subject: {"subject_hash":"` + subject + `"}
base_tree: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
candidate_tree: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
changed_path_manifest: [{"path":"parser.go","status":"M"},{"path":"parser_test.go","status":"M"}]
name_status: "M\tparser.go\nM\tparser_test.go"
numstat: "1\t1\tparser.go\n1\t1\tparser_test.go"
path_evidence:
` + evidenceA + "\n" + evidenceB + "\nGENTLE_AI_CLAUDE_REVIEW_CONTEXT_END"
	for _, test := range []struct {
		name, prompt string
		unknown      bool
	}{
		{"complete ordered patch evidence", prompt, false},
		{"missing evidence", strings.Replace(prompt, evidenceA+"\n"+evidenceB, "missing", 1), false},
		{"partial evidence", strings.Replace(prompt, "\n"+evidenceB, "", 1), false},
		{"reordered evidence", strings.Replace(prompt, evidenceA+"\n"+evidenceB, evidenceB+"\n"+evidenceA, 1), false},
		{"mismatched evidence", strings.Replace(prompt, "path: parser_test.go", "path: other_test.go", 1), false},
		{"unavailable evidence", strings.Replace(prompt, evidenceA, "- path_index: 0\n  path: parser.go\n  patch: unavailable", 1), false},
		{"unknown result field", prompt, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var sawPrompt atomic.Bool
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
				sawPrompt.Store(bytes.Contains(body, []byte("claude-runtime-e2e")))
				writeClaudeSSE(w, claudeMockResult(subject, claudePromptComplete(body), test.unknown))
			})}}
			server.Start()
			defer server.Close()
			attacker, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", strings.NewReader("claude-runtime-e2e"))
			attacker.Host = "attacker.invalid"
			response, err := server.Client().Do(attacker)
			if err != nil || response.StatusCode != http.StatusMisdirectedRequest || sawPrompt.Load() {
				t.Fatalf("attacker Host reached prompt handling: response=%v prompt=%v error=%v", response, sawPrompt.Load(), err)
			}
			_ = response.Body.Close()
			home := t.TempDir()
			settings := filepath.Join(home, "settings.json")
			if err := os.WriteFile(settings, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(claude, "--bare", "--print", "--agent", "gentle-ai-review-transport-e2e", "--agents", string(agents), "--tools", "Read,Grep,Glob", "--disallowedTools", "Bash,Write,Edit", "--permission-mode", "dontAsk", "--settings", settings, "--setting-sources=", "--strict-mcp-config", "--disable-slash-commands", "--no-chrome", "--no-session-persistence", "--output-format", "json", "--prompt-suggestions=false", test.prompt)
			command.Dir = t.TempDir()
			command.Env = []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude"), "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"), "TMPDIR=" + command.Dir, "ANTHROPIC_API_KEY=" + claudeRuntimeFakeKey, "ANTHROPIC_BASE_URL=" + server.URL, "NO_PROXY=127.0.0.1,localhost", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "LANG=C", "LC_ALL=C"}
			output, err := command.Output()
			var envelope struct {
				IsError bool   `json:"is_error"`
				Result  string `json:"result"`
			}
			if decodeErr := json.Unmarshal(output, &envelope); err != nil || decodeErr != nil || envelope.IsError || !sawPrompt.Load() {
				t.Fatalf("invalid Claude protocol result: run=%v decode=%v result=%+v prompt=%v", err, decodeErr, envelope, sawPrompt.Load())
			}
			result, err := decodeClaudeRuntimeResult(envelope.Result)
			if test.unknown {
				if err == nil {
					t.Fatal("reviewer result accepted an unknown field")
				}
				return
			}
			if err != nil || result.SubjectHash != subject {
				t.Fatalf("invalid reviewer result: %+v, %v", result, err)
			}
			if test.name == "complete ordered patch evidence" {
				canonical, canonicalErr := reviewtransaction.CanonicalLensResult(reviewtransaction.LensResult{Lens: reviewtransaction.LensReliability, Findings: result.Findings, Evidence: result.Evidence})
				proof, _ := json.Marshal(canonical)
				if canonicalErr != nil || result.Inspection.Status != reviewtransaction.ArtifactInspectionCompleted || !slices.Equal(result.Inspection.Paths, []string{"parser.go", "parser_test.go"}) || len(canonical.Findings) == 0 || !strings.Contains(strings.ToLower(string(proof)), "errmalformedport") || !strings.Contains(strings.ToLower(string(proof)), "return nil") {
					t.Fatalf("completed result lacks native patch proof: %+v, %v", result, canonicalErr)
				}
			} else if result.Inspection.Status != "incomplete" || len(result.Inspection.Paths) != 0 || len(result.Findings) != 0 {
				t.Fatalf("invalid evidence did not fail closed: %+v", result)
			}
		})
	}
}
