package opencodeagents

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
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

// releasedEntry renders a v4.5.0 template exactly as Entry renders the current
// one: the verifier's method placeholder expands to the shared method, which
// v4.5.0 shipped byte-identical.
func releasedEntry(t *testing.T, spec Spec) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "v4.5.0", spec.Name+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if sum := fmt.Sprintf("%x", sha256.Sum256(raw)); sum != releasedAgentAssets[spec.Name] {
		t.Fatalf("%s v4.5.0 fixture changed: %s", spec.Name, sum)
	}
	prompt := string(raw)
	if spec.Name == "gentle-ai-verify" {
		method, err := assets.Read("skills/_shared/verify-method.md")
		if err != nil {
			t.Fatal(err)
		}
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
