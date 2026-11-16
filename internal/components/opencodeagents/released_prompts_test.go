package opencodeagents

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// releasedAgentAssets are the exact agent templates v4.5.0
// (f2f7d7b16c2cf1afe5e07f717cf8265946ac3feb) embedded, copied byte for byte
// into testdata, with their sha256. The harness-continuity edits (worker edit
// surfaces, the retired Key Learnings ritual) changed the current templates, so
// these fixtures are the only record of what an upgrading user has installed.
var releasedAgentAssets = map[string]string{
	"gentle-ai-explore": "44cec90dd0156fafe5a29e2ec9cb9070e83ba9e32086aa0eaccbf5dd3a538cee",
	"gentle-ai-worker":  "856460b95ac45e0f74b02e4187b50278a6e4da2ed020e099aa926e03fd89fa7e",
	"gentle-ai-verify":  "980896ec5384109fb14b834027d94937a3355d637680ac6930b6ba52e2d8231b",
}

// releasedVerifyMethod is the shared verification method v4.5.0 expanded into
// the verifier prompt, pinned like the templates: rendering the released
// verifier with the current method would hide a method edit that stops
// recognizing v4.5.0 installs.
const releasedVerifyMethod = "3f85280bdaf8097b0382da5eebe25be3d2e1507f8a71da2099caad2a18645857"

// releasedFixture reads one byte-pinned v4.5.0 fixture.
func releasedFixture(t *testing.T, name, sum string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "v4.5.0", name))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != sum {
		t.Fatalf("%s v4.5.0 fixture changed: %s", name, got)
	}
	return string(raw)
}

// releasedEntry renders a v4.5.0 template exactly as the v4.5.0 installer did:
// the verifier's method placeholder expands to the method v4.5.0 shipped.
func releasedEntry(t *testing.T, spec Spec) map[string]any {
	t.Helper()
	prompt := releasedFixture(t, spec.Name+".md", releasedAgentAssets[spec.Name])
	if spec.Name == "gentle-ai-verify" {
		method := releasedFixture(t, "verify-method.md", releasedVerifyMethod)
		prompt = strings.ReplaceAll(prompt, "{{GENTLE_AI_VERIFY_METHOD}}", method)
	}
	entry, err := Entry(spec)
	if err != nil {
		t.Fatal(err)
	}
	entry["prompt"] = prompt
	return entry
}

// TestShapeRecognizesTheReleasedAgentPrompts: an unmodified agent the previous
// release installed is still ours, so uninstall and upgrade replace it instead
// of preserving it as a user edit; any user modification of it is not ours.
func TestShapeRecognizesTheReleasedAgentPrompts(t *testing.T) {
	for _, spec := range parity {
		if _, ok := releasedAgentAssets[spec.Name]; !ok {
			continue
		}
		t.Run(spec.Name, func(t *testing.T) {
			released := releasedEntry(t, spec)
			if got, err := Shape(spec.Name, released); err != nil || !got {
				t.Fatalf("unmodified v4.5.0 %s: owned = %v, %v; want true", spec.Name, got, err)
			}
			for name, mutate := range map[string]func(map[string]any){
				"edited word":         func(e map[string]any) { e["prompt"] = strings.Replace(e["prompt"].(string), "You", "We", 1) },
				"trailing whitespace": func(e map[string]any) { e["prompt"] = e["prompt"].(string) + " " },
				"permission map":      func(e map[string]any) { e["permission"] = map[string]any{"write": "allow"} },
				"description":         func(e map[string]any) { e["description"] = "user description" },
			} {
				modified := releasedEntry(t, spec)
				mutate(modified)
				if got, err := Shape(spec.Name, modified); err != nil || got {
					t.Errorf("v4.5.0 %s with %s: owned = %v, %v; want false", spec.Name, name, got, err)
				}
			}
		})
	}
	if len(releasedAgentAssets) != 3 {
		t.Fatalf("released fixtures = %d, want explore, worker, and verify", len(releasedAgentAssets))
	}
}

// TestReleasedPromptRequiresEachSnippetOnce: the derivation edits only text it
// can place exactly; a current snippet that is missing or duplicated derives
// nothing, so no unrelated prompt is ever claimed as a released one.
func TestReleasedPromptRequiresEachSnippetOnce(t *testing.T) {
	for _, spec := range parity {
		edits, ok := releasedPromptEdits[spec.Name]
		if !ok {
			continue
		}
		t.Run(spec.Name, func(t *testing.T) {
			entry, err := Entry(spec)
			if err != nil {
				t.Fatal(err)
			}
			current := entry["prompt"].(string)
			for _, edit := range edits {
				if count := strings.Count(current, edit[0]); count != 1 {
					t.Errorf("current snippet %q appears %d times, want 1", edit[0], count)
				}
			}
			released, ok := releasedPrompt(spec.Name, current)
			if !ok || released != releasedEntry(t, spec)["prompt"].(string) {
				t.Fatalf("derived %s prompt differs from the v4.5.0 install (ok = %v)", spec.Name, ok)
			}
			first := edits[0][0]
			for name, prompt := range map[string]string{
				"missing snippet":    strings.Replace(current, first, "", 1),
				"duplicated snippet": current + "\n" + first,
			} {
				if got, ok := releasedPrompt(spec.Name, prompt); ok {
					t.Errorf("%s derived a released prompt (%d bytes); want none", name, len(got))
				}
			}
		})
	}
	if _, ok := releasedPrompt("jd-judge-a", "any prompt"); ok {
		t.Error("an agent without released edits derived a prompt")
	}
}
