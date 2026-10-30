# Process-owned native handoff

A process-owning CLI may call `RunUserEntryReplacingOwner` only after restoring
its terminal. Fresh Linux amd64 Separate installation replaces the current
process with its loaded executable through `/proc/self/exe`, retaining the PID
and standard descriptors. This discards the previous TUI runtime rather than
keeping another Go process alive beside installation.

The replacement receives exactly one `GOMAXPROCS=1` entry and the original
selection, including its experience profile and physical confirmation. Its
absolute deadline is capped at 140 seconds or the earlier caller deadline;
re-entry revalidates the deadline and fresh selection before creating state.
This is not an OS-thread ceiling or an increase to resource limits.

Existing `RunUserEntry` remains in-process, including arbitrary context
cancellation. Custom streams, Shared/resume and other platforms keep that path.
Callers that need their stack, deferred functions or cancellation callbacks
must not opt into process replacement. Internal selectors grant no authority.

## Ordinary TUI

After its single Bubble Tea program restores the terminal, the ordinary app
calls `RunShellInstallSelectionProcess`. It rechecks physical consent through
the shared selection-execution boundary. Cancelled,
stale and invalid selections never reach the replacement boundary. This does
not add another TUI program or another waiting Go parent process.
