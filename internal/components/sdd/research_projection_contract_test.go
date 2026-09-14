package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/researchcapability"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// openCodeResearchEntryFromOverlay returns the research agent entry of one
// OpenCode overlay document.
func openCodeResearchEntryFromOverlay(t *testing.T, overlayBytes []byte, key string) map[string]any {
	t.Helper()

	var overlay struct {
		Agent map[string]any `json:"agent"`
	}
	if err := json.Unmarshal(overlayBytes, &overlay); err != nil {
		t.Fatalf("unmarshal overlay for %q: %v", key, err)
	}
	entry, ok := overlay.Agent[key].(map[string]any)
	if !ok {
		t.Fatalf("overlay missing agent %q", key)
	}
	return entry
}

func deepCopyOpenCodeAgentEntry(t *testing.T, entry map[string]any) map[string]any {
	t.Helper()

	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal agent entry: %v", err)
	}
	var copyOf map[string]any
	if err := json.Unmarshal(raw, &copyOf); err != nil {
		t.Fatalf("unmarshal agent entry: %v", err)
	}
	return copyOf
}

// assertOpenCodeResearchProjection runs the production extraction, canonical
// resolution, materialization, and verification boundary against one generated
// research entry.
func assertOpenCodeResearchProjection(t *testing.T, entry map[string]any, agentName, homeDir, settingsPath, capability string) {
	t.Helper()

	if err := verifyOpenCodeResearchEntry(entry, agentName, homeDir, settingsPath, capability, ""); err != nil {
		t.Fatalf("verifyOpenCodeResearchEntry(%q) error = %v", agentName, err)
	}
}

