# Retained GGA integrity before runtime launch

A private ON stage records optional `gga: true` in its installation manifest.
OFF/absence omits it, preserving the exact historical manifest serialization.
The ordinary POSIX manifest reader rejects a Shared GGA marker and verifies the
retained private payload before continuing to supervisor/Node qualification.

One authenticated member recipe serves both initial writes and later checks.
The fixed original source archive (0600) must still match its size/SHA; every
stamped CLI/library member (0700) must match the reconstructed recipe. Private
0700 directories, exact layout, single links and stable identities are checked.
Extra files, aliases, missing/tampered data or mode drift are refused, not repaired.

`TestUserGGARetainedIntegrity` uses the authentic archive in the existing offline
Guest; readback never changes the stage. Manifest DATA controls stop before
native supplier/supervisor qualification, rather than fabricating an installation.
This is custody proof only: no GGA PATH, public binding, provider command, model,
authentication, hook, installed OS acceptance or Ready claim. Rollback removes
this optional manifest check and recipe reuse, retaining config/acquisition units.

Reconciliation composes the bounded descriptor reader directly into this
production consumer; no intermediate unlimited archive read is admitted. The
complete retained-integrity controls accompany it. Additional owned replacement
fixtures remain in the next characterization unit, not removed or waived. No
Guest, archive, CLI or installation was executed for this recovered candidate.
