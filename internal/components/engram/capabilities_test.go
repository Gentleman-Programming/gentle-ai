package engram

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestAgentRequiresInstanceIdentity(t *testing.T) {
	tests := []struct {
		name     string
		agent    model.AgentID
		expected bool
	}{
		{name: "Pi hard-requires instance-id", agent: model.AgentPi, expected: true},
		{name: "OpenCode hard-requires instance-id", agent: model.AgentOpenCode, expected: true},
		{name: "Claude Code does not require instance-id", agent: model.AgentClaudeCode, expected: false},
		{name: "Codex does not require instance-id", agent: model.AgentCodex, expected: false},
		{name: "Gemini CLI does not require instance-id", agent: model.AgentGeminiCLI, expected: false},
		{name: "Cursor does not require instance-id", agent: model.AgentCursor, expected: false},
		{name: "Windsurf does not require instance-id", agent: model.AgentWindsurf, expected: false},
		{name: "Kimi does not require instance-id", agent: model.AgentKimi, expected: false},
		{name: "Kiro does not require instance-id", agent: model.AgentKiroIDE, expected: false},
		{name: "Qwen does not require instance-id", agent: model.AgentQwenCode, expected: false},
		{name: "OpenClaw does not require instance-id", agent: model.AgentOpenClaw, expected: false},
		{name: "Antigravity does not require instance-id", agent: model.AgentAntigravity, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresInstanceIdentity(tt.agent); got != tt.expected {
				t.Errorf("RequiresInstanceIdentity(%q) = %v, want %v", tt.agent, got, tt.expected)
			}
		})
	}
}

func TestAnyAgentRequiresInstanceIdentity(t *testing.T) {
	tests := []struct {
		name     string
		agents   []model.AgentID
		expected bool
	}{
		{
			name:     "empty slice requires nothing",
			agents:   []model.AgentID{},
			expected: false,
		},
		{
			name:     "nil slice requires nothing",
			agents:   nil,
			expected: false,
		},
		{
			name:     "agents without requirement return false",
			agents:   []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentGeminiCLI},
			expected: false,
		},
		{
			name:     "selection containing Pi returns true",
			agents:   []model.AgentID{model.AgentClaudeCode, model.AgentPi},
			expected: true,
		},
		{
			name:     "selection containing OpenCode returns true",
			agents:   []model.AgentID{model.AgentOpenCode, model.AgentCodex},
			expected: true,
		},
		{
			name:     "selection containing both Pi and OpenCode returns true",
			agents:   []model.AgentID{model.AgentPi, model.AgentOpenCode},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnyAgentRequiresInstanceIdentity(tt.agents); got != tt.expected {
				t.Errorf("AnyAgentRequiresInstanceIdentity(%v) = %v, want %v", tt.agents, got, tt.expected)
			}
		})
	}
}

func TestResolveEffectiveRuntime(t *testing.T) {
	t.Run("ENGRAM_BIN set wins", func(t *testing.T) {
		t.Setenv("ENGRAM_BIN", "/custom/opt/engram")
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			if cmd == "/custom/opt/engram" {
				return "/custom/opt/engram", nil
			}
			return "", exec.ErrNotFound
		})

		got, err := ResolveEffectiveRuntime()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/custom/opt/engram" {
			t.Errorf("ResolveEffectiveRuntime() = %q, want %q", got, "/custom/opt/engram")
		}
	})

	t.Run("blank ENGRAM_BIN falls back to PATH engram", func(t *testing.T) {
		t.Setenv("ENGRAM_BIN", "   ")
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			if cmd == "engram" {
				return "/usr/local/bin/engram", nil
			}
			return "", exec.ErrNotFound
		})

		got, err := ResolveEffectiveRuntime()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/usr/local/bin/engram" {
			t.Errorf("ResolveEffectiveRuntime() = %q, want %q", got, "/usr/local/bin/engram")
		}
	})

	t.Run("bare name reported as absolute path via lookup seam", func(t *testing.T) {
		t.Setenv("ENGRAM_BIN", "my-engram")
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			if cmd == "my-engram" {
				return "/home/user/bin/my-engram", nil
			}
			return "", exec.ErrNotFound
		})

		got, err := ResolveEffectiveRuntime()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/home/user/bin/my-engram" {
			t.Errorf("ResolveEffectiveRuntime() = %q, want %q", got, "/home/user/bin/my-engram")
		}
	})

	t.Run("missing binary returns distinct missing binary error", func(t *testing.T) {
		t.Setenv("ENGRAM_BIN", "")
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "", exec.ErrNotFound
		})

		got, err := ResolveEffectiveRuntime()
		if err == nil {
			t.Fatalf("expected error, got path %q", got)
		}
		if !IsMissingBinary(err) {
			t.Errorf("expected IsMissingBinary(err) to be true, got err: %v", err)
		}
		if IsIncompatibleCore(err) {
			t.Errorf("missing binary must not be reported as incompatible core: %v", err)
		}
	})
}

