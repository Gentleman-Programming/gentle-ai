package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const testSDDSessionPreflightInitAnchor = "### SDD Init Guard (MANDATORY)"
const testSDDSessionPreflightEntryAnchor = "### SDD Entry Routing (MANDATORY)"

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