// TestOpenCodeResearchProjectionMatchesCapabilityContract proves every
// production OpenCode research projection (single overlay, multi overlay,
// named profile, and an actual injection) matches the canonical research
// capability authority, and that tampering fails closed (#4088).
func TestOpenCodeResearchProjectionMatchesCapabilityContract(t *testing.T) {
	t.Run("single overlay asset", func(t *testing.T) {
		home := t.TempDir()
		entry := openCodeResearchEntryFromOverlay(t, []byte(assets.MustRead("opencode/sdd-overlay-single.json")), "sdd-research")
		assertOpenCodeResearchProjection(t, entry, "sdd-research", home, openCodeSettingsPathForTest(home), "capable")
	})

	t.Run("multi overlay asset", func(t *testing.T) {
		home := t.TempDir()
		entry := openCodeResearchEntryFromOverlay(t, []byte(assets.MustRead("opencode/sdd-overlay-multi.json")), "sdd-research")
		assertOpenCodeResearchProjection(t, entry, "sdd-research", home, openCodeSettingsPathForTest(home), "capable")
	})

	t.Run("generated profile overlay", func(t *testing.T) {
		home := t.TempDir()
		settingsPath := openCodeSettingsPathForTest(home)
		overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home, settingsPath, nil, "")
		if err != nil {
			t.Fatalf("GenerateProfileOverlay() error = %v", err)
		}
		entry := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")
		assertOpenCodeResearchProjection(t, entry, "sdd-research-cheap", home, settingsPath, "small")
	})

	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		mode := mode
		t.Run("injected "+string(mode), func(t *testing.T) {
			home := t.TempDir()
			mockNoPackageManager(t)
			if _, err := Inject(home, opencodeAdapter(), mode); err != nil {
				t.Fatalf("Inject(%s) error = %v", mode, err)
			}
			agentsMap := readOpenCodeAgents(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
			entry, ok := agentsMap["sdd-research"].(map[string]any)
			if !ok {
				t.Fatal("installed opencode.json missing sdd-research agent")
			}
			assertOpenCodeResearchProjection(t, entry, "sdd-research", home, filepath.Join(home, ".config", "opencode", "opencode.json"), "capable")
		})
	}

	t.Run("tampered variants fail closed", func(t *testing.T) {
		base := openCodeResearchEntryFromOverlay(t, []byte(assets.MustRead("opencode/sdd-overlay-single.json")), "sdd-research")

		t.Run("webfetch allow", func(t *testing.T) {
			tampered := deepCopyOpenCodeAgentEntry(t, base)
			permission := tampered["permission"].(map[string]any)
			permission["webfetch"] = "allow"
			projection, err := researchcapability.OpenCodeProjection(model.AgentOpenCode, tampered)
			if err != nil {
				t.Fatalf("OpenCodeProjection() error = %v, want extraction to succeed", err)
			}
			if err := researchcapability.VerifyProjection(projection); err == nil {
				t.Fatal("VerifyProjection() = nil, want refusal for an admitted OpenCode evidence tool")
			}
		})

		t.Run("permission removed", func(t *testing.T) {
			tampered := deepCopyOpenCodeAgentEntry(t, base)
			delete(tampered, "permission")
			if _, err := researchcapability.OpenCodeProjection(model.AgentOpenCode, tampered); err == nil {
				t.Fatal("OpenCodeProjection() = nil, want refusal for a missing permission map")
			}
		})

		t.Run("declaration changed", func(t *testing.T) {
			tampered := deepCopyOpenCodeAgentEntry(t, base)
			prompt := tampered["prompt"].(string)
			tampered["prompt"] = strings.Replace(prompt, "documentation=[]; open-web=[]", "documentation=[WebFetch]; open-web=[]", 1)
			projection, err := researchcapability.OpenCodeProjection(model.AgentOpenCode, tampered)
			if err != nil {
				t.Fatalf("OpenCodeProjection() error = %v, want extraction to succeed", err)
			}
			if err := researchcapability.VerifyProjection(projection); err == nil {
				t.Fatal("VerifyProjection() = nil, want refusal for a non-canonical declaration")
			}
		})

		t.Run("deprecated tools key", func(t *testing.T) {
			tampered := deepCopyOpenCodeAgentEntry(t, base)
			tampered["tools"] = map[string]any{"webfetch": true}
			if _, err := researchcapability.OpenCodeProjection(model.AgentOpenCode, tampered); err == nil {
				t.Fatal("OpenCodeProjection() = nil, want refusal for a deprecated tools key")
			}
		})

		t.Run("executor posture deny removed", func(t *testing.T) {
			tampered := deepCopyOpenCodeAgentEntry(t, base)
			delete(tampered["permission"].(map[string]any), "write")
			projection, err := researchcapability.OpenCodeProjection(model.AgentOpenCode, tampered)
			if err != nil {
				t.Fatalf("OpenCodeProjection() error = %v, want extraction to succeed", err)
			}
			err = researchcapability.VerifyProjection(projection)
			if err == nil {
				t.Fatal("VerifyProjection() = nil, want refusal for a missing research executor posture deny")
			}
			if !strings.Contains(err.Error(), "write") {
				t.Fatalf("VerifyProjection() error = %q, want the missing posture deny named", err)
			}
		})

		t.Run("shared prompt reference resolves elsewhere", func(t *testing.T) {
			home := t.TempDir()
			settingsPath := openCodeSettingsPathForTest(home)
			overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home, settingsPath, nil, "")
			if err != nil {
				t.Fatalf("GenerateProfileOverlay() error = %v", err)
			}
			entry := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")
			// A lookalike suffix must not be enough: this path is not the
			// canonical shared research prompt the runtime would load.
			entry["prompt"] = "{file:./elsewhere/prompts/sdd/sdd-research.md}"
			if err := verifyOpenCodeResearchEntry(entry, "sdd-research-cheap", home, settingsPath, "small", ""); err == nil {
				t.Fatal("verifyOpenCodeResearchEntry() = nil, want refusal for a non-canonical shared prompt reference")
			}
		})

		t.Run("shared prompt reference escapes the settings directory", func(t *testing.T) {
			home := t.TempDir()
			settingsPath := openCodeSettingsPathForTest(home)
			overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home, settingsPath, nil, "")
			if err != nil {
				t.Fatalf("GenerateProfileOverlay() error = %v", err)
			}
			entry := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")
			entry["prompt"] = "{file:../../opencode/prompts/sdd/sdd-research.md}"
			if err := verifyOpenCodeResearchEntry(entry, "sdd-research-cheap", home, settingsPath, "small", ""); err == nil {
				t.Fatal("verifyOpenCodeResearchEntry() = nil, want refusal for a traversing shared prompt reference")
			}
		})
	})
}

// TestGenerateProfileOverlay_OpenCodeResearchUsesCanonicalGrants proves the
// generated named-profile research agent derives its permission map from the
// canonical capability authority instead of a hardcoded copy, including the
// repository-mutation denies that keep the executor output-only (#4088).
func TestGenerateProfileOverlay_OpenCodeResearchUsesCanonicalGrants(t *testing.T) {
	home := t.TempDir()
	settingsPath := openCodeSettingsPathForTest(home)

	overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home, settingsPath, nil, "")
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}
	research := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")

	wantPermission := map[string]any{"bash": "deny", "edit": "deny", "task": "deny", "write": "deny"}
	for _, decision := range researchcapability.EvidenceToolDecisions(model.AgentOpenCode) {
		if decision.Allowed {
			wantPermission[decision.Tool] = "allow"
		} else {
			wantPermission[decision.Tool] = "deny"
		}
	}
	if got := research["permission"]; !reflect.DeepEqual(got, wantPermission) {
		t.Fatalf("sdd-research-cheap permission = %#v, want authority-derived %#v", got, wantPermission)
	}

	wantPrompt, err := SharedPromptFileRef(settingsPath, home, "sdd-research")
	if err != nil {
		t.Fatalf("SharedPromptFileRef() error = %v", err)
	}
	if got := research["prompt"]; got != wantPrompt {
		t.Fatalf("sdd-research-cheap prompt = %#v, want shared ref %q", got, wantPrompt)
	}

	assertOpenCodeResearchProjection(t, research, "sdd-research-cheap", home, settingsPath, "small")
}

