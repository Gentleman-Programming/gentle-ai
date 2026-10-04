package engram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

var osGetenv = os.Getenv

// RequiresInstanceIdentity reports whether an agent's integration hard-requires
// the engram instance-identity capability (the hidden "engram instance-id" CLI
// subcommand introduced upstream in engram core v2.0.0-rc.11).
//
// Only Pi and OpenCode require this capability:
//   - Pi fails deterministically at startup (DeterministicStartupError) when
//     the core lacks instance-id.
//   - OpenCode blocks write tools via tool.execute.before when the core lacks
//     instance-id, unless ENGRAM_URL is configured.
//
// Other agents are deliberately excluded based on observed upstream behavior:
//   - Claude Code: warns to stderr on older cores and exits 0; memory still works.
//   - Codex: exhibits identical soft-fail behavior to Claude Code.
//   - Gemini CLI, Cursor, Windsurf, etc.: never probe instance identity.
//
// A uniform all-agent gate was considered and rejected because it would
// falsely block working Claude Code, Codex, and Gemini setups on an older core.
func RequiresInstanceIdentity(agent model.AgentID) bool {
	switch agent {
	case model.AgentPi, model.AgentOpenCode:
		return true
	default:
		return false
	}
}

// AnyAgentRequiresInstanceIdentity reports whether any of the selected agents
// hard-requires the engram instance-identity capability. An empty or nil slice
// returns false.
func AnyAgentRequiresInstanceIdentity(agents []model.AgentID) bool {
	for _, agent := range agents {
		if RequiresInstanceIdentity(agent) {
			return true
		}
	}
	return false
}

// EffectiveRuntimeCommand returns the configured engram command string:
// ENGRAM_BIN when set and non-blank, otherwise the bare name "engram".
func EffectiveRuntimeCommand() string {
	bin := strings.TrimSpace(osGetenv("ENGRAM_BIN"))
	if bin != "" {
		return bin
	}
	return "engram"
}

// ResolveEffectiveRuntime resolves the effective engram executable path.
// It uses ENGRAM_BIN when set and non-blank, falling back to the bare name "engram".
// When the command is a bare name (or to verify executable presence), it consults
// the existing package lookPath seam in verify.go to obtain the real absolute path
// for user-facing messages. Returns a MissingBinaryError if lookPath fails.
func ResolveEffectiveRuntime() (string, error) {
	cmd := EffectiveRuntimeCommand()
	resolved, err := lookPath(cmd)
	if err != nil {
		return "", &MissingBinaryError{
			Command: cmd,
			Cause:   err,
		}
	}
	return resolved, nil
}

// ExternalServerConfigured reports whether an externally managed HTTP server
// is configured via ENGRAM_URL. When set and non-blank, the remote server is
// authoritative: upstream plugins skip local identity probing and avoid spawning
// the local binary, so the local core is not judged.
func ExternalServerConfigured() bool {
	return strings.TrimSpace(osGetenv("ENGRAM_URL")) != ""
}

// ExternalServerURL returns the configured ENGRAM_URL value, if any.
func ExternalServerURL() string {
	return strings.TrimSpace(osGetenv("ENGRAM_URL"))
}

// Canonical CLI recovery command to upgrade the Engram core.
// Looked up from the canonical CLI path in internal/app/app.go ("upgrade" command with an engram tool filter).
const EngramUpgradeRecoveryCommand = "gentle-ai upgrade engram"

// Capability identifiers and requirement descriptions.
const (
	CapabilityInstanceID               = "instance-id"
	DefaultInstanceIdentityRequirement = "Pi and OpenCode require engram core with instance-identity capability (minimum v2.0.0-rc.11)"
)

// ErrBinaryNotFound is the sentinel error indicating the engram binary is not found.
var ErrBinaryNotFound = errors.New("engram binary not found")

