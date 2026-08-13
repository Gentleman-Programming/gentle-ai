// Retirement note (change #3138, slice 6).
//
// This file previously held the gated organic conformance proofs for the
// OpenCode plugin-interception transport: a real review-driver launch against
// a poisoned worktree with the legacy plugin (review-result-artifacts.ts)
// injecting the frozen provider context, plus their fixture
// (setupOpenCodePoisonedReview, the review-driver model fixture server,
// generatedOpenCodeReviewConfig, the runtime-pin helpers, and the transcript
// scanners no other test used). Those proofs demonstrated a mechanism this
// change removes: the legacy plugin's review half is gone and its SDD half is
// native Go (sdd_task_result.go), so the in-session interception no longer
// exists and the CI gate (GENTLE_AI_REAL_AGENT_E2E=1) would fail their
// fixture's stat on the deleted plugin. The organic replacement proof for the
// Go-backed shim stays deferred: the task plan defines no dispatch-activation
// slice, and that debt is recorded in the change's apply-progress.
//
// What remains here is still live and shared with the organic/codex/claude
// proofs: the negotiated-transition argument parsing and quoting family, the
// status binding/collect helpers, and the bash/read tool-use detector that
// pins the generated review-risk agent's no-tool contract
// (TestBashOrReadToolUseDetectsRegression).
package organicruntime_test

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

// organicCommandArguments removes POSIX shell quoting before passing Windows CWDs to Go flags.
func organicCommandArguments(t *testing.T, command string) []string {
	t.Helper()
	fields, err := organicCommandWords(command)
	if err != nil {
		t.Fatalf("parse negotiated transition command %q: %v", command, err)
	}
	if len(fields) == 0 || fields[0] != "gentle-ai" {
		t.Fatalf("negotiated transition command does not start with gentle-ai: %q", command)
	}
	return append([]string(nil), fields[1:]...)
}
func organicCommandWords(command string) ([]string, error) {
	words := []string{}
	var word strings.Builder
	inQuote := false
	for index := 0; index < len(command); index++ {
		char := command[index]
		switch {
		case char == '\'':
			inQuote = !inQuote
		case char == '\\' && !inQuote && index+1 < len(command):
			index++
			word.WriteByte(command[index])
		case (char == ' ' || char == '\t') && !inQuote:
			if word.Len() != 0 {
				words = append(words, word.String())
				word.Reset()
			}
		default:
			word.WriteByte(char)
		}
	}
	if inQuote {
		return nil, errors.New("unterminated shell quote")
	}
	if word.Len() != 0 {
		words = append(words, word.String())
	}
	return words, nil
}
func TestOrganicCommandArgumentsPreservesQuotedWindowsCWD(t *testing.T) {
	arguments := organicCommandArguments(t, "gentle-ai review start --cwd='C:\\Users\\reviewer name\\repo' --contract=gentle-ai.review-integration/v2")
	want := []string{"review", "start", "--cwd=C:\\Users\\reviewer name\\repo", "--contract=gentle-ai.review-integration/v2"}
	if len(arguments) != len(want) {
		t.Fatalf("argv = %#v, want %#v", arguments, want)
	}
	for index, value := range want {
		if got := arguments[index]; got != value || strings.ContainsAny(got, "'\"") {
			t.Fatalf("argv[%d] = %q, want unquoted %q", index, got, value)
		}
	}
}
func TestOrganicCommandArgumentsExecuteStartWithWindowsCWD(t *testing.T) {
	harness := newOrganicHarness(t)
	spacedWorktree := harness.repo.worktree + " space"
	if err := os.Rename(harness.repo.worktree, spacedWorktree); err != nil {
		t.Fatal(err)
	}
	harness.repo.worktree = spacedWorktree
	harness.writeFiles(map[string]string{"docs/candidate.md": "candidate\n"})
	harness.git("commit", "-qm", "test: spaced cwd candidate")
	status := organicNegotiatedStatus(t, harness, "windows-cwd-start")
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("STATUS transition = %#v", status.NextTransition)
	}
	publishedCWD := ""
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Name == "cwd" {
			publishedCWD = argument.Value
			break
		}
	}
	if !strings.Contains(status.NextTransition.Execute.Command, pathquote.ShellWord("--cwd="+publishedCWD)) {
		t.Fatalf("START command does not safely render the spaced cwd: %q", status.NextTransition.Execute.Command)
	}
	arguments := organicCommandArguments(t, status.NextTransition.Execute.Command)
	if got := organicArgumentValue(arguments, "--cwd"); got != publishedCWD || strings.ContainsAny(got, "'\"") {
		t.Fatalf("START argv cwd = %q, want unquoted %q (argv %#v)", got, publishedCWD, arguments)
	}
	if !sameOrganicDirectory(publishedCWD, spacedWorktree) {
		t.Fatalf("START cwd %q does not identify worktree %q", publishedCWD, spacedWorktree)
	}
	harness.gentle(arguments...)
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
	Operation string                      `json:"operation"`
	Command   string                      `json:"command"`
	Arguments []organicNegotiatedArgument `json:"arguments"`
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
