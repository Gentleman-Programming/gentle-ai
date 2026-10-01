# GGA PowerShell Shim — Windows Support

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v3.7.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v3.7.0/docs).

## What This Is

When `gentle-ai` installs GGA on Windows, it now installs a `gga.ps1` wrapper
alongside the main bash script. This allows users to run `gga` directly from
PowerShell without manually switching to Git Bash.

## How It Works

```
User types: gga init   (in PowerShell)
                │
                ▼
     Windows resolves gga.ps1
     (PowerShell understands .ps1 extensions)
                │
                ▼
     gga.ps1 finds Git Bash via Get-Command git
                │
                ▼
     Git Bash executes the original gga bash script
                │
                ▼
     Exit code + output returned to PowerShell
```

The shim is installed to the same directory as the `gga` binary
(`~/.local/share/gga/bin/gga.ps1`) and uses an atomic write with content-equality
check — re-running `gentle-ai install` is idempotent.

## Requirements

- Git for Windows must be installed (provides Git Bash)
- The shim is Windows-only — macOS and Linux are unaffected

## Known Limitations & Future Iterations

The following items were identified during verification and deferred for future work.
They are not bugs — GGA works correctly for the common case. These are improvements
worth revisiting.

### Iteration 1 — Argument forwarding with quoted spaces (W-01)

The shim uses:
```powershell
& $gitBash -c "gga $args"
```

Arguments with embedded quotes or spaces are passed via string interpolation into
`bash -c`, which can lose quoting fidelity in edge cases. For example:

```powershell
gga commit -m "my message"   # may arrive as: gga commit -m my message
```

**Recommended fix**: use `@args` splatting or construct the argument array explicitly
instead of string interpolation.

### Iteration 2 — Git Bash not-found error surface (W-02)

The original spec described surfacing a "Git Bash not found" error **during
`gentle-ai install`**. In the final design this was moved to **runtime** — the `.ps1`
shim detects Git Bash when the user first runs `gga`. The spec scenario is now
inaccurate and should be updated to reflect the runtime detection model.

**Recommended fix**: update `openspec/changes/gga-powershell-support/specs/gga/spec.md`
to rename the scenario from "install-time" to "runtime detection", and add an
integration test that exercises the not-found code path at PS runtime.

### Iteration 3 — Non-Windows guard test coverage (W-03)

The call-sites in `internal/cli/run.go` and `internal/cli/sync.go` guard the shim
with `if runtime.GOOS == "windows"`. This is verified structurally (the guard
exists in the source) but there is no automated test that simulates a non-Windows
OS and asserts that `EnsurePowerShellShim` is never called.

**Recommended fix**: add a table-driven test that injects a fake `GOOS` value and
asserts the shim install path is skipped on `linux` and `darwin`.
