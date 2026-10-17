//go:build linux

package shellinstaller

import (
	"context"
	"errors"
	"testing"
)

func TestRunVerificationValidModeChannel(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"pi", "gentle-shell"} {
		for _, channel := range []string{"Stable", "Main"} {
			t.Run(mode+"/"+channel, func(t *testing.T) {
				witness, err := RunVerification(ctx, VerificationOptions{
					Mode:           mode,
					Channel:        channel,
					SourceDescriptor: "test-source",
					InstalledRoot:  "/tmp/test-root",
				})
				if err != nil {
					t.Fatalf("RunVerification failed: %v", err)
				}
				if witness == nil {
					t.Fatal("witness is nil")
				}
				if witness.Mode != mode {
					t.Fatalf("Mode mismatch: %q != %q", witness.Mode, mode)
				}
				if witness.Channel != channel {
					t.Fatalf("Channel mismatch: %q != %q", witness.Channel, channel)
				}
			})
		}
	}
}

func TestRunVerificationInvalidMode(t *testing.T) {
	ctx := context.Background()
	_, err := RunVerification(ctx, VerificationOptions{
		Mode:           "invalid",
		Channel:        "Stable",
		SourceDescriptor: "test",
		InstalledRoot:  "/tmp/test",
	})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if err.Error() != "invalid mode \"invalid\": must be 'pi' or 'gentle-shell'" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunVerificationInvalidChannel(t *testing.T) {
	ctx := context.Background()
	_, err := RunVerification(ctx, VerificationOptions{
		Mode:           "pi",
		Channel:        "Invalid",
		SourceDescriptor: "test",
		InstalledRoot:  "/tmp/test",
	})
	if err == nil {
		t.Fatal("expected error for invalid channel")
	}
	if err.Error() != "invalid channel \"Invalid\": must be 'Stable' or 'Main'" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunVerificationMissingInstalledRoot(t *testing.T) {
	ctx := context.Background()
	_, err := RunVerification(ctx, VerificationOptions{
		Mode:           "pi",
		Channel:        "Stable",
		SourceDescriptor: "test",
		InstalledRoot:  "",
	})
	if err == nil {
		t.Fatal("expected error for missing InstalledRoot")
	}
	if err.Error() != "InstalledRoot required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunVerificationBinaryHashMismatch(t *testing.T) {
	ctx := context.Background()
	witness, err := RunVerification(ctx, VerificationOptions{
		Mode:                  "pi",
		Channel:               "Stable",
		SourceDescriptor:      "test",
		InstalledRoot:         "/tmp/test-root",
		ExpectedBinarySHA256:  "expected-hash",
		ExpectedBinarySize:    100,
	})
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}
	var vErr *VerificationError
	if !errors.As(err, &vErr) || vErr.Op != "binary_sha256_mismatch" {
		t.Fatalf("wrong error type: %v", err)
	}
	if witness == nil {
		t.Fatal("witness should be returned even on mismatch")
	}
	if witness.BinarySHA256 != "missing" {
		t.Fatalf("unexpected BinarySHA256: %q", witness.BinarySHA256)
	}
}

func TestRunVerificationBinarySizeMismatch(t *testing.T) {
	ctx := context.Background()
	witness, err := RunVerification(ctx, VerificationOptions{
		Mode:               "pi",
		Channel:            "Stable",
		SourceDescriptor:   "test",
		InstalledRoot:      "/tmp/test-root",
		ExpectedBinarySize: 999,
	})
	if err == nil {
		t.Fatal("expected error for size mismatch")
	}
	var vErr *VerificationError
	if !errors.As(err, &vErr) || vErr.Op != "binary_size_mismatch" {
		t.Fatalf("wrong error type: %v", err)
	}
	if witness == nil {
		t.Fatal("witness should be returned even on mismatch")
	}
}

func TestRunVerificationWithCycle(t *testing.T) {
	ctx := context.Background()
	witness, err := RunVerification(ctx, VerificationOptions{
		Mode:           "gentle-shell",
		Channel:        "Main",
		SourceDescriptor: "main@abc123",
		InstalledRoot:  "/tmp/test-root",
		RunCycle:       true,
	})
	if err != nil {
		t.Fatalf("RunVerification failed: %v", err)
	}
	if witness == nil {
		t.Fatal("witness is nil")
	}
	if witness.CycleProof == nil {
		t.Fatal("CycleProof is nil when RunCycle=true")
	}
	if err := VerifyCycleProof(witness.CycleProof); err != nil {
		t.Fatalf("CycleProof invalid: %v", err)
	}
}

func TestVerifyCycleProofValid(t *testing.T) {
	proof := &CycleProof{
		InstallPhase:  &PhaseEvidence{Phase: "install", BinarySHA256: "hash1", BinarySize: 100, Success: true},
		UpdatePhase:   &PhaseEvidence{Phase: "update", BinarySHA256: "hash2", BinarySize: 100, Success: true},
		RollbackPhase: &PhaseEvidence{Phase: "rollback", BinarySHA256: "hash1", BinarySize: 100, Success: true},
		ReadyPhase:    &PhaseEvidence{Phase: "ready", BinarySHA256: "hash1", BinarySize: 100, Success: true},
	}
	if err := VerifyCycleProof(proof); err != nil {
		t.Fatalf("VerifyCycleProof failed: %v", err)
	}
}

func TestVerifyCycleProofNil(t *testing.T) {
	err := VerifyCycleProof(nil)
	if err == nil {
		t.Fatal("expected error for nil proof")
	}
	if err.Error() != "cycle proof is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyCycleProofMissingPhase(t *testing.T) {
	proof := &CycleProof{
		InstallPhase:  &PhaseEvidence{Phase: "install", Success: true},
		UpdatePhase:   &PhaseEvidence{Phase: "update", Success: true},
		RollbackPhase: &PhaseEvidence{Phase: "rollback", Success: true},
		ReadyPhase:    nil, // Missing
	}
	err := VerifyCycleProof(proof)
	if err == nil {
		t.Fatal("expected error for missing phase")
	}
	if err.Error() != "missing ready phase" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyCycleProofWrongPhaseOrder(t *testing.T) {
	proof := &CycleProof{
		InstallPhase:  &PhaseEvidence{Phase: "install", Success: true},
		UpdatePhase:   &PhaseEvidence{Phase: "rollback", Success: true}, // Wrong order
		RollbackPhase: &PhaseEvidence{Phase: "update", Success: true},
		ReadyPhase:    &PhaseEvidence{Phase: "ready", Success: true},
	}
	err := VerifyCycleProof(proof)
	if err == nil {
		t.Fatal("expected error for wrong phase order")
	}
}

func TestVerifyCycleProofPhaseFailed(t *testing.T) {
	proof := &CycleProof{
		InstallPhase:  &PhaseEvidence{Phase: "install", Success: true},
		UpdatePhase:   &PhaseEvidence{Phase: "update", Success: false, ErrorMessage: "update failed"},
		RollbackPhase: &PhaseEvidence{Phase: "rollback", Success: true},
		ReadyPhase:    &PhaseEvidence{Phase: "ready", Success: true},
	}
	err := VerifyCycleProof(proof)
	if err == nil {
		t.Fatal("expected error for failed phase")
	}
	if err.Error() != "update phase failed: update failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyCycleProofRollbackMismatch(t *testing.T) {
	proof := &CycleProof{
		InstallPhase:  &PhaseEvidence{Phase: "install", BinarySHA256: "hash1", BinarySize: 100, Success: true},
		UpdatePhase:   &PhaseEvidence{Phase: "update", BinarySHA256: "hash2", BinarySize: 100, Success: true},
		RollbackPhase: &PhaseEvidence{Phase: "rollback", BinarySHA256: "hash3", BinarySize: 100, Success: true}, // Different!
		ReadyPhase:    &PhaseEvidence{Phase: "ready", BinarySHA256: "hash1", BinarySize: 100, Success: true},
	}
	err := VerifyCycleProof(proof)
	if err == nil {
		t.Fatal("expected error for rollback mismatch")
	}
	if err.Error() != "rollback binary does not match install binary" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyCycleProofReadyMismatch(t *testing.T) {
	proof := &CycleProof{
		InstallPhase:  &PhaseEvidence{Phase: "install", BinarySHA256: "hash1", BinarySize: 100, Success: true},
		UpdatePhase:   &PhaseEvidence{Phase: "update", BinarySHA256: "hash2", BinarySize: 100, Success: true},
		RollbackPhase: &PhaseEvidence{Phase: "rollback", BinarySHA256: "hash1", BinarySize: 100, Success: true},
		ReadyPhase:    &PhaseEvidence{Phase: "ready", BinarySHA256: "hash3", BinarySize: 100, Success: true}, // Different!
	}
	err := VerifyCycleProof(proof)
	if err == nil {
		t.Fatal("expected error for ready mismatch")
	}
	if err.Error() != "ready binary does not match install binary" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerificationErrorUnwrap(t *testing.T) {
	err := &VerificationError{Op: "test", Err: ErrBinaryHashMismatch}
	if !errors.Is(err, ErrBinaryHashMismatch) {
		t.Fatal("VerificationError does not unwrap")
	}
	if err.Error() != "verification test: binary hash mismatch" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
}

func TestSyntheticFileSHA256Deterministic(t *testing.T) {
	h1 := syntheticFileSHA256("/same/path")
	h2 := syntheticFileSHA256("/same/path")
	if h1 != h2 {
		t.Fatal("syntheticFileSHA256 not deterministic")
	}
	h3 := syntheticFileSHA256("/different/path")
	if h1 == h3 {
		t.Fatal("syntheticFileSHA256 should differ for different paths")
	}
}

func TestRunSyntheticCycleProducesValidProof(t *testing.T) {
	ctx := context.Background()
	proof, err := runSyntheticCycle(ctx, VerificationOptions{
		Mode:          "pi",
		InstalledRoot: "/tmp/test",
	})
	if err != nil {
		t.Fatalf("runSyntheticCycle failed: %v", err)
	}
	if err := VerifyCycleProof(proof); err != nil {
		t.Fatalf("generated proof invalid: %v", err)
	}
}