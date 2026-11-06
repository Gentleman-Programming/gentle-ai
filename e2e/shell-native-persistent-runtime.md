# Original persistent runtime before publication

Run only in the established offline Linux amd64 Guest with authentic source,
bootstrap tools and the retained fixed persistence inputs:

```sh
go test -p 1 -tags=shell_native_runtime ./internal/shellinstaller \
  -run '^TestUserNativeInstallerPersistentRuntime$' -count=1 -v
```

Prepare seals system-CA trust as in the existing worker; no custom CA override
or disabled certificate verification. The original plan/actions remain unchanged.
Require completed persist-node,
managers, runtime/npm checks and npm-prefix configuration. A later genuine
install-global failure is intentional scope separation, not installed success.
Independently resolve Node inside PNPM_HOME, authenticate its held binary bytes,
read all three versions through the actual global bin and read private npm prefix.
Source, bootstrap tools and private project are compared after process settlement.
The private worker seals PNPM_CONFIG_STORE_DIR to its owned pnpm/store, refusing
store/v11 path aliases; personal configuration is not imported or rewritten.
This avoids pnpm's cwd-local store-discovery probe without changing the full plan.
Bounded nonrecursive inotify observations supplement, never replace, the complete
project stamp; a second stamp also checks preservation after version/prefix probes.
The closed replay uses original response bytes and the qualified legacy import
guard. Unknown requests are refused before upstream; TLS failures are diagnostic.
Normal owner-pipe shutdown must settle the mirror before success-gated cleanup.
Unknown fixture URLs return permanent404, not a synthetic transient502. The
first full-plan pilot exhausted its140s deadline after final package requests;
that failure is retained and is not persistence acceptance. No limits are raised.

This does not qualify full PNPM linking/integrity/manifest publication, source
publisher/production immutable custody, actual user manager, ordinary physical
TUI consent, installation, both launchers or three-OS acceptance. No plan rewrite,
process/probe/digest replacement, bootstrap-as-persistent evidence or relaxed
32MiB tree-stamp bound. All runtime resources and process primitives remain fixed.
