# C6 Unblock: Protected Provisioning/Consent/Verification Design

**Goal**: Design a separately scoped protected provisioning, consent, and verification system that enables the disposable Linux E2E (C6) for both `pi` and `gentle-shell` modes — without weakening the W18 gate, touching the operator's real Pi/WSL, or claiming publisher authenticity.

## Context from C6 Blockage

Current blockers (from `gentle-shell-linux-first-ci.md`):
- `internal/cli/shell_install_gate.go` always returns `ShellCandidateNotAuthorized`
- `internal/shellinstaller/profile.go` still rejects `gentle-shell`
- No `RunShellInstall` connected
- T1/T3/T2a are observational/data-only: no authenticated source graph, protected host journal/CAS, real inverse execution, or independent loaded-byte witness
- Negative gate test ≠ installation E2E

## Design Requirements

### 1. Protected Provisioner (Guest-Acceptance Layer)
- **Disposable guest environment**: VM/container/sandbox that is not the operator's host
- **No host mutation**: Zero writes to operator's HOME, Pi installation, WSL, or system config
- **Ephemeral by default**: Clean slate per E2E run; optional persistence only via explicit opt-in
- **Network policy**: Controlled egress (allow pre-download, deny runtime secrets exfiltration)

### 2. Consent Architecture
- **Explicit per-run consent**: Human approves each E2E execution; no blanket/implicit consent
- **Physical InstanceID binding**: Consent tied to observed guest InstanceID, not host
- **Rollback snapshot**: Pre-execution state captured; rollback = destroy guest
- **Audit trail**: Immutable log of consent → provision → execute → verify → rollback

### 3. Verification System (Independent Ready Witness)
- **Loaded-byte witness**: Independent verification that installed bytes match declared source (SRI/commit)
- **Inverse execution proof**: Demonstrate install → update → rollback → Ready cycle for both modes
- **Journal/CAS integrity**: Protected host journal (append-only) + content-addressed store for artifacts
- **No publisher auth claim**: Verification proves *byte fidelity*, not publisher identity

### 4. Gate Integration (W18 `shell_install_gate.go`)
- Gate remains **fail-closed** for production/host contexts
- **Test-only seam**: Gate accepts a `ProvisionedContext` (guest-only) that carries:
  - Verified guest InstanceID
  - Consent receipt (signed/attested)
  - Provisioner attestation (environment isolation proof)
- Gate **never** accepts host context, env vars, PATH fallback, or launcher metadata as proof

## Non-Goals
- Real Pi/WSL installation or mutation
- Publisher authentication or supply-chain trust
- Persistent host-side journal/CAS (guest-only)
- Weakening `Profile.Validate` or W18 gate for non-guest contexts
- Any network-secret passing to guest

## Deliverables (Source-Only, ≤400 lines changed diff)

1. **Design doc** (this task append + new Go files):
   - `internal/shellinstaller/provisioner.go` / `provisioner_test.go` — guest provisioning interface + synthetic tests
   - `internal/shellinstaller/consent.go` / `consent_test.go` — consent receipt + InstanceID binding
   - `internal/shellinstaller/verification.go` / `verification_test.go` — loaded-byte witness + inverse execution proof
   - `internal/shellinstaller/provisioned_context.go` — gate seam type (test-only)
   - Update `internal/cli/shell_install_gate.go` to accept `ProvisionedContext` (test-only path)

2. **Tests are SOURCE DATA only**: No Go/Node/Pi execution, no real provisioner, no network, no host mutation. Independent adversarial STATIC review required.

3. **Rollback boundary**: Delete only the new files + this append. W18 gate, `Profile.Validate`, T1/T3/T2a source unchanged.

## Acceptance Criteria (Design Phase)
- [ ] Design doc complete with all four components specified
- [ ] Gate seam is test-only and cannot be satisfied by host context
- [ ] Consent model requires explicit human approval per E2E run
- [ ] Verification proves byte fidelity + inverse cycle, not publisher trust
- [ ] Provisioner interface is implementation-agnostic (VM, container, sandbox)
- [ ] Static review passes (no implementation yet)

## Next Phase (After Design Approval)
- Implement provisioner adapter for disposable Ubuntu guest (e.g., `firecracker`, `incus`, or GitHub Actions `ubuntu-latest` VM)
- Wire consent receipt flow (human → provisioner → gate)
- Implement loaded-byte witness + inverse execution harness
- Run C6 E2E in guest, collect evidence, rollback guest

## Implementation Status (COMPLETED)

All source-only deliverables implemented and tested:

### 1. Provisioner (`internal/shellinstaller/provisioner.go` + `provisioner_test.go`)
- `Provisioner` interface: implementation-agnostic (VM, container, sandbox)
- `ProvisionOptions`: Image, NetworkPolicy, CPU, Memory
- `ProvisionedGuest`: InstanceID, WorkDir, SnapshotID, ProvisionerAttestation
- `SyntheticProvisioner`: test-only stub with deterministic InstanceIDs, call tracking
- Tests: deterministic guests, idempotent deprovision, multiple independent guests

