package screens

import (
	"strings"
	"testing"
)

func TestRenderCompleteSuccessShowsGGANotesWhenInstalled(t *testing.T) {
	out := RenderComplete(CompletePayload{
		ConfiguredAgents:    1,
		InstalledComponents: 1,
		GGAInstalled:        true,
	})

	if !strings.Contains(out, "GGA (per project)") {
		t.Fatalf("missing GGA section: %q", out)
	}
	if !strings.Contains(out, "gga init") || !strings.Contains(out, "gga install") {
		t.Fatalf("missing GGA repo commands: %q", out)
	}
}

func TestRenderCompleteSuccessHidesGGANotesWhenNotInstalled(t *testing.T) {
	out := RenderComplete(CompletePayload{
		ConfiguredAgents:    1,
		InstalledComponents: 1,
		GGAInstalled:        false,
	})

	if strings.Contains(out, "GGA (per project)") {
		t.Fatalf("unexpected GGA section: %q", out)
	}
}

func TestRenderCompleteSuccessShowsEngramDataSize(t *testing.T) {
	out := RenderComplete(CompletePayload{
		ConfiguredAgents:    1,
		InstalledComponents: 1,
		EngramDataDir:       "/home/user/.engram",
		EngramDataSize:      1024 * 1024 * 1024, // 1.0 GB
	})

	if !strings.Contains(out, "/home/user/.engram") {
		t.Fatalf("missing Engram data dir: %q", out)
	}
	if !strings.Contains(out, "1.0 GB") {
		t.Fatalf("missing Engram data size: %q", out)
	}
}

func TestRenderCompleteSuccessOmitsSizeWhenZero(t *testing.T) {
	out := RenderComplete(CompletePayload{
		ConfiguredAgents:    1,
		InstalledComponents: 1,
		EngramDataDir:       "/home/user/.engram",
		EngramDataSize:      0,
	})

	if !strings.Contains(out, "/home/user/.engram") {
		t.Fatalf("missing Engram data dir: %q", out)
	}
	// Size line should be omitted when zero.
	if strings.Contains(out, "Database size:") {
		t.Fatalf("unexpected size line when zero: %q", out)
	}
}
