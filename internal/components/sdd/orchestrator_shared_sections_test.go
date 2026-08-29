package sdd

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// #3817: the SDD orchestrator contract is maintained as twelve hand-written
// near-duplicates. Measured across them, 19 of 21 shared subsections have
// drifted -- Delegation Rules alone has eleven variants across eleven runtimes
// -- so reconciling those is a decision per section, not a refactor.
//
// These five are the subsections that had NOT drifted semantically: three are
// byte-identical across every runtime that carries them, and the other two
// differ only cosmetically (a ```text fence, and "the user" for "user"). They
// move to one shared asset so the mechanism exists and so this set cannot drift
// again. Each runtime keeps its own heading line, which is why codex may hold a
// section at ## while the others hold it at ###.

var sharedOrchestratorSectionNames = []string{
	"Native SDD Dispatcher Guard",
	"Native Runtime Attempt Authority (MANDATORY)",
	"Language Domain Contract",
	"Dependency Graph",
	"Recovery Rule",
}

// TestSharedOrchestratorSectionsHaveOneSource pins that each shared section
// body lives in the shared asset and NOT in the per-runtime orchestrators.
//
// It opens the runtime assets. An earlier version only checked the shared
// asset against a substring extracted from that same shared asset, which is a
// tautology: it could not fail, while its name promised that no duplicated
// body survives in the eleven per-runtime files.
func TestSharedOrchestratorSectionsHaveOneSource(t *testing.T) {
	runtimeAssets := map[string]string{}
	err := fs.WalkDir(assets.FS, ".", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && path.Base(assetPath) == "sdd-orchestrator.md" {
			runtimeAssets[assetPath] = assets.MustRead(assetPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk orchestrator assets: %v", err)
	}
	if len(runtimeAssets) == 0 {
		t.Fatal("no runtime orchestrator assets were found")
	}

	for _, name := range sharedOrchestratorSectionNames {
		body := sharedOrchestratorSection(name)
		if strings.TrimSpace(body) == "" {
			t.Fatalf("shared asset carries no body for %q", name)
		}
		for assetPath, content := range runtimeAssets {
			if strings.Contains(content, body) {
				t.Errorf("%s still carries the %q body inline; it must carry the placeholder instead", assetPath, name)
			}
		}
	}
}

// TestEveryRuntimeRendersTheSharedSections pins that the substitution actually
// reaches the rendered prompt: deleting duplication must not delete content.
func TestEveryRuntimeRendersTheSharedSections(t *testing.T) {
	for _, agent := range []model.AgentID{
		model.AgentOpenCode, model.AgentCursor, model.AgentGeminiCLI, model.AgentQwenCode,
		model.AgentHermes, model.AgentKimi, model.AgentWindsurf, model.AgentCodex,
	} {
		rendered := renderSDDOrchestratorAsset(agent)
		for _, name := range sharedOrchestratorSectionNames {
			if !strings.Contains(rendered, name) {
				continue // a runtime that never carried this section keeps not carrying it
			}
			body := sharedOrchestratorSection(name)
			first := strings.SplitN(strings.TrimSpace(body), "\n", 2)[0]
			if !strings.Contains(rendered, first) {
				t.Errorf("%s rendered %q without its shared body", agent, name)
			}
		}
	}
}

// TestNoRawSharedSectionPlaceholderSurvivesRendering pins that no placeholder
// reaches a rendered prompt.
func TestNoRawSharedSectionPlaceholderSurvivesRendering(t *testing.T) {
	for _, agent := range []model.AgentID{
		model.AgentOpenCode, model.AgentCursor, model.AgentGeminiCLI, model.AgentQwenCode,
		model.AgentHermes, model.AgentKimi, model.AgentWindsurf, model.AgentCodex,
		model.AgentKiroIDE, model.AgentAntigravity, model.AgentClaudeCode,
	} {
		if rendered := renderSDDOrchestratorAsset(agent); strings.Contains(rendered, "{{GENTLE_AI_SDD_SECTION:") {
			t.Errorf("%s kept a raw shared-section placeholder", agent)
		}
	}
}
