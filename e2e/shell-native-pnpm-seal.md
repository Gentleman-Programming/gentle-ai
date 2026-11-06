# Complete PNPM code seal

`PNPMCodeSeal` covers every PNPM directory, regular file and symbolic link,
including transitive dependencies absent from the selected entrypoint readback.
It hashes deterministic relative paths, modes, file bytes and literal link targets.
Source ownership and physical file guards apply; large files use streaming hashes.

Compare the result with a trusted staging seal. Computing a hash does not establish
supplier authenticity or safe link resolution. The staging transaction owns those
checks; the existing target readback remains required after publication.

Mutable HOME, agent sessions, configuration and project state are deliberately
outside this code seal. The copy transaction must verify them separately.

Run `go test -tags=shell_pnpm_seal -run '^TestUserPNPMCodeSeal$' ./internal/shellinstaller`
only in the qualified private sandbox with original DATA read-only at `/out`.
The regression covers added, removed and changed dependencies, modes and links.
Rollback removes this API, its test and this note; no launch path is activated.
