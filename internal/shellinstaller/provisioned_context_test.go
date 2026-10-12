//go:build linux

package shellinstaller

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewProvisionedContextValid(t *testing.T) {
	ctx := context.Background()
	p := &SyntheticProvisioner{}
	guest, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	consent, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	pc, err := NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err != nil {
		t.Fatalf("NewProvisionedContext failed: %v", err)
	}
	if pc == nil {
		t.Fatal("ProvisionedContext is nil")
	}
	if pc.Guest != guest {
		t.Fatal("Guest not set")
	}
	if pc.Consent != consent {
		t.Fatal("Consent not set")
	}
	if !pc.IsValid() {
		t.Fatal("IsValid returned false for valid context")
	}
	if pc.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
	if pc.ExpiresAt.Before(pc.CreatedAt) {
		t.Fatal("ExpiresAt before CreatedAt")
	}
}

func TestNewProvisionedContextNilGuest(t *testing.T) {
	ctx := context.Background()
	consent, _ := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test",
		Scope:      "test",
	})
	_, err := NewProvisionedContext(ctx, nil, consent, "test")
	if err == nil {
		t.Fatal("expected error for nil guest")
	}
	if err.Error() != "guest is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewProvisionedContextEmptyInstanceID(t *testing.T) {
	ctx := context.Background()
	guest := &ProvisionedGuest{InstanceID: ""}
	consent, _ := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "test",
		Scope:      "test",
	})
	_, err := NewProvisionedContext(ctx, guest, consent, "test")
	if err == nil {
		t.Fatal("expected error for empty InstanceID")
	}
	if err.Error() != "guest InstanceID is empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewProvisionedContextNilConsent(t *testing.T) {
	ctx := context.Background()
	p := &SyntheticProvisioner{}
	guest, _ := p.Provision(ctx, ProvisionOptions{})
	_, err := NewProvisionedContext(ctx, guest, nil, "test")
	if err == nil {
		t.Fatal("expected error for nil consent")
	}
	if err.Error() != "consent is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewProvisionedContextConsentInstanceIDMismatch(t *testing.T) {
	ctx := context.Background()
	p := &SyntheticProvisioner{}
	guest, _ := p.Provision(ctx, ProvisionOptions{})

	consent, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: "different-instance",
		Scope:      "gentle-shell-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err == nil {
		t.Fatal("expected error for InstanceID mismatch")
	}
	// Should wrap the consent verification error
}

func TestNewProvisionedContextConsentScopeMismatch(t *testing.T) {
	ctx := context.Background()
	p := &SyntheticProvisioner{}
	guest, _ := p.Provision(ctx, ProvisionOptions{})

	consent, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "pi-takeover-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err == nil {
		t.Fatal("expected error for Scope mismatch")
	}
}

func TestNewProvisionedContextExpiredConsent(t *testing.T) {
	ctx := context.Background()
	p := &SyntheticProvisioner{}
	guest, _ := p.Provision(ctx, ProvisionOptions{})

	consent, err := CreateConsentReceipt(ctx, ConsentOptions{
		InstanceID: guest.InstanceID,
		Scope:      "gentle-shell-e2e",
		TTL:        -1 * time.Second, // Expired
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewProvisionedContext(ctx, guest, consent, "gentle-shell-e2e")
	if err == nil {
		t.Fatal("expected error for expired consent")
	}
	if err.Error() != "consent receipt expired" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvisionedContextIsValidNil(t *testing.T) {
	var pc *ProvisionedContext
	if pc.IsValid() {
		t.Fatal("IsValid should return false for nil")
	}
}

func TestProvisionedContextIsValidEmptyGuest(t *testing.T) {
	pc := &ProvisionedContext{
		Guest: &ProvisionedGuest{},
		Consent: &ConsentReceipt{},
	}
	if pc.IsValid() {
		t.Fatal("IsValid should return false for empty guest")
	}
}

func TestProvisionedContextIsValidEmptyConsent(t *testing.T) {
	pc := &ProvisionedContext{
		Guest: &ProvisionedGuest{InstanceID: "test"},
		Consent: nil,
	}
	if pc.IsValid() {
		t.Fatal("IsValid should return false for nil consent")
	}
}

func TestProvisionedContextIsValidExpired(t *testing.T) {
	pc := &ProvisionedContext{
		Guest:      &ProvisionedGuest{InstanceID: "test"},
		Consent:    &ConsentReceipt{},
		ExpiresAt:  time.Now().Add(-1 * time.Second),
	}
	if pc.IsValid() {
		t.Fatal("IsValid should return false for expired context")
	}
}

func TestProvisionedContextValidNotExpired(t *testing.T) {
	pc := &ProvisionedContext{
		Guest:      &ProvisionedGuest{InstanceID: "test"},
		Consent:    &ConsentReceipt{},
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}
	if !pc.IsValid() {
		t.Fatal("IsValid should return true for valid non-expired context")
	}
}

func TestProvisionedContextErrorUnwrap(t *testing.T) {
	err := &ProvisionedContextError{Op: "test", Err: ErrProvisionedContextNil}
	if !errors.Is(err, ErrProvisionedContextNil) {
		t.Fatal("ProvisionedContextError does not unwrap")
	}
	if err.Error() != "provisioned_context test: provisioned context is nil" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
}