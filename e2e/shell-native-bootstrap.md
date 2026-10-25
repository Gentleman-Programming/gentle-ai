# Linux native bootstrap handoff

`NativeInstallerRuntime.Prepare(ctx, consent)` requires the existing physical
confirmation before invoking original bootstrap. It captures the entire callback
environment and a sealed release plan. Private Node/PNPM are marked bootstrap-only
using the original probe policy; they are not installed runtime evidence.

Caller-owned `runtime/native-bootstrap` survives collection. Prepared `RunWithLocation`
requires consent, reuses those tools/environment, and compares a fresh plan
before the original runner. The caller owns whole-source authentication,
isolation, cancellation/settlement and cleanup. No browser or second TUI is used.
Legacy unprepared `collect`/`run` wire protocols retain their direct transport
controls; their superseded Go wrappers are retired. The stock npm worker is unchanged.

In the bounded offline Linux amd64 Guest, provide the authenticated Shell source
at `/alan-source` and authentic pinned PNPM DATA at `/pnpm-11.1.1.tgz`, then run:

```sh
go test -p 1 -tags=shell_native_runtime ./internal/shellinstaller -run '^TestUserNativeInstaller(Runtime|Bootstrap)$' -count=1 -v
```

Readbacks cover prerequisite consent refusal, pre-start cancellation, retained
tools, captured environment and stale-plan blocking. They do not prove after-start
cancellation, a delegated manager, installation, publication or both launchers.
