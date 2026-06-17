package screens

import (
	"strings"
	"testing"
)

func TestRenderABWorkflow_NonEmpty(t *testing.T) {
	workflows := []string{"sdd", "academic-article-review", "paper-review"}
	out := RenderABWorkflow(workflows, 0)
	if out == "" {
		t.Fatal("RenderABWorkflow returned empty string")
	}
}

func TestRenderABWorkflow_SDDIsFirst(t *testing.T) {
	workflows := []string{"sdd", "academic-article-review", "paper-review"}
	out := RenderABWorkflow(workflows, 0)
	if !strings.Contains(out, "sdd") {
		t.Errorf("sdd option not found; output:\n%s", out)
	}
	// sdd should appear before academic-article-review in the rendered output.
	sddIdx := strings.Index(out, "sdd")
	acadIdx := strings.Index(out, "academic-article-review")
	if sddIdx < 0 || acadIdx < 0 {
		t.Fatal("expected both sdd and academic-article-review in output")
	}
	if sddIdx > acadIdx {
		t.Errorf("sdd should appear before academic-article-review; sdd at %d, acad at %d", sddIdx, acadIdx)
	}
}

func TestRenderABWorkflow_AllWorkflowsListed(t *testing.T) {
	workflows := []string{"sdd", "academic-article-review", "paper-review"}
	out := RenderABWorkflow(workflows, 0)
	for _, w := range workflows {
		if !strings.Contains(out, w) {
			t.Errorf("workflow %q not found in output", w)
		}
	}
}

func TestRenderABWorkflow_SingleWorkflow(t *testing.T) {
	workflows := []string{"sdd"}
	out := RenderABWorkflow(workflows, 0)
	if !strings.Contains(out, "sdd") {
		t.Errorf("sdd not found in single-workflow output:\n%s", out)
	}
}

func TestRenderABWorkflow_HeadingPresent(t *testing.T) {
	workflows := []string{"sdd", "paper-review"}
	out := RenderABWorkflow(workflows, 0)
	if !strings.Contains(out, "Workflow") {
		t.Errorf("heading not found; output:\n%s", out)
	}
}

func TestRenderABWorkflow_HelpTextPresent(t *testing.T) {
	workflows := []string{"sdd"}
	out := RenderABWorkflow(workflows, 0)
	if !strings.Contains(out, "j/k") || !strings.Contains(out, "enter") || !strings.Contains(out, "esc") {
		t.Errorf("help text not found; output:\n%s", out)
	}
}

func TestABWorkflowOptions_IncludesBack(t *testing.T) {
	workflows := []string{"sdd", "academic-article-review"}
	opts := ABWorkflowOptions(workflows)
	if len(opts) != 3 { // 2 workflows + Back
		t.Errorf("len(opts) = %d, want 3", len(opts))
	}
	if opts[len(opts)-1] != "Back" {
		t.Errorf("last option = %q, want 'Back'", opts[len(opts)-1])
	}
}

func TestABWorkflowOptions_WorkflowsComeFirst(t *testing.T) {
	workflows := []string{"sdd", "academic-article-review", "paper-review"}
	opts := ABWorkflowOptions(workflows)
	if len(opts) != 4 {
		t.Fatalf("len(opts) = %d, want 4", len(opts))
	}
	if opts[0] != "sdd" {
		t.Errorf("opts[0] = %q, want 'sdd'", opts[0])
	}
	if opts[1] != "academic-article-review" {
		t.Errorf("opts[1] = %q, want 'academic-article-review'", opts[1])
	}
}

func TestRenderABWorkflow_CursorHighlighted(t *testing.T) {
	workflows := []string{"sdd", "paper-review"}
	// With cursor at 0, sdd should appear with the cursor prefix.
	out := RenderABWorkflow(workflows, 0)
	// Cursor prefix is "▸" (styles.Cursor).
	if !strings.Contains(out, "▸") {
		t.Errorf("cursor marker not found in output with cursor=0:\n%s", out)
	}
}
