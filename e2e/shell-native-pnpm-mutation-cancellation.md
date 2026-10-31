# Original PNPM directory effects before cancellation

This qualification uses the original local reviewed installer, not a replacement
runner or a published installation. Run only inside the established bounded,
offline Linux amd64 Guest with authentic held Node/PNPM and complete source:

```sh
go test -p 1 -tags=shell_native_runtime ./internal/shellinstaller \
  -run '^TestUserNativeInstallerPNPMMutationCancellation$' -count=1 -v
```

## Required sequence

- Original bootstrap and genuine Prepare seal the complete environment and plan.
- Run with consent unchanged. Observe the exact owned native Node invocation.
- Observe the original `pnpm runtime set node 24.21.0 -g` child by exact argv,
  executable, cwd, PPid, PGID; acquire pidfd, reread association and reject exit.
- Wait for physical PNPM directory effects while both invocations remain live.
  Hold that child with its owned pidfd; confirm stopped state and live parent.
- Independently stamp the private PNPM tree against its pre-Run baseline and
  require physical global/store directory witnesses while both invocations live.
- Only then cancel the parent context. Require canceled category and original
  context cause, both pidfd exits and empty normal invocation group. Allow
  bounded read-only exit/reaping observation; zombie-only groups still fail.
- After settlement, independently compare full source, authenticate copied Node,
  compare bootstrap tooling and private project, and read PNPM effects.
- Cleanup only after these readbacks and no test failure. Failed/inconclusive
  workspaces remain evidence; no extra cleanup signal or kill/reap policy.

PNPM pre-gates may create some directories: this proves cancellation after real
PNPM directory effects with a genuine persistence child alive, not attribution
of every write to that action. It does not prove a partial package-file install,
successful persistent runtime, rollback, detached descendants, real caller
project preservation, production source authority/immutable custody, actual
user manager, physical TUI consent, publication, both launchers or three-OS
installed acceptance. Observer absence, timeout, refusal or uncertain settlement
cannot be negative-behavior PASS. The pins and process primitives are unchanged.
