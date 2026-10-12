//go:build linux

package cli

import (
	"context"
	"errors"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// ShellCandidateNotAuthorized is returned when the gate refuses to
// authorize a shell installation candidate. This is the fail-closed
// default for all production/host contexts.
var ShellCandidateNotAuthorized = errors.New("shell candidate not authorized: explicit per-run consent and guest provisioning required")

// GateShellInstallCandidate evaluates whether a shell installation
// candidate is authorized to proceed. It is the single gate that
// enforces the protected provisioning/consent/verification design.
//
// For production/host contexts: ALWAYS returns ShellCandidateNotAuthorized.
// The gate NEVER accepts host context, env vars, PATH fallback, launcher
// metadata, or any host-derived values as proof.
//
// For test contexts: accepts ONLY a valid *shellinstaller.ProvisionedContext
// carried via context.Value with key gateContextKey. The context must be
// non-nil, valid (IsValid()), and have a consent scope matching the
// requested mode ("gentle-shell-e2e" or "pi-takeover-e2e").
//
// This function is the ONLY authorization path for shell installation.
// RunShellInstall (in shell_install.go) MUST call this before executing
// any installation.
func GateShellInstallCandidate(ctx context.Context, mode string) error {
	// Determine expected consent scope from mode
	var expectedScope string
	switch mode {
	case "gentle-shell":
		expectedScope = "gentle-shell-e2e"
	case "pi":
		expectedScope = "pi-takeover-e2e"
	default:
		return ShellCandidateNotAuthorized
	}

	// Extract ProvisionedContext from context (test-only seam)
	val := ctx.Value(gateContextKey{})
	pc, ok := val.(*shellinstaller.ProvisionedContext)
	if !ok || pc == nil {
		// No ProvisionedContext in context -> fail closed
		return ShellCandidateNotAuthorized
	}

	// Fast validity check
	if !pc.IsValid() {
		return ShellCandidateNotAuthorized
	}

	// Verify consent scope matches requested mode
	if pc.Consent == nil || pc.Consent.Scope != expectedScope {
		return ShellCandidateNotAuthorized
	}

	// Verify consent is still valid for the guest instance
	if pc.Guest == nil || pc.Guest.InstanceID != pc.Consent.InstanceID {
		return ShellCandidateNotAuthorized
	}

	// Verify provisioner attestation matches (synthetic test value)
	if string(pc.ProvisionerAttestation) != "synthetic-attestation-v1" {
		return ShellCandidateNotAuthorized
	}

	// All checks passed - test context authorized
	return nil
}

// gateContextKey is the context key for ProvisionedContext.
// It is unexported to prevent accidental use outside tests.
type gateContextKey struct{}

// WithProvisionedContext returns a new context carrying the
// ProvisionedContext for test authorization. This is the ONLY way
// to create a context that passes GateShellInstallCandidate.
//
// Usage in tests:
//
//	ctx = cli.WithProvisionedContext(ctx, pc)
//	err := cli.GateShellInstallCandidate(ctx, "gentle-shell")
func WithProvisionedContext(ctx context.Context, pc *shellinstaller.ProvisionedContext) context.Context {
	return context.WithValue(ctx, gateContextKey{}, pc)
}