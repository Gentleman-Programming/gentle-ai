package sdd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const testGenericFallbackOnlyNativeRoute = "- Native route: This variant has no classified native question UI for this contract; always use the plain chat or terminal fallback below. When the closed domain of a single-select envelope is unrepresentable here, fall through to the Fallback clause below."

const testPiClosedSingleSelectNativeRoute = "- Native route: For every strictly closed single-select envelope, use ask_user_choice only when the interactive Pi TUI can represent its complete one-question 2-4 ordered-option domain. Pass each user-facing label and description with the envelope-owned canonical option token as value. The selector returns exactly one value; map it to the exact envelope-owned choice once, then select any envelope-owned continuation or invocation once where present. It has no custom/free-text or multi-select path. If the native TUI is unavailable or the envelope is not exactly representable, use the complete chat fallback. ask_user_question is the external open/free-text questionnaire and must not be used for a closed domain; open/free-text questionnaires may use ask_user_question when exactly representable. For gentle-ai.review-integration.consent/v3, the chosen continuation is still the exact captured provider-owned choice invocation, used once without synthesis."

func TestCanonicalCompositionPreservesHistoricalOrchestratorBytes(t *testing.T) {
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			path := sddOrchestratorAsset(agent.ID)
			before := renderBoundedReviewAsset(agent.ID, path)
			if agent.ID == model.AgentPi {
				before = strings.Replace(before, testGenericFallbackOnlyNativeRoute, testPiClosedSingleSelectNativeRoute, 1)
			}
			after := composeOrchestratorPrompt(agent.ID)
			if after != before {
				t.Fatalf("canonical composition changed %s orchestrator bytes", agent.ID)
			}
		})
	}
}

func TestPiClosedChoiceRouteReplacesTheGenericFallback(t *testing.T) {
	pi := composeOrchestratorPrompt(model.AgentPi)
	if got := strings.Count(pi, testGenericFallbackOnlyNativeRoute); got != 0 {
		t.Fatalf("Pi composition contains %d generic fallback-only route clauses, want 0", got)
	}
	if got := strings.Count(pi, testPiClosedSingleSelectNativeRoute); got != 1 {
		t.Fatalf("Pi composition contains %d closed-choice route clauses, want 1", got)
	}

	generic := renderBoundedReviewAsset(model.AgentPi, sddOrchestratorAsset(model.AgentPi))
	if got := strings.Count(generic, testGenericFallbackOnlyNativeRoute); got != 1 {
		t.Fatalf("generic source contains %d fallback-only route clauses, want 1", got)
	}
	if strings.Contains(generic, testPiClosedSingleSelectNativeRoute) {
		t.Fatal("generic source contains the Pi closed-choice route")
	}

	if strings.Contains(composeOrchestratorPrompt(model.AgentKilocode), testPiClosedSingleSelectNativeRoute) {
		t.Fatal("Kilo composition received the Pi closed-choice route")
	}
}

func TestPiClosedChoiceRouteFailsClosedWhenGenericSourceClauseIsNotUnique(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
	}{
		{name: "absent", content: "- Native route: custom runtime route"},
		{name: "duplicated", content: testGenericFallbackOnlyNativeRoute + "\n" + testGenericFallbackOnlyNativeRoute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				recovered := recover()
				if recovered == nil || !strings.Contains(fmt.Sprint(recovered), "Pi native route source clause count") {
					t.Fatalf("replacePiClosedSingleSelectRoute() panic = %v, want source clause count failure", recovered)
				}
			}()
			replacePiClosedSingleSelectRoute(tt.content, model.AgentPi)
		})
	}
}

