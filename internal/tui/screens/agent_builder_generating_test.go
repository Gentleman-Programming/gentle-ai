package screens

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderABGenerating_NonEmpty(t *testing.T) {
	out := RenderABGenerating("claude-code", 0, nil, 0)
	if out == "" {
		t.Fatal("RenderABGenerating returned empty string")
	}
}

func TestRenderABGenerating_HeadingPresent(t *testing.T) {
	out := RenderABGenerating("claude-code", 0, nil, 0)
	if !strings.Contains(out, "Generating Your Agent") {
		t.Errorf("heading not found; output:\n%s", out)
	}
}

func TestRenderABGenerating_ShowsEngineName(t *testing.T) {
	out := RenderABGenerating("opencode", 2, nil, 0)
	if !strings.Contains(out, "opencode") {
		t.Errorf("engine name not found; output:\n%s", out)
	}
}

func TestRenderABGenerating_WithError_ShowsErrorMessage(t *testing.T) {
	genErr := errors.New("connection timeout")
	out := RenderABGenerating("claude-code", 0, genErr, 0)
	if !strings.Contains(out, "connection timeout") {
		t.Errorf("error message not found; output:\n%s", out)
	}
	if !strings.Contains(out, "failed") {
		t.Errorf("expected 'failed' indicator in error state; output:\n%s", out)
	}
}

func TestRenderABGenerating_WithError_ShowsRetryOption(t *testing.T) {
	genErr := errors.New("some error")
	out := RenderABGenerating("claude-code", 0, genErr, 0)
	if !strings.Contains(out, "Retry") {
		t.Errorf("Retry option not found in error state; output:\n%s", out)
	}
}

func TestRenderABGenerating_WithError_FocusesCursor(t *testing.T) {
	out := RenderABGenerating("claude-code", 0, errors.New("failed"), 1)
	if !strings.Contains(out, "▸ Back") {
		t.Fatalf("Back should be focused for cursor 1; output:\n%s", out)
	}
}

func TestRenderABGenerating_NoError_ShowsSpinner(t *testing.T) {
	// Without error, should show spinner/loading text.
	out := RenderABGenerating("gemini", 1, nil, 0)
	if !strings.Contains(out, "Running") {
		t.Errorf("expected 'Running' spinner text; output:\n%s", out)
	}
}
