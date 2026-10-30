# Read back an original PNPM runner failure

`TestUserNativeInstallerOfflineEffects` exercises the original factory and
bootstrap through `Prepare` → sealed plan/environment → consented `RunWithLocation`.
It does not publish a destination, replace the npm backend, or install a fixture
product. No installer, probe, process, or persistence-pin adapter is substituted.

## Bounded offline check

Run only in the existing Linux amd64 Guest boundary: UID1000, cleared groups,
zero capabilities and bounding set, NNP, fresh private HOME/environment/network,
read-only source/tools/modules, memory3GiB, swap0, CPU1, tasks64, tmpfs2GiB,
RuntimeMaxSec300s, TimeoutStopSec2s, UMask0077. The held complete local73960fc
source ZIP, authentic Node24.18 and authentic PNPM11.1.1 archive are prerequisites.
Only archive-byte transport is supplied to original bootstrap; no new acquisition.

```sh
go test -p 1 -tags=shell_native_runtime ./internal/shellinstaller \
  -run '^TestUserNativeInstallerOfflineEffects$' -count=1 -v
```

The original multi-action plan may have `ready:false`; the runner accepts its
supported action sequence independently. The check requires its actual global-bin
and empty-stack gates, then `failed/persist-node` without a completed persistence
or global-install action. A timeout, refusal, missing observer, or uncertain
settlement is a failed/inconclusive check, never this negative behavior's PASS.

Independent post-return readbacks compare the full source, reauthenticate Node,
and inventory bounded private scopes. Changed scopes and up to32 path entries per changed scope are diagnostics;
the scope stamps cover all entries. A private project marker and bootstrap tools
must remain unchanged. Cleanup follows verified Node exit, empty owned process
group, readbacks and `!t.Failed()`; failed workspaces are retained.

## Scope and next step

Observed changes are not presumed absent, atomic rollback, or completed PNPM
installation. This checks one owned private fixture, not a caller's real project,
detached descendants, a production user manager, publisher authentication,
production immutable custody, physical TUI consent, publication, either launcher,
or cancellation after PNPM mutation. Those remain separate E3 prerequisites.
Rollback removes this test and document only; original code and pins are untouched.