func TestIsMissingBinary_Precision(t *testing.T) {
	// Must match MissingBinaryError and ErrBinaryNotFound only.
	missingErr := &MissingBinaryError{Command: "engram", Cause: exec.ErrNotFound}
	if !IsMissingBinary(missingErr) {
		t.Errorf("expected IsMissingBinary(missingErr) to be true")
	}
	if !IsMissingBinary(ErrBinaryNotFound) {
		t.Errorf("expected IsMissingBinary(ErrBinaryNotFound) to be true")
	}

	// Must NOT match general os.ErrNotExist or other errors.
	if IsMissingBinary(os.ErrNotExist) {
		t.Errorf("IsMissingBinary must not match generic os.ErrNotExist")
	}
	if IsMissingBinary(errors.New("file does not exist: something.txt")) {
		t.Errorf("IsMissingBinary must not match arbitrary error")
	}
	if IsMissingBinary(nil) {
		t.Errorf("IsMissingBinary(nil) must be false")
	}
}

func TestExternalServerConfiguredAndURL(t *testing.T) {
	t.Run("ENGRAM_URL set", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "http://127.0.0.1:8080")
		if !ExternalServerConfigured() {
			t.Errorf("expected ExternalServerConfigured() to be true when ENGRAM_URL is set")
		}
		if ExternalServerURL() != "http://127.0.0.1:8080" {
			t.Errorf("ExternalServerURL() = %q, want %q", ExternalServerURL(), "http://127.0.0.1:8080")
		}
	})

	t.Run("blank ENGRAM_URL", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "  ")
		if ExternalServerConfigured() {
			t.Errorf("expected ExternalServerConfigured() to be false when ENGRAM_URL is blank")
		}
		if ExternalServerURL() != "" {
			t.Errorf("ExternalServerURL() = %q, want empty", ExternalServerURL())
		}
	})

	t.Run("unset ENGRAM_URL", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "")
		if ExternalServerConfigured() {
			t.Errorf("expected ExternalServerConfigured() to be false when ENGRAM_URL is unset")
		}
		if ExternalServerURL() != "" {
			t.Errorf("ExternalServerURL() = %q, want empty", ExternalServerURL())
		}
	})
}