// TestOpenCodeResearchPermissionMapsDenyExecutorPosture pins both generated
// default surfaces (single and multi overlay assets) and the named-profile
// surface to the exact research executor deny set. OpenCode tools are
// default-open, so an omitted decision is an implicit grant: bash would give
// shell access, task delegation, and write/edit repository mutation (#4088).
func TestOpenCodeResearchPermissionMapsDenyExecutorPosture(t *testing.T) {
	t.Parallel()

	exactWant := func() map[string]any {
		want := map[string]any{"bash": "deny", "edit": "deny", "task": "deny", "write": "deny"}
		for _, decision := range researchcapability.EvidenceToolDecisions(model.AgentOpenCode) {
			if decision.Allowed {
				want[decision.Tool] = "allow"
			} else {
				want[decision.Tool] = "deny"
			}
		}
		return want
	}

	t.Run("base overlay assets", func(t *testing.T) {
		t.Parallel()

		for _, path := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
			path := path
			t.Run(path, func(t *testing.T) {
				t.Parallel()

				entry := openCodeResearchEntryFromOverlay(t, []byte(assets.MustRead(path)), "sdd-research")
				if got := entry["permission"]; !reflect.DeepEqual(got, exactWant()) {
					t.Fatalf("%s research permission = %#v, want exact deny set %#v", path, got, exactWant())
				}
			})
		}
	})

	t.Run("named profile overlay", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home, openCodeSettingsPathForTest(home), nil, "")
		if err != nil {
			t.Fatalf("GenerateProfileOverlay() error = %v", err)
		}
		research := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")
		for _, tool := range []string{"bash", "task", "write", "edit"} {
			if got := research["permission"].(map[string]any)[tool]; got != "deny" {
				t.Fatalf("sdd-research-cheap permission %q = %v, want deny", tool, got)
			}
		}
		if got := research["permission"]; !reflect.DeepEqual(got, exactWant()) {
			t.Fatalf("sdd-research-cheap permission = %#v, want exact deny set %#v", got, exactWant())
		}
	})
}

// TestSharedResearchPromptVerificationBoundary proves the production research
// verification boundary accepts the production shared prompt reference and
// refuses genuinely drifted rendered bytes, so removing the materialization or
// declaration check would fail this test (#4088).
func TestSharedResearchPromptVerificationBoundary(t *testing.T) {
	home := t.TempDir()
	settingsPath := openCodeSettingsPathForTest(home)
	profiles := []model.Profile{makeHaikuProfile()}
	overlay, err := GenerateProfileOverlay(profiles[0], home, settingsPath, nil, "")
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}
	entry := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")
	ref, ok := entry["prompt"].(string)
	if !ok {
		t.Fatalf("sdd-research-cheap prompt = %#v, want a shared reference", entry["prompt"])
	}
	wantRef, err := SharedPromptFileRef(settingsPath, home, "sdd-research")
	if err != nil {
		t.Fatalf("SharedPromptFileRef() error = %v", err)
	}
	if ref != wantRef {
		t.Fatalf("sdd-research-cheap prompt = %q, want production reference %q", ref, wantRef)
	}
	capability := sharedPromptCapability(sharedPromptPhaseCapabilities(nil, profiles), "sdd-research")

	if err := verifyOpenCodeResearchEntry(entry, "sdd-research-cheap", home, settingsPath, capability, ""); err != nil {
		t.Fatalf("verifyOpenCodeResearchEntry() error = %v, want the production reference accepted", err)
	}

	original := sharedResearchPromptRenderer
	t.Cleanup(func() { sharedResearchPromptRenderer = original })
	if _, content, err := original(home, "sdd-research", capability, ""); err != nil || !strings.Contains(content, "## Execution Role") {
		t.Fatalf("test fixture anchor missing from the rendered shared research prompt (err = %v)", err)
	}
	sharedResearchPromptRenderer = func(homeDir, phase, capability, guidance string) (string, string, error) {
		path, content, err := original(homeDir, phase, capability, guidance)
		if err != nil {
			return "", "", err
		}
		if phase == "sdd-research" {
			// The runtime would load a prompt claiming a documentation grant
			// the canonical capability does not declare.
			content = strings.Replace(content, "## Execution Role", "Evidence grants: documentation=[WebFetch]; open-web=[].\n\n## Execution Role", 1)
		}
		return path, content, nil
	}
	if err := verifyOpenCodeResearchEntry(entry, "sdd-research-cheap", home, settingsPath, capability, ""); err == nil {
		t.Fatal("verifyOpenCodeResearchEntry() = nil, want refusal for a grant-claiming shared research prompt")
	}
}

