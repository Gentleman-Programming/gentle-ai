package engram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestParseSetupModeDefaultsToSupported(t *testing.T) {
	tests := []string{"", "invalid", "  weird  "}
	for _, value := range tests {
		if got := ParseSetupMode(value); got != SetupModeSupported {
			t.Fatalf("ParseSetupMode(%q) = %q, want %q", value, got, SetupModeSupported)
		}
	}
}

func TestParseSetupModeValues(t *testing.T) {
	if got := ParseSetupMode("off"); got != SetupModeOff {
		t.Fatalf("ParseSetupMode(off) = %q, want %q", got, SetupModeOff)
	}
	if got := ParseSetupMode("supported"); got != SetupModeSupported {
		t.Fatalf("ParseSetupMode(supported) = %q, want %q", got, SetupModeSupported)
	}
	if got := ParseSetupMode("opencode"); got != SetupModeOpenCode {
		t.Fatalf("ParseSetupMode(opencode) = %q, want %q", got, SetupModeOpenCode)
	}
}

func TestParseSetupStrict(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "yes", "on"}
	for _, value := range truthy {
		if !ParseSetupStrict(value) {
			t.Fatalf("ParseSetupStrict(%q) = false, want true", value)
		}
	}

	falsy := []string{"", "0", "false", "no", "off", "nah"}
	for _, value := range falsy {
		if ParseSetupStrict(value) {
			t.Fatalf("ParseSetupStrict(%q) = true, want false", value)
		}
	}
}

func TestSetupAgentSlug(t *testing.T) {
	tests := []struct {
		agent model.AgentID
		slug  string
		ok    bool
	}{
		{model.AgentOpenCode, "opencode", true},
		{model.AgentClaudeCode, "claude-code", true},
		{model.AgentGeminiCLI, "gemini-cli", true},
		{model.AgentCodex, "codex", true},
		{model.AgentAntigravity, "gemini-cli", true},
		{model.AgentWindsurf, "windsurf", true},
		{model.AgentQwenCode, "", false},
		{model.AgentCursor, "", false},
		{model.AgentVSCodeCopilot, "", false},
		// Hermes MCP is injected directly via YAML helpers — no engram setup slug.
		{model.AgentHermes, "", false},
	}

	for _, tt := range tests {
		slug, ok := SetupAgentSlug(tt.agent)
		if slug != tt.slug || ok != tt.ok {
			t.Fatalf("SetupAgentSlug(%q) = (%q, %v), want (%q, %v)", tt.agent, slug, ok, tt.slug, tt.ok)
		}
	}
}

func TestShouldAttemptSetup(t *testing.T) {
	if ShouldAttemptSetup(SetupModeOff, model.AgentOpenCode) {
		t.Fatal("ShouldAttemptSetup(off, opencode) = true, want false")
	}
	if !ShouldAttemptSetup(SetupModeOpenCode, model.AgentOpenCode) {
		t.Fatal("ShouldAttemptSetup(opencode, opencode) = false, want true")
	}
	if ShouldAttemptSetup(SetupModeOpenCode, model.AgentGeminiCLI) {
		t.Fatal("ShouldAttemptSetup(opencode, gemini-cli) = true, want false")
	}
	if !ShouldAttemptSetup(SetupModeSupported, model.AgentClaudeCode) {
		t.Fatal("ShouldAttemptSetup(supported, claude-code) = false, want true")
	}
	if ShouldAttemptSetup(SetupModeSupported, model.AgentQwenCode) {
		t.Fatal("ShouldAttemptSetup(supported, qwen-code) = true, want false")
	}
	if ShouldAttemptSetup(SetupModeSupported, model.AgentCursor) {
		t.Fatal("ShouldAttemptSetup(supported, cursor) = true, want false")
	}
}

// ---------------------------------------------------------------------------
// ProbeProtocolFlag (task 1.4) — canned-output tests faking the
// runProtocolProbeCommand seam, same pattern as VerifyVersion's execCommand
// fakes: no real process is spawned, so the four scenarios are deterministic
// and portable across environments.
// ---------------------------------------------------------------------------

func withFakeProtocolProbe(t *testing.T, fake func(ctx context.Context, command string) ([]byte, error)) {
	t.Helper()
	orig := runProtocolProbeCommand
	runProtocolProbeCommand = fake
	t.Cleanup(func() { runProtocolProbeCommand = orig })
}

func TestProbeProtocolFlagDetectsSupportedBinary(t *testing.T) {
	withFakeProtocolProbe(t, func(context.Context, string) ([]byte, error) {
		return []byte("Usage: engram setup <slug> [--protocol=slim|full]\n"), nil
	})

	stdout, err := ProbeProtocolFlag(context.Background())
	if err != nil {
		t.Fatalf("ProbeProtocolFlag() error = %v, want nil", err)
	}
	if !strings.Contains(stdout, "--protocol") {
		t.Fatalf("ProbeProtocolFlag() stdout = %q, want it to contain --protocol", stdout)
	}
}

func TestProbeProtocolFlagDegradesWhenFlagAbsent(t *testing.T) {
	withFakeProtocolProbe(t, func(context.Context, string) ([]byte, error) {
		return []byte("Usage: engram setup <slug>\n\nInteractive agent menu:\n  1) claude-code\n  2) codex\n"), nil
	})

	stdout, err := ProbeProtocolFlag(context.Background())
	if err != nil {
		t.Fatalf("ProbeProtocolFlag() error = %v, want nil", err)
	}
	if strings.Contains(stdout, "--protocol") {
		t.Fatalf("ProbeProtocolFlag() stdout = %q, want it to NOT contain --protocol (old binary)", stdout)
	}
}

func TestProbeProtocolFlagDegradesOnContextDeadlineTimeout(t *testing.T) {
	withFakeProtocolProbe(t, func(ctx context.Context, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	// A short deadline lets the test complete quickly instead of waiting out
	// the real 5-second production timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := ProbeProtocolFlag(ctx)
	if err == nil {
		t.Fatal("ProbeProtocolFlag() error = nil, want a timeout error so the caller degrades to flag-unsupported")
	}
}

func TestProbeProtocolFlagDegradesOnNonZeroExit(t *testing.T) {
	withFakeProtocolProbe(t, func(context.Context, string) ([]byte, error) {
		return nil, errors.New("exit status 2")
	})

	_, err := ProbeProtocolFlag(context.Background())
	if err == nil {
		t.Fatal("ProbeProtocolFlag() error = nil, want a non-nil error so the caller degrades to flag-unsupported")
	}
}

func TestProbeProtocolFlagCommandUsesProvidedBinary(t *testing.T) {
	var gotCommand string
	withFakeProtocolProbe(t, func(_ context.Context, command string) ([]byte, error) {
		gotCommand = command
		return []byte("Usage: engram setup <slug> [--protocol=slim|full]\n"), nil
	})

	if _, err := ProbeProtocolFlagCommand(context.Background(), "/tmp/beta/engram"); err != nil {
		t.Fatalf("ProbeProtocolFlagCommand() error = %v", err)
	}
	if gotCommand != "/tmp/beta/engram" {
		t.Fatalf("probe command = %q, want beta binary path", gotCommand)
	}
}
