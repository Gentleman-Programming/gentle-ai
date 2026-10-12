//go:build linux

package shellinstaller

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateConsentReceiptValid(t *testing.T) {
	ctx := context.Background()
	opts := ConsentOptions{
		InstanceID: "test-instance-123",
		Scope:      "gentle-shell-e2e",
		TTL:        10 * time.Minute,
	}

	receipt, err := CreateConsentReceipt(ctx, opts)
	if err != nil {
		t.Fatalf("CreateConsentReceipt failed: %v", err)
	}
	if receipt == nil {
		t.Fatal("receipt is nil")
	}
	if receipt.InstanceID != opts.InstanceID {
		t.Fatalf("InstanceID mismatch: %q != %q", receipt.InstanceID, opts.InstanceID)
	}
	if receipt.Scope != opts.Scope {
		t.Fatalf("Scope mismatch: %q != %q", receipt.Scope, opts.Scope)
	}
	if receipt.HumanAttestation == nil || len(receipt.HumanAttestation) == 0 {
		t.Fatal("HumanAttestation is empty")
	}
	if receipt.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
	if receipt.ExpiresAt.Before(receipt.CreatedAt) {
		t.Fatal("ExpiresAt before CreatedAt")
	}
	if receipt.ReceiptID == "" {
		t.Fatal("ReceiptID is empty")
	}
}

func TestCreateConsentReceiptRequiresInstanceID(t *testing.T) {
	ctx := context.Background()
	_, err := CreateConsentReceipt(ctx, ConsentOptions{Scope: "test"})
	if err == nil {
		t.Fatal("expected error for missing InstanceID")
	}
	if !errors.Is(err, ErrConsentRequired) && err.Error() != "InstanceID required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateConsentReceiptRequiresScope(t *testing.T) {
	ctx := context.Background()
	_, err := CreateConsentReceipt(ctx, ConsentOptions{InstanceID: "test"})
	if err == nil {
		t.Fatal("expected error for missing Scope")
	}
	if err.Error() != "Scope required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateConsentReceiptTTLDefault(t *testing.T) {
	ctx := context.Background()
	before := time.Now()
	receipt, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test",
		Scope:      "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()

	expectedMin := before.Add(5 * time.Minute)
	expectedMax := after.Add(5 * time.Minute)
	if receipt.ExpiresAt.Before(expectedMin) || receipt.ExpiresAt.After(expectedMax) {
		t.Fatalf("ExpiresAt %v not in expected range [%v, %v]", receipt.ExpiresAt, expectedMin, expectedMax)
	}
}

func TestCreateConsentReceiptTTLMaxOneHour(t *testing.T) {
	ctx := context.Background()
	_, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test",
		Scope:      "test",
		TTL:        2 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for TTL > 1 hour")
	}
	if err.Error() != "TTL exceeds maximum 1 hour" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyConsentReceiptValid(t *testing.T) {
	ctx := context.Background()
	receipt, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test-instance",
		Scope:      "gentle-shell-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyConsentReceipt(ctx, receipt, "test-instance", "gentle-shell-e2e")
	if err != nil {
		t.Fatalf("VerifyConsentReceipt failed: %v", err)
	}
}

func TestVerifyConsentReceiptInstanceIDMismatch(t *testing.T) {
	ctx := context.Background()
	receipt, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "instance-A",
		Scope:      "gentle-shell-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyConsentReceipt(ctx, receipt, "instance-B", "gentle-shell-e2e")
	if err == nil {
		t.Fatal("expected error for InstanceID mismatch")
	}
	if err.Error() != "receipt InstanceID \"instance-A\" does not match guest \"instance-B\"" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyConsentReceiptScopeMismatch(t *testing.T) {
	ctx := context.Background()
	receipt, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test-instance",
		Scope:      "gentle-shell-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyConsentReceipt(ctx, receipt, "test-instance", "pi-takeover-e2e")
	if err == nil {
		t.Fatal("expected error for Scope mismatch")
	}
	if err.Error() != "receipt scope \"gentle-shell-e2e\" does not match requested \"pi-takeover-e2e\"" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyConsentReceiptExpired(t *testing.T) {
	ctx := context.Background()
	receipt, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test-instance",
		Scope:      "gentle-shell-e2e",
		TTL:        -1 * time.Second, // Already expired
	})
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyConsentReceipt(ctx, receipt, "test-instance", "gentle-shell-e2e")
	if err == nil {
		t.Fatal("expected error for expired receipt")
	}
	if err.Error() != "consent receipt expired" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyConsentReceiptTamperedReceiptID(t *testing.T) {
	ctx := context.Background()
	receipt, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test-instance",
		Scope:      "gentle-shell-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with ReceiptID
	receipt.ReceiptID = "tampered"

	err = VerifyConsentReceipt(ctx, receipt, "test-instance", "gentle-shell-e2e")
	if err == nil {
		t.Fatal("expected error for tampered ReceiptID")
	}
	if err.Error() != "receipt ID mismatch; possible tampering" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyConsentReceiptTamperedAttestation(t *testing.T) {
	ctx := context.Background()
	receipt, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test-instance",
		Scope:      "gentle-shell-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with HumanAttestation
	receipt.HumanAttestation = []byte("tampered")

	err = VerifyConsentReceipt(ctx, receipt, "test-instance", "gentle-shell-e2e")
	if err == nil {
		t.Fatal("expected error for tampered attestation")
	}
	if err.Error() != "receipt ID mismatch; possible tampering" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyConsentReceiptNilReceipt(t *testing.T) {
	ctx := context.Background()
	err := VerifyConsentReceipt(ctx, nil, "test", "test")
	if err == nil {
		t.Fatal("expected error for nil receipt")
	}
	if err.Error() != "receipt is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConsentReceiptDeterministicAttestation(t *testing.T) {
	ctx := context.Background()
	r1, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "same-instance",
		Scope:      "same-scope",
	})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "same-instance",
		Scope:      "same-scope",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Same InstanceID+Scope should produce same attestation (deterministic for tests)
	if !bytesEqual(r1.HumanAttestation, r2.HumanAttestation) {
		t.Fatal("attestation not deterministic for same InstanceID+Scope")
	}
	// But ReceiptID differs because CreatedAt differs
	if r1.ReceiptID == r2.ReceiptID {
		t.Fatal("ReceiptID should differ due to different CreatedAt")
	}
}

func TestConsentReceiptDifferentInstanceIDDifferentAttestation(t *testing.T) {
	ctx := context.Background()
	r1, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "instance-1",
		Scope:      "same-scope",
	})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "instance-2",
		Scope:      "same-scope",
	})
	if err != nil {
		t.Fatal(err)
	}

	if bytesEqual(r1.HumanAttestation, r2.HumanAttestation) {
		t.Fatal("attestation should differ for different InstanceID")
	}
}

func TestConsentReceiptDifferentScopeDifferentAttestation(t *testing.T) {
	ctx := context.Background()
	r1, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "same-instance",
		Scope:      "scope-A",
	})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "same-instance",
		Scope:      "scope-B",
	})
	if err != nil {
		t.Fatal(err)
	}

	if bytesEqual(r1.HumanAttestation, r2.HumanAttestation) {
		t.Fatal("attestation should differ for different Scope")
	}
}

func TestConsentErrorUnwrap(t *testing.T) {
	err := &ConsentError{Op: "Verify", Err: ErrConsentExpired}
	if !errors.Is(err, ErrConsentExpired) {
		t.Fatal("ConsentError does not unwrap")
	}
	if err.Error() != "consent Verify: consent receipt expired" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
}