package opencodeagents

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
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

func TestEntryEmitsExpectedVisibility(t *testing.T) {
	for _, spec := range Parity(model.AgentOpenCode) {
		entry, err := Entry(spec)
		if err != nil {
			t.Fatalf("Entry(%q): %v", spec.Name, err)
		}
		if entry["mode"] != "subagent" {
			t.Fatalf("%s mode = %#v, want \"subagent\"", spec.Name, entry["mode"])
		}
		wantHidden := !strings.HasPrefix(spec.Name, "gentle-ai-")
		if entry["hidden"] != wantHidden {
			t.Fatalf("%s hidden = %#v, want %v", spec.Name, entry["hidden"], wantHidden)
		}
	}
	if Refuter()["hidden"] != true {
		t.Fatalf("Refuter hidden = %#v, want true", Refuter()["hidden"])
	}
	if Validator()["hidden"] != true {
		t.Fatalf("Validator hidden = %#v, want true", Validator()["hidden"])
	}
}

func TestShapeRecognizesCurrentAndLegacyODDVisibility(t *testing.T) {
	for _, spec := range Parity(model.AgentOpenCode) {
		cur, err := Entry(spec)
		if err != nil {
			t.Fatalf("Entry(%q): %v", spec.Name, err)
		}
		if got, err := Shape(spec.Name, cur); err != nil || !got {
			t.Fatalf("%s current Shape = (%v, %v), want (true, nil)", spec.Name, got, err)
		}
		if strings.HasPrefix(spec.Name, "gentle-ai-") {
			legacyHidden := maps.Clone(cur)
			legacyHidden["hidden"] = true
			legacyHidden["model"] = "user/model"
			legacyHidden["variant"] = "high"
			if got, err := Shape(spec.Name, legacyHidden); err != nil || !got {
				t.Fatalf("%s legacy hidden Shape = (%v, %v), want (true, nil)", spec.Name, got, err)
			}

			missingHidden := maps.Clone(cur)
			delete(missingHidden, "hidden")
			if got, err := Shape(spec.Name, missingHidden); err != nil || got {
				t.Fatalf("%s missing hidden Shape = (%v, %v), want (false, nil)", spec.Name, got, err)
			}

			stringHidden := maps.Clone(cur)
			stringHidden["hidden"] = "false"
			if got, err := Shape(spec.Name, stringHidden); err != nil || got {
				t.Fatalf("%s non-bool hidden Shape = (%v, %v), want (false, nil)", spec.Name, got, err)
			}
			continue
		}

		unhiddenNonODD := maps.Clone(cur)
		unhiddenNonODD["hidden"] = false
		if got, err := Shape(spec.Name, unhiddenNonODD); err != nil || got {
			t.Fatalf("%s unhidden non-ODD Shape = (%v, %v), want (false, nil)", spec.Name, got, err)
		}
	}

	verifyEntry, err := Entry(parity[1]) // gentle-ai-verify
	if err != nil {
		t.Fatal(err)
	}
	legacyPrompt, err := legacyVerifyPrompt(verifyEntry["prompt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	verifyLegacy := maps.Clone(verifyEntry)
	verifyLegacy["prompt"] = legacyPrompt
	verifyLegacy["hidden"] = true
	if got, err := Shape("gentle-ai-verify", verifyLegacy); err != nil || !got {
		t.Fatalf("gentle-ai-verify pre-refactor+legacy-hidden Shape = (%v, %v), want (true, nil)", got, err)
	}
}
