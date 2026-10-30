# Bind the native worker to the complete source snapshot

`userNativeRuntimeFromSnapshot` reuses retained archive custody and exact private
source readback. It seals a copy of the caller-authorized descriptor, refuses
complete-tree drift before bootstrap tooling is created, and rechecks before
`Prepare`, `RunWithLocation`, or the shared transport starts a process. No source-root CLI ABI, new bundle
format, installer engine, web frontend, dependency pin, or publication is added.

## Proof boundaries

- Default `TestUserNativeSourceBinding` uses inert DATA: descriptor/archive and
  import drift, missing/extra members, hardlinks, symlinks, cancellation, and
  refusal without tooling or preimage changes. It does not execute fixture code.
- Tagged `TestUserNativeInstallerBootstrap` uses the complete reviewed local
  Shell tree `73960fc`, ZIP SHA-256
  `feed2eb9961c6ab3b3a3d55ea314bfca881a5d901c44f97493231d72e0326453`,
  with authentic retained PNPM bytes inside the bounded offline Linux Guest.
- Caller authorization and immutable source custody remain mandatory. Descriptor
  construction and archive equality are not publisher authentication or a
  read-only mount. Unmodified Main `43541` cannot authenticate changed helpers.

Run the default source-binding test and existing tagged native runtime/bootstrap
checks, then default worker regressions and cross-builds in the authorized Guest.
A failure or SKIP is not installed acceptance. The ordinary TUI route, published
source carrier, after-start cancellation, installation, publication, and both
launchers remain separate work. Rollback removes this binding and its guards;
existing low-level transport, original bootstrap, and npm worker remain intact.
