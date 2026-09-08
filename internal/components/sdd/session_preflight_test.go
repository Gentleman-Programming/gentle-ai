package sdd

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const testSDDSessionPreflightInitAnchor = "### SDD Init Guard (MANDATORY)"
const testSDDSessionPreflightEntryAnchor = "### SDD Entry Routing (MANDATORY)"

func assertFallbackSessionPreflight(t *testing.T, got string) {
	t.Helper()
	open, close := "<!-- gentle-ai:sdd-session-preflight -->", "<!-- /gentle-ai:sdd-session-preflight -->"
	if strings.Count(got, open) != 1 || strings.Count(got, close) != 1 {
		t.Fatal("installed prompt requires exactly one bounded preflight")
	}
	start, end := strings.Index(got, open), strings.Index(got, close)
	if end <= start || end >= strings.Index(got, "### SDD Init Guard (MANDATORY)") {
		t.Fatal("complete preflight must precede init")
	}
	for _, want := range []string{"Interactive or Automatic", "OpenSpec, Engram, or Both", "Ask me, Single PR, or Auto", "Both -> `hybrid`", "fixed at 400", "NEVER ask it as a fourth group", "plain chat or terminal", "STOP", "Pace: <label>; Artifacts: <label>; PR strategy: <label>"} {
		if !strings.Contains(got[start:end], want) {
			t.Errorf("preflight missing %q", want)
		}
	}
	for _, stale := range []string{"`question`", "AskUserQuestion", "ALSO ASK", "ASK which execution mode", "default to **Automatic**", "default when available", "default `engram`", "ask once for and cache delivery strategy", "**Interactive** is the default behavior"} {
		if strings.Contains(got, stale) {
			t.Errorf("contradictory installed producer %q", stale)
		}
	}
	init := got[strings.Index(got, "### SDD Init Guard (MANDATORY)"):strings.Index(got, "### Execution Mode")]
	if !strings.Contains(init, "In `openspec` mode") || !strings.Contains(init, "without calling Engram") || !strings.Contains(init, "openspec/config.yaml") {
		t.Error("OpenSpec init lookup must not require Engram")
	}
}

