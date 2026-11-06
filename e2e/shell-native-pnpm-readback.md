# Native PNPM target readback

`ReadNativePNPMInstallation` reads retained original-installer output without
executing suppliers, modifying DATA, publishing a manifest, or activating launchers.
The caller must retain exclusive custody of the immutable private output.

The readback checks the two generated global shims, resolves contained PNPM links,
compares filesystem-order resolution, checks package metadata, and records file
identities with streaming hashes. Persistent Node 24.21.0, primary Pi 1.0.0 and
Gentle Shell 4.0.0 are distinct from the installer-reported adjacent Pi peer.
The embedded native SDK must pass the existing pinned readback.

This is not dependency-closure authentication or installed acceptance. Publication,
relocation normalization and ordinary supervised launchers remain separate work.

The Linux integration test uses genuine retained DATA mounted read-only at `/out`:
`go test -tags=shell_pnpm_readback -run '^TestUserPNPMReadback$' ./internal/shellinstaller`.
Run only in the qualified private sandbox. It copies selected genuine targets into
an isolated fixture and checks metadata, shim, SDK and escaping-link rejection.
Rollback boundary: remove the readback API, its tagged integration test and this
note; legacy native installation and locator transport remain unchanged.
