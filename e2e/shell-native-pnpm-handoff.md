# Native installer location handoff

`NativeInstallerRuntime.RunWithLocation` requests the original fresh-release
installer's `onInstallationVerified` callback. The original `run` wire protocol
remains an independent transport control, without a duplicate Go `Run` wrapper.
Both require the established private worker, immutable source,
terminal restoration and physical consent; neither creates a sandbox.

The opt-in response preserves the complete original result separately from a
location containing the package root, PNPM home, global bin, Shell version and
adjacent Pi version. A blocked run carries no location. A successful fresh run
without the callback is refused, including an older factory that ignores it.
Success uses the original `action: "open-new-terminal"` field; blocked outcomes
retain their `reason` field. The adjacent Pi version is not forced to equal the
primary global Pi version.

The decoder validates the private PNPM namespace, not filesystem authenticity.
Before publication, the caller still must authenticate the physical owner/layout,
preserve the store and dependency topology, normalize only generated references,
write final bindings, rename without replacement, sync and read back the result.
This API does not change the existing npm manifest or enable a PNPM backend.

## Checks

- `go test ./internal/shellinstaller -run 'TestNativeInstallationHandoff|TestRunWithLocationRejectsUnownedPlanBeforeExecution'`
- In the qualified Linux sandbox, `go test -tags=shell_native_runtime ./internal/shellinstaller -run TestUserNativeInstallerRuntime`
- The original installer callback's positive execution is qualified separately;
  decoder fixtures and blocked transport tests are not installed acceptance.

Rollback removes the location handoff and its tests without changing the original `run` wire protocol. The wrapper cleanup preserves these qualification scenarios; it has not rerun their Guest proofs.