// MissingBinaryError records that the engram binary could not be found on PATH.
// A missing binary is explicitly distinct from an incompatible core so that install
// flows that tolerate uninstalled engram binaries do not fail on compatibility.
type MissingBinaryError struct {
	Command string
	Cause   error
}

func (e *MissingBinaryError) Error() string {
	if e.Command != "" {
		return fmt.Sprintf("engram binary %q not found in PATH: %v", e.Command, e.Cause)
	}
	return fmt.Sprintf("engram binary not found in PATH: %v", e.Cause)
}

func (e *MissingBinaryError) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return ErrBinaryNotFound
}

// IsMissingBinary reports whether err represents a missing binary condition.
// It strictly matches *MissingBinaryError and ErrBinaryNotFound.
func IsMissingBinary(err error) bool {
	if err == nil {
		return false
	}
	var missingErr *MissingBinaryError
	return errors.As(err, &missingErr) || errors.Is(err, ErrBinaryNotFound)
}

// IncompatibleCoreError records that the effective engram runtime lacks a required capability.
// User-facing messages name, in this exact order:
// 1. the resolved runtime
// 2. the reported version
// 3. the unmet capability
// 4. the requirement text
// 5. a runnable recovery command
// No em dashes anywhere.
type IncompatibleCoreError struct {
	Runtime         string
	Version         string
	Capability      string
	Requirement     string
	RecoveryCommand string
	Cause           error
}

func (e *IncompatibleCoreError) Error() string {
	versionStr := e.Version
	if strings.TrimSpace(versionStr) == "" {
		versionStr = "unknown"
	}
	capStr := e.Capability
	if strings.TrimSpace(capStr) == "" {
		capStr = CapabilityInstanceID
	}
	reqStr := e.Requirement
	if strings.TrimSpace(reqStr) == "" {
		reqStr = DefaultInstanceIdentityRequirement
	}
	recCmd := e.RecoveryCommand
	if strings.TrimSpace(recCmd) == "" {
		recCmd = EngramUpgradeRecoveryCommand
	}

	msg := fmt.Sprintf(
		"engram runtime %s (version %s) lacks required capability %s: %s.\nRun `%s` to upgrade.",
		e.Runtime,
		versionStr,
		capStr,
		reqStr,
		recCmd,
	)
	if e.Cause != nil {
		msg += fmt.Sprintf("\nProbe details: %v", e.Cause)
	}
	return msg
}

func (e *IncompatibleCoreError) Unwrap() error {
	return e.Cause
}

func newIncompatibleCoreError(runtime, version string, cause error) *IncompatibleCoreError {
	versionStr := strings.TrimSpace(version)
	if versionStr == "" {
		versionStr = "unknown"
	}
	return &IncompatibleCoreError{
		Runtime:         runtime,
		Version:         versionStr,
		Capability:      CapabilityInstanceID,
		Requirement:     DefaultInstanceIdentityRequirement,
		RecoveryCommand: EngramUpgradeRecoveryCommand,
		Cause:           cause,
	}
}

// IsIncompatibleCore reports whether err is an IncompatibleCoreError.
func IsIncompatibleCore(err error) bool {
	if err == nil {
		return false
	}
	var incompatErr *IncompatibleCoreError
	return errors.As(err, &incompatErr)
}

// probeCommandError carries the stderr from a failed capability probe so the
// verdict can rest on evidence rather than on the command having failed.
type probeCommandError struct {
	cause  error
	stderr string
}

func (e *probeCommandError) Error() string {
	if e.stderr != "" {
		return fmt.Sprintf("engram instance-id failed: %v (stderr: %s)", e.cause, e.stderr)
	}
	return fmt.Sprintf("engram instance-id failed: %v", e.cause)
}

func (e *probeCommandError) Unwrap() error { return e.cause }

// ProbeInconclusiveError records that the capability probe could not run or did
// not complete, so the core's capability was never established either way. Its
// message deliberately never advises upgrading the core, because no upgrade
// repairs a canceled context or a failed temporary directory.
type ProbeInconclusiveError struct {
	Runtime string
	Reason  string
	Cause   error
}