func TestProbeInstanceID_Outcomes(t *testing.T) {
	ctx := context.Background()

	t.Run("probe success with valid single line token", func(t *testing.T) {
		expectedToken := "a1b2c3d4e5f678901234567890abcdef"
		setInstanceIDProbeForTest(t, expectedToken+"\n", "", nil)

		token, err := ProbeInstanceID(ctx, "engram")
		if err != nil {
			t.Fatalf("unexpected probe error: %v", err)
		}
		if token != expectedToken {
			t.Errorf("ProbeInstanceID() = %q, want %q", token, expectedToken)
		}
	})

	t.Run("probe failure by exit code", func(t *testing.T) {
		exitErr := errors.New("exit status 1")
		setInstanceIDProbeForTest(t, "", "unknown command: instance-id\n", exitErr)

		token, err := ProbeInstanceID(ctx, "engram")
		if err == nil {
			t.Fatalf("expected error, got token %q", token)
		}
		if !strings.Contains(err.Error(), "unknown command: instance-id") {
			t.Errorf("error %q should capture stderr", err.Error())
		}
	})

	t.Run("probe failure by empty output", func(t *testing.T) {
		setInstanceIDProbeForTest(t, "", "", nil)

		token, err := ProbeInstanceID(ctx, "engram")
		if err == nil {
			t.Fatalf("expected error for empty output, got token %q", token)
		}
	})

	t.Run("probe failure by whitespace output", func(t *testing.T) {
		setInstanceIDProbeForTest(t, "   \n\n", "", nil)

		token, err := ProbeInstanceID(ctx, "engram")
		if err == nil {
			t.Fatalf("expected error for whitespace output, got token %q", token)
		}
	})

	t.Run("probe failure by multiline output", func(t *testing.T) {
		setInstanceIDProbeForTest(t, "token-line-1\ntoken-line-2\n", "", nil)

		token, err := ProbeInstanceID(ctx, "engram")
		if err == nil {
			t.Fatalf("expected error for multiline output, got token %q", token)
		}
		if !strings.Contains(err.Error(), "single line") {
			t.Errorf("error %q should mention single line requirement", err.Error())
		}
	})

	t.Run("probe failure by timeout even with long caller context", func(t *testing.T) {
		// Pass a 1-hour caller context to prove probe enforces its own bound.
		longCtx, cancel := context.WithTimeout(ctx, time.Hour)
		defer cancel()

		origTimeout := instanceIDProbeTimeout
		instanceIDProbeTimeout = 20 * time.Millisecond
		t.Cleanup(func() { instanceIDProbeTimeout = origTimeout })

		setInstanceIDProbeRunnerForTest(t, func(pCtx context.Context, command string, env []string) ([]byte, []byte, error) {
			deadline, ok := pCtx.Deadline()
			if !ok {
				t.Fatal("probe context missing deadline")
			}
			// Deadline must be bounded by instanceIDProbeTimeout (<= 50ms), not the 1-hour parent deadline
			if time.Until(deadline) > 50*time.Millisecond {
				t.Fatalf("probe deadline too large: %v", time.Until(deadline))
			}
			<-pCtx.Done()
			return nil, nil, pCtx.Err()
		})

		token, err := ProbeInstanceID(longCtx, "engram")
		if err == nil {
			t.Fatalf("expected error on timeout, got token %q", token)
		}
	})
}

func TestProbeInstanceID_Hygiene(t *testing.T) {
	ctx := context.Background()

	var recordedEnv []string
	var recordedDataDir string

	setInstanceIDProbeRunnerForTest(t, func(ctx context.Context, command string, env []string) ([]byte, []byte, error) {
		recordedEnv = env
		for _, e := range env {
			if strings.HasPrefix(e, "ENGRAM_DATA_DIR=") {
				recordedDataDir = strings.TrimPrefix(e, "ENGRAM_DATA_DIR=")
			}
		}

		if recordedDataDir == "" {
			return nil, nil, errors.New("ENGRAM_DATA_DIR was not set")
		}
		stat, err := os.Stat(recordedDataDir)
		if err != nil {
			return nil, nil, err
		}
		if !stat.IsDir() {
			return nil, nil, errors.New("ENGRAM_DATA_DIR is not a directory")
		}

		return []byte("0123456789abcdef0123456789abcdef\n"), nil, nil
	})

	token, err := ProbeInstanceID(ctx, "engram")
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if token != "0123456789abcdef0123456789abcdef" {
		t.Errorf("token = %q", token)
	}

	// 1. Child environment receives ENGRAM_NO_UPDATE_CHECK=1
	hasNoUpdateCheck := false
	for _, e := range recordedEnv {
		if e == "ENGRAM_NO_UPDATE_CHECK=1" {
			hasNoUpdateCheck = true
			break
		}
	}
	if !hasNoUpdateCheck {
		t.Errorf("child environment missing ENGRAM_NO_UPDATE_CHECK=1; got %v", recordedEnv)
	}

	// 2. Child environment receives ENGRAM_DATA_DIR that is not the user's real directory
	homeDir, _ := os.UserHomeDir()
	if recordedDataDir == "" {
		t.Fatal("ENGRAM_DATA_DIR was not set")
	}
	if homeDir != "" && strings.HasPrefix(recordedDataDir, filepath.Join(homeDir, ".engram")) {
		t.Errorf("ENGRAM_DATA_DIR points to user's real ~/.engram directory: %s", recordedDataDir)
	}

	// 3. The temporary directory is removed after the probe returns
	if _, err := os.Stat(recordedDataDir); !os.IsNotExist(err) {
		t.Errorf("temporary data dir %s was not cleaned up after probe; stat err: %v", recordedDataDir, err)
	}
}

