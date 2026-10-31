# Closed global response replay

This is a Guest-only TLS transport qualification, not PNPM resolution,
installation, publisher verification, publication, or three-OS acceptance.
The caller must establish UID1000/caps0/NNP/private network and immutable RO custody.

Supply `/global-fixture/requests.json`: ≤512 unique routes, ≤256KiB inventory.
Each body must be regular, non-symlink, ≤64MiB and match its size/SHA256.
Unknown routes receive404 before lookup; rejected known bodies receive502.
There is no upstream. Four request workers join on close; owner stop interrupts
open connections, including incomplete handshakes/headers, before joining.

Reports keep32 ASCII-safe samples per bucket, ≤256 bytes each, complete counts
and explicit sample incompleteness; report size remains ≤32KiB.
`TestUserNativeGlobalMirror` uses authenticated Node and genuine TLS:40 original
responses, unknown404 including non-ASCII raw targets, alias502 and four pending
header connections at owner stop. It rechecks Node/project preservation and
retains failures. The seven-response persistence oracle remains separate.
Run `-tags=shell_native_runtime -run '^TestUserNativeGlobalMirror$'` in the
qualified Guest. Full original installation is the next independent unit.

## Reuse persistence bodies without duplicating storage

An optional fourth mirror argument supplies an absolute, canonical secondary
fixture directory. Its regular, single-link `requests.json` must contain exactly
seven unique URLs already in the global index, with identical file, size and
SHA256 fields. The global index remains the sole route authority.

Only these bodies use secondary storage; authentication failure never falls back
to the primary directory. Both roots require immutable RO custody in real runs.
`TestUserNativeGlobalMirrorShared` checks seven shared bodies plus one primary,
same-size corruption and body aliases (502), unknown URLs (404), and specific
startup refusals for conflicting metadata and an aliased secondary index.
Run `-tags=shell_native_runtime` with
`-run '^TestUserNative(GlobalMirror(Shared)?|PersistenceMirror)$'` in the same
qualified Guest to include both existing replay regressions. This does not
qualify an installer run or publication.
