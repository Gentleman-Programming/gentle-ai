package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
)

func newTestTextarea(value string) textarea.Model {
	ta := textarea.New()
	ta.SetValue(value)
	return ta
}

func TestRenderABPrompt_NonEmpty(t *testing.T) {
	ta := newTestTextarea("")
	out := RenderABPrompt(ta)
	if out == "" {
		t.Fatal("RenderABPrompt returned empty string")
	}
}

func TestRenderABPrompt_HeadingPresent(t *testing.T) {
	ta := newTestTextarea("")
	out := RenderABPrompt(ta)
	if !strings.Contains(out, "Describe Your Agent") {
		t.Errorf("heading not found; output:\n%s", out)
	}
}

func TestRenderABPrompt_EmptyTextarea_ShowsTypingHint(t *testing.T) {
	ta := newTestTextarea("")
	out := RenderABPrompt(ta)
	if !strings.Contains(out, "type a description") {
		t.Errorf("expected 'type a description' hint for empty textarea; output:\n%s", out)
	}
}

func TestRenderABPrompt_BackOptionPresent(t *testing.T) {
	ta := newTestTextarea("")
	out := RenderABPrompt(ta)
	if !strings.Contains(out, "Back") {
		t.Errorf("Back option not found; output:\n%s", out)
	}
}