func TestOpenCodeBackgroundPolicyComposition(t *testing.T) {
	base := composeOrchestratorPrompt(model.AgentOpenCode)
	enabled := composeOrchestratorPrompt(model.AgentOpenCode, (InjectOptions{
		IncludeOpenCodeBackgroundPolicy: true,
	}).orchestratorPolicyRenderOptions())

	if strings.Contains(base, openCodeBackgroundPolicyMarker) {
		t.Fatal("default OpenCode composition unexpectedly included background policy")
	}
	if !strings.Contains(enabled, openCodeBackgroundPolicyMarker) || !strings.Contains(enabled, openCodeBackgroundPolicyEnd) {
		t.Fatal("enabled OpenCode composition missing policy markers")
	}
	if strings.Count(enabled, openCodeBackgroundPolicyMarker) != 1 || strings.Count(enabled, openCodeBackgroundPolicyEnd) != 1 {
		t.Fatal("enabled OpenCode composition duplicated policy markers")
	}
	for _, sentinel := range []string{
		"background: true",
		"independent, read-only exploration, audit, or review",
		"foreground tasks when the result is needed before the next action",
		"user decisions",
		"SDD apply",
		"dependent verify evidence",
		"archive",
		"formal RDD/4R lenses",
		"refuters",
		"fix validators",
		"Judgment Day actors",
		"no more than 2 concurrent background tasks",
		"do not poll, sleep, run status checks, or proactively read",
		"Do not duplicate launches or work, and do not overlap files or topics",
		"Never run parallel writers in one worktree",
		"process-local and non-durable",
		"restart loses them",
		"`background` is absent from the Task tool schema",
		"capability is disabled or unknown",
	} {
		if !strings.Contains(enabled, sentinel) {
			t.Fatalf("enabled OpenCode composition missing policy sentinel %q", sentinel)
		}
	}
}

func TestOpenCodeBackgroundPolicyInjectionThroughPublicBoundary(t *testing.T) {
	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		t.Run(string(mode), func(t *testing.T) {
			home := t.TempDir()
			adapter := opencodeAdapter()
			if _, err := Inject(home, adapter, mode); err != nil {
				t.Fatalf("Inject(off) error = %v", err)
			}
			settingsPath := adapter.SettingsPath(home)
			off := agentPrompt(t, readOpenCodeAgents(t, settingsPath), "gentle-orchestrator")
			if strings.Contains(off, openCodeBackgroundPolicyMarker) {
				t.Fatal("default injection unexpectedly included background policy")
			}

			opts := InjectOptions{IncludeOpenCodeBackgroundPolicy: true}
			if _, err := Inject(home, adapter, mode, opts); err != nil {
				t.Fatalf("Inject(on) error = %v", err)
			}
			first := agentPrompt(t, readOpenCodeAgents(t, settingsPath), "gentle-orchestrator")
			if strings.Count(first, openCodeBackgroundPolicyMarker) != 1 || strings.Count(first, openCodeBackgroundPolicyEnd) != 1 {
				t.Fatal("enabled injection did not compose background policy exactly once")
			}

			if _, err := Inject(home, adapter, mode, opts); err != nil {
				t.Fatalf("Inject(repeat) error = %v", err)
			}
			repeated := agentPrompt(t, readOpenCodeAgents(t, settingsPath), "gentle-orchestrator")
			if strings.Count(repeated, openCodeBackgroundPolicyMarker) != 1 || strings.Count(repeated, openCodeBackgroundPolicyEnd) != 1 {
				t.Fatal("repeated enabled injection duplicated background policy")
			}
		})
	}
}

func TestCanonicalCompositionFeedsBaseAndNamedProfile(t *testing.T) {
	canonical := composeOrchestratorPrompt(model.AgentOpenCode)
	if got := renderSDDOrchestratorAsset(model.AgentOpenCode); got != canonical {
		t.Fatal("default OpenCode rendering bypassed canonical composition")
	}

	profile, err := buildProfileOrchestratorPrompt(model.Profile{Name: "rapid"})
	if err != nil {
		t.Fatalf("buildProfileOrchestratorPrompt() error = %v", err)
	}
	for _, marker := range []string{
		"### Lossless Blocking Prompts (MANDATORY)",
		"### Native SDD Dispatcher Guard",
		"### SDD Session Preflight (HARD GATE)",
		"#### Review Execution Contract",
	} {
		if strings.Count(profile, marker) != 1 {
			t.Fatalf("named profile marker %q count = %d, want 1", marker, strings.Count(profile, marker))
		}
		if !strings.Contains(canonical, marker) {
			t.Fatalf("canonical OpenCode composition missing profile marker %q", marker)
		}
	}
}

