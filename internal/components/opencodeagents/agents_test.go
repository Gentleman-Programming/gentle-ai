package opencodeagents

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
)

func TestShapeVerifierOwnership(t *testing.T) {
	cur, err := Entry(parity[1]) // gentle-ai-verify
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := assets.Read("opencode/agents/gentle-ai-verify.md")
	if err != nil {
		t.Fatal(err)
	}
	method, err := assets.Read("skills/_shared/verify-method.md")
	if err != nil {
		t.Fatal(err)
	}
	body := method[strings.Index(method, "For behavior changes"):]
	prereq := strings.ReplaceAll(strings.ReplaceAll(tpl,
		"plus the scratch-copy probes in the shared method.", "plus the scratch-copy probes below."),
		"{{GENTLE_AI_VERIFY_METHOD}}", body)
	legacy, _ := Entry(parity[1])
	if legacy["prompt"], err = legacyVerifyPrompt(cur["prompt"].(string)); err != nil || legacy["prompt"].(string) != prereq {
		t.Fatalf("legacy derivation drifted from prerequisite: %v", err)
	}
	// 3132b5bc6 installed this prompt; an asset edit that changes it must fail ownership.
	if sum := fmt.Sprintf("%x", sha256.Sum256([]byte(prereq))); sum != "9e3cca49a957c420aafc482114300fb6bf6558ab556f72bfc300dac216d5b9a3" {
		t.Fatalf("pre-refactor verifier prompt changed: %s", sum)
	}
	mod, _ := Entry(parity[1])
	mod["prompt"] = strings.Replace(cur["prompt"].(string), "You", "We", 1)
	for _, tc := range []struct {
		name string
		e    map[string]any
		want bool
	}{
		{"current", cur, true}, {"prerequisite", legacy, true}, {"modified", mod, false},
	} {
		if got, err := Shape("gentle-ai-verify", tc.e); err != nil || got != tc.want {
			t.Fatalf("%s: %v, %v", tc.name, got, err)
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"trailing whitespace": func(e map[string]any) { e["prompt"] = e["prompt"].(string) + " " },
		"added line":          func(e map[string]any) { e["prompt"] = e["prompt"].(string) + "\n" },
		"permission map": func(e map[string]any) {
			e["permission"] = map[string]any{"write": "allow", "edit": "deny", "task": "deny"}
		},
		"body without header": func(e map[string]any) { e["prompt"] = body },
	} {
		e, _ := Entry(parity[1])
		mutate(e)
		if got, err := Shape("gentle-ai-verify", e); err != nil || got {
			t.Fatalf("%s: %v, %v", name, got, err)
		}
	}
	for _, s := range parity {
		if s.Name != "gentle-ai-verify" {
			if e, err := Entry(s); err != nil {
				t.Fatal(err)
			} else if got, err := Shape(s.Name, e); err != nil || !got {
				t.Fatalf("%s: %v, %v", s.Name, got, err)
			}
		}
	}
}
