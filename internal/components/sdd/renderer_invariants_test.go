package sdd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type orchestratorContractSection struct {
	name      string
	marker    string
	sentinels []string
}

// These are stable contract boundaries, not a snapshot of prompt prose. They
// protect the current OpenCode-only behavior while the renderer is composed in
// later slices.
var currentOpenCodeOrchestratorSections = []orchestratorContractSection{
	{
		name:   "blocking prompts",
		marker: "### Lossless Blocking Prompts (MANDATORY)",
		sentinels: []string{
			"complete user-facing choice envelope",
			"Never summarize, abbreviate, reorder, relabel, merge, or omit choices",
			"exact allowed-answer domain",
			"Then STOP",
		},
	},
	{
		name:   "provider defect handoff",
		marker: "#### Gentle AI Provider Defect Handoff (MANDATORY)",
		sentinels: []string{
			"`report_and_continue`, `continue_without_reporting`, `stop_here`",
			"Only after explicit consent and that final privacy scan",
			"Both continue choices execute that exact captured decline invocation exactly once",
		},
	},
	{
		name:   "edit-authority consent relay",
		marker: "#### SDD Edit-Authority Consent Relay (MANDATORY)",
		sentinels: []string{
			"gentle-ai.sdd-integration.consent/v1",
			"never run the grant unprompted",
			"re-enter through native status",
		},
	},
	{
		name:   "dispatcher guard",
		marker: "### Native SDD Dispatcher Guard",
		sentinels: []string{
			"reads ONLY OpenSpec file artifacts",
			"When the session artifact store is `engram`, do NOT invoke the dispatcher",
			"Route only by `nextRecommended` and dependency states",
		},
	},
	{
		name:   "session preflight",
		marker: "### SDD Session Preflight (HARD GATE)",
		sentinels: []string{
			"all four preflight groups in one single `question` tool call",
			"Do NOT issue four separate `question` tool calls",
			"Cache the choices for this session",
		},
	},
	{
		name:   "lifecycle routing",
		marker: "### SDD Entry Routing (MANDATORY)",
		sentinels: []string{
			"Never launch `sdd-apply` just because the user asked to implement a feature",
			"Only launch `sdd-apply` when all are true",
			"Session preflight is complete",
		},
	},
	{
		name:   "initialization gate",
		marker: "### SDD Init Guard (MANDATORY)",
		sentinels: []string{
			"Search Engram",
			"mem_search(query: \"sdd-init/{project}\"",
			"run `sdd-init` FIRST",
		},
	},
	{
		name:   "automatic gatekeeper",
		marker: "### Automatic Mode Gatekeeper (MANDATORY)",
		sentinels: []string{
			"gatekeeper runs after every phase",
			"re-run the same phase exactly once",
			"Do not advance to dependent phases on a failed gate",
		},
	},
	{
		name:   "runtime attempt authority",
		marker: "### Native Runtime Attempt Authority (MANDATORY)",
		sentinels: []string{
			"provider-owned Git-common-dir runtime ledger",
			"sdd-attempt acquire",
			"sdd-attempt settle",
		},
	},
	{
		name:   "delivery strategy",
		marker: "### Delivery Strategy",
		sentinels: []string{
			"`ask-on-risk`",
			"`auto-chain`",
			"`single-pr`",
			"`exception-ok`",
			"Pass it as `delivery_strategy`",
		},
	},
	{
		name:   "review workload guard",
		marker: "### Review Workload Guard (MANDATORY)",
		sentinels: []string{
			"Chained PRs recommended: Yes",
			"size:exception",
			"Do this even in Automatic mode",
		},
	},
	{
		name:   "skill resolution",
		marker: "### Sub-Agent Launch Pattern",
		sentinels: []string{
			"pre-resolved skill paths from the skill registry",
			"exact path",
			"BEFORE task-specific work",
		},
	},
	{
		name:      "skill resolution feedback",
		marker:    "### Skill Resolution Feedback",
		sentinels: []string{"`paths-injected`", "fallback-registry", "re-read the registry"},
	},
	{
		name:   "sub-agent context protocol",
		marker: "### Sub-Agent Context Protocol",
		sentinels: []string{
			"Sub-agents get a fresh context with NO memory",
			"Sub-agent does NOT search engram itself",
			"save significant discoveries, decisions, or bug fixes to engram",
		},
	},
}

