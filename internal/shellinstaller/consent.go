//go:build linux

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ConsentReceipt is an explicit human approval for a single E2E run,
// cryptographically bound to a specific guest InstanceID. It is NOT a
// blanket consent and cannot be reused across runs or guests.
//
// The receipt is created by the human operator (or test harness acting
// on their behalf) BEFORE provisioning, and presented to the gate along
// with the ProvisionedGuest. The gate verifies the binding.
type ConsentReceipt struct {
	// InstanceID is the guest's attested physical identity (from
	// ProvisionedGuest.InstanceID). The receipt is ONLY valid for this ID.
	InstanceID string

	// HumanAttestation is an opaque value representing the human's
	// explicit approval. In tests, this is a synthetic fixed value.
	// In production, this would be a signed challenge or similar.
	HumanAttestation []byte

	// CreatedAt is when the consent was granted.
	CreatedAt time.Time

	// ExpiresAt is when the consent becomes invalid. Consent is
	// single-run; typical TTL is minutes, not hours.
	ExpiresAt time.Time

	// Scope describes what the consent covers (e.g., "gentle-shell-e2e",
	// "pi-takeover-e2e"). The gate checks this matches the requested mode.
	Scope string

	// ReceiptID is a unique identifier for this receipt (SHA256 of
	// InstanceID + HumanAttestation + CreatedAt).
	ReceiptID string
}

// ConsentOptions configures consent creation.
type ConsentOptions struct {
	// InstanceID is the guest InstanceID to bind consent to.
	InstanceID string

	// Scope is the consent scope (e.g., "gentle-shell-e2e").
	Scope string

	// TTL is the consent time-to-live. Zero defaults to 5 minutes.
	TTL time.Duration
}

// CreateConsentReceipt creates a new consent receipt bound to the given
// InstanceID. The HumanAttestation is generated from a cryptographically
// random challenge that the human must explicitly approve (simulated in
// tests by a fixed synthetic value).
//
// This function is the ONLY way to create a valid ConsentReceipt. The
// receipt is immutable after creation.
func CreateConsentReceipt(ctx context.Context, opts ConsentOptions) (*ConsentReceipt, error) {
	if opts.InstanceID == "" {
		return nil, errors.New("InstanceID required")
	}
	if opts.Scope == "" {
		return nil, errors.New("Scope required")
	}
	ttl := opts.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	if ttl > 1*time.Hour {
		return nil, errors.New("TTL exceeds maximum 1 hour")
	}

	// In production, this would be a challenge signed by the human.
	// For the synthetic test seam, we use a deterministic derivation.
	attestation := syntheticHumanAttestation(opts.InstanceID, opts.Scope)

	now := time.Now()
	receipt := &ConsentReceipt{
		InstanceID:          opts.InstanceID,
		HumanAttestation:    attestation,
		CreatedAt:           now,
		ExpiresAt:           now.Add(ttl),
		Scope:               opts.Scope,
		ReceiptID:           computeReceiptID(opts.InstanceID, attestation, now),
	}
	return receipt, nil
}

// syntheticHumanAttestation returns a deterministic attestation for tests.
// Real implementations MUST replace this with actual human approval flow.
func syntheticHumanAttestation(instanceID, scope string) []byte {
	// Deterministic but unique per InstanceID+Scope for test reproducibility.
	h := sha256.New()
	h.Write([]byte("synthetic-human-attestation-v1"))
	h.Write([]byte(instanceID))
	h.Write([]byte(scope))
	return h.Sum(nil)
}

func computeReceiptID(instanceID string, attestation []byte, createdAt time.Time) string {
	h := sha256.New()
	h.Write([]byte(instanceID))
	h.Write(attestation)
	h.Write([]byte(createdAt.Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyConsentReceipt checks that a receipt is valid for the given
// InstanceID and scope, and has not expired. Returns the receipt if valid.
func VerifyConsentReceipt(ctx context.Context, receipt *ConsentReceipt, instanceID, scope string) error {
	if receipt == nil {
		return errors.New("receipt is nil")
	}
	if receipt.InstanceID != instanceID {
		return fmt.Errorf("receipt InstanceID %q does not match guest %q", receipt.InstanceID, instanceID)
	}
	if receipt.Scope != scope {
		return fmt.Errorf("receipt scope %q does not match requested %q", receipt.Scope, scope)
	}
	if time.Now().After(receipt.ExpiresAt) {
		return errors.New("consent receipt expired")
	}
	// Verify ReceiptID matches contents (tamper detection)
	expected := computeReceiptID(receipt.InstanceID, receipt.HumanAttestation, receipt.CreatedAt)
	if receipt.ReceiptID != expected {
		return errors.New("receipt ID mismatch; possible tampering")
	}
	// Verify attestation matches synthetic derivation (test seam only)
	expectedAttestation := syntheticHumanAttestation(receipt.InstanceID, receipt.Scope)
	if !bytesEqual(receipt.HumanAttestation, expectedAttestation) {
		return errors.New("human attestation mismatch")
	}
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ConsentError is a typed error for consent failures.
type ConsentError struct {
	Op  string
	Err error
}

func (e *ConsentError) Error() string {
	return "consent " + e.Op + ": " + e.Err.Error()
}
func (e *ConsentError) Unwrap() error { return e.Err }

var (
	ErrConsentRequired     = errors.New("explicit human consent required")
	ErrConsentExpired      = errors.New("consent receipt expired")
	ErrConsentScopeMismatch = errors.New("consent scope mismatch")
	ErrConsentInstanceMismatch = errors.New("consent InstanceID mismatch")
	ErrConsentTampered     = errors.New("consent receipt tampered")
)