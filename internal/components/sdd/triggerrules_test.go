package sdd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
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

func TestRenderTriggerRules_UsesBoundedReceiptLifecycle(t *testing.T) {
	rendered := RenderTriggerRules(catalog.DefaultTriggerRuleSet())
	for _, want := range []string{
		"Post-apply starts `review/start(target)` only when no valid receipt exists",
		"Pre-commit, pre-push, and pre-PR validate the same content-bound receipt",
		"Release from protected `main` may bypass receipt validation only when",
		"tag targets the current immutable `origin/main` SHA",
		"Major and post-incident releases require explicit extraordinary review",
		"missing → start explicitly after implementation/post-apply",
		"scope-changed → create a new lineage",
		"invalidated → require explicit maintainer action",
		"escalated → stop",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered trigger rules missing %q\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{
		"run the full 4R fan-out",
		"run exactly ONE lens selected by the risk table",
		"run `judgment-day`",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("rendered lifecycle rules retain automatic review clause %q\n%s", forbidden, rendered)
		}
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

// 3.3 — RenderTriggerRules output frames the block as a deterministic triage
// router (4R v2), not as organic/advisory recommendations.
func TestRenderTriggerRules_DeterministicRouterNote(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out := RenderTriggerRules(rs)
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "deterministic") {
		t.Errorf("RenderTriggerRules() output does not frame itself as deterministic; got:\n%s", out)
	}
	if !strings.Contains(lower, "decision procedure, not advice") {
		t.Errorf("RenderTriggerRules() output missing 'decision procedure, not advice' framing; got:\n%s", out)
	}
	for _, stale := range []string{"organic recommendation", "not enforced checkpoints", "consider running", "strongly recommend"} {
		if strings.Contains(lower, stale) {
			t.Errorf("RenderTriggerRules() output contains stale v1 advisory phrase %q; got:\n%s", stale, out)
		}
	}
}

// 3.3b — Risk classification exists only inside explicit review/start; lifecycle gates validate receipts.
func TestRenderTriggerRules_TriageTiers(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out := RenderTriggerRules(rs)
	for _, want := range []string{
		"Inside explicit `review/start(target)` only",
		"**Low**",
		"**Medium**",
		"exactly ONE dominant-risk lens",
		"**High**",
		"four initial 4R lens sweeps",
		"Generated goldens are excluded from the authored threshold but remain in snapshot identity",
		"Pre-commit, pre-push, and pre-PR validate the same content-bound receipt",
		"Release from protected `main` may bypass receipt validation only when",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderTriggerRules() output missing triage tier fragment %q; got:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"At **pre-commit**", "At **pre-push**", "At **pre-pr**"} {
		if strings.Contains(out, forbidden+", run `review-") {
			t.Errorf("lifecycle gate launches reviewer via %q", forbidden)
		}
	}
}

// 3.3c — The inline risk table carries the SAME row scopes as the fuller
// Review Lens Selection table in the orchestrator assets (R2-004): verbatim
// scope parity between the two tables in the rendered document.
func TestRenderTriggerRules_RiskTableScopeParity(t *testing.T) {
	rs := catalog.DefaultTriggerRuleSet()
	out := RenderTriggerRules(rs)
	for _, want := range []string{
		"Clear naming, structure, maintainability, or small refactors → `review-readability`",
		"Behavior, state, tests, determinism, or regressions → `review-reliability`",
		"Shell/process integration, partial failures, recovery, or degraded dependencies → `review-resilience`",
		"Security, permissions, data exposure/loss, architecture, or dependencies → `review-risk`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderTriggerRules() risk table missing lens-selection row scope %q; got:\n%s", want, out)
		}
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

	t.Run("advisory routes with trivial-diff exemption", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.ModeAdvisory))
		lower := strings.ToLower(out)
		if !strings.Contains(out, "trivial diff → no lens; otherwise") {
			t.Errorf("advisory mode: expected trivial-diff exemption routing in output; got:\n%s", out)
		}
		for _, forbidden := range []string{"consider running", "strongly recommend", "organic"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("advisory mode: found stale v1 phrase %q in output; got:\n%s", forbidden, out)
			}
		}
	})

	t.Run("strong runs unconditionally without trivial exemption", func(t *testing.T) {
		out := RenderTriggerRules(makeSet(model.ModeStrong))
		lower := strings.ToLower(out)
		if !strings.Contains(out, "run `review-readability`") {
			t.Errorf("strong mode: expected direct 'run `review-readability`' directive; got:\n%s", out)
		}
		if strings.Contains(out, "trivial diff → no lens; otherwise") {
			t.Errorf("strong mode: binding must not carry the trivial-diff exemption; got:\n%s", out)
		}
		for _, forbidden := range []string{"consider running", "strongly recommend", "gate", "block", "halt", "must not proceed"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("strong mode: found forbidden word %q in output; got:\n%s", forbidden, out)
			}
		}
	})

	t.Run("strong conditional full-4R carries the trivial exemption", func(t *testing.T) {
		set := model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{
					On: model.EventPrePR,
					When: model.TriggerWhen{
						PathGlobs:    []string{"**/auth/**"},
						MinDiffLines: 400,
						Combine:      "or",
					},
					Run:  []string{"review-risk", "review-resilience", "review-readability", "review-reliability"},
					Mode: model.ModeStrong,
				},
			},
		}
		out := RenderTriggerRules(set)
		if !strings.Contains(out, "trivial diff → no lens; else if the diff touches") {
			t.Errorf("strong conditional full-4R: expected trivial-diff exemption before the fan-out; got:\n%s", out)
		}
		if !strings.Contains(out, "else run exactly ONE lens selected by the risk table") {
			t.Errorf("strong conditional full-4R: expected standard-diff single-lens fallback; got:\n%s", out)
		}
		if strings.Count(out, "else if") != 1 || strings.Count(out, "; else run") != 1 {
			t.Errorf("strong conditional full-4R: expected one exhaustive if/else-if/else decision; got:\n%s", out)
		}
	})

	t.Run("advisory conditional full-4R preserves its condition", func(t *testing.T) {
		set := model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{
					On: model.EventPrePR,
					When: model.TriggerWhen{
						PathGlobs:    []string{"**/auth/**"},
						MinDiffLines: 400,
						Combine:      "or",
					},
					Run:  []string{"review-risk", "review-resilience", "review-readability", "review-reliability"},
					Mode: model.ModeAdvisory,
				},
			},
		}
		out := RenderTriggerRules(set)
		conditionalPrefix := "- At **pre-pr**, when the diff touches `**/auth/**` OR when the diff exceeds 400 changed lines:"
		if !strings.Contains(out, conditionalPrefix) {
			t.Errorf("advisory conditional full-4R: condition was dropped; got:\n%s", out)
		}
		if strings.Contains(out, "- At **pre-pr**: trivial diff → no lens; otherwise run `review-risk`") {
			t.Errorf("advisory conditional full-4R: unmatched standard diffs are routed unconditionally to full 4R; got:\n%s", out)
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