func assertCurrentOpenCodeOrchestratorContract(t *testing.T, label string, content string, agent model.AgentID, profileName string) {
	t.Helper()

	if strings.Contains(content, runtimeAgentIDPlaceholder) {
		t.Fatalf("%s retains runtime identity placeholder", label)
	}
	if !strings.Contains(content, "--agent "+string(agent)+" --next-transition") {
		t.Fatalf("%s does not bind negotiated review routing to runtime %q", label, agent)
	}

	lastMarker := -1
	for _, section := range currentOpenCodeOrchestratorSections {
		markerIndex := strings.Index(content, section.marker)
		if markerIndex < 0 {
			t.Errorf("%s missing %s section marker %q", label, section.name, section.marker)
			continue
		}
		if markerIndex <= lastMarker {
			t.Errorf("%s moved %s before a preceding contract section", label, section.name)
		}
		lastMarker = markerIndex
		sectionBody := content[markerIndex+len(section.marker):]
		nearestFollowing := len(sectionBody)
		for _, following := range currentOpenCodeOrchestratorSections {
			if following.marker == section.marker {
				continue
			}
			if followingIndex := strings.Index(sectionBody, following.marker); followingIndex >= 0 && followingIndex < nearestFollowing {
				nearestFollowing = followingIndex
			}
		}
		sectionBody = sectionBody[:nearestFollowing]
		for _, sentinel := range section.sentinels {
			if profileName != "" {
				for _, phase := range ProfilePhaseOrder() {
					sentinel = strings.ReplaceAll(sentinel, phase, phase+"-"+profileName)
				}
			}
			if !strings.Contains(sectionBody, sentinel) {
				t.Errorf("%s %s section lost sentinel %q", label, section.name, sentinel)
			}
		}
	}

	assertTextContainsClauses(t, label+" bounded review", content, []string{
		"#### Review Execution Contract",
		"Selectorless STATUS only preflights the current worktree candidate",
		"START freezes one compact atomic transaction",
		"Only candidate-caused severe findings block",
		"burns that exact authority and its artifacts",
		"Clean FINALIZE success stops with no terminal STATUS.",
	})
	if profileName == "" {
		assertTextContainsClauses(t, label+" model assignment contract", content, []string{
			"<!-- gentle-ai:sdd-model-assignments -->",
			"## Model Assignments",
			"agent.gentle-orchestrator.model",
			"agent.sdd-<phase>.model",
			"default OpenCode runtime model",
		})
	}
}

func TestOpenCodeBaseOrchestratorPreservesCurrentContract(t *testing.T) {
	content := renderSDDOrchestratorAsset(model.AgentOpenCode)
	assertCurrentOpenCodeOrchestratorContract(t, "OpenCode base orchestrator", content, model.AgentOpenCode, "")

	visibilityMarker := "<!-- gentle-ai:" + openCodeDelegationVisibilitySectionID + " -->"
	for _, required := range []string{
		visibilityMarker,
		"#### Delegation Visibility (OpenCode Desktop)",
		"assistant-visible status line immediately before the call",
		"15 tokens or fewer",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("OpenCode base orchestrator lost current Desktop delegation section sentinel %q", required)
		}
	}
}

