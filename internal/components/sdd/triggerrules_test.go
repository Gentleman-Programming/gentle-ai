package sdd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

var updateTriggerRules = flag.Bool("update-trigger-rules", false, "update trigger-rules golden files")

// 3.1 — RenderTriggerRules is deterministic.
func TestRenderTriggerRules_Deterministic(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out1 := RenderTriggerRules(rs)
	out2 := RenderTriggerRules(rs)
	if out1 != out2 {
		t.Error("RenderTriggerRules() is not deterministic: two calls returned different output")
	}
}

// 3.2 — RenderTriggerRules output is marker-free.
func TestRenderTriggerRules_MarkerFree(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out := RenderTriggerRules(rs)
	if strings.Contains(out, "<!-- gentle-ai:") {
		t.Error("RenderTriggerRules() output contains <!-- gentle-ai: marker (markers are added by InjectMarkdownSection)")
	}
	if strings.Contains(out, "<!-- /gentle-ai:") {
		t.Error("RenderTriggerRules() output contains <!-- /gentle-ai: close marker")
	}
}

// 3.3 — RenderTriggerRules output contains organic/not-a-gate note.
func TestRenderTriggerRules_OrganicNote(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out := RenderTriggerRules(rs)
	lower := strings.ToLower(out)
	hasOrganic := strings.Contains(lower, "organic")
	hasNotGate := strings.Contains(lower, "not a gate") || strings.Contains(lower, "not hard") || strings.Contains(lower, "not a hard gate")
	if !hasOrganic && !hasNotGate {
		t.Errorf("RenderTriggerRules() output does not contain organic/not-a-gate note; got:\n%s", out)
	}
}

// 3.4 — RenderTriggerRules mode wording.
func TestRenderTriggerRules_ModeWording(t *testing.T) {
	makeSet := func(mode model.TriggerMode) model.TriggerRuleSet {
		return model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{
					On:   model.EventPreCommit,
					When: model.TriggerWhen{Always: true},
					Run:  []string{"review-readability"},
					Mode: mode,
				},
			},
		}
	}

	t.Run("advisory uses consider language", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.ModeAdvisory))
		lower := strings.ToLower(out)
		if !strings.Contains(lower, "consider") {
			t.Errorf("advisory mode: expected 'consider' in output; got:\n%s", out)
		}
		for _, forbidden := range []string{"strongly", "must", "required", "critical"} {
			// Allow "strongly" only in the organic-note context (if it appears), but
			// not in the actual binding line. A simple check: count occurrences.
			if strings.Contains(lower, forbidden) {
				// Check if it's ONLY in the organic note header.
				// For simplicity, reject if forbidden words appear anywhere.
				t.Errorf("advisory mode: found forbidden word %q in output; got:\n%s", forbidden, out)
			}
		}
	})

	t.Run("strong uses strongly recommend language", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.ModeStrong))
		lower := strings.ToLower(out)
		if !strings.Contains(lower, "strongly recommend") {
			t.Errorf("strong mode: expected 'strongly recommend' in output; got:\n%s", out)
		}
		for _, forbidden := range []string{"gate", "block", "halt", "must not proceed"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("strong mode: found forbidden word %q in output; got:\n%s", forbidden, out)
			}
		}
	})

	t.Run("advisory and strong renderings are not equal", func(t *testing.T) {
		advOut := RenderTriggerRules(makeSet(model.ModeAdvisory))
		strOut := RenderTriggerRules(makeSet(model.ModeStrong))
		if advOut == strOut {
			t.Error("advisory and strong mode renderings must not be identical")
		}
	})
}

// 3.5 — RenderTriggerRules when phrasing.
func TestRenderTriggerRules_WhenPhrasing(t *testing.T) {
	makeSet := func(when model.TriggerWhen) model.TriggerRuleSet {
		return model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{
					On:   model.EventPreCommit,
					When: when,
					Run:  []string{"review-readability"},
					Mode: model.ModeAdvisory,
				},
			},
		}
	}

	t.Run("always", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.TriggerWhen{Always: true}))
		lower := strings.ToLower(out)
		if !strings.Contains(lower, "always") && !strings.Contains(lower, "every occurrence") && !strings.Contains(lower, "unconditionally") {
			t.Errorf("Always=true: expected 'always'/'every occurrence'/'unconditionally' in output; got:\n%s", out)
		}
	})

	t.Run("path globs", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.TriggerWhen{PathGlobs: []string{"**/auth/**", "**/payments/**"}}))
		if !strings.Contains(out, "**/auth/**") {
			t.Errorf("PathGlobs: expected '**/auth/**' verbatim in output; got:\n%s", out)
		}
		if !strings.Contains(out, "**/payments/**") {
			t.Errorf("PathGlobs: expected '**/payments/**' verbatim in output; got:\n%s", out)
		}
	})

	t.Run("min diff lines 400", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.TriggerWhen{MinDiffLines: 400}))
		if !strings.Contains(out, "400") {
			t.Errorf("MinDiffLines=400: expected '400' in output; got:\n%s", out)
		}
	})

	t.Run("phases contains design", func(t *testing.T) {
		set := model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{
					On:   model.EventPostSDDPhase,
					When: model.TriggerWhen{Phases: []string{"design", "apply"}},
					Run:  []string{"judgment-day"},
					Mode: model.ModeStrong,
				},
			},
		}
		out := RenderTriggerRules(set)
		if !strings.Contains(out, "design") {
			t.Errorf("Phases with design: expected 'design' in output; got:\n%s", out)
		}
	})

	t.Run("compound path OR diff lines", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.TriggerWhen{
			PathGlobs:    []string{"**/auth/**"},
			MinDiffLines: 200,
			Combine:      "or",
		}))
		lower := strings.ToLower(out)
		if !strings.Contains(out, "**/auth/**") {
			t.Errorf("compound: expected '**/auth/**' in output; got:\n%s", out)
		}
		if !strings.Contains(out, "200") {
			t.Errorf("compound: expected '200' in output; got:\n%s", out)
		}
		if !strings.Contains(lower, "or") {
			t.Errorf("compound: expected 'or' combinator in output; got:\n%s", out)
		}
	})
}

// 3.6 — RenderTriggerRules output has no more than 40 lines.
func TestRenderTriggerRules_LineBudget(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out := RenderTriggerRules(rs)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 40 {
		t.Errorf("RenderTriggerRules() output has %d lines, want <= 40; got:\n%s", len(lines), out)
	}
}

// 3.7 — RenderTriggerRules golden file test.
func TestRenderTriggerRules_Golden(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out := RenderTriggerRules(rs)

	goldenPath := filepath.Join("..", "..", "testdata", "golden", "trigger-rules-default.golden")

	if *updateTriggerRules {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("MkdirAll for golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(out), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", goldenPath, err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v\n\nRun with -update-trigger-rules to generate:\n  go test ./internal/components/sdd/ -run TestRenderTriggerRules_Golden -update-trigger-rules", goldenPath, err)
	}

	if out != string(expected) {
		t.Fatalf("golden mismatch for trigger-rules-default.golden\n\nRun with -update-trigger-rules to regenerate:\n  go test ./internal/components/sdd/ -run TestRenderTriggerRules_Golden -update-trigger-rules\n\nGot:\n%s", out)
	}
}