func (e *ProbeInconclusiveError) Error() string {
	msg := fmt.Sprintf("engram capability probe for %s was inconclusive: %s", e.Runtime, e.Reason)
	if e.Cause != nil {
		msg += fmt.Sprintf(" (%v)", e.Cause)
	}
	return msg
}

func (e *ProbeInconclusiveError) Unwrap() error { return e.Cause }

// IsProbeInconclusive reports whether err means the probe could not complete.
func IsProbeInconclusive(err error) bool {
	if err == nil {
		return false
	}
	var inconclusive *ProbeInconclusiveError
	return errors.As(err, &inconclusive)
}

// isProvenIncompatible reports whether err is evidence that the core lacks the
// instance-identity capability. The only such proof is a non-zero exit whose
// stderr names the subcommand as unknown. A canceled context, a failed
// temporary directory, or an exit carrying no such evidence proves nothing, so
// it must never be reported as an incompatible core.
func isProvenIncompatible(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr *probeCommandError
	if !errors.As(err, &cmdErr) {
		return false
	}
	var exited interface{ ExitCode() int }
	if !errors.As(cmdErr.cause, &exited) || exited.ExitCode() == 0 {
		return false
	}
	stderr := strings.ToLower(cmdErr.stderr)
	return strings.Contains(stderr, "unknown command") && strings.Contains(stderr, "instance-id")
}

var instanceIDProbeTimeout = 5 * time.Second

// probeInstanceIDCommand executes `<command> instance-id` with the given environment and context.
// Package-level seam so tests can drive the probe without spawning a real process.
var probeInstanceIDCommand = func(ctx context.Context, command string, env []string) ([]byte, []byte, error) {
	if strings.TrimSpace(command) == "" {
		command = "engram"
	}
	cmd := execCommandContext(ctx, command, "instance-id")
	cmd.Stdin = nil
	cmd.Env = env
	// Bound pipe draining even if process-tree cleanup itself fails.
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	terminateTree, err := startProbeProcessTree(cmd)
	if err != nil {
		return stdout.Bytes(), stderr.Bytes(), err
	}
	terminated := false
	defer func() {
		if !terminated {
			// Best-effort cleanup preserves the primary probe result after normal exit.
			_ = terminateTree()
		}
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return stdout.Bytes(), stderr.Bytes(), err
	case <-ctx.Done():
		terminated = true
		cleanupErr := terminateTree()
		<-done
		return stdout.Bytes(), stderr.Bytes(), errors.Join(ctx.Err(), cleanupErr)
	}
}