func TestIncompatibleCoreError_OrderAndSubstrings(t *testing.T) {
	runtimePath := "/usr/local/bin/engram"
	versionStr := "1.20.0"
	causeErr := errors.New("unknown command: instance-id")

	err := newIncompatibleCoreError(runtimePath, versionStr, causeErr)
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	errStr := err.Error()

	// 1. Assert on substrings
	for _, sub := range []string{runtimePath, versionStr, CapabilityInstanceID, DefaultInstanceIdentityRequirement, EngramUpgradeRecoveryCommand} {
		if !strings.Contains(errStr, sub) {
			t.Errorf("error string missing %q; got: %s", sub, errStr)
		}
	}

	// 2. Assert strictly the ordered sequence: runtime, version, capability, requirement, recovery command
	idxRuntime := strings.Index(errStr, runtimePath)
	idxVersion := strings.Index(errStr, versionStr)
	idxCapability := strings.Index(errStr, CapabilityInstanceID)
	idxRequirement := strings.Index(errStr, DefaultInstanceIdentityRequirement)
	idxRecovery := strings.Index(errStr, EngramUpgradeRecoveryCommand)

	if !(idxRuntime < idxVersion && idxVersion < idxCapability && idxCapability < idxRequirement && idxRequirement < idxRecovery) {
		t.Errorf("error string substrings not in required order: runtime(%d), version(%d), capability(%d), requirement(%d), recovery(%d)\nFull error:\n%s",
			idxRuntime, idxVersion, idxCapability, idxRequirement, idxRecovery, errStr)
	}

	// 3. No em dashes anywhere
	if strings.Contains(errStr, "—") || strings.Contains(errStr, "–") {
		t.Errorf("error string contains em dash; got: %s", errStr)
	}

	// 4. Incompatibility predicate check
	if !IsIncompatibleCore(err) {
		t.Errorf("expected IsIncompatibleCore(err) to be true")
	}
	if IsMissingBinary(err) {
		t.Errorf("expected IsMissingBinary(err) to be false for incompatible core error")
	}
}

func TestCheckInstanceIdentityCapability_DistinctConditions(t *testing.T) {
	ctx := context.Background()

	t.Run("compatible core returns nil", func(t *testing.T) {
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "/usr/bin/engram", nil
		})
		SetVersionForTest(t, "engram 3.0.0")
		setInstanceIDProbeForTest(t, "fedcba9876543210fedcba9876543210\n", "", nil)

		err := CheckInstanceIdentityCapability(ctx, "")
		if err != nil {
			t.Fatalf("expected nil for compatible core, got: %v", err)
		}
	})

	t.Run("incompatible core returns IncompatibleCoreError with version and recovery command", func(t *testing.T) {
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "/usr/bin/engram", nil
		})
		SetVersionForTest(t, "1.20.0")
		setInstanceIDProbeForTest(t, "", "unknown command: instance-id\n", errors.New("exit status 1"))

		err := CheckInstanceIdentityCapability(ctx, "")
		if err == nil {
			t.Fatal("expected error for incompatible core, got nil")
		}

		if !IsIncompatibleCore(err) {
			t.Errorf("expected IsIncompatibleCore(err) to be true, got %v", err)
		}
		if IsMissingBinary(err) {
			t.Errorf("incompatible core must not be classified as missing binary: %v", err)
		}

		errStr := err.Error()
		if !strings.Contains(errStr, "/usr/bin/engram") {
			t.Errorf("error missing runtime path: %s", errStr)
		}
		if !strings.Contains(errStr, "1.20.0") {
			t.Errorf("error missing version: %s", errStr)
		}
		if !strings.Contains(errStr, "gentle-ai upgrade engram") {
			t.Errorf("error missing recovery command: %s", errStr)
		}
	})

	t.Run("missing binary returns MissingBinaryError and not IncompatibleCoreError", func(t *testing.T) {
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "", exec.ErrNotFound
		})

		err := CheckInstanceIdentityCapability(ctx, "")
		if err == nil {
			t.Fatal("expected error for missing binary, got nil")
		}

		if !IsMissingBinary(err) {
			t.Errorf("expected IsMissingBinary(err) to be true, got: %v", err)
		}
		if IsIncompatibleCore(err) {
			t.Errorf("missing binary must not be classified as incompatible core: %v", err)
		}
		if strings.Contains(err.Error(), "gentle-ai upgrade engram") {
			t.Errorf("missing binary should not tell the user to upgrade engram: %s", err.Error())
		}
	})
}