func TestOpenCodeBackgroundPolicyRoutesThroughNamedProfileRendering(t *testing.T) {
	profile := model.Profile{
		Name:              "rapid",
		OrchestratorModel: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.1"},
	}

	off, err := buildProfileOrchestratorPrompt(profile)
	if err != nil {
		t.Fatalf("buildProfileOrchestratorPrompt(off) error = %v", err)
	}
	on, err := buildProfileOrchestratorPrompt(profile, (InjectOptions{
		IncludeOpenCodeBackgroundPolicy: true,
	}).orchestratorPolicyRenderOptions())
	if err != nil {
		t.Fatalf("buildProfileOrchestratorPrompt(on) error = %v", err)
	}

	if strings.Contains(off, openCodeBackgroundPolicyMarker) {
		t.Fatal("default named profile rendering unexpectedly included background policy")
	}
	if strings.Count(on, openCodeBackgroundPolicyMarker) != 1 || strings.Count(on, openCodeBackgroundPolicyEnd) != 1 {
		t.Fatal("enabled named profile rendering did not compose the policy exactly once")
	}
	for _, want := range []string{"gentle-orchestrator", "sdd-init-rapid", "openai/gpt-5.1"} {
		if !strings.Contains(on, want) {
			t.Fatalf("enabled named profile rendering lost substitution %q", want)
		}
	}
}

func TestOpenCodeBackgroundPolicyPreservesPromptBranches(t *testing.T) {
	tests := []struct {
		name         string
		seed         string
		wantCustom   string
		wantFallback bool
	}{
		{
			name:       "gentle-orchestrator",
			seed:       `{"agent":{"gentle-orchestrator":{"prompt":"CUSTOM_GENTLE"}}}`,
			wantCustom: "CUSTOM_GENTLE",
		},
		{
			name:       "legacy sdd-orchestrator",
			seed:       `{"agent":{"sdd-orchestrator":{"prompt":"CUSTOM_LEGACY"}}}`,
			wantCustom: "CUSTOM_LEGACY",
		},
		{
			name:         "malformed prompt",
			seed:         `{"agent":{"gentle-orchestrator":{"prompt":42}}}`,
			wantFallback: true,
		},
		{
			name:         "unrecognized gentleman",
			seed:         `{"agent":{"gentleman":{"prompt":"CUSTOM_UNRECOGNIZED"}}}`,
			wantFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			adapter := opencodeAdapter()
			settingsPath := adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
				t.Fatalf("MkdirAll(settings) error = %v", err)
			}
			if err := os.WriteFile(settingsPath, []byte(tt.seed), 0o644); err != nil {
				t.Fatalf("WriteFile(settings) error = %v", err)
			}

			_, err := Inject(home, adapter, model.SDDModeSingle, InjectOptions{
				IncludeOpenCodeBackgroundPolicy:    true,
				PreserveOpenCodeOrchestratorPrompt: true,
			})
			if err != nil {
				t.Fatalf("Inject() error = %v", err)
			}
			prompt := agentPrompt(t, readOpenCodeAgents(t, settingsPath), "gentle-orchestrator")
			if strings.Count(prompt, openCodeBackgroundPolicyMarker) != 1 || strings.Count(prompt, openCodeBackgroundPolicyEnd) != 1 {
				t.Fatalf("background policy markers are not exactly once: %q", prompt)
			}
			if tt.wantCustom != "" && !strings.Contains(prompt, tt.wantCustom) {
				t.Fatalf("preserved prompt lost custom content %q", tt.wantCustom)
			}
			if tt.wantFallback && strings.Contains(prompt, "CUSTOM_") {
				t.Fatalf("fallback retained unrecognized custom prompt: %q", prompt)
			}
		})
	}
}