// ProbeInstanceID executes the instance-identity capability probe on the given command.
// It sets ENGRAM_NO_UPDATE_CHECK=1 in the child environment and points ENGRAM_DATA_DIR
// to a temporary directory that is cleaned up afterward.
// The probe deadline is always bounded to the earlier of the caller's deadline and instanceIDProbeTimeout.
// It validates that the probe returned exit code 0 and a single non-empty line of stdout.
func ProbeInstanceID(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		command = EffectiveRuntimeCommand()
	}

	tempDir, err := os.MkdirTemp("", "engram-probe-*")
	if err != nil {
		return "", &ProbeInconclusiveError{
			Runtime: command,
			Reason:  fmt.Sprintf("could not create a temporary probe data directory: %v", err),
			Cause:   err,
		}
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Build child environment:
	// Start with current environment, filtering out any existing ENGRAM_NO_UPDATE_CHECK
	// and ENGRAM_DATA_DIR to guarantee our probe hygiene values take effect.
	baseEnv := os.Environ()
	childEnv := make([]string, 0, len(baseEnv)+2)
	for _, kv := range baseEnv {
		if strings.HasPrefix(kv, "ENGRAM_NO_UPDATE_CHECK=") || strings.HasPrefix(kv, "ENGRAM_DATA_DIR=") {
			continue
		}
		childEnv = append(childEnv, kv)
	}
	childEnv = append(childEnv, "ENGRAM_NO_UPDATE_CHECK=1", "ENGRAM_DATA_DIR="+tempDir)

	// Always bound the probe to the earlier of the caller's deadline and instanceIDProbeTimeout.
	probeCtx, cancel := context.WithTimeout(ctx, instanceIDProbeTimeout)
	defer cancel()

	stdoutBytes, stderrBytes, cmdErr := probeInstanceIDCommand(probeCtx, command, childEnv)
	if probeCtx.Err() != nil {
		return "", &ProbeInconclusiveError{
			Runtime: command,
			Reason:  probeCtx.Err().Error(),
			Cause:   probeCtx.Err(),
		}
	}
	if cmdErr != nil {
		return "", &probeCommandError{
			cause:  cmdErr,
			stderr: strings.TrimSpace(string(stderrBytes)),
		}
	}

	// Validate stdout: must be a single non-empty line.
	rawOut := string(stdoutBytes)
	trimmedRight := strings.TrimSuffix(rawOut, "\n")
	trimmedRight = strings.TrimSuffix(trimmedRight, "\r")
	if trimmedRight == "" {
		return "", fmt.Errorf("engram instance-id returned empty output")
	}
	if strings.Contains(trimmedRight, "\n") || strings.Contains(trimmedRight, "\r") {
		return "", fmt.Errorf("engram instance-id returned invalid multi-line output; expected single line")
	}
	token := strings.TrimSpace(trimmedRight)
	if token == "" {
		return "", fmt.Errorf("engram instance-id returned whitespace-only output")
	}

	return token, nil
}

// CheckInstanceIdentityCapability tests whether the effective or specified engram
// runtime implements the instance-identity capability.
//
// If the binary is not found on PATH or via ENGRAM_BIN, it returns a MissingBinaryError.
// If the binary is found and passes the probe, it returns nil.
// Only a proven unsupported subcommand yields IncompatibleCoreError; other probe failures yield ProbeInconclusiveError.
func CheckInstanceIdentityCapability(ctx context.Context, command string) error {
	var resolvedPath string
	var err error

	if strings.TrimSpace(command) == "" {
		resolvedPath, err = ResolveEffectiveRuntime()
		if err != nil {
			return err
		}
	} else {
		resolvedPath, err = lookPath(command)
		if err != nil {
			return &MissingBinaryError{Command: command, Cause: err}
		}
	}

	_, probeErr := ProbeInstanceID(ctx, resolvedPath)
	if probeErr == nil {
		return nil
	}
	if !isProvenIncompatible(probeErr) {
		if IsProbeInconclusive(probeErr) {
			return probeErr
		}
		return &ProbeInconclusiveError{Runtime: resolvedPath, Reason: probeErr.Error(), Cause: probeErr}
	}
	var version string
	if ctx.Err() == nil {
		version, _ = VerifyVersionCommand(resolvedPath)
	}
	return newIncompatibleCoreError(resolvedPath, version, probeErr)
}

// InstanceIdentityOutcome describes the outcome of judging an engram runtime
// against companion instance-identity requirements.
type InstanceIdentityOutcome string

const (
	OutcomeExternalServer InstanceIdentityOutcome = "external-server"
	OutcomeNotRequired    InstanceIdentityOutcome = "not-required"
	OutcomeMissingBinary  InstanceIdentityOutcome = "missing-binary"
	OutcomeCompatible     InstanceIdentityOutcome = "compatible"
	OutcomeIncompatible   InstanceIdentityOutcome = "incompatible"
	// OutcomeProbeInconclusive means the probe could not run or did not finish,
	// so the core's capability was neither confirmed nor refuted.
	OutcomeProbeInconclusive InstanceIdentityOutcome = "probe-inconclusive"
)