func TestJudgeInstanceIdentityCapability_AllFiveOutcomes(t *testing.T) {
	ctx := context.Background()

	t.Run("1. OutcomeExternalServer when ENGRAM_URL is set", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "http://127.0.0.1:8080")
		// Even if selected agents include Pi and binary is missing, external server takes precedence.
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "", exec.ErrNotFound
		})

		judgment := JudgeInstanceIdentityCapability(ctx, []model.AgentID{model.AgentPi}, "")
		if judgment.Outcome != OutcomeExternalServer {
			t.Errorf("judgment.Outcome = %v, want %v", judgment.Outcome, OutcomeExternalServer)
		}
		if judgment.Err != nil {
			t.Errorf("expected nil error for OutcomeExternalServer, got: %v", judgment.Err)
		}
	})

	t.Run("2. OutcomeNotRequired when no selected agent requires capability", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "")
		// Even if binary is missing, no requirement means not required.
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "", exec.ErrNotFound
		})

		// Empty agents
		jEmpty := JudgeInstanceIdentityCapability(ctx, nil, "")
		if jEmpty.Outcome != OutcomeNotRequired {
			t.Errorf("empty agents outcome = %v, want %v", jEmpty.Outcome, OutcomeNotRequired)
		}

		// Agents not requiring instance-id
		jClaude := JudgeInstanceIdentityCapability(ctx, []model.AgentID{model.AgentClaudeCode, model.AgentCodex}, "")
		if jClaude.Outcome != OutcomeNotRequired {
			t.Errorf("non-requiring agents outcome = %v, want %v", jClaude.Outcome, OutcomeNotRequired)
		}
	})

	t.Run("3. OutcomeMissingBinary when required but binary not found", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "")
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "", exec.ErrNotFound
		})

		judgment := JudgeInstanceIdentityCapability(ctx, []model.AgentID{model.AgentPi}, "")
		if judgment.Outcome != OutcomeMissingBinary {
			t.Errorf("judgment.Outcome = %v, want %v", judgment.Outcome, OutcomeMissingBinary)
		}
		if judgment.Err == nil || !IsMissingBinary(judgment.Err) {
			t.Errorf("expected MissingBinaryError payload, got: %v", judgment.Err)
		}
		if IsIncompatibleCore(judgment.Err) {
			t.Errorf("missing binary outcome must not carry IncompatibleCoreError: %v", judgment.Err)
		}
	})

	t.Run("4. OutcomeCompatible when required and probe succeeds", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "")
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "/usr/bin/engram", nil
		})
		setInstanceIDProbeForTest(t, "11223344556677889900aabbccddeeff\n", "", nil)

		judgment := JudgeInstanceIdentityCapability(ctx, []model.AgentID{model.AgentOpenCode}, "")
		if judgment.Outcome != OutcomeCompatible {
			t.Errorf("judgment.Outcome = %v, want %v", judgment.Outcome, OutcomeCompatible)
		}
		if judgment.Err != nil {
			t.Errorf("expected nil error for OutcomeCompatible, got: %v", judgment.Err)
		}
		if judgment.Runtime != "/usr/bin/engram" {
			t.Errorf("judgment.Runtime = %q, want /usr/bin/engram", judgment.Runtime)
		}
	})

	t.Run("5. OutcomeIncompatible when required and probe fails", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "")
		setVerifyLookPathForTest(t, func(cmd string) (string, error) {
			return "/usr/bin/engram", nil
		})
		SetVersionForTest(t, "1.18.0")
		setInstanceIDProbeForTest(t, "", "unknown command: instance-id\n", errors.New("exit status 1"))

		judgment := JudgeInstanceIdentityCapability(ctx, []model.AgentID{model.AgentPi}, "")
		if judgment.Outcome != OutcomeIncompatible {
			t.Errorf("judgment.Outcome = %v, want %v", judgment.Outcome, OutcomeIncompatible)
		}
		if judgment.Err == nil || !IsIncompatibleCore(judgment.Err) {
			t.Errorf("expected IncompatibleCoreError payload, got: %v", judgment.Err)
		}
		if IsMissingBinary(judgment.Err) {
			t.Errorf("incompatible outcome must not be classified as missing binary: %v", judgment.Err)
		}
		if !strings.Contains(judgment.Err.Error(), "gentle-ai upgrade engram") {
			t.Errorf("incompatible error payload missing upgrade command: %v", judgment.Err)
		}
	})
}