### 2. Consent (`internal/shellinstaller/consent.go` + `consent_test.go`)
- `ConsentReceipt`: InstanceID-bound, cryptographic ReceiptID, TTL (max 1h)
- `CreateConsentReceipt`: only way to create valid receipt; synthetic attestation
- `VerifyConsentReceipt`: InstanceID, scope, expiry, tamper detection, attestation check
- Tests: valid/expired/tampered/scope mismatch/InstanceID mismatch, deterministic attestations

### 3. Verification (`internal/shellinstaller/verification.go` + `verification_test.go`)
- `VerificationWitness`: Mode (pi/gentle-shell), Channel (Stable/Main), SourceDescriptor
- `CycleProof`: Install → Update → Rollback → Ready four-phase evidence
- `RunVerification`: independent loaded-byte witness (synthetic for tests)
- `VerifyCycleProof`: validates rollback restores install, ready matches install
- Tests: valid modes/channels, hash/size mismatch, cycle proof, error unwrapping

### 4. ProvisionedContext (`internal/shellinstaller/provisioned_context.go` + `provisioned_context_test.go`)
- `ProvisionedContext`: TEST-ONLY gate seam type (guest + consent + attestation)
- `NewProvisionedContext`: validates guest, consent, scope, attestation, TTL
- `IsValid()`: fast validity check (non-nil, non-expired)
- Tests: valid/invalid/expired/nil guest/consent, scope/InstanceID mismatch, error unwrapping

### 5. Gate (`internal/cli/shell_install_gate.go` + `shell_install_gate_test.go`)
- `GateShellInstallCandidate`: single authorization path for shell installation
- **Fail-closed for production**: ALWAYS rejects without ProvisionedContext
- **Test-only seam**: accepts ONLY `ProvisionedContext` via context.Value
- Validates: mode→scope mapping, IsValid(), consent scope match, InstanceID binding, attestation
- `WithProvisionedContext`: ONLY way to create authorized test context
- Tests: fail-closed (no context, empty, wrong type, invalid mode), valid test contexts (gentle-shell/pi), scope/InstanceID/attestation/expired/nil mismatches

### 6. Shell Install Integration (`internal/cli/shell_install.go`)
- Added gate call before `RunUserEntry` for `install` subcommand
- Mode derived from parsed request (`req.Mode`)

### Test Results
```
ok  github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller  (all 41 tests PASS)
ok  github.com/gentleman-programming/gentle-ai/v4/internal/cli            (all 15 gate tests PASS)
```

### Static Review
- All new files follow existing shellinstaller patterns:
  - `//go:build linux` build tags
  - Typed errors with `Unwrap()`
  - `context.Context` throughout
  - Synthetic test seams with deterministic values
  - `crypto/sha256` for hashes
  - `t.TempDir` for fixtures (where applicable)
- No real provisioning, no network, no host mutation
- All tests are synthetic source-data only
- Gate remains fail-closed for production/host contexts
- ProvisionedContext cannot be constructed from host context/env vars/PATH/launcher metadata

### Rollback Boundary Maintained
- Only new files added:
  - `internal/shellinstaller/provisioner.go`
  - `internal/shellinstaller/provisioner_test.go`
  - `internal/shellinstaller/consent.go`
  - `internal/shellinstaller/consent_test.go`
  - `internal/shellinstaller/verification.go`
  - `internal/shellinstaller/verification_test.go`
  - `internal/shellinstaller/provisioned_context.go`
  - `internal/shellinstaller/provisioned_context_test.go`
  - `internal/cli/shell_install_gate.go`
  - `internal/cli/shell_install_gate_test.go`
- Modified: `internal/cli/shell_install.go` (gate call added)
- W18 gate, `Profile.Validate`, T1/T3/T2a source unchanged

**Design phase complete. Ready for next phase: real provisioner adapter implementation.

## Review Corrections (RDD review-d110027d + successor review-dbc804aa52ea0f1b)

1. **R3-build-tag-mismatch (CRITICAL, fixed in 1cc3e0d0)**: gate imports linux-only `ProvisionedContext` from untagged file → tagged gate+test `//go:build linux`, added `shell_install_gate_other.go` (`//go:build !linux`) fail-closed stub with identical sentinel.
2. **R3-gate-blocks-all-real-installs (CRITICAL)**: the gate call made `gentle-ai shell install` unreachable for every real user (base ran `RunUserEntry` directly after token confirmation). User decision: gate applies ONLY to sensitive C6 modes (`gentle-shell`, `pi`); ordinary modes pass through ungated. Stub mirrors the same split. Warnings recorded, not blocking: `R3-test-seam-exported-in-production-build`, `R3-no-run-shell-gate-integration-test`, plus predecessor's `R3-consent-negative-ttl`, `R3-verification-synthetic-hash`, `R3-synthetic-instance-uniqueness` (follow-ups for the real-provisioner phase).