func TestOpenCodeBaseInjectionBindsAssignmentsAndPreservesContract(t *testing.T) {
	mockNoPackageManager(t)
	home := t.TempDir()
	assignments := map[string]model.ModelAssignment{
		"gentle-orchestrator": {ProviderID: "openai", ModelID: "gpt-5.1", Effort: "high"},
		"sdd-apply":           {ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
	}
	if _, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{OpenCodeModelAssignments: assignments}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	agentsMap := readOpenCodeAgents(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	prompt := agentPrompt(t, agentsMap, "gentle-orchestrator")
	assertCurrentOpenCodeOrchestratorContract(t, "OpenCode base injection", prompt, model.AgentOpenCode, "")

	for agentName, wantModel := range map[string]string{
		"gentle-orchestrator": "openai/gpt-5.1",
		"sdd-apply":           "anthropic/claude-sonnet-4-5",
	} {
		entry, ok := agentsMap[agentName].(map[string]any)
		if !ok || entry["model"] != wantModel {
			t.Fatalf("%s model = %#v, want %q", agentName, entry["model"], wantModel)
		}
	}
	if variant := agentsMap["gentle-orchestrator"].(map[string]any)["variant"]; variant != "high" {
		t.Fatalf("gentle-orchestrator variant = %#v, want high", variant)
	}
}

func TestOpenCodeNamedProfileOrchestratorPreservesCurrentContract(t *testing.T) {
	const profileName = "rapid"
	profile := model.Profile{
		Name:              profileName,
		OrchestratorModel: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.1"},
		PhaseAssignments: map[string]model.ModelAssignment{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
		},
	}
	home := t.TempDir()
	overlay, err := GenerateProfileOverlay(profile, home, filepath.Join(home, "opencode.json"), nil, "")
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}

	var root struct {
		Agent map[string]any `json:"agent"`
	}
	if err := json.Unmarshal(overlay, &root); err != nil {
		t.Fatalf("unmarshal profile overlay: %v", err)
	}
	orchestratorName := "sdd-orchestrator-" + profileName
	content := agentPrompt(t, root.Agent, orchestratorName)
	assertCurrentOpenCodeOrchestratorContract(t, "OpenCode named profile", content, model.AgentOpenCode, profileName)

	assertTextContainsClauses(t, "OpenCode named profile model assignments", content, []string{
		"<!-- gentle-ai:sdd-model-assignments -->",
		"<!-- /gentle-ai:sdd-model-assignments -->",
		"## Model Assignments",
		"| orchestrator | openai/gpt-5.1 |",
		"| sdd-apply-" + profileName + " | anthropic/claude-sonnet-4-5 |",
	})
	for _, phase := range ProfilePhaseOrder() {
		suffixed := phase + "-" + profileName
		if !strings.Contains(content, suffixed) {
			t.Errorf("OpenCode named profile missing suffixed reference %q", suffixed)
		}
		if strings.Contains(content, suffixed+"-"+profileName) {
			t.Errorf("OpenCode named profile double-suffixed reference %q", suffixed+"-"+profileName)
		}
		for offset := 0; ; {
			relative := strings.Index(content[offset:], phase)
			if relative < 0 {
				break
			}
			start := offset + relative
			end := start + len(phase)
			if !strings.HasPrefix(content[end:], "-"+profileName) {
				t.Errorf("OpenCode named profile retained unsuffixed reference %q", content[start:end])
			}
			offset = end
		}
	}

	profileAgent, ok := root.Agent["sdd-apply-"+profileName].(map[string]any)
	if !ok || profileAgent["model"] != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("named sdd-apply agent model = %#v, want anthropic/claude-sonnet-4-5", profileAgent["model"])
	}
	if orchestrator, ok := root.Agent[orchestratorName].(map[string]any); !ok || orchestrator["model"] != "openai/gpt-5.1" {
		t.Fatalf("named orchestrator model = %#v, want openai/gpt-5.1", orchestrator["model"])
	}

	// This is an existing named-profile exclusion, not a future inheritance rule:
	// profile rendering deliberately removes the default Desktop narration block.
	visibilityMarker := "<!-- gentle-ai:" + openCodeDelegationVisibilitySectionID + " -->"
	if strings.Contains(content, visibilityMarker) {
		t.Fatalf("named profile unexpectedly inherited the default Desktop delegation section")
	}
}

func TestKilocodeOrchestratorBaselineSharesHistoricalAssetWithoutReviewLifecycle(t *testing.T) {
	if got, want := sddOrchestratorAsset(model.AgentKilocode), sddOrchestratorAsset(model.AgentOpenCode); got != want {
		t.Fatalf("Kilocode orchestrator asset = %q, want shared historical asset %q", got, want)
	}

	content := renderSDDOrchestratorAsset(model.AgentKilocode)
	if strings.Contains(content, "### Authority-First Terminal Procedure") {
		t.Fatal("Kilocode baseline received the shared review lifecycle")
	}
	for _, want := range []string{"## SDD Workflow", "### Native SDD Dispatcher Guard", "### SDD Init Guard (MANDATORY)"} {
		if !strings.Contains(content, want) {
			t.Fatalf("Kilocode baseline lost normal SDD clause %q", want)
		}
	}

	if strings.Contains(content, openCodeBackgroundPolicyMarker) {
		t.Fatal("Kilocode baseline received an OpenCode-only background addendum")
	}
}
