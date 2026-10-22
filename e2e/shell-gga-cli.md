# Fixed opt-in GGA CLI stage

GGA ON in a private Separate profile acquires the fixed source commit
`1124c3672f082c56b033c4e23a30e95d0e8cd593` before any Shared mutation.
OFF/absence acquires nothing. The cold TLS client has no proxy, decompression,
cookies or redirects; the bounded 81841-byte archive must match SHA256
`fb681f48706be5ee8f7d1efd558fdfacbc7757aaf4ca9d8fd05fce4c44d7d004`.

Four exact authenticated members become `TARGET/runtime/gga/bin/gga` and
`lib/{providers,cache,pr_mode}.sh`. The deterministic upstream tag-stamping recipe
sets literal `VERSION="2.10.1"`; no environment-provided version claim. Keep the
original `source.tgz` (0600) beside the private 0700 directories/files. Rooted
exclusive writes, directory pins, sync and byte/mode custody readbacks apply.

## Functional proof

In the existing offline Guest, `go test -tags=shell_gga_canonical -run '^TestUserGGA'
./internal/shellinstaller` uses the authentic retained archive. Tampered bytes
fail before creating a payload. Actual staged CLI prints `gga v2.10.1`, ignoring
an untrusted version override; repeated provisioning refuses replacement and
preserves the tree. ON/OFF/absent native configuration controls still pass.
This proves the recipe/materialization, not live Go HTTP acquisition or installed
Linux/Darwin/Windows acceptance. No provider command/authentication or git hook.

Public profile admission stays refused. No PATH activation or public binding is
added: managed launch/retained integrity checks must be qualified before exposing
the optional CLI. Source TLS/commit/hash pins are not independent SLSA/Sigstore
release provenance. Rollback removes only this opt-in acquisition/materializer,
its call before global provisioning and tests; keep the canonical config unit.

Recovery retains all source031 assertions without rerunning this Guest, archive
or CLI. The recipe subtest uses the existing owned-process-group helper and its
bounded pipe drain, keeping the original 45-second aggregate context. Early
acquisition/materialization cancellation preserves `ctx.Err()`; untagged controls
assert both cancellation and deadline causes before any filesystem write. These
new candidate controls are not a recorded local GREEN or installed acceptance.
