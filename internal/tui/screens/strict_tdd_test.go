package screens

import (
	"strings"
	"testing"
)

func TestRenderStrictTDDContainsTitle(t *testing.T) {
	output := RenderStrictTDD(false, 0)
	if !strings.Contains(output, "STRICT TDD MODE") {
		t.Errorf("RenderStrictTDD output missing title %q\ngot: %s", "STRICT TDD MODE", output)
	}
}

func TestRenderStrictTDDContainsEnableOption(t *testing.T) {
	output := RenderStrictTDD(false, 0)
	if !strings.Contains(output, "Enable") {
		t.Errorf("RenderStrictTDD output missing Enable option\ngot: %s", output)
	}
}

func TestRenderStrictTDDContainsDisableOption(t *testing.T) {
	output := RenderStrictTDD(false, 0)
	if !strings.Contains(output, "Disable") {
		t.Errorf("RenderStrictTDD output missing Disable option\ngot: %s", output)
	}
}

func TestRenderStrictTDDContainsBackOption(t *testing.T) {
	output := RenderStrictTDD(false, 0)
	if !strings.Contains(output, "Back") {
		t.Errorf("RenderStrictTDD output missing Back option\ngot: %s", output)
	}
}

func TestRenderStrictTDDEnabledState(t *testing.T) {
	// When enabled=true, "Enable" should appear as the selected radio.
	output := RenderStrictTDD(true, 0)
	if !strings.Contains(output, "(*) Enable") {
		t.Errorf("RenderStrictTDD(enabled=true) should show Enable as selected\ngot: %s", output)
	}
}

func TestRenderStrictTDDDisabledState(t *testing.T) {
	// When enabled=false, "Disable" should appear as the selected radio.
	output := RenderStrictTDD(false, 0)
	if !strings.Contains(output, "(*) Disable") {
		t.Errorf("RenderStrictTDD(enabled=false) should show Disable as selected\ngot: %s", output)
	}
}
