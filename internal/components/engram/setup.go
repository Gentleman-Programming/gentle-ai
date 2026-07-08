package engram

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// execCommandContext is a package-level seam over exec.CommandContext,
// mirroring the execCommand (verify.go) convention. It exists so tests can
// override the spawned process (e.g. substitute a short-lived real process
// for "engram setup --help") while exercising the real, unfaked
// runProtocolProbeCommand body under `go test -race` (JD-012).
var execCommandContext = exec.CommandContext

// protocolProbeTimeout is the hard deadline for ProbeProtocolFlag. It exists
// so a menu-printing old engram binary (see design.md Decision 4 Open
// Questions) can never hang the setup loop.
const protocolProbeTimeout = 5 * time.Second

// runProtocolProbeCommand executes `engram setup --help` with stdin detached
// (no TTY attached, so a menu-printing binary reading from stdin gets
// immediate EOF instead of blocking) and returns captured stdout. It is a
// package-level seam (built on the execCommandContext precedent from
// verify.go's execCommand) so tests can pin all four ProbeProtocolFlag
// outcomes — supported, unsupported, timeout, non-zero exit — without
// spawning a real process.
//
// Cancellation is delegated entirely to exec.CommandContext (design.md
// Decision 4): the stdlib starts and synchronizes the kill-on-cancel
// watcher internally, so there is no hand-rolled goroutine/select racing an
// unsynchronized read of cmd.Process against cmd.Start() (JD-012 — the
// prior hand-rolled version was reproducibly racy under `go test -race` and
// could leak the child process if ctx fired before Process was set).
//
// The command argument is the already-resolved engram binary for install paths
// that must avoid PATH shadowing; empty falls back to "engram".
var runProtocolProbeCommand = func(ctx context.Context, command string) ([]byte, error) {
	if strings.TrimSpace(command) == "" {
		command = "engram"
	}
	cmd := execCommandContext(ctx, command, "setup", "--help")
	cmd.Stdin = nil // explicit: never attach a TTY, avoids blocking on interactive input
	return cmd.Output()
}

// ProbeProtocolFlag detects whether the installed engram binary supports the
// --protocol verbosity flag by running a side-effect-free `engram setup
// --help` probe with a hard 5-second deadline. It returns the captured
// stdout on success. Any error (timeout, non-zero exit, binary not found)
// MUST be treated by the caller as "flag unsupported" — omit the flag and
// fall back to today's behavior (design.md Decision 4). Setup invocation
// itself MUST NOT fail as a result of this detection.
func ProbeProtocolFlag(ctx context.Context) (string, error) {
	return ProbeProtocolFlagCommand(ctx, "engram")
}

// ProbeProtocolFlagCommand is like ProbeProtocolFlag but probes the provided
// engram command path so beta installs and repaired Windows binaries use the
// same binary for setup, protocol detection, and version gating.
func ProbeProtocolFlagCommand(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, protocolProbeTimeout)
	defer cancel()

	out, err := runProtocolProbeCommand(ctx, command)
	if err != nil {
		return "", fmt.Errorf("probe engram setup --help: %w", err)
	}

	return string(out), nil
}

const (
	SetupModeEnvVar   = "GENTLE_AI_ENGRAM_SETUP_MODE"
	SetupStrictEnvVar = "GENTLE_AI_ENGRAM_SETUP_STRICT"
)

type SetupMode string

const (
	SetupModeOff       SetupMode = "off"
	SetupModeOpenCode  SetupMode = "opencode"
	SetupModeSupported SetupMode = "supported"
)

func ParseSetupMode(value string) SetupMode {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case string(SetupModeOff):
		return SetupModeOff
	case string(SetupModeOpenCode):
		return SetupModeOpenCode
	case "", string(SetupModeSupported):
		return SetupModeSupported
	default:
		return SetupModeSupported
	}
}

func ParseSetupStrict(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func SetupAgentSlug(agent model.AgentID) (string, bool) {
	switch agent {
	case model.AgentOpenCode:
		return "opencode", true
	case model.AgentKilocode:
		return "kilocode", true
	case model.AgentClaudeCode:
		return "claude-code", true
	case model.AgentGeminiCLI:
		return "gemini-cli", true
	case model.AgentCodex:
		// Codex slug registered for future MCP support; ShouldAttemptSetup gates on SupportsMCP().
		return "codex", true
	case model.AgentAntigravity:
		// Antigravity relies on Gemini's engram setup surface; the engram binary
		// does not currently expose a native "antigravity" slug.
		return "gemini-cli", true
	case model.AgentWindsurf:
		return "windsurf", true
	case model.AgentCursor, model.AgentVSCodeCopilot:
		// Cursor and VS Code Copilot do not use `engram setup` — their MCP
		// config is injected directly by the engram component. Returning false
		// here is intentional, not an omission.
		return "", false
	case model.AgentQwenCode:
		// Qwen uses direct settings.json injection only. The engram binary does
		// not currently expose a native `qwen-code` setup target.
		return "", false
	case model.AgentHermes:
		// Hermes MCP is injected directly via YAML helpers (UpsertHermesEngramBlock).
		// The engram binary does not expose a native Hermes setup target.
		return "", false
	default:
		return "", false
	}
}

func ShouldAttemptSetup(mode SetupMode, agent model.AgentID) bool {
	slug, ok := SetupAgentSlug(agent)
	if !ok {
		return false
	}

	switch mode {
	case SetupModeOff:
		return false
	case SetupModeSupported:
		return true
	case SetupModeOpenCode:
		return slug == "opencode"
	default:
		return slug == "opencode"
	}
}
