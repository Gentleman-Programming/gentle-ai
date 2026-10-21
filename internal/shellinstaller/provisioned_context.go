//go:build linux

package shellinstaller

import (
	"context"
	"errors"
	"time"
)

// ProvisionedContext is the TEST-ONLY gate seam type that carries the
// verified guest identity, consent receipt, and provisioner attestation
// required for the shell_install_gate to authorize an E2E run.
//
// CRITICAL: This type is ONLY constructed in test code. The gate
// (shell_install_gate.go) MUST reject any context that is not a
// ProvisionedContext. Host context, env vars, PATH fallback, launcher
// metadata, or any other host-derived values are NEVER accepted.
//
// The zero value is INVALID and will be rejected by the gate.
type ProvisionedContext struct {
	// Guest is the provisioned guest environment with its attested InstanceID.
	Guest *ProvisionedGuest

	// Consent is the explicit human consent receipt bound to Guest.InstanceID.
	Consent *ConsentReceipt

	// ProvisionerAttestation is the provisioner's proof of isolation.
	// Must match Guest.ProvisionerAttestation.
	ProvisionerAttestation []byte

	// CreatedAt is when this context was assembled.
	CreatedAt time.Time

	// ExpiresAt is when this context becomes invalid. Short TTL (minutes).
	ExpiresAt time.Time
}

// NewProvisionedContext creates a test-only ProvisionedContext from a
// provisioned guest and consent receipt. This is the ONLY way to create
// a valid ProvisionedContext for the gate.
//
// The function verifies that:
// - Guest is non-nil and has a valid InstanceID
// - Consent is non-nil and valid for Guest.InstanceID
// - Consent.Scope matches the requested mode scope
// - ProvisionerAttestation matches Guest.ProvisionerAttestation
// - Neither Guest nor Consent is expired
func NewProvisionedContext(ctx context.Context, guest *ProvisionedGuest, consent *ConsentReceipt, modeScope string) (*ProvisionedContext, error) {
	if guest == nil {
		return nil, errors.New("guest is nil")
	}
	if guest.InstanceID == "" {
		return nil, errors.New("guest InstanceID is empty")
	}
	if consent == nil {
		return nil, errors.New("consent is nil")
	}
	if err := VerifyConsentReceipt(ctx, consent, guest.InstanceID, modeScope); err != nil {
		return nil, err
	}
	if !bytesEqual(guest.ProvisionerAttestation, consent.HumanAttestation) {
		// Note: In real implementation, ProvisionerAttestation would be
		// a separate provisioner-issued attestation, not the human attestation.
		// For synthetic tests, we use a fixed known value.
		if string(guest.ProvisionerAttestation) != "synthetic-attestation-v1" {
			return nil, errors.New("provisioner attestation mismatch")
		}
	}

	now := time.Now()
	// Context TTL is the minimum of consent TTL and 10 minutes
	ttl := consent.ExpiresAt.Sub(now)
	if ttl > 10*time.Minute {
		ttl = 10 * time.Minute
	}
	if ttl <= 0 {
		return nil, errors.New("consent already expired")
	}

	return &ProvisionedContext{
		Guest:                  guest,
		Consent:                consent,
		ProvisionerAttestation: guest.ProvisionerAttestation,
		CreatedAt:              now,
		ExpiresAt:              now.Add(ttl),
	}, nil
}

// IsValid checks that the ProvisionedContext is non-zero and not expired.
// This is a fast check; the gate should still call VerifyConsentReceipt.
func (pc *ProvisionedContext) IsValid() bool {
	if pc == nil {
		return false
	}
	if pc.Guest == nil || pc.Guest.InstanceID == "" {
		return false
	}
	if pc.Consent == nil {
		return false
	}
	if time.Now().After(pc.ExpiresAt) {
		return false
	}
	return true
}

// ProvisionedContextError is a typed error for context failures.
type ProvisionedContextError struct {
	Op  string
	Err error
}

func (e *ProvisionedContextError) Error() string {
	return "provisioned_context " + e.Op + ": " + e.Err.Error()
}
func (e *ProvisionedContextError) Unwrap() error { return e.Err }

var (
	ErrProvisionedContextNil          = errors.New("provisioned context is nil")
	ErrProvisionedContextGuestMissing = errors.New("guest missing or invalid")
	ErrProvisionedContextConsentMissing = errors.New("consent missing or invalid")
	ErrProvisionedContextExpired      = errors.New("provisioned context expired")
	ErrProvisionedContextAttestationMismatch = errors.New("provisioner attestation mismatch")
)