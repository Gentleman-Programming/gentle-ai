package organicruntime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
)

func TestRealOpenCodeRejectsUnsupportedImmutableReviewerBash(t *testing.T) {
	if os.Getenv(realAgentE2EEnvironment) != "1" {
		t.Skip("set GENTLE_AI_REAL_AGENT_E2E=1 to run the real OpenCode reviewer rejection proof")
	}
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only proof for OpenCode 1.18.10 Bash permission behavior")
	}
	requireOrganicExecutableVersion(t, "opencode", pinnedOpenCodeVersion)

	harness := newOrganicHarness(t)
	home := t.TempDir()
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

	marker := filepath.Join(harness.repo.worktree, "reviewer-bash-ran")
	prompt := "Attempt this immutable Bash inspection exactly once: gentle-ai review inspect-candidate " +
		"--repository-context rctx1_" + strings.Repeat("a", 64) +
		" --expected-revision sha256:" + strings.Repeat("b", 64) +
		" --lineage reviewer-e2e --target sha256:" + strings.Repeat("d", 64) +
		" --lens review-risk --order 0 --operation name-status > " + marker
	reviewerPrompt := "GENTLE_AI_REVIEWER_LAUNCH_PROBE\n" + prompt
	fixture := newOpenCodeFixtureServer(t, []openCodeTurn{{tool: "task", arguments: map[string]any{
		"description":   "Attempt immutable reviewer inspection",
		"subagent_type": "review-risk",
		"prompt": "GENTLE_AI_REVIEW_BINDING {\"lineage\":\"reviewer-e2e\",\"target\":\"sha256:" + strings.Repeat("d", 64) +
			"\",\"lens\":\"review-risk\",\"order\":0,\"repository_context\":\"rctx1_" + strings.Repeat("a", 64) +
			"\",\"revision\":\"sha256:" + strings.Repeat("b", 64) + "\",\"subject_hash\":\"sha256:" + strings.Repeat("c", 64) + "\"}\n" + reviewerPrompt,
	}}}, reviewerPrompt)
	defer fixture.Close()
	assertFixtureRecognizesReviewerRequest(t, fixture, reviewerPrompt)

	config := generatedOpenCodeReviewConfig(t, filepath.Join(configRoot, "opencode", "opencode.json"), fixture.URL)
	environment := replaceOrganicEnvironment(organicEnvironment(home), map[string]string{
		"XDG_CONFIG_HOME":                           configRoot,
		"XDG_CACHE_HOME":                            t.TempDir(),
		"OPENCODE_CONFIG_DIR":                       filepath.Join(configRoot, "opencode"),
		"OPENCODE_TEST_HOME":                        filepath.Join(home, "opencode"),
		"OPENCODE_CONFIG_CONTENT":                   config,
		"OPENCODE_AUTH_CONTENT":                     "{}",
		"OPENCODE_DISABLE_PROJECT_CONFIG":           "1",
		"OPENCODE_DISABLE_AUTOUPDATE":               "1",
		"OPENCODE_DISABLE_AUTOCOMPACT":              "1",
		"OPENCODE_DISABLE_CLAUDE_CODE":              "1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS":          "1",
		"OPENCODE_DISABLE_EXTERNAL_SKILLS":          "1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD":             "1",
		"OPENCODE_DISABLE_MODELS_FETCH":             "1",
		"OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER": "1",
		"OPENCODE_FAST_BOOT":                        "1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), organicAgentTimeout)
	defer cancel()
	command := organicCommandContext(ctx, "opencode", "run", "--format", "json", "--agent", "review-driver", "--model", "fixture/fixture", "--dir", harness.repo.worktree, "Delegate the immutable review inspection.")
	command.Dir = harness.repo.worktree
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("opencode run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	fixture.mu.Lock()
	launches := fixture.subagentStarts
	fixture.mu.Unlock()
	accepted, err := reviewerRejectedBeforeLaunch(stdout.String(), launches)
	if err != nil {
		t.Fatalf("decode OpenCode JSON event: %v\n%s", err, stdout.String())
	}
	if !accepted {
		t.Fatalf("OpenCode did not emit the exact unsupported-capability task rejection before reviewer launch:\n%s", stdout.String())
	}
	if launches != 0 {
		t.Fatalf("rejected reviewer Bash launched %d reviewer subagents", launches)
	}
	fixture.assertComplete(t, false)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected reviewer Bash changed the worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(harness.commonDir(), "gentle-ai", "review-transactions", "v2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected reviewer Bash created review authority: %v", err)
	}
}

func reviewerRejectedBeforeLaunch(output string, launches int) (bool, error) {
	decoder := json.NewDecoder(strings.NewReader(output))
	qualifying := 0
	invalid := false
	for {
		var event struct {
			Type string `json:"type"`
			Part *struct {
				Type  string `json:"type"`
				Tool  string `json:"tool"`
				State struct {
					Status string `json:"status"`
					Error  string `json:"error"`
				} `json:"state"`
			} `json:"part"`
		}
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			return launches == 0 && !invalid && qualifying == 1, nil
		} else if err != nil {
			return false, err
		}
		if event.Type != "tool_use" {
			continue
		}
		if event.Part == nil || event.Part.Type != "tool" || event.Part.Tool != "task" ||
			event.Part.State.Status != "error" || event.Part.State.Error != "unsupported-capability" {
			invalid = true
			continue
		}
		qualifying++
	}
}

func TestReviewerRejectedBeforeLaunch(t *testing.T) {
	const valid = `{"type":"tool_use","part":{"type":"tool","tool":"task","state":{"status":"error","error":"unsupported-capability"}}}`
	tests := []struct {
		name     string
		output   string
		launches int
		want     bool
		wantErr  bool
	}{
		{name: "exact task rejection", output: valid, want: true},
		{name: "malformed JSON", output: `{`, wantErr: true},
		{name: "missing event", output: `{"type":"text"}`},
		{name: "wrong tool", output: strings.Replace(valid, `"tool":"task"`, `"tool":"bash"`, 1)},
		{name: "wrong status", output: strings.Replace(valid, `"status":"error"`, `"status":"completed"`, 1)},
		{name: "unrelated substring", output: strings.Replace(valid, "unsupported-capability", "unrelated unsupported-capability diagnostic", 1)},
		{name: "rejection after launch", output: valid, launches: 1},
		{name: "duplicate rejection", output: valid + "\n" + valid},
		{name: "unrelated event before rejection", output: strings.Replace(valid, `"tool":"task"`, `"tool":"bash"`, 1) + "\n" + valid},
		{name: "malformed trailing event", output: valid + "\n{", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := reviewerRejectedBeforeLaunch(test.output, test.launches)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("reviewerRejectedBeforeLaunch() = (%t, %v), want (%t, error=%t)", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestOpenCodeFixtureRecognizesReviewerRequest(t *testing.T) {
	fixture := newOpenCodeFixtureServer(t, nil, "GENTLE_AI_REVIEWER_LAUNCH_PROBE")
	defer fixture.Close()
	assertFixtureRecognizesReviewerRequest(t, fixture, "GENTLE_AI_REVIEWER_LAUNCH_PROBE")
}

func assertFixtureRecognizesReviewerRequest(t *testing.T, fixture *openCodeFixtureServer, prompt string) {
	t.Helper()
	if !fixture.isSubagent(openAIRequest{Messages: []openAIMessage{{Role: "user", Content: prompt}}}) {
		t.Fatal("reviewer launch detector does not recognize a representative child request")
	}
}

func generatedOpenCodeReviewConfig(t *testing.T, settingsPath, serverURL string) string {
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
