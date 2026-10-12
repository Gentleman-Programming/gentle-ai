//go:build linux

package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestGateShellInstallCandidateFailClosedNoContext(t *testing.T) {
	ctx := context.Background()
	err := GateShellInstallCandidate(ctx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for nil context")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateFailClosedEmptyContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), gateContextKey{}, nil)
	err := GateShellInstallCandidate(ctx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for empty context value")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateFailClosedWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), gateContextKey{}, "not-a-context")
	err := GateShellInstallCandidate(ctx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for wrong type in context")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateOrdinaryModeNotGated(t *testing.T) {
	ctx := context.Background()
	// Ordinary installation modes are outside the C6 protected scope and
	// preserve pre-gate production behavior: the gate must not block them.
	for _, mode := range []string{"separate", "shared", "invalid-mode"} {
		if err := GateShellInstallCandidate(ctx, mode); err != nil {
			t.Fatalf("mode %q must not be gated: %v", mode, err)
		}
	}
	// Sensitive modes stay fail-closed without a provisioned context.
	if err := GateShellInstallCandidate(ctx, "gentle-shell"); !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("sensitive mode must fail closed: %v", err)
	}
}

func TestGateShellInstallCandidateValidTestContext(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, err := p.Provision(ctx, shellinstaller.ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	consent, err := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	pc, err := shellinstaller.NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err != nil {
		t.Fatal(err)
	}

	testCtx := WithProvisionedContext(ctx, pc)
	err = GateShellInstallCandidate(testCtx, "gentle-shell")
	if err != nil {
		t.Fatalf("GateShellInstallCandidate failed for valid test context: %v", err)
	}
}

func TestGateShellInstallCandidatePiModeValidTestContext(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, err := p.Provision(ctx, shellinstaller.ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	consent, err := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "pi-takeover-e2e",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	pc, err := shellinstaller.NewProvisionedContext(ctx, guest, consent, "pi-takeover-e2e")
	if err != nil {
		t.Fatal(err)
	}

	testCtx := WithProvisionedContext(ctx, pc)
	err = GateShellInstallCandidate(testCtx, "pi")
	if err != nil {
		t.Fatalf("GateShellInstallCandidate failed for valid pi test context: %v", err)
	}
}

func TestGateShellInstallCandidateScopeMismatch(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, err := p.Provision(ctx, shellinstaller.ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Consent for gentle-shell but requesting pi mode
	consent, err := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	pc, err := shellinstaller.NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err != nil {
		t.Fatal(err)
	}

	testCtx := WithProvisionedContext(ctx, pc)
	err = GateShellInstallCandidate(testCtx, "pi")
	if err == nil {
		t.Fatal("expected error for scope mismatch")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateInstanceIDMismatch(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, err := p.Provision(ctx, shellinstaller.ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Create valid consent for this guest
	consent, err := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	pc, err := shellinstaller.NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err != nil {
		t.Fatal(err)
	}

	// Manually corrupt the context to have different InstanceID in guest vs consent
	// This tests the gate's InstanceID check directly
	pc.Guest.InstanceID = "different-instance"

	testCtx := WithProvisionedContext(ctx, pc)
	err = GateShellInstallCandidate(testCtx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for InstanceID mismatch")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateExpiredContext(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, err := p.Provision(ctx, shellinstaller.ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	consent, err := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	pc, err := shellinstaller.NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err != nil {
		t.Fatal(err)
	}

	// Manually expire the context
	pc.ExpiresAt = time.Now().Add(-1 * time.Second)

	testCtx := WithProvisionedContext(ctx, pc)
	err = GateShellInstallCandidate(testCtx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for expired context")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateInvalidAttestation(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, err := p.Provision(ctx, shellinstaller.ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	consent, err := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	pc, err := shellinstaller.NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt attestation
	pc.ProvisionerAttestation = []byte("wrong-attestation")

	testCtx := WithProvisionedContext(ctx, pc)
	err = GateShellInstallCandidate(testCtx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for invalid attestation")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateNilGuestInContext(t *testing.T) {
	ctx := context.Background()
	consent, _ := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: "test",
		Scope:      "gentle-shell-e2e",
	})

	pc := &shellinstaller.ProvisionedContext{
		Guest:   nil,
		Consent: consent,
	}

	testCtx := WithProvisionedContext(ctx, pc)
	err := GateShellInstallCandidate(testCtx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for nil guest")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGateShellInstallCandidateNilConsentInContext(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, _ := p.Provision(ctx, shellinstaller.ProvisionOptions{})

	pc := &shellinstaller.ProvisionedContext{
		Guest:   guest,
		Consent: nil,
	}

	testCtx := WithProvisionedContext(ctx, pc)
	err := GateShellInstallCandidate(testCtx, "gentle-shell")
	if err == nil {
		t.Fatal("expected error for nil consent")
	}
	if !errors.Is(err, ShellCandidateNotAuthorized) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestWithProvisionedContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	p := &shellinstaller.SyntheticProvisioner{}
	guest, _ := p.Provision(ctx, shellinstaller.ProvisionOptions{})
	consent, _ := shellinstaller.CreateConsentReceipt(ctx, shellinstaller.ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
	})
	pc, _ := shellinstaller.NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")

	testCtx := WithProvisionedContext(ctx, pc)
	val := testCtx.Value(gateContextKey{})
	retrieved, ok := val.(*shellinstaller.ProvisionedContext)
	if !ok || retrieved != pc {
		t.Fatal("WithProvisionedContext round-trip failed")
	}
}