// InstanceIdentityJudgment is the unified result of evaluating engram compatibility
// for a selection of agents.
type InstanceIdentityJudgment struct {
	Outcome InstanceIdentityOutcome
	Runtime string
	Err     error
}

// JudgeInstanceIdentityCapability evaluates whether the local engram core must be
// judged and whether it meets the instance-identity capability requirement.
//
// It evaluates conditions in this strict order:
//  1. External server configured via ENGRAM_URL: local core is not judged (OutcomeExternalServer).
//  2. No selected agent requires instance identity: requirement skipped (OutcomeNotRequired).
//  3. Binary missing: cannot locate executable (OutcomeMissingBinary, with MissingBinaryError payload).
//  4. Core compatible: probe succeeded (OutcomeCompatible).
//  5. Core incompatible: probe failed with proven unsupported subcommand (OutcomeIncompatible, with IncompatibleCoreError payload).
//  6. Probe inconclusive: probe failed or could not run without proving incompatibility (OutcomeProbeInconclusive, with ProbeInconclusiveError payload).
func JudgeInstanceIdentityCapability(ctx context.Context, agents []model.AgentID, command string) InstanceIdentityJudgment {
	if ExternalServerConfigured() {
		return InstanceIdentityJudgment{
			Outcome: OutcomeExternalServer,
		}
	}

	if !AnyAgentRequiresInstanceIdentity(agents) {
		return InstanceIdentityJudgment{
			Outcome: OutcomeNotRequired,
		}
	}

	var resolvedPath string
	var err error
	if strings.TrimSpace(command) == "" {
		resolvedPath, err = ResolveEffectiveRuntime()
	} else {
		resolvedPath, err = lookPath(command)
		if err != nil {
			err = &MissingBinaryError{Command: command, Cause: err}
		}
	}
	if err != nil {
		return InstanceIdentityJudgment{
			Outcome: OutcomeMissingBinary,
			Err:     err,
		}
	}

	_, probeErr := ProbeInstanceID(ctx, resolvedPath)
	if probeErr != nil {
		if !isProvenIncompatible(probeErr) {
			inconclusiveErr := probeErr
			if !IsProbeInconclusive(probeErr) {
				inconclusiveErr = &ProbeInconclusiveError{Runtime: resolvedPath, Reason: probeErr.Error(), Cause: probeErr}
			}
			return InstanceIdentityJudgment{Outcome: OutcomeProbeInconclusive, Runtime: resolvedPath, Err: inconclusiveErr}
		}
		var version string
		if ctx.Err() == nil {
			version, _ = VerifyVersionCommand(resolvedPath)
		}
		return InstanceIdentityJudgment{Outcome: OutcomeIncompatible, Runtime: resolvedPath, Err: newIncompatibleCoreError(resolvedPath, version, probeErr)}
	}

	return InstanceIdentityJudgment{
		Outcome: OutcomeCompatible,
		Runtime: resolvedPath,
	}
}

// Package-level test seam helpers (unexported to prevent leaking internal test seams into public API).

func setInstanceIDProbeForTest(t interface {
	Helper()
	Cleanup(func())
}, stdout, stderr string, err error) {
	t.Helper()
	orig := probeInstanceIDCommand
	probeInstanceIDCommand = func(ctx context.Context, command string, env []string) ([]byte, []byte, error) {
		return []byte(stdout), []byte(stderr), err
	}
	t.Cleanup(func() { probeInstanceIDCommand = orig })
}

func setInstanceIDProbeRunnerForTest(t interface {
	Helper()
	Cleanup(func())
}, fn func(ctx context.Context, command string, env []string) ([]byte, []byte, error)) {
	t.Helper()
	orig := probeInstanceIDCommand
	probeInstanceIDCommand = fn
	t.Cleanup(func() { probeInstanceIDCommand = orig })
}

func setVerifyLookPathForTest(t interface {
	Helper()
	Cleanup(func())
}, fn func(string) (string, error)) {
	t.Helper()
	orig := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = orig })
}