func TestOpenCodeBackgroundPolicyPreservedPromptFollowsOnOffTransitions(t *testing.T) {
	home := t.TempDir()
	adapter := opencodeAdapter()
	settingsPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"agent":{"gentle-orchestrator":{"prompt":"CUSTOM_CONTENT"}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	inject := func(include bool) string {
		t.Helper()
		if _, err := Inject(home, adapter, model.SDDModeSingle, InjectOptions{
			IncludeOpenCodeBackgroundPolicy:    include,
			PreserveOpenCodeOrchestratorPrompt: true,
		}); err != nil {
			t.Fatalf("Inject(include=%t) error = %v", include, err)
		}
		return agentPrompt(t, readOpenCodeAgents(t, settingsPath), "gentle-orchestrator")
	}

	if prompt := inject(false); strings.Contains(prompt, openCodeBackgroundPolicyMarker) || !strings.Contains(prompt, "CUSTOM_CONTENT") {
		t.Fatalf("off prompt = %q, want custom content without policy", prompt)
	}
	if prompt := inject(true); strings.Count(prompt, openCodeBackgroundPolicyMarker) != 1 || !strings.Contains(prompt, "CUSTOM_CONTENT") {
		t.Fatalf("on prompt = %q, want one policy block and custom content", prompt)
	}
	if prompt := inject(false); strings.Contains(prompt, openCodeBackgroundPolicyMarker) || strings.Contains(prompt, openCodeBackgroundPolicyEnd) || !strings.Contains(prompt, "CUSTOM_CONTENT") {
		t.Fatalf("final off prompt = %q, want custom content without policy", prompt)
	}
}

func TestOpenCodeBackgroundPolicyOffRemovesSeededOwnedBlockOnly(t *testing.T) {
	home := t.TempDir()
	adapter := opencodeAdapter()
	settingsPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings) error = %v", err)
	}
	seedPrompt := "BEFORE_CUSTOM\n\n" + mustReadOpenCodeBackgroundPolicy() + "\n\nAFTER_CUSTOM"
	seed := `{"agent":{"gentle-orchestrator":{"prompt":` + strconv.Quote(seedPrompt) + `}}}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	if _, err := Inject(home, adapter, model.SDDModeSingle, InjectOptions{PreserveOpenCodeOrchestratorPrompt: true}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	prompt := agentPrompt(t, readOpenCodeAgents(t, settingsPath), "gentle-orchestrator")
	if strings.Contains(prompt, openCodeBackgroundPolicyMarker) || strings.Contains(prompt, openCodeBackgroundPolicyEnd) || !strings.Contains(prompt, "BEFORE_CUSTOM") || !strings.Contains(prompt, "AFTER_CUSTOM") {
		t.Fatalf("off prompt = %q, want both custom regions without policy", prompt)
	}
}

func TestOpenCodeBackgroundPolicyMalformedPreservedPromptReturnsError(t *testing.T) {
	home := t.TempDir()
	adapter := opencodeAdapter()
	settingsPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings) error = %v", err)
	}
	seed := `{"agent":{"gentle-orchestrator":{"prompt":"` + openCodeBackgroundPolicyMarker + `\nuser content"}}}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Inject() panicked on malformed preserved prompt: %v", recovered)
		}
	}()
	if _, err := Inject(home, adapter, model.SDDModeSingle, InjectOptions{PreserveOpenCodeOrchestratorPrompt: true}); err == nil || !strings.Contains(err.Error(), "validate preserved OpenCode background policy") {
		t.Fatalf("Inject() error = %v, want preserved-policy validation error", err)
	}
}

func TestOpenCodeBackgroundPolicyIsExcludedByKilocodePublicInject(t *testing.T) {
	home := t.TempDir()
	adapter := kilocodeAdapter()
	_, err := Inject(home, adapter, model.SDDModeMulti, InjectOptions{IncludeOpenCodeBackgroundPolicy: true})
	if err != nil {
		t.Fatalf("Inject(Kilocode) error = %v", err)
	}
	prompt := agentPrompt(t, readOpenCodeAgents(t, adapter.SettingsPath(home)), "gentle-orchestrator")
	if strings.Contains(prompt, openCodeBackgroundPolicyMarker) || strings.Contains(prompt, openCodeBackgroundPolicyEnd) {
		t.Fatal("Kilocode received the OpenCode-only background policy")
	}
}
