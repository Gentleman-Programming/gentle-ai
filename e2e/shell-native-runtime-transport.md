# Native installer process transport

Linux amd64 Go client → Alan's shared factory/runner. **Not production TUI routing or installed acceptance.**

## Caller responsibilities

- Authenticate the **whole immutable supplier**; checking the factory entry's physical identity does not authenticate its imports.
- Use the existing bounded worker and physical confirmation/post-terminal-restoration boundary. This client checks the kernel but creates neither isolation, delegation nor consent.
- Prepare an unpublished stage with `userPrepareInstallStage` and the qualified private Node. HOME/XDG/PNPM_HOME/npm configuration/agent/cwd stay private; caller environment is not inherited.
- Prepare bootstrap prerequisites through the existing native consumer seam. The client does not bootstrap pnpm or reconstruct installation actions.

## Transport contract

The shared `invokeWithEnvironment` transport's `collect` protocol returns the release plan; its `run` protocol recollects, compares that snapshot and calls the genuine runner. Qualification compares that original protocol with `RunWithLocation`, which preserves the native verdict while returning a separate installation locator. Changed selection and native blocked outcomes remain blocked. The full native result is retained: `ready`/exit zero does not prove publication. Existing process-group cancellation, bounded output, wait/reap and error taxonomy are reused; output overflow fails closed. The superseded Go `Collect`/`Run` wrappers are retired, not reintroduced as test-only APIs.

## Guest check and limits

`go test -tags=shell_native_runtime ./internal/shellinstaller -run '^TestUserNativeInstallerRuntime$' -count=1 -v` uses the authenticated supplier at `/alan-source`. It checks missing pnpm, denied consent, stale selection, escaping private paths, pre-start cancellation/deadline, named writable-tree/Node preservation and caller HOME/cwd. The 1ms deadline does **not** prove after-start cancellation/settlement. No dependency installation or publication is proven.

The default npm worker, manifest, CLI positional ABI and both launchers are unchanged by this transport cleanup. These retained scenarios alone do not qualify source admission, native bootstrap orchestration, progress, PNPM integrity/readbacks or ordinary TUI installed acceptance. They have not been re-executed for this cleanup candidate.
