package assets

import (
	"crypto/sha256"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// #3817: ODD orchestrators are hand-maintained across twelve runtimes. Keep
// historical ceilings for retained shared sections. The lossless-prompt
// section's ceiling rises from four to six for the intentional ODD rewrite;
// its common safety anchors remain checked on every runtime below. Removing
// a section is an inventory change; new uncoordinated drift must fail.
var orchestratorSectionDriftRatchet = []struct {
	name        string
	maxVariants int
}{
	{"Delegation Rules", 11},
	{"State and Conventions", 10},
	{"Skill Resolution Feedback", 8},
	{"Agent Teams Orchestrator", 6},
	{"Lossless Blocking Prompts (MANDATORY)", 6},
	{"Language Domain Contract", 2},
}

var orchestratorHeading = regexp.MustCompile(`(?m)^#{2,3} .+$`)

// orchestratorSectionVariants maps each subsection name to the distinct bodies
// the runtime orchestrators give it.
func orchestratorSectionVariants(t *testing.T) map[string]map[string]struct{} {
	t.Helper()
	variants := map[string]map[string]struct{}{}
	seen := map[string]bool{}
	err := fs.WalkDir(FS, ".", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Base(assetPath) != "orchestrator.md" {
			return nil
		}
		seen[path.Dir(assetPath)] = true
		content := MustRead(assetPath)
		for _, anchor := range []string{"Lossless Blocking Prompts (MANDATORY)", "Gentle AI Provider Defect Handoff", "Never summarize, abbreviate, reorder, relabel, merge, or omit choices."} {
			if !strings.Contains(content, anchor) {
				t.Errorf("%s lost lossless prompt safety anchor %q", assetPath, anchor)
			}
		}
		headings := orchestratorHeading.FindAllStringIndex(content, -1)
		for i, span := range headings {
			name := strings.TrimSpace(strings.TrimLeft(content[span[0]:span[1]], "# "))
			end := len(content)
			if i+1 < len(headings) {
				end = headings[i+1][0]
			}
			body := strings.TrimSpace(content[span[1]:end])
			digest := sha256.Sum256([]byte(body))
			if variants[name] == nil {
				variants[name] = map[string]struct{}{}
			}
			variants[name][string(digest[:])] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk orchestrator assets: %v", err)
	}
	for _, runtime := range []string{"antigravity", "claude", "codex", "cursor", "gemini", "generic", "hermes", "kimi", "kiro", "opencode", "qwen", "windsurf"} {
		if !seen[runtime] {
			t.Errorf("missing ODD orchestrator for %s", runtime)
		}
	}
	if len(seen) != 12 {
		t.Errorf("ODD orchestrator inventory changed: got %d runtimes, want 12; update the ratchet deliberately", len(seen))
	}
	return variants
}

// TestOrchestratorSectionDriftDoesNotGrow fails when a shared subsection gains
// a new per-runtime variant.
func TestOrchestratorSectionDriftDoesNotGrow(t *testing.T) {
	variants := orchestratorSectionVariants(t)
	for _, pinned := range orchestratorSectionDriftRatchet {
		got := len(variants[pinned.name])
		if got == 0 {
			t.Errorf("section %q vanished from every runtime orchestrator; remove its ratchet entry deliberately", pinned.name)
			continue
		}
		if got > pinned.maxVariants {
			t.Errorf("section %q drifted to %d variants, ratchet allows %d — edit every runtime or move the section to the shared asset", pinned.name, got, pinned.maxVariants)
		}
		if got < pinned.maxVariants {
			t.Logf("section %q converged to %d variants (historical ceiling %d); consider lowering the ratchet", pinned.name, got, pinned.maxVariants)
		}
	}
}
