package assets_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

// openCodeRuntimeCommand matches every review verb that negotiates the runtime
// identity: assess and STATUS carry it onto the next transition, and START
// binds consent to it. Without --agent the consent envelope defaults to
// claude-code (a documented contract), so an OpenCode asset that omits it hands
// an OpenCode session a Claude Code review (#4805, #4808). The tail is the rest
// of the inline command, up to its closing backtick or the end of the line.
var openCodeRuntimeCommand = regexp.MustCompile("gentle-ai review (assess|status|start)\\b([^`\\n]*)")

// TestShippedOpenCodeAssetsDeclareTheOpenCodeRuntime is the ratchet over every
// text Gentle AI installs for OpenCode, as rendered for OpenCode V1 and V2, and
// over the rendered Claude Code and Codex guidance: every runtime declares
// itself.
func TestShippedOpenCodeAssetsDeclareTheOpenCodeRuntime(t *testing.T) {
	for _, tt := range []struct {
		name    string
		agent   model.AgentID
		version string
	}{
		{name: "opencode V1", agent: model.AgentOpenCode, version: "1.18.30"},
		{name: "opencode V2", agent: model.AgentOpenCode, version: "2.0.23"},
		{name: "claude-code", agent: model.AgentClaudeCode},
		{name: "codex", agent: model.AgentCodex},
	} {
		t.Run(tt.name, func(t *testing.T) {
			old := opencode.VersionRunnerOverride
			t.Cleanup(func() { opencode.VersionRunnerOverride = old })
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte(tt.version)}, nil
			}
			texts := renderedRuntimeGuidance(t, tt.agent)
			if tt.agent == model.AgentOpenCode {
				for name, text := range shippedOpenCodeTexts(t) {
					texts[name] = text
				}
			}
			commands := 0
			for name, text := range texts {
				for _, match := range openCodeRuntimeCommand.FindAllStringSubmatch(text, -1) {
					commands++
					if !strings.Contains(match[2], "--agent "+string(tt.agent)+" ") {
						t.Errorf("%s: %q does not declare --agent %s", name, match[0], tt.agent)
					}
				}
			}
			if commands == 0 {
				t.Fatalf("no shipped %s text invokes a runtime-negotiating review command; this ratchet would pass vacuously", tt.agent)
			}
		})
	}
}

// renderedRuntimeGuidance renders the orchestrator and routing block agent
// receives, with the runtime identity bound exactly as the renderers bind it.
func renderedRuntimeGuidance(t *testing.T, agent model.AgentID) map[string]string {
	t.Helper()
	orchestrator, err := agentguidance.RenderOrchestratorWithSource(agent, reviewassets.ReviewExecutionContractFor, "")
	if err != nil {
		t.Fatal(err)
	}
	routing, err := agentguidance.RenderRouting(agent)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"rendered orchestrator": orchestrator, "rendered routing block": routing}
}

// shippedOpenCodeTexts returns every embedded OpenCode asset and installed
// shared skill text, keyed by origin, with the runtime identity bound to
// OpenCode, plus the managed OpenCode agent entries.
func shippedOpenCodeTexts(t *testing.T) map[string]string {
	t.Helper()
	texts := map[string]string{}
	for _, root := range []string{"opencode", "skills"} {
		err := fs.WalkDir(assets.FS, root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || path == "opencode/orchestrator.md" || path == "skills/_shared/odd-orchestrator-sections.md" {
				return walkErr
			}
			content, readErr := fs.ReadFile(assets.FS, path)
			texts[path] = strings.ReplaceAll(string(content), "{{GENTLE_AI_RUNTIME_AGENT_ID}}", string(model.AgentOpenCode))
			return readErr
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	entries := map[string]map[string]any{"refuter agent": opencodeagents.Refuter(), "validator agent": opencodeagents.Validator()}
	for _, spec := range opencodeagents.Parity(model.AgentOpenCode) {
		entry, err := opencodeagents.Entry(spec)
		if err != nil {
			t.Fatal(err)
		}
		entries["agent "+spec.Name] = entry
	}
	for name, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		texts[name] = string(encoded)
	}
	return texts
}