// TestSingleModeNamedProfileWritesVerifiedSharedPrompts proves a single-mode
// install with a named profile writes the shared prompt files the profile
// overlay references, and that the research prompt on disk carries exactly the
// bytes the verification boundary approved (#4088).
func TestSingleModeNamedProfileWritesVerifiedSharedPrompts(t *testing.T) {
	home := t.TempDir()
	settingsPath := openCodeSettingsPathForTest(home)
	profiles := []model.Profile{makeHaikuProfile()}
	if _, err := Inject(home, opencodeAdapter(), model.SDDModeSingle, InjectOptions{Profiles: profiles}); err != nil {
		t.Fatalf("Inject(single, named profile) error = %v", err)
	}

	for _, phase := range ProfilePhaseOrder() {
		path := filepath.Join(SharedPromptDir(home), phase+".md")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("shared prompt %q missing after single-mode named-profile install: %v", path, err)
		}
	}

	agentsMap := readOpenCodeAgents(t, settingsPath)
	research, ok := agentsMap["sdd-research-cheap"].(map[string]any)
	if !ok {
		t.Fatal("installed settings missing sdd-research-cheap agent")
	}
	projection, err := researchcapability.OpenCodeProjection(model.AgentOpenCode, research)
	if err != nil {
		t.Fatalf("OpenCodeProjection() error = %v", err)
	}
	if projection.SharedPromptRef == "" {
		t.Fatalf("sdd-research-cheap prompt = %#v, want a shared reference", research["prompt"])
	}
	capability := sharedPromptCapability(sharedPromptPhaseCapabilities(nil, profiles), "sdd-research")
	verified, err := materializeSharedResearchPrompt(projection.SharedPromptRef, home, settingsPath, capability, "")
	if err != nil {
		t.Fatalf("materializeSharedResearchPrompt() error = %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(SharedPromptDir(home), "sdd-research.md"))
	if err != nil {
		t.Fatalf("ReadFile(shared research prompt) error = %v", err)
	}
	if string(onDisk) != verified {
		t.Fatal("single-mode named-profile install wrote shared research prompt bytes different from the verified bytes")
	}
}

// TestSingleModeWithoutProfilesDoesNotWriteSharedPrompts preserves the
// profile-less single-mode contract: nothing emits a shared prompt reference,
// so no shared prompt files are written (#4088).
func TestSingleModeWithoutProfilesDoesNotWriteSharedPrompts(t *testing.T) {
	home := t.TempDir()
	if _, err := Inject(home, opencodeAdapter(), model.SDDModeSingle); err != nil {
		t.Fatalf("Inject(single) error = %v", err)
	}
	if _, err := os.Stat(SharedPromptDir(home)); !os.IsNotExist(err) {
		t.Fatalf("shared prompt directory exists after profile-less single-mode install (stat error = %v)", err)
	}
}

// TestInjectResearchProjectionFailureLeavesNoAgentsDirectory proves a refused
// research projection stops the sub-agent section before the agents directory
// is created, so a failed injection cannot leave filesystem residue (#4088).
func TestInjectResearchProjectionFailureLeavesNoAgentsDirectory(t *testing.T) {
	original := renderSubAgentAssetContent
	t.Cleanup(func() { renderSubAgentAssetContent = original })
	renderSubAgentAssetContent = func(agent model.AgentID, assetPath string) string {
		content := original(agent, assetPath)
		if agent == model.AgentClaudeCode && strings.HasSuffix(assetPath, "/sdd-research.md") {
			// Claim a documentation grant the canonical capability does not
			// declare; extraction must succeed and verification must refuse.
			return strings.Replace(content, "documentation=[WebFetch]", "documentation=[curl]", 1)
		}
		return content
	}

	home := t.TempDir()
	_, err := Inject(home, claudeAdapter(), model.SDDModeSingle)
	if err == nil {
		t.Fatal("Inject() = nil, want research projection refusal")
	}
	if !strings.Contains(err.Error(), "research projection") {
		t.Fatalf("Inject() error = %v, want research projection refusal", err)
	}
	agentsDir := claudeAdapter().SubAgentsDir(home)
	if _, statErr := os.Stat(agentsDir); !os.IsNotExist(statErr) {
		t.Fatalf("agents directory %q exists after refused injection (stat error = %v)", agentsDir, statErr)
	}
}
