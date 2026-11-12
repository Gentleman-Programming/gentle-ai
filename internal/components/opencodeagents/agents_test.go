package opencodeagents

import (
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
	prereq := strings.ReplaceAll(strings.ReplaceAll(tpl,
		"plus the scratch-copy probes in the shared method.", "plus the scratch-copy probes below."),
		"{{GENTLE_AI_VERIFY_METHOD}}", method[strings.Index(method, "For behavior changes"):])
	legacy, _ := Entry(parity[1])
	if legacy["prompt"], err = legacyVerifyPrompt(cur["prompt"].(string)); err != nil || legacy["prompt"].(string) != prereq {
		t.Fatalf("legacy derivation drifted from prerequisite: %v", err)
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
