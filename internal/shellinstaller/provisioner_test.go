//go:build linux

package shellinstaller

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSyntheticProvisionerProvisionReturnsDeterministicGuest(t *testing.T) {
	p := &SyntheticProvisioner{}
	ctx := context.Background()

	guest, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if guest == nil {
		t.Fatal("guest is nil")
	}
	if guest.InstanceID == "" {
		t.Fatal("InstanceID is empty")
	}
	if guest.WorkDir == "" {
		t.Fatal("WorkDir is empty")
	}
	if guest.SnapshotID == "" {
		t.Fatal("SnapshotID is empty")
	}
	if len(guest.ProvisionerAttestation) == 0 {
		t.Fatal("ProvisionerAttestation is empty")
	}
	if p.ProvisionCalls != 1 {
		t.Fatalf("ProvisionCalls = %d, want 1", p.ProvisionCalls)
	}
	if p.LastGuest != guest {
		t.Fatal("LastGuest not recorded")
	}
}

func TestSyntheticProvisionerDeprovisionRecordsCall(t *testing.T) {
	p := &SyntheticProvisioner{}
	ctx := context.Background()

	guest, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if err := p.Deprovision(ctx, guest); err != nil {
		t.Fatalf("Deprovision failed: %v", err)
	}
	if p.DeprovisionCalls != 1 {
		t.Fatalf("DeprovisionCalls = %d, want 1", p.DeprovisionCalls)
	}
}

func TestSyntheticProvisionerIdempotentDeprovision(t *testing.T) {
	p := &SyntheticProvisioner{}
	ctx := context.Background()

	guest, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := p.Deprovision(ctx, guest); err != nil {
			t.Fatalf("Deprovision %d failed: %v", i, err)
		}
	}
	if p.DeprovisionCalls != 3 {
		t.Fatalf("DeprovisionCalls = %d, want 3", p.DeprovisionCalls)
	}
}

func TestSyntheticProvisionerProvisionError(t *testing.T) {
	p := &SyntheticProvisioner{ProvisionErr: ErrProvisionFailed}
	ctx := context.Background()

	_, err := p.Provision(ctx, ProvisionOptions{})
	if err == nil {
		t.Fatal("expected provision error")
	}
	if !errors.Is(err, ErrProvisionFailed) {
		t.Fatalf("wrong error: %v", err)
	}
	if p.ProvisionCalls != 1 {
		t.Fatalf("ProvisionCalls not incremented on error")
	}
}

func TestSyntheticProvisionerDeprovisionError(t *testing.T) {
	p := &SyntheticProvisioner{DeprovisionErr: ErrDeprovisionFailed}
	ctx := context.Background()

	guest, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	err = p.Deprovision(ctx, guest)
	if err == nil {
		t.Fatal("expected deprovision error")
	}
	if !errors.Is(err, ErrDeprovisionFailed) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestProvisionOptionsFieldsIgnoredBySyntheticStub(t *testing.T) {
	p := &SyntheticProvisioner{}
	ctx := context.Background()

	opts := ProvisionOptions{
		Image:         "ubuntu:24.04",
		NetworkPolicy: "pre-download",
		CPU:           "2",
		Memory:        "4G",
	}
	guest, err := p.Provision(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if guest == nil {
		t.Fatal("guest is nil")
	}
	// Synthetic stub ignores all options; just verify it doesn't crash.
}

func TestProvisionedGuestInstanceIDFormat(t *testing.T) {
	p := &SyntheticProvisioner{}
	ctx := context.Background()

	guest, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// InstanceID should have synthetic prefix and timestamp-like suffix
	if len(guest.InstanceID) < len("synthetic-instance-")+15 {
		t.Fatalf("InstanceID too short: %q", guest.InstanceID)
	}
}

func TestProvisionedGuestAttestationNonEmpty(t *testing.T) {
	p := &SyntheticProvisioner{}
	ctx := context.Background()

	guest, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(guest.ProvisionerAttestation) != "synthetic-attestation-v1" {
		t.Fatalf("unexpected attestation: %q", guest.ProvisionerAttestation)
	}
}

func TestProvisionerErrorUnwrap(t *testing.T) {
	err := &ProvisionerError{Op: "Provision", Err: ErrProvisionFailed}
	if !errors.Is(err, ErrProvisionFailed) {
		t.Fatal("ProvisionerError does not unwrap")
	}
	if err.Error() != "provisioner Provision: provision failed" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
}

func TestSyntheticProvisionerMultipleGuestsIndependent(t *testing.T) {
	p := &SyntheticProvisioner{}
	ctx := context.Background()

	guest1, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)
	guest2, err := p.Provision(ctx, ProvisionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if guest1.InstanceID == guest2.InstanceID {
		t.Fatal("InstanceIDs should differ across provisions")
	}
	if guest1.WorkDir == guest2.WorkDir {
		t.Fatal("WorkDirs should differ across provisions")
	}
	if guest1.SnapshotID == guest2.SnapshotID {
		t.Fatal("SnapshotIDs should differ across provisions")
	}
	if p.ProvisionCalls != 2 {
		t.Fatalf("ProvisionCalls = %d, want 2", p.ProvisionCalls)
	}
	if p.LastGuest != guest2 {
		t.Fatal("LastGuest should be the most recent guest")
	}
}