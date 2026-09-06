package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const (
	testPreflightOpen  = "<!-- gentle-ai:sdd-session-preflight -->"
	testPreflightClose = "<!-- /gentle-ai:sdd-session-preflight -->"
)

func TestSessionPreflightHasOnePrivateMarkerBoundedAuthority(t *testing.T) {
	shared := assets.MustRead(sharedOrchestratorSectionsAsset)
	if strings.Contains(shared, "Before every SDD command or natural-language SDD request") {
		t.Fatal("shared orchestrator asset still owns the session-preflight body")
	}
	raw := assets.MustRead("opencode/sdd-orchestrator.md")
	if strings.Count(raw, "{{GENTLE_AI_SDD_SECTION:SDD Session Preflight (HARD GATE)}}") != 1 ||
		strings.Count(raw, "### SDD Session Preflight (HARD GATE)") != 1 || strings.Contains(raw, testPreflightOpen) {
		t.Fatal("OpenCode asset must contain only the preflight heading and private-authority placeholder")
	}

	var canonical string
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		rendered := composeOrchestratorPrompt(agent)
		block := testMarkedPreflight(t, rendered)
		if canonical == "" {
			canonical = block
		} else if block != canonical {
			t.Fatalf("%s preflight differs from OpenCode", agent)
		}
		assertTestPreflightPolicy(t, rendered, block)
	}

	profile, err := buildProfileOrchestratorPrompt(model.Profile{Name: "rapid"})
	if err != nil {
		t.Fatal(err)
	}
	if got := testMarkedPreflight(t, profile); got != canonical {
		t.Fatal("named profile changed the canonical session-preflight block")
	}
}

func TestSessionPreflightValidationRejectsDrift(t *testing.T) {
	valid := composeOrchestratorPrompt(model.AgentOpenCode)
	if err := validateSDDSessionPreflight(strings.ReplaceAll(valid, "\n", "\r\n")); err != nil {
		t.Fatalf("valid CRLF prompt rejected: %v", err)
	}
	block := sddSessionPreflightBlock()
	for _, test := range []struct{ name, prompt string }{
		{"changed mapping", strings.Replace(valid, "Both -> `hybrid`", "Both -> `both`", 1)},
		{"selectable budget", strings.Replace(valid, block, block+"\nReview: 400 lines, 800 lines, Other", 1)},
		{"duplicate authority", valid + "\n" + block},
		{"after init", strings.Replace(strings.Replace(valid, block+"\n\n", "", 1), sddSessionPreflightInit, sddSessionPreflightInit+"\n\n"+block, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateSDDSessionPreflight(test.prompt); err == nil {
				t.Fatal("validator accepted session-preflight drift")
			}
		})
	}
}

func testMarkedPreflight(t *testing.T, rendered string) string {
	t.Helper()
	if strings.Count(rendered, testPreflightOpen) != 1 || strings.Count(rendered, testPreflightClose) != 1 {
		t.Fatal("rendered prompt must contain exactly one session-preflight marker pair")
	}
	start := strings.Index(rendered, testPreflightOpen)
	end := strings.Index(rendered, testPreflightClose) + len(testPreflightClose)
	return rendered[start:end]
}

func assertTestPreflightPolicy(t *testing.T, rendered, block string) {
	t.Helper()
	for _, want := range []string{"1. **Pace**", "2. **Artifacts**", "3. **PR strategy**", "Both -> `hybrid`", "fixed at 400 changed lines"} {
		if !strings.Contains(block, want) {
			t.Fatalf("session preflight missing %q", want)
		}
	}
	for _, forbidden := range []string{"Both -> `both`", "Review: 400 lines, 800 lines, Other", "review_budget_lines: 800", "4. **Review"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered prompt retains legacy preflight policy %q", forbidden)
		}
	}
	entry := strings.Index(rendered, "### SDD Entry Routing (MANDATORY)")
	init := strings.Index(rendered, "### SDD Init Guard (MANDATORY)")
	if !strings.Contains(rendered, block+"\n\n### SDD Entry Routing (MANDATORY)") || entry < 0 || init < 0 || entry >= init {
		t.Fatal("session preflight is not in its exact pre-init placement")
	}
}
