package sdd

import (
	"encoding/json"
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

// assertOpenCodeResearchProjection runs the production extraction plus
// verification boundary against one generated research entry.
func assertOpenCodeResearchProjection(t *testing.T, entry map[string]any) {
	t.Helper()

	projection, err := researchcapability.OpenCodeProjection(model.AgentOpenCode, entry)
	if err != nil {
		t.Fatalf("OpenCodeProjection() error = %v", err)
	}
	if err := researchcapability.VerifyProjection(projection); err != nil {
		t.Fatalf("VerifyProjection() error = %v", err)
	}
}

// TestOpenCodeResearchProjectionMatchesCapabilityContract proves every
// production OpenCode research projection (single overlay, multi overlay,
// named profile, and an actual injection) matches the canonical research
// capability authority, and that tampering fails closed (#4088).
func TestOpenCodeResearchProjectionMatchesCapabilityContract(t *testing.T) {
	t.Run("single overlay asset", func(t *testing.T) {
		entry := openCodeResearchEntryFromOverlay(t, []byte(assets.MustRead("opencode/sdd-overlay-single.json")), "sdd-research")
		assertOpenCodeResearchProjection(t, entry)
	})

	t.Run("multi overlay asset", func(t *testing.T) {
		entry := openCodeResearchEntryFromOverlay(t, []byte(assets.MustRead("opencode/sdd-overlay-multi.json")), "sdd-research")
		assertOpenCodeResearchProjection(t, entry)
	})

	t.Run("generated profile overlay", func(t *testing.T) {
		home := t.TempDir()
		overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home, openCodeSettingsPathForTest(home), nil, "")
		if err != nil {
			t.Fatalf("GenerateProfileOverlay() error = %v", err)
		}
		entry := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")
		assertOpenCodeResearchProjection(t, entry)
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
			assertOpenCodeResearchProjection(t, entry)
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
	})
}

// TestGenerateProfileOverlay_OpenCodeResearchUsesCanonicalGrants proves the
// generated named-profile research agent derives its permission map from the
// canonical capability authority instead of a hardcoded copy (#4088).
func TestGenerateProfileOverlay_OpenCodeResearchUsesCanonicalGrants(t *testing.T) {
	home := t.TempDir()
	settingsPath := openCodeSettingsPathForTest(home)

	overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home, settingsPath, nil, "")
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}
	research := openCodeResearchEntryFromOverlay(t, overlay, "sdd-research-cheap")

	wantPermission := map[string]any{"bash": "deny", "task": "deny"}
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

	assertOpenCodeResearchProjection(t, research)
}
