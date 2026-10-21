//go:build linux

package shellinstaller

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// Provisioner is the guest-provisioning abstraction. Implementations must not
// mutate the operator's host: no writes to HOME, Pi installation, WSL, or
// system config. The guest is ephemeral by default; persistence requires
// explicit opt-in and is not the test seam's concern.
//
// This interface is implementation-agnostic (firecracker, incus, GitHub Actions
// ubuntu-latest VM, etc.). The test seam uses a synthetic in-memory stub that
// satisfies the interface without any real provisioning.
type Provisioner interface {
	// Provision creates a disposable guest environment and returns a
	// ProvisionedGuest with its attested InstanceID. The guest MUST be
	// isolated from the host (separate filesystem, network namespace, UID
	// mapping). The Provisioner MUST capture a pre-execution snapshot for
	// rollback.
	Provision(ctx context.Context, opts ProvisionOptions) (*ProvisionedGuest, error)

	// Deprovision destroys the guest and verifies rollback to the captured
	// snapshot. It is idempotent and safe to call multiple times.
	Deprovision(ctx context.Context, guest *ProvisionedGuest) error
}

// ProvisionOptions configures guest provisioning. All fields are optional
// for the synthetic test stub; real implementations may require specific
// values.
type ProvisionOptions struct {
	// Image is the guest base image identifier (e.g., "ubuntu:24.04").
	// Synthetic stub ignores this.
	Image string

	// NetworkPolicy controls guest egress. "pre-download" allows network
	// during provisioning only; "deny-all" blocks all egress; "allow"
	// permits unrestricted egress (not recommended for tests).
	NetworkPolicy string

	// CPU and Memory limits for the guest. Synthetic stub ignores these.
	CPU, Memory string
}

// ProvisionedGuest represents a provisioned guest environment with its
// attested identity. The InstanceID is a physical binding (e.g., cgroup
// path, VM UUID, container ID) that ties consent to this specific guest.
type ProvisionedGuest struct {
	InstanceID string
	// WorkDir is the guest's working directory for the E2E run.
	WorkDir string
	// SnapshotID identifies the pre-execution rollback snapshot.
	SnapshotID string
	// ProvisionerAttestation is an opaque blob from the provisioner
	// proving isolation (e.g., cgroup2 membership, VM measurements).
	// Synthetic stub returns a fixed test value.
	ProvisionerAttestation []byte
}

// SyntheticProvisioner is a test-only stub that satisfies Provisioner
// without any real provisioning. It returns deterministic synthetic
// values and tracks calls for test assertions. It MUST NOT be used
// outside of _test.go files.
type SyntheticProvisioner struct {
	ProvisionCalls  int
	DeprovisionCalls int
	LastGuest       *ProvisionedGuest
	ProvisionErr    error
	DeprovisionErr  error
	counter         int
}

// Provision returns a synthetic guest with a fixed InstanceID.
func (s *SyntheticProvisioner) Provision(ctx context.Context, opts ProvisionOptions) (*ProvisionedGuest, error) {
	s.ProvisionCalls++
	s.counter++
	if s.ProvisionErr != nil {
		return nil, s.ProvisionErr
	}
	ts := time.Now().Format("20060102T150405.000000000")
	guest := &ProvisionedGuest{
		InstanceID:             "synthetic-instance-" + ts + "-" + strconv.Itoa(s.counter),
		WorkDir:                "/tmp/gentle-shell-e2e-" + ts + "-" + strconv.Itoa(s.counter),
		SnapshotID:             "synthetic-snapshot-" + ts + "-" + strconv.Itoa(s.counter),
		ProvisionerAttestation: []byte("synthetic-attestation-v1"),
	}
	s.LastGuest = guest
	return guest, nil
}

// Deprovision records the call and returns success.
func (s *SyntheticProvisioner) Deprovision(ctx context.Context, guest *ProvisionedGuest) error {
	s.DeprovisionCalls++
	return s.DeprovisionErr
}

// ProvisionerError is a typed error for provisioner failures.
type ProvisionerError struct {
	Op  string
	Err error
}

func (e *ProvisionerError) Error() string {
	return "provisioner " + e.Op + ": " + e.Err.Error()
}
func (e *ProvisionerError) Unwrap() error { return e.Err }

var (
	ErrProvisionFailed    = errors.New("provision failed")
	ErrDeprovisionFailed  = errors.New("deprovision failed")
	ErrGuestNotProvisioned = errors.New("guest not provisioned")
)
