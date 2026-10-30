# Ordinary TUI process-handoff regression

These inert fixtures preserve the independently executed ordinary-TUI proof.
They are not an installer or a production hook. Never copy the probe into a
working checkout or a distributable binary.

## Qualified execution

1. Create a fresh scratch copy outside the repository. Copy `owner-probe.go.txt`
   into its `internal/shellinstaller` package as `owner_probe_linux.go`.
2. In that scratch copy only, replace the last call in `userNativeOwnerEntry`:
   `return userNativeInstall(ctx, self, args[1:], req, stdin, stdout, stderr)`
   with `return independentTUIOwnerProbe(ctx, self, args, req, stdin, stdout, stderr)`.
   Keep deadline, physical-consent and fresh-selection checks unchanged.
3. Build the ordinary `cmd/gentle-ai` binary. Mount it at `/probe/gentle-ai`,
   the driver as the qualified fixture's entry, and an `actual` text file at
   `/probe/tui-mode`. Use a fresh private host-backed `/fixture/install-parent`
   and `/fixture/home`; do not reuse any real installation destination.
4. Run `python3 owner-pty.py parent-green` inside the existing qualified Linux
   namespace/private-user-manager boundary: UID/GID1000, no supplementary
   groups/capabilities, NNP1, CPU1, RAM3GiB, swap0, Tasks64, tmpfs2GiB,
   service300s/outer140s. The driver validates these controls before execution.
5. Retain `/fixture/tui-result.json`, `owner-proof.json`, `terminal-raw.json`
   and bounded `tui-output.raw`. Verify source/sentinel preimages and physical
   service/manager/cgroup settlement separately using the owning harness.

The driver navigates the real no-argument app, edits Ready to neutral/off,
confirms a new selection, and verifies same-PID replacement, standard descriptors,
raw terminal state, absolute deadline, physical consent, input and signal/exit
behavior. It stops terminal-query emulation when the TUI releases ownership.
The probe deliberately returns a sentinel error instead of running installation.

The observed result was one alternate-screen entry, exact inherited input,
SIGTERM delivery, CLI error exit1, restored terminal, and no final destination.
This proves the handoff only: original installation, publication and final
launchers require separate unchanged-binary acceptance.