func TestSessionPreflightActualProjectionInventory(t *testing.T) {
	paths, err := fs.Glob(assets.FS, "*/sdd-orchestrator.md")
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	for _, agent := range []model.AgentID{model.AgentVSCodeCopilot, model.AgentCursor, model.AgentGeminiCLI, model.AgentAntigravity, model.AgentQwenCode, model.AgentHermes, model.AgentKimi, model.AgentKiroIDE, model.AgentCodex, model.AgentWindsurf, model.AgentOpenCode, model.AgentKilocode, model.AgentClaudeCode} {
		t.Run(string(agent), func(t *testing.T) {
			// OpenCode/Kilo share a template. Claude's full projection is lazy;
			// its always-on bootstrap and Windsurf's native entry are consumers.
			covered[sddOrchestratorAsset(agent)] = true
			content, err := composeOpenCodeOrchestratorPrompt(agent)
			if agent == model.AgentClaudeCode {
				content, err = renderClaudeSessionPreflight()
			}
			if err != nil {
				t.Fatal(err)
			}
			open, end, err := sddSessionPreflightMarkerRange(content)
			if err != nil || open < 0 {
				t.Fatalf("missing projection: %v", err)
			}
			block := content[open:end]
			for name, invalid := range map[string]string{
				"missing":         strings.Replace(content, block, "", 1),
				"duplicate":       block + "\n" + content,
				"orphan":          strings.Replace(content, sddSessionPreflightEnd, "", 1),
				"reversed":        strings.Replace(content, block, sddSessionPreflightEnd+"\n"+sddSessionPreflightMarker, 1),
				"misplaced":       strings.Replace(content, block, "", 1) + "\n" + block,
				"stale":           strings.Replace(content, "Both -> `hybrid`", "Both -> `both`", 1),
				"wrong transport": strings.Replace(content, block, sddSessionPreflightBlockWithTool("wrong-tool"), 1),
				"mixed endings":   content + "\r\n",
				"bare CR":         content + "\r",
			} {
				if err := validateRenderedSessionPreflight(invalid, agent); err == nil {
					t.Errorf("accepted %s", name)
				}
			}
			for _, valid := range []string{content, strings.ReplaceAll(content, "\n", "\r\n")} {
				if err := validateRenderedSessionPreflight(valid, agent); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if len(paths) != len(covered) {
		t.Fatalf("actual templates %v; covered %v", paths, covered)
	}
	for _, path := range paths {
		if !covered[path] {
			t.Errorf("uncovered template %s", path)
		}
	}
}

func TestFallbackSessionPreflightCohort(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentVSCodeCopilot, model.AgentCursor, model.AgentGeminiCLI, model.AgentAntigravity, model.AgentQwenCode, model.AgentHermes, model.AgentKimi, model.AgentKiroIDE, model.AgentCodex, model.AgentWindsurf} {
		t.Run(string(agent), func(t *testing.T) {
			got := composeOrchestratorPrompt(agent)
			if agent == model.AgentCodex {
				for _, want := range []string{"{{CODEX_PHASE_EFFORTS}}", "only in `engram` or `hybrid` mode", "In `openspec` mode, use OpenSpec artifacts instead; do not call Engram for this table."} {
					if !strings.Contains(got, want) {
						t.Errorf("Codex projection missing %q", want)
					}
				}
				if strings.Contains(got, "engram default") || strings.Contains(got, "Default: `engram`") {
					t.Error("Codex projection retained an independent artifact default")
				}
			}
			for _, marker := range []string{sddSessionPreflightMarker, sddSessionPreflightEnd} {
				if strings.Count(got, marker) != 1 {
					t.Fatalf("expected exactly one %s", marker)
				}
			}
			end := strings.Index(got, sddSessionPreflightEnd)
			if end > strings.Index(got, testSDDSessionPreflightInitAnchor) {
				t.Fatal("preflight follows init")
			}
			block := got[strings.Index(got, sddSessionPreflightMarker):end]
			for _, want := range []string{"Interactive or Automatic", "OpenSpec, Engram, or Both", "Ask me, Single PR, or Auto", "Both -> `hybrid`", "Interactive -> `interactive`", "Automatic -> `auto`", "OpenSpec -> `openspec`", "Engram -> `engram`", "Ask me -> `ask-on-risk`", "Single PR -> `single-pr`", "Auto -> `auto-chain`", "fixed at 400", "NEVER ask it as a fourth group", "plain chat or terminal", "STOP", "Pace: <label>; Artifacts: <label>; PR strategy: <label>"} {
				if !strings.Contains(block, want) {
					t.Errorf("preflight missing %q", want)
				}
			}
			for _, stale := range []string{"`question`", "AskUserQuestion", "ALSO ASK", "ASK which execution mode", "default to **Automatic**", "default when available", "default `engram`", "ask once for and cache delivery strategy", "**Interactive** is the default behavior"} {
				if strings.Contains(got, stale) {
					t.Errorf("stale producer or native vocabulary %q", stale)
				}
			}
		})
	}
}

func TestFallbackSessionPreflightProjectionBoundaries(t *testing.T) {
	anchor := testSDDSessionPreflightInitAnchor
	template := "prefix\n" + anchor + "\nsuffix"
	got, err := projectSDDSessionPreflightWithTool(template, anchor, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, content string
		valid         bool
	}{
		{"canonical", got, true},
		{"CRLF", strings.ReplaceAll(got, "\n", "\r\n"), true},
		{"missing", template, false},
		{"duplicate", sddSessionPreflightBlockWithTool("") + "\n" + got, false},
		{"after init", template + "\n" + sddSessionPreflightBlockWithTool(""), false},
		{"native transport", strings.Replace(got, sddSessionPreflightBlockWithTool(""), sddSessionPreflightBlock(), 1), false},
		{"wrong mapping", strings.ReplaceAll(got, "Both -> `hybrid`", "Both -> `both`"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSDDSessionPreflightProjection(tc.content, anchor, ""); (err == nil) != tc.valid {
				t.Fatalf("validation = %v; valid = %v", err, tc.valid)
			}
		})
	}
	if again, err := projectSDDSessionPreflightWithTool(got, anchor, ""); err != nil || again != got {
		t.Fatalf("projection not idempotent: %v", err)
	}
}

func TestClaudeSessionPreflightProjectionStructure(t *testing.T) {
	const tool = "AskUserQuestion"
	anchor := testSDDSessionPreflightEntryAnchor
	template := "prefix\n" + anchor + "\n" + sddSessionPreflightInit + "\nsuffix"
	got, err := projectSDDSessionPreflightWithTool(template, anchor, tool)
	if err != nil {
		t.Fatal(err)
	}
	block := strings.ReplaceAll(sddSessionPreflightBlock(), "`question`", "`AskUserQuestion`")
	for _, tc := range []struct {
		name, content string
		valid         bool
	}{
		{"canonical", got, true},
		{"CRLF", strings.ReplaceAll(got, "\n", "\r\n"), true},
		{"missing", template, false},
		{"duplicate", block + "\n" + got, false},
		{"after init", template + "\n" + block, false},
		{"after routing", strings.Replace(template, anchor, anchor+"\n"+block, 1), false},
		{"missing close", strings.Replace(got, sddSessionPreflightEnd, "", 1), false},
		{"wrong tool", strings.ReplaceAll(got, "`AskUserQuestion`", "`question`"), false},
		{"legacy mapping", strings.ReplaceAll(got, "Both -> `hybrid`", "Both -> `both`"), false},
		{"duplicate init", got + "\n" + sddSessionPreflightInit, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSDDSessionPreflightProjection(tc.content, anchor, tool)
			if (err == nil) != tc.valid {
				t.Fatalf("validation = %v; valid = %v", err, tc.valid)
			}
		})
	}
	if again, err := projectSDDSessionPreflightWithTool(got, anchor, tool); err != nil || again != got {
		t.Fatalf("projection not idempotent: %v", err)
	}
}

func TestOpenCodeSessionPreflightCompositionIsRuntimeScoped(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		content, err := composeOpenCodeOrchestratorPrompt(agent)
		if err != nil {
			t.Fatalf("composeOpenCodeOrchestratorPrompt(%s) error = %v", agent, err)
		}
		if !strings.Contains(content, sddSessionPreflightMarker) || !strings.Contains(content, "Both -> `hybrid`") {
			t.Fatalf("composeOpenCodeOrchestratorPrompt(%s) omitted canonical session preflight", agent)
		}
	}
	claude, err := composeOpenCodeOrchestratorPrompt(model.AgentClaudeCode)
	if err != nil {
		t.Fatalf("composeOpenCodeOrchestratorPrompt(claude) error = %v", err)
	}
	if strings.Contains(claude, sddSessionPreflightMarker) {
		t.Fatal("Claude composition unexpectedly received the OpenCode session preflight projection")
	}
}
func TestMigratePreservedSDDSessionPreflightReplacesOnlyOwnedBytes(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		for _, testCase := range []struct {
			name, prompt, want string
		}{
			{name: "unmarked", prompt: "PREFIX" + newline + "SUFFIX", want: "PREFIX" + newline + "SUFFIX" + newline + newline},
			{name: "legacy", prompt: "PREFIX" + newline + legacySDDSessionPreflightMarker + newline + "Both -> `both`" + newline + legacySDDSessionPreflightEnd + newline + "SUFFIX", want: "PREFIX" + newline},
			{name: "canonical", prompt: "PREFIX" + newline + sddSessionPreflightMarker + newline + "Both -> `both`" + newline + sddSessionPreflightEnd + newline + "SUFFIX", want: "PREFIX" + newline},
		} {
			t.Run(testCase.name+"/"+strings.ReplaceAll(newline, "\r", "cr"), func(t *testing.T) {
				got, err := migratePreservedSDDSessionPreflight(testCase.prompt)
				if err != nil {
					t.Fatal(err)
				}
				block := strings.ReplaceAll(sddSessionPreflightBlock(), "\n", newline)
				want := testCase.want + block
				if testCase.name != "unmarked" {
					want += newline + "SUFFIX"
				}
				if got != want || strings.Contains(got, "Both -> `both`") {
					t.Fatalf("migration = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestSDDSessionPreflightProjectionCanonicalAndBounded(t *testing.T) {
	block := sddSessionPreflightBlock()
	for _, want := range []string{"<!-- gentle-ai:sdd-session-preflight -->", "### SDD Session Preflight (HARD GATE)", "1. **Pace**", "2. **Artifacts**", "3. **PR strategy**", "Both -> `hybrid`", "fixed at 400 changed lines", "<!-- /gentle-ai:sdd-session-preflight -->"} {
		if !strings.Contains(block, want) {
			t.Fatalf("canonical block missing %q", want)
		}
	}
	for _, retired := range []string{"4. **", "Both -> `both`", "Review: 800 lines", "Other ->"} {
		if strings.Contains(block, retired) {
			t.Fatalf("canonical block retains retired content %q", retired)
		}
	}
	for _, newline := range []string{"\n", "\r\n"} {
		rendered := strings.Join([]string{"before", testSDDSessionPreflightEntryAnchor, testSDDSessionPreflightInitAnchor, "after"}, newline)
		got, err := projectSDDSessionPreflight(rendered, testSDDSessionPreflightEntryAnchor)
		if err != nil {
			t.Fatalf("projectSDDSessionPreflight() error = %v", err)
		}
		want := strings.Join([]string{"before", strings.ReplaceAll(block, "\n", newline), testSDDSessionPreflightEntryAnchor, testSDDSessionPreflightInitAnchor, "after"}, newline)
		if got != want {
			t.Fatalf("projected preflight = %q, want %q", got, want)
		}
	}
	for _, rendered := range []string{"prefix " + testSDDSessionPreflightEntryAnchor + "\n" + testSDDSessionPreflightInitAnchor, testSDDSessionPreflightEntryAnchor + " suffix\n" + testSDDSessionPreflightInitAnchor} {
		if _, err := projectSDDSessionPreflight(rendered, testSDDSessionPreflightEntryAnchor); err == nil {
			t.Fatal("projectSDDSessionPreflight() accepted an inline anchor")
		}
	}
}
